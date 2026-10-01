package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/bulk"
	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/config"
	"Melrakkiie/Tamiyo/internal/cors"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/deckinsights"
	"Melrakkiie/Tamiyo/internal/health"
	"Melrakkiie/Tamiyo/internal/mail"
	"Melrakkiie/Tamiyo/internal/passwordreset"
	"Melrakkiie/Tamiyo/internal/ratelimit"
	"Melrakkiie/Tamiyo/internal/scryfall"
	"Melrakkiie/Tamiyo/internal/security"
	"Melrakkiie/Tamiyo/internal/storage"
	"Melrakkiie/Tamiyo/internal/token"
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

	tokenRepo := token.NewPostgresRepository(db)
	tokenService := token.NewService(tokenRepo, cfg.JWTRefreshTokenTTL)
	tokenHandler := token.NewHandler(tokenService, cfg.JWTSecret, cfg.JWTAccessTokenTTL)

	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	userHandler := user.NewHandler(userService, cfg.JWTSecret, cfg.JWTAccessTokenTTL, tokenService)

	var mailer mail.Mailer
	if cfg.SMTPHost != "" {
		mailer = mail.NewSMTPMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom, cfg.PasswordResetURLTemplate)
	} else {
		mailer = mail.NewLoggingMailer(logger)
	}

	passwordResetRepo := passwordreset.NewPostgresRepository(db)
	passwordResetService := passwordreset.NewService(passwordResetRepo, cfg.PasswordResetTokenTTL)
	passwordResetHandler := passwordreset.NewHandler(passwordResetService, userService, tokenService, mailer)

	cardRepo := card.NewPostgresRepository(db)
	cardService := card.NewService(cardRepo)
	cardHandler := card.NewHandler(cardService)

	storageRepo := storage.NewPostgresRepository(db)
	storageService := storage.NewService(storageRepo)
	storageHandler := storage.NewHandler(storageService)

	deckRepo := deck.NewPostgresRepository(db)
	deckService := deck.NewService(deckRepo)
	deckHandler := deck.NewHandler(deckService)

	importService := bulk.NewService(cardService, storageService, deckService, bulk.NewScryfallClient())
	importHandler := bulk.NewHandler(importService)

	insightsService := deckinsights.NewService(deckService, scryfall.NewClient())
	insightsHandler := deckinsights.NewHandler(insightsService)

	healthHandler := health.NewHandler(db)

	authLimiter := ratelimit.NewLimiter(cfg.AuthRateLimitMax, cfg.AuthRateLimitWindow)

	router := gin.Default()
	router.Use(security.Headers())
	router.Use(cors.Middleware(cfg.CORSAllowedOrigins))

	healthHandler.RegisterRoutes(router)
	userHandler.RegisterRoutes(router, ratelimit.Middleware(authLimiter))
	tokenHandler.RegisterRoutes(router)
	passwordResetHandler.RegisterRoutes(router, ratelimit.Middleware(authLimiter))

	protected := router.Group("/")
	protected.Use(auth.RequireAuth(cfg.JWTSecret))
	cardHandler.RegisterRoutes(protected)
	storageHandler.RegisterRoutes(protected)
	deckHandler.RegisterRoutes(protected)
	importHandler.RegisterRoutes(protected)
	insightsHandler.RegisterRoutes(protected)
	userHandler.RegisterProtectedRoutes(protected)

	logger.Info("starting server", zap.String("port", cfg.AppPort))
	if err := router.Run(":" + cfg.AppPort); err != nil {
		logger.Fatal("server failed", zap.Error(err))
	}
}
