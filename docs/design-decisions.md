# Design Decisions

## Backend: Go, not Go-only
The reliability/concurrency requirement (atomic claiming, concurrent execution, graceful shutdown, worker pools) is the core engineering challenge here and is worth 15+20=35 of the 100 grading marks combined with architecture. Go's goroutines/channels and mature Postgres/Kafka drivers map directly onto that without a framework doing it for us — which keeps the concurrency logic visibly our own engineering rather than hidden inside a library (e.g. Celery, BullMQ), which is what the assignment is grading. Existing auth/org/project code (`cmd/api`) and the core job pipeline stay Go by default, but a new component can use a different language if it's a meaningfully better fit for that component (e.g. a stronger library for one job-executor type) — robustness of the overall system outranks stack uniformity. Any such choice gets called out here with the tradeoff (extra runtime to build/deploy), not introduced silently.

## PostgreSQL as source of truth
Chosen over MySQL for `SELECT/UPDATE ... WHERE ... RETURNING`, native `jsonb` (job payloads are arbitrary per job `type`), and strong constraint/index support for the required entity list. It is the single authority on job state — every other component (RabbitMQ, Redis) is a cache or transport in front of it, never a second source of truth. This avoids split-brain between "what RabbitMQ thinks is queued" and "what actually happened," which is the most common reliability bug in job systems.

## CAP tradeoff: availability over consistency
The assignment's NFRs call this out explicitly (10K jobs/sec, at-least-once execution, 2s scheduling latency — see [architecture.md](./architecture.md#non-functional-requirements)). We favor a component staying up and making progress (Kafka partitions, Watcher polling, workers claiming) over every reader seeing the latest job status instantly. Postgres remains the single strongly-consistent source of truth for job state; everything in front of it (Kafka, Redis) is allowed to be eventually consistent/stale by design, which is why the atomic-claim step against Postgres — not message delivery — is what actually prevents duplicate execution.

## Kafka as transport, not authority
Kafka gives push-style delivery at the required throughput (10K jobs/sec) and partitioned topics (`run`/`retry`/`dead`) map onto the job lifecycle's happy path, retry path, and dead-letter path. Kafka only guarantees *at-least-once* delivery, so it cannot be trusted alone to prevent duplicate execution — every worker still performs an atomic conditional `UPDATE` against Postgres before treating a message as claimed (see [architecture.md](./architecture.md#job-lifecycle)). This is deliberately more infrastructure than a pure DB-backed queue (Postgres `SKIP LOCKED` alone would have worked), chosen because the assignment explicitly rewards a "production-inspired" design and lists event-driven execution as a bonus feature — but the atomic-claim logic is kept regardless, so if Kafka were ever removed the correctness guarantee wouldn't change, only the latency would.

## Redis: broader than a pure cache, still not a source of truth
Redis holds heartbeats, in-flight concurrency counters, the Watcher Service's polling watermark (`last_polled_time`), and a short-TTL lookup for pending cancel requests — more uses than "heartbeats only," but all of them are either disposable liveness/rate data or a fast-path cache in front of a Postgres column, never a value that only exists in Redis. Losing Redis loses latency (Watcher falls back to scanning further back, cancel checks fall back to a DB read), not correctness.

## Scheduler as its own service
Polling "what's due" and expanding cron schedules is cheap but must not compete with API request handling or run multiple times concurrently (which would double-publish jobs). Splitting it out lets it run as a single leader-elected process (via a Redis lock) independent of how many API/worker replicas exist, and keeps the "when does a job become ready" logic in one place instead of duplicated between delayed-job and cron-job code paths.

## Frontend: React + Vite
A plain SPA talking to the REST API is the right amount of frontend for a dashboard whose job is to display and act on backend state (queues, jobs, workers, logs) — no SSR/SEO need exists here, so Next.js's extra structure buys nothing. Live updates start as polling (simplest thing that works); WebSockets are listed as a bonus and can be layered on the same API without a rewrite if time allows.

## What's deliberately deferred
Bonus items (not core requirements):
- Workflow dependencies (job DAGs), queue sharding, and AI failure summaries — not designed in detail here; the schema (`jobs.type`, `jsonb payload`) doesn't preclude adding a `job_dependencies` table later.
- RBAC beyond `org_members.role` (owner/member) — left coarse until core scheduling is solid.
- Cron parsing, web dashboard, DLQ viewer, metrics, distributed tracing — per the MVP build workflow's own skip list.

**MVP bootstrap ledger — graded core requirements, deferred to get one job running end-to-end first, not dropped.** Each is a straightforward additive migration/change on top of the MVP schema (`jobs`/`job_runs`), not a rewrite — the flat schema was chosen so this stays true. Remind the user to schedule these once the MVP checkpoints (Layers 0-8 of the build workflow) pass:
- **Project/queue scoping** — `jobs` is flat/global in the MVP; add `queue_id`/`project_id` FKs plus a `queues` table (priority, concurrency_limit, pause/resume) once single-tenant flow works. This is what makes "each project owns multiple job queues" true.
- **`retry_policies` table** — MVP only has `retries_count`/`retries_max` (fixed cap), no configurable strategy. Add a `retry_policies` table (fixed/linear/exponential + delay params) and a `retry_policy_id` FK on `jobs`.
- **`workers` / `worker_heartbeats` tables** — MVP's `job_runs` doesn't record which executor instance ran an attempt, and there's no heartbeat/liveness tracking. Add worker identity + a heartbeat table once there's more than one `job-executor` instance to distinguish.
- **`job_logs` table** — MVP only has a single `err_msg TEXT` on `job_runs`. Add structured per-attempt execution logs once the executor does more than a single HTTP call/sleep.
- **`dead_letter_queue` table** — MVP marks dead jobs via `jobs.status='dead'` with no separate audit record. Add a real DLQ table (final error, original queue, moved_at) once dead-lettering needs to be browsable/replayable, not just a status flag.
- **Auth on job-service routes** — MVP's job endpoints are unauthenticated (per the workflow's own skip list) while `cmd/api`'s existing JWT auth stays untouched. Wire job-service routes under the existing auth once the core Kafka flow (create → dispatch → execute → retry/dead) passes checkpoint 8.
