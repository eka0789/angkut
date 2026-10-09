// Package database menyediakan pool, transaksi, dan implementasi repository.
package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TxManager menjalankan fungsi dalam satu transaksi database.
//
// Boundary transaksi ada di application layer (aturan proyek), bukan di handler
// dan bukan di repository. Repository menerima `pgx.Tx` atau pool melalui
// Querier sehingga dapat ikut serta dalam transaksi yang sama.
type TxManager struct {
	pool *pgxpool.Pool
}

// NewTxManager membuat manager atas pool tertentu.
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// Querier adalah irisan operasi yang dimiliki bersama *pgxpool.Pool dan pgx.Tx,
// sehingga repository dapat dipakai baik di dalam maupun di luar transaksi.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InTx menjalankan fn dalam transaksi. Rollback otomatis bila fn error atau
// panic; commit hanya bila fn sukses.
func (m *TxManager) InTx(ctx context.Context, fn func(q Querier) error) error {
	return m.inTx(ctx, fn)
}

// InTx menjalankan fn dalam transaksi pada pool mana pun.
func (m *TxManager) inTx(ctx context.Context, fn func(q Querier) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memulai transaksi: %w", err)
	}

	defer func() {
		// Rollback setelah commit adalah no-op yang aman.
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaksi: %w", err)
	}
	return nil
}

// Pool mengembalikan pool dasar (untuk health check).
func (m *TxManager) Pool() *pgxpool.Pool { return m.pool }

// TxFromContext / WithTx tidak dipakai: transaksi selalu diberikan sebagai
// argumen eksplisit agar alur data terlihat di tanda tangan fungsi.

// ErrNoRows diterjemahkan dari pgx.ErrNoRows supaya pemanggil tidak perlu
// mengimpor pgx hanya untuk memeriksa "tidak ada baris".
var ErrNoRows = pgx.ErrNoRows

// IsNoRows melaporkan apakah error berarti "tidak ada baris".
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
