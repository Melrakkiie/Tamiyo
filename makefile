.PHONY: test test-unit test-integration test-all coverage coverage-all build vet lint check install-hooks db-reset

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

check: build vet lint test-all
	@echo "Build, vet, lint et tests OK"

db-reset:
	docker compose down -v
	docker compose up --build
