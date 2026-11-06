package ton

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"tonnet/internal/domain"
)

const (
	testMarket = "0:1111111111111111111111111111111111111111111111111111111111111111"
	testNFT    = "0:2222222222222222222222222222222222222222222222222222222222222222"
	testSeller = "0:3333333333333333333333333333333333333333333333333333333333333333"
	testRoyal  = "0:4444444444444444444444444444444444444444444444444444444444444444"
)

func newTestBuilder(t *testing.T) *PayloadBuilder {
	t.Helper()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	b, err := NewPayloadBuilder(PayloadConfig{
		MarketplaceAddress:    testMarket,
		MarketplacePrivateKey: hex.EncodeToString(seed),
		Testnet:               true,
	})
	require.NoError(t, err)
	b.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	return b
}

func decode(t *testing.T, b64 string) *cell.Slice {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	c, err := cell.FromBOC(raw)
	require.NoError(t, err)
	s, err := c.BeginParse()
	require.NoError(t, err)
	return s
}

func TestNewPayloadBuilderValidation(t *testing.T) {
	_, err := NewPayloadBuilder(PayloadConfig{MarketplaceAddress: "bad", MarketplacePrivateKey: "00"})
	assert.Error(t, err)
	_, err = NewPayloadBuilder(PayloadConfig{MarketplaceAddress: testMarket, MarketplacePrivateKey: "00"})
	assert.Error(t, err)
	_, err = NewPayloadBuilder(PayloadConfig{MarketplaceAddress: testMarket, MarketplaceFeeAddress: "x", MarketplacePrivateKey: hex.EncodeToString(make([]byte, 32))})
	assert.Error(t, err)
}

func TestBuildListing(t *testing.T) {
	b := newTestBuilder(t)
	terms := domain.SaleTerms{
		NFTAddress:         testNFT,
		Seller:             testSeller,
		PriceNano:          5_000_000_000,
		MarketplaceFeeNano: 250_000_000,
		Royalty:            domain.RoyaltyParams{Numerator: 100_000_000, Denominator: 1, Address: testRoyal},
		QueryID:            42,
	}
	intent, err := b.BuildListing(terms)
	require.NoError(t, err)
	require.Len(t, intent.Messages, 2)

	// Deterministic: same terms, same sale address.
	again, err := b.BuildListing(terms)
	require.NoError(t, err)
	assert.Equal(t, intent.SaleContract, again.SaleContract)

	// Message 1: deploy through the marketplace with a valid signature.
	deploy := intent.Messages[0]
	assert.Equal(t, DeployAmountNano, deploy.AmountNano)
	assert.Equal(t, testMarket, mustRaw(t, deploy.Address))
	s := decode(t, deploy.Payload)
	op, err := s.LoadUInt(32)
	require.NoError(t, err)
	assert.EqualValues(t, opMarketDeploy, op)
	sig, err := s.LoadSlice(512)
	require.NoError(t, err)
	stateInit, err := s.LoadRef()
	require.NoError(t, err)
	body, err := s.LoadRef()
	require.NoError(t, err)
	siCell := stateInit.MustToCell()
	bodyCell := body.MustToCell()
	signed := cell.BeginCell().MustStoreRef(siCell).MustStoreRef(bodyCell).EndCell()
	assert.True(t, ed25519.Verify(b.PublicKey(), signed.Hash(), sig), "marketplace signature must verify")

	// The sale address is derived from the state init in workchain 0.
	var si tlb.StateInit
	require.NoError(t, tlb.LoadFromCell(&si, siCell.MustBeginParse()))
	assert.Equal(t, intent.SaleContract, mustRaw(t, si.CalcAddress(0).String()))

	// Sale data layout matches nft-fixprice-sale-v3r3.fc load_data().
	ds := si.Data.MustBeginParse()
	isComplete, _ := ds.LoadUInt(1)
	assert.EqualValues(t, 0, isComplete)
	createdAt, _ := ds.LoadUInt(32)
	assert.EqualValues(t, 1_700_000_000, createdAt)
	market, _ := ds.LoadAddr()
	assert.Equal(t, testMarket, mustRaw(t, market.String()))
	nft, _ := ds.LoadAddr()
	assert.Equal(t, testNFT, mustRaw(t, nft.String()))
	owner, _ := ds.LoadAddr()
	assert.True(t, owner.IsAddrNone())
	price, _ := ds.LoadCoins()
	assert.EqualValues(t, terms.PriceNano, price)
	fees, _ := ds.LoadRef()
	feeAddr, _ := fees.LoadAddr()
	assert.Equal(t, testMarket, mustRaw(t, feeAddr.String()), "fee address defaults to marketplace")
	fee, _ := fees.LoadCoins()
	assert.EqualValues(t, 250_000_000, fee)
	royaltyAddr, _ := fees.LoadAddr()
	assert.Equal(t, testRoyal, mustRaw(t, royaltyAddr.String()))
	royalty, _ := fees.LoadCoins()
	assert.EqualValues(t, 100_000_000, royalty)

	// Sale deploy body: op 1 + query id.
	bop, _ := body.LoadUInt(32)
	assert.EqualValues(t, opSaleAcceptCoins, bop)
	qid, _ := body.LoadUInt(64)
	assert.EqualValues(t, 42, qid)

	// Message 2: TEP-62 transfer of the NFT to the sale contract.
	transfer := intent.Messages[1]
	assert.Equal(t, TransferAmountNano, transfer.AmountNano)
	assert.Equal(t, testNFT, mustRaw(t, transfer.Address))
	ts := decode(t, transfer.Payload)
	top, _ := ts.LoadUInt(32)
	assert.EqualValues(t, opNFTTransfer, top)
	tq, _ := ts.LoadUInt(64)
	assert.EqualValues(t, 42, tq)
	newOwner, _ := ts.LoadAddr()
	assert.Equal(t, intent.SaleContract, mustRaw(t, newOwner.String()))
	resp, _ := ts.LoadAddr()
	assert.Equal(t, testSeller, mustRaw(t, resp.String()))
	custom, _ := ts.LoadBoolBit()
	assert.False(t, custom)
	fwd, _ := ts.LoadCoins()
	assert.EqualValues(t, TransferForwardNano, fwd)
}

func TestBuildListingWithoutRoyalty(t *testing.T) {
	b := newTestBuilder(t)
	intent, err := b.BuildListing(domain.SaleTerms{NFTAddress: testNFT, Seller: testSeller, PriceNano: 1_000_000_000})
	require.NoError(t, err)
	s := decode(t, intent.Messages[0].Payload)
	_, _ = s.LoadUInt(32)
	_, _ = s.LoadSlice(512)
	stateInit, _ := s.LoadRef()
	var si tlb.StateInit
	require.NoError(t, tlb.LoadFromCell(&si, stateInit))
	ds := si.Data.MustBeginParse()
	_, _ = ds.LoadUInt(1)
	_, _ = ds.LoadUInt(32)
	_, _ = ds.LoadAddr()
	_, _ = ds.LoadAddr()
	_, _ = ds.LoadAddr()
	_, _ = ds.LoadCoins()
	fees, _ := ds.LoadRef()
	_, _ = fees.LoadAddr()
	_, _ = fees.LoadCoins()
	royaltyAddr, _ := fees.LoadAddr()
	assert.True(t, royaltyAddr.IsAddrNone())

	_, err = b.BuildListing(domain.SaleTerms{NFTAddress: "bad", Seller: testSeller, PriceNano: 1})
	assert.Error(t, err)
	_, err = b.BuildListing(domain.SaleTerms{NFTAddress: testNFT, Seller: "bad", PriceNano: 1})
	assert.Error(t, err)
}

func TestBuildBuyAndCancel(t *testing.T) {
	b := newTestBuilder(t)
	sale := "0:5555555555555555555555555555555555555555555555555555555555555555"
	buy, err := b.BuildBuy(sale, 2_000_000_000, 7)
	require.NoError(t, err)
	assert.Equal(t, 2_000_000_000+BuyGasNano, buy.AmountNano)
	assert.Equal(t, sale, mustRaw(t, buy.Address))
	s := decode(t, buy.Payload)
	op, _ := s.LoadUInt(32)
	assert.EqualValues(t, opSaleBuy, op)
	q, _ := s.LoadUInt(64)
	assert.EqualValues(t, 7, q)

	cancel, err := b.BuildCancel(sale, 9)
	require.NoError(t, err)
	assert.Equal(t, CancelGasNano, cancel.AmountNano)
	s = decode(t, cancel.Payload)
	op, _ = s.LoadUInt(32)
	assert.EqualValues(t, opSaleCancel, op)

	_, err = b.BuildBuy("bad", 1, 1)
	assert.Error(t, err)
	_, err = b.BuildCancel("bad", 1)
	assert.Error(t, err)
}

func TestMessageHash(t *testing.T) {
	c := cell.BeginCell().MustStoreUInt(0xdeadbeef, 32).EndCell()
	h, err := MessageHash(base64.StdEncoding.EncodeToString(c.ToBOC()))
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(c.Hash()), h)
	_, err = MessageHash("!!!")
	assert.Error(t, err)
	_, err = MessageHash(base64.StdEncoding.EncodeToString([]byte("nope")))
	assert.Error(t, err)
}

func mustRaw(t *testing.T, s string) string {
	t.Helper()
	raw, err := NormalizeRaw(s)
	require.NoError(t, err)
	return raw
}

var _ = address.NewAddressNone
