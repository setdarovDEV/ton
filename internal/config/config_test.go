package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("COLLECTIONS", " 0:aa , 0:bb ,")
	t.Setenv("JWT_SECRET", "0123456789abcdef")
	t.Setenv("MARKETPLACE_ADDRESS", "0:cc")
	t.Setenv("MARKETPLACE_PRIVATE_KEY", "dd")
	c, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"0:aa", "0:bb"}, c.Collections)
	assert.True(t, c.IsTestnet())
	assert.NoError(t, c.ValidateAPI())

	c.JWTSecret = "short"
	assert.Error(t, c.ValidateAPI())
	c.JWTSecret = "0123456789abcdef"
	c.MarketplaceFeeBPS = 9999
	assert.Error(t, c.ValidateAPI())
	c.MarketplaceFeeBPS = 0
	c.MarketplaceAddress = ""
	assert.Error(t, c.ValidateAPI())
	c.TONNetwork = "mainnet"
	assert.False(t, c.IsTestnet())
}

func TestLoadRequiresDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	assert.Error(t, err)
}
