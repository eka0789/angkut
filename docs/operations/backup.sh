#!/usr/bin/env bash
# Backup harian PostgreSQL Angkut dengan retensi.
# Pasang di cron:  30 2 * * *  /opt/angkut/backup.sh >> /var/log/angkut-backup.log 2>&1
#
# Rahasia dibaca dari /opt/angkut/.env (jangan menaruh password di skrip ini).

set -euo pipefail

ENV_FILE="${ENV_FILE:-/opt/angkut/.env}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/angkut}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

# Muat DATABASE_URL dari file env tanpa mengeksekusinya.
if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a; source "$ENV_FILE"; set +a
fi

: "${DATABASE_URL:?DATABASE_URL tidak diset — periksa $ENV_FILE}"

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="$BACKUP_DIR/angkut-$STAMP.dump"

echo "[$(date -Is)] mulai backup ke $OUT"
pg_dump --format=custom --no-owner --no-privileges --file="$OUT" "$DATABASE_URL"

# Verifikasi arsip dapat dibaca (gagal cepat bila korup).
pg_restore --list "$OUT" > /dev/null
echo "[$(date -Is)] backup OK ($(du -h "$OUT" | cut -f1))"

# Hapus backup lebih lama dari retensi.
find "$BACKUP_DIR" -name 'angkut-*.dump' -type f -mtime "+$RETENTION_DAYS" -delete
echo "[$(date -Is)] retensi diterapkan (>${RETENTION_DAYS} hari dibuang)"
