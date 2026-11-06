package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tonnet/internal/domain"
)

// eventsChannel is the LISTEN/NOTIFY channel that carries domain events from
// the indexer process to the API process (which fans them out over WebSocket).
const eventsChannel = "tonnet_events"

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func notify(ctx context.Context, db execer, e domain.Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if _, err := db.Exec(ctx, `SELECT pg_notify($1, $2)`, eventsChannel, string(payload)); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// Listener delivers domain events published through Postgres to a handler.
type Listener struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewListener creates a listener.
func NewListener(pool *pgxpool.Pool, log *slog.Logger) *Listener {
	return &Listener{pool: pool, log: log.With("component", "pg-listener")}
}

// Run blocks until ctx is done, reconnecting on errors.
func (l *Listener) Run(ctx context.Context, handle func(domain.Event)) {
	for ctx.Err() == nil {
		if err := l.listenOnce(ctx, handle); err != nil && ctx.Err() == nil {
			l.log.Warn("listener disconnected, retrying", "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (l *Listener) listenOnce(ctx context.Context, handle func(domain.Event)) error {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `LISTEN `+eventsChannel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("wait notification: %w", err)
		}
		var e domain.Event
		if err := json.Unmarshal([]byte(n.Payload), &e); err != nil {
			l.log.Warn("bad event payload", "err", err)
			continue
		}
		handle(e)
	}
}
