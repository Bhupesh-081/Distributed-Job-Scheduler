# System Architecture

## Stack

| Layer | Choice |
|---|---|
| Backend | Go |
| Primary datastore | PostgreSQL |
| Cache / heartbeats / locks | Redis |
| Message broker | RabbitMQ |
| Frontend | React + Vite |

Rationale for these choices lives in [design-decisions.md](./design-decisions.md).

## Components

```mermaid
flowchart LR
    subgraph Client
        UI[React Dashboard]
    end

    subgraph API_Service["API Service (Go)"]
        REST[REST API]
    end

    subgraph Scheduler_Service["Scheduler Service (Go)"]
        SCHED[Scheduler loop]
    end

    subgraph Worker_Fleet["Worker Service (Go) — N instances"]
        W1[Worker]
        W2[Worker]
        W3[Worker...]
    end

    PG[(PostgreSQL\nsource of truth)]
    REDIS[(Redis\nheartbeats, locks, rate limits)]
    MQ[[RabbitMQ\nper-queue work queues + DLX]]

    UI -->|HTTPS REST| REST
    REST -->|read/write| PG
    REST -->|publish ready jobs| MQ

    SCHED -->|poll scheduled_for <= now\n+ expand cron| PG
    SCHED -->|publish due jobs| MQ

    MQ -->|deliver job message| W1
    MQ -->|deliver job message| W2
    MQ -->|deliver job message| W3

    W1 -->|atomic claim (UPDATE ... WHERE status='queued')| PG
    W2 --> PG
    W3 --> PG

    W1 -->|heartbeat, in-flight counters| REDIS
    W2 --> REDIS
    W3 --> REDIS

    MQ -->|dead-letter exchange\n(exhausted retries)| DLQ_C[DLQ consumer]
    DLQ_C -->|persist| PG
```

## Services

- **API Service** — stateless REST API. Owns auth, project/queue/job CRUD, pagination/filtering, and read endpoints for the dashboard (queue stats, worker status, execution logs, DLQ). On job creation it writes the `jobs` row and, for immediate jobs, publishes directly to RabbitMQ. Delayed/scheduled/cron jobs are written with `status='scheduled'` and left for the Scheduler service to publish when due.
- **Scheduler Service** — single small Go service running a polling loop (~1s tick) over Postgres: finds `jobs` rows with `status='scheduled' AND scheduled_for <= now()` and publishes them; expands `scheduled_jobs` (cron) rows whose `next_run_at <= now()` into new `jobs` rows and advances `next_run_at`. Kept separate from the API so publish load and API request load don't compete, and so it can run as a single leader-elected instance (Redis lock) even if the API is horizontally scaled.
- **Worker Service** — horizontally scaled consumers. Each subscribes to RabbitMQ queues (one queue per job queue, bound with routing key = `queue_id`) with a prefetch count that enforces the queue's `concurrency_limit`. On message delivery it does **not** trust the message alone — it attempts an atomic conditional claim in Postgres (`UPDATE jobs SET status='claimed', claimed_by=$worker WHERE id=$job AND status='queued' RETURNING *`). Zero rows updated means another delivery already claimed it (RabbitMQ redelivery after a crash, etc.) — the worker just acks and moves on. This is what makes "exactly-once execution" hold even though RabbitMQ only guarantees at-least-once delivery.
- **RabbitMQ topology** — one exchange per project (or a single topic exchange with `queue_id` routing keys, simpler to operate), one queue per job queue, and a dead-letter-exchange bound to each queue's DLX so that a `basic.reject`/`nack` after exhausting retries routes the message to a DLQ consumer, which writes a `dead_letter_queue` row. Retry backoff between attempts is implemented via a per-attempt "delay queue" (message TTL + DLX-back-to-work-queue trick), not a Rabbit plugin, to avoid an extra dependency.
- **Redis** — three uses only, no general-purpose cache: (1) worker heartbeats (`SET worker:<id>:hb EX 30`, cheap liveness checks without hammering Postgres), (2) per-queue in-flight counters for concurrency-limit enforcement across worker processes, (3) a distributed lock for Scheduler leader election.

## Job lifecycle

```
Queued ──(worker claims)──> Claimed ──> Running ──┬──> Completed
   ▲                                               │
   │                                          (failure, retries left)
   │                                               │
   └────────────── backoff delay ─────────────────┘
                                                    │
                                          (failure, retries exhausted)
                                                    ▼
                                              Dead Letter
```

`Scheduled` is a pre-state for delayed/cron jobs (`scheduled_for` in the future); the Scheduler service moves them to `Queued` and publishes at the right time.

## Data flow: creating and running a job

1. `POST /projects/:id/queues/:id/jobs` → API validates payload against the queue's config, inserts a `jobs` row.
   - Immediate job: `status='queued'`, publish to RabbitMQ now.
   - Delayed/scheduled job: `status='scheduled'`, `scheduled_for` set; Scheduler picks it up later.
   - Recurring (cron): a `scheduled_jobs` row is created/updated instead; each firing produces a new `jobs` row.
2. Worker receives the RabbitMQ message, runs the atomic claim UPDATE.
3. On successful claim: insert a `job_executions` row (`attempt_number` = current `jobs.attempt_count + 1`), set `jobs.status='running'`, execute the handler, stream logs to `job_logs`, refresh Redis heartbeat periodically during long-running work.
4. On success: `jobs.status='completed'`, finalize the `job_executions` row, ack the message.
5. On failure: increment `attempt_count`; if `< max_attempts`, compute the next delay from the queue/job's `retry_policies` row (fixed/linear/exponential) and requeue via the delay-queue trick; if attempts are exhausted, nack without requeue → RabbitMQ DLX → DLQ consumer inserts a `dead_letter_queue` row and sets `jobs.status='dead_letter'`.

## Concurrency & reliability guarantees

- **Atomic claim** — the conditional `UPDATE ... WHERE status='queued'` is the single source of truth for "who owns this job"; RabbitMQ delivery is just a low-latency trigger, not the authority.
- **Idempotent execution** — `job_executions` has a unique constraint on `(job_id, attempt_number)`, so even a duplicate claim attempt (should one ever slip through) can't create two execution records for the same attempt. Job handlers are expected to be idempotent by contract (documented per job `type`), since at-least-once delivery is a property of the whole pipeline.
- **Concurrency limits** — enforced twice: RabbitMQ consumer prefetch (soft limit, per worker) and a Redis in-flight counter checked at claim time (hard limit, cluster-wide).
- **Graceful shutdown** — workers stop consuming new deliveries on SIGTERM, finish in-flight jobs (or requeue them by nacking with requeue=true if they don't finish before a grace deadline), then exit.
- **Worker liveness** — a worker missing its Redis heartbeat past a TTL is considered dead; its `claimed`/`running` jobs are recoverable via a periodic reaper (Scheduler service) that resets stale claims back to `queued`.
