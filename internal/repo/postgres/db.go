// Package postgres implements the domain repositories on PostgreSQL via pgx.
// SQL is handwritten and explicit; every call takes a context so callers can
// bound latency.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5 driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tonnet/internal/domain"
	"tonnet/migrations"
)

// Connect opens a pool and verifies connectivity.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies all embedded migrations. golang-migrate takes an advisory
// lock, so the api and indexer may both call this concurrently at start-up.
func Migrate(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgxURL(dsn))
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func pgxURL(dsn string) string {
	const prefix = "postgres://"
	if len(dsn) > len(prefix) && dsn[:len(prefix)] == prefix {
		return "pgx5://" + dsn[len(prefix):]
	}
	const prefix2 = "postgresql://"
	if len(dsn) > len(prefix2) && dsn[:len(prefix2)] == prefix2 {
		return "pgx5://" + dsn[len(prefix2):]
	}
	return dsn
}

// Store groups all repositories over one pool. It satisfies every domain
// repository port plus domain.EventPublisher.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps a pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the pool for listeners.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// defaultQueryTimeout bounds every single statement; callers may pass tighter
// deadlines through ctx.
const defaultQueryTimeout = 5 * time.Second

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
