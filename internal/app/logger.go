// Package app holds the composition root shared by both binaries: logging,
// config-driven construction of the chain client and database.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"tonnet/internal/chain/ton"
	"tonnet/internal/config"
	"tonnet/internal/repo/postgres"
)

// NewLogger builds a slog logger from config.
func NewLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(cfg.LogLevel))); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.EqualFold(cfg.LogFormat, "text") {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

// NewChainClient builds the TON client from config.
func NewChainClient(cfg *config.Config, log *slog.Logger) *ton.Client {
	return ton.NewClient(ton.Config{BaseURL: cfg.TonAPIURL, APIKey: cfg.TonAPIKey, RPS: cfg.TonAPIRPS, Timeout: cfg.ChainCallTO}, log)
}

// OpenDatabase migrates and connects.
func OpenDatabase(ctx context.Context, cfg *config.Config, log *slog.Logger) (*pgxpool.Pool, error) {
	if err := postgres.Migrate(cfg.DatabaseURL); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	log.Info("database migrated")
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return pool, nil
}
