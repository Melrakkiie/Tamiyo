// Package migrations embeds the SQL migration files in this directory into
// the compiled binary, via goose (github.com/pressly/goose/v3), so the
// schema history ships inside the binary itself rather than depending on a
// separate copy of these .sql files being present next to it at runtime —
// which matters in particular for the distroless production image built by
// the Dockerfile, which copies nothing but the compiled binary.
//
// Used from cmd/api/main.go (applies pending migrations on boot) and from
// each bounded context's postgres_repository_test.go (provisions the
// schema of the ephemeral testcontainers database for integration tests).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
