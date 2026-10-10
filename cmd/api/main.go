package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"

	"Melrakkiie/Tamiyo/_devops/database/migrations"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/authcookie"
	"Melrakkiie/Tamiyo/internal/bulk"
	"Melrakkiie/Tamiyo/internal/card"
	"Melrakkiie/Tamiyo/internal/config"
	"Melrakkiie/Tamiyo/internal/cors"
	"Melrakkiie/Tamiyo/internal/deck"
	"Melrakkiie/Tamiyo/internal/deckinsights"
	"Melrakkiie/Tamiyo/internal/deckshare"
	"Melrakkiie/Tamiyo/internal/emailchange"
	"Melrakkiie/Tamiyo/internal/follow"
	"Melrakkiie/Tamiyo/internal/health"
	"Melrakkiie/Tamiyo/internal/httplog"
	"Melrakkiie/Tamiyo/internal/mail"
	"Melrakkiie/Tamiyo/internal/passwordreset"
	"Melrakkiie/Tamiyo/internal/preference"
	"Melrakkiie/Tamiyo/internal/printing"
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
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.PGHost, cfg.PGPort, cfg.PGUser, cfg.PGPassword, cfg.PGDatabase, cfg.PGSSLMode,
	)

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		logger.Fatal("failed to set migration dialect", zap.Error(err))
	}
	if err := goose.Up(db.DB, "."); err != nil {
		logger.Fatal("failed to apply database migrations", zap.Error(err))
	}
	logger.Info("database migrations applied")

	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("failed to close database connection", zap.Error(err))
		}
	}()

	tokenRepo := token.NewPostgresRepository(db)
	tokenService := token.NewService(tokenRepo, cfg.JWTRefreshTokenTTL)
	refreshCookie := authcookie.New(cfg.APIBasePath, cfg.RefreshCookieSecure, cfg.JWTRefreshTokenTTL)

	tokenHandler := token.NewHandler(tokenService, cfg.JWTSecret, cfg.JWTAccessTokenTTL, refreshCookie)

	userRepo := user.NewPostgresRepository(db)
	userService := user.NewService(userRepo)
	userHandler := user.NewHandler(userService, cfg.JWTSecret, cfg.JWTAccessTokenTTL, tokenService, refreshCookie)

	var mailer mail.Mailer
	if cfg.SMTPHost != "" {
		mailer = mail.NewSMTPMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom, cfg.PasswordResetURLTemplate, cfg.EmailChangeURLTemplate)
	} else {
		mailer = mail.NewLoggingMailer(logger)
	}

	passwordResetRepo := passwordreset.NewPostgresRepository(db)
	passwordResetService := passwordreset.NewService(passwordResetRepo, cfg.PasswordResetTokenTTL)
	passwordResetHandler := passwordreset.NewHandler(passwordResetService, userService, tokenService, mailer)

	emailChangeRepo := emailchange.NewPostgresRepository(db)
	emailChangeService := emailchange.NewService(emailChangeRepo, cfg.EmailChangeTokenTTL)
	emailChangeHandler := emailchange.NewHandler(emailChangeService, userService, mailer)

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

	shareService := deckshare.NewService(deckService, userService, insightsService)
	shareHandler := deckshare.NewHandler(shareService)

	followHandler := follow.NewHandler(follow.NewService(follow.NewPostgresRepository(db)))
	preferenceHandler := preference.NewHandler(preference.NewService(preference.NewPostgresRepository(db)))

	healthHandler := health.NewHandler(db)

	refresherCtx, stopRefresher := context.WithCancel(context.Background())
	defer stopRefresher()
	go printing.NewRefresher(printing.NewPostgresRepository(db), scryfall.NewClient(), logger).Run(refresherCtx)

	authLimiter := ratelimit.NewLimiter(cfg.AuthRateLimitMax, cfg.AuthRateLimitWindow)
	shareLimiter := ratelimit.NewLimiter(cfg.ShareRateLimitMax, cfg.ShareRateLimitWindow)

	router := gin.New()
	router.Use(httplog.Recovery(logger))
	router.Use(httplog.Middleware(logger))
	router.Use(security.Headers())
	router.Use(cors.Middleware(cfg.CORSAllowedOrigins))

	api := router.Group(cfg.APIBasePath)

	healthHandler.RegisterRoutes(router)
	if cfg.APIBasePath != "" {
		healthHandler.RegisterRoutes(api)
	}

	userHandler.RegisterRoutes(api, ratelimit.Middleware(authLimiter))
	tokenHandler.RegisterRoutes(api)
	passwordResetHandler.RegisterRoutes(api, ratelimit.Middleware(authLimiter))
	emailChangeHandler.RegisterRoutes(api, ratelimit.Middleware(authLimiter))
	shareHandler.RegisterRoutes(api.Group("", ratelimit.Middleware(shareLimiter)))
	importHandler.RegisterPublicRoutes(api.Group("", ratelimit.Middleware(shareLimiter)))

	protected := api.Group("")
	protected.Use(auth.RequireAuth(cfg.JWTSecret))
	cardHandler.RegisterRoutes(protected)
	storageHandler.RegisterRoutes(protected)
	deckHandler.RegisterRoutes(protected)
	importHandler.RegisterRoutes(protected)
	insightsHandler.RegisterRoutes(protected)
	shareHandler.RegisterProtectedRoutes(protected)
	userHandler.RegisterProtectedRoutes(protected)
	emailChangeHandler.RegisterProtectedRoutes(protected)
	preferenceHandler.RegisterRoutes(protected)
	followHandler.RegisterRoutes(protected)

	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("starting server", zap.String("port", cfg.AppPort))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	// Block until SIGINT/SIGTERM (e.g. a platform redeploy or `docker stop`),
	// then stop accepting new connections and give in-flight requests a
	// chance to finish before the process exits.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	<-ctx.Done()
	stop()

	logger.Info("shutting down server")
	stopRefresher()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown did not complete cleanly", zap.Error(err))
	}
}
