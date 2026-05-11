package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Slambot01/resilient-settlement-orchestrator/internal/config"
)

// NewPool creates a new PostgreSQL connection pool.
func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing database config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxConns)
	poolConfig.MinConns = int32(cfg.MinConns)
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	slog.Info("database connected",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.String("database", cfg.DBName),
		slog.Int("max_conns", cfg.MaxConns),
	)

	return pool, nil
}
