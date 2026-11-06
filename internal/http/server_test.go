package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"tonnet/internal/auth"
	"tonnet/internal/domain"
	"tonnet/internal/domain/domaintest"
)

const (
	colRaw    = "0:6a29ea8f26e1cbef5b5448c759cb852f27cde7526f9cb211f1d472111c3e4f8f"
	nftRaw    = "0:75b3dabef4be7f4fc6323230475b674601ada031ad7bd3b0ebd6c9e72c8b67dc"
	sellerRaw = "0:b248417cf607e5a4a3bffc04a6e042fd4629cc5bc34fd83c8bae1a89b80a3766"
	buyerRaw  = "0:1111111111111111111111111111111111111111111111111111111111111111"
	saleRaw   = "0:5555555555555555555555555555555555555555555555555555555555555555"
)

type fakePayloads struct{}

func (fakePayloads) BuildListing(t domain.SaleTerms) (*domain.ListingIntent, error) {
	return &domain.ListingIntent{SaleContract: saleRaw, Messages: []domain.Message{{Address: "MKT", AmountNano: 50_000_000, Payload: "AA=="}, {Address: "NFT", AmountNano: 100_000_000, Payload: "BB=="}}}, nil
}
func (fakePayloads) BuildBuy(sale string, price int64, _ uint64) (domain.Message, error) {
	return domain.Message{Address: sale, AmountNano: price + 150_000_000, Payload: "CC=="}, nil
}
func (fakePayloads) BuildCancel(sale string, _ uint64) (domain.Message, error) {
	return domain.Message{Address: sale, AmountNano: 120_000_000, Payload: "DD=="}, nil
}
func (fakePayloads) MessageHash(boc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(boc)
	if err != nil {
		return "", err
	}
	c, err := cell.FromBOC(raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", c.Hash()), nil
}

type noRoyalty struct{}

func (noRoyalty) GetRoyaltyParams(context.Context, string) (*domain.RoyaltyParams, error) {
	return &domain.RoyaltyParams{}, nil
}

type env struct {
	srv    *httptest.Server
	repo   *domaintest.MemRepo
	tokens *auth.TokenIssuer
	hub    *Hub
}

func newEnv(t *testing.T) *env {
	t.Helper()
	repo := domaintest.NewMemRepo()
	ctx := context.Background()
	require.NoError(t, repo.UpsertCollection(ctx, domain.Collection{Address: colRaw, Owner: sellerRaw, Name: "Demo", Metadata: json.RawMessage(`{"name":"Demo"}`)}))
	require.NoError(t, repo.UpsertItem(ctx, domain.NFTItem{Address: nftRaw, CollectionAddress: colRaw, Index: 0, Owner: sellerRaw, Name: "Zero"}))
	secret := []byte("0123456789abcdef0123456789abcdef")
	payloads := auth.NewPayloadIssuer(secret, time.Minute)
	tokens := auth.NewTokenIssuer(secret, time.Hour)
	hub := NewHub([]string{"*"}, slog.Default())
	s := NewServer(Deps{
		Catalog:  domain.NewCatalogService(repo, repo, repo, repo),
		Listings: domain.NewListingService(repo, repo, fakePayloads{}, noRoyalty{}, domain.MarketplaceConfig{FeeBPS: 500}, nil),
		Txs:      domain.NewTxService(repo, repo, fakePayloads{}, nil, time.Minute),
		Payloads: payloads,
		Proofs:   auth.NewVerifier(payloads, nil, []string{"localhost"}, time.Minute),
		Tokens:   tokens,
		Hub:      hub,
		Testnet:  true,
		Origins:  []string{"*"},
		Log:      slog.Default(),
	})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &env{srv: srv, repo: repo, tokens: tokens, hub: hub}
}

func (e *env) do(t *testing.T, method, path string, body any, token string) (*http.Response, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func (e *env) token(t *testing.T, addr string) string {
	tok, _, err := e.tokens.Issue(addr)
	require.NoError(t, err)
	return tok
}

func TestHealthAndCatalog(t *testing.T) {
	e := newEnv(t)
	resp, body := e.do(t, http.MethodGet, "/healthz", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", body["status"])

	resp, body = e.do(t, http.MethodGet, "/api/collections", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	items := body["items"].([]any)
	require.Len(t, items, 1)
	col := items[0].(map[string]any)
	addr := col["address"].(map[string]any)
	assert.Equal(t, colRaw, addr["raw"])
	assert.True(t, strings.HasPrefix(addr["address"].(string), "k"), "testnet bounceable form")
	assert.Equal(t, "Demo", col["name"])

	// Friendly address in the path is accepted and normalised.
	resp, body = e.do(t, http.MethodGet, "/api/collections/"+addr["address"].(string)+"/items?sort=price&page=1&limit=10", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.EqualValues(t, 1, body["total"])
	assert.EqualValues(t, 10, body["limit"])

	// Percent-encoded raw form (what encodeURIComponent produces) is accepted too.
	resp, _ = e.do(t, http.MethodGet, "/api/collections/"+strings.ReplaceAll(colRaw, ":", "%3A")+"/items", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = e.do(t, http.MethodGet, "/api/collections/not-an-address/items", nil, "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodGet, "/api/collections/"+buyerRaw, nil, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp, body = e.do(t, http.MethodGet, "/api/items/"+nftRaw, nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	item := body["item"].(map[string]any)
	assert.Equal(t, "Zero", item["name"])
	assert.Nil(t, item["listing"])
	assert.NotNil(t, body["transactions"])
	assert.Equal(t, "Demo", item["collection"].(map[string]any)["name"])
	owner := item["owner"].(map[string]any)
	assert.True(t, strings.HasPrefix(owner["address"].(string), "0Q"), "wallets rendered non-bounceable testnet")

	resp, body = e.do(t, http.MethodGet, "/api/users/"+sellerRaw+"/items", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, body["items"].([]any), 1)

	resp, body = e.do(t, http.MethodGet, "/api/listings", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.EqualValues(t, 0, body["total"])
	resp, _ = e.do(t, http.MethodGet, "/api/listings?status=weird", nil, "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodGet, "/api/listings/abc", nil, "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodGet, "/api/listings/5", nil, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp, _ = e.do(t, http.MethodGet, "/api/tx/short", nil, "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodGet, "/api/tx/"+strings.Repeat("a", 64), nil, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAuthEndpoints(t *testing.T) {
	e := newEnv(t)
	resp, body := e.do(t, http.MethodPost, "/api/auth/payload", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, body["payload"].(string), 48)

	resp, body = e.do(t, http.MethodPost, "/api/auth/verify", map[string]any{"address": sellerRaw, "proof": map[string]any{"payload": "x"}}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "invalid_proof", body["error"].(map[string]any)["code"])

	resp, _ = e.do(t, http.MethodPost, "/api/auth/verify", nil, "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp, _ = e.do(t, http.MethodGet, "/api/auth/me", nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp, body = e.do(t, http.MethodGet, "/api/auth/me", nil, e.token(t, sellerRaw))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, sellerRaw, body["address"].(map[string]any)["raw"])
}

func TestListingFlowOverHTTP(t *testing.T) {
	e := newEnv(t)
	seller := e.token(t, sellerRaw)
	buyer := e.token(t, buyerRaw)
	ctx := context.Background()

	resp, _ := e.do(t, http.MethodPost, "/api/listings/intent", map[string]any{"nft_address": nftRaw, "price_nano": "2000000000"}, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp, _ = e.do(t, http.MethodPost, "/api/listings/intent", map[string]any{"nft_address": nftRaw, "price_nano": "2000000000"}, buyer)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, _ = e.do(t, http.MethodPost, "/api/listings/intent", map[string]any{"nft_address": nftRaw, "price_nano": "abc"}, seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodPost, "/api/listings/intent", map[string]any{"nft_address": "zzz", "price_nano": "1"}, seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodPost, "/api/listings/intent", map[string]any{"nft_address": nftRaw, "price_nano": "0"}, seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp, body := e.do(t, http.MethodPost, "/api/listings/intent", map[string]any{"nft_address": nftRaw, "price_nano": "2000000000"}, seller)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	listing := body["listing"].(map[string]any)
	assert.Equal(t, "pending", listing["status"])
	assert.Equal(t, "2", listing["price_ton"])
	assert.Equal(t, "2000000000", listing["price_nano"])
	msgs := body["messages"].([]any)
	require.Len(t, msgs, 2)
	assert.Equal(t, "50000000", msgs[0].(map[string]any)["amount"])
	assert.Greater(t, body["valid_until"].(float64), float64(time.Now().Unix()))
	id := int64(listing["id"].(float64))

	resp, _ = e.do(t, http.MethodPost, "/api/listings/intent", map[string]any{"nft_address": nftRaw, "price_nano": "1"}, seller)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	// Register the wallet's signed external message.
	boc := base64.StdEncoding.EncodeToString(cell.BeginCell().MustStoreUInt(1, 8).EndCell().ToBOC())
	resp, body = e.do(t, http.MethodPost, "/api/tx", map[string]any{"boc": boc, "type": "list", "listing_id": id}, seller)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	hash := body["hash"].(string)
	assert.Len(t, hash, 64)
	assert.Equal(t, "pending", body["status"])
	assert.Equal(t, nftRaw, body["nft_address"].(map[string]any)["raw"])

	resp, body = e.do(t, http.MethodGet, "/api/tx/"+hash, nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "list", body["type"])

	resp, _ = e.do(t, http.MethodPost, "/api/tx", map[string]any{"boc": "!!", "type": "list", "listing_id": id}, seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodPost, "/api/tx", map[string]any{"boc": boc, "type": "list", "nft_address": "bad"}, seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodPost, "/api/tx", "not json", seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Buy is refused until the indexer activates the listing.
	resp, _ = e.do(t, http.MethodPost, fmt.Sprintf("/api/listings/%d/buy-intent", id), nil, buyer)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	require.NoError(t, e.repo.UpdateListingStatus(ctx, id, domain.ListingActive, ""))

	resp, body = e.do(t, http.MethodGet, "/api/listings?status=active", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.EqualValues(t, 1, body["total"])
	resp, body = e.do(t, http.MethodGet, fmt.Sprintf("/api/listings/%d", id), nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "active", body["status"])
	assert.NotNil(t, body["item"])

	resp, body = e.do(t, http.MethodPost, fmt.Sprintf("/api/listings/%d/buy-intent", id), nil, buyer)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	msg := body["messages"].([]any)[0].(map[string]any)
	assert.Equal(t, "2150000000", msg["amount"])
	resp, _ = e.do(t, http.MethodPost, fmt.Sprintf("/api/listings/%d/buy-intent", id), nil, seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = e.do(t, http.MethodPost, "/api/listings/x/buy-intent", nil, buyer)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp, _ = e.do(t, http.MethodPost, fmt.Sprintf("/api/listings/%d/cancel-intent", id), nil, buyer)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, body = e.do(t, http.MethodPost, fmt.Sprintf("/api/listings/%d/cancel-intent", id), nil, seller)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "DD==", body["messages"].([]any)[0].(map[string]any)["payload"])
	resp, _ = e.do(t, http.MethodPost, "/api/listings/x/cancel-intent", nil, seller)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestWebSocketBroadcast(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	defer c.Close(websocket.StatusNormalClosure, "")

	_, hello, err := c.Read(ctx)
	require.NoError(t, err)
	assert.Contains(t, string(hello), "hello")
	require.Eventually(t, func() bool { return e.hub.ClientCount() == 1 }, time.Second, 10*time.Millisecond)

	e.hub.Broadcast(domain.Event{Type: "tx.updated", TxHash: "abc", Status: "confirmed", At: time.Now()})
	_, msg, err := c.Read(ctx)
	require.NoError(t, err)
	var ev domain.Event
	require.NoError(t, json.Unmarshal(msg, &ev))
	assert.Equal(t, "tx.updated", ev.Type)
	assert.Equal(t, "confirmed", ev.Status)

	resp, body := e.do(t, http.MethodGet, "/healthz", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.EqualValues(t, 1, body["ws_clients"])
}

func TestHealthDegraded(t *testing.T) {
	s := NewServer(Deps{Health: func() error { return fmt.Errorf("db down") }})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
