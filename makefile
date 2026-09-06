.PHONY: test coverage

test:
	go test ./internal/...

coverage:
	go test ./internal/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Rapport généré : coverage.html"
