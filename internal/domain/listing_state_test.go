package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	seller = "0:aa"
	sale   = "0:55"
	buyer  = "0:bb"
)

func TestNextListingStatus(t *testing.T) {
	cases := []struct {
		name    string
		status  ListingStatus
		owner   string
		want    ListingStatus
		changed bool
	}{
		{"pending->active when sale owns nft", ListingPending, sale, ListingActive, true},
		{"pending stays while seller owns nft", ListingPending, seller, ListingPending, false},
		{"pending stays with unknown owner", ListingPending, "", ListingPending, false},
		{"pending->cancelled if nft moved elsewhere", ListingPending, buyer, ListingCancelled, true},
		{"active stays while sale owns nft", ListingActive, sale, ListingActive, false},
		{"active->cancelled when back to seller", ListingActive, seller, ListingCancelled, true},
		{"active->sold when third party owns", ListingActive, buyer, ListingSold, true},
		{"active unchanged for empty owner", ListingActive, "", ListingActive, false},
		{"sold is terminal", ListingSold, seller, ListingSold, false},
		{"cancelled is terminal", ListingCancelled, buyer, ListingCancelled, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := Listing{Seller: seller, SaleContract: sale, Status: tc.status}
			got, changed := NextListingStatus(l, tc.owner)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.changed, changed)
		})
	}
}

func TestApplyRecordsBuyer(t *testing.T) {
	l := Listing{Seller: seller, SaleContract: sale, Status: ListingActive}
	got, changed := l.Apply(buyer)
	assert.True(t, changed)
	assert.Equal(t, ListingSold, got.Status)
	assert.Equal(t, buyer, got.Buyer)

	same, changed := got.Apply(seller)
	assert.False(t, changed)
	assert.Equal(t, got, same)
}

func TestTxTypeForOwnerChange(t *testing.T) {
	l := Listing{Seller: seller, SaleContract: sale, Status: ListingActive}
	typ, ok := TxTypeForOwnerChange(l, sale)
	assert.True(t, ok)
	assert.Equal(t, TxList, typ)
	typ, ok = TxTypeForOwnerChange(l, seller)
	assert.True(t, ok)
	assert.Equal(t, TxCancel, typ)
	typ, ok = TxTypeForOwnerChange(l, buyer)
	assert.True(t, ok)
	assert.Equal(t, TxBuy, typ)
	_, ok = TxTypeForOwnerChange(l, "")
	assert.False(t, ok)
}

func TestFormatTON(t *testing.T) {
	assert.Equal(t, "1", FormatTON(1_000_000_000))
	assert.Equal(t, "0.5", FormatTON(500_000_000))
	assert.Equal(t, "12.000000001", FormatTON(12_000_000_001))
	assert.Equal(t, "-0.1", FormatTON(-100_000_000))
	assert.Equal(t, "0", FormatTON(0))
}

func TestIsTerminalAndPage(t *testing.T) {
	assert.True(t, ListingSold.IsTerminal())
	assert.False(t, ListingActive.IsTerminal())
	assert.Equal(t, 0, Page{Number: 0, Size: 20}.Offset())
	assert.Equal(t, 40, Page{Number: 3, Size: 20}.Offset())
}
