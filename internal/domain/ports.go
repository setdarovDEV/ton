package domain

import (
	"context"
	"time"
)

// CollectionRepository persists collections.
type CollectionRepository interface {
	UpsertCollection(ctx context.Context, c Collection) error
	GetCollection(ctx context.Context, address string) (*Collection, error)
	ListCollections(ctx context.Context) ([]Collection, error)
}

// ItemRepository persists NFT items.
type ItemRepository interface {
	UpsertItem(ctx context.Context, item NFTItem) error
	GetItem(ctx context.Context, address string) (*NFTItem, error)
	ListItemsByCollection(ctx context.Context, collection string, sort ItemSort, page Page) ([]ItemWithListing, int, error)
	ListItemsByOwner(ctx context.Context, owner string) ([]ItemWithListing, error)
	// ListItemsBySaleContracts returns items whose on-chain owner is one of the
	// given sale contracts, so "My NFTs" can show items the user has listed.
	ListItemsBySaleContracts(ctx context.Context, contracts []string) ([]ItemWithListing, error)
	// ItemOwners returns the known owner of every item of the collection.
	ItemOwners(ctx context.Context, collection string) (map[string]string, error)
}

// ListingRepository persists listings.
type ListingRepository interface {
	CreateListing(ctx context.Context, l Listing) (*Listing, error)
	GetListing(ctx context.Context, id int64) (*Listing, error)
	GetListingBySaleContract(ctx context.Context, sale string) (*Listing, error)
	// GetOpenListingByNFT returns the pending or active listing for an NFT.
	GetOpenListingByNFT(ctx context.Context, nft string) (*Listing, error)
	ListListings(ctx context.Context, status ListingStatus, sort ListingSort, page Page) ([]ItemWithListing, int, error)
	ListOpenListingsBySeller(ctx context.Context, seller string) ([]Listing, error)
	ListOpenListings(ctx context.Context) ([]Listing, error)
	UpdateListingStatus(ctx context.Context, id int64, status ListingStatus, buyer string) error
}

// TransactionRepository persists tracked transactions.
type TransactionRepository interface {
	UpsertTransaction(ctx context.Context, tx Transaction) error
	GetTransaction(ctx context.Context, hash string) (*Transaction, error)
	ListTransactionsByNFT(ctx context.Context, nft string, limit int) ([]Transaction, error)
	ListPendingTransactions(ctx context.Context) ([]Transaction, error)
	UpdateTransactionStatus(ctx context.Context, hash string, status TxStatus, lt uint64) error
	// ConfirmPendingForNFT marks pending transactions of the given type for an
	// NFT as confirmed; it returns the affected hashes.
	ConfirmPendingForNFT(ctx context.Context, nft string, typ TxType) ([]string, error)
}

// CursorRepository persists indexer progress per key (collection / item address).
type CursorRepository interface {
	GetCursor(ctx context.Context, key string) (uint64, error)
	SetCursor(ctx context.Context, key string, lt uint64) error
}

// Event is a realtime notification emitted when a listing or transaction
// changes state; the API fans it out over WebSocket.
type Event struct {
	Type       string    `json:"type"` // listing.updated | tx.updated
	NFTAddress string    `json:"nft_address,omitempty"`
	ListingID  int64     `json:"listing_id,omitempty"`
	TxHash     string    `json:"tx_hash,omitempty"`
	Status     string    `json:"status"`
	At         time.Time `json:"at"`
}

// EventPublisher publishes events to interested processes.
type EventPublisher interface {
	Publish(ctx context.Context, e Event) error
}

// Message is a chain-agnostic description of a wallet message the frontend
// should send through TON Connect.
type Message struct {
	Address    string // destination, user-friendly form
	AmountNano int64
	Payload    string // base64 BOC, empty for plain transfer
}

// ListingIntent is what the backend prepares for a seller: the deterministic
// sale contract address and the messages that deploy it and hand over the NFT.
type ListingIntent struct {
	SaleContract string
	Messages     []Message
}

// RoyaltyParams are read from the collection (TEP-66); never computed here.
type RoyaltyParams struct {
	Numerator   int64
	Denominator int64
	Address     string
}

// SaleTerms are the economic parameters of a fixed-price sale.
type SaleTerms struct {
	NFTAddress         string
	Seller             string
	PriceNano          int64
	MarketplaceFeeNano int64
	Royalty            RoyaltyParams
	QueryID            uint64
}

// PayloadBuilder encodes chain-specific messages. Implementations live in
// internal/chain/<chain>; the domain only knows these shapes.
type PayloadBuilder interface {
	BuildListing(terms SaleTerms) (*ListingIntent, error)
	BuildBuy(saleContract string, priceNano int64, queryID uint64) (Message, error)
	BuildCancel(saleContract string, queryID uint64) (Message, error)
	// MessageHash returns the normalized hash of an external message BOC.
	MessageHash(boc string) (string, error)
}

// RoyaltyReader fetches royalty parameters from chain.
type RoyaltyReader interface {
	GetRoyaltyParams(ctx context.Context, collection string) (*RoyaltyParams, error)
}

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}

// RealClock uses the system clock.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }
