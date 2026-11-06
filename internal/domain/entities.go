// Package domain holds the marketplace entities and business rules. It depends
// on nothing but the standard library: chain adapters, storage and transport
// all point inward to this package.
package domain

import (
	"encoding/json"
	"time"
)

// Collection is an indexed NFT collection (TEP-62 collection contract).
type Collection struct {
	Address       string
	Owner         string
	Name          string
	Description   string
	ImageURL      string
	Metadata      json.RawMessage
	NextItemIndex int64
	// ItemsCount is the number of items indexed so far (read model only).
	ItemsCount int64
	IndexedAt  time.Time
}

// NFTItem is an indexed NFT item (TEP-62 item contract, TEP-64 metadata).
type NFTItem struct {
	Address           string
	CollectionAddress string
	Index             int64
	Owner             string
	Name              string
	ImageURL          string
	Metadata          json.RawMessage
	UpdatedAt         time.Time
}

// ListingStatus is the lifecycle state of a fixed-price listing.
type ListingStatus string

// Listing lifecycle: pending (sale contract deploy requested) -> active (NFT
// owned by the sale contract) -> sold | cancelled.
const (
	ListingPending   ListingStatus = "pending"
	ListingActive    ListingStatus = "active"
	ListingSold      ListingStatus = "sold"
	ListingCancelled ListingStatus = "cancelled"
)

// IsTerminal reports whether no further transitions are possible.
func (s ListingStatus) IsTerminal() bool {
	return s == ListingSold || s == ListingCancelled
}

// Listing is a fixed-price sale of one NFT through a Getgems sale contract.
type Listing struct {
	ID           int64
	NFTAddress   string
	SaleContract string
	Seller       string
	Buyer        string
	PriceNano    int64
	Status       ListingStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TxType classifies a tracked transaction.
type TxType string

// Transaction types.
const (
	TxList     TxType = "list"
	TxBuy      TxType = "buy"
	TxCancel   TxType = "cancel"
	TxTransfer TxType = "transfer"
	TxMint     TxType = "mint"
)

// TxStatus is the confirmation state of a tracked transaction.
type TxStatus string

// Transaction statuses.
const (
	TxPending   TxStatus = "pending"
	TxConfirmed TxStatus = "confirmed"
	TxFailed    TxStatus = "failed"
)

// Transaction is either a user-submitted transaction awaiting confirmation
// (Hash = normalized external message hash) or an on-chain transaction seen by
// the indexer (Hash = on-chain transaction hash).
type Transaction struct {
	Hash       string
	Type       TxType
	NFTAddress string
	ListingID  *int64
	From       string
	To         string
	AmountNano int64
	LT         uint64
	Status     TxStatus
	BOC        string
	ExpiresAt  *time.Time
	SeenAt     time.Time
	UpdatedAt  time.Time
}

// ItemWithListing is an NFT item together with its current open listing, if any.
type ItemWithListing struct {
	Item    NFTItem
	Listing *Listing
}

// Page describes an offset-based page request.
type Page struct {
	Number int
	Size   int
}

// Offset converts the page to a SQL offset.
func (p Page) Offset() int {
	if p.Number < 1 {
		return 0
	}
	return (p.Number - 1) * p.Size
}

// ItemSort is a sort order for item listings.
type ItemSort string

// Item sort orders.
const (
	SortByIndex     ItemSort = "index"
	SortByPriceAsc  ItemSort = "price_asc"
	SortByPriceDesc ItemSort = "price_desc"
)

// ListingSort is a sort order for listing pages.
type ListingSort string

// Listing sort orders.
const (
	ListingSortNewest    ListingSort = "newest"
	ListingSortPriceAsc  ListingSort = "price_asc"
	ListingSortPriceDesc ListingSort = "price_desc"
)
