package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/config"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.PGHost, cfg.PGPort, cfg.PGUser, cfg.PGPassword, cfg.PGDatabase,
	)

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	// infrastructure -> application -> interface
	cardRepo := card.NewPostgresRepository(db)
	cardService := card.NewService(cardRepo)
	cardHandler := card.NewHandler(cardService)

	router := gin.Default()
	cardHandler.RegisterRoutes(router)

	logger.Info("starting server", zap.String("port", cfg.AppPort))
	if err := router.Run(":" + cfg.AppPort); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}
