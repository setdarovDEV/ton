package domain

import (
	"context"
	"errors"
	"fmt"
)

// CatalogService serves read models for the UI straight from the database,
// which is the source of truth for presentation (the chain is the source of
// truth for ownership and the indexer keeps the two in sync).
type CatalogService struct {
	collections CollectionRepository
	items       ItemRepository
	listings    ListingRepository
	txs         TransactionRepository
}

// NewCatalogService wires the service.
func NewCatalogService(c CollectionRepository, i ItemRepository, l ListingRepository, t TransactionRepository) *CatalogService {
	return &CatalogService{collections: c, items: i, listings: l, txs: t}
}

// Collections pages indexed collections.
func (s *CatalogService) Collections(ctx context.Context, page Page) ([]Collection, int, error) {
	cs, total, err := s.collections.ListCollections(ctx, page)
	if err != nil {
		return nil, 0, fmt.Errorf("list collections: %w", err)
	}
	return cs, total, nil
}

// CollectionNames maps every tracked collection address to its display name
// (for embedding into item read models).
func (s *CatalogService) CollectionNames(ctx context.Context) (map[string]string, error) {
	cs, err := s.collections.AllCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	out := make(map[string]string, len(cs))
	for _, c := range cs {
		out[c.Address] = c.Name
	}
	return out, nil
}

// Collection returns one collection.
func (s *CatalogService) Collection(ctx context.Context, address string) (*Collection, error) {
	c, err := s.collections.GetCollection(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	return c, nil
}

// CollectionItems pages through a collection's items with their active price.
func (s *CatalogService) CollectionItems(ctx context.Context, collection string, sort ItemSort, page Page) ([]ItemWithListing, int, error) {
	if _, err := s.collections.GetCollection(ctx, collection); err != nil {
		return nil, 0, fmt.Errorf("get collection: %w", err)
	}
	items, total, err := s.items.ListItemsByCollection(ctx, collection, sort, page)
	if err != nil {
		return nil, 0, fmt.Errorf("list items: %w", err)
	}
	return items, total, nil
}

// ItemDetail is the item page read model.
type ItemDetail struct {
	Item    NFTItem
	Listing *Listing
	History []Transaction
}

// Item returns an item with its open listing and transaction history.
func (s *CatalogService) Item(ctx context.Context, address string) (*ItemDetail, error) {
	item, err := s.items.GetItem(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	listing, err := s.listings.GetOpenListingByNFT(ctx, address)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get listing: %w", err)
	}
	history, err := s.txs.ListTransactionsByNFT(ctx, address, 50)
	if err != nil {
		return nil, fmt.Errorf("list history: %w", err)
	}
	return &ItemDetail{Item: *item, Listing: listing, History: history}, nil
}

// UserItems pages the NFTs a wallet owns directly plus the ones it has put
// on sale (owned by its sale contracts).
func (s *CatalogService) UserItems(ctx context.Context, owner string, page Page) ([]ItemWithListing, int, error) {
	items, total, err := s.items.ListItemsByWallet(ctx, owner, page)
	if err != nil {
		return nil, 0, fmt.Errorf("list wallet items: %w", err)
	}
	return items, total, nil
}

// Listings pages listings by status.
func (s *CatalogService) Listings(ctx context.Context, status ListingStatus, sort ListingSort, page Page) ([]ItemWithListing, int, error) {
	items, total, err := s.listings.ListListings(ctx, status, sort, page)
	if err != nil {
		return nil, 0, fmt.Errorf("list listings: %w", err)
	}
	return items, total, nil
}

// Listing returns a listing with its item.
func (s *CatalogService) Listing(ctx context.Context, id int64) (*ItemWithListing, error) {
	l, err := s.listings.GetListing(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get listing: %w", err)
	}
	item, err := s.items.GetItem(ctx, l.NFTAddress)
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	return &ItemWithListing{Item: *item, Listing: l}, nil
}
