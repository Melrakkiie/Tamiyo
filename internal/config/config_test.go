package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PGHOST", "db")
	t.Setenv("PGUSER", "login")
	t.Setenv("PGPASSWORD", "password")
	t.Setenv("PGDATABASE", "tamiyo_db")
}

func TestLoad_ReadsValuesFromEnvironment(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PGPORT", "5432")
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
	setRequiredEnv(t)
	// APP_PORT is missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.AppPort)
}

func TestLoad_DefaultsPGPortWhenNotSet(t *testing.T) {
	setRequiredEnv(t)
	// PGPORT is missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "5432", cfg.PGPort)
}

func TestLoad_DefaultsCORSAllowedOriginsToEmptyWhenNotSet(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()

	require.NoError(t, err)
	assert.Empty(t, cfg.CORSAllowedOrigins)
}

func TestLoad_ParsesCORSAllowedOriginsFromCommaSeparatedList(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,https://tamiyo.example.com")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, []string{"http://localhost:5173", "https://tamiyo.example.com"}, cfg.CORSAllowedOrigins)
}

func TestLoad_TrimsWhitespaceAroundEachCORSOrigin(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", " http://localhost:5173 , https://tamiyo.example.com ")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, []string{"http://localhost:5173", "https://tamiyo.example.com"}, cfg.CORSAllowedOrigins)
}

func TestLoad_DropsEmptyEntriesInCORSAllowedOrigins(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,,https://tamiyo.example.com,")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, []string{"http://localhost:5173", "https://tamiyo.example.com"}, cfg.CORSAllowedOrigins)
}

func TestLoad_DefaultsAPIBasePathToEmpty(t *testing.T) {
	setRequiredEnv(t)
	// API_BASE_PATH is missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "", cfg.APIBasePath)
}

func TestLoad_NormalizesAPIBasePath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"already normalized", "/api", "/api"},
		{"missing leading slash", "api", "/api"},
		{"trailing slash", "/api/", "/api"},
		{"surrounding whitespace", " /api ", "/api"},
		{"nested path", "/v1/api/", "/v1/api"},
		{"only a slash", "/", ""},
		{"blank", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv("API_BASE_PATH", tt.raw)

			cfg, err := Load()

			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.APIBasePath)
		})
	}
}

func TestLoad_DefaultsRefreshCookieSecureToTrue(t *testing.T) {
	setRequiredEnv(t)
	// REFRESH_COOKIE_SECURE is missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.True(t, cfg.RefreshCookieSecure)
}

func TestLoad_ReadsRefreshCookieSecureFromEnvironment(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("REFRESH_COOKIE_SECURE", "false")

	cfg, err := Load()

	require.NoError(t, err)
	assert.False(t, cfg.RefreshCookieSecure)
}

func TestLoad_DefaultsPGSSLModeToDisable(t *testing.T) {
	setRequiredEnv(t)
	// PGSSLMODE is missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "disable", cfg.PGSSLMode)
}

func TestLoad_ReadsPGSSLModeFromEnvironment(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PGSSLMODE", "require")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "require", cfg.PGSSLMode)
}

func TestLoad_DefaultsDBConnectionPoolSettings(t *testing.T) {
	setRequiredEnv(t)
	// DB_MAX_OPEN_CONNS / DB_MAX_IDLE_CONNS / DB_CONN_MAX_LIFETIME_MINUTES are missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 10, cfg.DBMaxOpenConns)
	assert.Equal(t, 5, cfg.DBMaxIdleConns)
	assert.Equal(t, 5*time.Minute, cfg.DBConnMaxLifetime)
}

func TestLoad_ReadsDBConnectionPoolSettingsFromEnvironment(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DB_MAX_OPEN_CONNS", "20")
	t.Setenv("DB_MAX_IDLE_CONNS", "8")
	t.Setenv("DB_CONN_MAX_LIFETIME_MINUTES", "15")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 20, cfg.DBMaxOpenConns)
	assert.Equal(t, 8, cfg.DBMaxIdleConns)
	assert.Equal(t, 15*time.Minute, cfg.DBConnMaxLifetime)
}

func TestLoad_DefaultsShutdownTimeoutWhenNotSet(t *testing.T) {
	setRequiredEnv(t)
	// SHUTDOWN_TIMEOUT_SECONDS is missing

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
}

func TestLoad_ReadsShutdownTimeoutFromEnvironment(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "30")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
}

func TestLoad_ReturnsErrorWhenJWTSecretMissing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("JWT_SECRET", "")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "JWT_SECRET")
}

func TestLoad_ReturnsErrorWhenRequiredPGVarsMissing(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PGHOST", "")
	t.Setenv("PGUSER", "")
	t.Setenv("PGPASSWORD", "")
	t.Setenv("PGDATABASE", "")

	cfg, err := Load()

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "PGHOST")
	assert.Contains(t, err.Error(), "PGUSER")
	assert.Contains(t, err.Error(), "PGPASSWORD")
	assert.Contains(t, err.Error(), "PGDATABASE")
}

func TestLoad_FallsBackToPostgreSQLAddonVariablesWhenPGVarsMissing(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PGHOST", "")
	t.Setenv("PGPORT", "")
	t.Setenv("PGUSER", "")
	t.Setenv("PGPASSWORD", "")
	t.Setenv("PGDATABASE", "")
	t.Setenv("POSTGRESQL_ADDON_HOST", "addon-host.example.com")
	t.Setenv("POSTGRESQL_ADDON_PORT", "5433")
	t.Setenv("POSTGRESQL_ADDON_USER", "addon-user")
	t.Setenv("POSTGRESQL_ADDON_PASSWORD", "addon-password")
	t.Setenv("POSTGRESQL_ADDON_DB", "addon_db")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "addon-host.example.com", cfg.PGHost)
	assert.Equal(t, "5433", cfg.PGPort)
	assert.Equal(t, "addon-user", cfg.PGUser)
	assert.Equal(t, "addon-password", cfg.PGPassword)
	assert.Equal(t, "addon_db", cfg.PGDatabase)
}

func TestLoad_ExplicitPGVarsTakePrecedenceOverPostgreSQLAddonVariables(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PGPORT", "5432")
	t.Setenv("POSTGRESQL_ADDON_HOST", "addon-host.example.com")
	t.Setenv("POSTGRESQL_ADDON_PORT", "5433")
	t.Setenv("POSTGRESQL_ADDON_USER", "addon-user")
	t.Setenv("POSTGRESQL_ADDON_PASSWORD", "addon-password")
	t.Setenv("POSTGRESQL_ADDON_DB", "addon_db")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "db", cfg.PGHost)
	assert.Equal(t, "5432", cfg.PGPort)
	assert.Equal(t, "login", cfg.PGUser)
	assert.Equal(t, "password", cfg.PGPassword)
	assert.Equal(t, "tamiyo_db", cfg.PGDatabase)
}

func TestLoad_DefaultsPGPortWhenNeitherPGPortNorAddonPortIsSet(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PGPORT", "")
	t.Setenv("POSTGRESQL_ADDON_PORT", "")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "5432", cfg.PGPort)
}

func TestLoad_DefaultsShareRateLimit(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 60, cfg.ShareRateLimitMax)
	assert.Equal(t, 60*time.Second, cfg.ShareRateLimitWindow)
}

func TestLoad_ReadsShareRateLimitFromEnvironment(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SHARE_RATE_LIMIT_MAX", "20")
	t.Setenv("SHARE_RATE_LIMIT_WINDOW_SECONDS", "120")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, 20, cfg.ShareRateLimitMax)
	assert.Equal(t, 120*time.Second, cfg.ShareRateLimitWindow)
}
