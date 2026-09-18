# Tamiyo

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
