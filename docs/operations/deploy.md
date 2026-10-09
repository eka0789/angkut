# Prosedur Rilis — Angkut

Prinsip: **artefak yang diuji adalah artefak yang dirilis**; tidak ada build di
server produksi.

## 0. Prasyarat

- CI hijau pada commit yang akan dirilis (lihat `.github/workflows/ci.yml`).
- `dev-docs/ai/CHANGELOG` dan `VERSION.md` sudah diperbarui.
- Backup terakhir < 24 jam (`docs/operations/backup.sh`).

## 1. Build & Uji (di mesin build / CI)

```bash
make build                       # bin/angkut-api, bin/angkut-worker
make test
cd web && npm ci && npm run build
```

## 2. Rilis

1. **Snapshot database** (wajib sebelum migrasi):
   ```bash
   /opt/angkut/backup.sh
   ```
2. **Kirim artefak** ke `/opt/angkut` (rsync/scp), jaga kepemilikan `angkut:angkut`.
3. **Migrasi** (sebelum restart, agar kode baru menemukan skema yang benar):
   ```bash
   cd /opt/angkut
   sudo -u angkut ./bin/angkut-migrate up
   sudo -u angkut ./bin/angkut-migrate status
   ```
4. **Restart berurutan**: worker dulu (berhenti menerima event baru), lalu API,
   lalu web — sehingga tidak ada jendela di mana API memanggil worker lama.
   ```bash
   sudo systemctl restart angkut-worker
   sudo systemctl restart angkut-api
   sudo systemctl restart angkut-web
   ```
5. **Verifikasi**
   ```bash
   curl -sf http://127.0.0.1:8080/readyz
   curl -sf http://127.0.0.1:8080/healthz
   curl -s  http://127.0.0.1:8080/metrics | grep angkut_http_requests_total
   ```

## 3. Rollback

### Kode
```bash
# Kembalikan binary versi sebelumnya (simpan 2 versi terakhir).
sudo install -o angkut -g angkut -m 0755 /opt/angkut/releases/angkut-api-<prev> /opt/angkut/bin/angkut-api
sudo systemctl restart angkut-api angkut-worker
```

### Skema
Rollback migrasi bersifat **membalik perubahan tipe**, bukan hanya kode:
```bash
sudo -u angkut ./bin/angkut-migrate down 1     # batalkan satu migrasi terakhir
```
Untuk perubahan yang tidak dapat dibalik (mis. kolom terhapus), pulihkan dari
backup langkah 2.1 — inilah alasan snapshot wajib.

## 4. Deployment Gagal (jam-jam pertama)

| Gejala | Pemeriksaan cepat |
|--------|-------------------|
| `/readyz` 503 | Postgres/Redis hidup? cek `journalctl -u angkut-api` |
| API naik tapi 500 massal | Skema tertinggal — migrasi belum dijalankan? |
| Event macet | `outbox_events.status='pending'` menua → worker mati / RabbitMQ down |
| Web kosong | `journalctl -u angkut-web`; pastikan `.output/` terkirim |
| Rate limit terlalu ketat | `RATE_LIMIT_*` di `.env`, lalu restart |

## 5. Catatan

- Jangan pernah menjalankan `migrate down` di produksi tanpa snapshot.
- Jangan mengedit data langsung di DB produksi; koreksi lewat API agar audit
  log dan ledger tetap utuh.
