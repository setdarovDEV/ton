// Package auth implements TON Connect ton_proof verification and JWT session
// tokens. The flow: the frontend asks for a payload, passes it to the wallet,
// the wallet signs a ton_proof over (address, domain, timestamp, payload), the
// backend verifies the signature against the wallet's public key and issues a
// JWT bound to the wallet address. Trusting a bare address is never enough.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// PayloadIssuer creates and validates stateless ton_proof payloads:
// hex(nonce[8] ‖ expiry[8]) ‖ hex(hmac(secret, nonce‖expiry))[:16].
// Stateless means no DB round-trip and horizontal scaling of the API.
type PayloadIssuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewPayloadIssuer creates an issuer; ttl bounds payload validity.
func NewPayloadIssuer(secret []byte, ttl time.Duration) *PayloadIssuer {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &PayloadIssuer{secret: secret, ttl: ttl, now: time.Now}
}

// Issue returns a fresh payload for the wallet to sign.
func (p *PayloadIssuer) Issue() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf[:8]); err != nil {
		return "", fmt.Errorf("random nonce: %w", err)
	}
	exp := p.now().Add(p.ttl).Unix()
	binary.BigEndian.PutUint64(buf[8:], uint64(exp)) //nolint:gosec // unix time is positive
	return hex.EncodeToString(buf) + p.mac(buf), nil
}

// Validate checks the payload's MAC and expiry.
func (p *PayloadIssuer) Validate(payload string) error {
	if len(payload) != 32+16 {
		return fmt.Errorf("payload has wrong length")
	}
	buf, err := hex.DecodeString(payload[:32])
	if err != nil {
		return fmt.Errorf("payload is not hex: %w", err)
	}
	if !hmac.Equal([]byte(p.mac(buf)), []byte(payload[32:])) {
		return fmt.Errorf("payload signature mismatch")
	}
	exp := int64(binary.BigEndian.Uint64(buf[8:])) //nolint:gosec // written by us
	if p.now().Unix() > exp {
		return fmt.Errorf("payload expired")
	}
	return nil
}

func (p *PayloadIssuer) mac(buf []byte) string {
	m := hmac.New(sha256.New, p.secret)
	m.Write(buf)
	return hex.EncodeToString(m.Sum(nil))[:16]
}
