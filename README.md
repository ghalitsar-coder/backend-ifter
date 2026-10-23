# Backend Skeleton

A production-oriented Go API skeleton: layered structure, explicit SQL, JWT
auth, structured logging, metrics, health checks, graceful shutdown, and CI
gates wired from day one.

It is intentionally small. Every file exists to demonstrate a rule the team
should keep, not to guess at future features. Replace the `item` module with the
first real domain module and grow from there.

## Stack

| Concern | Choice |
|---|---|
| Language | Go 1.26 |
| HTTP | Gin |
| Config | Viper (YAML + `APP_*` env overrides) |
| Database | PostgreSQL + `pgx/v5` |
| Queries | `sqlc` (typed, generated) |
| Migrations | `golang-migrate` (versioned up/down) |
| Auth | JWT HS256, roles claim, middleware + RBAC |
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
  auth/                 # JWT issue/parse
  config/               # typed config + validation
  db/                   # sqlc-generated code (do not edit)
  item/                 # reference module: types, repository, service
  platform/
    database/           # pgx pool, transactions, error mapping
    logger/             # slog construction
    metrics/            # Prometheus registry, DB pool collector
  transport/http/
    response/           # single response envelope
    middleware/         # request-id, auth, RBAC, CORS, rate limit, metrics, recovery
    router.go           # route table and middleware wiring
    items.go            # handler for the reference module
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
make run
```

Mint a development token and call the reference module:

```bash
TOKEN="$(make -s token SUB=demo ROLE=admin)"
curl -s localhost:8080/v1/items -H "Authorization: Bearer $TOKEN"
curl -s -X POST localhost:8080/v1/items \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"first item"}'
```

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
| GET | `/v1/items` | Bearer | Paginated list (`page`, `per_page`) |
| POST | `/v1/items` | Bearer | Create |
| GET | `/v1/items/:id` | Bearer | Fetch one |
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

Delete `internal/item`, `items.go`, and their tests once a real module exists.

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
