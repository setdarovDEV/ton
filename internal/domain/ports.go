package domain

import (
	"context"
	"time"
)

// CollectionRepository persists collections.
type CollectionRepository interface {
	UpsertCollection(ctx context.Context, c Collection) error
	GetCollection(ctx context.Context, address string) (*Collection, error)
	// ListCollections pages indexed collections; total is the unpaged count.
	ListCollections(ctx context.Context, page Page) ([]Collection, int, error)
	// AllCollections returns every tracked collection (indexer bootstrap).
	AllCollections(ctx context.Context) ([]Collection, error)
}

// ItemRepository persists NFT items.
type ItemRepository interface {
	UpsertItem(ctx context.Context, item NFTItem) error
	GetItem(ctx context.Context, address string) (*NFTItem, error)
	ListItemsByCollection(ctx context.Context, collection string, sort ItemSort, page Page) ([]ItemWithListing, int, error)
	// ListItemsByWallet pages the NFTs a wallet owns directly plus the ones it
	// has put on sale (owned by its own pending/active sale contracts), so
	// "My NFTs" keeps showing listed items; total is the unpaged count.
	ListItemsByWallet(ctx context.Context, wallet string, page Page) ([]ItemWithListing, int, error)
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
	// UpdateListingStatus transitions a listing. Implementations MUST emit an
	// EventListingUpdated event atomically with the write (same transaction),
	// because the API process relays those events to WebSocket clients. buyer
	// is only meaningful for ListingSold and may be empty otherwise.
	UpdateListingStatus(ctx context.Context, id int64, status ListingStatus, buyer string) error
}

// TransactionRepository persists tracked transactions.
type TransactionRepository interface {
	UpsertTransaction(ctx context.Context, tx Transaction) error
	GetTransaction(ctx context.Context, hash string) (*Transaction, error)
	ListTransactionsByNFT(ctx context.Context, nft string, limit int) ([]Transaction, error)
	ListPendingTransactions(ctx context.Context) ([]Transaction, error)
	// UpdateTransactionStatus transitions a tracked transaction. Implementations
	// MUST emit an EventTxUpdated event atomically with the write.
	UpdateTransactionStatus(ctx context.Context, hash string, status TxStatus, lt uint64) error
	// ConfirmPendingForNFT marks pending transactions of the given type for an
	// NFT as confirmed; it returns the affected hashes. Implementations MUST
	// emit one EventTxUpdated event per returned hash.
	ConfirmPendingForNFT(ctx context.Context, nft string, typ TxType) ([]string, error)
}

// CursorRepository persists indexer progress per key (collection / item address).
type CursorRepository interface {
	GetCursor(ctx context.Context, key string) (uint64, error)
	SetCursor(ctx context.Context, key string, lt uint64) error
}

// EventType names the realtime events emitted by repositories.
type EventType string

// Event types. The strings are part of the WebSocket contract (web/src/lib/types.ts).
const (
	EventListingUpdated EventType = "listing.updated"
	EventTxUpdated      EventType = "tx.updated"
)

// Event is a realtime notification emitted by repositories when a listing or
// transaction changes state (see ListingRepository.UpdateListingStatus); the
// API fans it out over WebSocket.
type Event struct {
	Type       EventType `json:"type"`
	NFTAddress string    `json:"nft_address,omitempty"`
	ListingID  int64     `json:"listing_id,omitempty"`
	TxHash     string    `json:"tx_hash,omitempty"`
	Status     string    `json:"status"`
	At         time.Time `json:"at"`
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

// SaleTerms are the economic parameters of a fixed-price sale. All amounts
// are absolute nanotons already computed by the domain (see ComputeFees);
// builders only encode them.
type SaleTerms struct {
	NFTAddress         string
	Seller             string
	PriceNano          int64
	MarketplaceFeeNano int64
	RoyaltyNano        int64
	RoyaltyAddress     string // empty when no royalty is paid
	QueryID            uint64
}

// ComputeFees applies the marketplace fee (basis points) and the collection's
// TEP-66 royalty share to a price. Royalty is only read from chain and
// applied here, never recomputed elsewhere.
func ComputeFees(priceNano, feeBPS int64, r RoyaltyParams) (feeNano, royaltyNano int64) {
	feeNano = priceNano * feeBPS / 10_000
	if r.Denominator > 0 && r.Numerator > 0 {
		royaltyNano = priceNano * r.Numerator / r.Denominator
	}
	return feeNano, royaltyNano
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
