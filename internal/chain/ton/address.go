package ton

import (
	"fmt"
	"strings"

	"github.com/xssnick/tonutils-go/address"
)

// NormalizeRaw accepts any TON address form (raw "0:hex", user-friendly
// base64/base64url, with any flags) and returns the canonical lowercase raw form
// used as the storage key everywhere in the system.
func NormalizeRaw(s string) (string, error) {
	a, err := parse(s)
	if err != nil {
		return "", err
	}
	return strings.ToLower(a.StringRaw()), nil
}

// ToFriendly renders a raw address in user-friendly form. Wallet addresses are
// usually shown non-bounceable; contracts bounceable. Testnet flag follows the
// configured network.
func ToFriendly(raw string, testnet, bounceable bool) string {
	a, err := parse(raw)
	if err != nil || a.Type() != address.StdAddress {
		return raw
	}
	return a.Testnet(testnet).Bounce(bounceable).String()
}

func parse(s string) (*address.Address, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty address")
	}
	if strings.Contains(s, ":") {
		a, err := address.ParseRawAddr(s)
		if err != nil {
			return nil, fmt.Errorf("parse raw address %q: %w", s, err)
		}
		return a, nil
	}
	a, err := address.ParseAddr(s)
	if err != nil {
		return nil, fmt.Errorf("parse address %q: %w", s, err)
	}
	return a, nil
}

// mustAddr parses a raw address that is known to be valid (from storage).
func parseStd(raw string) (*address.Address, error) {
	a, err := parse(raw)
	if err != nil {
		return nil, err
	}
	if a.Type() != address.StdAddress {
		return nil, fmt.Errorf("address %q is not a standard address", raw)
	}
	return a, nil
}
