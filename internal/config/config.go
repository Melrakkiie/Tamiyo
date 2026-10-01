package config

import "github.com/spf13/viper"

type Config struct {
	PGHost     string `mapstructure:"PGHOST"`
	PGPort     string `mapstructure:"PGPORT"`
	PGUser     string `mapstructure:"PGUSER"`
	PGPassword string `mapstructure:"PGPASSWORD"`
	PGDatabase string `mapstructure:"PGDATABASE"`
	AppPort    string `mapstructure:"APP_PORT"`
	JWTSecret  string `mapstructure:"JWT_SECRET"`
}

func Load() (*Config, error) {
	viper.AutomaticEnv()
	viper.SetDefault("APP_PORT", "8080")

	cfg := &Config{
		PGHost:     viper.GetString("PGHOST"),
		PGPort:     viper.GetString("PGPORT"),
		PGUser:     viper.GetString("PGUSER"),
		PGPassword: viper.GetString("PGPASSWORD"),
		PGDatabase: viper.GetString("PGDATABASE"),
		AppPort:    viper.GetString("APP_PORT"),
		JWTSecret:  viper.GetString("JWT_SECRET"),
	}

	return cfg, nil
}
