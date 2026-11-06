package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tonnet/internal/domain"
)

// openStatuses is the SQL form of the non-terminal listing statuses
// (domain.ListingStatus.IsTerminal); keep the two in sync.
const openStatuses = `('pending','active')`

const listingCols = `id, nft_address, sale_contract, seller, COALESCE(buyer,''), price_nano, status, created_at, updated_at`

func scanListing(row pgx.Row) (*domain.Listing, error) {
	var l domain.Listing
	if err := row.Scan(&l.ID, &l.NFTAddress, &l.SaleContract, &l.Seller, &l.Buyer, &l.PriceNano, &l.Status, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
	}
	return &l, nil
}

// CreateListing implements domain.ListingRepository.
func (s *Store) CreateListing(ctx context.Context, l domain.Listing) (*domain.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := s.pool.QueryRow(ctx, `
		INSERT INTO listings (nft_address, sale_contract, seller, price_nano, status)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+listingCols,
		l.NFTAddress, l.SaleContract, l.Seller, l.PriceNano, l.Status)
	created, err := scanListing(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("create listing: %w", domain.ErrConflict)
		}
		return nil, fmt.Errorf("create listing: %w", err)
	}
	return created, nil
}

// GetListing implements domain.ListingRepository.
func (s *Store) GetListing(ctx context.Context, id int64) (*domain.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	l, err := scanListing(s.pool.QueryRow(ctx, `SELECT `+listingCols+` FROM listings WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("get listing %d: %w", id, mapErr(err))
	}
	return l, nil
}

// GetListingBySaleContract implements domain.ListingRepository.
func (s *Store) GetListingBySaleContract(ctx context.Context, sale string) (*domain.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	l, err := scanListing(s.pool.QueryRow(ctx, `SELECT `+listingCols+` FROM listings WHERE sale_contract = $1`, sale))
	if err != nil {
		return nil, fmt.Errorf("get listing by sale %s: %w", sale, mapErr(err))
	}
	return l, nil
}

// GetOpenListingByNFT implements domain.ListingRepository.
func (s *Store) GetOpenListingByNFT(ctx context.Context, nft string) (*domain.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	l, err := scanListing(s.pool.QueryRow(ctx, `SELECT `+listingCols+` FROM listings
		WHERE nft_address = $1 AND status IN `+openStatuses+` ORDER BY created_at DESC LIMIT 1`, nft))
	if err != nil {
		return nil, fmt.Errorf("get open listing for %s: %w", nft, mapErr(err))
	}
	return l, nil
}

// ListListings implements domain.ListingRepository.
func (s *Store) ListListings(ctx context.Context, status domain.ListingStatus, sort domain.ListingSort, page domain.Page) ([]domain.ItemWithListing, int, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM listings WHERE status = $1`, status).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count listings: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT i.address, i.collection_address, i.item_index, COALESCE(i.owner,''), COALESCE(i.name,''), COALESCE(i.image_url,''), i.metadata, i.updated_at,
		       l.id, l.nft_address, l.sale_contract, l.seller, l.buyer, l.price_nano, l.status, l.created_at, l.updated_at
		FROM listings l JOIN nft_items i ON i.address = l.nft_address
		WHERE l.status = $1 ORDER BY `+listingOrder(sort)+` LIMIT $2 OFFSET $3`, status, page.Size, page.Offset())
	if err != nil {
		return nil, 0, fmt.Errorf("list listings: %w", err)
	}
	defer rows.Close()
	out := []domain.ItemWithListing{}
	for rows.Next() {
		it, err := scanItemWithListing(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan listing: %w", err)
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// listingOrder maps a domain sort to a fixed ORDER BY clause (never user input).
func listingOrder(sort domain.ListingSort) string {
	switch sort {
	case domain.ListingSortPriceAsc:
		return `l.price_nano ASC, l.id ASC`
	case domain.ListingSortPriceDesc:
		return `l.price_nano DESC, l.id DESC`
	default:
		return `l.updated_at DESC, l.id DESC`
	}
}

func (s *Store) queryListings(ctx context.Context, sql string, args ...any) ([]domain.Listing, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Listing{}
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return nil, fmt.Errorf("scan listing: %w", err)
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// ListOpenListingsBySeller implements domain.ListingRepository.
func (s *Store) ListOpenListingsBySeller(ctx context.Context, seller string) ([]domain.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	ls, err := s.queryListings(ctx, `SELECT `+listingCols+` FROM listings WHERE seller = $1 AND status IN `+openStatuses+``, seller)
	if err != nil {
		return nil, fmt.Errorf("list seller listings: %w", err)
	}
	return ls, nil
}

// ListOpenListings implements domain.ListingRepository.
func (s *Store) ListOpenListings(ctx context.Context) ([]domain.Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	ls, err := s.queryListings(ctx, `SELECT `+listingCols+` FROM listings WHERE status IN `+openStatuses+``)
	if err != nil {
		return nil, fmt.Errorf("list open listings: %w", err)
	}
	return ls, nil
}

// UpdateListingStatus implements domain.ListingRepository: the status change
// and the listing.updated notification are committed in one transaction.
func (s *Store) UpdateListingStatus(ctx context.Context, id int64, status domain.ListingStatus, buyer string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var nft string
	err = tx.QueryRow(ctx, `UPDATE listings SET status = $2, buyer = COALESCE(NULLIF($3,''), buyer), updated_at = now()
		WHERE id = $1 RETURNING nft_address`, id, status, buyer).Scan(&nft)
	if err != nil {
		return fmt.Errorf("update listing %d: %w", id, mapErr(err))
	}
	if err := notify(ctx, tx, domain.Event{Type: domain.EventListingUpdated, NFTAddress: nft, ListingID: id, Status: string(status), At: time.Now()}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
