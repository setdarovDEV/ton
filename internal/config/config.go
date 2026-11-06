// Package config loads process configuration from the environment. Both
// binaries share one struct so docker-compose can pass the same env block.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the full environment configuration.
type Config struct {
	// Shared
	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat   string `env:"LOG_FORMAT" envDefault:"json"` // json | text

	// Chain
	TONNetwork  string        `env:"TON_NETWORK" envDefault:"testnet"` // testnet | mainnet
	TonAPIURL   string        `env:"TONAPI_BASE_URL" envDefault:"https://testnet.tonapi.io"`
	TonAPIKey   string        `env:"TONAPI_KEY"`
	TonAPIRPS   float64       `env:"TONAPI_RPS" envDefault:"1"`
	Collections []string      `env:"COLLECTIONS" envSeparator:","`
	ChainCallTO time.Duration `env:"CHAIN_CALL_TIMEOUT" envDefault:"15s"`

	// Marketplace (sale deploy signing)
	MarketplaceAddress    string `env:"MARKETPLACE_ADDRESS"`
	MarketplacePrivateKey string `env:"MARKETPLACE_PRIVATE_KEY"`
	MarketplaceFeeAddress string `env:"MARKETPLACE_FEE_ADDRESS"`
	MarketplaceFeeBPS     int64  `env:"MARKETPLACE_FEE_BPS" envDefault:"500"`

	// API
	HTTPAddr        string        `env:"HTTP_ADDR" envDefault:":8080"`
	CORSOrigins     []string      `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:5173,http://localhost:3000"`
	JWTSecret       string        `env:"JWT_SECRET"`
	JWTTTL          time.Duration `env:"JWT_TTL" envDefault:"24h"`
	TonProofDomains []string      `env:"TON_PROOF_DOMAINS" envSeparator:"," envDefault:"localhost:5173,localhost:3000"`
	TxPendingTTL    time.Duration `env:"TX_PENDING_TTL" envDefault:"10m"`

	// Indexer
	IndexerInterval   time.Duration `env:"INDEXER_INTERVAL" envDefault:"4s"`
	ItemScanInterval  time.Duration `env:"ITEM_SCAN_INTERVAL" envDefault:"60s"`
	PendingListingTTL time.Duration `env:"PENDING_LISTING_TTL" envDefault:"15m"`
}

// Load parses the environment.
func Load() (*Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	c.Collections = trimAll(c.Collections)
	c.CORSOrigins = trimAll(c.CORSOrigins)
	c.TonProofDomains = trimAll(c.TonProofDomains)
	return &c, nil
}

// IsTestnet reports whether addresses should be rendered with the testnet flag.
func (c *Config) IsTestnet() bool { return strings.ToLower(c.TONNetwork) != "mainnet" }

// ValidateAPI checks the settings the API process cannot run without.
func (c *Config) ValidateAPI() error {
	if len(c.JWTSecret) < 16 {
		return fmt.Errorf("JWT_SECRET must be at least 16 characters")
	}
	if c.MarketplaceAddress == "" || c.MarketplacePrivateKey == "" {
		return fmt.Errorf("MARKETPLACE_ADDRESS and MARKETPLACE_PRIVATE_KEY are required")
	}
	if c.MarketplaceFeeBPS < 0 || c.MarketplaceFeeBPS > 5000 {
		return fmt.Errorf("MARKETPLACE_FEE_BPS must be within [0, 5000]")
	}
	return nil
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
