package postgres

import (
	"context"
	"errors"
	"fmt"

	"tonnet/internal/domain"
)

// GetCursor implements domain.CursorRepository; unknown keys return 0.
func (s *Store) GetCursor(ctx context.Context, key string) (uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var lt int64
	err := s.pool.QueryRow(ctx, `SELECT last_lt FROM indexer_cursor WHERE key = $1`, key).Scan(&lt)
	if err != nil {
		if errors.Is(mapErr(err), domain.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("get cursor %s: %w", key, err)
	}
	return uint64(lt), nil //nolint:gosec // non-negative
}

// SetCursor implements domain.CursorRepository; it never moves backwards.
func (s *Store) SetCursor(ctx context.Context, key string, lt uint64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := s.pool.Exec(ctx, `INSERT INTO indexer_cursor (key, last_lt) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET last_lt = GREATEST(indexer_cursor.last_lt, EXCLUDED.last_lt)`, key, int64(lt)) //nolint:gosec // fits
	if err != nil {
		return fmt.Errorf("set cursor %s: %w", key, err)
	}
	return nil
}
