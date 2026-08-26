# Auth & Project Management - Implementation Workflow

Status: implemented (`cmd/api`, `internal/{authsvc,store,httpapi,db,config,
mailer}`). Covers `users`, `organizations`, `org_members`, `projects`,
`refresh_tokens`, `otps` from [database-schema.md](./database-schema.md).

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
| `internal/db` | Postgres pool + schema migration (one embedded, idempotent SQL file - see [database-schema.md](./database-schema.md)). |
| `internal/authsvc` | Pure, DB-free crypto: password hashing, JWT issue/verify, refresh-token generation/hashing. No `net/http`, no SQL - testable in isolation. |
| `internal/store` | All SQL. One file per entity (`users.go`, `orgs.go`, `projects.go`, `refresh_tokens.go`). Handlers never write SQL directly. |
| `internal/httpapi` | Routing, request validation, authz checks, JSON responses. |
| `cmd/api` | Wires the above together, handles graceful shutdown. |

This split mirrors the standard Go service layering (transport → domain
logic → persistence) so each layer can be tested/replaced without touching
the others - e.g. `authsvc` is unit-tested with no database at all
(`internal/authsvc/authsvc_test.go`).

## Auth model

- **Passwords**: bcrypt (`golang.org/x/crypto/bcrypt`), default cost. Never
  stored or logged in plaintext.
- **Access tokens**: JWT (HS256), 15-minute TTL, `sub` claim = user UUID.
  Stateless - verified locally on every request, no DB round trip.
- **Refresh tokens**: opaque 32-byte random values (`crypto/rand`), *not*
  JWTs. The server stores only their SHA-256 hash in `refresh_tokens`, so a
  stolen database dump alone can't be replayed as a session. Every
  `/auth/refresh` call **rotates**: the presented token is revoked and a new
  pair issued in the same request, so a leaked-and-reused old token becomes
  detectable (it will already be revoked).
- **Logout**: `/auth/logout` revokes one refresh token by hash. Access
  tokens are not revocable (stateless by design) - this is why their TTL is
  short.

## Email verification & password reset (OTP)

`internal/mailer` wraps stdlib `net/smtp` - no third-party SDK, no
per-provider code; `net/smtp.SendMail` negotiates STARTTLS on its own
whenever the server offers it, so any provider's plain SMTP endpoint works
here unmodified. `SMTP_HOST`/`SMTP_FROM`/`SMTP_USER`/`SMTP_PASSWORD` are
required at startup (`internal/config`), same fail-fast treatment as
`DATABASE_URL`/`JWT_SECRET` - there's no local no-op fallback, every
environment sends through a real relay. `.env.example` documents Gmail
setup (an App Password, since Gmail requires 2-Step Verification for SMTP
auth); any other relay (SES/Brevo/Resend/etc, all free or a few cents per
1000 emails at this project's scale) works the same way, just different
host/port/credentials.

Both flows email a 6-digit one-time code (OTP) rather than a link: the
dashboard now has a real login screen (`AuthScreen.jsx`), so a code the
user types back in beats a link with nowhere obvious to land. Codes live
in one `otps` table (`purpose` = `verify_email` or `reset_password`),
hashed with SHA-256 - the same "never store the secret itself" reasoning
as `refresh_tokens`, though a 6-digit code's real protection is its short
lifetime and attempt cap, not the hash. `store.CreateOTP` invalidates any
previous unused code for that (user, purpose) before inserting the new
one, so requesting a fresh code (e.g. "resend") can't leave two valid
codes around. A code expires after 10 minutes or after 5 wrong guesses
(`store.VerifyOTP`, compared via `crypto/subtle.ConstantTimeCompare`),
whichever comes first - both dead ends require requesting a new code, not
a longer wait.

`POST /auth/register` sends a verify-email code but does **not** block the
session it returns - registering still logs you in immediately; the
frontend then drops that session and walks the user through OTP
verification before returning them to the login form (see "AuthScreen"
below). `users.email_verified` is tracked but login itself isn't gated on
it, since nothing yet depends on a verified address beyond the flag.

`POST /auth/forgot-password` and `POST /auth/resend-verification` always
return 200 regardless of whether the address exists (or is already
verified), so neither endpoint can be used to enumerate registered
emails. A successful password reset revokes every refresh token for that
user - a leaked-and-reset password shouldn't leave old sessions valid.

**AuthScreen** (`web/src/components/AuthScreen.jsx`) is a single component
cycling through five modes - `login`, `register`, `verify`, `forgot`,
`reset` - sharing one card and the same animated background. Registering
moves straight to `verify` (code entry) rather than signing the user in;
successfully verifying or resetting always lands back on `login`, never
auto-signed-in, matching how registration itself behaves.

## Account page

`GET /auth/me` / `PATCH /auth/me` / `POST /auth/change-password` back the
in-app Account page (`web/src/components/Account.jsx`) - separate from the
OTP flows above, which exist for someone who can't log in at all.
`change-password` requires the current password (`authsvc.CheckPassword`)
rather than an emailed code, and revokes every refresh token including the
caller's own on success, so the frontend treats a 200 there as "log out
now, sign in again" - same session-invalidation behavior as an OTP reset,
just reached from inside the app instead of from a locked-out state.
`display_name` (nullable, 60 chars max) is the one piece of account
customization: unset, the UI falls back to showing the email everywhere it
would otherwise show a name (e.g. the sidebar).

## Authorization model

Every organization has exactly one `owner` (set at creation, inside the same
transaction as the org row) and zero or more `member`s via `org_members`.
Every project belongs to one org.

- `requireOrgMember` - used by any org/project read: 404 if the org doesn't
  exist, 403 if the caller isn't in `org_members`.
- `requireOrgOwner` - used by mutating org actions (rename, delete, add
  member): 403 unless the caller's role is `owner`.
- `requireProjectMember` - same idea for `/projects/:id`, resolved via the
  project's `org_id`.

RBAC stays intentionally coarse (`owner`/`member`, no per-permission grants)
per [design-decisions.md](./design-decisions.md) - the grading rubric weighs
reliability/concurrency far higher than fine-grained authz.

## Transport (HTTPS)

The Go binary itself only ever speaks plain HTTP (`ListenAndServe`, no
`ListenAndServeTLS`) - it stays protocol-agnostic on purpose. HTTPS is
terminated in front of it by **Caddy** (`docker-compose.yml`, `Caddyfile`):
Caddy listens on `:443`, gets a cert automatically (self-signed for
`localhost`, real Let's Encrypt for a real `SITE_ADDRESS`), and reverse-proxies
plain HTTP to the `api` container over the private compose network. The `api`
container publishes no port of its own - Caddy is the only way in, the same
shape a real load balancer would take in production.

This matters because a JWT is a bearer token: whoever holds the string is
treated as that user. Sent over plain HTTP, both the login password and the
resulting access/refresh tokens are readable by anyone on the network path.
HTTPS (transport) and JWT (identity) solve different problems and both are
needed - see the "why HTTP+JWT alone isn't enough" discussion this doc
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
- Rate limiting / lockout on `/auth/login`, `/auth/forgot-password`, or
  `/auth/resend-verification` (brute-force / mail-bombing protection) -
  the per-code 5-attempt cap limits guessing a single OTP, but nothing yet
  limits how many codes a given email/IP can request - worth adding before
  this is internet-facing.
- Login is not gated on `email_verified` - the flag is tracked but not yet
  enforced.
