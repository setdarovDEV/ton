// Package indexer keeps PostgreSQL in sync with the chain. The chain is the
// source of truth for ownership; the database is the source of truth for the
// UI. Every step is idempotent (upserts, monotonic cursors, state machine that
// ignores no-op transitions), so re-processing after a crash is safe.
package indexer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"tonnet/internal/chain"
	"tonnet/internal/domain"
)

// Repository is the storage the indexer needs; *postgres.Store satisfies it.
type Repository interface {
	domain.CollectionRepository
	domain.ItemRepository
	domain.ListingRepository
	domain.TransactionRepository
	domain.CursorRepository
}

// Config tunes the indexer loop.
type Config struct {
	// Collections are seed collection addresses (raw) tracked from start-up;
	// collections already in the database are tracked as well.
	Collections []string
	// Interval is the pause between two sync rounds.
	Interval time.Duration
	// ItemScanInterval is how often every item's transaction history is
	// scanned; items with open listings or pending transactions are scanned
	// every round regardless.
	ItemScanInterval time.Duration
	// PendingListingTTL bounds how long a pending listing waits for the NFT to
	// arrive at the sale contract before it is cancelled.
	PendingListingTTL time.Duration
	PageSize          int
}

func (c *Config) defaults() {
	if c.Interval <= 0 {
		c.Interval = 4 * time.Second
	}
	if c.ItemScanInterval <= 0 {
		c.ItemScanInterval = time.Minute
	}
	if c.PendingListingTTL <= 0 {
		c.PendingListingTTL = 15 * time.Minute
	}
	if c.PageSize <= 0 {
		c.PageSize = 100
	}
}

// Service runs the indexing loop.
type Service struct {
	chain    chain.Client
	repo     Repository
	cfg      Config
	log      *slog.Logger
	clock    domain.Clock
	lastFull time.Time
}

// New wires an indexer.
func New(ch chain.Client, repo Repository, cfg Config, log *slog.Logger, clock domain.Clock) *Service {
	cfg.defaults()
	if log == nil {
		log = slog.Default()
	}
	if clock == nil {
		clock = domain.RealClock{}
	}
	return &Service{chain: ch, repo: repo, cfg: cfg, log: log.With("component", "indexer"), clock: clock}
}

// Run loops until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	s.log.Info("indexer started", "interval", s.cfg.Interval, "seed_collections", len(s.cfg.Collections))
	for {
		start := s.clock.Now()
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("sync round failed", "err", err)
		}
		s.log.Debug("sync round done", "took", s.clock.Now().Sub(start))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.cfg.Interval):
		}
	}
}

// Tick runs one full sync round: every collection, then pending bookkeeping.
func (s *Service) Tick(ctx context.Context) error {
	collections, err := s.trackedCollections(ctx)
	if err != nil {
		return err
	}
	full := s.clock.Now().Sub(s.lastFull) >= s.cfg.ItemScanInterval
	var errs []error
	for _, addr := range collections {
		if err := s.syncCollection(ctx, addr, full); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			errs = append(errs, fmt.Errorf("collection %s: %w", addr, err))
		}
	}
	if full {
		s.lastFull = s.clock.Now()
	}
	if err := s.trackPending(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Service) trackedCollections(ctx context.Context) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	add := func(a string) {
		if _, ok := seen[a]; !ok && a != "" {
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	for _, c := range s.cfg.Collections {
		add(c)
	}
	stored, err := s.repo.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stored collections: %w", err)
	}
	for _, c := range stored {
		add(c.Address)
	}
	return out, nil
}

// syncCollection refreshes collection metadata, reconciles item owners against
// the chain (the owner diff drives listings and tx confirmation), and advances
// transaction cursors.
func (s *Service) syncCollection(ctx context.Context, addr string, fullItemScan bool) error {
	col, err := s.chain.GetCollection(ctx, addr)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	if err := s.repo.UpsertCollection(ctx, toDomainCollection(col)); err != nil {
		return err
	}

	known, err := s.repo.ItemOwners(ctx, col.Address)
	if err != nil {
		return err
	}
	changed, err := s.reconcileItems(ctx, col.Address, known)
	if err != nil {
		return err
	}
	if err := s.scanCollectionTxs(ctx, col.Address); err != nil {
		return err
	}
	hot, err := s.hotItems(ctx, col.Address)
	if err != nil {
		return err
	}
	for a := range changed {
		hot[a] = struct{}{}
	}
	if fullItemScan {
		for a := range known {
			hot[a] = struct{}{}
		}
	}
	for item := range hot {
		if err := s.scanItemTxs(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// reconcileItems pages through the collection on chain and applies every
// owner change to items, listings and pending transactions. It returns the set
// of items whose owner changed.
func (s *Service) reconcileItems(ctx context.Context, collection string, known map[string]string) (map[string]struct{}, error) {
	changed := map[string]struct{}{}
	for offset := 0; ; offset += s.cfg.PageSize {
		items, err := s.chain.GetCollectionItems(ctx, collection, s.cfg.PageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("get items: %w", err)
		}
		for _, it := range items {
			prev, existed := known[it.Address]
			if existed && prev == it.Owner {
				continue
			}
			if err := s.repo.UpsertItem(ctx, toDomainItem(it, collection)); err != nil {
				return nil, err
			}
			known[it.Address] = it.Owner
			if !existed && it.Owner == "" {
				continue
			}
			changed[it.Address] = struct{}{}
			if err := s.applyOwnerChange(ctx, it.Address, it.Owner); err != nil {
				return nil, err
			}
			s.log.Info("owner change", "nft", it.Address, "from", prev, "to", it.Owner)
		}
		if len(items) < s.cfg.PageSize {
			return changed, nil
		}
	}
}

// applyOwnerChange runs the listing state machine for the NFT and confirms the
// pending transaction that the change proves.
func (s *Service) applyOwnerChange(ctx context.Context, nft, newOwner string) error {
	listing, err := s.repo.GetOpenListingByNFT(ctx, nft)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			_, err = s.repo.ConfirmPendingForNFT(ctx, nft, domain.TxTransfer)
			return err
		}
		return err
	}
	updated, changed := listing.Apply(newOwner)
	if changed {
		if err := s.repo.UpdateListingStatus(ctx, listing.ID, updated.Status, updated.Buyer); err != nil {
			return err
		}
		s.log.Info("listing transition", "listing", listing.ID, "from", listing.Status, "to", updated.Status, "nft", nft)
	}
	if typ, ok := domain.TxTypeForOwnerChange(*listing, newOwner); ok {
		if _, err := s.repo.ConfirmPendingForNFT(ctx, nft, typ); err != nil {
			return err
		}
	}
	return nil
}

// scanCollectionTxs records mints (collection -> item deploy messages) using a
// per-collection logical-time cursor.
func (s *Service) scanCollectionTxs(ctx context.Context, collection string) error {
	cursor, err := s.repo.GetCursor(ctx, collection)
	if err != nil {
		return err
	}
	txs, err := s.fetchTxs(ctx, collection, cursor)
	if err != nil {
		return fmt.Errorf("get collection txs: %w", err)
	}
	for _, tx := range txs {
		if tx.Success {
			for _, out := range tx.OutMsgs {
				if out.Destination == "" || out.Destination == collection {
					continue
				}
				if _, err := s.repo.GetItem(ctx, out.Destination); err != nil {
					continue // not one of our items (e.g. royalty report)
				}
				from := ""
				if tx.InMsg != nil {
					from = tx.InMsg.Source
				}
				rec := domain.Transaction{Hash: tx.Hash, Type: domain.TxMint, NFTAddress: out.Destination, From: from, To: out.Destination,
					AmountNano: out.ValueNano, LT: tx.LT, Status: domain.TxConfirmed, SeenAt: time.Unix(tx.Time, 0)}
				if err := s.repo.UpsertTransaction(ctx, rec); err != nil {
					return err
				}
			}
		}
		if tx.LT > cursor {
			cursor = tx.LT
		}
	}
	return s.repo.SetCursor(ctx, collection, cursor)
}

// maxTxPagesPerRound bounds catch-up work per account per round so one busy
// account cannot starve the others; the cursor resumes next round.
const maxTxPagesPerRound = 10

// fetchTxs pages through an account's transactions after the cursor.
func (s *Service) fetchTxs(ctx context.Context, account string, after uint64) ([]chain.Transaction, error) {
	var all []chain.Transaction
	for page := 0; page < maxTxPagesPerRound; page++ {
		txs, err := s.chain.GetTransactions(ctx, account, after, s.cfg.PageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, txs...)
		if len(txs) < s.cfg.PageSize {
			break
		}
		after = txs[len(txs)-1].LT
	}
	return all, nil
}

// hotItems are items that need their history scanned every round: open
// listings and pending user transactions.
func (s *Service) hotItems(ctx context.Context, collection string) (map[string]struct{}, error) {
	hot := map[string]struct{}{}
	open, err := s.repo.ListOpenListings(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := s.repo.ListPendingTransactions(ctx)
	if err != nil {
		return nil, err
	}
	owners, err := s.repo.ItemOwners(ctx, collection)
	if err != nil {
		return nil, err
	}
	for _, l := range open {
		if _, ok := owners[l.NFTAddress]; ok {
			hot[l.NFTAddress] = struct{}{}
		}
	}
	for _, t := range pending {
		if _, ok := owners[t.NFTAddress]; ok {
			hot[t.NFTAddress] = struct{}{}
		}
	}
	return hot, nil
}

// TEP-62 op codes seen on item accounts.
const (
	opTransfer          uint32 = 0x5fcc3d14
	opOwnershipAssigned uint32 = 0x05138d91
)

// scanItemTxs records on-chain transfer transactions of one item, classified
// against the item's listings, with a per-item cursor.
func (s *Service) scanItemTxs(ctx context.Context, item string) error {
	cursor, err := s.repo.GetCursor(ctx, item)
	if err != nil {
		return err
	}
	txs, err := s.fetchTxs(ctx, item, cursor)
	if err != nil {
		if errors.Is(err, chain.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("get item txs: %w", err)
	}
	for _, tx := range txs {
		if tx.LT > cursor {
			cursor = tx.LT
		}
		if !tx.Success || tx.InMsg == nil || tx.InMsg.OpCode != opTransfer {
			continue
		}
		to := ""
		for _, out := range tx.OutMsgs {
			if out.OpCode == opOwnershipAssigned {
				to = out.Destination
			}
		}
		typ := s.classifyTransfer(ctx, item, tx.InMsg.Source, to)
		rec := domain.Transaction{Hash: tx.Hash, Type: typ, NFTAddress: item, From: tx.InMsg.Source, To: to,
			AmountNano: tx.InMsg.ValueNano, LT: tx.LT, Status: domain.TxConfirmed, SeenAt: time.Unix(tx.Time, 0)}
		if err := s.repo.UpsertTransaction(ctx, rec); err != nil {
			return err
		}
	}
	return s.repo.SetCursor(ctx, item, cursor)
}

// classifyTransfer labels an on-chain transfer by who sent it and where the
// NFT went, using the listing that involves either party.
func (s *Service) classifyTransfer(ctx context.Context, item, from, to string) domain.TxType {
	for _, candidate := range []string{to, from} {
		if candidate == "" {
			continue
		}
		l, err := s.repo.GetListingBySaleContract(ctx, candidate)
		if err != nil {
			continue
		}
		switch {
		case candidate == to && from == l.Seller:
			return domain.TxList
		case candidate == from && to == l.Seller:
			return domain.TxCancel
		case candidate == from:
			return domain.TxBuy
		}
	}
	_ = item
	return domain.TxTransfer
}

// trackPending fails user transactions that never produced the expected owner
// change in time (or were rejected on chain) and expires stale pending listings.
func (s *Service) trackPending(ctx context.Context) error {
	now := s.clock.Now()
	pending, err := s.repo.ListPendingTransactions(ctx)
	if err != nil {
		return err
	}
	for _, tx := range pending {
		if tx.BOC == "" {
			continue // chain-observed transaction, not user-submitted
		}
		onChain, err := s.chain.GetTransactionByMessageHash(ctx, tx.Hash)
		switch {
		case err == nil && !onChain.Success:
			s.log.Info("tx failed on chain", "hash", tx.Hash)
			if err := s.repo.UpdateTransactionStatus(ctx, tx.Hash, domain.TxFailed, onChain.LT); err != nil {
				return err
			}
			continue
		case err == nil && onChain.LT > tx.LT:
			if err := s.repo.UpsertTransaction(ctx, domain.Transaction{Hash: tx.Hash, Type: tx.Type, NFTAddress: tx.NFTAddress, LT: onChain.LT, Status: domain.TxPending}); err != nil {
				return err
			}
		case err != nil && !errors.Is(err, chain.ErrNotFound):
			s.log.Warn("lookup pending tx", "hash", tx.Hash, "err", err)
		}
		if tx.ExpiresAt != nil && now.After(*tx.ExpiresAt) {
			s.log.Info("tx expired without effect", "hash", tx.Hash, "type", tx.Type)
			if err := s.repo.UpdateTransactionStatus(ctx, tx.Hash, domain.TxFailed, 0); err != nil {
				return err
			}
		}
	}

	open, err := s.repo.ListOpenListings(ctx)
	if err != nil {
		return err
	}
	for _, l := range open {
		if l.Status == domain.ListingPending && now.Sub(l.CreatedAt) > s.cfg.PendingListingTTL {
			s.log.Info("pending listing expired", "listing", l.ID)
			if err := s.repo.UpdateListingStatus(ctx, l.ID, domain.ListingCancelled, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func toDomainCollection(c *chain.Collection) domain.Collection {
	return domain.Collection{Address: c.Address, Owner: c.Owner, Name: c.Name, Description: c.Description, ImageURL: c.ImageURL, Metadata: c.Metadata, NextItemIndex: c.NextItemIndex}
}

func toDomainItem(it chain.Item, collection string) domain.NFTItem {
	col := it.Collection
	if col == "" {
		col = collection
	}
	return domain.NFTItem{Address: it.Address, CollectionAddress: col, Index: it.Index, Owner: it.Owner, Name: it.Name, ImageURL: it.ImageURL, Metadata: it.Metadata}
}
