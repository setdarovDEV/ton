// Package domaintest provides in-memory test doubles for the domain ports.
package domaintest

import (
	"context"
	"sort"
	"sync"
	"time"

	"tonnet/internal/domain"
)

// FakeClock is a settable domain.Clock.
type FakeClock struct{ Unix int64 }

// Now returns the fake time.
func (c *FakeClock) Now() time.Time { return time.Unix(c.Unix, 0) }

// Advance moves the clock forward.
func (c *FakeClock) Advance(d time.Duration) { c.Unix += int64(d / time.Second) }

// MemRepo is an in-memory implementation of every domain repository port,
// shared by unit tests across packages. Fields are exported for assertions.
type MemRepo struct {
	mu          sync.Mutex
	Collections map[string]domain.Collection
	Items       map[string]domain.NFTItem
	Listings    map[int64]*domain.Listing
	Txs         map[string]domain.Transaction
	Cursors     map[string]uint64
	NextID      int64
	Events      []string
}

// NewMemRepo creates an empty repository.
func NewMemRepo() *MemRepo {
	return &MemRepo{Collections: map[string]domain.Collection{}, Items: map[string]domain.NFTItem{}, Listings: map[int64]*domain.Listing{}, Txs: map[string]domain.Transaction{}, Cursors: map[string]uint64{}}
}

func (m *MemRepo) UpsertCollection(_ context.Context, c domain.Collection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Collections[c.Address] = c
	return nil
}
func (m *MemRepo) GetCollection(_ context.Context, a string) (*domain.Collection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.Collections[a]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &c, nil
}
func (m *MemRepo) AllCollections(context.Context) ([]domain.Collection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sortedCollections(), nil
}
func (m *MemRepo) ListCollections(_ context.Context, p domain.Page) ([]domain.Collection, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.sortedCollections()
	lo, hi := pageBounds(p, len(all))
	return all[lo:hi], len(all), nil
}
func (m *MemRepo) sortedCollections() []domain.Collection {
	out := make([]domain.Collection, 0, len(m.Collections))
	for _, c := range m.Collections {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out
}

// pageBounds clamps a page onto a slice of length n.
func pageBounds(p domain.Page, n int) (lo, hi int) {
	lo = p.Offset()
	if lo >= n {
		return 0, 0
	}
	hi = lo + p.Size
	if hi > n {
		hi = n
	}
	return lo, hi
}
func (m *MemRepo) UpsertItem(_ context.Context, it domain.NFTItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Items[it.Address] = it
	return nil
}
func (m *MemRepo) GetItem(_ context.Context, a string) (*domain.NFTItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.Items[a]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &it, nil
}
func (m *MemRepo) openListingFor(nft string) *domain.Listing {
	for _, l := range m.Listings {
		if l.NFTAddress == nft && !l.Status.IsTerminal() {
			cp := *l
			return &cp
		}
	}
	return nil
}

func (m *MemRepo) withListings(match func(domain.NFTItem) bool) []domain.ItemWithListing {
	out := []domain.ItemWithListing{}
	for _, it := range m.Items {
		if match(it) {
			out = append(out, domain.ItemWithListing{Item: it, Listing: m.openListingFor(it.Address)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Item.Index < out[j].Item.Index })
	return out
}

func (m *MemRepo) ListItemsByCollection(_ context.Context, c string, _ domain.ItemSort, p domain.Page) ([]domain.ItemWithListing, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.withListings(func(it domain.NFTItem) bool { return it.CollectionAddress == c })
	lo, hi := pageBounds(p, len(all))
	return all[lo:hi], len(all), nil
}
func (m *MemRepo) ListItemsByWallet(_ context.Context, wallet string, p domain.Page) ([]domain.ItemWithListing, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sales := map[string]struct{}{}
	for _, l := range m.Listings {
		if l.Seller == wallet && !l.Status.IsTerminal() {
			sales[l.SaleContract] = struct{}{}
		}
	}
	all := m.withListings(func(it domain.NFTItem) bool {
		_, listed := sales[it.Owner]
		return it.Owner == wallet || listed
	})
	lo, hi := pageBounds(p, len(all))
	return all[lo:hi], len(all), nil
}
func (m *MemRepo) ItemOwners(_ context.Context, c string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for _, it := range m.Items {
		if it.CollectionAddress == c {
			out[it.Address] = it.Owner
		}
	}
	return out, nil
}
func (m *MemRepo) CreateListing(_ context.Context, l domain.Listing) (*domain.Listing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NextID++
	l.ID = m.NextID
	m.Listings[l.ID] = &l
	return &l, nil
}
func (m *MemRepo) GetListing(_ context.Context, id int64) (*domain.Listing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.Listings[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *l
	return &cp, nil
}
func (m *MemRepo) GetListingBySaleContract(_ context.Context, s string) (*domain.Listing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.Listings {
		if l.SaleContract == s {
			cp := *l
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *MemRepo) GetOpenListingByNFT(_ context.Context, nft string) (*domain.Listing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.Listings {
		if l.NFTAddress == nft && !l.Status.IsTerminal() {
			cp := *l
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *MemRepo) ListListings(_ context.Context, st domain.ListingStatus, _ domain.ListingSort, _ domain.Page) ([]domain.ItemWithListing, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.ItemWithListing{}
	for _, l := range m.Listings {
		if l.Status == st {
			cp := *l
			out = append(out, domain.ItemWithListing{Item: m.Items[l.NFTAddress], Listing: &cp})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Listing.ID < out[j].Listing.ID })
	return out, len(out), nil
}
func (m *MemRepo) ListOpenListingsBySeller(_ context.Context, seller string) ([]domain.Listing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.Listing{}
	for _, l := range m.Listings {
		if l.Seller == seller && !l.Status.IsTerminal() {
			out = append(out, *l)
		}
	}
	return out, nil
}
func (m *MemRepo) ListOpenListings(context.Context) ([]domain.Listing, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Listing
	for _, l := range m.Listings {
		if !l.Status.IsTerminal() {
			out = append(out, *l)
		}
	}
	return out, nil
}
func (m *MemRepo) UpdateListingStatus(_ context.Context, id int64, st domain.ListingStatus, buyer string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.Listings[id]
	if !ok {
		return domain.ErrNotFound
	}
	l.Status = st
	if buyer != "" {
		l.Buyer = buyer
	}
	m.Events = append(m.Events, string(domain.EventListingUpdated)+":"+string(st))
	return nil
}
func (m *MemRepo) UpsertTransaction(_ context.Context, t domain.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.Txs[t.Hash]; ok {
		if t.LT > old.LT {
			old.LT = t.LT
		}
		if old.Status == domain.TxPending {
			old.Status = t.Status
		}
		m.Txs[t.Hash] = old
		return nil
	}
	m.Txs[t.Hash] = t
	return nil
}
func (m *MemRepo) GetTransaction(_ context.Context, h string) (*domain.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.Txs[h]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &t, nil
}
func (m *MemRepo) ListTransactionsByNFT(_ context.Context, nft string, limit int) ([]domain.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []domain.Transaction{}
	for _, t := range m.Txs {
		if t.NFTAddress == nft && len(out) < limit {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LT > out[j].LT })
	return out, nil
}
func (m *MemRepo) ListPendingTransactions(context.Context) ([]domain.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Transaction
	for _, t := range m.Txs {
		if t.Status == domain.TxPending {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	return out, nil
}
func (m *MemRepo) UpdateTransactionStatus(_ context.Context, h string, st domain.TxStatus, lt uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.Txs[h]
	if !ok {
		return domain.ErrNotFound
	}
	t.Status = st
	if lt > t.LT {
		t.LT = lt
	}
	m.Txs[h] = t
	m.Events = append(m.Events, string(domain.EventTxUpdated)+":"+string(st))
	return nil
}
func (m *MemRepo) ConfirmPendingForNFT(_ context.Context, nft string, typ domain.TxType) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for h, t := range m.Txs {
		if t.NFTAddress == nft && t.Type == typ && t.Status == domain.TxPending {
			t.Status = domain.TxConfirmed
			m.Txs[h] = t
			m.Events = append(m.Events, string(domain.EventTxUpdated)+":"+string(domain.TxConfirmed))
			out = append(out, h)
		}
	}
	return out, nil
}
func (m *MemRepo) GetCursor(_ context.Context, k string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Cursors[k], nil
}
func (m *MemRepo) SetCursor(_ context.Context, k string, lt uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lt > m.Cursors[k] {
		m.Cursors[k] = lt
	}
	return nil
}
