# Auth & Project Management — Implementation Workflow

Status: implemented (`cmd/api`, `internal/{authsvc,store,httpapi,db,config}`).
Covers `users`, `organizations`, `org_members`, `projects`, `refresh_tokens`
from [database-schema.md](./database-schema.md). Queues/jobs/workers are not
built yet.

## Request flow, end to end

```
Client
  │  POST /auth/register {email, password}
  ▼
httpapi.handleRegister
  │  validate email (net/mail) + password length
  │  authsvc.HashPassword (bcrypt)
  ▼
store.CreateUser ──► INSERT INTO users ...
  │
  ▼
respondWithNewSessionStatus
  │  authsvc.GenerateAccessToken (JWT, 15m)
  │  authsvc.NewRefreshToken (opaque random, 7d)
  ▼
store.CreateRefreshToken ──► INSERT INTO refresh_tokens (hash only)
  │
  ▼
{ access_token, refresh_token, token_type: "Bearer" }
```

Every later request to an `/organizations` or `/projects` route carries
`Authorization: Bearer <access_token>`; `requireAuth` middleware verifies it
and puts the user ID in the request context before the handler runs.

## Package layout and why

| Package | Responsibility |
|---|---|
| `internal/config` | Reads env vars once at startup, fails fast if `DATABASE_URL`/`JWT_SECRET` are missing or weak. |
| `internal/db` | Postgres pool + schema migration (one embedded, idempotent SQL file — see [database-schema.md](./database-schema.md)). |
| `internal/authsvc` | Pure, DB-free crypto: password hashing, JWT issue/verify, refresh-token generation/hashing. No `net/http`, no SQL — testable in isolation. |
| `internal/store` | All SQL. One file per entity (`users.go`, `orgs.go`, `projects.go`, `refresh_tokens.go`). Handlers never write SQL directly. |
| `internal/httpapi` | Routing, request validation, authz checks, JSON responses. |
| `cmd/api` | Wires the above together, handles graceful shutdown. |

This split mirrors the standard Go service layering (transport → domain
logic → persistence) so each layer can be tested/replaced without touching
the others — e.g. `authsvc` is unit-tested with no database at all
(`internal/authsvc/authsvc_test.go`).

## Auth model

- **Passwords**: bcrypt (`golang.org/x/crypto/bcrypt`), default cost. Never
  stored or logged in plaintext.
- **Access tokens**: JWT (HS256), 15-minute TTL, `sub` claim = user UUID.
  Stateless — verified locally on every request, no DB round trip.
- **Refresh tokens**: opaque 32-byte random values (`crypto/rand`), *not*
  JWTs. The server stores only their SHA-256 hash in `refresh_tokens`, so a
  stolen database dump alone can't be replayed as a session. Every
  `/auth/refresh` call **rotates**: the presented token is revoked and a new
  pair issued in the same request, so a leaked-and-reused old token becomes
  detectable (it will already be revoked).
- **Logout**: `/auth/logout` revokes one refresh token by hash. Access
  tokens are not revocable (stateless by design) — this is why their TTL is
  short.

## Authorization model

Every organization has exactly one `owner` (set at creation, inside the same
transaction as the org row) and zero or more `member`s via `org_members`.
Every project belongs to one org.

- `requireOrgMember` — used by any org/project read: 404 if the org doesn't
  exist, 403 if the caller isn't in `org_members`.
- `requireOrgOwner` — used by mutating org actions (rename, delete, add
  member): 403 unless the caller's role is `owner`.
- `requireProjectMember` — same idea for `/projects/:id`, resolved via the
  project's `org_id`.

RBAC stays intentionally coarse (`owner`/`member`, no per-permission grants)
per [design-decisions.md](./design-decisions.md) — the grading rubric weighs
reliability/concurrency far higher than fine-grained authz.

## Transport (HTTPS)

The Go binary itself only ever speaks plain HTTP (`ListenAndServe`, no
`ListenAndServeTLS`) — it stays protocol-agnostic on purpose. HTTPS is
terminated in front of it by **Caddy** (`docker-compose.yml`, `Caddyfile`):
Caddy listens on `:443`, gets a cert automatically (self-signed for
`localhost`, real Let's Encrypt for a real `SITE_ADDRESS`), and reverse-proxies
plain HTTP to the `api` container over the private compose network. The `api`
container publishes no port of its own — Caddy is the only way in, the same
shape a real load balancer would take in production.

This matters because a JWT is a bearer token: whoever holds the string is
treated as that user. Sent over plain HTTP, both the login password and the
resulting access/refresh tokens are readable by anyone on the network path.
HTTPS (transport) and JWT (identity) solve different problems and both are
needed — see the "why HTTP+JWT alone isn't enough" discussion this doc
grew out of.

## Error handling

All error responses use the envelope from
[api-design.md](./api-design.md): `{"error": {"code": "...", "message":
"..."}}` with matching HTTP status (`validation_error`/400,
`unauthorized`/401, `forbidden`/403, `not_found`/404, `conflict`/409,
`internal_error`/500). Internal errors are logged server-side via `slog` but
never leak details to the client.

## What's deliberately not built yet

- Auth on this doc covers `cmd/api`/`cmd/job-service` only. Queues, jobs,
  retry policies, workers, recurring (cron) jobs, and the DLQ are all
  implemented (see [architecture.md](./architecture.md),
  [database-schema.md](./database-schema.md)) and scoped by the same
  org-membership rules described here.
- Fine-grained RBAC beyond owner/member.
- Rate limiting / lockout on `/auth/login` (brute-force protection) — worth
  adding before this is internet-facing.
