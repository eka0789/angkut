# Operasi Produksi — Angkut

> **PENGECUALIAN:** file operasi sengaja berada DI DALAM repo `apps/` (lihat
> `dev-docs/architecture/backend-structure.md` §1) agar deployment terkontrol
> versi bersama kode.

## Arsitektur Deployment

```
Internet → Cloudflare (TLS, WAF, cache) → Nginx (rate limit edge, proxy)
                                              ├── Nuxt SSR        (127.0.0.1:3000)
                                              └── Go API          (127.0.0.1:8080)
                                                     │
        ┌────────────────────────────────────────────┼──────────────────────────┐
        │                                            │                          │
   PostgreSQL 16                                  Redis 7                  RabbitMQ 3.13
   (data utama)                             (cache, lock, idempotency,    (event, retry, DLQ)
        │                                     rate limit, pub/sub)
        │
   MinIO / S3-compatible (label, export, bukti settlement)
```

Worker berjalan sebagai proses terpisah tanpa port publik; hanya membuka
`:9090` internal untuk probe `/healthz` dan `/metrics`.

## Berkas di Folder Ini

| Berkas | Fungsi |
|--------|--------|
| `angkut-api.service` | Unit systemd API |
| `angkut-worker.service` | Unit systemd worker |
| `angkut-web.service` | Unit systemd Nuxt SSR |
| `nginx.conf.example` | Virtual host Nginx (proxy, rate limit, SSE/WS) |
| `backup.sh` | Backup harian Postgres + retensi |
| `deploy.md` | Prosedur rilis langkah demi langkah |

## Langkah Pasang (ringkas)

1. **Siapkan host** — jalankan PostgreSQL, Redis, RabbitMQ, MinIO (terpisah dari repo).
2. **Buat user & direktori**
   ```bash
   sudo useradd --system --home /opt/angkut --shell /usr/sbin/nologin angkut
   sudo mkdir -p /opt/angkut/{bin,storage,web}
   ```
3. **Build & kirim artefak**
   ```bash
   make build && cd web && npm ci && npm run build
   ```
4. **Pasang env** — `/opt/angkut/.env` (mode `0640`, milik `angkut`), isi dari `.env.example`.
5. **Migrasi** — `./bin/angkut-api` tidak memigrasi otomatis; jalankan runner:
   ```bash
   cd /opt/angkut && sudo -u angkut ./bin/angkut-migrate up   # atau: go run ./api/cmd/migrate up
   ```
6. **Aktifkan unit** — `systemctl enable --now angkut-api angkut-worker angkut-web`.
7. **Nginx** — salin `nginx.conf.example`, sesuaikan domain & sertifikat, lalu reload.
8. **Verifikasi**
   ```bash
   curl -sf https://angkut.id/api/../healthz   # via Nginx
   curl -sf http://127.0.0.1:8080/readyz
   ```

## Kesehatan & Monitoring

| Endpoint | Fungsi | Catatan |
|----------|--------|---------|
| `GET /healthz` | Liveness | Tidak menyentuh DB — proses hidup |
| `GET /readyz` | Readiness | 503 bila Postgres/Redis tidak siap |
| `GET /metrics` | Metrik Prometheus | Batasi akses dari internet |

Janji operasional yang dipantau: tingkat keberhasilan booking, latensi quote,
umur stok outbox tertua, backlog webhook, saldo COD menunggu cair.

## Rahasia

- Semua kredensial lewat `EnvironmentFile`, bukan di unit systemd atau kode.
- Kunci per-kurir/per-toko disimpan terenkripsi (`*_enc`) di DB, bukan env.
- Rotasi: perbarui `.env` → `systemctl restart angkut-api angkut-worker`.