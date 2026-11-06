//go:build e2e

package ton

import (
	"context"
	"encoding/hex"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run with: go test -tags e2e ./internal/chain/ton -run TestE2E -v
func TestE2ETestnet(t *testing.T) {
	c := NewClient(Config{BaseURL: "https://testnet.tonapi.io", APIKey: os.Getenv("TONAPI_KEY"), RPS: 1, Timeout: 20 * time.Second}, slog.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	col, err := c.GetCollection(ctx, colRaw)
	require.NoError(t, err)
	assert.Equal(t, "test collection", col.Name)
	assert.EqualValues(t, 1, col.NextItemIndex)

	items, err := c.GetCollectionItems(ctx, colRaw, 10, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, itemRaw, items[0].Address)
	assert.Equal(t, ownerRaw, items[0].Owner)

	key, err := c.GetPublicKey(ctx, ownerRaw)
	require.NoError(t, err)
	assert.Equal(t, "ac117a42e8b29a41a594d0d4ace10a956fcacbea695914db48c50eadc061f55e", hex.EncodeToString(key))

	txs, err := c.GetTransactions(ctx, colRaw, 0, 10)
	require.NoError(t, err)
	require.NotEmpty(t, txs)
	assert.Equal(t, "13c884a5fe7ffb687bb2d28d820fa4bf462bf926b958e8016672bd92b8e6f6de", txs[0].Hash)

	roy, err := c.GetRoyaltyParams(ctx, colRaw)
	require.NoError(t, err)
	t.Logf("royalty: %+v", roy)

	mine, err := c.GetAccountItems(ctx, ownerRaw, 5, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, mine)
}
