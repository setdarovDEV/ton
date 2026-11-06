package ton

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"

	"tonnet/internal/chain/ton/codes"
	"tonnet/internal/domain"
)

// Getgems op-codes and the gas budget used in the buy / list / cancel flows.
// Values are deliberately explicit so the README can explain the fee model.
const (
	opNFTTransfer     = 0x5fcc3d14 // TEP-62 transfer
	opMarketDeploy    = 1          // nft-marketplace-v2: deploy signed sale
	opSaleAcceptCoins = 1          // fixprice-v3: "just accept coins" (deploy body)
	opSaleBuy         = 2          // fixprice-v3: buy with query_id
	opSaleCancel      = 3          // fixprice-v3: cancel

	// DeployAmountNano is forwarded through the marketplace to the new sale
	// contract (mode 64 carries the remaining value) and pays its storage.
	DeployAmountNano int64 = 50_000_000 // 0.05 TON
	// TransferAmountNano is attached to the NFT transfer; the item keeps
	// min_tons_for_storage (0.05) and forwards TransferForwardNano to the sale.
	TransferAmountNano  int64 = 100_000_000 // 0.10 TON
	TransferForwardNano int64 = 50_000_000  // 0.05 TON, carries ownership_assigned
	// SaleMinGasNano is min_gas_amount() in nft-fixprice-sale-v3r3.fc; the
	// buyer must send price + at least this much. Excess is returned by the
	// contract (mode 128 on the NFT transfer + response_destination = buyer).
	SaleMinGasNano int64 = 100_000_000 // 0.10 TON
	BuyGasNano     int64 = 150_000_000 // 0.15 TON: min gas + margin, excess refunded
	CancelGasNano  int64 = 120_000_000 // 0.12 TON
)

// PayloadBuilder implements domain.PayloadBuilder for Getgems fixed-price
// sales deployed through the nft-marketplace-v2 contract, which only accepts
// deploy requests signed by the marketplace key held by this backend.
type PayloadBuilder struct {
	marketplace    *address.Address
	marketplaceFee *address.Address
	signer         ed25519.PrivateKey
	saleCode       *cell.Cell
	testnet        bool
	now            func() time.Time
}

var _ domain.PayloadBuilder = (*PayloadBuilder)(nil)

// PayloadConfig configures the builder.
type PayloadConfig struct {
	MarketplaceAddress    string
	MarketplaceFeeAddress string // defaults to MarketplaceAddress
	// MarketplacePrivateKey is the 32-byte ed25519 seed (hex) whose public key
	// is stored in the deployed marketplace contract.
	MarketplacePrivateKey string
	Testnet               bool
}

// NewPayloadBuilder validates the configuration and loads the sale code.
func NewPayloadBuilder(cfg PayloadConfig) (*PayloadBuilder, error) {
	market, err := parseStd(cfg.MarketplaceAddress)
	if err != nil {
		return nil, fmt.Errorf("marketplace address: %w", err)
	}
	feeAddr := market
	if cfg.MarketplaceFeeAddress != "" {
		if feeAddr, err = parseStd(cfg.MarketplaceFeeAddress); err != nil {
			return nil, fmt.Errorf("marketplace fee address: %w", err)
		}
	}
	seed, err := hex.DecodeString(strings.TrimPrefix(cfg.MarketplacePrivateKey, "0x"))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("marketplace private key must be a 32-byte hex seed")
	}
	code, err := codes.Load("NftFixpriceSaleV3R3")
	if err != nil {
		return nil, fmt.Errorf("load sale code: %w", err)
	}
	return &PayloadBuilder{
		marketplace:    market,
		marketplaceFee: feeAddr,
		signer:         ed25519.NewKeyFromSeed(seed),
		saleCode:       code,
		testnet:        cfg.Testnet,
		now:            time.Now,
	}, nil
}

// PublicKey returns the marketplace public key (to compare with the deployed contract).
func (b *PayloadBuilder) PublicKey() ed25519.PublicKey {
	return b.signer.Public().(ed25519.PublicKey) //nolint:forcetypeassert // ed25519 always returns PublicKey
}

func (b *PayloadBuilder) friendly(a *address.Address) string {
	return a.Testnet(b.testnet).Bounce(true).String()
}

// saleStateInit builds the fixprice-v3r3 initial data; nft_owner is addr_none
// until the NFT's ownership_assigned message initialises the sale.
func (b *PayloadBuilder) saleStateInit(t domain.SaleTerms, nft *address.Address) (*cell.Cell, error) {
	var royaltyAddr *address.Address
	royaltyNano := int64(0)
	if t.RoyaltyAddress != "" && t.RoyaltyNano > 0 {
		a, err := parseStd(t.RoyaltyAddress)
		if err != nil {
			return nil, fmt.Errorf("royalty address: %w", err)
		}
		royaltyAddr, royaltyNano = a, t.RoyaltyNano
	}
	fees := cell.BeginCell().
		MustStoreAddr(b.marketplaceFee).
		MustStoreCoins(uint64(t.MarketplaceFeeNano)). //nolint:gosec // validated non-negative by domain
		MustStoreAddr(royaltyAddr).
		MustStoreCoins(uint64(royaltyNano)). //nolint:gosec // non-negative
		EndCell()
	data := cell.BeginCell().
		MustStoreUInt(0, 1).                       // is_complete
		MustStoreUInt(uint64(b.now().Unix()), 32). //nolint:gosec // created_at
		MustStoreAddr(b.marketplace).
		MustStoreAddr(nft).
		MustStoreAddr(nil).                  // nft_owner_address = addr_none
		MustStoreCoins(uint64(t.PriceNano)). //nolint:gosec // validated positive
		MustStoreRef(fees).
		MustStoreUInt(0, 32). // sold_at
		MustStoreUInt(0, 64). // query_id
		EndCell()
	si := tlb.StateInit{Code: b.saleCode, Data: data}
	c, err := si.ToCell()
	if err != nil {
		return nil, fmt.Errorf("state init: %w", err)
	}
	return c, nil
}

// BuildListing implements domain.PayloadBuilder.
func (b *PayloadBuilder) BuildListing(t domain.SaleTerms) (*domain.ListingIntent, error) {
	nft, err := parseStd(t.NFTAddress)
	if err != nil {
		return nil, fmt.Errorf("nft address: %w", err)
	}
	seller, err := parseStd(t.Seller)
	if err != nil {
		return nil, fmt.Errorf("seller address: %w", err)
	}
	stateInit, err := b.saleStateInit(t, nft)
	if err != nil {
		return nil, err
	}
	saleAddr := address.NewAddress(0, 0, stateInit.Hash())

	// Message 1: wallet -> marketplace. op=1, signature(512) over
	// hash(cell{^state_init ^body}), then the two refs. The marketplace
	// verifies the signature with its stored public key and deploys the sale.
	saleBody := cell.BeginCell().MustStoreUInt(opSaleAcceptCoins, 32).MustStoreUInt(t.QueryID, 64).EndCell()
	signed := cell.BeginCell().MustStoreRef(stateInit).MustStoreRef(saleBody).EndCell()
	sig := ed25519.Sign(b.signer, signed.Hash())
	deploy := cell.BeginCell().
		MustStoreUInt(opMarketDeploy, 32).
		MustStoreSlice(sig, 512).
		MustStoreRef(stateInit).
		MustStoreRef(saleBody).
		EndCell()

	// Message 2: wallet -> NFT item. TEP-62 transfer to the sale contract with
	// a forward amount so the item emits ownership_assigned to the sale, which
	// records the seller as nft_owner and makes the listing active.
	transfer := cell.BeginCell().
		MustStoreUInt(opNFTTransfer, 32).
		MustStoreUInt(t.QueryID, 64).
		MustStoreAddr(saleAddr).                     // new_owner
		MustStoreAddr(seller).                       // response_destination (excess back to seller)
		MustStoreBoolBit(false).                     // no custom_payload
		MustStoreCoins(uint64(TransferForwardNano)). //nolint:gosec // constant
		MustStoreBoolBit(false).                     // forward_payload inline, empty
		EndCell()

	return &domain.ListingIntent{
		SaleContract: strings.ToLower(saleAddr.StringRaw()),
		Messages: []domain.Message{
			{Address: b.friendly(b.marketplace), AmountNano: DeployAmountNano, Payload: toBase64(deploy)},
			{Address: b.friendly(nft), AmountNano: TransferAmountNano, Payload: toBase64(transfer)},
		},
	}, nil
}

// BuildBuy implements domain.PayloadBuilder.
func (b *PayloadBuilder) BuildBuy(saleContract string, priceNano int64, queryID uint64) (domain.Message, error) {
	sale, err := parseStd(saleContract)
	if err != nil {
		return domain.Message{}, fmt.Errorf("sale address: %w", err)
	}
	body := cell.BeginCell().MustStoreUInt(opSaleBuy, 32).MustStoreUInt(queryID, 64).EndCell()
	return domain.Message{Address: b.friendly(sale), AmountNano: priceNano + BuyGasNano, Payload: toBase64(body)}, nil
}

// BuildCancel implements domain.PayloadBuilder.
func (b *PayloadBuilder) BuildCancel(saleContract string, queryID uint64) (domain.Message, error) {
	sale, err := parseStd(saleContract)
	if err != nil {
		return domain.Message{}, fmt.Errorf("sale address: %w", err)
	}
	body := cell.BeginCell().MustStoreUInt(opSaleCancel, 32).MustStoreUInt(queryID, 64).EndCell()
	return domain.Message{Address: b.friendly(sale), AmountNano: CancelGasNano, Payload: toBase64(body)}, nil
}

// MessageHash implements domain.PayloadBuilder: the hash of the external
// message cell returned by TON Connect, which tonapi indexes transactions by.
func (b *PayloadBuilder) MessageHash(boc string) (string, error) {
	return MessageHash(boc)
}

// MessageHash returns the lowercase hex hash of a base64 BOC root cell.
func MessageHash(boc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(boc)
	if err != nil {
		return "", fmt.Errorf("decode boc base64: %w", err)
	}
	c, err := cell.FromBOC(raw)
	if err != nil {
		return "", fmt.Errorf("parse boc: %w", err)
	}
	return hex.EncodeToString(c.Hash()), nil
}

func toBase64(c *cell.Cell) string {
	return base64.StdEncoding.EncodeToString(c.ToBOC())
}
