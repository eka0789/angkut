# AGENTS.md — repo apps/

Panduan kerja AI di dalam repo `apps/`. Aturan global ada di `../dev-docs/ai/AGENTS.md` dan `../ai-rules/`.

## Git

- **Repo ini adalah `apps/`.** Semua perintah `git` dijalankan dari `apps/`, bukan dari root workspace.
- Branch: `main` (protected) · `feat/*` · `fix/*` · `chore/*`.
- Commit: [Conventional Commits](https://www.conventionalcommits.org/) — `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`, `ci:`. Pesan dalam bahasa Indonesia atau Inggris, konsisten.
- Jangan commit `.env`, `bin/`, `node_modules/`, secret apa pun.

## Struktur & boundary

```text
api/internal/domain/         entity + aturan bisnis (TANPA I/O)
api/internal/application/    use case + transaction boundary
api/internal/infrastructure/ adapter (postgres, redis, rabbitmq, courier, ...)
api/internal/interfaces/     HTTP handler, middleware, DTO, WS, webhook
api/internal/platform/       util murni (uuid, money, timeutil, validate)
```

Aturan batas (wajib):

| Boleh impor | Tidak boleh |
|-------------|-------------|
| `interfaces` → `application` → `domain` | `domain` impor `application`/`interfaces`/`infrastructure` |
| `infrastructure` → `domain` (port) + `platform` | `domain` impor driver DB/vendor |
| `application` → `domain` + port | `application` panggil `interfaces` |

- Modul lintas akses lewat **application service / port**, bukan query tabel modul lain.
- Kode eksternal (vendor) hanya di `infrastructure/`, selalu dengan adapter `mock`.

## Ukuran file (anti-monster)

| Artefak | Batas |
|---------|-------|
| `router.go` | ≤ 200 baris |
| handler | ≤ 1000 baris |
| service | ≤ 800 baris |
| entity | ≤ 300 baris |

Melampaui batas → split, jangan biarkan tumbuh.

## Konvensi

- Konfigurasi: semua harga/bobot/divisor dari config/DB, **bukan** hardcode.
- Logging: `log/slog` JSON dengan `request_id`, `trace_id`, `user_id`, `organization_id`, `module`.
- Error: pakai typed error `domain/shared/errors.go`; handler memetakan ke kode HTTP via `interfaces/http/response/error_map.go`.
- `organization_id` SELALU dari context (middleware tenant), tidak pernah dari body/query.
- Setiap mutasi kritis: idempotency key + transaksi + outbox event.
- Migrasi: `NNNN_nama.up.sql` + `NNNN_nama.down.sql`; setiap `up` wajib punya `down` yang benar.

## Sebelum selesai

```bash
go build ./... && go vet ./... && go test ./...
```

Lalu update dokumen terkait (lihat `../dev-docs/ai/MAINTENANCE` di AGENTS global): `TASKS.md`, `CURRENT_STATE.md`, `CHANGELOG.md`.