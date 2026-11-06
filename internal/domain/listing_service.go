package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// MarketplaceConfig holds the fee policy applied to new listings.
type MarketplaceConfig struct {
	FeeBPS int64 // marketplace fee in basis points (1/10000)
}

// ListingService prepares listing / buy / cancel intents. It never talks to the
// chain for writes: the user's wallet signs and sends, the indexer confirms.
type ListingService struct {
	items    ItemRepository
	listings ListingRepository
	payloads PayloadBuilder
	royalty  RoyaltyReader
	cfg      MarketplaceConfig
	clock    Clock
}

// NewListingService wires the service.
func NewListingService(items ItemRepository, listings ListingRepository, payloads PayloadBuilder, royalty RoyaltyReader, cfg MarketplaceConfig, clock Clock) *ListingService {
	if clock == nil {
		clock = RealClock{}
	}
	return &ListingService{items: items, listings: listings, payloads: payloads, royalty: royalty, cfg: cfg, clock: clock}
}

// IntentResult is a prepared set of wallet messages plus the listing it belongs to.
type IntentResult struct {
	Listing    Listing
	Messages   []Message
	ValidUntil time.Time
}

const intentTTL = 5 * time.Minute

// CreateListingIntent validates ownership, computes fees, derives the sale
// contract address and records a pending listing.
func (s *ListingService) CreateListingIntent(ctx context.Context, seller, nftAddress string, priceNano int64) (*IntentResult, error) {
	if priceNano <= 0 {
		return nil, fmt.Errorf("%w: price must be positive", ErrInvalidInput)
	}
	item, err := s.items.GetItem(ctx, nftAddress)
	if err != nil {
		return nil, fmt.Errorf("load item: %w", err)
	}
	if item.Owner != seller {
		return nil, fmt.Errorf("%w: nft is not owned by %s", ErrForbidden, seller)
	}
	if open, err := s.listings.GetOpenListingByNFT(ctx, nftAddress); err == nil && open != nil {
		return nil, fmt.Errorf("%w: nft already has listing %d (%s)", ErrConflict, open.ID, open.Status)
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("check open listing: %w", err)
	}

	royalty, err := s.royalty.GetRoyaltyParams(ctx, item.CollectionAddress)
	if err != nil {
		return nil, fmt.Errorf("read royalty params: %w", err)
	}
	fee := priceNano * s.cfg.FeeBPS / 10_000
	royaltyNano := int64(0)
	if royalty.Denominator > 0 {
		royaltyNano = priceNano * royalty.Numerator / royalty.Denominator
	}
	if priceNano-fee-royaltyNano < 1 {
		return nil, fmt.Errorf("%w: price does not cover fees and royalty", ErrInvalidInput)
	}

	terms := SaleTerms{
		NFTAddress:         nftAddress,
		Seller:             seller,
		PriceNano:          priceNano,
		MarketplaceFeeNano: fee,
		Royalty:            *royalty,
		QueryID:            uint64(s.clock.Now().Unix()), //nolint:gosec // unix seconds are positive
	}
	// Royalty amount travels in the fee cell; keep Royalty.Numerator as the
	// absolute nano amount for the builder (TEP-66 share already applied).
	terms.Royalty.Numerator = royaltyNano
	terms.Royalty.Denominator = 1

	intent, err := s.payloads.BuildListing(terms)
	if err != nil {
		return nil, fmt.Errorf("build listing payload: %w", err)
	}
	created, err := s.listings.CreateListing(ctx, Listing{
		NFTAddress:   nftAddress,
		SaleContract: intent.SaleContract,
		Seller:       seller,
		PriceNano:    priceNano,
		Status:       ListingPending,
	})
	if err != nil {
		return nil, fmt.Errorf("create pending listing: %w", err)
	}
	return &IntentResult{Listing: *created, Messages: intent.Messages, ValidUntil: s.clock.Now().Add(intentTTL)}, nil
}

// BuyIntent prepares the purchase message for an active listing.
func (s *ListingService) BuyIntent(ctx context.Context, buyer string, listingID int64) (*IntentResult, error) {
	l, err := s.listings.GetListing(ctx, listingID)
	if err != nil {
		return nil, fmt.Errorf("load listing: %w", err)
	}
	if l.Status != ListingActive {
		return nil, fmt.Errorf("%w: listing is %s", ErrConflict, l.Status)
	}
	if l.Seller == buyer {
		return nil, fmt.Errorf("%w: seller cannot buy own listing", ErrInvalidInput)
	}
	msg, err := s.payloads.BuildBuy(l.SaleContract, l.PriceNano, uint64(l.ID)) //nolint:gosec // ids are positive
	if err != nil {
		return nil, fmt.Errorf("build buy payload: %w", err)
	}
	return &IntentResult{Listing: *l, Messages: []Message{msg}, ValidUntil: s.clock.Now().Add(intentTTL)}, nil
}

// CancelIntent prepares the cancel message; only the seller may cancel.
func (s *ListingService) CancelIntent(ctx context.Context, seller string, listingID int64) (*IntentResult, error) {
	l, err := s.listings.GetListing(ctx, listingID)
	if err != nil {
		return nil, fmt.Errorf("load listing: %w", err)
	}
	if l.Seller != seller {
		return nil, fmt.Errorf("%w: only the seller can cancel", ErrForbidden)
	}
	if l.Status != ListingActive {
		return nil, fmt.Errorf("%w: listing is %s", ErrConflict, l.Status)
	}
	msg, err := s.payloads.BuildCancel(l.SaleContract, uint64(l.ID)) //nolint:gosec // ids are positive
	if err != nil {
		return nil, fmt.Errorf("build cancel payload: %w", err)
	}
	return &IntentResult{Listing: *l, Messages: []Message{msg}, ValidUntil: s.clock.Now().Add(intentTTL)}, nil
}
