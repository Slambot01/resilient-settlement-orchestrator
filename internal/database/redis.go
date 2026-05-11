package database

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/config"
)

// NewRedisClient creates a new Redis client with the given configuration.
func NewRedisClient(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: 20,
	})

	// Verify connectivity
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	slog.Info("redis connected",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.Int("db", cfg.DB),
	)

	return client, nil
}
