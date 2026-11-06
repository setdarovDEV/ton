package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"tonnet/internal/domain"
)

// UpsertItem implements domain.ItemRepository.
func (s *Store) UpsertItem(ctx context.Context, it domain.NFTItem) error {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO nft_items (address, collection_address, item_index, owner, name, image_url, metadata, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (address) DO UPDATE SET
			collection_address = EXCLUDED.collection_address, item_index = EXCLUDED.item_index,
			owner = EXCLUDED.owner, name = EXCLUDED.name, image_url = EXCLUDED.image_url,
			metadata = EXCLUDED.metadata, updated_at = now()`,
		it.Address, it.CollectionAddress, it.Index, it.Owner, it.Name, it.ImageURL, nonNil(it.Metadata))
	if err != nil {
		return fmt.Errorf("upsert item %s: %w", it.Address, err)
	}
	return nil
}

// Item columns prefixed with i., open listing columns prefixed with l. (LEFT JOIN).
const itemWithListingSelect = `
	SELECT i.address, i.collection_address, i.item_index, COALESCE(i.owner,''), COALESCE(i.name,''), COALESCE(i.image_url,''), i.metadata, i.updated_at,
	       l.id, l.nft_address, l.sale_contract, l.seller, l.buyer, l.price_nano, l.status, l.created_at, l.updated_at
	FROM nft_items i
	LEFT JOIN listings l ON l.nft_address = i.address AND l.status IN ('pending','active')`

func scanItemWithListing(row pgx.Row) (domain.ItemWithListing, error) {
	var it domain.ItemWithListing
	var (
		id           *int64
		nft, sale    *string
		seller       *string
		buyer        *string
		price        *int64
		status       *string
		created, upd *time.Time
	)
	err := row.Scan(&it.Item.Address, &it.Item.CollectionAddress, &it.Item.Index, &it.Item.Owner, &it.Item.Name, &it.Item.ImageURL, &it.Item.Metadata, &it.Item.UpdatedAt,
		&id, &nft, &sale, &seller, &buyer, &price, &status, &created, &upd)
	if err != nil {
		return it, err
	}
	if id != nil {
		it.Listing = &domain.Listing{ID: *id, NFTAddress: deref(nft), SaleContract: deref(sale), Seller: deref(seller), Buyer: deref(buyer), PriceNano: *price, Status: domain.ListingStatus(deref(status))}
		if created != nil {
			it.Listing.CreatedAt = *created
		}
		if upd != nil {
			it.Listing.UpdatedAt = *upd
		}
	}
	return it, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Store) queryItems(ctx context.Context, sql string, args ...any) ([]domain.ItemWithListing, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ItemWithListing{}
	for rows.Next() {
		it, err := scanItemWithListing(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetItem implements domain.ItemRepository.
func (s *Store) GetItem(ctx context.Context, address string) (*domain.NFTItem, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	items, err := s.queryItems(ctx, itemWithListingSelect+` WHERE i.address = $1`, address)
	if err != nil {
		return nil, fmt.Errorf("get item %s: %w", address, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("get item %s: %w", address, domain.ErrNotFound)
	}
	return &items[0].Item, nil
}

// ListItemsByCollection implements domain.ItemRepository.
func (s *Store) ListItemsByCollection(ctx context.Context, collection string, sort domain.ItemSort, page domain.Page) ([]domain.ItemWithListing, int, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM nft_items WHERE collection_address = $1`, collection).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count items: %w", err)
	}
	order := `i.item_index ASC`
	switch sort {
	case domain.SortByPriceAsc:
		// Listed items first, cheapest first; unlisted items after, by index.
		order = `(l.status = 'active') DESC NULLS LAST, l.price_nano ASC NULLS LAST, i.item_index ASC`
	case domain.SortByPriceDesc:
		order = `(l.status = 'active') DESC NULLS LAST, l.price_nano DESC NULLS LAST, i.item_index ASC`
	case domain.SortByIndex:
	}
	items, err := s.queryItems(ctx, itemWithListingSelect+` WHERE i.collection_address = $1 ORDER BY `+order+` LIMIT $2 OFFSET $3`,
		collection, page.Size, page.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("list items: %w", err)
	}
	return items, total, nil
}

// ListItemsByOwner implements domain.ItemRepository.
func (s *Store) ListItemsByOwner(ctx context.Context, owner string) ([]domain.ItemWithListing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	items, err := s.queryItems(ctx, itemWithListingSelect+` WHERE i.owner = $1 ORDER BY i.updated_at DESC`, owner)
	if err != nil {
		return nil, fmt.Errorf("list items by owner: %w", err)
	}
	return items, nil
}

// ListItemsBySaleContracts implements domain.ItemRepository.
func (s *Store) ListItemsBySaleContracts(ctx context.Context, contracts []string) ([]domain.ItemWithListing, error) {
	if len(contracts) == 0 {
		return []domain.ItemWithListing{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	items, err := s.queryItems(ctx, itemWithListingSelect+` WHERE i.owner = ANY($1) ORDER BY i.updated_at DESC`, contracts)
	if err != nil {
		return nil, fmt.Errorf("list items by sale contracts: %w", err)
	}
	return items, nil
}

// ItemOwners implements domain.ItemRepository.
func (s *Store) ItemOwners(ctx context.Context, collection string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT address, COALESCE(owner,'') FROM nft_items WHERE collection_address = $1`, collection)
	if err != nil {
		return nil, fmt.Errorf("item owners: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var a, o string
		if err := rows.Scan(&a, &o); err != nil {
			return nil, fmt.Errorf("scan owner: %w", err)
		}
		out[a] = o
	}
	return out, rows.Err()
}
