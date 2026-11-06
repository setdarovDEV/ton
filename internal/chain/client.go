// Package chain defines the read interface the marketplace is built on.
// Concrete blockchains (TON today) implement Client and AddressCodec in
// sub-packages; the domain and HTTP layers only see these plain types.
//
// Scope of the abstraction: reads of collections, items, accounts and raw
// transactions are chain-agnostic. Interpreting what a transaction *means*
// (TEP-62 transfer / ownership_assigned op codes, mint fan-out) still lives in
// internal/indexer, so a second chain would need either TEP-compatible message
// semantics or a higher-level adapter method such as
// GetItemTransfers(ctx, item, afterLT, limit) returning ownership changes,
// which is where that logic would move.
package chain

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrNotFound is returned when the account/entity does not exist on chain.
var ErrNotFound = errors.New("chain: not found")

// AddressCodec converts between the canonical raw address used as a storage
// key and the chain's user-facing representation. Transport layers depend on
// this instead of on a concrete chain package.
type AddressCodec interface {
	// Normalize accepts any address form and returns the canonical raw form.
	Normalize(s string) (string, error)
	// Friendly renders a raw address for display / wallets. bounceable is
	// true for contracts and false for wallets.
	Friendly(raw string, bounceable bool) string
}

// Collection is an NFT collection as seen on chain.
type Collection struct {
	Address       string // raw form
	Owner         string
	NextItemIndex int64
	Name          string
	Description   string
	ImageURL      string
	Metadata      json.RawMessage
}

// Item is an NFT item as seen on chain.
type Item struct {
	Address    string
	Collection string
	Index      int64
	Owner      string
	Name       string
	ImageURL   string
	Metadata   json.RawMessage
	// Sale is set when the chain data provider recognises the owner as a sale
	// contract; informational only, the indexer relies on its own listings.
	Sale *SaleInfo
}

// SaleInfo is a recognised sale contract holding an item.
type SaleInfo struct {
	Address   string
	Owner     string
	PriceNano int64
}

// Message is an inbound or outbound message of a transaction.
type Message struct {
	Hash        string
	Source      string
	Destination string
	ValueNano   int64
	OpCode      uint32
	CreatedLT   uint64
	Bounced     bool
}

// Transaction is an account transaction.
type Transaction struct {
	Hash    string
	LT      uint64
	Account string
	Success bool
	Time    int64
	InMsg   *Message
	OutMsgs []Message
}

// RoyaltyParams mirrors TEP-66 royalty_params get-method output.
type RoyaltyParams struct {
	Numerator   int64
	Denominator int64
	Address     string
}

// SaleData is the state of a fixed-price sale contract.
type SaleData struct {
	IsComplete bool
	Market     string
	NFT        string
	Owner      string
	PriceNano  int64
}

// Client is the read-only chain access the indexer and services need.
// Every method must honour ctx cancellation and deadlines.
type Client interface {
	GetCollection(ctx context.Context, address string) (*Collection, error)
	GetCollectionItems(ctx context.Context, collection string, limit, offset int) ([]Item, error)
	GetItem(ctx context.Context, address string) (*Item, error)
	GetAccountItems(ctx context.Context, owner string, limit, offset int) ([]Item, error)
	// GetTransactions returns transactions of an account with lt > afterLT in
	// ascending order.
	GetTransactions(ctx context.Context, address string, afterLT uint64, limit int) ([]Transaction, error)
	// GetTransactionByMessageHash resolves the transaction that consumed the
	// given (external) message.
	GetTransactionByMessageHash(ctx context.Context, msgHash string) (*Transaction, error)
	GetPublicKey(ctx context.Context, address string) ([]byte, error)
	GetRoyaltyParams(ctx context.Context, collection string) (*RoyaltyParams, error)
	GetSaleData(ctx context.Context, sale string) (*SaleData, error)
}
