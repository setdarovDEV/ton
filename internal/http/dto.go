package http

import (
	"encoding/json"
	"time"

	"tonnet/internal/chain/ton"
	"tonnet/internal/domain"
)

// Addr renders an address in both forms: user-friendly for display/wallets
// and raw for stable identity.
type Addr struct {
	Address string `json:"address"`
	Raw     string `json:"raw"`
}

// addrFormatter knows the network (testnet flag) for user-friendly rendering.
type addrFormatter struct{ testnet bool }

func (f addrFormatter) wallet(raw string) *Addr {
	if raw == "" {
		return nil
	}
	return &Addr{Address: ton.ToFriendly(raw, f.testnet, false), Raw: raw}
}

func (f addrFormatter) contract(raw string) *Addr {
	if raw == "" {
		return nil
	}
	return &Addr{Address: ton.ToFriendly(raw, f.testnet, true), Raw: raw}
}

type collectionDTO struct {
	Address       Addr            `json:"address"`
	Owner         *Addr           `json:"owner"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	ImageURL      string          `json:"image_url"`
	Metadata      json.RawMessage `json:"metadata"`
	NextItemIndex int64           `json:"next_item_index"`
	ItemsCount    int64           `json:"items_count"`
	IndexedAt     time.Time       `json:"indexed_at"`
}

// collectionRefDTO is the compact collection reference embedded in items.
type collectionRefDTO struct {
	Address Addr   `json:"address"`
	Name    string `json:"name"`
}

// itemSummaryDTO is the compact item embedded in listing pages (Explore).
type itemSummaryDTO struct {
	Address    Addr              `json:"address"`
	Index      int64             `json:"index"`
	Name       string            `json:"name"`
	ImageURL   string            `json:"image_url"`
	Collection *collectionRefDTO `json:"collection"`
}

type listingDTO struct {
	ID           int64     `json:"id"`
	NFTAddress   Addr      `json:"nft_address"`
	SaleContract Addr      `json:"sale_contract"`
	Seller       Addr      `json:"seller"`
	Buyer        *Addr     `json:"buyer"`
	PriceNano    string    `json:"price_nano"`
	PriceTON     string    `json:"price_ton"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Item is embedded on listing pages so Explore renders without extra calls.
	Item *itemSummaryDTO `json:"item,omitempty"`
}

type itemDTO struct {
	Address           Addr              `json:"address"`
	CollectionAddress Addr              `json:"collection_address"`
	Collection        *collectionRefDTO `json:"collection"`
	Index             int64             `json:"index"`
	Owner             *Addr             `json:"owner"`
	Name              string            `json:"name"`
	ImageURL          string            `json:"image_url"`
	Metadata          json.RawMessage   `json:"metadata"`
	UpdatedAt         time.Time         `json:"updated_at"`
	Listing           *listingDTO       `json:"listing"`
}

type txDTO struct {
	Hash       string     `json:"hash"`
	Type       string     `json:"type"`
	NFTAddress *Addr      `json:"nft_address"`
	ListingID  *int64     `json:"listing_id"`
	From       *Addr      `json:"from"`
	To         *Addr      `json:"to"`
	AmountNano string     `json:"amount_nano"`
	LT         uint64     `json:"lt"`
	Status     string     `json:"status"`
	ExpiresAt  *time.Time `json:"expires_at"`
	SeenAt     time.Time  `json:"seen_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type itemDetailDTO struct {
	Item         itemDTO `json:"item"`
	Transactions []txDTO `json:"transactions"`
}

type pageDTO[T any] struct {
	Items []T `json:"items"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
}

func newPage[T any](items []T, page domain.Page, total int) pageDTO[T] {
	return pageDTO[T]{Items: items, Page: page.Number, Limit: page.Size, Total: total}
}

type messageDTO struct {
	Address string `json:"address"`
	Amount  string `json:"amount"` // nanotons as string (TON Connect format)
	Payload string `json:"payload,omitempty"`
}

type intentDTO struct {
	ListingID   int64        `json:"listing_id"`
	SaleAddress Addr         `json:"sale_address"`
	Listing     listingDTO   `json:"listing"`
	Messages    []messageDTO `json:"messages"`
	ValidUntil  int64        `json:"valid_until"` // unix seconds for TON Connect
}

func (f addrFormatter) collection(c domain.Collection) collectionDTO {
	return collectionDTO{Address: *f.contract(c.Address), Owner: f.wallet(c.Owner), Name: c.Name, Description: c.Description, ImageURL: c.ImageURL, Metadata: nonNil(c.Metadata), NextItemIndex: c.NextItemIndex, ItemsCount: c.ItemsCount, IndexedAt: c.IndexedAt}
}

func (f addrFormatter) listing(l domain.Listing) listingDTO {
	return listingDTO{ID: l.ID, NFTAddress: *f.contract(l.NFTAddress), SaleContract: *f.contract(l.SaleContract), Seller: *f.wallet(l.Seller), Buyer: f.wallet(l.Buyer),
		PriceNano: itoa(l.PriceNano), PriceTON: domain.FormatTON(l.PriceNano), Status: string(l.Status), CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt}
}

// collectionNames resolves collection address -> display name for embedding
// into items; nil map simply omits the names.
type collectionNames map[string]string

func (f addrFormatter) collectionRef(raw string, names collectionNames) *collectionRefDTO {
	if raw == "" {
		return nil
	}
	return &collectionRefDTO{Address: *f.contract(raw), Name: names[raw]}
}

func (f addrFormatter) itemSummary(it domain.NFTItem, names collectionNames) *itemSummaryDTO {
	return &itemSummaryDTO{Address: *f.contract(it.Address), Index: it.Index, Name: it.Name, ImageURL: it.ImageURL, Collection: f.collectionRef(it.CollectionAddress, names)}
}

func (f addrFormatter) item(it domain.ItemWithListing, names collectionNames) itemDTO {
	d := itemDTO{Address: *f.contract(it.Item.Address), CollectionAddress: *f.contract(it.Item.CollectionAddress), Collection: f.collectionRef(it.Item.CollectionAddress, names),
		Index: it.Item.Index, Owner: f.wallet(it.Item.Owner), Name: it.Item.Name, ImageURL: it.Item.ImageURL, Metadata: nonNil(it.Item.Metadata), UpdatedAt: it.Item.UpdatedAt}
	if it.Listing != nil {
		l := f.listing(*it.Listing)
		d.Listing = &l
	}
	return d
}

func (f addrFormatter) items(in []domain.ItemWithListing, names collectionNames) []itemDTO {
	out := make([]itemDTO, 0, len(in))
	for _, it := range in {
		out = append(out, f.item(it, names))
	}
	return out
}

// listingWithItem renders a listing page row: the listing plus its item summary.
func (f addrFormatter) listingWithItem(it domain.ItemWithListing, names collectionNames) listingDTO {
	l := f.listing(*it.Listing)
	l.Item = f.itemSummary(it.Item, names)
	return l
}

func (f addrFormatter) listingsWithItems(in []domain.ItemWithListing, names collectionNames) []listingDTO {
	out := make([]listingDTO, 0, len(in))
	for _, it := range in {
		if it.Listing == nil {
			continue
		}
		out = append(out, f.listingWithItem(it, names))
	}
	return out
}

func (f addrFormatter) tx(t domain.Transaction) txDTO {
	return txDTO{Hash: t.Hash, Type: string(t.Type), NFTAddress: f.contract(t.NFTAddress), ListingID: t.ListingID, From: f.wallet(t.From), To: f.wallet(t.To),
		AmountNano: itoa(t.AmountNano), LT: t.LT, Status: string(t.Status), ExpiresAt: t.ExpiresAt, SeenAt: t.SeenAt, UpdatedAt: t.UpdatedAt}
}

func (f addrFormatter) intent(r domain.IntentResult) intentDTO {
	out := intentDTO{ListingID: r.Listing.ID, SaleAddress: *f.contract(r.Listing.SaleContract), Listing: f.listing(r.Listing), ValidUntil: r.ValidUntil.Unix(), Messages: make([]messageDTO, 0, len(r.Messages))}
	for _, m := range r.Messages {
		out.Messages = append(out.Messages, messageDTO{Address: m.Address, Amount: itoa(m.AmountNano), Payload: m.Payload})
	}
	return out
}

func nonNil(m json.RawMessage) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage("{}")
	}
	return m
}
