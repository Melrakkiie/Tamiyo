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
│   ├── bulk/             # bulk import/export routes (ManaBox, Moxfield)
│   └── config/           # environment configuration
├── _devops/database/     # SQL schema
├── .githooks/            # versioned git hooks (see Code Quality)
├── docker-compose.yml
└── Dockerfile
```

Each domain follows the same layered structure: `domain.go` (entities + repository interface), `service.go` (use cases), `postgres_repository.go` (persistence), `http_handler.go` (routes + HTTP concerns).

Copy [`.env.example`](./.env.example) to `.env` and fill in `JWT_SECRET` at minimum before running the app.

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

## CORS and security headers

Every response carries a baseline set of HTTP security headers (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, a strict `Content-Security-Policy`) — see [`internal/security`](./internal/security).

Cross-origin browser access is denied by default: no frontend origin can read Tamiyo's responses until you list it in `CORS_ALLOWED_ORIGINS` (comma-separated, see [`.env.example`](./.env.example)). This only matters for a web frontend running in a browser — curl, a mobile app, or a server-to-server call are never affected by CORS either way. See [`internal/cors`](./internal/cors) for the exact rules.

```bash
CORS_ALLOWED_ORIGINS=http://localhost:5173
```

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

## Bulk Import

Three authenticated routes import a collection or decklist export produced by a third-party tool in one request, instead of one `POST /cards` call per card — see [`openapi.yaml`](./openapi.yaml) and [`doc/API.md`](./doc/API.md#bulk-import) for the full request/response shapes:

- `POST /import/manabox` — a [ManaBox](https://manabox.app/) collection CSV export. Carries its own storage/binder and Scryfall ID, so nothing else is needed.
- `POST /import/moxfield/collection` — a [Moxfield](https://www.moxfield.com/) "Export Collection" CSV. Has no storage concept, so every imported card is assigned to an existing `storage_id` you pass in; has no Scryfall ID either, so each row is resolved by set + collector number against the [Scryfall API](https://scryfall.com/docs/api/cards/collection).
- `POST /import/moxfield/deck` — a Moxfield deck's plain-text export (deck page → **More → Export → Plain Text**). The format has no section headers, so by convention the first line is treated as the commander unless `commander_from_first_line=false` is passed.

All three are `multipart/form-data` requests with the file in a field named `file`, e.g.:

```bash
curl -X POST localhost:8080/import/moxfield/collection \
  -H "Authorization: Bearer <token>" \
  -F "file=@moxfield_collection.csv" \
  -F "storage_id=1"
```

A bulk import never fails outright over a single bad row — it returns `200 OK` with a summary (`cards_created`, `cards_skipped`, `storages_created`, `decks_created`, and a `warnings` list for anything skipped).

## Bulk Export

Routes export back out in the same formats the import routes above read — see [`doc/API.md`](./doc/API.md#bulk-export) for details:

- `GET /export/manabox` — the whole collection as a ManaBox-compatible CSV, grouped by storage (a card with no storage lands in a synthetic "Unsorted" binder).
- `GET /export/moxfield/collection` — the whole collection as a Moxfield-compatible "Export Collection" CSV; Moxfield's format has no storage concept, so this groups the entire collection together regardless of storage.
- `GET /export/moxfield/deck/:id` — **one deck** (not the whole collection) as a Moxfield deck plain-text export. If the deck has a commander, its line is written first with its full quantity, so re-importing reconstructs the same commander.

The two collection-wide routes always export everything (no filtering by storage or deck) and return a raw CSV download, not JSON; the deck route returns a raw `.txt` download:

```bash
curl -X GET localhost:8080/export/manabox \
  -H "Authorization: Bearer <token>" \
  -o ManaBox_Collection_export.csv

curl -X GET localhost:8080/export/moxfield/deck/1 \
  -H "Authorization: Bearer <token>" \
  -o Moxfield_Deck_export.txt
```

### Importing a ManaBox Collection via script (alternative)

[`_devops/utils/import_manabox.py`](./_devops/utils/import_manabox.py) does the same ManaBox import as `POST /import/manabox` above, but as an external script driving the HTTP API instead of a native route — useful if you'd rather not upload the CSV directly, or want to see a dry run of what would be created before calling the API at all.

Since the API is multi-tenant, the script needs an account to import into: pass `--email` (and optionally `--password`, otherwise you're prompted for it). It registers that account on first use, or logs in if it already exists, then sends the resulting token on every request — everything it creates belongs to that one account.

```bash
pip install requests

# Dry run first — parses the CSV and prints what would happen, without calling the API
python3 _devops/utils/import_manabox.py _devops/utils/ManaBox_Collection.csv --dry-run --email you@example.com

# Then run it for real (the API must be running) — you'll be prompted for the password
python3 _devops/utils/import_manabox.py _devops/utils/ManaBox_Collection.csv --email you@example.com
```

Targets `http://localhost:8080` by default; override with `--api-url` or the `TAMIYO_API_URL` environment variable.
