package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ReadsValuesFromEnvironment(t *testing.T) {
	t.Setenv("PGHOST", "db")
	t.Setenv("PGPORT", "5432")
	t.Setenv("PGUSER", "login")
	t.Setenv("PGPASSWORD", "password")
	t.Setenv("PGDATABASE", "tamiyo_db")
	t.Setenv("APP_PORT", "9090")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "db", cfg.PGHost)
	assert.Equal(t, "5432", cfg.PGPort)
	assert.Equal(t, "login", cfg.PGUser)
	assert.Equal(t, "password", cfg.PGPassword)
	assert.Equal(t, "tamiyo_db", cfg.PGDatabase)
	assert.Equal(t, "9090", cfg.AppPort)
}

func TestLoad_DefaultsAppPortWhenNotSet(t *testing.T) {
	t.Setenv("PGHOST", "db")
	t.Setenv("PGPORT", "5432")
	t.Setenv("PGUSER", "login")
	t.Setenv("PGPASSWORD", "password")
	t.Setenv("PGDATABASE", "tamiyo_db")
	// APP_PORT is missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.AppPort)
}
