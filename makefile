.PHONY: test test-unit test-integration test-all coverage coverage-all build vet lint govulncheck check install-hooks db-reset migrate-up migrate-down migrate-status

GOOSE_DB_STRING := "host=localhost port=5432 user=login password=password dbname=tamiyo_db sslmode=disable"
GOOSE_DIR := _devops/database/migrations

install-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit
	@echo "Hooks git activés (core.hooksPath=.githooks)"

test: test-unit

test-unit:
	go test ./internal/...

test-integration:
	TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/... -tags=integration -run TestPostgresRepository -v

test-all: test-unit test-integration

coverage:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Rapport généré : coverage.html"

coverage-all:
	TESTCONTAINERS_RYUK_DISABLED=true go test ./internal/... -tags=integration -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Rapport généré (unitaire + intégration) : coverage.html"

build:
	go build ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

check: build vet lint govulncheck test-all
	@echo "Build, vet, lint, govulncheck et tests OK"

db-reset:
	docker compose down -v
	docker compose up --build

# Manual goose CLI usage against the Postgres started by `docker compose up`.
# The app itself applies pending migrations automatically on boot
# (cmd/api/main.go) — these targets are only for inspecting status or
# rolling back by hand during local development.
migrate-up:
	go run github.com/pressly/goose/v3/cmd/goose@v3.27.0 -dir=$(GOOSE_DIR) postgres $(GOOSE_DB_STRING) up

migrate-down:
	go run github.com/pressly/goose/v3/cmd/goose@v3.27.0 -dir=$(GOOSE_DIR) postgres $(GOOSE_DB_STRING) down

migrate-status:
	go run github.com/pressly/goose/v3/cmd/goose@v3.27.0 -dir=$(GOOSE_DIR) postgres $(GOOSE_DB_STRING) status
