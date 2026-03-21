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
	"github.com/nilabh/arthaledger/pkg/database"
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

	// ── HTTP server setup ───────────────────────────────────────────────────

	// Silence Gin's debug noise in production
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// Health check endpoint (no auth required)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"status": "ok",
			},
		})
	})

	// API v1 routes
	apiV1 := router.Group("/api/v1")
	{
		// Auth routes (to be implemented in Phase 1)
		authRoutes := apiV1.Group("/auth")
		{
			authRoutes.POST("/register", func(c *gin.Context) {
				c.JSON(http.StatusNotImplemented, gin.H{
					"success": false,
					"error":   "Not implemented yet",
				})
			})

			authRoutes.POST("/login", func(c *gin.Context) {
				c.JSON(http.StatusNotImplemented, gin.H{
					"success": false,
					"error":   "Not implemented yet",
				})
			})
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
