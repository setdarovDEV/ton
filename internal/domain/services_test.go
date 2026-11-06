package domain_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tonnet/internal/domain"
	"tonnet/internal/domain/domaintest"
)

const (
	seller = "0:5e"
	buyer  = "0:b0"
	nft    = "0:a0"
	col    = "0:c0"
)

type fakePayloads struct {
	hash string
	err  error
}

func (f fakePayloads) BuildListing(t domain.SaleTerms) (*domain.ListingIntent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.ListingIntent{SaleContract: "0:5a1e", Messages: []domain.Message{{Address: "market", AmountNano: 1}, {Address: t.NFTAddress, AmountNano: 2}}}, nil
}
func (f fakePayloads) BuildBuy(sale string, price int64, qid uint64) (domain.Message, error) {
	return domain.Message{Address: sale, AmountNano: price + 1, Payload: "buy"}, f.err
}
func (f fakePayloads) BuildCancel(sale string, qid uint64) (domain.Message, error) {
	return domain.Message{Address: sale, AmountNano: 1, Payload: "cancel"}, f.err
}
func (f fakePayloads) MessageHash(boc string) (string, error) {
	if boc == "bad" {
		return "", errors.New("bad boc")
	}
	if f.hash != "" {
		return f.hash, nil
	}
	return "hash-" + boc, nil
}

type fakeRoyalty struct {
	r   domain.RoyaltyParams
	err error
}

func (f fakeRoyalty) GetRoyaltyParams(context.Context, string) (*domain.RoyaltyParams, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &f.r, nil
}

func seed(t *testing.T) (*domaintest.MemRepo, *domaintest.FakeClock) {
	t.Helper()
	repo := domaintest.NewMemRepo()
	ctx := context.Background()
	require.NoError(t, repo.UpsertCollection(ctx, domain.Collection{Address: col, Name: "C"}))
	require.NoError(t, repo.UpsertItem(ctx, domain.NFTItem{Address: nft, CollectionAddress: col, Owner: seller, Name: "N"}))
	return repo, &domaintest.FakeClock{Unix: 1_700_000_000}
}

func TestCreateListingIntent(t *testing.T) {
	repo, clock := seed(t)
	ctx := context.Background()
	svc := domain.NewListingService(repo, repo, fakePayloads{}, fakeRoyalty{r: domain.RoyaltyParams{Numerator: 5, Denominator: 100, Address: "0:70"}}, domain.MarketplaceConfig{FeeBPS: 500}, clock)

	res, err := svc.CreateListingIntent(ctx, seller, nft, 10_000_000_000)
	require.NoError(t, err)
	assert.Equal(t, domain.ListingPending, res.Listing.Status)
	assert.Equal(t, "0:5a1e", res.Listing.SaleContract)
	assert.Len(t, res.Messages, 2)
	assert.Equal(t, clock.Now().Add(5*time.Minute), res.ValidUntil)
	stored, err := repo.GetOpenListingByNFT(ctx, nft)
	require.NoError(t, err)
	assert.Equal(t, res.Listing.ID, stored.ID)

	// Second listing for the same NFT conflicts.
	_, err = svc.CreateListingIntent(ctx, seller, nft, 1_000_000_000)
	assert.ErrorIs(t, err, domain.ErrConflict)

	_, err = svc.CreateListingIntent(ctx, buyer, nft, 1_000_000_000)
	assert.ErrorIs(t, err, domain.ErrForbidden)
	_, err = svc.CreateListingIntent(ctx, seller, nft, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = svc.CreateListingIntent(ctx, seller, "0:missing", 1)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateListingIntentFeeRules(t *testing.T) {
	repo, clock := seed(t)
	ctx := context.Background()
	// 100% royalty leaves nothing for the seller.
	svc := domain.NewListingService(repo, repo, fakePayloads{}, fakeRoyalty{r: domain.RoyaltyParams{Numerator: 1, Denominator: 1, Address: "0:70"}}, domain.MarketplaceConfig{}, clock)
	_, err := svc.CreateListingIntent(ctx, seller, nft, 1_000)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)

	svc = domain.NewListingService(repo, repo, fakePayloads{}, fakeRoyalty{err: errors.New("chain down")}, domain.MarketplaceConfig{}, clock)
	_, err = svc.CreateListingIntent(ctx, seller, nft, 1_000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chain down")

	svc = domain.NewListingService(repo, repo, fakePayloads{err: errors.New("encode")}, fakeRoyalty{}, domain.MarketplaceConfig{}, nil)
	_, err = svc.CreateListingIntent(ctx, seller, nft, 1_000)
	assert.Error(t, err)
}

func TestBuyAndCancelIntent(t *testing.T) {
	repo, clock := seed(t)
	ctx := context.Background()
	svc := domain.NewListingService(repo, repo, fakePayloads{}, fakeRoyalty{}, domain.MarketplaceConfig{}, clock)
	l, err := repo.CreateListing(ctx, domain.Listing{NFTAddress: nft, SaleContract: "0:5a1e", Seller: seller, PriceNano: 5, Status: domain.ListingPending})
	require.NoError(t, err)

	_, err = svc.BuyIntent(ctx, buyer, l.ID)
	assert.ErrorIs(t, err, domain.ErrConflict, "pending listing cannot be bought")
	_, err = svc.CancelIntent(ctx, seller, l.ID)
	assert.ErrorIs(t, err, domain.ErrConflict)

	require.NoError(t, repo.UpdateListingStatus(ctx, l.ID, domain.ListingActive, ""))
	res, err := svc.BuyIntent(ctx, buyer, l.ID)
	require.NoError(t, err)
	require.Len(t, res.Messages, 1)
	assert.Equal(t, "0:5a1e", res.Messages[0].Address)
	assert.EqualValues(t, 6, res.Messages[0].AmountNano)

	_, err = svc.BuyIntent(ctx, seller, l.ID)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = svc.BuyIntent(ctx, buyer, 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, err = svc.CancelIntent(ctx, buyer, l.ID)
	assert.ErrorIs(t, err, domain.ErrForbidden)
	res, err = svc.CancelIntent(ctx, seller, l.ID)
	require.NoError(t, err)
	assert.Equal(t, "cancel", res.Messages[0].Payload)
	_, err = svc.CancelIntent(ctx, seller, 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTxService(t *testing.T) {
	repo, clock := seed(t)
	ctx := context.Background()
	svc := domain.NewTxService(repo, repo, fakePayloads{}, clock, 0)
	l, _ := repo.CreateListing(ctx, domain.Listing{NFTAddress: nft, SaleContract: "0:5a1e", Seller: seller, PriceNano: 5, Status: domain.ListingActive})

	tx, err := svc.Register(ctx, domain.RegisterInput{Sender: buyer, BOC: "b1", Type: domain.TxBuy, ListingID: &l.ID})
	require.NoError(t, err)
	assert.Equal(t, "hash-b1", tx.Hash)
	assert.Equal(t, domain.TxPending, tx.Status)
	assert.Equal(t, nft, tx.NFTAddress, "nft resolved from listing")
	assert.Equal(t, "0:5a1e", tx.To)
	assert.EqualValues(t, 5, tx.AmountNano)
	require.NotNil(t, tx.ExpiresAt)
	assert.Equal(t, clock.Now().Add(10*time.Minute), *tx.ExpiresAt)

	again, err := svc.Register(ctx, domain.RegisterInput{Sender: buyer, BOC: "b1", Type: domain.TxBuy, ListingID: &l.ID})
	require.NoError(t, err)
	assert.Equal(t, tx.Hash, again.Hash, "idempotent")

	got, err := svc.Get(ctx, "hash-b1")
	require.NoError(t, err)
	assert.Equal(t, tx.Hash, got.Hash)
	_, err = svc.Get(ctx, "nope")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, err = svc.Register(ctx, domain.RegisterInput{Sender: buyer, BOC: "b2", Type: "weird"})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = svc.Register(ctx, domain.RegisterInput{Sender: buyer, Type: domain.TxBuy})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = svc.Register(ctx, domain.RegisterInput{Sender: buyer, BOC: "bad", Type: domain.TxBuy})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = svc.Register(ctx, domain.RegisterInput{Sender: buyer, BOC: "b3", Type: domain.TxTransfer})
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "needs nft or listing")
	missing := int64(42)
	_, err = svc.Register(ctx, domain.RegisterInput{Sender: buyer, BOC: "b4", Type: domain.TxBuy, ListingID: &missing})
	assert.ErrorIs(t, err, domain.ErrNotFound)

	tr, err := svc.Register(ctx, domain.RegisterInput{Sender: seller, BOC: "b5", Type: domain.TxTransfer, NFTAddress: nft})
	require.NoError(t, err)
	assert.Equal(t, nft, tr.NFTAddress)
}

func TestCatalogService(t *testing.T) {
	repo, _ := seed(t)
	ctx := context.Background()
	svc := domain.NewCatalogService(repo, repo, repo, repo)

	cs, err := svc.Collections(ctx)
	require.NoError(t, err)
	assert.Len(t, cs, 1)
	c, err := svc.Collection(ctx, col)
	require.NoError(t, err)
	assert.Equal(t, "C", c.Name)
	_, err = svc.Collection(ctx, "0:zz")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, _, err = svc.CollectionItems(ctx, "0:zz", domain.SortByIndex, domain.Page{Number: 1, Size: 10})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	_, _, err = svc.CollectionItems(ctx, col, domain.SortByIndex, domain.Page{Number: 1, Size: 10})
	require.NoError(t, err)

	d, err := svc.Item(ctx, nft)
	require.NoError(t, err)
	assert.Nil(t, d.Listing)
	_, err = svc.Item(ctx, "0:zz")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	l, _ := repo.CreateListing(ctx, domain.Listing{NFTAddress: nft, SaleContract: "0:5a1e", Seller: seller, PriceNano: 5, Status: domain.ListingActive})
	d, err = svc.Item(ctx, nft)
	require.NoError(t, err)
	require.NotNil(t, d.Listing)
	assert.Equal(t, l.ID, d.Listing.ID)

	iw, err := svc.Listing(ctx, l.ID)
	require.NoError(t, err)
	assert.Equal(t, nft, iw.Item.Address)
	_, err = svc.Listing(ctx, 77)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, _, err = svc.Listings(ctx, domain.ListingActive, domain.ListingSortNewest, domain.Page{Number: 1, Size: 10})
	require.NoError(t, err)

	items, err := svc.UserItems(ctx, seller)
	require.NoError(t, err)
	assert.NotNil(t, items)
}
