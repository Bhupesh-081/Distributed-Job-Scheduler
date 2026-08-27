# API Design

REST, JSON bodies, JWT bearer auth (except `/auth/*`). All list endpoints support `?page=&page_size=` (cursor or offset - TBD at implementation) plus relevant `?filter=` params. Errors return a consistent envelope: `{"error": {"code": "...", "message": "...", "details": {...}}}` with standard HTTP status codes.

## Auth
| Method | Path | Notes |
|---|---|---|
| POST | `/auth/register` | Create user; also emails a 6-digit verification code (doesn't block the session it returns) |
| POST | `/auth/login` | Returns JWT |
| POST | `/auth/refresh` | Refresh token |
| POST | `/auth/logout` | Revokes the presented refresh token |
| POST | `/auth/verify-email` | `{email, code}`; marks `users.email_verified` |
| POST | `/auth/resend-verification` | `{email}`; always 200 (doesn't reveal whether the address exists or is already verified) |
| POST | `/auth/forgot-password` | `{email}`; always 200, same non-enumeration reasoning; emails a reset code if the account exists |
| POST | `/auth/reset-password` | `{email, code, new_password}`; on success revokes every refresh token for that user |
| GET | `/auth/me` | Current user's email, verification status, display name, created_at |
| PATCH | `/auth/me` | `{display_name}` (60 chars max, empty string clears it) |
| POST | `/auth/change-password` | `{current_password, new_password}` for a logged-in user (vs. the OTP-based reset above for a locked-out one); also revokes every refresh token, including the caller's own |

## Organizations / Projects
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/organizations` | |
| GET/PATCH/DELETE | `/organizations/:orgId` | |
| POST | `/organizations/:orgId/members` | Invite/add member (RBAC bonus) |
| GET/POST | `/organizations/:orgId/projects` | |
| GET/PATCH/DELETE | `/projects/:projectId` | |

## Queues
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/projects/:projectId/queues` | Create sets name/priority/concurrency_limit; `default_retry_policy_id` set separately via PATCH |
| GET/PATCH/DELETE | `/queues/:queueId` | PATCH replaces name/priority/concurrency_limit/default_retry_policy_id; DELETE 409s while jobs still reference the queue |
| POST | `/queues/:queueId/pause` | |
| POST | `/queues/:queueId/resume` | |
| GET | `/queues/:queueId/stats` | job counts by status for the queue |

## Retry policies
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/projects/:projectId/retry-policies` | `strategy`: fixed/linear/exponential, `base_delay_seconds`, optional `max_delay_seconds` cap |
| GET/PATCH/DELETE | `/retry-policies/:id` | A job's `retry_policy_id` overrides its queue's `default_retry_policy_id`; neither set falls back to a fixed 5s delay |

## Script library
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/projects/:projectId/scripts` | `script_type`: python/bash, `content` (the script body) |
| PATCH/DELETE | `/scripts/:id` | Not referenced by jobs at the database level - the dashboard's "load from library" copies `content` into the job payload at creation time, same as picking any other job type |

## Jobs
Served by job-service (separate binary from `cmd/api`, sharing its JWT secret - a token from `/auth/login` works on both). Every route requires `Authorization: Bearer` and is scoped by the target job's `queue_id` → the caller's org membership; a job/queue outside the caller's orgs 403s (or 404s for a legacy pre-auth job with no `queue_id` at all).
| Method | Path | Notes |
|---|---|---|
| POST | `/jobs` | `scheduled_type`: immediate/delayed/scheduled; `queue_id` required, `retry_policy_id` optional override |
| POST | `/jobs/batch` | Bulk create, all-or-nothing |
| GET | `/jobs` | `queue_id` required (auth needs a single queue to scope against), optional `status` filter; paginated |
| GET | `/jobs/:jobId` | |
| GET | `/jobs/:jobId/logs` | Chronological (oldest-first) log stream: claim, output, outcome per attempt |
| POST | `/jobs/:jobId/cancel` | Cancels outright if still queued; else sets a Redis flag consumer-service polls mid-execution |

## Recurring jobs
Served by `cmd/api` (authenticated, scoped via the queue's org - same as queues/retry-policies), on `internal/cronexpr` (standard 5-field cron, e.g. `*/5 * * * *`). A `scheduled_jobs` row is a template, never itself run - watcher-service expands due ones into ordinary jobs (`GET /jobs/{id}.scheduled_job_id` traces a job back to its definition).
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/queues/:queueId/scheduled-jobs` | `cron_expression` + `payload` template, optional `retries_max`/`retry_policy_id` |
| GET/PATCH/DELETE | `/scheduled-jobs/:id` | PATCH replaces cron_expression/payload/retries_max/retry_policy_id and recomputes `next_run_at`; DELETE doesn't touch jobs already spawned |
| POST | `/scheduled-jobs/:id/pause` \| `/resume` | Reversible on/off; a paused definition just stops generating new jobs |

## Workers
| Method | Path | Notes |
|---|---|---|
| GET | `/workers` | `?status=active\|stopped`. Not project-scoped - shared infra, any authenticated user |
| GET | `/workers/:id` | Detail + last 20 heartbeats (in_flight_count over time) |

## Dead Letter Queue
| Method | Path | Notes |
|---|---|---|
| GET | `/queues/:queueId/dlq` | Newest-first; entries are an append-only audit log (a replayed-then-failed job gets a second entry) |
| GET | `/dlq/:id` | |
| POST | `/dlq/:id/replay` | Re-queues the original job (status, retries_count, dispatched_at reset) and deletes the entry |
| DELETE | `/dlq/:id` | Discards the entry; job's `status='dead'` is untouched |

## System
| Method | Path | Notes |
|---|---|---|
| GET | `/system/health` | `cmd/api`'s own liveness, plus `watcher_service: {alive, last_poll_at, seconds_since_last_poll}` read from its Redis heartbeat (dead if unset or >30s stale). job-service has its own `/system/health` too (liveness only, no watcher status) - Caddy's default route sends `/system/health` to `cmd/api`. |
| GET | `/system/metrics` | Aggregate throughput/health for dashboard overview - **not implemented yet** |

Full OpenAPI spec to be generated from the Go handler annotations once implementation starts, rather than hand-maintained separately.
