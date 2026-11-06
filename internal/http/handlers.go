package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"tonnet/internal/auth"
	"tonnet/internal/domain"
)

const (
	defaultPageSize = 24
	maxPageSize     = 100
)

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// rawAddr normalises a path/body address or returns a 400-able error. Path
// params arrive percent-encoded (chi does not unescape them), so "0%3Aabc"
// must become "0:abc" before parsing.
func (s *Server) rawAddr(in string) (string, error) {
	if unescaped, err := url.PathUnescape(in); err == nil {
		in = unescaped
	}
	raw, err := s.deps.Addrs.Normalize(in)
	if err != nil {
		return "", fmt.Errorf("%w: invalid address %q", errBadRequest, in)
	}
	return raw, nil
}

func pageFrom(r *http.Request) domain.Page {
	p := domain.Page{Number: 1, Size: defaultPageSize}
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		p.Number = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		p.Size = min(v, maxPageSize)
	}
	return p
}

// names loads collection display names for embedding into item DTOs. The
// collections table is tiny (tracked collections only), so one query per
// request is cheaper than a join on every item query.
func (s *Server) names(r *http.Request) collectionNames {
	out, err := s.deps.Catalog.CollectionNames(r.Context())
	if err != nil {
		return nil
	}
	return out
}

// itemSortFrom parses ?sort= for item pages; unknown values are a 400 so a
// typo never silently falls back to the default order.
func itemSortFrom(r *http.Request) (domain.ItemSort, error) {
	switch v := strings.ToLower(r.URL.Query().Get("sort")); v {
	case "", string(domain.SortByIndex):
		return domain.SortByIndex, nil
	case string(domain.SortByPriceAsc):
		return domain.SortByPriceAsc, nil
	case string(domain.SortByPriceDesc):
		return domain.SortByPriceDesc, nil
	default:
		return "", fmt.Errorf("%w: unknown sort %q (index|price_asc|price_desc)", errBadRequest, v)
	}
}

// listingSortFrom parses ?sort= for listing pages (see itemSortFrom).
func listingSortFrom(r *http.Request) (domain.ListingSort, error) {
	switch v := strings.ToLower(r.URL.Query().Get("sort")); v {
	case "", string(domain.ListingSortNewest):
		return domain.ListingSortNewest, nil
	case string(domain.ListingSortPriceAsc):
		return domain.ListingSortPriceAsc, nil
	case string(domain.ListingSortPriceDesc):
		return domain.ListingSortPriceDesc, nil
	default:
		return "", fmt.Errorf("%w: unknown sort %q (newest|price_asc|price_desc)", errBadRequest, v)
	}
}

// tonConnectChainIDs maps TON Connect CHAIN values to network names.
var tonConnectChainIDs = map[string]string{"-239": "mainnet", "-3": "testnet"}

// checkNetwork rejects proofs produced on a different network than the one
// this API serves; an empty value is accepted for clients that omit it.
func (s *Server) checkNetwork(network string) error {
	if network == "" || s.deps.Network == "" {
		return nil
	}
	name, ok := tonConnectChainIDs[network]
	if !ok {
		name = strings.ToLower(network)
	}
	if name != strings.ToLower(s.deps.Network) {
		return fmt.Errorf("%w: wallet is on %s, this API serves %s", auth.ErrInvalidProof, name, s.deps.Network)
	}
	return nil
}

func decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: read body", errBadRequest)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%w: invalid json: %w", errBadRequest, err)
	}
	return nil
}

func wallet(r *http.Request) (string, error) {
	addr, ok := auth.WalletFromContext(r.Context())
	if !ok {
		return "", auth.ErrInvalidToken
	}
	return addr, nil
}

// ---- auth ----

func (s *Server) authPayload(w http.ResponseWriter, r *http.Request) {
	p, err := s.deps.Payloads.Issue()
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"payload": p})
}

type verifyRequest struct {
	Address   string `json:"address"`
	Network   string `json:"network"`
	PublicKey string `json:"public_key"`
	StateInit string `json:"state_init"`
	Proof     struct {
		Timestamp int64 `json:"timestamp"`
		Domain    struct {
			LengthBytes int    `json:"lengthBytes"`
			Value       string `json:"value"`
		} `json:"domain"`
		Signature string `json:"signature"`
		Payload   string `json:"payload"`
		StateInit string `json:"state_init"`
	} `json:"proof"`
}

func (s *Server) authVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.checkNetwork(req.Network); err != nil {
		writeError(w, r, err)
		return
	}
	stateInit := req.StateInit
	if stateInit == "" {
		stateInit = req.Proof.StateInit
	}
	raw, err := s.deps.Proofs.Verify(r.Context(), req.Address, auth.Proof{
		Timestamp: req.Proof.Timestamp,
		Domain:    auth.Domain{LengthBytes: req.Proof.Domain.LengthBytes, Value: req.Proof.Domain.Value},
		Signature: req.Proof.Signature,
		Payload:   req.Proof.Payload,
		StateInit: stateInit,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	token, exp, err := s.deps.Tokens.Issue(raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": exp.Unix(), "address": s.render.wallet(raw)})
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	addr, err := wallet(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"address": s.render.wallet(addr)})
}

// ---- catalog ----

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	page := pageFrom(r)
	cs, total, err := s.deps.Catalog.Collections(r.Context(), page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]collectionDTO, 0, len(cs))
	for _, c := range cs {
		out = append(out, s.render.collection(c))
	}
	writeJSON(w, http.StatusOK, newPage(out, page, total))
}

func (s *Server) getCollection(w http.ResponseWriter, r *http.Request) {
	raw, err := s.rawAddr(chi.URLParam(r, "addr"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	c, err := s.deps.Catalog.Collection(r.Context(), raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.render.collection(*c))
}

func (s *Server) listCollectionItems(w http.ResponseWriter, r *http.Request) {
	raw, err := s.rawAddr(chi.URLParam(r, "addr"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	sort, err := itemSortFrom(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page := pageFrom(r)
	items, total, err := s.deps.Catalog.CollectionItems(r.Context(), raw, sort, page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPage(s.render.items(items, s.names(r)), page, total))
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	raw, err := s.rawAddr(chi.URLParam(r, "addr"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	d, err := s.deps.Catalog.Item(r.Context(), raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := itemDetailDTO{Item: s.render.item(domain.ItemWithListing{Item: d.Item, Listing: d.Listing}, s.names(r)), Transactions: make([]txDTO, 0, len(d.History))}
	for _, t := range d.History {
		out.Transactions = append(out.Transactions, s.render.tx(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listUserItems(w http.ResponseWriter, r *http.Request) {
	raw, err := s.rawAddr(chi.URLParam(r, "addr"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	page := pageFrom(r)
	items, total, err := s.deps.Catalog.UserItems(r.Context(), raw, page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPage(s.render.items(items, s.names(r)), page, total))
}

func (s *Server) listListings(w http.ResponseWriter, r *http.Request) {
	status := domain.ListingStatus(strings.ToLower(r.URL.Query().Get("status")))
	if status == "" {
		status = domain.ListingActive
	}
	switch status {
	case domain.ListingPending, domain.ListingActive, domain.ListingSold, domain.ListingCancelled:
	default:
		writeError(w, r, fmt.Errorf("%w: unknown status %q", errBadRequest, status))
		return
	}
	sort, err := listingSortFrom(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	page := pageFrom(r)
	items, total, err := s.deps.Catalog.Listings(r.Context(), status, sort, page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newPage(s.render.listingsWithItems(items, s.names(r)), page, total))
}

func listingID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: invalid listing id", errBadRequest)
	}
	return id, nil
}

func (s *Server) getListing(w http.ResponseWriter, r *http.Request) {
	id, err := listingID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	it, err := s.deps.Catalog.Listing(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.render.listingWithItem(*it, s.names(r)))
}

// ---- intents ----

type listingIntentRequest struct {
	NFTAddress string `json:"nft_address"`
	PriceNano  string `json:"price_nano"` // string to survive JS number precision
}

func (s *Server) listingIntent(w http.ResponseWriter, r *http.Request) {
	seller, err := wallet(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req listingIntentRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	nft, err := s.rawAddr(req.NFTAddress)
	if err != nil {
		writeError(w, r, err)
		return
	}
	price, err := strconv.ParseInt(strings.TrimSpace(req.PriceNano), 10, 64)
	if err != nil {
		writeError(w, r, fmt.Errorf("%w: price_nano must be an integer string", errBadRequest))
		return
	}
	res, err := s.deps.Listings.CreateListingIntent(r.Context(), seller, nft, price)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.render.intent(*res))
}

func (s *Server) buyIntent(w http.ResponseWriter, r *http.Request) {
	buyer, err := wallet(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	id, err := listingID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	res, err := s.deps.Listings.BuyIntent(r.Context(), buyer, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.render.intent(*res))
}

func (s *Server) cancelIntent(w http.ResponseWriter, r *http.Request) {
	seller, err := wallet(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	id, err := listingID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	res, err := s.deps.Listings.CancelIntent(r.Context(), seller, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.render.intent(*res))
}

// ---- transactions ----

type registerTxRequest struct {
	BOC        string `json:"boc"`
	Type       string `json:"type"`
	NFTAddress string `json:"nft_address"`
	ListingID  *int64 `json:"listing_id"`
}

func (s *Server) registerTx(w http.ResponseWriter, r *http.Request) {
	sender, err := wallet(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req registerTxRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	in := domain.RegisterInput{Sender: sender, BOC: req.BOC, Type: domain.TxType(strings.ToLower(req.Type)), ListingID: req.ListingID}
	if req.NFTAddress != "" {
		if in.NFTAddress, err = s.rawAddr(req.NFTAddress); err != nil {
			writeError(w, r, err)
			return
		}
	}
	tx, err := s.deps.Txs.Register(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s.render.tx(*tx))
}

func (s *Server) getTx(w http.ResponseWriter, r *http.Request) {
	hash := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "hash")))
	if len(hash) != 64 {
		writeError(w, r, fmt.Errorf("%w: hash must be 64 hex chars", errBadRequest))
		return
	}
	tx, err := s.deps.Txs.Get(r.Context(), hash)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.render.tx(*tx))
}
