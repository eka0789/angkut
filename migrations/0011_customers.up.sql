-- 0011 — Customers: CRM dasar tenant-scoped (TASK-021).
-- Kolom metrics (jsonb) menyimpan orders_count, total_spend, aov, return_rate,
-- cod_success_rate, preferred_courier — diisi oleh modul Order/Shipment (Phase 3
-- berikutnya); untuk sekarang default '{}' dan dibaca via endpoint /stats.

CREATE TABLE customers (
    id                 uuid PRIMARY KEY,
    organization_id    uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,

    name               text NOT NULL,
    phone              text NOT NULL,               -- kunci pencarian utama
    email              text,                        -- citext tidak dipakai (lihat 0001); pencarian pakai lower()
    notes              text,

    default_address_id uuid REFERENCES addresses (id) ON DELETE SET NULL,

    -- Segmen: dihitung otomatis, bisa di-override manual.
    segment            text NOT NULL DEFAULT 'new'
                       CHECK (segment IN ('new', 'regular', 'vip', 'high_risk', 'cod_risk')),
    segment_source     text NOT NULL DEFAULT 'auto'
                       CHECK (segment_source IN ('auto', 'manual')),

    -- Metrik agregat; dihitung dari data historis order/shipment.
    metrics            jsonb NOT NULL DEFAULT '{}'::jsonb,

    -- Soft delete: baris tetap ada agar order historis tidak kehilangan referensi.
    deleted_at         timestamptz,

    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT customers_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT customers_phone_not_blank CHECK (length(btrim(phone)) > 0)
);

-- Semua index tenant diawali organization_id (lihat planning/database.md).
CREATE INDEX customers_org_idx ON customers (organization_id, deleted_at);
CREATE UNIQUE INDEX customers_org_phone_idx ON customers (organization_id, phone) WHERE deleted_at IS NULL;
CREATE INDEX customers_org_segment_idx ON customers (organization_id, segment) WHERE deleted_at IS NULL;
CREATE INDEX customers_name_trgm_idx ON customers USING gin (name gin_trgm_ops);
CREATE INDEX customers_phone_trgm_idx ON customers USING gin (phone gin_trgm_ops);
CREATE INDEX customers_email_lower_idx ON customers (lower(email)) WHERE email IS NOT NULL AND deleted_at IS NULL;

CREATE TRIGGER customers_set_updated_at
    BEFORE UPDATE ON customers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE customers IS 'CRM dasar tenant-scoped: profil, segmen, metrik agregat.';
