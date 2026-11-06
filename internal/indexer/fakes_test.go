package indexer

import (
	"context"
	"sync"

	"tonnet/internal/chain"
)

// fakeChain is an in-memory chain.Client.
type fakeChain struct {
	mu          sync.Mutex
	collections map[string]*chain.Collection
	items       map[string][]chain.Item // by collection
	txs         map[string][]chain.Transaction
	byMsg       map[string]*chain.Transaction
	calls       map[string]int
	err         error
}

func newFakeChain() *fakeChain {
	return &fakeChain{collections: map[string]*chain.Collection{}, items: map[string][]chain.Item{}, txs: map[string][]chain.Transaction{}, byMsg: map[string]*chain.Transaction{}, calls: map[string]int{}}
}

func (f *fakeChain) count(name string) { f.calls[name]++ }

func (f *fakeChain) setOwner(collection, item, owner string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.items[collection] {
		if f.items[collection][i].Address == item {
			f.items[collection][i].Owner = owner
		}
	}
}

func (f *fakeChain) GetCollection(_ context.Context, a string) (*chain.Collection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("GetCollection")
	if f.err != nil {
		return nil, f.err
	}
	c, ok := f.collections[a]
	if !ok {
		return nil, chain.ErrNotFound
	}
	return c, nil
}

func (f *fakeChain) GetCollectionItems(_ context.Context, c string, limit, offset int) ([]chain.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("GetCollectionItems")
	all := f.items[c]
	if offset >= len(all) {
		return nil, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	out := make([]chain.Item, end-offset)
	copy(out, all[offset:end])
	return out, nil
}

func (f *fakeChain) GetItem(context.Context, string) (*chain.Item, error) {
	return nil, chain.ErrNotFound
}
func (f *fakeChain) GetAccountItems(context.Context, string, int, int) ([]chain.Item, error) {
	return nil, nil
}

func (f *fakeChain) GetTransactions(_ context.Context, a string, after uint64, limit int) ([]chain.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("GetTransactions:" + a)
	var out []chain.Transaction
	for _, t := range f.txs[a] {
		if t.LT > after && len(out) < limit {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeChain) GetTransactionByMessageHash(_ context.Context, h string) (*chain.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("GetTransactionByMessageHash")
	t, ok := f.byMsg[h]
	if !ok {
		return nil, chain.ErrNotFound
	}
	return t, nil
}

func (f *fakeChain) GetPublicKey(context.Context, string) ([]byte, error) { return nil, nil }
func (f *fakeChain) GetRoyaltyParams(context.Context, string) (*chain.RoyaltyParams, error) {
	return &chain.RoyaltyParams{}, nil
}
func (f *fakeChain) GetSaleData(context.Context, string) (*chain.SaleData, error) { return nil, nil }
