package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

// Proof is the ton_proof item returned by the wallet (TON Connect 2).
type Proof struct {
	Timestamp int64
	Domain    Domain
	Signature string // base64
	Payload   string
	StateInit string // base64 BOC of the wallet state init (walletStateInit)
}

// Domain is the dApp domain the wallet signed for.
type Domain struct {
	LengthBytes int
	Value       string
}

// PublicKeyResolver fetches a wallet's public key from chain (fallback when
// the state init cannot be parsed, e.g. unknown wallet contracts).
type PublicKeyResolver interface {
	GetPublicKey(ctx context.Context, address string) ([]byte, error)
}

// Verifier checks ton_proof signatures.
type Verifier struct {
	payloads *PayloadIssuer
	keys     PublicKeyResolver
	domains  map[string]struct{}
	maxAge   time.Duration
	now      func() time.Time
}

// NewVerifier creates a verifier that accepts proofs for the given domains.
func NewVerifier(payloads *PayloadIssuer, keys PublicKeyResolver, allowedDomains []string, maxAge time.Duration) *Verifier {
	d := make(map[string]struct{}, len(allowedDomains))
	for _, dom := range allowedDomains {
		dom = strings.ToLower(strings.TrimSpace(dom))
		if dom != "" {
			d[dom] = struct{}{}
		}
	}
	if maxAge <= 0 {
		maxAge = 15 * time.Minute
	}
	return &Verifier{payloads: payloads, keys: keys, domains: d, maxAge: maxAge, now: time.Now}
}

// ErrInvalidProof is returned for any verification failure; details are wrapped.
var ErrInvalidProof = errors.New("invalid ton_proof")

// Verify validates the proof for the wallet address (any form) and returns the
// canonical raw address on success.
func (v *Verifier) Verify(ctx context.Context, walletAddr string, proof Proof) (string, error) {
	addr, err := parseAddr(walletAddr)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidProof, err)
	}
	if err := v.payloads.Validate(proof.Payload); err != nil {
		return "", fmt.Errorf("%w: payload: %w", ErrInvalidProof, err)
	}
	if _, ok := v.domains[strings.ToLower(proof.Domain.Value)]; !ok {
		return "", fmt.Errorf("%w: domain %q not allowed", ErrInvalidProof, proof.Domain.Value)
	}
	if proof.Domain.LengthBytes != len(proof.Domain.Value) {
		return "", fmt.Errorf("%w: domain length mismatch", ErrInvalidProof)
	}
	ts := time.Unix(proof.Timestamp, 0)
	if d := v.now().Sub(ts); d > v.maxAge || d < -v.maxAge {
		return "", fmt.Errorf("%w: timestamp out of range", ErrInvalidProof)
	}
	sig, err := base64.StdEncoding.DecodeString(proof.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return "", fmt.Errorf("%w: bad signature encoding", ErrInvalidProof)
	}

	pub, err := v.publicKey(ctx, addr, proof.StateInit)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidProof, err)
	}
	msg := ProofMessage(addr, proof.Domain.Value, proof.Timestamp, proof.Payload)
	if !ed25519.Verify(pub, msg, sig) {
		return "", fmt.Errorf("%w: signature does not verify", ErrInvalidProof)
	}
	return strings.ToLower(addr.StringRaw()), nil
}

// publicKey derives the wallet public key from the provided state init (which
// must hash to the address) and falls back to the chain.
func (v *Verifier) publicKey(ctx context.Context, addr *address.Address, stateInit string) (ed25519.PublicKey, error) {
	if stateInit != "" {
		pub, err := PublicKeyFromStateInit(addr, stateInit)
		if err == nil {
			return pub, nil
		}
		if v.keys == nil {
			return nil, err
		}
	}
	if v.keys == nil {
		return nil, fmt.Errorf("no state init and no key resolver")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pub, err := v.keys.GetPublicKey(ctx, addr.StringRaw())
	if err != nil {
		return nil, fmt.Errorf("resolve public key on chain: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("chain returned %d-byte key", len(pub))
	}
	return pub, nil
}

// ProofMessage builds the digest the wallet signed:
//
//	sha256(0xffff ‖ "ton-connect" ‖ sha256("ton-proof-item-v2/" ‖ wc ‖ hash ‖ len(domain) ‖ domain ‖ ts ‖ payload))
func ProofMessage(addr *address.Address, domain string, timestamp int64, payload string) []byte {
	var buf bytes.Buffer
	buf.WriteString("ton-proof-item-v2/")
	_ = binary.Write(&buf, binary.BigEndian, addr.Workchain())
	buf.Write(addr.Data())
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(domain))) //nolint:gosec // domain is short
	buf.WriteString(domain)
	_ = binary.Write(&buf, binary.LittleEndian, uint64(timestamp)) //nolint:gosec // unix time
	buf.WriteString(payload)
	inner := sha256.Sum256(buf.Bytes())

	var outer bytes.Buffer
	outer.Write([]byte{0xff, 0xff})
	outer.WriteString("ton-connect")
	outer.Write(inner[:])
	digest := sha256.Sum256(outer.Bytes())
	return digest[:]
}

// PublicKeyFromStateInit parses the wallet's public key out of its state init
// after checking that the state init really belongs to the address. Supports
// the data layouts of wallet v3 / v4 (seqno u32, subwallet u32, pubkey) and
// v5r1 (is_signature_allowed bit, seqno u32, wallet_id u32, pubkey).
func PublicKeyFromStateInit(addr *address.Address, stateInitB64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(stateInitB64)
	if err != nil {
		return nil, fmt.Errorf("state init is not base64: %w", err)
	}
	si, err := cell.FromBOC(raw)
	if err != nil {
		return nil, fmt.Errorf("state init is not a BOC: %w", err)
	}
	if !bytes.Equal(si.Hash(), addr.Data()) {
		return nil, fmt.Errorf("state init hash does not match address")
	}
	s := si.MustBeginParse()
	// StateInit: split_depth:(Maybe (## 5)) special:(Maybe TickTock) code:(Maybe ^Cell) data:(Maybe ^Cell) library:(HashmapE 256 SimpleLib)
	if has, _ := s.LoadBoolBit(); has {
		if _, err := s.LoadUInt(5); err != nil {
			return nil, fmt.Errorf("split depth: %w", err)
		}
	}
	if has, _ := s.LoadBoolBit(); has {
		if _, err := s.LoadUInt(2); err != nil {
			return nil, fmt.Errorf("tick tock: %w", err)
		}
	}
	if _, err := s.LoadMaybeRef(); err != nil { // code
		return nil, fmt.Errorf("code ref: %w", err)
	}
	data, err := s.LoadMaybeRef()
	if err != nil || data == nil {
		return nil, fmt.Errorf("state init has no data cell")
	}
	for _, layout := range []func(*cell.Slice) ([]byte, error){pubkeyV3V4, pubkeyV5} {
		d := data.Copy()
		if key, err := layout(d); err == nil {
			return key, nil
		}
	}
	return nil, fmt.Errorf("unsupported wallet data layout")
}

func pubkeyV3V4(d *cell.Slice) ([]byte, error) {
	if _, err := d.LoadUInt(32); err != nil { // seqno
		return nil, err
	}
	if _, err := d.LoadUInt(32); err != nil { // subwallet id
		return nil, err
	}
	key, err := d.LoadSlice(256)
	if err != nil {
		return nil, err
	}
	// v4 continues with plugins dict (1 bit); v3 ends here. Either way the
	// remaining bits must be < 2 to rule out v5 layouts being misread.
	if d.BitsLeft() > 1 {
		return nil, fmt.Errorf("not v3/v4 layout")
	}
	return key, nil
}

func pubkeyV5(d *cell.Slice) ([]byte, error) {
	if _, err := d.LoadBoolBit(); err != nil { // is_signature_allowed
		return nil, err
	}
	if _, err := d.LoadUInt(32); err != nil { // seqno
		return nil, err
	}
	if _, err := d.LoadUInt(32); err != nil { // wallet id
		return nil, err
	}
	key, err := d.LoadSlice(256)
	if err != nil {
		return nil, err
	}
	if d.BitsLeft() > 1 { // extensions dict bit
		return nil, fmt.Errorf("not v5 layout")
	}
	return key, nil
}

func parseAddr(s string) (*address.Address, error) {
	s = strings.TrimSpace(s)
	var a *address.Address
	var err error
	if strings.Contains(s, ":") {
		a, err = address.ParseRawAddr(s)
	} else {
		a, err = address.ParseAddr(s)
	}
	if err != nil {
		return nil, fmt.Errorf("bad wallet address: %w", err)
	}
	if a.Type() != address.StdAddress {
		return nil, fmt.Errorf("wallet address must be a standard address")
	}
	return a, nil
}
