# Patient & Evacuee Locator — Server

Centralized registry for finding rescued individuals after the La Guaira 7.7 earthquake.

Low-bandwidth, fuzzy-searchable API that aggregates handwritten hospital, shelter, CDI
(Diagnostic Center), and morgue lists digitized by trusted volunteers.

## Tech stack

- **Go** with `go-chi/chi/v5`
- **PostgreSQL** via `jackc/pgx/v5`
- **OpenTelemetry** (OTLP → local collector or New Relic)
- **Goose** SQL migrations

## Architecture

```
Handlers (HTTP) → Services (business logic) → Stores (SQL) → PostgreSQL
```

Wiring in `internal/app/`.

## Getting started

```bash
cp .env.example .env
# edit .env — at minimum fill in DB_*, JWT_*
make docker-run     # PostgreSQL + OTel collector
make migration-run  # goose up
make run            # or: make watch (live reload via air)
```

## Commands

| Target | Purpose |
|---|---|
| `make build` | Compile to `./main.exe` |
| `make run` | Run with `go run` |
| `make watch` | Live reload with `air` |
| `make test` | Run all unit tests |
| `make itest` | Integration tests |
| `make docker-run` | Start dev stack |
| `make docker-down` | Stop dev stack |
| `make migration name=X` | Create SQL migration |
| `make migration-run` | `goose up` |

## Project

Disaster response tool for Venezuela. Open source.
