# API Design

REST, JSON bodies, JWT bearer auth (except `/auth/*`). All list endpoints support `?page=&page_size=` (cursor or offset — TBD at implementation) plus relevant `?filter=` params. Errors return a consistent envelope: `{"error": {"code": "...", "message": "...", "details": {...}}}` with standard HTTP status codes.

## Auth
| Method | Path | Notes |
|---|---|---|
| POST | `/auth/register` | Create user |
| POST | `/auth/login` | Returns JWT |
| POST | `/auth/refresh` | Refresh token |

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
| GET/POST | `/projects/:projectId/queues` | Create sets priority, concurrency_limit, default retry policy |
| GET/PATCH/DELETE | `/queues/:queueId` | |
| POST | `/queues/:queueId/pause` | |
| POST | `/queues/:queueId/resume` | |
| GET | `/queues/:queueId/stats` | throughput, in-flight, failure rate — backs dashboard charts |

## Jobs
| Method | Path | Notes |
|---|---|---|
| POST | `/queues/:queueId/jobs` | Body includes `type`, `payload`, and one of: immediate (default), `scheduled_for` (delayed/scheduled), or batch array |
| POST | `/queues/:queueId/jobs/batch` | Bulk create |
| GET | `/queues/:queueId/jobs` | Filter by `status`, `type`, date range; paginated |
| GET | `/jobs/:jobId` | Includes latest execution summary |
| GET | `/jobs/:jobId/executions` | Retry/attempt history |
| GET | `/jobs/:jobId/logs` | Execution logs (paginated/tailable) |
| POST | `/jobs/:jobId/retry` | Manually re-queue a failed/dead-lettered job |
| DELETE | `/jobs/:jobId` | Cancel a scheduled/queued job (not running/completed) |

## Recurring jobs
| Method | Path | Notes |
|---|---|---|
| GET/POST | `/queues/:queueId/scheduled-jobs` | Cron expression + payload template |
| PATCH/DELETE | `/scheduled-jobs/:id` | Update cron/pause/delete |

## Workers
| Method | Path | Notes |
|---|---|---|
| GET | `/workers` | Status, last heartbeat, in-flight count |
| GET | `/workers/:id` | Detail + recent heartbeat history |

## Dead Letter Queue
| Method | Path | Notes |
|---|---|---|
| GET | `/dlq` | Filter by queue/date |
| POST | `/dlq/:id/replay` | Re-queues the original job |
| DELETE | `/dlq/:id` | Discard permanently |

## System
| Method | Path | Notes |
|---|---|---|
| GET | `/system/health` | Liveness/readiness |
| GET | `/system/metrics` | Aggregate throughput/health for dashboard overview |

Full OpenAPI spec to be generated from the Go handler annotations once implementation starts, rather than hand-maintained separately.
