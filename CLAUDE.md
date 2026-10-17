# Tamiyo API

Go REST API for a Magic: The Gathering collection: cards, physical storages, decks (boards, pending cards, tags, folders), sharing, likes, follows, display preferences. Module `Melrakkiie/Tamiyo`, Gin, sqlx + lib/pq, PostgreSQL (schema `tamiyo`), goose migrations, zap logs. The web front is a separate repo, `Tamiyo-front`, served on the same origin (`/api` prefix in prod on Clever Cloud).

## Commands

```bash
docker compose up --build        # API on :8080 + Postgres, live reload (air)
make lint                        # golangci-lint, integration-tagged files included (see .golangci.yml)
make test-unit                   # go test ./internal/...
go test ./internal/... -tags=integration   # integration tests, real Postgres via testcontainers (needs Docker)
make check                       # build, vet, lint, govulncheck, all tests
```

Run the relevant tests and the linter before calling a change done. Integration tests live in `*_test.go` files tagged `//go:build integration`.

## Layout

- `cmd/api/main.go`: wiring only (config, DB, migrations, services, routes, background printing refresher).
- `internal/<context>/`: one folder per bounded context. Layers: `domain.go` (entities, repository interface), `service.go` (business rules), `postgres_repository.go` (SQL), `http_handler.go` (routes, JSON, status codes). Large contexts split by sub-topic (`deck/folders.go`, `folders_http.go`, `folders_postgres.go`; same for tags, view, pending, likes, public, ownership).
- `bulk` and `deckshare` orchestrate other contexts and own no table.
- `_devops/database/migrations/`: goose SQL, embedded in the binary and applied on boot.
- `doc/API.md` and `doc/openapi.yaml`: the API reference. The front generates its client types from `openapi.yaml`.

## Rules

- **Migrations:** never edit a migration that has been delivered; add a new `NNNNN_description.sql` with `-- +goose Up` / `-- +goose Down` (wrap function bodies in `StatementBegin`/`StatementEnd`). A down migration must restore a state the previous code accepts. Extending a `CHECK` constraint means dropping and re-adding it in a new migration.
- **Interfaces:** each consumer redeclares the narrow interface it needs instead of importing another context's concrete type. After changing a shared method's signature, grep the method name across the repo: the compiler only catches the copies whose interface you also changed.
- **Errors:** services return sentinel errors (`var ErrX = errors.New(...)`); handlers map them with `apierr.Respond(ctx, err, apierr.Mapping{Err: ErrX, Status: http.StatusBadRequest})`.
- **Isolation:** every repository query filters by `user_id`; cross-account ownership is also enforced by triggers.
- **External calls:** Scryfall and SMTP are called from services, never from handlers or repositories. Scryfall goes through `internal/scryfall` (batches of 75, paced, retried on 429).
- **Preferences and sortable data live in the database**, never in the browser.
- **Docs:** any change to a route, a field or a validation rule updates `doc/API.md` and `doc/openapi.yaml` in the same change, and the README when it lists the feature.
- **Comments:** names and tests carry the meaning; a comment only states a constraint the code can't show. SQL migrations keep their header comment explaining the change.
- **Remove dead code** a change leaves behind.
- Code, README, docs and commit messages in English.

## Commits

Imperative subject under ~70 characters, then a short body saying what changed and why. The API is pushed and deployed before the front that depends on it. Pushing to `main` deploys to production once CI passes. Commit on a branch and open a PR; merging to `main` is mine.

## Agent skills

### Issue tracker

Issues are tracked in this repo's GitHub Issues via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
