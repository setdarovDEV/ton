package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"tonnet/internal/domain"
)

const txCols = `hash, type, COALESCE(nft_address,''), listing_id, COALESCE(from_addr,''), COALESCE(to_addr,''), amount_nano, lt, status, COALESCE(boc,''), expires_at, seen_at, updated_at`

func scanTx(row pgx.Row) (*domain.Transaction, error) {
	var t domain.Transaction
	var lt int64
	if err := row.Scan(&t.Hash, &t.Type, &t.NFTAddress, &t.ListingID, &t.From, &t.To, &t.AmountNano, &lt, &t.Status, &t.BOC, &t.ExpiresAt, &t.SeenAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.LT = uint64(lt) //nolint:gosec // lt is non-negative
	return &t, nil
}

// UpsertTransaction implements domain.TransactionRepository. On conflict the
// row is only refreshed with chain facts (lt, status if not pending); user
// supplied fields are kept, which makes the indexer idempotent.
func (s *Store) UpsertTransaction(ctx context.Context, t domain.Transaction) error {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO transactions (hash, type, nft_address, listing_id, from_addr, to_addr, amount_nano, lt, status, boc, expires_at, seen_at, updated_at)
		VALUES ($1, $2, NULLIF($3,''), $4, NULLIF($5,''), NULLIF($6,''), $7, $8, $9, NULLIF($10,''), $11, COALESCE($12, now()), now())
		ON CONFLICT (hash) DO UPDATE SET
			lt = GREATEST(transactions.lt, EXCLUDED.lt),
			status = CASE WHEN transactions.status = 'pending' THEN EXCLUDED.status ELSE transactions.status END,
			listing_id = COALESCE(transactions.listing_id, EXCLUDED.listing_id),
			nft_address = COALESCE(transactions.nft_address, EXCLUDED.nft_address),
			to_addr = COALESCE(transactions.to_addr, EXCLUDED.to_addr),
			updated_at = now()`,
		t.Hash, t.Type, t.NFTAddress, t.ListingID, t.From, t.To, t.AmountNano, int64(t.LT), t.Status, t.BOC, t.ExpiresAt, nullTime(t.SeenAt)) //nolint:gosec // lt fits
	if err != nil {
		return fmt.Errorf("upsert tx %s: %w", t.Hash, err)
	}
	return nil
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// GetTransaction implements domain.TransactionRepository.
func (s *Store) GetTransaction(ctx context.Context, hash string) (*domain.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	t, err := scanTx(s.pool.QueryRow(ctx, `SELECT `+txCols+` FROM transactions WHERE hash = $1`, hash))
	if err != nil {
		return nil, fmt.Errorf("get tx %s: %w", hash, mapErr(err))
	}
	return t, nil
}

func (s *Store) queryTxs(ctx context.Context, sql string, args ...any) ([]domain.Transaction, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Transaction{}
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tx: %w", err)
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ListTransactionsByNFT implements domain.TransactionRepository.
func (s *Store) ListTransactionsByNFT(ctx context.Context, nft string, limit int) ([]domain.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	txs, err := s.queryTxs(ctx, `SELECT `+txCols+` FROM transactions WHERE nft_address = $1 ORDER BY lt DESC, seen_at DESC LIMIT $2`, nft, limit)
	if err != nil {
		return nil, fmt.Errorf("list txs for %s: %w", nft, err)
	}
	return txs, nil
}

// ListPendingTransactions implements domain.TransactionRepository.
func (s *Store) ListPendingTransactions(ctx context.Context) ([]domain.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	txs, err := s.queryTxs(ctx, `SELECT `+txCols+` FROM transactions WHERE status = 'pending' ORDER BY seen_at`)
	if err != nil {
		return nil, fmt.Errorf("list pending txs: %w", err)
	}
	return txs, nil
}

// UpdateTransactionStatus implements domain.TransactionRepository: the status
// change and the tx.updated notification are committed in one transaction.
func (s *Store) UpdateTransactionStatus(ctx context.Context, hash string, status domain.TxStatus, lt uint64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var nft string
	var listingID *int64
	err = tx.QueryRow(ctx, `UPDATE transactions SET status = $2, lt = GREATEST(lt, $3), updated_at = now()
		WHERE hash = $1 RETURNING COALESCE(nft_address,''), listing_id`, hash, status, int64(lt)).Scan(&nft, &listingID) //nolint:gosec // lt fits
	if err != nil {
		return fmt.Errorf("update tx %s: %w", hash, mapErr(err))
	}
	ev := domain.Event{Type: domain.EventTxUpdated, NFTAddress: nft, TxHash: hash, Status: string(status), At: time.Now()}
	if listingID != nil {
		ev.ListingID = *listingID
	}
	if err := notify(ctx, tx, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ConfirmPendingForNFT implements domain.TransactionRepository.
func (s *Store) ConfirmPendingForNFT(ctx context.Context, nft string, typ domain.TxType) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT hash FROM transactions WHERE nft_address = $1 AND type = $2 AND status = 'pending'`, nft, typ)
	if err != nil {
		return nil, fmt.Errorf("find pending %s txs for %s: %w", typ, nft, err)
	}
	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hash: %w", err)
		}
		hashes = append(hashes, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, h := range hashes {
		if err := s.UpdateTransactionStatus(ctx, h, domain.TxConfirmed, 0); err != nil {
			return hashes, err
		}
	}
	return hashes, nil
}
