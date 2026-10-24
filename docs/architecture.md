# Architecture

## Dependency rule

```
cmd/api
  └── internal/app              (composition root)
        ├── internal/transport/http
        │     ├── middleware
        │     └── response
        ├── internal/attendance   (service + repository + domain types)
        ├── internal/auth       (token service)
        ├── internal/config
        └── internal/platform   (database, logger, metrics)
              └── internal/db   (sqlc-generated)
```

Arrows point inward and downward only:

- `transport` may import a service. It must not import `internal/db` or `pgx`.
- A service may import its repository interface and platform helpers. It must
  not import `gin`, `net/http`, or the generated `db` package.
- A repository may import `internal/db` and `pgx`. It returns domain types.
- `platform` knows nothing about a domain module.

`internal/db` is generated from `db/query` and `db/migrations`. It is the only
place that knows column names.

## Layers

| Layer | Responsibility | Must not |
|---|---|---|
| Handler | Decode request, validate shape, map errors to status codes | Contain business rules or SQL |
| Service | Business rules, validation, orchestration, domain errors | Know HTTP or SQL |
| Repository | Parameterized SQL, row-to-domain mapping | Decide HTTP status |
| Platform | Pool, transactions, logging, metrics | Know a domain |

## Request lifecycle

1. `RequestID` assigns or propagates `X-Request-ID`.
2. `Metrics` starts latency and in-flight tracking.
3. `AccessLog` records the completed request with its correlation ID.
4. `Recovery` turns a panic into a logged `500`.
5. `CORS` answers preflight and adds origin headers.
6. `/v1` applies `BodyLimit` and `RateLimit`.
7. `Authenticate` parses the JWT and stores claims in the Gin context.
8. `RequireRoles` checks the roles claim for the route.
9. The handler decodes, calls the service, and renders the response envelope.

Health and metrics endpoints sit outside `/v1`, so probes are never rate
limited.

## Error handling

- Domain errors are sentinels in the service package (`ErrNotFound`,
  `ErrForbidden`).
- Handlers translate them with `errors.Is`. Unexpected errors are logged with
  the request ID and returned as a generic `500`.
- PostgreSQL errors are classified in `platform/database` (`IsNotFound`,
  `IsUniqueViolation`, ...). Never leak driver errors to clients.

## Persistence

- Migrations are additive, versioned, and have a matching `down`.
- Queries live in `db/query` and are regenerated with `sqlc`; generated code is
  committed.
- Multi-statement writes use `database.WithTx`, which rolls back on error and
  survives a cancelled request context.
- Constraints live in the database (primary keys, foreign keys, checks), not only
  in Go.

## Observability

- JSON logs with `request_id`, method, path, status, duration, client IP.
- `http_requests_total` and `http_request_duration_seconds` use the registered
  route pattern, not the raw URL, so cardinality stays bounded.
- `db_pool_connections{state}` and `db_pool_max_connections` expose pool
  saturation. Watch `acquired / max` before raising `db.max_conns`.
- `/healthz` reports the process; `/readyz` reports database reachability.

## Testing

| Level | Where | Needs |
|---|---|---|
| Unit | `internal/*/**_test.go` with fakes | nothing |
| HTTP | `internal/transport/http/*_test.go` with `httptest` | nothing |
| Integration | `*_integration_test.go` | `TEST_DATABASE_URL` + migrations |
| E2E | separate suite against a running instance | full stack |

Unit and HTTP tests must not require a database. Integration tests skip when
`TEST_DATABASE_URL` is empty, so `go test ./...` stays fast on a laptop.

## Configuration

`config.Load()` is the only place that reads the environment. Everything else
receives a typed struct. New settings must be added to the struct, defaults,
normalization, validation, and `configs/config.example.yaml`.

## Deliberate omissions

The skeleton does not include an ORM, a message queue, Redis, OpenAPI
generation, or multi-tenancy. Add them when a real requirement appears, not in
anticipation. The `shortcut:` marker in `ratelimit.go` records the one known
limit that must be revisited when the service scales horizontally.
