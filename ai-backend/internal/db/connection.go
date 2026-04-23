package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qa-automation/ai-backend/internal/config"
)

// DB represents the database connection
type DB struct {
	*pgxpool.Pool
}

// NewConnection creates a new database connection
func NewConnection(cfg *config.Config) (*DB, error) {
	// Get connection string from config
	connString := cfg.GetDatabaseURL()

	// Create connection pool
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = map[string]string{}
	}
	// Keep schema resolution consistent across environments.
	poolConfig.ConnConfig.RuntimeParams["search_path"] = "public,qa,ai,logging"

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{pool}, nil
}

// Close closes the database connection
func (db *DB) Close() {
	db.Pool.Reset()
}
