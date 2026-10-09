-- 0001 — Ekstensi & tipe dasar
-- Ekstensi dipakai lintas domain: UUID, pencarian teks, trigram (ADR-006).
-- Catatan: citext TIDAK dipakai (buang ketergantungan tak perlu; email sudah
-- dinormalisasi ke huruf kecil di application layer).

CREATE EXTENSION IF NOT EXISTS "pgcrypto";   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pg_trgm";    -- pencarian fuzzy (nama, kota)
CREATE EXTENSION IF NOT EXISTS "unaccent";   -- pencarian tanpa aksen

-- Fungsi trigger generik untuk mengisi updated_at.
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Semua tabel tenant menyimpan organization_id; helper ini membakukan tipe.
COMMENT ON EXTENSION pg_trgm IS 'Digunakan untuk pencarian teks (lihat ADR-006: PostgreSQL FTS dulu).';