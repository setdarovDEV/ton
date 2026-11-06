package auth

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey struct{}

// WalletFromContext returns the authenticated wallet raw address, if any.
func WalletFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKey{}).(string)
	return v, ok && v != ""
}

// Middleware authenticates `Authorization: Bearer <jwt>` requests. onError
// renders the 401 response so transport error formatting stays in one place.
func Middleware(tokens *TokenIssuer, onError func(w http.ResponseWriter, r *http.Request, err error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				onError(w, r, ErrInvalidToken)
				return
			}
			addr, err := tokens.Verify(strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")))
			if err != nil {
				onError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, addr)))
		})
	}
}
