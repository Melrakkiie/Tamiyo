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
│   ├── deckinsights/     # deck legality + stats routes
│   ├── deckshare/        # public, read-only shared deck routes
│   ├── scryfall/         # shared Scryfall API client (used by bulk and deckinsights)
│   ├── printing/         # background refresh of printings' type lines and legalities
│   └── config/           # environment configuration
├── _devops/database/     # goose SQL migrations (embedded, applied on API boot)
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

### Database migrations

The schema lives in versioned [goose](https://github.com/pressly/goose) migrations under [`_devops/database/migrations/`](./_devops/database/migrations), embedded into the compiled binary via `go:embed`. The API applies any pending migrations automatically on boot (see `cmd/api/main.go`) — there is nothing to run manually after `docker compose up`.

To add a schema change, add a new `NNNNN_description.sql` file in that directory with `-- +goose Up` / `-- +goose Down` sections (wrap multi-statement function bodies in `-- +goose StatementBegin` / `-- +goose StatementEnd`), then restart the app.

Manual goose CLI usage (status, rollback, etc.), against the database started by `docker compose up`:

```bash
make migrate-status
make migrate-up
make migrate-down
```

### Reset the database

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

## Card search

`GET /cards` filters by name, storage, storage type, colors (exactly / at least / at most), mana value (`=`, `<`, `<=`, `>`, `>=`), card type, subtype, format legality, number of colors, commander color identity and foil. Type lines and legalities come from Scryfall: the API keeps them in `tamiyo.printings`, fetching new printings within a few minutes and refreshing every printing once a day in a background job, so a ban shows up the next day. See [`doc/API.md`](./doc/API.md#get-cards).

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

Four authenticated routes import a collection export, a card list or a decklist produced by a third-party tool in one request, instead of one `POST /cards` call per card — see [`openapi.yaml`](./openapi.yaml) and [`doc/API.md`](./doc/API.md#bulk-import) for the full request/response shapes:

- `POST /import/manabox` — a [ManaBox](https://manabox.app/) collection CSV export. Carries its own storage/binder and Scryfall ID, so nothing else is needed; pass an optional `storage_id` to put every card in that storage instead.
- `POST /import/moxfield/collection` — a [Moxfield](https://www.moxfield.com/) "Export Collection" CSV. Has no storage concept, so every imported card is assigned to an existing `storage_id` you pass in; has no Scryfall ID either, so each row is resolved by set + collector number against the [Scryfall API](https://scryfall.com/docs/api/cards/collection).
- `POST /import/list` — a plain card list (`4 Lightning Bolt`, or `1 Sol Ring (SLD) 1011 *F*` for an exact printing), one card created per copy, in the optional `storage_id`.
- `POST /import/tamiyo` — a Tamiyo file (see below) of a collection or a storage: cards keep their printing, finish, proxy status and storage (storages are matched by name or created), or all go in the optional `storage_id`.
- `POST /deck/:id/import` — a decklist added to an existing deck: a Moxfield plain-text export (deck page → **More → Export → Plain Text**), a plain `4 Lightning Bolt` list matched by name, or any format `GET /deck/:id/export` produces, Tamiyo's included. It never creates cards: owned copies of each printing go in the deck, and missing ones are added as pending cards. `Sideboard` and `Maybeboard` / `Considering` headers send the lines below them to that board. With `commander_from_first_line=true`, the first line of the deck itself becomes the commander if the deck has none.

All four are `multipart/form-data` requests with the file in a field named `file`, e.g.:

```bash
curl -X POST localhost:8080/import/moxfield/collection \
  -H "Authorization: Bearer <token>" \
  -F "file=@moxfield_collection.csv" \
  -F "storage_id=1"
```

A bulk import never fails outright over a single bad row — it returns `200 OK` with a summary (`cards_created`, `cards_linked` and `cards_pending` for a deck, `cards_skipped`, `storages_created`, `decks_created`, and a `warnings` list for anything skipped).

Two more routes work from an existing deck, yours or someone else's public or unlisted one: `POST /deck/:id/duplicate` copies it into a new private deck of yours (cards added as `POST /deck/:id/import` would), and `POST /deck/:id/collect` adds its cards to your collection — all of them, only its pending cards on your own deck, or only those you lack on someone else's. See [`doc/API.md`](./doc/API.md#post-deckidduplicate).

## Bulk Export

Routes export back out in the same formats the import routes above read — see [`doc/API.md`](./doc/API.md#bulk-export) for details:

- `GET /export/manabox` — the collection as a ManaBox-compatible CSV, grouped by storage (a card with no storage lands in a synthetic "Unsorted" binder).
- `GET /export/moxfield/collection` — the collection as a Moxfield-compatible "Export Collection" CSV; Moxfield's format has no storage concept, so this groups the entire collection together regardless of storage.
- `GET /export/tamiyo` — the collection (or one storage) as a Tamiyo file.
- `GET /deck/:id/export?format=moxfield|plain|arena|tamiyo|cardmarket` — **one deck** (not the whole collection) as text: a Moxfield deck export, a plain `4 Lightning Bolt` list, or MTG Arena's `Commander` / `Deck` sections. The commander comes first, so re-importing reconstructs it; pending cards are included, and the sideboard and considered cards follow in their own sections (Arena leaves the considered cards out). `tamiyo` is a Tamiyo file, with the deck's tags when `tags=true`. `cardmarket` is a list for a Cardmarket wants list: the main deck and sideboard, or the boards listed in `boards`, or only their pending cards with `pending=true`, by name, or with each card's expansion name with `printings=true`.

The Tamiyo format is Tamiyo's own JSON file, versioned, which keeps what the other formats lose: storages and their type, proxies, exact printings, a deck's boards, commander and tags. See [`doc/API.md`](./doc/API.md#tamiyo-format).

The collection routes export everything, or a single storage with `?storage_id=4`, and return a raw download (CSV, or JSON for the Tamiyo format); the deck route returns a raw `.txt` download, or `.json` for `tamiyo`:

```bash
curl -X GET localhost:8080/export/manabox \
  -H "Authorization: Bearer <token>" \
  -o ManaBox_Collection_export.csv

curl -X GET "localhost:8080/deck/<deck-id>/export?format=moxfield" \
  -H "Authorization: Bearer <token>" \
  -o Deck_moxfield.txt
```

## Deck Insights

Two read-only, player-facing routes analyze a deck — nothing financial, nothing persisted, computed fresh from [Scryfall](https://scryfall.com/) data on every call (see [`internal/scryfall`](./internal/scryfall) for the shared client, also used by Bulk Import):

- `GET /deck/:id/legality` — checks every card against the deck's own format; for `commander` specifically, also checks singleton (one copy per card name, basic lands excepted) and color identity against the deck's commander.
- `GET /deck/:id/stats` — mana curve, color breakdown and card types (lands excluded from the curve/colors/average, since they skew every one of those without adding anything).

See [`doc/API.md`](./doc/API.md#deck-insights) for the exact response shapes and known limitations (e.g. no partner commanders, no named singleton exceptions like Relentless Rats).

```bash
curl localhost:8080/deck/<deck-id>/legality -H "Authorization: Bearer <token>"
curl localhost:8080/deck/<deck-id>/stats -H "Authorization: Bearer <token>"
```

### Sideboard and considering

Each card of a deck, owned or pending, sits on a board: `main` (the deck itself), `sideboard`, or `considering` (cards being weighed for the deck). `PUT /deck/:id/cards/:card_id` and `PATCH /deck/:id/pending/:pending_id` take a `board` to move a card. Only `main` counts in the deck's card count, its statistics, deck size and comparisons; legality checks every board. Exports write the sideboard and considered cards in their own sections, and imports read them back. See [`doc/API.md`](./doc/API.md#boards).

### Card tags

A deck's owner can tag its cards (`Ramp`, `Pioche`, `Removal`…) to organize deckbuilding. A tag belongs to a card name within a deck: every copy shares it, pending ones included, whatever the printing. A card can have several tags, and each deck has its own. `GET /deck/:id/tags` lists them, `PUT /deck/:id/tags/cards` sets a card's tags, `PATCH` / `DELETE /deck/:id/tags` rename or remove a tag on every card. Shared decks and deck comparisons include each card's tags. See [`doc/API.md`](./doc/API.md#card-tags-deckidtags).

### Shared decks

Deck ids are random UUIDs, so they can't be guessed from one another. `GET /shared/decks` browses every public deck, with filters on the name, format, commander, a card, the owner and the color identity. `GET /shared/decks/:id` (plus `/legality`, `/stats`, `/export` and `/compare/:other_id`) serves a public or unlisted deck read-only, without authentication: its name, format, owner profile and card list, with no storage, proxy or ownership details. A private deck answers `404`, like an unknown link. These routes are rate-limited per client IP (60 requests per minute by default, `SHARE_RATE_LIMIT_MAX` / `SHARE_RATE_LIMIT_WINDOW_SECONDS`). Signed in, `GET /deck/:id/ownership` tells how many copies of each card of a deck (yours, or anyone's public or unlisted one) your collection holds, by name whatever the printing, and `GET /deck/:id/compare/:other_id` compares two decks (yours, or anyone's public or unlisted ones) by card name, ignoring printings: cards in common with each deck's quantity, and cards only in one of them. See [`doc/API.md`](./doc/API.md#shared-decks).

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
