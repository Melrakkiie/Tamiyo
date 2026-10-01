# Tamiyo
[![codecov](https://codecov.io/github/Melrakkiie/Tamiyo/graph/badge.svg?token=JQKBS058ZW)](https://codecov.io/github/Melrakkiie/Tamiyo)

A REST API for managing a Magic: The Gathering card collection — cards, physical storage (binders, boxes, deckboxes), and decks.

## Tech Stack

- **Go** + [Gin](https://github.com/gin-gonic/gin) — HTTP framework
- **PostgreSQL** — persistence
- **sqlx** — database access
- **Docker Compose** — local development, with live-reload via [air](https://github.com/air-verse/air)
- **testcontainers-go** — integration tests against a real Postgres instance

## Project Structure

```
tamiyo/
├── cmd/api/              # application entrypoint
├── internal/
│   ├── card/             # card domain: model, service, repository, HTTP handler
│   ├── storage/          # storage domain (binders, boxes, deckboxes)
│   ├── deck/             # deck domain, including deck ↔ card relationship
│   └── config/           # environment configuration
├── _devops/database/     # SQL schema
├── .githooks/            # versioned git hooks (see Code Quality)
├── docker-compose.yml
└── Dockerfile
```

Each domain follows the same layered structure: `domain.go` (entities + repository interface), `service.go` (use cases), `postgres_repository.go` (persistence), `http_handler.go` (routes + HTTP concerns).

## Getting Started

### Prerequisites

- Docker and Docker Compose

### Run the app

```bash
docker compose up --build
```

The API is available at `http://localhost:8080`. Code changes are picked up automatically via `docker compose watch` / air live-reload.

### Reset the database

Since the schema init script only runs on a fresh volume, use this whenever the schema changes:

```bash
make db-reset
```

### Set up git hooks
 
A pre-commit hook (build + `go vet` + `golangci-lint` + unit tests) is versioned in [`.githooks/`](./.githooks). Enable it once per clone:
 
```bash
make install-hooks
```

It only runs when staged files include `.go` changes, and only checks unit tests (integration tests are left to CI/`make test-integration`, since they spin up a Postgres container). Requires [`golangci-lint`](https://golangci-lint.run/) installed locally for the lint step; if it's missing, that step is skipped with a warning rather than blocking the commit.


## Authentication

Tamiyo is multi-tenant: every account has its own cards, storages, and decks, completely isolated from other accounts.

1. **Register** an account:
   ```bash
   curl -X POST localhost:8080/auth/register \
     -H "Content-Type: application/json" \
     -d '{"email": "you@example.com", "password": "at-least-8-chars"}'
   ```
2. **Log in** (or reuse the token from registration) to get a JWT:
   ```bash
   curl -X POST localhost:8080/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email": "you@example.com", "password": "at-least-8-chars"}'
   ```
   Both return `{"token": "..."}`.
3. Send that token on every other request:
   ```bash
   curl localhost:8080/cards -H "Authorization: Bearer <token>"
   ```

`/health`, `/auth/register`, and `/auth/login` are the only public routes — everything else (`/cards`, `/storage`, `/deck`) requires a valid Bearer token and only ever returns or modifies that account's own data.

## API Documentation

See [`openapi.yaml`](./openapi.yaml) for the full API reference — endpoints, request/response schemas, and error codes. You can view it interactively by pasting it into [Swagger Editor](https://editor.swagger.io/).

## Testing

```bash
make test-unit          # fast unit tests, no dependencies
make test-integration   # integration tests against a real Postgres (requires Docker)
make test-all           # both

make coverage            # unit test coverage report (coverage.html)
make coverage-all        # unit + integration coverage report
```

## Seed Data

A sample dataset (one test account, plus storages, cards and decks scoped to it) is available in [`_devops/database/seedTestData.sql`](./_devops/database/seedTestData.sql) for local experimentation:

```bash
docker exec -i tamiyo-db psql -U login -d tamiyo_db < _devops/database/seedTestData.sql
```

Log in as the seeded account with `seed@tamiyo.local` / `password123` (see `POST /auth/login` above) to see the sample data. The script enables `pgcrypto` to hash that password the same way the API does (bcrypt).

## Importing a ManaBox Collection

[`_devops/utils/import_manabox.py`](./_devops/utils/import_manabox.py) imports a collection exported from the [ManaBox](https://manabox.app/) app (CSV format) into a running Tamiyo instance via the API.

For each row, it gets or creates the matching Storage (and, if the binder type is `deck`, a Deck too), creates one Card per physical copy, and links deck cards accordingly.

Since the API is multi-tenant, the script needs an account to import into: pass `--email` (and optionally `--password`, otherwise you're prompted for it). It registers that account on first use, or logs in if it already exists, then sends the resulting token on every request — everything it creates belongs to that one account.

```bash
pip install requests

# Dry run first — parses the CSV and prints what would happen, without calling the API
python3 _devops/utils/import_manabox.py _devops/utils/ManaBox_Collection.csv --dry-run --email you@example.com

# Then run it for real (the API must be running) — you'll be prompted for the password
python3 _devops/utils/import_manabox.py _devops/utils/ManaBox_Collection.csv --email you@example.com
```

Targets `http://localhost:8080` by default; override with `--api-url` or the `TAMIYO_API_URL` environment variable.
