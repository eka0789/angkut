-- 0012 — Courier registry: zones, couriers, courier_services, courier_rates
-- (TASK-022). Kurir bersifat global (platform-level), bukan per-tenant.
-- Kredensial kurir TIDAK disimpan sebagai teks — kolom credentials_enc diisi
-- cipher AES-GCM dan hanya dibuka di adapter.

-- Zona tarif: kelompok wilayah (province/city/district/postal_prefix).
-- `values` menyimpan kode wilayah (id provinsi/kota/dsst) anggota zona.
CREATE TABLE zones (
    code        text PRIMARY KEY,
    name        text NOT NULL,
    type        text NOT NULL
                CHECK (type IN ('province', 'city', 'district', 'postal_prefix')),
    "values"    text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN zones."values" IS 'Daftar kode wilayah anggota zona (mis. id provinsi).';

-- Registry kurir.
CREATE TABLE couriers (
    id              uuid PRIMARY KEY,
    code            text NOT NULL UNIQUE,           -- jne, jnt, sicepat, mock, ...
    name            text NOT NULL,
    adapter_type    text NOT NULL,                  -- kunci registry adapter
    config          jsonb NOT NULL DEFAULT '{}'::jsonb,  -- config non-rahasia
    credentials_enc bytea,                          -- rahasia terenkripsi (opsional)
    is_active       boolean NOT NULL DEFAULT true,
    logo_url        text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Layanan per kurir (REG, YES, BEST, EZ, ...).
CREATE TABLE courier_services (
    id                 uuid PRIMARY KEY,
    courier_id         uuid NOT NULL REFERENCES couriers (id) ON DELETE CASCADE,
    code               text NOT NULL,
    name               text NOT NULL,
    supports_cod       boolean NOT NULL DEFAULT false,
    supports_pickup    boolean NOT NULL DEFAULT false,
    supports_insurance boolean NOT NULL DEFAULT false,
    eta_min_days       int NOT NULL,
    eta_max_days       int NOT NULL,
    volumetric_divisor int NOT NULL DEFAULT 6000,
    reliability_score  numeric(5,2) NOT NULL DEFAULT 0,  -- 0–100, dari data historis
    is_active          boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT courier_services_eta_order CHECK (eta_min_days <= eta_max_days),
    CONSTRAINT courier_services_volumetric_positive CHECK (volumetric_divisor > 0),
    CONSTRAINT courier_services_reliability_range CHECK (reliability_score BETWEEN 0 AND 100),
    CONSTRAINT courier_services_pair_unique UNIQUE (courier_id, code)
);

-- Tarif per pasangan zona per tier berat (kg). max_weight NULL = tanpa batas
-- atas (tier terakhir). Hanya satu rate efektif per (service, origin, dest,
-- tier) — constraint UNIQUE menegakkan.
CREATE TABLE courier_rates (
    id                 uuid PRIMARY KEY,
    courier_service_id uuid NOT NULL REFERENCES courier_services (id) ON DELETE CASCADE,
    origin_zone        text NOT NULL REFERENCES zones (code),
    destination_zone   text NOT NULL REFERENCES zones (code),
    min_weight         numeric(6,2) NOT NULL,
    max_weight         numeric(6,2),
    price              bigint NOT NULL CHECK (price >= 0),
    effective_from     timestamptz NOT NULL DEFAULT now(),
    effective_to       timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT courier_rates_tier_sane CHECK (max_weight IS NULL OR max_weight > min_weight),
    CONSTRAINT courier_rates_pair_unique
        UNIQUE (courier_service_id, origin_zone, destination_zone, min_weight, effective_from)
);

CREATE INDEX courier_services_courier_idx ON courier_services (courier_id, is_active);
CREATE INDEX courier_rates_lookup_idx ON courier_rates (courier_service_id, origin_zone, destination_zone, min_weight)
    WHERE effective_to IS NULL;
-- Resolver zona: cari zona yang memuat sebuah kode wilayah.
CREATE INDEX zones_values_idx ON zones USING gin ("values");

CREATE TRIGGER couriers_set_updated_at
    BEFORE UPDATE ON couriers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER courier_services_set_updated_at
    BEFORE UPDATE ON courier_services
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE zones IS 'Zona tarif pengiriman: kelompok wilayah dengan kode tarif.';
COMMENT ON TABLE couriers IS 'Registry kurir: identitas, adapter, status aktif.';
COMMENT ON TABLE courier_services IS 'Layanan kurir: kode, kemampuan, ETA, reliabilitas.';
COMMENT ON TABLE courier_rates IS 'Tarif per pasangan zona per tier berat.';
