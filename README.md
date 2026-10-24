# Backend Skeleton

A production-oriented Go API skeleton: layered structure, explicit SQL, JWT
auth, structured logging, metrics, health checks, graceful shutdown, and CI
gates wired from day one.

It is intentionally small. Every file exists to demonstrate a rule the team
should keep, not to guess at future features. The reference `item` module was
replaced by the `attendance` module (student attendance).

## Stack

| Concern | Choice |
|---|---|
| Language | Go 1.26 |
| HTTP | Gin |
| Config | Viper (YAML + `APP_*` env overrides) |
| Database | PostgreSQL + `pgx/v5` |
| Queries | `sqlc` (typed, generated) |
| Migrations | `golang-migrate` (versioned up/down) |
| Auth | JWT HS256, roles + school claims, middleware + RBAC |
| Logging | `log/slog` (JSON in production) |
| Metrics | Prometheus (`/metrics`) |
| Lint / CI | golangci-lint v2, GitHub Actions |

## Layout

```
cmd/
  api/                  # service entrypoint (signal handling only)
  token/                # local dev token minting
internal/
  app/                  # composition root, HTTP server, graceful shutdown
  auth/                 # JWT issue/parse, login and school registration
  config/               # typed config + validation
  db/                   # sqlc-generated code (do not edit)
  attendance/           # student attendance: types, repository, service
  platform/
    database/           # pgx pool, transactions, error mapping
    logger/             # slog construction
    metrics/            # Prometheus registry, DB pool collector
  transport/http/
    response/           # single response envelope
    middleware/         # request-id, auth, RBAC, CORS, rate limit, metrics, recovery
    router.go           # route table and middleware wiring
    attendance.go       # handler for student attendance and the class list
    auth.go             # login and registration handlers
db/
  migrations/           # versioned schema migrations
  query/                # sqlc queries
docs/
  architecture.md       # dependency rules and request lifecycle
configs/config.example.yaml
deployments/Dockerfile
compose.yaml
```

Dependency rule: `transport -> service -> repository -> database`. Dependencies
never point the other way. Handlers contain no business logic; services contain
no HTTP or SQL details.

## Quick start

```bash
cp configs/config.example.yaml configs/config.yaml   # or: cp .env.example .env
docker compose up -d postgres
make migrate-up
docker compose exec -T postgres psql -U app -d app < db/seed/demo.sql   # local demo data
make run
```

Sign in with the seeded guru (`db/seed/demo.sql` stores the password
`guru123`) and record attendance:

```bash
TOKEN="$(curl -s -X POST localhost:8080/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"guru1","password":"guru123"}' | jq -r .data.token)"
curl -s localhost:8080/v1/classes -H "Authorization: Bearer $TOKEN"
curl -s -X POST localhost:8080/v1/attendance-sessions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"class_id":1,"tanggal":"2026-10-24"}'
curl -s -X PUT localhost:8080/v1/attendance-sessions/1/attendances \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"attendances":[{"student_id":1,"status":"hadir"},{"student_id":2,"status":"sakit","keterangan":"demam"}]}'
curl -s localhost:8080/v1/attendance-sessions/1 -H "Authorization: Bearer $TOKEN"
```

Register a new school and its admin in one call. The response carries an access
token, so the client is signed in immediately:

```bash
curl -s -X POST localhost:8080/v1/auth/register -H 'Content-Type: application/json' \
  -d '{"school_name":"SMP Negeri 1 Sleman","npsn":"20501234","jenjang":"SMP",
       "admin_name":"Drs. Hendra","email":"admin@smpn1sleman.sch.id",
       "phone":"081234567890","password":"HadirTerus2026"}'
```

The new school starts with no academic year or classes, so `/v1/classes` returns
an empty list until they are provisioned. Registrations never leak into the demo
school: every query is scoped to the caller's `school_id`.

`make token SUB=1 ROLE=guru` mints the same token without a password; its
subject must be a `users.id` linked to an active `staff` row. Add `SCHOOL=2` to
mint a token for another tenant.

Full stack in containers (runs migrations first):

```bash
APP_AUTH_JWT_SECRET="$(openssl rand -hex 32)" docker compose up --build
```

## Configuration

Resolution order: `APP_*` environment variables override `configs/config.yaml`.
Set `CONFIG_FILE=/path/to/file.yaml` to load a file from elsewhere.

| Variable | Default | Notes |
|---|---|---|
| `APP_APP_ENV` | `development` | `development`, `staging`, `production` |
| `APP_HTTP_PORT` | `8080` | |
| `APP_HTTP_ALLOWED_ORIGINS` | empty | YAML list; empty means same-origin only |
| `APP_DB_HOST` / `APP_DB_PORT` | `localhost` / `5432` | |
| `APP_DB_USER` / `APP_DB_PASSWORD` / `APP_DB_NAME` | `app` / empty / `app` | Password is required in production |
| `APP_DB_MAX_CONNS` | `10` | Pool size per instance |
| `APP_LOG_LEVEL` / `APP_LOG_FORMAT` | `info` / `json` | `debug`, `info`, `warn`, `error` |
| `APP_AUTH_JWT_SECRET` | none | Required, at least 32 characters, no default on purpose |
| `APP_AUTH_ACCESS_TOKEN_TTL` | `15m` | |

The service refuses to start on invalid configuration. That is deliberate: a
misconfigured secret or port should fail at deploy time, not on the first
request.

## API

All responses use one envelope:

```json
{"data": {"id": "..."}, "meta": {"request_id": "..."}}
{"error": {"code": "not_found", "message": "route not found", "request_id": "..."}}
```

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/healthz` | none | Liveness |
| GET | `/readyz` | none | Readiness (pings the database) |
| GET | `/metrics` | none | Prometheus metrics |
| GET | `/` | none | Service metadata |
| GET | `/v1/version` | none | Build version |
| POST | `/v1/auth/login` | none | Exchange username/password for an access token (`guru1` / `guru123` in the demo seed) |
| POST | `/v1/auth/register` | none | Register a school plus its admin user and return an access token (`201`; `409` on duplicate NPSN or email) |
| GET | `/v1/classes` | Bearer | Classes of the active academic year, with `wali_kelas` |
| GET | `/v1/attendance-sessions/:id` | Bearer | Session detail with class roster and saved statuses |
| POST | `/v1/attendance-sessions` | Bearer + `guru`/`tu`/`admin` | Create (or fetch) a session; idempotent per class/date/subject |
| PUT | `/v1/attendance-sessions/:id/attendances` | Bearer + `guru`/`tu`/`admin` | Upsert student statuses (`hadir`, `sakit`, `izin`, `alpa`) |
| POST | `/v1/attendance-sessions/:id/lock` | Bearer + `guru`/`tu`/`admin` | Lock the session; later writes return `409` |
| GET | `/v1/admin/ping` | Bearer + `admin` role | RBAC example |

## Adding a module

1. Add `db/migrations/NNNNNN_name.up.sql` and `.down.sql`. Never edit an applied
   migration.
2. Write queries in `db/query/` and run `make sqlc`.
3. Add `internal/<module>/types.go`, `service.go`, `repository.go`. The service
   validates input and returns domain errors; the repository maps rows to domain
   types.
4. Add a handler in `internal/transport/http/`, translating HTTP errors only.
5. Wire the repository and service in `internal/app/app.go`, then register routes
   in `internal/transport/http/router.go`.
6. Add a service unit test with a fake repository and an integration test guarded
   by `TEST_DATABASE_URL`.

## Quality gates

```bash
make check             # gofmt check, go vet, tests, build
make lint              # golangci-lint (make tools installs it)
make test-integration  # needs Postgres + make migrate-up
```

CI runs format, vet, migrations, tests, build, `govulncheck`, and golangci-lint
on every push to `main` and on pull requests. Generated code from `sqlc` is
committed so builds do not need the generator.

## Design decisions and limits

- **No ORM.** SQL is explicit and typed via `sqlc`; parameters are always bound,
  never concatenated.
- **Access tokens only.** Refresh-token rotation and revocation need a store
  (Redis or a `refresh_tokens` table); add it when sessions must outlive the
  access TTL. `internal/auth` is the extension point.
- **Rate limit is per process.** It is a fixed-window limiter in memory. With more
  than one replica, move the counter to Redis. This is marked with a
  `shortcut:` comment in `ratelimit.go`.
- **Metrics are unauthenticated.** Scrape them over the private network or block
  `/metrics` at the reverse proxy.
- **TLS terminates at the proxy.** The app speaks plain HTTP inside the cluster.
- **Migrations are a separate step.** The app never mutates the schema on boot;
  `compose.yaml` runs a one-shot migrate service.
- **Student attendance scope.** Sessions, per-student statuses, roster reads, and
  locking are implemented. Monthly `student_attendance_recaps` snapshots are not
  generated yet; add a recap job when TU reporting is needed.
- **Attendance identity.** The JWT subject is the numeric `users.id`; it is
  resolved to `staff.user_id`. `tu`/`admin` may record for any class, `guru` only
  for classes they teach or are wali kelas of.
- **Tenant scoping.** Every access token carries `school_id`. Class, session, and
  roster queries filter on the school of the active academic year, so an ID from
  another tenant behaves like a missing one (`404`, or `403` for a session
  write). Registration writes the school, admin user, staff row, and `admin`
  role in one statement; a duplicate NPSN or email rolls the whole thing back.
- **Demo login.** `db/seed/demo.sql` stores `guru123` for `guru1`/`tu1`; the
  seeded school uses NPSN `00000000`. These are fixtures, not a bootstrap path.
