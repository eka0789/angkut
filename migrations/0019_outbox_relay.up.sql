-- 0019 — Outbox relay: klaim baris + backoff (TASK-032).
--
-- Migrasi 0006 sudah membuat `outbox_events` (append-only + status pending/
-- published/failed/dead). Relay butuh dua hal yang belum ada:
--
--   1. Cara mengklaim baris agar aman dijalankan LEBIH DARI SATU replika relay
--      tanpa dua replika menerbitkan event yang sama. WITHOUT MODE: dipakai
--      `FOR UPDATE SKIP LOCKED` (tanpa kolom tambahan) — jadi tidak perlu
--      skema baru untuk itu.
--   2. Status 'processing' + kolom `claimed_at` agar baris yang ditinggalkan
--      relay yang mati (crash) bisa dipulihkan, bukan macet selamanya.
--
-- Catatan: 0006 TIDAK diubah (aturan: migrasi yang sudah di-merge tidak
-- disentuh) — perubahan skema dibuat sebagai migrasi baru di sini.

-- Status 'processing' ditambahkan ke CHECK yang sudah ada.
alter table outbox_events drop constraint if exists outbox_events_status_check;
alter table outbox_events add constraint outbox_events_status_check
    check (status in ('pending', 'processing', 'published', 'failed', 'dead'));

alter table outbox_events add column if not exists claimed_at timestamptz;
alter table outbox_events add column if not exists claimed_by text;

comment on column outbox_events.claimed_at is
    'Kapan baris diklaim relay; dipakai untuk memulihkan klaim yang ditinggalkan (crash).';
comment on column outbox_events.claimed_by is
    'Identitas relay (hostname/pid) yang mengklaim; membantu diagnosis, bukan kontrol.';

-- Relay memilih pending yang sudah waktunya; index 0006 sudah tepat untuk itu.
-- Tambahan: index untuk memulihkan klaim basi.
create index if not exists outbox_events_stale_claim_idx
    on outbox_events (claimed_at)
    where status = 'processing';