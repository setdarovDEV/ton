//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"tonnet/internal/domain"
)

const (
	col    = "0:c0"
	item0  = "0:a0"
	item1  = "0:a1"
	seller = "0:5e"
	sale   = "0:5a"
	buyer  = "0:b0"
)

func startPostgres(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("tonnet"), tcpostgres.WithUsername("tonnet"), tcpostgres.WithPassword("tonnet"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, Migrate(dsn))
	require.NoError(t, Migrate(dsn), "migrations are idempotent")
	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return NewStore(pool)
}

func TestIntegrationStore(t *testing.T) {
	s := startPostgres(t)
	ctx := context.Background()

	// Collections & items
	require.NoError(t, s.UpsertCollection(ctx, domain.Collection{Address: col, Owner: seller, Name: "Demo", Metadata: json.RawMessage(`{"name":"Demo"}`), NextItemIndex: 2}))
	require.NoError(t, s.UpsertCollection(ctx, domain.Collection{Address: col, Owner: seller, Name: "Demo v2", NextItemIndex: 3}))
	c, err := s.GetCollection(ctx, col)
	require.NoError(t, err)
	assert.Equal(t, "Demo v2", c.Name)
	assert.EqualValues(t, 3, c.NextItemIndex)
	assert.JSONEq(t, "{}", string(c.Metadata))
	cs, err := s.ListCollections(ctx)
	require.NoError(t, err)
	assert.Len(t, cs, 1)
	_, err = s.GetCollection(ctx, "0:zz")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	require.NoError(t, s.UpsertItem(ctx, domain.NFTItem{Address: item0, CollectionAddress: col, Index: 0, Owner: seller, Name: "Zero", Metadata: json.RawMessage(`{"a":1}`)}))
	require.NoError(t, s.UpsertItem(ctx, domain.NFTItem{Address: item1, CollectionAddress: col, Index: 1, Owner: buyer, Name: "One"}))
	it, err := s.GetItem(ctx, item0)
	require.NoError(t, err)
	assert.Equal(t, "Zero", it.Name)
	owners, err := s.ItemOwners(ctx, col)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{item0: seller, item1: buyer}, owners)

	page, total, err := s.ListItemsByCollection(ctx, col, domain.SortByIndex, domain.Page{Number: 1, Size: 1})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, page, 1)
	assert.Equal(t, item0, page[0].Item.Address)
	assert.Nil(t, page[0].Listing)

	// Listings
	l, err := s.CreateListing(ctx, domain.Listing{NFTAddress: item0, SaleContract: sale, Seller: seller, PriceNano: 2_000_000_000, Status: domain.ListingPending})
	require.NoError(t, err)
	assert.NotZero(t, l.ID)
	_, err = s.CreateListing(ctx, domain.Listing{NFTAddress: item1, SaleContract: sale, Seller: seller, PriceNano: 1, Status: domain.ListingPending})
	assert.ErrorIs(t, err, domain.ErrConflict, "sale contract is unique")

	open, err := s.GetOpenListingByNFT(ctx, item0)
	require.NoError(t, err)
	assert.Equal(t, l.ID, open.ID)
	bySale, err := s.GetListingBySaleContract(ctx, sale)
	require.NoError(t, err)
	assert.Equal(t, l.ID, bySale.ID)
	_, err = s.GetListing(ctx, 9999)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Price sort puts active listings first.
	require.NoError(t, s.UpdateListingStatus(ctx, l.ID, domain.ListingActive, ""))
	page, _, err = s.ListItemsByCollection(ctx, col, domain.SortByPriceAsc, domain.Page{Number: 1, Size: 10})
	require.NoError(t, err)
	require.NotNil(t, page[0].Listing)
	assert.Equal(t, domain.ListingActive, page[0].Listing.Status)
	assert.EqualValues(t, 2_000_000_000, page[0].Listing.PriceNano)

	active, total, err := s.ListListings(ctx, domain.ListingActive, domain.ListingSortPriceAsc, domain.Page{Number: 1, Size: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, item0, active[0].Item.Address)
	mine, err := s.ListOpenListingsBySeller(ctx, seller)
	require.NoError(t, err)
	assert.Len(t, mine, 1)
	all, err := s.ListOpenListings(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	// Item moved to sale contract: My NFTs still finds it through the listing.
	require.NoError(t, s.UpsertItem(ctx, domain.NFTItem{Address: item0, CollectionAddress: col, Index: 0, Owner: sale, Name: "Zero"}))
	owned, err := s.ListItemsByOwner(ctx, seller)
	require.NoError(t, err)
	assert.Empty(t, owned)
	listed, err := s.ListItemsBySaleContracts(ctx, []string{sale})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, item0, listed[0].Item.Address)
	none, err := s.ListItemsBySaleContracts(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	// Transactions
	exp := time.Now().Add(time.Hour)
	tx := domain.Transaction{Hash: "h1", Type: domain.TxBuy, NFTAddress: item0, ListingID: &l.ID, From: buyer, To: sale, AmountNano: 5, Status: domain.TxPending, BOC: "boc", ExpiresAt: &exp}
	require.NoError(t, s.UpsertTransaction(ctx, tx))
	require.NoError(t, s.UpsertTransaction(ctx, domain.Transaction{Hash: "h1", Type: domain.TxBuy, NFTAddress: item0, LT: 77, Status: domain.TxPending}))
	got, err := s.GetTransaction(ctx, "h1")
	require.NoError(t, err)
	assert.EqualValues(t, 77, got.LT)
	assert.Equal(t, "boc", got.BOC, "user fields preserved on conflict")
	assert.Equal(t, buyer, got.From)
	require.NotNil(t, got.ListingID)

	pending, err := s.ListPendingTransactions(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	hashes, err := s.ConfirmPendingForNFT(ctx, item0, domain.TxBuy)
	require.NoError(t, err)
	assert.Equal(t, []string{"h1"}, hashes)
	got, _ = s.GetTransaction(ctx, "h1")
	assert.Equal(t, domain.TxConfirmed, got.Status)
	// Confirmed rows are not downgraded by later upserts.
	require.NoError(t, s.UpsertTransaction(ctx, domain.Transaction{Hash: "h1", Type: domain.TxBuy, NFTAddress: item0, Status: domain.TxPending}))
	got, _ = s.GetTransaction(ctx, "h1")
	assert.Equal(t, domain.TxConfirmed, got.Status)

	require.NoError(t, s.UpsertTransaction(ctx, domain.Transaction{Hash: "h2", Type: domain.TxMint, NFTAddress: item0, LT: 10, Status: domain.TxConfirmed}))
	hist, err := s.ListTransactionsByNFT(ctx, item0, 10)
	require.NoError(t, err)
	require.Len(t, hist, 2)
	assert.Equal(t, "h1", hist[0].Hash, "newest lt first")
	require.NoError(t, s.UpdateTransactionStatus(ctx, "h2", domain.TxFailed, 11))
	assert.ErrorIs(t, s.UpdateTransactionStatus(ctx, "nope", domain.TxFailed, 0), domain.ErrNotFound)

	// Sold listing with buyer
	require.NoError(t, s.UpdateListingStatus(ctx, l.ID, domain.ListingSold, buyer))
	sold, _ := s.GetListing(ctx, l.ID)
	assert.Equal(t, buyer, sold.Buyer)
	_, err = s.GetOpenListingByNFT(ctx, item0)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.ErrorIs(t, s.UpdateListingStatus(ctx, 9999, domain.ListingSold, ""), domain.ErrNotFound)

	// Cursors are monotonic
	lt, err := s.GetCursor(ctx, col)
	require.NoError(t, err)
	assert.Zero(t, lt)
	require.NoError(t, s.SetCursor(ctx, col, 50))
	require.NoError(t, s.SetCursor(ctx, col, 40))
	lt, _ = s.GetCursor(ctx, col)
	assert.EqualValues(t, 50, lt)
}

func TestIntegrationEventsRoundTrip(t *testing.T) {
	s := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	events := make(chan domain.Event, 4)
	go NewListener(s.Pool(), discardLogger()).Run(ctx, func(e domain.Event) { events <- e })
	time.Sleep(500 * time.Millisecond) // let LISTEN register

	require.NoError(t, s.UpsertCollection(ctx, domain.Collection{Address: col}))
	require.NoError(t, s.UpsertItem(ctx, domain.NFTItem{Address: item0, CollectionAddress: col}))
	l, err := s.CreateListing(ctx, domain.Listing{NFTAddress: item0, SaleContract: sale, Seller: seller, PriceNano: 1, Status: domain.ListingPending})
	require.NoError(t, err)
	require.NoError(t, s.UpdateListingStatus(ctx, l.ID, domain.ListingActive, ""))
	require.NoError(t, s.Publish(ctx, domain.Event{Type: "custom", Status: "x", At: time.Now()}))

	select {
	case e := <-events:
		assert.Equal(t, "listing.updated", e.Type)
		assert.Equal(t, l.ID, e.ListingID)
		assert.Equal(t, "active", e.Status)
	case <-ctx.Done():
		t.Fatal("no event received")
	}
	select {
	case e := <-events:
		assert.Equal(t, "custom", e.Type)
	case <-ctx.Done():
		t.Fatal("no custom event received")
	}
}
