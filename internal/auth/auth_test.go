package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

// fakeWallet mimics a wallet v4 whose state init we can hash into an address.
type fakeWallet struct {
	pub       ed25519.PublicKey
	priv      ed25519.PrivateKey
	addr      *address.Address
	stateInit string
}

func newFakeWallet(t *testing.T, v5 bool) fakeWallet {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	code := cell.BeginCell().MustStoreUInt(0xC0DE, 16).EndCell()
	var data *cell.Cell
	if v5 {
		data = cell.BeginCell().MustStoreBoolBit(true).MustStoreUInt(0, 32).MustStoreUInt(2147483409, 32).MustStoreSlice(pub, 256).MustStoreBoolBit(false).EndCell()
	} else {
		data = cell.BeginCell().MustStoreUInt(0, 32).MustStoreUInt(698983191, 32).MustStoreSlice(pub, 256).MustStoreBoolBit(false).EndCell()
	}
	si, err := tlb.StateInit{Code: code, Data: data}.ToCell()
	require.NoError(t, err)
	return fakeWallet{pub: pub, priv: priv, addr: address.NewAddress(0, 0, si.Hash()), stateInit: base64.StdEncoding.EncodeToString(si.ToBOC())}
}

func (w fakeWallet) sign(domain string, ts int64, payload string) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(w.priv, ProofMessage(w.addr, domain, ts, payload)))
}

type fakeKeys struct {
	key []byte
	err error
}

func (f fakeKeys) GetPublicKey(context.Context, string) ([]byte, error) { return f.key, f.err }

func TestPayloadIssuer(t *testing.T) {
	p := NewPayloadIssuer([]byte("s"), time.Minute)
	pl, err := p.Issue()
	require.NoError(t, err)
	assert.NoError(t, p.Validate(pl))
	assert.Error(t, p.Validate(pl[:len(pl)-1]+"0"))
	assert.Error(t, p.Validate("short"))
	assert.Error(t, p.Validate("zz"+pl[2:]))
	other := NewPayloadIssuer([]byte("other"), time.Minute)
	assert.Error(t, other.Validate(pl))
	p.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	assert.Error(t, p.Validate(pl))
}

func TestVerifyProof(t *testing.T) {
	for _, v5 := range []bool{false, true} {
		w := newFakeWallet(t, v5)
		payloads := NewPayloadIssuer([]byte("secret"), time.Minute)
		v := NewVerifier(payloads, nil, []string{"Localhost:5173", " demo.example.com "}, 5*time.Minute)
		payload, err := payloads.Issue()
		require.NoError(t, err)
		ts := time.Now().Unix()
		proof := Proof{Timestamp: ts, Domain: Domain{LengthBytes: 14, Value: "localhost:5173"}, Payload: payload, StateInit: w.stateInit, Signature: w.sign("localhost:5173", ts, payload)}

		got, err := v.Verify(context.Background(), w.addr.String(), proof)
		require.NoError(t, err, "v5=%v", v5)
		assert.Equal(t, w.addr.StringRaw(), got)

		// Also accepts the raw form of the address.
		_, err = v.Verify(context.Background(), w.addr.StringRaw(), proof)
		require.NoError(t, err)

		bad := proof
		bad.Domain.Value = "evil.example.com"
		bad.Domain.LengthBytes = len(bad.Domain.Value)
		_, err = v.Verify(context.Background(), w.addr.String(), bad)
		assert.ErrorIs(t, err, ErrInvalidProof)

		bad = proof
		bad.Domain.LengthBytes = 3
		_, err = v.Verify(context.Background(), w.addr.String(), bad)
		assert.ErrorIs(t, err, ErrInvalidProof)

		bad = proof
		bad.Timestamp = ts - 3600
		bad.Signature = w.sign("localhost:5173", bad.Timestamp, payload)
		_, err = v.Verify(context.Background(), w.addr.String(), bad)
		assert.ErrorIs(t, err, ErrInvalidProof)

		bad = proof
		bad.Payload = payload[:len(payload)-1] + "f"
		_, err = v.Verify(context.Background(), w.addr.String(), bad)
		assert.ErrorIs(t, err, ErrInvalidProof)

		bad = proof
		bad.Signature = base64.StdEncoding.EncodeToString(make([]byte, 64))
		_, err = v.Verify(context.Background(), w.addr.String(), bad)
		assert.ErrorIs(t, err, ErrInvalidProof)

		bad = proof
		bad.Signature = "%%%"
		_, err = v.Verify(context.Background(), w.addr.String(), bad)
		assert.ErrorIs(t, err, ErrInvalidProof)

		// Someone else's address with this state init: hash mismatch.
		other := newFakeWallet(t, v5)
		_, err = v.Verify(context.Background(), other.addr.String(), proof)
		assert.ErrorIs(t, err, ErrInvalidProof)

		_, err = v.Verify(context.Background(), "not-an-address", proof)
		assert.ErrorIs(t, err, ErrInvalidProof)
	}
}

func TestVerifyFallsBackToChainKey(t *testing.T) {
	w := newFakeWallet(t, false)
	payloads := NewPayloadIssuer([]byte("secret"), time.Minute)
	payload, _ := payloads.Issue()
	ts := time.Now().Unix()
	proof := Proof{Timestamp: ts, Domain: Domain{LengthBytes: 7, Value: "app.dev"}, Payload: payload, Signature: w.sign("app.dev", ts, payload)}

	v := NewVerifier(payloads, fakeKeys{key: w.pub}, []string{"app.dev"}, 0)
	got, err := v.Verify(context.Background(), w.addr.String(), proof)
	require.NoError(t, err)
	assert.Equal(t, w.addr.StringRaw(), got)

	v = NewVerifier(payloads, fakeKeys{err: errors.New("offline")}, []string{"app.dev"}, 0)
	_, err = v.Verify(context.Background(), w.addr.String(), proof)
	assert.ErrorIs(t, err, ErrInvalidProof)

	v = NewVerifier(payloads, fakeKeys{key: []byte{1, 2}}, []string{"app.dev"}, 0)
	_, err = v.Verify(context.Background(), w.addr.String(), proof)
	assert.ErrorIs(t, err, ErrInvalidProof)

	v = NewVerifier(payloads, nil, []string{"app.dev"}, 0)
	_, err = v.Verify(context.Background(), w.addr.String(), proof)
	assert.ErrorIs(t, err, ErrInvalidProof)

	// Bad state init with a resolver falls back to chain.
	proof.StateInit = "@@@"
	v = NewVerifier(payloads, fakeKeys{key: w.pub}, []string{"app.dev"}, 0)
	_, err = v.Verify(context.Background(), w.addr.String(), proof)
	require.NoError(t, err)
}

func TestPublicKeyFromStateInitErrors(t *testing.T) {
	w := newFakeWallet(t, false)
	_, err := PublicKeyFromStateInit(w.addr, "###")
	assert.Error(t, err)
	_, err = PublicKeyFromStateInit(w.addr, base64.StdEncoding.EncodeToString([]byte("junk")))
	assert.Error(t, err)
	// state init with unsupported data layout
	data := cell.BeginCell().MustStoreUInt(1, 8).EndCell()
	si, _ := tlb.StateInit{Code: cell.BeginCell().EndCell(), Data: data}.ToCell()
	addr := address.NewAddress(0, 0, si.Hash())
	_, err = PublicKeyFromStateInit(addr, base64.StdEncoding.EncodeToString(si.ToBOC()))
	assert.Error(t, err)
	// no data cell
	si, _ = tlb.StateInit{Code: cell.BeginCell().EndCell()}.ToCell()
	addr = address.NewAddress(0, 0, si.Hash())
	_, err = PublicKeyFromStateInit(addr, base64.StdEncoding.EncodeToString(si.ToBOC()))
	assert.Error(t, err)
}

func TestTokens(t *testing.T) {
	ti := NewTokenIssuer([]byte("jwt-secret"), time.Hour)
	tok, exp, err := ti.Issue("0:abc")
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(time.Hour), exp, 5*time.Second)
	sub, err := ti.Verify(tok)
	require.NoError(t, err)
	assert.Equal(t, "0:abc", sub)

	_, err = NewTokenIssuer([]byte("other"), time.Hour).Verify(tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
	_, err = ti.Verify("garbage")
	assert.ErrorIs(t, err, ErrInvalidToken)

	expired := NewTokenIssuer([]byte("jwt-secret"), time.Hour)
	expired.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	_, err = expired.Verify(tok)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestMiddleware(t *testing.T) {
	ti := NewTokenIssuer([]byte("s"), time.Hour)
	tok, _, _ := ti.Issue("0:wallet")
	var gotErr error
	mw := Middleware(ti, func(w http.ResponseWriter, _ *http.Request, err error) {
		gotErr = err
		w.WriteHeader(http.StatusUnauthorized)
	})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr, ok := WalletFromContext(r.Context())
		assert.True(t, ok)
		_, _ = w.Write([]byte(addr))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "0:wallet", rec.Body.String())

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.ErrorIs(t, gotErr, ErrInvalidToken)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer nope")
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	_, ok := WalletFromContext(context.Background())
	assert.False(t, ok)
}
