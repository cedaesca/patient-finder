# CLAUDE.md

## Project

Patient & Evacuee Locator — a Go backend for aggregating patient/shelter/morgue lists
after the La Guaira 7.7 earthquake in Venezuela. Trusted volunteers digitize handwritten
lists; the API provides fuzzy-searchable public lookup for families.

## Architecture

Three-layer with DI: Handlers → Services → Stores → PostgreSQL. Wiring in `internal/app/`.

## Modules

| Module        | Purpose                                              |
| ------------- | ---------------------------------------------------- |
| `auth`        | JWT access + refresh tokens, email-OTP login/signup  |
| `users`       | User accounts, rate-limited OTP flows                |
| `audit`       | Immutable write log                                  |
| `otp`         | One-time code generation (no email delivery)         |
| `permissions` | Permission codes                                     |
| `pagination`  | Shared pagination filters/metadata                   |
| `request`     | Chi middleware, auth context                          |
| `config`      | Env var constants                                    |
| `database`    | pgx pool, transactor                                 |
| `server`      | HTTP server, route registration, OTel                |
| `app`         | Wiring layer                                         |
| `logging`     | slog setup + handler (trace_id, span_id, request_id) |
| `utils`       | Helpers                                              |

## Commands

```bash
make build         # compile
make run           # go run
make watch         # live reload (air)
make test          # unit tests
make itest         # integration tests (RUN_DATABASE_TESTS=1)
make docker-run    # start PostgreSQL + OTel
make migration name=X   # create SQL migration
make migration-run      # goose up
```

Run a single test: `go test ./internal/auth/... -run TestXxx -v`

## Conventions

- Interfaces in the same file as the store/service that defines them
- Tests alongside source files
- testify for assertions
- Integration tests use testcontainers-go, gated by `RUN_DATABASE_TESTS=1`
