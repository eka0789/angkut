# Migrations — Angkut

SQL migrations untuk golang-migrate. Setiap migrasi **wajib** punya pasangan
`up` dan `down`; tanpa `down` yang benar, rollback produksi menjadi tidak
mungkin.

## Konvensi

| Aturan | Detail |
|--------|--------|
| Nama | `NNNN_nama.up.sql` + `NNNN_nama.down.sql` (4 digit, urut) |
| Transaksi | golang-migrate menjalankan per-file; jangan menaruh `BEGIN`/`COMMIT` manual |
| UUID | Di-generate application layer (UUID v7), bukan `DEFAULT gen_random_uuid()` |
| Tenant | Setiap tabel domain punya `organization_id` + FK ke `organizations` |
| Uang | `bigint` rupiah utuh. Berat/dimensi: `numeric`. Waktu: `timestamptz` (UTC) |
| Append-only | `audit_logs`, `inventory_movements`, `ledger_entries`, `cod_transactions`, `tracking_events` |
| Idempotensi | UNIQUE natural key (mis. `(shipment_id, status, occurred_at, source)`) |

## Daftar

| # | File | Isi |
|---|------|-----|
| 0001 | `0001_extensions` | `pgcrypto`, `pg_trgm`, `unaccent`, fungsi `set_updated_at()` |
| 0002 | `0002_users_sessions` | `users`, `user_sessions` (refresh token di-hash) |
| 0003 | `0003_rbac` | `roles`, `permissions`, `role_permissions` + seed 6 role |
| 0004 | `0004_plans_subscriptions` | `plans`, `subscriptions`, `usage_counters` + seed 5 paket |
| 0005 | `0005_organizations_members` | `organizations`, `organization_members` (FK tenant lahir di sini) |
| 0006 | `0006_audit_outbox_apikeys` | `audit_logs`, `outbox_events`, `api_keys` |

Migrasi 0007+ (domain logistik: products, inventory, orders, shipments, dst)
menyusul pada fase terkait sesuai `planning/database.md` §6.

## Menjalankan

```bash
make migrate-up                       # terapkan semua
make migrate-down                     # batalkan 1 terakhir
make migrate-create name=create_x     # buat pasangan baru
```

Atau langsung lewat binary migrate bawaan repo (tanpa perlu CLI migrate):

```bash
go run ./api/cmd/migrate up
DATABASE_URL=... go run ./api/cmd/migrate status
```

## Menambah migrasi

1. `make migrate-create name=nama_singkat`
2. Tulis `up` (idempoten bila masuk akal: `IF NOT EXISTS`) dan `down` (benar-benar membalik).
3. Uji naik-turun di DB lokal: `up` → `down 1` → `up` lagi.
4. Update tabel di atas dan `dev-docs/ai/CURRENT_STATE.md`.