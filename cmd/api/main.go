// Command api serves the marketplace REST/WebSocket API. It only reads from
// PostgreSQL (fast, scalable) and prepares wallet messages; the indexer process
// writes chain state.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tonnet/internal/app"
	"tonnet/internal/auth"
	"tonnet/internal/chain/ton"
	"tonnet/internal/config"
	"tonnet/internal/domain"
	apihttp "tonnet/internal/http"
	"tonnet/internal/repo/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.ValidateAPI(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := app.NewLogger(cfg).With("service", "api")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := app.OpenDatabase(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := postgres.NewStore(pool)
	chainClient := app.NewChainClient(cfg, log)

	payloads, err := ton.NewPayloadBuilder(ton.PayloadConfig{
		MarketplaceAddress:    cfg.MarketplaceAddress,
		MarketplaceFeeAddress: cfg.MarketplaceFeeAddress,
		MarketplacePrivateKey: cfg.MarketplacePrivateKey,
		Testnet:               cfg.IsTestnet(),
	})
	if err != nil {
		return fmt.Errorf("payload builder: %w", err)
	}
	log.Info("marketplace signer ready", "marketplace", cfg.MarketplaceAddress, "public_key", fmt.Sprintf("%x", payloads.PublicKey()))

	secret := []byte(cfg.JWTSecret)
	payloadIssuer := auth.NewPayloadIssuer(secret, 15*time.Minute)
	hub := apihttp.NewHub(cfg.CORSOrigins, log)
	server := apihttp.NewServer(apihttp.Deps{
		Catalog:  domain.NewCatalogService(store, store, store, store),
		Listings: domain.NewListingService(store, store, payloads, royaltyAdapter{chainClient}, domain.MarketplaceConfig{FeeBPS: cfg.MarketplaceFeeBPS}, nil),
		Txs:      domain.NewTxService(store, store, payloads, nil, cfg.TxPendingTTL),
		Payloads: payloadIssuer,
		Proofs:   auth.NewVerifier(payloadIssuer, chainClient, cfg.TonProofDomains, 15*time.Minute),
		Tokens:   auth.NewTokenIssuer(secret, cfg.JWTTTL),
		Hub:      hub,
		Testnet:  cfg.IsTestnet(),
		Origins:  cfg.CORSOrigins,
		Log:      log,
		Health: func() error {
			c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return pool.Ping(c)
		},
	})

	go postgres.NewListener(pool, log).Run(ctx, hub.Broadcast)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "network", cfg.TONNetwork)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// royaltyAdapter maps chain.RoyaltyParams onto the domain port.
type royaltyAdapter struct{ c *ton.Client }

func (a royaltyAdapter) GetRoyaltyParams(ctx context.Context, collection string) (*domain.RoyaltyParams, error) {
	r, err := a.c.GetRoyaltyParams(ctx, collection)
	if err != nil {
		return nil, err
	}
	return &domain.RoyaltyParams{Numerator: r.Numerator, Denominator: r.Denominator, Address: r.Address}, nil
}
