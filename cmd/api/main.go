package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/config"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/health"
	"Melrakkiie/Tamiyo/internal/storage"
	"Melrakkiie/Tamiyo/internal/user"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

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
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("failed to close database connection", zap.Error(err))
		}
	}()

	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	userHandler := user.NewHandler(userService, cfg.JWTSecret)

	cardRepo := card.NewPostgresRepository(db)
	cardService := card.NewService(cardRepo)
	cardHandler := card.NewHandler(cardService)

	storageRepo := storage.NewPostgresRepository(db)
	storageService := storage.NewService(storageRepo)
	storageHandler := storage.NewHandler(storageService)

	deckRepo := deck.NewPostgresRepository(db)
	deckService := deck.NewService(deckRepo)
	deckHandler := deck.NewHandler(deckService)

	healthHandler := health.NewHandler(db)

	router := gin.Default()

	healthHandler.RegisterRoutes(router)
	userHandler.RegisterRoutes(router)

	protected := router.Group("/")
	protected.Use(auth.RequireAuth(cfg.JWTSecret))
	cardHandler.RegisterRoutes(protected)
	storageHandler.RegisterRoutes(protected)
	deckHandler.RegisterRoutes(protected)

	logger.Info("starting server", zap.String("port", cfg.AppPort))
	if err := router.Run(":" + cfg.AppPort); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}
