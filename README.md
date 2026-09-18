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

## API Documentation

See [`doc/openapi.yaml`](./doc/openapi.yaml) for the full API reference — endpoints, request/response schemas, and error codes. You can view it interactively by pasting it into [Swagger Editor](https://editor.swagger.io/).

## Testing

```bash
make test-unit          # fast unit tests, no dependencies
make test-integration   # integration tests against a real Postgres (requires Docker)
make test-all           # both

make coverage            # unit test coverage report (coverage.html)
make coverage-all        # unit + integration coverage report
```

## Seed Data

A sample dataset (storages, cards, decks) is available in [`_devops/database/seedTestData.sql`](./_devops/database/seedTestData.sql) for local experimentation:

```bash
docker exec -i tamiyo-db psql -U login -d tamiyo_db < _devops/database/seedTestData.sql
```

## Importing a ManaBox Collection

[`_devops/utils/import_manabox.py`](./_devops/utils/import_manabox.py) imports a collection exported from the [ManaBox](https://manabox.app/) app (CSV format) into a running Tamiyo instance via the API.

For each row, it gets or creates the matching Storage (and, if the binder type is `deck`, a Deck too), creates one Card per physical copy, and links deck cards accordingly.

```bash
pip install requests

# Dry run first — parses the CSV and prints what would happen, without calling the API
python3 _devops/utils/import_manabox.py _devops/utils/ManaBox_Collection.csv --dry-run

# Then run it for real (the API must be running)
python3 _devops/utils/import_manabox.py _devops/utils/ManaBox_Collection.csv
```

Targets `http://localhost:8080` by default; override with `--api-url` or the `TAMIYO_API_URL` environment variable.
