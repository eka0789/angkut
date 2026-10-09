// Runner migrasi minimal di atas pgx.
//
// Sengaja tidak memakai ORM/CLI berat: hanya butuh daftar berkas SQL berurutan,
// satu tabel pelacak, dan penguncian agar dua proses tidak berlomba.
//
// Perilaku:
//   - Migrasi dijalankan berurutan menurut nomor versi.
//   - Setiap migrasi berjalan dalam SATU transaksi (up mau pun down), sehingga
//     kegagalan tidak meninggalkan skema setengah jadi.
//   - Advisory lock Postgres mencegah dua instance menerapkan migrasi bersamaan.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey adalah kunci advisory lock khusus migrasi Angkut.
const lockKey int64 = 918273645

var filePattern = regexp.MustCompile(`^(\d{4})_(.+)\.(up|down)\.sql$`)

// Migration adalah satu versi migrasi.
type Migration struct {
	Version int
	Name    string
	UpSQL   string
	DownSQL string
}

// Runner menerapkan migrasi ke satu database.
type Runner struct {
	pool *pgxpool.Pool
	migs []Migration
}

// New memuat seluruh migrasi dari filesystem yang diberikan (biasanya
// migrations.FS) dan menyiapkan runner.
func New(pool *pgxpool.Pool, fsys fs.FS) (*Runner, error) {
	migs, err := load(fsys)
	if err != nil {
		return nil, err
	}
	return &Runner{pool: pool, migs: migs}, nil
}

func load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("membaca direktori migrasi: %w", err)
	}

	byVersion := make(map[int]*Migration)
	names := make(map[int]string)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := filePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		version, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("versi tidak valid pada %s", e.Name())
		}
		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("membaca %s: %w", e.Name(), err)
		}

		cur := byVersion[version]
		if cur == nil {
			cur = &Migration{Version: version}
			byVersion[version] = cur
			names[version] = m[2]
		}
		if names[version] != m[2] {
			return nil, fmt.Errorf("nama migrasi tidak konsisten untuk versi %04d: %q vs %q",
				version, names[version], m[2])
		}
		switch m[3] {
		case "up":
			cur.UpSQL = string(body)
		case "down":
			cur.DownSQL = string(body)
		}
	}

	out := make([]Migration, 0, len(byVersion))
	for v, mg := range byVersion {
		mg.Name = names[v]
		if strings.TrimSpace(mg.UpSQL) == "" {
			return nil, fmt.Errorf("migrasi %04d_%s tidak punya file .up.sql", v, mg.Name)
		}
		if strings.TrimSpace(mg.DownSQL) == "" {
			return nil, fmt.Errorf("migrasi %04d_%s tidak punya file .down.sql (rollback tidak mungkin)", v, mg.Name)
		}
		out = append(out, *mg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// ensureTable membuat tabel pelacak bila belum ada.
func (r *Runner) ensureTable(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    int PRIMARY KEY,
			name       text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	return err
}

// applied mengembalikan versi yang sudah diterapkan.
func (r *Runner) applied(ctx context.Context, tx pgx.Tx) (map[int]bool, error) {
	rows, err := tx.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// Current mengembalikan versi tertinggi yang sudah diterapkan (0 bila kosong).
func (r *Runner) Current(ctx context.Context) (int, error) {
	if err := r.ensureTable(ctx); err != nil {
		return 0, err
	}
	var v *int
	err := r.pool.QueryRow(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&v)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, nil
	}
	return *v, nil
}

// Up menerapkan semua migrasi yang belum dijalankan. Mengembalikan jumlah yang
// diterapkan.
func (r *Runner) Up(ctx context.Context) (int, error) {
	if err := r.ensureTable(ctx); err != nil {
		return 0, err
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return 0, fmt.Errorf("mengambil advisory lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey) }()

	count := 0
	for _, m := range r.migs {
		done := false
		err := func() error {
			tx, err := conn.Begin(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback(ctx) }()

			applied, err := r.applied(ctx, tx)
			if err != nil {
				return err
			}
			if applied[m.Version] {
				done = true
				return nil
			}
			if _, err := tx.Exec(ctx, m.UpSQL); err != nil {
				return fmt.Errorf("menerapkan %04d_%s: %w", m.Version, m.Name, err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
				m.Version, m.Name); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}()
		if err != nil {
			return count, err
		}
		if !done {
			count++
		}
	}
	return count, nil
}

// Down membatalkan sejumlah n migrasi terakhir (default 1).
func (r *Runner) Down(ctx context.Context, n int) (int, error) {
	if n <= 0 {
		n = 1
	}
	if err := r.ensureTable(ctx); err != nil {
		return 0, err
	}

	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return 0, fmt.Errorf("mengambil advisory lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockKey) }()

	reverted := 0
	for reverted < n {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return reverted, err
		}

		applied, err := r.applied(ctx, tx)
		if err != nil {
			_ = tx.Rollback(ctx)
			return reverted, err
		}
		target := r.highest(applied)
		if target == nil {
			_ = tx.Rollback(ctx)
			break
		}
		if _, err := tx.Exec(ctx, target.DownSQL); err != nil {
			_ = tx.Rollback(ctx)
			return reverted, fmt.Errorf("membatalkan %04d_%s: %w", target.Version, target.Name, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, target.Version); err != nil {
			_ = tx.Rollback(ctx)
			return reverted, err
		}
		if err := tx.Commit(ctx); err != nil {
			return reverted, err
		}
		reverted++
	}
	return reverted, nil
}

// highest mengembalikan migrasi terpasang dengan versi tertinggi.
func (r *Runner) highest(applied map[int]bool) *Migration {
	var best *Migration
	for i := range r.migs {
		m := &r.migs[i]
		if applied[m.Version] && (best == nil || m.Version > best.Version) {
			best = m
		}
	}
	return best
}

// Pending mengembalikan versi yang belum diterapkan.
func (r *Runner) Pending(ctx context.Context) ([]int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	applied, err := r.applied(ctx, tx)
	if err != nil {
		return nil, err
	}
	var out []int
	for _, m := range r.migs {
		if !applied[m.Version] {
			out = append(out, m.Version)
		}
	}
	return out, nil
}

// ErrNoDatabase dikembalikan bila DSN kosong.
var ErrNoDatabase = errors.New("DATABASE_URL kosong")