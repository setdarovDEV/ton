package indexer

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tonnet/internal/chain"
	"tonnet/internal/domain"
	"tonnet/internal/domain/domaintest"
)

const (
	col    = "0:c0"
	item0  = "0:a0"
	item1  = "0:a1"
	seller = "0:5e"
	sale   = "0:5a"
	buyer  = "0:b0"
)

func setup(t *testing.T) (*fakeChain, *domaintest.MemRepo, *Service, *domaintest.FakeClock) {
	t.Helper()
	ch := newFakeChain()
	ch.collections[col] = &chain.Collection{Address: col, Owner: seller, Name: "Demo", NextItemIndex: 2}
	ch.items[col] = []chain.Item{
		{Address: item0, Collection: col, Index: 0, Owner: seller, Name: "Zero"},
		{Address: item1, Collection: col, Index: 1, Owner: seller, Name: "One"},
	}
	ch.txs[col] = []chain.Transaction{
		{Hash: "mint0", LT: 10, Account: col, Success: true, Time: 1, InMsg: &chain.Message{Source: seller, OpCode: 1}, OutMsgs: []chain.Message{{Destination: item0, ValueNano: 5}}},
		{Hash: "mint1", LT: 20, Account: col, Success: true, Time: 2, InMsg: &chain.Message{Source: seller, OpCode: 1}, OutMsgs: []chain.Message{{Destination: item1, ValueNano: 5}}},
		{Hash: "royalty", LT: 30, Account: col, Success: true, Time: 3, OutMsgs: []chain.Message{{Destination: "0:ff"}}},
	}
	repo := domaintest.NewMemRepo()
	clock := &domaintest.FakeClock{Unix: 1_000}
	svc := New(ch, repo, Config{Collections: []string{col}, PageSize: 1, ItemScanInterval: time.Hour}, slog.Default(), clock)
	return ch, repo, svc, clock
}

func TestInitialSyncIsIdempotent(t *testing.T) {
	ch, repo, svc, _ := setup(t)
	ctx := context.Background()
	require.NoError(t, svc.Tick(ctx))

	c, err := repo.GetCollection(ctx, col)
	require.NoError(t, err)
	assert.Equal(t, "Demo", c.Name)
	assert.Len(t, repo.Items, 2)
	assert.Equal(t, seller, repo.Items[item0].Owner)
	assert.EqualValues(t, 30, repo.Cursors[col], "cursor advances past all collection txs")
	assert.Len(t, repo.Txs, 2, "two mints recorded, royalty report ignored")
	assert.Equal(t, domain.TxMint, repo.Txs["mint0"].Type)
	assert.Equal(t, domain.TxConfirmed, repo.Txs["mint0"].Status)

	// Re-running produces no changes and no extra tx fetches past the cursor.
	before := ch.calls["GetTransactions:"+col]
	require.NoError(t, svc.Tick(ctx))
	assert.Equal(t, before+1, ch.calls["GetTransactions:"+col], "nothing after the cursor: one short page")
	assert.Len(t, repo.Txs, 2)
	assert.Empty(t, repo.Events)
}

func TestListingLifecycle(t *testing.T) {
	ch, repo, svc, clock := setup(t)
	ctx := context.Background()
	require.NoError(t, svc.Tick(ctx))

	// Seller creates a listing intent and submits the tx.
	l, err := repo.CreateListing(ctx, domain.Listing{NFTAddress: item0, SaleContract: sale, Seller: seller, PriceNano: 10, Status: domain.ListingPending, CreatedAt: clock.Now()})
	require.NoError(t, err)
	exp := clock.Now().Add(10 * time.Minute)
	require.NoError(t, repo.UpsertTransaction(ctx, domain.Transaction{Hash: "listtx", Type: domain.TxList, NFTAddress: item0, ListingID: &l.ID, From: seller, Status: domain.TxPending, BOC: "x", ExpiresAt: &exp}))

	// Nothing happened on chain yet: still pending.
	require.NoError(t, svc.Tick(ctx))
	got, _ := repo.GetListing(ctx, l.ID)
	assert.Equal(t, domain.ListingPending, got.Status)
	assert.Equal(t, domain.TxPending, repo.Txs["listtx"].Status)

	// NFT arrives at the sale contract -> active, list tx confirmed.
	ch.setOwner(col, item0, sale)
	ch.txs[item0] = []chain.Transaction{{Hash: "xfer1", LT: 100, Success: true, Time: 5,
		InMsg: &chain.Message{Source: seller, OpCode: opTransfer, ValueNano: 7}, OutMsgs: []chain.Message{{OpCode: opOwnershipAssigned, Destination: sale}}}}
	require.NoError(t, svc.Tick(ctx))
	got, _ = repo.GetListing(ctx, l.ID)
	assert.Equal(t, domain.ListingActive, got.Status)
	assert.Equal(t, domain.TxConfirmed, repo.Txs["listtx"].Status)
	assert.Equal(t, sale, repo.Items[item0].Owner)
	require.Contains(t, repo.Txs, "xfer1")
	assert.Equal(t, domain.TxList, repo.Txs["xfer1"].Type, "on-chain transfer classified as list")
	assert.EqualValues(t, 100, repo.Cursors[item0])

	// Buyer pays: NFT goes to buyer -> sold, buy tx confirmed.
	require.NoError(t, repo.UpsertTransaction(ctx, domain.Transaction{Hash: "buytx", Type: domain.TxBuy, NFTAddress: item0, ListingID: &l.ID, From: buyer, Status: domain.TxPending, BOC: "y", ExpiresAt: &exp}))
	ch.setOwner(col, item0, buyer)
	ch.txs[item0] = append(ch.txs[item0], chain.Transaction{Hash: "xfer2", LT: 200, Success: true, Time: 6,
		InMsg: &chain.Message{Source: sale, OpCode: opTransfer}, OutMsgs: []chain.Message{{OpCode: opOwnershipAssigned, Destination: buyer}}})
	require.NoError(t, svc.Tick(ctx))
	got, _ = repo.GetListing(ctx, l.ID)
	assert.Equal(t, domain.ListingSold, got.Status)
	assert.Equal(t, buyer, got.Buyer)
	assert.Equal(t, domain.TxConfirmed, repo.Txs["buytx"].Status)
	assert.Equal(t, domain.TxBuy, repo.Txs["xfer2"].Type)
	assert.Equal(t, buyer, repo.Items[item0].Owner)

	// Replaying the same chain state changes nothing (idempotent).
	events := len(repo.Events)
	require.NoError(t, svc.Tick(ctx))
	assert.Len(t, repo.Events, events)
}

func TestCancelFlow(t *testing.T) {
	ch, repo, svc, clock := setup(t)
	ctx := context.Background()
	require.NoError(t, svc.Tick(ctx))
	l, _ := repo.CreateListing(ctx, domain.Listing{NFTAddress: item1, SaleContract: sale, Seller: seller, PriceNano: 10, Status: domain.ListingPending, CreatedAt: clock.Now()})
	ch.setOwner(col, item1, sale)
	require.NoError(t, svc.Tick(ctx))
	got, _ := repo.GetListing(ctx, l.ID)
	require.Equal(t, domain.ListingActive, got.Status)

	exp := clock.Now().Add(time.Hour)
	require.NoError(t, repo.UpsertTransaction(ctx, domain.Transaction{Hash: "canceltx", Type: domain.TxCancel, NFTAddress: item1, From: seller, Status: domain.TxPending, BOC: "z", ExpiresAt: &exp}))
	ch.setOwner(col, item1, seller)
	ch.txs[item1] = []chain.Transaction{{Hash: "xfer3", LT: 300, Success: true, InMsg: &chain.Message{Source: sale, OpCode: opTransfer}, OutMsgs: []chain.Message{{OpCode: opOwnershipAssigned, Destination: seller}}}}
	require.NoError(t, svc.Tick(ctx))
	got, _ = repo.GetListing(ctx, l.ID)
	assert.Equal(t, domain.ListingCancelled, got.Status)
	assert.Equal(t, domain.TxConfirmed, repo.Txs["canceltx"].Status)
	assert.Equal(t, domain.TxCancel, repo.Txs["xfer3"].Type)
}

func TestPlainTransferConfirmsTransferTx(t *testing.T) {
	ch, repo, svc, clock := setup(t)
	ctx := context.Background()
	require.NoError(t, svc.Tick(ctx))
	exp := clock.Now().Add(time.Hour)
	require.NoError(t, repo.UpsertTransaction(ctx, domain.Transaction{Hash: "t", Type: domain.TxTransfer, NFTAddress: item1, Status: domain.TxPending, BOC: "b", ExpiresAt: &exp}))
	ch.setOwner(col, item1, buyer)
	ch.txs[item1] = []chain.Transaction{{Hash: "xfer", LT: 1, Success: true, InMsg: &chain.Message{Source: seller, OpCode: opTransfer}, OutMsgs: []chain.Message{{OpCode: opOwnershipAssigned, Destination: buyer}}}}
	require.NoError(t, svc.Tick(ctx))
	assert.Equal(t, domain.TxConfirmed, repo.Txs["t"].Status)
	assert.Equal(t, domain.TxTransfer, repo.Txs["xfer"].Type)
}

func TestPendingExpiryAndFailures(t *testing.T) {
	ch, repo, svc, clock := setup(t)
	ctx := context.Background()
	require.NoError(t, svc.Tick(ctx))

	exp := clock.Now().Add(time.Minute)
	require.NoError(t, repo.UpsertTransaction(ctx, domain.Transaction{Hash: "late", Type: domain.TxBuy, NFTAddress: item0, Status: domain.TxPending, BOC: "b", ExpiresAt: &exp}))
	require.NoError(t, repo.UpsertTransaction(ctx, domain.Transaction{Hash: "rejected", Type: domain.TxBuy, NFTAddress: item0, Status: domain.TxPending, BOC: "b", ExpiresAt: &exp}))
	require.NoError(t, repo.UpsertTransaction(ctx, domain.Transaction{Hash: "seen", Type: domain.TxBuy, NFTAddress: item0, Status: domain.TxPending, BOC: "b", ExpiresAt: &exp}))
	ch.byMsg["rejected"] = &chain.Transaction{Hash: "r", LT: 5, Success: false}
	ch.byMsg["seen"] = &chain.Transaction{Hash: "s", LT: 77, Success: true}
	l, _ := repo.CreateListing(ctx, domain.Listing{NFTAddress: item1, SaleContract: sale, Seller: seller, PriceNano: 1, Status: domain.ListingPending, CreatedAt: clock.Now()})

	require.NoError(t, svc.Tick(ctx))
	assert.Equal(t, domain.TxPending, repo.Txs["late"].Status)
	assert.Equal(t, domain.TxFailed, repo.Txs["rejected"].Status, "rejected on chain fails immediately")
	assert.Equal(t, domain.TxPending, repo.Txs["seen"].Status)
	assert.EqualValues(t, 77, repo.Txs["seen"].LT, "lt is recorded once the message lands")

	clock.Unix += 2 * 60
	require.NoError(t, svc.Tick(ctx))
	assert.Equal(t, domain.TxFailed, repo.Txs["late"].Status, "expired without owner change")
	got, _ := repo.GetListing(ctx, l.ID)
	assert.Equal(t, domain.ListingPending, got.Status, "listing TTL not reached yet")

	clock.Unix += 60 * 60
	require.NoError(t, svc.Tick(ctx))
	got, _ = repo.GetListing(ctx, l.ID)
	assert.Equal(t, domain.ListingCancelled, got.Status, "pending listing expired")
}

func TestPendingListingCancelledWhenNFTMovesElsewhere(t *testing.T) {
	ch, repo, svc, clock := setup(t)
	ctx := context.Background()
	require.NoError(t, svc.Tick(ctx))
	l, _ := repo.CreateListing(ctx, domain.Listing{NFTAddress: item0, SaleContract: sale, Seller: seller, PriceNano: 1, Status: domain.ListingPending, CreatedAt: clock.Now()})
	ch.setOwner(col, item0, buyer)
	require.NoError(t, svc.Tick(ctx))
	got, _ := repo.GetListing(ctx, l.ID)
	assert.Equal(t, domain.ListingCancelled, got.Status)
}

func TestFullItemScanAndStoredCollections(t *testing.T) {
	ch, repo, svc, clock := setup(t)
	svc.cfg.Collections = nil
	require.NoError(t, repo.UpsertCollection(context.Background(), domain.Collection{Address: col}))
	svc.cfg.ItemScanInterval = time.Minute
	ctx := context.Background()
	require.NoError(t, svc.Tick(ctx)) // first tick is a full scan (lastFull zero)
	assert.Equal(t, 1, ch.calls["GetTransactions:"+item0])
	require.NoError(t, svc.Tick(ctx))
	assert.Equal(t, 1, ch.calls["GetTransactions:"+item0], "cold items are not rescanned every round")
	clock.Unix += 120
	require.NoError(t, svc.Tick(ctx))
	assert.Equal(t, 2, ch.calls["GetTransactions:"+item0])
}

func TestChainErrorsAreReportedNotFatal(t *testing.T) {
	ch, _, svc, _ := setup(t)
	ch.err = errors.New("tonapi down")
	err := svc.Tick(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tonapi down")
}

func TestRunStopsOnCancel(t *testing.T) {
	_, _, svc, _ := setup(t)
	svc.cfg.Interval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := svc.Run(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestConfigDefaults(t *testing.T) {
	var c Config
	c.defaults()
	assert.Equal(t, 4*time.Second, c.Interval)
	assert.Equal(t, time.Minute, c.ItemScanInterval)
	assert.Equal(t, 100, c.PageSize)
	s := New(newFakeChain(), domaintest.NewMemRepo(), Config{}, nil, nil)
	assert.NotNil(t, s.log)
	assert.NotNil(t, s.clock)
}
