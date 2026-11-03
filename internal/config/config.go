package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// envReader reads environment variables and collects every unparseable or
// out-of-range value so Load can report them all at once.
type envReader struct{ bad []string }

// str returns the variable, or def when it is unset or empty.
func (r *envReader) str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// integer parses the variable strictly; unset or empty gives def. Values below min are refused.
func (r *envReader) integer(key string, def, min int) int {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		r.bad = append(r.bad, fmt.Sprintf("%s=%q (not an integer)", key, v))
		return def
	}
	if n < min {
		r.bad = append(r.bad, fmt.Sprintf("%s=%q (must be at least %d)", key, v, min))
		return def
	}
	return n
}

func (r *envReader) boolean(key string, def bool) bool {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.bad = append(r.bad, fmt.Sprintf("%s=%q (not a boolean)", key, v))
		return def
	}
	return b
}

func (r *envReader) duration(key string, def int, unit time.Duration) time.Duration {
	return time.Duration(r.integer(key, def, 1)) * unit
}

type Config struct {
	PGHost     string
	PGPort     string
	PGUser     string
	PGPassword string
	PGDatabase string
	PGSSLMode  string
	AppPort    string
	JWTSecret  string

	APIBasePath string

	RefreshCookieSecure bool

	AuthRateLimitMax    int
	AuthRateLimitWindow time.Duration

	ShareRateLimitMax    int
	ShareRateLimitWindow time.Duration

	JWTAccessTokenTTL  time.Duration
	JWTRefreshTokenTTL time.Duration

	PasswordResetTokenTTL     time.Duration
	EmailChangeTokenTTL       time.Duration
	EmailVerificationTokenTTL time.Duration

	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	PasswordResetURLTemplate     string
	EmailChangeURLTemplate       string
	EmailVerificationURLTemplate string

	CORSAllowedOrigins []string

	// TrustedProxies are the CIDR ranges or bare IPs whose X-Forwarded-For is believed.
	TrustedProxies []string

	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	ShutdownTimeout time.Duration
}

func Load() (*Config, error) {
	r := &envReader{}

	cfg := &Config{
		PGHost:              firstNonEmpty(r.str("PGHOST", ""), r.str("POSTGRESQL_ADDON_HOST", "")),
		PGPort:              firstNonEmpty(r.str("PGPORT", ""), r.str("POSTGRESQL_ADDON_PORT", ""), "5432"),
		PGUser:              firstNonEmpty(r.str("PGUSER", ""), r.str("POSTGRESQL_ADDON_USER", "")),
		PGPassword:          firstNonEmpty(r.str("PGPASSWORD", ""), r.str("POSTGRESQL_ADDON_PASSWORD", "")),
		PGDatabase:          firstNonEmpty(r.str("PGDATABASE", ""), r.str("POSTGRESQL_ADDON_DB", "")),
		PGSSLMode:           r.str("PGSSLMODE", "disable"),
		AppPort:             r.str("APP_PORT", "8080"),
		JWTSecret:           r.str("JWT_SECRET", ""),
		APIBasePath:         normalizeBasePath(r.str("API_BASE_PATH", "")),
		RefreshCookieSecure: r.boolean("REFRESH_COOKIE_SECURE", true),
		AuthRateLimitMax:    r.integer("AUTH_RATE_LIMIT_MAX", 5, 1),
		AuthRateLimitWindow: r.duration("AUTH_RATE_LIMIT_WINDOW_SECONDS", 60, time.Second),
		JWTAccessTokenTTL:   r.duration("JWT_ACCESS_TOKEN_TTL_MINUTES", 15, time.Minute),
		JWTRefreshTokenTTL:  r.duration("JWT_REFRESH_TOKEN_TTL_DAYS", 30, 24*time.Hour),

		ShareRateLimitMax:    r.integer("SHARE_RATE_LIMIT_MAX", 60, 1),
		ShareRateLimitWindow: r.duration("SHARE_RATE_LIMIT_WINDOW_SECONDS", 60, time.Second),

		PasswordResetTokenTTL:     r.duration("PASSWORD_RESET_TOKEN_TTL_MINUTES", 30, time.Minute),
		EmailChangeTokenTTL:       r.duration("EMAIL_CHANGE_TOKEN_TTL_MINUTES", 1440, time.Minute),
		EmailVerificationTokenTTL: r.duration("EMAIL_VERIFICATION_TOKEN_TTL_MINUTES", 1440, time.Minute),

		SMTPHost:     r.str("SMTP_HOST", ""),
		SMTPPort:     r.str("SMTP_PORT", "587"),
		SMTPUsername: r.str("SMTP_USERNAME", ""),
		SMTPPassword: r.str("SMTP_PASSWORD", ""),
		SMTPFrom:     r.str("SMTP_FROM", ""),

		PasswordResetURLTemplate:     r.str("PASSWORD_RESET_URL_TEMPLATE", ""),
		EmailChangeURLTemplate:       r.str("EMAIL_CHANGE_URL_TEMPLATE", ""),
		EmailVerificationURLTemplate: r.str("EMAIL_VERIFICATION_URL_TEMPLATE", ""),

		CORSAllowedOrigins: parseList(r.str("CORS_ALLOWED_ORIGINS", "")),
		TrustedProxies:     parseList(r.str("TRUSTED_PROXIES", "")),

		DBMaxOpenConns:    r.integer("DB_MAX_OPEN_CONNS", 10, 1),
		DBMaxIdleConns:    r.integer("DB_MAX_IDLE_CONNS", 5, 0),
		DBConnMaxLifetime: r.duration("DB_CONN_MAX_LIFETIME_MINUTES", 5, time.Minute),

		ShutdownTimeout: r.duration("SHUTDOWN_TIMEOUT_SECONDS", 10, time.Second),
	}

	var invalidErr error
	if len(r.bad) > 0 {
		invalidErr = fmt.Errorf("invalid environment variable(s): %s", strings.Join(r.bad, ", "))
	}

	// Joined so one restart shows every mistake, not only the first kind.
	if err := errors.Join(invalidErr, validate(cfg)); err != nil {
		return nil, err
	}

	return cfg, nil
}

// firstNonEmpty returns the first non-empty value. It is how the PG*
// settings fall back to the POSTGRESQL_ADDON_* variables that a Clever Cloud
// PostgreSQL add-on injects into a linked application, so the app works there
// without duplicating every value by hand. An explicit PG* variable always
// wins over the add-on one.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func validate(cfg *Config) error {
	var missing []string

	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.PGHost == "" {
		missing = append(missing, "PGHOST")
	}
	if cfg.PGUser == "" {
		missing = append(missing, "PGUSER")
	}
	if cfg.PGPassword == "" {
		missing = append(missing, "PGPASSWORD")
	}
	if cfg.PGDatabase == "" {
		missing = append(missing, "PGDATABASE")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}

	for _, p := range cfg.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil && net.ParseIP(p) == nil {
			return fmt.Errorf("invalid TRUSTED_PROXIES entry %q: want a CIDR range or an IP", p)
		}
	}

	return nil
}

func normalizeBasePath(raw string) string {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}

	return "/" + trimmed
}

// parseList splits a comma-separated setting, trimming entries and dropping empty ones.
func parseList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			items = append(items, p)
		}
	}

	return items
}
