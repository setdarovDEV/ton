package ton

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"tonnet/internal/chain"
)

const (
	colRaw   = "0:6a29ea8f26e1cbef5b5448c759cb852f27cde7526f9cb211f1d472111c3e4f8f"
	itemRaw  = "0:75b3dabef4be7f4fc6323230475b674601ada031ad7bd3b0ebd6c9e72c8b67dc"
	ownerRaw = "0:b248417cf607e5a4a3bffc04a6e042fd4629cc5bc34fd83c8bae1a89b80a3766"
)

func addrCell(raw string) string {
	a := address.MustParseRawAddr(raw)
	c := cell.BeginCell().MustStoreAddr(a).EndCell()
	return base64.StdEncoding.EncodeToString(c.ToBOC())
}

func newServer(t *testing.T, routes map[string]string, status *int32) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if status != nil && atomic.LoadInt32(status) != 0 {
			code := int(atomic.SwapInt32(status, 0))
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"entity not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(Config{BaseURL: srv.URL + "/", APIKey: "key", RPS: 1000, Timeout: 2 * time.Second}, slog.Default())
	return srv, c
}

func TestClientReads(t *testing.T) {
	routes := map[string]string{
		"/v2/nfts/collections/" + colRaw: `{"address":"0:6A29EA8F26E1CBEF5B5448C759CB852F27CDE7526F9CB211F1D472111C3E4F8F","next_item_index":1,
			"owner":{"address":"` + ownerRaw + `"},"metadata":{"name":"test collection","description":"d","image":"https://img/c.png"}}`,
		"/v2/nfts/collections/" + colRaw + "/items": `{"nft_items":[{"address":"` + itemRaw + `","index":0,"owner":{"address":"` + ownerRaw + `"},
			"collection":{"address":"` + colRaw + `"},"metadata":{"name":"Item 0","image":"https://img/0.png"},
			"sale":{"address":"0:aa","owner":{"address":"0:bb"},"price":{"value":"1500000000","token_name":"TON"}}}]}`,
		"/v2/nfts/" + itemRaw:                     `{"address":"` + itemRaw + `","index":0,"owner":{"address":"` + ownerRaw + `"},"metadata":{}}`,
		"/v2/accounts/" + ownerRaw + "/nfts":      `{"nft_items":[{"address":"` + itemRaw + `","index":0,"metadata":{"name":"x"}}]}`,
		"/v2/accounts/" + ownerRaw + "/publickey": `{"public_key":"ac117a42e8b29a41a594d0d4ace10a956fcacbea695914db48c50eadc061f55e"}`,
		"/v2/blockchain/accounts/" + colRaw + "/transactions": `{"transactions":[
			{"hash":"AA","lt":5,"account":{"address":"` + colRaw + `"},"success":true,"utime":1,"in_msg":{"hash":"m1","created_lt":4,"value":10,"op_code":"0x00000001","source":{"address":"` + ownerRaw + `"},"destination":{"address":"` + colRaw + `"}},"out_msgs":[{"hash":"m2","created_lt":6,"value":5,"destination":{"address":"` + itemRaw + `"}}]},
			{"hash":"BB","lt":10,"account":{"address":"` + colRaw + `"},"success":false,"utime":2,"out_msgs":[]}]}`,
		"/v2/blockchain/messages/abc/transaction": `{"hash":"CC","lt":11,"account":{"address":"` + ownerRaw + `"},"success":true,"utime":3}`,
		"/v2/blockchain/accounts/" + colRaw + "/methods/royalty_params": `{"success":true,"exit_code":0,"stack":[
			{"type":"num","num":"0x5"},{"type":"num","num":"0x64"},{"type":"cell","cell":"` + addrCell(ownerRaw) + `"}]}`,
		"/v2/blockchain/accounts/0:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/methods/get_fix_price_data": `{"success":true,"exit_code":0,"stack":[
			{"type":"num","num":"0x0"},{"type":"num","num":"0x1"},{"type":"cell","cell":"` + addrCell(colRaw) + `"},{"type":"cell","cell":"` + addrCell(itemRaw) + `"},
			{"type":"cell","cell":"` + addrCell(ownerRaw) + `"},{"type":"num","num":"0x59682f00"}]}`,
		"/v2/blockchain/accounts/" + itemRaw + "/methods/royalty_params": `{"success":false,"exit_code":11,"stack":[]}`,
	}
	_, c := newServer(t, routes, nil)
	ctx := context.Background()

	col, err := c.GetCollection(ctx, "EQBqKeqPJuHL71tUSMdZy4UvJ83nUm-cshHx1HIRHD5Pj5YW")
	require.NoError(t, err)
	assert.Equal(t, colRaw, col.Address)
	assert.Equal(t, "test collection", col.Name)
	assert.Equal(t, "https://img/c.png", col.ImageURL)
	assert.Equal(t, ownerRaw, col.Owner)
	assert.EqualValues(t, 1, col.NextItemIndex)

	items, err := c.GetCollectionItems(ctx, colRaw, 10, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Item 0", items[0].Name)
	assert.Equal(t, colRaw, items[0].Collection)
	require.NotNil(t, items[0].Sale)
	assert.EqualValues(t, 1_500_000_000, items[0].Sale.PriceNano)

	item, err := c.GetItem(ctx, itemRaw)
	require.NoError(t, err)
	assert.Equal(t, ownerRaw, item.Owner)
	assert.JSONEq(t, "{}", string(item.Metadata))

	mine, err := c.GetAccountItems(ctx, ownerRaw, 10, 0)
	require.NoError(t, err)
	assert.Len(t, mine, 1)
	assert.Empty(t, mine[0].Owner)

	key, err := c.GetPublicKey(ctx, ownerRaw)
	require.NoError(t, err)
	assert.Len(t, key, 32)

	txs, err := c.GetTransactions(ctx, colRaw, 5, 100)
	require.NoError(t, err)
	require.Len(t, txs, 1, "lt <= afterLT must be filtered out")
	assert.Equal(t, "bb", txs[0].Hash)
	txs, err = c.GetTransactions(ctx, colRaw, 0, 100)
	require.NoError(t, err)
	require.Len(t, txs, 2)
	require.NotNil(t, txs[0].InMsg)
	assert.EqualValues(t, 1, txs[0].InMsg.OpCode)
	assert.Equal(t, ownerRaw, txs[0].InMsg.Source)
	assert.Equal(t, itemRaw, txs[0].OutMsgs[0].Destination)

	tx, err := c.GetTransactionByMessageHash(ctx, "ABC")
	require.NoError(t, err)
	assert.Equal(t, "cc", tx.Hash)

	roy, err := c.GetRoyaltyParams(ctx, colRaw)
	require.NoError(t, err)
	assert.Equal(t, &chain.RoyaltyParams{Numerator: 5, Denominator: 100, Address: ownerRaw}, roy)
	roy, err = c.GetRoyaltyParams(ctx, itemRaw)
	require.NoError(t, err, "non-zero exit code means no royalty")
	assert.Equal(t, &chain.RoyaltyParams{}, roy)

	sale, err := c.GetSaleData(ctx, "0:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	assert.Equal(t, &chain.SaleData{Market: colRaw, NFT: itemRaw, Owner: ownerRaw, PriceNano: 1_500_000_000}, sale)

	_, err = c.GetItem(ctx, "0:0000000000000000000000000000000000000000000000000000000000000000")
	assert.ErrorIs(t, err, chain.ErrNotFound)
	_, err = c.GetItem(ctx, "garbage")
	assert.Error(t, err)
}

func TestClientRetriesOn429(t *testing.T) {
	status := int32(http.StatusTooManyRequests)
	_, c := newServer(t, map[string]string{"/v2/nfts/" + itemRaw: `{"address":"` + itemRaw + `","index":1}`}, &status)
	item, err := c.GetItem(context.Background(), itemRaw)
	require.NoError(t, err)
	assert.EqualValues(t, 1, item.Index)
}

func TestClientHonoursContext(t *testing.T) {
	_, c := newServer(t, map[string]string{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetItem(ctx, itemRaw)
	assert.Error(t, err)
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(50, 1)
	start := time.Now()
	for i := 0; i < 4; i++ {
		require.NoError(t, rl.Wait(context.Background()))
	}
	assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_ = rl.Wait(ctx)
	_ = rl.Wait(ctx)
	assert.Error(t, rl.Wait(ctx))
}
