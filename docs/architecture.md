# System Architecture

## Non-functional requirements

| Requirement | Target |
|---|---|
| Throughput | 10K jobs/sec |
| CAP tradeoff | Availability over consistency |
| Delivery guarantee | At-least-once — every job runs at least once |
| Scheduling latency | A job starts within 2s of its scheduled/due time |

These drive the topology below: Kafka over a DB-polling-only queue (throughput,
push delivery), watcher-service's 1s poll interval (latency SLA), and the
atomic-claim + idempotent-claim-guard design (at-least-once delivery must not
become duplicate execution).

## Stack

| Layer | Choice |
|---|---|
| Backend | Go |
| Primary datastore | PostgreSQL |
| Cache / signaling | Redis (two uses: the cancel-request flag and watcher-service's self-liveness heartbeat — see below) |
| Message broker | Kafka |
| Frontend | React + Vite (not started) |

Rationale for these choices lives in [design-decisions.md](./design-decisions.md).

## Components

Four Go binaries, each `cmd/<name>`, sharing `internal/{db,store,kafka,executor,cancel}`:

```mermaid
flowchart LR
    Client[Client]
    Caddy[Caddy — HTTPS termination]

    subgraph api_svc["cmd/api"]
        API[REST: auth, orgs, projects,\nqueues, retry-policies, workers, DLQ]
    end

    subgraph job_svc["cmd/job-service"]
        JS[REST: jobs — create, list, get,\nlogs, cancel]
    end

    subgraph watcher_svc["cmd/watcher-service (single instance)"]
        WATCH[1s poll tick]
    end

    subgraph consumer_svc["cmd/consumer-service — N instances = Worker fleet"]
        C1[worker]
        C2[worker]
    end

    PG[(PostgreSQL — source of truth)]
    REDIS[(Redis — cancel-flag,\nwatcher-service heartbeat)]
    KAFKA{{Kafka: run / retry topics}}

    Client -->|HTTPS| Caddy
    Caddy -->|"/jobs*"| JS
    Caddy -->|everything else| API

    API -->|read/write| PG
    API -->|read watcher heartbeat\n(GET /system/health)| REDIS
    JS -->|read/write| PG
    JS -->|set cancel flag| REDIS

    WATCH -->|due jobs, stuck-job recovery,\nstale-worker reap| PG
    WATCH -->|report own liveness\nevery tick| REDIS
    WATCH -->|publish due job| KAFKA

    KAFKA -->|deliver| C1
    KAFKA -->|deliver| C2

    C1 -->|atomic claim, execute,\nlog, heartbeat| PG
    C2 --> PG
    C1 -->|poll cancel flag| REDIS
    C2 --> REDIS
    C1 -->|republish after retry delay| KAFKA
```

## Services

- **`cmd/api`** — auth (register/login/refresh/logout), organizations, projects, queues (CRUD + pause/resume/stats), retry policies (CRUD), workers (read-only status/heartbeat history), dead letter queue (list/get/replay/delete). Every route past `/auth/*` requires a JWT bearer token; queue/retry-policy/worker/DLQ routes additionally check the caller is a member of the owning org (via the org → project → queue chain — see [auth-workflow.md](./auth-workflow.md)).
- **`cmd/job-service`** — a separate binary/mux from `cmd/api`, but sharing its JWT secret, so one login token works on both. Owns `/jobs*`: create (single + batch), list, get, cancel, and the per-job log stream. Every job must belong to a `queue_id`; access is scoped by that queue's org, same as `cmd/api`'s queue routes. Split out from `cmd/api` so job-creation load (the hot path at the assignment's 10K/sec target) doesn't compete with org/project-management traffic, and so it can scale independently.
- **`cmd/watcher-service`** — single instance (no leader election — see `ponytail:` note in its source; a second replica would double-publish). One 1s ticker does five things every tick, in this order: (1) `RecoverStuckJobs` — resets `dispatched_at` on jobs that were dispatched but haven't progressed in >30s (lost Kafka message, or a claim blocked by a full/paused queue — see below), so they're picked up again; (2) `RecoverStuckRunningJobs` — requeues (or dead-letters, if retries are exhausted) any job whose worker stopped touching it for >30s, i.e. crashed mid-execution; (3) `ReapStaleWorkers` — marks `workers` rows `stopped` if their heartbeat is >30s old (a worker that crashed without running its graceful-shutdown path); (4) `ExpandDueScheduledJobs` — spawns a new `jobs` row for every active `scheduled_jobs` (cron) definition whose `next_run_at` has passed, and advances `next_run_at`; (5) `DispatchDueJobs` — publishes queued, due, undispatched jobs (including ones just spawned or requeued by steps 2 and 4) to Kafka's `run` topic, ordered by queue `priority` (desc) then `scheduled_time`, skipping paused queues.
- **`cmd/consumer-service`** — the assignment's Worker service. Horizontally scaled: run as many instances as needed, each independently registering itself in `workers` and consuming both the `run` and `retry` Kafka topics (as two separate `Reader`s in distinct consumer groups — `consumer-service-run` / `consumer-service-retry` — since one kafka-go group can't subscribe to two different single topics). A per-instance semaphore (`CONCURRENCY` env var, default 5) bounds how many jobs *that instance* runs at once; a Postgres advisory lock in `ClaimJob` separately bounds how many jobs run *for a given queue across every instance* at once, against that queue's `concurrency_limit`.
- **Kafka topology** — two topics actually used: `run` (due jobs from watcher-service) and `retry` (failed jobs republished by the consumer that failed them, after their resolved retry delay). A third constant, `dead`, exists in `internal/kafka` but nothing publishes or consumes it — dead-lettering is a direct Postgres write (a `dead_letter_queue` row) from consumer-service, not a Kafka message; see [design-decisions.md](./design-decisions.md) for why Kafka is a transport/trigger, never the authority.
- **Redis** — two real uses, both disposable signals, never authoritative state: (1) `internal/cancel`'s `cancel:{jobID}` key, set by job-service on `POST /jobs/{id}/cancel` and polled every 2s by consumer-service during execution; (2) `internal/heartbeat`'s single `watcher:last_poll` key, refreshed by watcher-service every tick and read by `cmd/api`'s `GET /system/health`, which reports the watcher dead if that key is missing or older than 30s. Everything else that might look like a Redis job (worker liveness, per-queue concurrency counters, the due-job watermark) is a Postgres column instead — see `workers.last_heartbeat_at`, the running-count query in `ClaimJob`, and `jobs.dispatched_at`. Losing Redis loses the ability to cancel a *running* job early (it just finishes instead) and the watcher's health signal (`GET /system/health` would report it dead even if it's fine) — nothing that affects job correctness.

## Job lifecycle

`jobs.status` (Postgres `CHECK` constraint): `queued | running | success | failed | dead | cancelled`. `failed` is transient — `RetryOrDeadLetter` immediately resolves it back to `queued` (retry) or forward to `dead` (exhausted) in the same statement, so it's never actually visible as a row's status; job history instead records each attempt as a `job_runs` row with its own transient `failed` state.

```
queued ──(watcher-service dispatches to Kafka "run")──> [claimed by a worker]──> running ──┬──> success
  ▲                                                                                          │
  │                                                                              (attempt fails, retries left)
  │                                                                                          │
  └── consumer-service republishes to Kafka "retry" after the resolved retry-policy delay ──┘
                                                                                              │
                                                                                    (attempt fails, retries exhausted)
                                                                                              ▼
                                                                              dead + a dead_letter_queue row

queued ──(POST /jobs/{id}/cancel, still unclaimed)──────────────────────────────────────> cancelled
running ─(POST /jobs/{id}/cancel → Redis flag → consumer-service polls mid-execution)────> cancelled
```

## Data flow: creating and running a job

1. `POST /jobs` (job-service) validates the payload and `queue_id`, checks the caller is a member of that queue's org, and inserts a `jobs` row: `status='queued'`, `scheduled_time` = now (immediate), now+N (delayed), or the given future time (scheduled). `scheduled_type='recurring'` is rejected here — a single job can't be "recurring" on its own; a **`scheduled_jobs`** row (a cron *definition*, created via `POST /queues/{id}/scheduled-jobs`) is what's recurring. watcher-service expands due `scheduled_jobs` into new, ordinary `jobs` rows (`scheduled_type='immediate'`, `scheduled_job_id` traces it back) every tick, before dispatching due jobs — see "Recurring (cron) jobs" below. From that point on, a cron-spawned job is indistinguishable from any other and goes through the rest of this flow unchanged.
2. watcher-service's next tick finds it (due, undispatched, queue not paused), sets `dispatched_at=now()`, and publishes the job ID to Kafka's `run` topic.
3. A consumer-service instance's `run`-topic reader fetches the message and hands it to a goroutine gated by its semaphore.
4. `ClaimJob` (one Postgres transaction): reads the job's `queue_id`, takes `pg_advisory_xact_lock(hashtext(queue_id))` — serializing concurrent claims for *that queue* across every worker instance without blocking any other queue — then checks the queue isn't paused and its current running-job count is under `concurrency_limit`, then does `UPDATE jobs SET status='running' WHERE id=$1 AND status='queued'`. That last `WHERE` guard is what actually prevents duplicate execution from Kafka's at-least-once redelivery — the message itself is just a trigger. If the pause/concurrency check fails, the transaction rolls back with nothing changed: the job stays `queued` with `dispatched_at` still set, so it's picked up again by watcher-service's stuck-job recovery once the queue has room — no separate backoff path needed for that case.
5. On a successful claim: insert a `job_runs` row (`attempt_number`, `worker_id`), log "claimed" to `job_logs`, run the payload via `internal/executor` (a shell command via `exec.CommandContext` — no shell interpolation), polling the Redis cancel flag every 2s.
6. On completion, log the captured output and finish the `job_runs` row, then:
   - **cancelled** — `jobs.status='cancelled'`, clear the Redis flag.
   - **success** — `jobs.status='success'`.
   - **failure** — `RetryOrDeadLetter` atomically increments `retries_count` and sets `status` to `queued` (retry) or `dead` (exhausted) based on `retries_max`. On `dead`, a `dead_letter_queue` row is written directly (final error, queue snapshot, retry count) and a `job_logs` entry records it. On retry, the effective retry policy (job override → queue default → a hardcoded 5s fallback if neither is set) computes the delay, and consumer-service schedules an in-process `time.AfterFunc` to republish the job ID to Kafka's `retry` topic after that delay.
7. **Fallback safety net — now covers every crash point.** `RecoverStuckJobs` (same watcher-service tick) resets any job that's `status='queued'` with `dispatched_at` set and a stale `modified_time` — the same code path that recovers a lost Kafka message, a concurrency/pause-blocked claim, and a worker that crashed after `RetryOrDeadLetter` requeued a job but before its retry timer fired. `RecoverStuckRunningJobs` (same tick, right before it) covers the case that leaves: a worker crashing *between* claim and finish, `status='running'`. Both queries key off `jobs.modified_time`, but a `running` job's `modified_time` isn't just set once at claim time — consumer-service refreshes it every 2s while a job actively executes (`TouchRunningJob`, piggybacked on the existing cancel-flag poll), so "stale" genuinely means "no worker is touching this," not "this payload is taking a while." A caught crash is treated exactly like a real execution failure: the orphaned `job_runs` row is closed out (`status='failed'`, a `job_logs` entry recorded) and `retryOrDeadLetter` makes its usual queued-vs-dead call — a payload that reliably kills its worker (OOM, etc.) dead-letters eventually instead of retrying forever, same as any other repeated failure.

## Recurring (cron) jobs

A `scheduled_jobs` row is a cron *definition* (queue, name, standard 5-field cron expression, payload template, retries config, `active` flag, `next_run_at`/`last_run_at`) — it is never itself dispatched, claimed, or executed. It only exists to generate ordinary `jobs` rows on a schedule:

1. `POST /queues/{id}/scheduled-jobs` (`cmd/api`) validates the cron expression (`internal/cronexpr`, a thin wrapper over `robfig/cron/v3`'s standard parser — chosen over hand-rolling cron math, which is a well-known source of leap-year/month-end/DST bugs) and the payload, computes the first `next_run_at` (`cronexpr.Next(expr, now())`), and inserts the definition.
2. Every watcher-service tick, `ExpandDueScheduledJobs` runs in one transaction per batch: `SELECT ... WHERE active AND next_run_at <= now() FOR UPDATE SKIP LOCKED` (the same locking pattern as `DispatchDueJobs`/`RecoverStuckJobs`, so it's safe if this service is ever run with more than one instance), then for each due row: `INSERT INTO jobs (..., scheduled_type='immediate', scheduled_time=now(), scheduled_job_id=...)` and advance `next_run_at` to `cronexpr.Next(expr, now())` — both in the same transaction, so a crash between the two can't happen (either both landed, or neither did).
3. The spawned job is now an ordinary job with `dispatched_at IS NULL` and `scheduled_time <= now()`, so `DispatchDueJobs` (running right after, same tick) picks it up immediately — no extra latency beyond the normal dispatch path, and no code anywhere in the claim/execute/retry/DLQ pipeline needs to know a job came from a cron firing. `GET /jobs/{id}`'s `scheduled_job_id` field is the only trace back to its origin.
4. Deleting a `scheduled_jobs` definition (`DELETE /scheduled-jobs/{id}`) does not delete jobs it already spawned (`jobs.scheduled_job_id` is `ON DELETE SET NULL`) — only stops future firings. `POST /scheduled-jobs/{id}/pause` (sets `active=false`) is the reversible version of the same thing.

## Concurrency & reliability guarantees

- **Atomic claim** — the conditional `UPDATE ... WHERE status='queued'` inside `ClaimJob`'s transaction is the single source of truth for "who owns this job." Kafka delivery is a low-latency trigger, never the authority (see [design-decisions.md](./design-decisions.md)).
- **Concurrency limits, enforced at two independent layers** — an in-process semaphore per consumer-service instance (soft, that instance's own resource budget) and a Postgres advisory-lock-guarded count against `queues.concurrency_limit` inside `ClaimJob` (hard, cluster-wide across every instance for that queue).
- **Idempotency** — job handlers are expected to be safe to re-run (documented per `internal/executor`'s `ShellPayload` contract), because at-least-once is a property of the whole pipeline, not just Kafka: a job stuck `running` after its worker crashed *will* re-execute as a second attempt, via `RecoverStuckRunningJobs` (see above). There's also no DB-level uniqueness constraint backstopping this (e.g. no unique `(job_id, attempt_number)` on `job_runs` — attempt numbers are caller-assigned, `job.RetriesCount + 1`, not DB-generated), so the claim guard is the only thing standing between "duplicate delivery" and "duplicate execution."
- **Graceful shutdown** — SIGTERM stops each consumer-service's Kafka fetch loops, waits for in-flight jobs via a `sync.WaitGroup`, then marks itself `stopped` in `workers` before exiting. An ungraceful crash (kill -9, OOM) instead relies on `RecoverStuckRunningJobs` noticing the missing heartbeat touch within `staleAfter` (default 30s) — live-verified: a job whose worker was `kill -9`'d mid-execution was requeued and re-executed by a different worker instance, while a job that simply ran longer than the threshold (but stayed alive, still touching) completed normally with zero false recovery.
- **Worker liveness** — each consumer-service instance heartbeats to `workers`/`worker_heartbeats` every 10s (with its current in-flight job count); watcher-service reaps (`status='stopped'`) any worker whose heartbeat is >30s stale. That's a separate signal from the per-job touch above (`TouchRunningJob`, every 2s) — a worker can miss its 10s heartbeat cycle without necessarily meaning the job it's running is abandoned, so the two recovery paths (`ReapStaleWorkers` for the worker row, `RecoverStuckRunningJobs` for the job it was running) are intentionally independent rather than one triggering the other.
- **Priority** — enforced at dispatch, not claim: when there's more due work in one tick than `DispatchDueJobs`' batch limit, higher-`priority` queues are published to Kafka first. It doesn't reorder work already sitting in Kafka.
