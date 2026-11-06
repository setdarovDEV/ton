// Package ton implements chain.Client for The Open Network using the tonapi.io
// REST API (testnet or mainnet). Switching to a lite-client later only means
// another implementation of chain.Client behind the same interface.
package ton

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/tvm/cell"

	"tonnet/internal/chain"
)

// Config configures the tonapi client.
type Config struct {
	BaseURL string  // e.g. https://testnet.tonapi.io
	APIKey  string  // optional bearer token
	RPS     float64 // requests per second budget
	Timeout time.Duration
}

// Client talks to tonapi.io.
type Client struct {
	base    string
	apiKey  string
	http    *http.Client
	limiter *rateLimiter
	timeout time.Duration
	log     *slog.Logger
}

var _ chain.Client = (*Client)(nil)

// NewClient builds a tonapi-backed chain client.
func NewClient(cfg Config, log *slog.Logger) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		base:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		http:    &http.Client{Timeout: cfg.Timeout},
		limiter: newRateLimiter(cfg.RPS, 2),
		timeout: cfg.Timeout,
		log:     log.With("component", "tonapi"),
	}
}

const maxRetries = 3

// get performs a rate-limited GET with a per-call deadline and retries on 429/5xx.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit wait: %w", err)
		}
		status, err := c.do(ctx, u, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if status == http.StatusNotFound {
			return fmt.Errorf("%s: %w", path, chain.ErrNotFound)
		}
		if status != http.StatusTooManyRequests && (status < 500 || status > 599) && status != 0 {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		backoff := time.Duration(attempt+1) * 700 * time.Millisecond
		c.log.Warn("tonapi request failed, retrying", "path", path, "status", status, "attempt", attempt+1, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
	return lastErr
}

func (c *Client) do(ctx context.Context, u string, out any) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("tonapi GET %s: %w", u, err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing to do on close error
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read tonapi response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var ae apiError
		_ = json.Unmarshal(body, &ae)
		return resp.StatusCode, fmt.Errorf("tonapi %s: status %d: %s", u, resp.StatusCode, strings.TrimSpace(ae.Error))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode tonapi response: %w", err)
	}
	return resp.StatusCode, nil
}

func addrOf(a *apiAccountAddress) string {
	if a == nil {
		return ""
	}
	return strings.ToLower(a.Address)
}

func parseView(meta json.RawMessage) metadataView {
	var v metadataView
	_ = json.Unmarshal(meta, &v)
	return v
}

func toCollection(a apiCollection) *chain.Collection {
	v := parseView(a.Metadata)
	return &chain.Collection{
		Address:       strings.ToLower(a.Address),
		Owner:         addrOf(a.Owner),
		NextItemIndex: a.NextItemIndex,
		Name:          v.Name,
		Description:   v.Description,
		ImageURL:      v.Image,
		Metadata:      nonNilJSON(a.Metadata),
	}
}

func toItem(a apiItem) chain.Item {
	v := parseView(a.Metadata)
	it := chain.Item{
		Address:  strings.ToLower(a.Address),
		Index:    a.Index,
		Owner:    addrOf(a.Owner),
		Name:     v.Name,
		ImageURL: v.Image,
		Metadata: nonNilJSON(a.Metadata),
	}
	if a.Collection != nil {
		it.Collection = strings.ToLower(a.Collection.Address)
	}
	if a.Sale != nil {
		price, _ := strconv.ParseInt(a.Sale.Price.Value, 10, 64)
		it.Sale = &chain.SaleInfo{Address: strings.ToLower(a.Sale.Address), Owner: addrOf(a.Sale.Owner), PriceNano: price}
	}
	return it
}

func nonNilJSON(m json.RawMessage) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage("{}")
	}
	return m
}

func toTx(a apiTransaction) chain.Transaction {
	tx := chain.Transaction{
		Hash:    strings.ToLower(a.Hash),
		LT:      a.LT,
		Account: strings.ToLower(a.Account.Address),
		Success: a.Success,
		Time:    a.Utime,
	}
	if a.InMsg != nil {
		m := toMsg(*a.InMsg)
		tx.InMsg = &m
	}
	for _, om := range a.OutMsgs {
		tx.OutMsgs = append(tx.OutMsgs, toMsg(om))
	}
	return tx
}

func toMsg(m apiMessage) chain.Message {
	var op uint32
	if m.OpCode != "" {
		if v, err := strconv.ParseUint(strings.TrimPrefix(m.OpCode, "0x"), 16, 32); err == nil {
			op = uint32(v)
		}
	}
	return chain.Message{
		Hash:        strings.ToLower(m.Hash),
		Source:      addrOf(m.Source),
		Destination: addrOf(m.Destination),
		ValueNano:   m.Value,
		OpCode:      op,
		CreatedLT:   m.CreatedLT,
		Bounced:     m.Bounced,
	}
}

// GetCollection implements chain.Client.
func (c *Client) GetCollection(ctx context.Context, address string) (*chain.Collection, error) {
	raw, err := NormalizeRaw(address)
	if err != nil {
		return nil, err
	}
	var out apiCollection
	if err := c.get(ctx, "/v2/nfts/collections/"+raw, nil, &out); err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	return toCollection(out), nil
}

// GetCollectionItems implements chain.Client.
func (c *Client) GetCollectionItems(ctx context.Context, collection string, limit, offset int) ([]chain.Item, error) {
	raw, err := NormalizeRaw(collection)
	if err != nil {
		return nil, err
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
	var out apiItems
	if err := c.get(ctx, "/v2/nfts/collections/"+raw+"/items", q, &out); err != nil {
		return nil, fmt.Errorf("get collection items: %w", err)
	}
	items := make([]chain.Item, 0, len(out.NFTItems))
	for _, it := range out.NFTItems {
		items = append(items, toItem(it))
	}
	return items, nil
}

// GetItem implements chain.Client.
func (c *Client) GetItem(ctx context.Context, address string) (*chain.Item, error) {
	raw, err := NormalizeRaw(address)
	if err != nil {
		return nil, err
	}
	var out apiItem
	if err := c.get(ctx, "/v2/nfts/"+raw, nil, &out); err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	it := toItem(out)
	return &it, nil
}

// GetAccountItems implements chain.Client (includes items held by sale contracts).
func (c *Client) GetAccountItems(ctx context.Context, owner string, limit, offset int) ([]chain.Item, error) {
	raw, err := NormalizeRaw(owner)
	if err != nil {
		return nil, err
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}, "indirect_ownership": {"true"}}
	var out apiItems
	if err := c.get(ctx, "/v2/accounts/"+raw+"/nfts", q, &out); err != nil {
		return nil, fmt.Errorf("get account items: %w", err)
	}
	items := make([]chain.Item, 0, len(out.NFTItems))
	for _, it := range out.NFTItems {
		items = append(items, toItem(it))
	}
	return items, nil
}

// GetTransactions implements chain.Client.
func (c *Client) GetTransactions(ctx context.Context, address string, afterLT uint64, limit int) ([]chain.Transaction, error) {
	raw, err := NormalizeRaw(address)
	if err != nil {
		return nil, err
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}, "sort_order": {"asc"}}
	if afterLT > 0 {
		q.Set("after_lt", strconv.FormatUint(afterLT, 10))
	}
	var out apiTransactions
	if err := c.get(ctx, "/v2/blockchain/accounts/"+raw+"/transactions", q, &out); err != nil {
		return nil, fmt.Errorf("get transactions: %w", err)
	}
	txs := make([]chain.Transaction, 0, len(out.Transactions))
	for _, t := range out.Transactions {
		if t.LT <= afterLT {
			continue // tonapi's after_lt is inclusive in some versions; stay strict
		}
		txs = append(txs, toTx(t))
	}
	return txs, nil
}

// GetTransactionByMessageHash implements chain.Client.
func (c *Client) GetTransactionByMessageHash(ctx context.Context, msgHash string) (*chain.Transaction, error) {
	var out apiTransaction
	if err := c.get(ctx, "/v2/blockchain/messages/"+strings.ToLower(msgHash)+"/transaction", nil, &out); err != nil {
		return nil, fmt.Errorf("get tx by message: %w", err)
	}
	tx := toTx(out)
	return &tx, nil
}

// GetPublicKey implements chain.Client.
func (c *Client) GetPublicKey(ctx context.Context, address string) ([]byte, error) {
	raw, err := NormalizeRaw(address)
	if err != nil {
		return nil, err
	}
	var out apiPublicKey
	if err := c.get(ctx, "/v2/accounts/"+raw+"/publickey", nil, &out); err != nil {
		return nil, fmt.Errorf("get public key: %w", err)
	}
	key, err := hex.DecodeString(out.PublicKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("invalid public key %q", out.PublicKey)
	}
	return key, nil
}

func (c *Client) runGetMethod(ctx context.Context, address, method string) ([]apiStackRecord, error) {
	raw, err := NormalizeRaw(address)
	if err != nil {
		return nil, err
	}
	var out apiMethodResult
	if err := c.get(ctx, "/v2/blockchain/accounts/"+raw+"/methods/"+method, nil, &out); err != nil {
		return nil, fmt.Errorf("run %s: %w", method, err)
	}
	if !out.Success || out.ExitCode != 0 {
		return nil, fmt.Errorf("run %s on %s: %w", method, raw, &exitCodeError{Method: method, Code: out.ExitCode})
	}
	return out.Stack, nil
}

func stackNum(r apiStackRecord) (*big.Int, error) {
	if r.Type != "num" {
		return nil, fmt.Errorf("stack entry is %s, want num", r.Type)
	}
	s := strings.TrimPrefix(r.Num, "0x")
	n, ok := new(big.Int).SetString(s, 16)
	if !ok {
		return nil, fmt.Errorf("bad num %q", r.Num)
	}
	return n, nil
}

func stackAddr(r apiStackRecord) (string, error) {
	var enc string
	switch r.Type {
	case "cell":
		enc = r.Cell
	case "slice":
		enc = r.Slice
		if enc == "" {
			enc = r.Cell
		}
	case "null":
		return "", nil
	default:
		return "", fmt.Errorf("stack entry is %s, want slice", r.Type)
	}
	c, err := decodeCell(enc)
	if err != nil {
		return "", err
	}
	s, err := c.BeginParse()
	if err != nil {
		return "", fmt.Errorf("parse slice: %w", err)
	}
	a, err := s.LoadAddr()
	if err != nil {
		return "", fmt.Errorf("load address: %w", err)
	}
	if a.IsAddrNone() {
		return "", nil
	}
	return strings.ToLower(a.StringRaw()), nil
}

func decodeCell(enc string) (*cell.Cell, error) {
	var b []byte
	var err error
	if b, err = base64.StdEncoding.DecodeString(enc); err != nil {
		if b, err = hex.DecodeString(enc); err != nil {
			return nil, fmt.Errorf("decode cell %q: not base64 nor hex", enc[:min(len(enc), 16)])
		}
	}
	c, err := cell.FromBOC(b)
	if err != nil {
		return nil, fmt.Errorf("parse cell boc: %w", err)
	}
	return c, nil
}

// GetRoyaltyParams implements chain.Client via TEP-66 royalty_params.
func (c *Client) GetRoyaltyParams(ctx context.Context, collection string) (*chain.RoyaltyParams, error) {
	stack, err := c.runGetMethod(ctx, collection, "royalty_params")
	if err != nil {
		// Collections without royalty support (or not deployed yet) fail the
		// get-method; TEP-66 is optional, so treat that as "no royalty".
		var ec *exitCodeError
		if errors.As(err, &ec) || errors.Is(err, chain.ErrNotFound) {
			return &chain.RoyaltyParams{}, nil
		}
		return nil, err
	}
	if len(stack) < 3 {
		return nil, fmt.Errorf("royalty_params: short stack (%d)", len(stack))
	}
	num, err := stackNum(stack[0])
	if err != nil {
		return nil, fmt.Errorf("royalty numerator: %w", err)
	}
	den, err := stackNum(stack[1])
	if err != nil {
		return nil, fmt.Errorf("royalty denominator: %w", err)
	}
	addr, err := stackAddr(stack[2])
	if err != nil {
		return nil, fmt.Errorf("royalty address: %w", err)
	}
	return &chain.RoyaltyParams{Numerator: num.Int64(), Denominator: den.Int64(), Address: addr}, nil
}

// exitCodeError reports a TVM get-method that completed with a non-zero exit code.
type exitCodeError struct {
	Method string
	Code   int
}

func (e *exitCodeError) Error() string { return fmt.Sprintf("%s: exit code %d", e.Method, e.Code) }

// GetSaleData implements chain.Client via get_fix_price_data of the Getgems
// fixed-price sale contract (v3): is_complete, created_at, marketplace, nft,
// owner, full_price, fee_addr, fee, royalty_addr, royalty, sold_at, query_id.
func (c *Client) GetSaleData(ctx context.Context, sale string) (*chain.SaleData, error) {
	stack, err := c.runGetMethod(ctx, sale, "get_fix_price_data")
	if err != nil {
		return nil, err
	}
	if len(stack) < 6 {
		return nil, fmt.Errorf("get_fix_price_data: short stack (%d)", len(stack))
	}
	complete, err := stackNum(stack[0])
	if err != nil {
		return nil, fmt.Errorf("is_complete: %w", err)
	}
	market, err := stackAddr(stack[2])
	if err != nil {
		return nil, fmt.Errorf("marketplace: %w", err)
	}
	nft, err := stackAddr(stack[3])
	if err != nil {
		return nil, fmt.Errorf("nft: %w", err)
	}
	owner, err := stackAddr(stack[4])
	if err != nil {
		return nil, fmt.Errorf("owner: %w", err)
	}
	price, err := stackNum(stack[5])
	if err != nil {
		return nil, fmt.Errorf("price: %w", err)
	}
	return &chain.SaleData{
		IsComplete: complete.Sign() != 0,
		Market:     market,
		NFT:        nft,
		Owner:      owner,
		PriceNano:  price.Int64(),
	}, nil
}
