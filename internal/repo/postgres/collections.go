package postgres

import (
	"context"
	"fmt"

	"tonnet/internal/domain"
)

// UpsertCollection implements domain.CollectionRepository.
func (s *Store) UpsertCollection(ctx context.Context, c domain.Collection) error {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO collections (address, owner, name, description, image_url, metadata, next_item_index, indexed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (address) DO UPDATE SET
			owner = EXCLUDED.owner, name = EXCLUDED.name, description = EXCLUDED.description,
			image_url = EXCLUDED.image_url, metadata = EXCLUDED.metadata,
			next_item_index = EXCLUDED.next_item_index, indexed_at = now()`,
		c.Address, c.Owner, c.Name, c.Description, c.ImageURL, nonNil(c.Metadata), c.NextItemIndex)
	if err != nil {
		return fmt.Errorf("upsert collection %s: %w", c.Address, err)
	}
	return nil
}

const collectionCols = `c.address, COALESCE(c.owner,''), COALESCE(c.name,''), COALESCE(c.description,''), COALESCE(c.image_url,''), c.metadata, c.next_item_index,
	(SELECT count(*) FROM nft_items i WHERE i.collection_address = c.address), c.indexed_at`

// GetCollection implements domain.CollectionRepository.
func (s *Store) GetCollection(ctx context.Context, address string) (*domain.Collection, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var c domain.Collection
	err := s.pool.QueryRow(ctx, `SELECT `+collectionCols+` FROM collections c WHERE c.address = $1`, address).
		Scan(&c.Address, &c.Owner, &c.Name, &c.Description, &c.ImageURL, &c.Metadata, &c.NextItemIndex, &c.ItemsCount, &c.IndexedAt)
	if err != nil {
		return nil, fmt.Errorf("get collection %s: %w", address, mapErr(err))
	}
	return &c, nil
}

// ListCollections implements domain.CollectionRepository.
func (s *Store) ListCollections(ctx context.Context, page domain.Page) ([]domain.Collection, int, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM collections`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count collections: %w", err)
	}
	cs, err := s.queryCollections(ctx, `SELECT `+collectionCols+` FROM collections c ORDER BY c.indexed_at DESC, c.address LIMIT $1 OFFSET $2`, page.Size, page.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("list collections: %w", err)
	}
	return cs, total, nil
}

// AllCollections implements domain.CollectionRepository.
func (s *Store) AllCollections(ctx context.Context) ([]domain.Collection, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	cs, err := s.queryCollections(ctx, `SELECT `+collectionCols+` FROM collections c ORDER BY c.indexed_at DESC, c.address`)
	if err != nil {
		return nil, fmt.Errorf("list all collections: %w", err)
	}
	return cs, nil
}

func (s *Store) queryCollections(ctx context.Context, sql string, args ...any) ([]domain.Collection, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Collection{}
	for rows.Next() {
		var c domain.Collection
		if err := rows.Scan(&c.Address, &c.Owner, &c.Name, &c.Description, &c.ImageURL, &c.Metadata, &c.NextItemIndex, &c.ItemsCount, &c.IndexedAt); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func nonNil(b []byte) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}
