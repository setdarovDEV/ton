package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenIssuer issues and verifies HS256 session tokens whose subject is the
// wallet's raw address.
type TokenIssuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
	now    func() time.Time
}

// NewTokenIssuer creates a token issuer.
func NewTokenIssuer(secret []byte, ttl time.Duration) *TokenIssuer {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &TokenIssuer{secret: secret, ttl: ttl, issuer: "tonnet", now: time.Now}
}

// Issue returns a signed token for the wallet address.
func (t *TokenIssuer) Issue(rawAddress string) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(t.ttl)
	claims := jwt.RegisteredClaims{
		Issuer:    t.issuer,
		Subject:   rawAddress,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return s, exp, nil
}

// ErrInvalidToken is returned for any token validation failure.
var ErrInvalidToken = errors.New("invalid token")

// Verify validates the token and returns the wallet raw address.
func (t *TokenIssuer) Verify(token string) (string, error) {
	var claims jwt.RegisteredClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(tok *jwt.Token) (any, error) {
		if tok.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected alg %s", tok.Method.Alg())
		}
		return t.secret, nil
	}, jwt.WithIssuer(t.issuer), jwt.WithTimeFunc(t.now), jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	return claims.Subject, nil
}
