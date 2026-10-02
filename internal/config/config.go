package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	PGHost     string `mapstructure:"PGHOST"`
	PGPort     string `mapstructure:"PGPORT"`
	PGUser     string `mapstructure:"PGUSER"`
	PGPassword string `mapstructure:"PGPASSWORD"`
	PGDatabase string `mapstructure:"PGDATABASE"`
	PGSSLMode  string `mapstructure:"PGSSLMODE"`
	AppPort    string `mapstructure:"APP_PORT"`
	JWTSecret  string `mapstructure:"JWT_SECRET"`

	AuthRateLimitMax    int
	AuthRateLimitWindow time.Duration

	JWTAccessTokenTTL  time.Duration
	JWTRefreshTokenTTL time.Duration

	PasswordResetTokenTTL time.Duration

	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     string `mapstructure:"SMTP_PORT"`
	SMTPUsername string `mapstructure:"SMTP_USERNAME"`
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"`
	SMTPFrom     string `mapstructure:"SMTP_FROM"`

	PasswordResetURLTemplate string `mapstructure:"PASSWORD_RESET_URL_TEMPLATE"`

	CORSAllowedOrigins []string

	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	ShutdownTimeout time.Duration
}

func Load() (*Config, error) {
	viper.AutomaticEnv()
	viper.SetDefault("APP_PORT", "8080")
	viper.SetDefault("PGPORT", "5432")
	viper.SetDefault("PGSSLMODE", "disable")
	viper.SetDefault("AUTH_RATE_LIMIT_MAX", 5)
	viper.SetDefault("AUTH_RATE_LIMIT_WINDOW_SECONDS", 60)
	viper.SetDefault("JWT_ACCESS_TOKEN_TTL_MINUTES", 15)
	viper.SetDefault("JWT_REFRESH_TOKEN_TTL_DAYS", 30)
	viper.SetDefault("PASSWORD_RESET_TOKEN_TTL_MINUTES", 30)
	viper.SetDefault("SMTP_PORT", "587")
	viper.SetDefault("CORS_ALLOWED_ORIGINS", "")
	viper.SetDefault("DB_MAX_OPEN_CONNS", 10)
	viper.SetDefault("DB_MAX_IDLE_CONNS", 5)
	viper.SetDefault("DB_CONN_MAX_LIFETIME_MINUTES", 5)
	viper.SetDefault("SHUTDOWN_TIMEOUT_SECONDS", 10)

	cfg := &Config{
		PGHost:              viper.GetString("PGHOST"),
		PGPort:              viper.GetString("PGPORT"),
		PGUser:              viper.GetString("PGUSER"),
		PGPassword:          viper.GetString("PGPASSWORD"),
		PGDatabase:          viper.GetString("PGDATABASE"),
		PGSSLMode:           viper.GetString("PGSSLMODE"),
		AppPort:             viper.GetString("APP_PORT"),
		JWTSecret:           viper.GetString("JWT_SECRET"),
		AuthRateLimitMax:    viper.GetInt("AUTH_RATE_LIMIT_MAX"),
		AuthRateLimitWindow: time.Duration(viper.GetInt("AUTH_RATE_LIMIT_WINDOW_SECONDS")) * time.Second,
		JWTAccessTokenTTL:   time.Duration(viper.GetInt("JWT_ACCESS_TOKEN_TTL_MINUTES")) * time.Minute,
		JWTRefreshTokenTTL:  time.Duration(viper.GetInt("JWT_REFRESH_TOKEN_TTL_DAYS")) * 24 * time.Hour,

		PasswordResetTokenTTL: time.Duration(viper.GetInt("PASSWORD_RESET_TOKEN_TTL_MINUTES")) * time.Minute,

		SMTPHost:     viper.GetString("SMTP_HOST"),
		SMTPPort:     viper.GetString("SMTP_PORT"),
		SMTPUsername: viper.GetString("SMTP_USERNAME"),
		SMTPPassword: viper.GetString("SMTP_PASSWORD"),
		SMTPFrom:     viper.GetString("SMTP_FROM"),

		PasswordResetURLTemplate: viper.GetString("PASSWORD_RESET_URL_TEMPLATE"),

		CORSAllowedOrigins: parseOrigins(viper.GetString("CORS_ALLOWED_ORIGINS")),

		DBMaxOpenConns:    viper.GetInt("DB_MAX_OPEN_CONNS"),
		DBMaxIdleConns:    viper.GetInt("DB_MAX_IDLE_CONNS"),
		DBConnMaxLifetime: time.Duration(viper.GetInt("DB_CONN_MAX_LIFETIME_MINUTES")) * time.Minute,

		ShutdownTimeout: time.Duration(viper.GetInt("SHUTDOWN_TIMEOUT_SECONDS")) * time.Second,
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
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

	return nil
}

func parseOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}

	return origins
}
