package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nilabh/arthaledger/config"
	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates and returns a new Redis client.
// It returns an error instead of panicking so the caller can handle it gracefully.
func NewRedisClient(cfg *config.Config) (*redis.Client, error) {
	// Create Redis client with connection pool and timeout settings
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	// Verify connectivity before returning
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis (addr=%s:%d): %w",
			cfg.Redis.Host, cfg.Redis.Port, err)
	}

	slog.Info("Connected to Redis",
		"host", cfg.Redis.Host,
		"port", cfg.Redis.Port,
	)

	return client, nil
}
