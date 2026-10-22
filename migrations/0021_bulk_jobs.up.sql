-- 0021: bulk shipping (TASK-039).
--
-- Tiga tabel:
--   * bulk_uploads    — hasil parse file CSV yang diunggah (server menyimpan
--                       baris terparse sebagai JSON, bukan file mentah; obyek
--                       storage eksternal belum dipakai di lingkungan ini).
--   * bulk_jobs       — satu pengiriman massal: status, penghitung progres,
--                       idempotency key (replay tidak membuat job kedua).
--   * bulk_job_rows   — setiap baris data, independen: status validasi lalu
--                       hasil eksekusi (shipment_id + awb bila berhasil).
--
-- Aturan "per baris independen" (CODING_RULES) terefleksi di sini: tidak ada
-- kolom transaksi antar baris; kegagalan satu baris hanya mengisi error pada
-- baris itu.

CREATE TABLE bulk_uploads (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    filename        text NOT NULL,
    row_count       int  NOT NULL CHECK (row_count >= 0),
    rows            jsonb NOT NULL,
    created_by      uuid NOT NULL REFERENCES users (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    consumed_at     timestamptz
);

CREATE INDEX bulk_uploads_org_created_idx ON bulk_uploads (organization_id, created_at DESC);

COMMENT ON TABLE bulk_uploads IS
    'Hasil parse unggahan CSV bulk: baris sudah dinormalisasi, siap divalidasi/disubmit.';

CREATE TABLE bulk_jobs (
    id                   uuid PRIMARY KEY,
    organization_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    idempotency_key      text,
    status               text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    courier_code         text NOT NULL,
    service_code         text NOT NULL,
    origin_warehouse_id  uuid NOT NULL REFERENCES warehouses (id),
    total_rows           int  NOT NULL DEFAULT 0 CHECK (total_rows >= 0),
    accepted_rows        int  NOT NULL DEFAULT 0 CHECK (accepted_rows >= 0),
    rejected_rows        int  NOT NULL DEFAULT 0 CHECK (rejected_rows >= 0),
    processed_rows       int  NOT NULL DEFAULT 0 CHECK (processed_rows >= 0),
    created_rows         int  NOT NULL DEFAULT 0 CHECK (created_rows >= 0),
    failed_rows          int  NOT NULL DEFAULT 0 CHECK (failed_rows >= 0),
    error_summary        text,
    created_by           uuid NOT NULL REFERENCES users (id),
    created_at           timestamptz NOT NULL DEFAULT now(),
    started_at           timestamptz,
    finished_at          timestamptz,
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- Replay Idempotency-Key harus mengembalikan job yang sama, bukan job baru.
CREATE UNIQUE INDEX uq_bulk_jobs_idempotency
    ON bulk_jobs (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX bulk_jobs_org_created_idx ON bulk_jobs (organization_id, created_at DESC);

COMMENT ON TABLE bulk_jobs IS 'Job pengiriman massal; progres dihitung per baris.';

CREATE TABLE bulk_job_rows (
    job_id         uuid NOT NULL REFERENCES bulk_jobs (id) ON DELETE CASCADE,
    row_no         int  NOT NULL CHECK (row_no >= 1),
    recipient_name text NOT NULL,
    recipient_phone text NOT NULL,
    street         text NOT NULL,
    postal_code    text NOT NULL,
    -- Nilai numerik TANPA CHECK nilai positif: tabel ini juga menyimpan baris
    -- yang DITOLAK sebagai laporan (berat 0, angka rusak, dst). Validasi
    -- milik kode (domain/bulk), bukan constraint — tabel menyimpan BUKTI
    -- apa adanya agar pengirim bisa memperbaiki barisnya.
    weight_kg      numeric(10,3) NOT NULL,
    length_cm      numeric(10,3),
    width_cm       numeric(10,3),
    height_cm      numeric(10,3),
    cod_amount     bigint NOT NULL DEFAULT 0,
    declared_value bigint NOT NULL DEFAULT 0,
    insurance      boolean NOT NULL DEFAULT false,
    notes          text,
    status         text NOT NULL
        CHECK (status IN ('valid', 'invalid', 'creating', 'created', 'failed')),
    issues         jsonb NOT NULL DEFAULT '[]'::jsonb,
    shipment_id    uuid,
    awb            text,
    error          text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, row_no)
);

CREATE INDEX bulk_job_rows_job_status_idx ON bulk_job_rows (job_id, status);
CREATE INDEX bulk_job_rows_shipment_idx ON bulk_job_rows (job_id)
    WHERE shipment_id IS NOT NULL;

COMMENT ON TABLE bulk_job_rows IS
    'Satu baris input bulk; status valid → creating → created/failed (atau invalid).';
