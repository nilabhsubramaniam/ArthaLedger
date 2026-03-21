// Package main is the entry point for the ArthaLedger API server.
// It wires together configuration, database connections, and HTTP routes,
// then starts the server with graceful shutdown support.
//
// Swagger / OpenAPI documentation is generated from the annotations in this
// file and in the handler packages. Run `make swagger` to regenerate.
//
//	@title           ArthaLedger API
//	@version         1.0
//	@description     Personal finance tracker — accounts, transactions, budgets, and alerts.
//
//	@contact.name   ArthaLedger Support
//	@contact.email  support@arthaledger.io
//
//	@license.name  MIT
//
//	@host      localhost:8080
//	@BasePath  /api/v1
//
//	@securityDefinitions.apikey  BearerAuth
//	@in                          header
//	@name                        Authorization
//	@description                 Type "Bearer" followed by a space and the JWT access token.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/config"
	"github.com/nilabh/arthaledger/internal/auth"
	"github.com/nilabh/arthaledger/pkg/database"
	"github.com/nilabh/arthaledger/pkg/middleware"

	// Generated swagger docs — imported for side-effect only (registers docs with gin-swagger).
	_ "github.com/nilabh/arthaledger/docs"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	// Initialize structured logger first so all subsequent log calls work
	setupLogger()

	// Load configuration from .env
	cfg := config.Load()

	// ── Database connections ────────────────────────────────────────────────

	// Connect to PostgreSQL
	db, err := database.NewPostgresDB(cfg)
	if err != nil {
		slog.Error("Failed to connect to PostgreSQL", "error", err)
		os.Exit(1)
	}
	// Close the underlying sql.DB on shutdown
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	// Connect to Redis (optional in development – server starts without it)
	redisClient, err := database.NewRedisClient(cfg)
	if err != nil {
		if cfg.App.Env == "development" {
			slog.Warn("Redis unavailable – caching disabled (start Redis or Docker to enable)", "error", err)
		} else {
			slog.Error("Failed to connect to Redis", "error", err)
			os.Exit(1)
		}
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	// ── Wire application dependencies ───────────────────────────────────────
	// Each layer receives only what it needs, following the dependency-inversion
	// principle: handlers depend on service interfaces, services on repositories.

	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(authRepo, redisClient, cfg)
	authHandler := auth.NewHandler(authSvc)

	// ── HTTP server setup ───────────────────────────────────────────────────

	// Silence Gin's debug noise in production
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// ── Swagger UI ─────────────────────────────────────────────────────────
	// Available at http://localhost:<port>/swagger/index.html
	// Only active when docs have been generated via `make swagger`.
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Health check endpoint (no auth required)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"status": "ok",
			},
		})
	})

	// ── API v1 routes ───────────────────────────────────────────────────────
	apiV1 := router.Group("/api/v1")
	{
		// Public auth routes — register, login, refresh (no token required)
		authHandler.RegisterRoutes(apiV1.Group("/auth"))

		// Protected logout route — requires a valid Bearer token.
		// The middleware validates the JWT and populates the Gin context with
		// user_id, email, jti, and token_expiry for downstream handlers.
		protected := apiV1.Group("", middleware.Auth(cfg, redisClient))
		{
			protected.DELETE("/auth/logout", authHandler.LogoutHandler())
		}
	}

	// Wrap Gin inside a proper http.Server so we can set timeouts and shut down gracefully
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Start server in background goroutine ────────────────────────────────
	go func() {
		slog.Info("ArthaLedger API server starting",
			"port", cfg.App.Port,
			"environment", cfg.App.Env,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server encountered a fatal error", "error", err)
			os.Exit(1)
		}
	}()

	// ── Graceful shutdown ───────────────────────────────────────────────────
	// Block until we receive SIGINT or SIGTERM (Ctrl+C or kill)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutdown signal received, draining connections…")

	// Give in-flight requests up to 30 s to finish
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	slog.Info("ArthaLedger server stopped cleanly")
}

// setupLogger initializes the structured logger with slog.
func setupLogger() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
	slog.Info("Logger initialized")
}
