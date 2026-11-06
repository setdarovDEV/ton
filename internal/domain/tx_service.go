package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TxService registers wallet-submitted transactions as pending so the indexer
// can confirm them once the on-chain effect (ownership change) is observed.
type TxService struct {
	txs      TransactionRepository
	listings ListingRepository
	payloads PayloadBuilder
	clock    Clock
	ttl      time.Duration
}

// NewTxService wires the service. ttl bounds how long a pending transaction
// may wait for confirmation before the indexer marks it failed.
func NewTxService(txs TransactionRepository, listings ListingRepository, payloads PayloadBuilder, clock Clock, ttl time.Duration) *TxService {
	if clock == nil {
		clock = RealClock{}
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &TxService{txs: txs, listings: listings, payloads: payloads, clock: clock, ttl: ttl}
}

// RegisterInput is the user-submitted transaction description.
type RegisterInput struct {
	Sender     string
	BOC        string
	Type       TxType
	NFTAddress string
	ListingID  *int64
}

// Register stores the transaction as pending and returns it. Re-registering
// the same BOC is idempotent.
func (s *TxService) Register(ctx context.Context, in RegisterInput) (*Transaction, error) {
	switch in.Type {
	case TxList, TxBuy, TxCancel, TxTransfer:
	default:
		return nil, fmt.Errorf("%w: unsupported tx type %q", ErrInvalidInput, in.Type)
	}
	if in.BOC == "" {
		return nil, fmt.Errorf("%w: boc is required", ErrInvalidInput)
	}
	hash, err := s.payloads.MessageHash(in.BOC)
	if err != nil {
		return nil, fmt.Errorf("%w: bad boc: %w", ErrInvalidInput, err)
	}
	if existing, err := s.txs.GetTransaction(ctx, hash); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("lookup tx: %w", err)
	}

	var to string
	var amount int64
	if in.ListingID != nil {
		l, err := s.listings.GetListing(ctx, *in.ListingID)
		if err != nil {
			return nil, fmt.Errorf("load listing %d: %w", *in.ListingID, err)
		}
		if in.NFTAddress == "" {
			in.NFTAddress = l.NFTAddress
		}
		to = l.SaleContract
		if in.Type == TxBuy {
			amount = l.PriceNano
		}
	}
	if in.NFTAddress == "" {
		return nil, fmt.Errorf("%w: nft_address or listing_id is required", ErrInvalidInput)
	}
	now := s.clock.Now()
	exp := now.Add(s.ttl)
	tx := Transaction{
		Hash:       hash,
		Type:       in.Type,
		NFTAddress: in.NFTAddress,
		ListingID:  in.ListingID,
		From:       in.Sender,
		To:         to,
		AmountNano: amount,
		Status:     TxPending,
		BOC:        in.BOC,
		ExpiresAt:  &exp,
		SeenAt:     now,
		UpdatedAt:  now,
	}
	if err := s.txs.UpsertTransaction(ctx, tx); err != nil {
		return nil, fmt.Errorf("store tx: %w", err)
	}
	return &tx, nil
}

// Get returns a tracked transaction by hash.
func (s *TxService) Get(ctx context.Context, hash string) (*Transaction, error) {
	tx, err := s.txs.GetTransaction(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("load tx: %w", err)
	}
	return tx, nil
}
