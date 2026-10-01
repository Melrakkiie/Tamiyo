package config

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	PGHost     string `mapstructure:"PGHOST"`
	PGPort     string `mapstructure:"PGPORT"`
	PGUser     string `mapstructure:"PGUSER"`
	PGPassword string `mapstructure:"PGPASSWORD"`
	PGDatabase string `mapstructure:"PGDATABASE"`
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
}

func Load() (*Config, error) {
	viper.AutomaticEnv()
	viper.SetDefault("APP_PORT", "8080")
	viper.SetDefault("AUTH_RATE_LIMIT_MAX", 5)
	viper.SetDefault("AUTH_RATE_LIMIT_WINDOW_SECONDS", 60)
	viper.SetDefault("JWT_ACCESS_TOKEN_TTL_MINUTES", 15)
	viper.SetDefault("JWT_REFRESH_TOKEN_TTL_DAYS", 30)
	viper.SetDefault("PASSWORD_RESET_TOKEN_TTL_MINUTES", 30)
	viper.SetDefault("SMTP_PORT", "587")

	cfg := &Config{
		PGHost:              viper.GetString("PGHOST"),
		PGPort:              viper.GetString("PGPORT"),
		PGUser:              viper.GetString("PGUSER"),
		PGPassword:          viper.GetString("PGPASSWORD"),
		PGDatabase:          viper.GetString("PGDATABASE"),
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
	}

	return cfg, nil
}
