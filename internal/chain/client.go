// Package chain defines the chain-agnostic read interface the marketplace is
// built on. Concrete blockchains (TON today) implement Client in sub-packages;
// the domain and indexer only ever see these plain types, so adding another
// chain means adding another adapter, not touching business logic.
package chain

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrNotFound is returned when the account/entity does not exist on chain.
var ErrNotFound = errors.New("chain: not found")

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
