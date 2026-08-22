# Distributed Job Scheduler

Currently implemented: **auth + project management**. Queues/jobs/workers
land once the job execution model is decided — see `docs/` for the full
target architecture.

## Run it

Full stack, HTTPS included (Postgres + API + Caddy reverse proxy):

```bash
cp .env.example .env          # then edit JWT_SECRET (openssl rand -base64 32)
docker compose up -d --build
```

Caddy listens on `:443` and terminates HTTPS — self-signed for local dev
(`SITE_ADDRESS` defaults to `localhost`), a real Let's Encrypt cert
automatically if you point `SITE_ADDRESS` at a real domain. The API
container has no published port; only Caddy can reach it, exactly as it
would sit behind a load balancer in production.

Bare-metal alternative, for iterating on the API without a container rebuild
per change (plain HTTP, `localhost` only — fine for local dev, don't do this
across a real network):

```bash
docker compose up -d postgres
export $(cat .env | xargs)
go run ./cmd/api               # applies schema on startup, listens on :8080
```

## Try it

Against the full HTTPS stack (`-k` accepts Caddy's local self-signed cert —
drop it once you trust Caddy's local CA, or once `SITE_ADDRESS` is a real
domain with a real cert):

```bash
curl -sk https://localhost/auth/register -d '{"email":"a@example.com","password":"password123"}' | jq

TOKEN=$(curl -sk https://localhost/auth/login -d '{"email":"a@example.com","password":"password123"}' | jq -r .access_token)

curl -sk https://localhost/organizations -H "Authorization: Bearer $TOKEN" -d '{"name":"Acme"}' | jq
curl -sk https://localhost/organizations -H "Authorization: Bearer $TOKEN" | jq
```

Against the bare-metal API directly, swap `https://localhost` for
`http://localhost:8080` and drop `-k`.

## Build / test

```bash
go build ./...
go vet ./...
go test ./...
```

## Auth model

- Passwords hashed with bcrypt.
- Access tokens: short-lived (15m) JWT, `Authorization: Bearer <token>`.
- Refresh tokens: opaque random tokens, stored server-side as a SHA-256 hash
  (so a DB leak alone can't be replayed), rotated on every `/auth/refresh`
  call, revocable via `/auth/logout`.
- Every organization/project endpoint checks org membership (and role for
  owner-only actions) server-side — the JWT only proves *who*, not *what
  they can touch*.
- Transport is HTTPS, terminated by Caddy in front of the API — see
  `docs/auth-workflow.md#transport-https`.

## What jobs actually run

Not decided yet — flagged as the next thing to design once auth/projects are
solid. Likely candidate: jobs shell out to a Python script per `jobs.type`,
with `payload` as its argv/stdin. Revisit before building the Worker service.
