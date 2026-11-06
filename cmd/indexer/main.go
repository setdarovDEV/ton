// Command indexer polls the chain and writes collections, items, listings and
// transactions into PostgreSQL. It is a separate process from the API so each
// can be scaled and restarted independently.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"tonnet/internal/app"
	"tonnet/internal/chain/ton"
	"tonnet/internal/config"
	"tonnet/internal/indexer"
	"tonnet/internal/repo/postgres"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("indexer exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := app.NewLogger(cfg).With("service", "indexer")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := app.OpenDatabase(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	seeds := make([]string, 0, len(cfg.Collections))
	for _, c := range cfg.Collections {
		raw, err := ton.NormalizeRaw(c)
		if err != nil {
			log.Warn("skipping invalid collection address", "address", c, "err", err)
			continue
		}
		seeds = append(seeds, raw)
	}

	svc := indexer.New(app.NewChainClient(cfg, log), postgres.NewStore(pool), indexer.Config{
		Collections:       seeds,
		Interval:          cfg.IndexerInterval,
		ItemScanInterval:  cfg.ItemScanInterval,
		PendingListingTTL: cfg.PendingListingTTL,
	}, log, nil)
	return svc.Run(ctx)
}
