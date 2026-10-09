# Angkut — Apps (monorepo)

Monorepo kode Angkut: **Intelligent Logistics OS** untuk bisnis Indonesia.
`api` dan `worker` adalah satu Go module; `web` adalah Nuxt 4.

```
apps/
├── api/            Go API server            (binary: angkut-api)
├── worker/         Go background workers    (binary: angkut-worker)
├── web/            Nuxt 4 frontend
├── migrations/     SQL migrations (golang-migrate)
├── docs/operations systemd/supervisor/cron
├── docker-compose.yml
└── Makefile
```

## Prasyarat

| Tool | Versi |
|------|-------|
| Go | 1.24+ |
| Node.js | 22+ |
| Docker + Docker Compose | untuk Postgres, Redis, RabbitMQ, MinIO, Mailpit |

## Menjalankan (dev)

```bash
cp .env.example .env          # isi bila perlu; default sudah cukup untuk dev
make up                       # docker compose up -d (postgres, redis, rabbitmq, minio, mailpit)
make migrate-up               # terapkan migrasi
make api                      # jalankan API  → http://localhost:8080
make worker                   # jalankan worker (terminal lain)
make web                      # jalankan Nuxt → http://localhost:3000
```

Cek kesehatan: `curl http://localhost:8080/healthz` → `{"status":"ok"}`.

> **Windows tanpa `make`:** setiap target bisa dijalankan langsung —
> `docker compose up -d` · `go run ./api/cmd/migrate up` · `go run ./api/cmd/api` ·
> `go run ./worker/cmd/worker` · `cd web && npm run dev`.

## Perintah

| Perintah | Fungsi |
|----------|--------|
| `make up` / `make down` | Start/stop seluruh stack dependensi (Docker) |
| `make migrate-up` / `make migrate-down` | Terapkan / batalkan migrasi (runner bawaan repo) |
| `make migrate-status` | Versi skema saat ini + migrasi yang belum diterapkan |
| `make migrate-create name=xxx` | Buat pasangan migrasi baru |
| `make api` / `make worker` / `make web` | Jalankan masing-masing service |
| `make test` | Unit test Go (`-race`) |
| `make lint` | golangci-lint + eslint |
| `make build` | Build binary `angkut-api` & `angkut-worker` ke `bin/` |

Runner migrasi tertanam di binary (tidak butuh CLI `migrate` terpasang):
`go run ./api/cmd/migrate up|down [n]|status`.

## Aturan

- **Git repository ada di `apps/`** — jalankan `git` dari direktori ini, bukan dari root workspace.
- Domain tidak boleh mengimpor `interfaces/` atau `infrastructure/`. Lihat `dev-docs/architecture/backend-structure.md` dan `dev-docs/ai/PROJECT_MENTAL_MODEL.md`.
- Tidak ada credential di kode; semua via env (`apps/.env.example` sebagai daftar lengkap).
- Adapter vendor selalu punya `mock`: platform harus berjalan end-to-end tanpa kredensial eksternal.

Dokumen kerja lengkap ada di `../dev-docs/` (di luar repo ini).