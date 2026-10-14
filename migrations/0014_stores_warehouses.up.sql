-- 0014 — Stores & warehouses: fondasi fulfillment tenant-scoped.
--
-- Prasyarat Order (TASK-026) & Shipment (TASK-027): order berasal dari sebuah
-- kanal penjualan (store), dan shipment berangkat dari gudang/alamat asal.
-- Kredensial kanal TIDAK disimpan sebagai teks — credentials_enc diisi cipher
-- AES-GCM (cryptox) dan hanya dibuka di adapter (Phase 4).
--
-- Urutan penting: warehouses dibuat lebih dulu karena stores.warehouse_id
-- mereferensikannya.

CREATE TABLE warehouses (
    id               uuid PRIMARY KEY,
    organization_id  uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,

    name             text NOT NULL,
    -- Alamat asal pengiriman. Nullable: gudang boleh didaftarkan sebelum
    -- alamatnya lengkap; shipment yang butuh origin tetap memvalidasi.
    address_id       uuid REFERENCES addresses (id) ON DELETE SET NULL,
    capacity         int,
    is_default       boolean NOT NULL DEFAULT false,
    operating_hours  jsonb,
    is_active        boolean NOT NULL DEFAULT true,
    deleted_at       timestamptz,

    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT warehouses_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT warehouses_capacity_positive CHECK (capacity IS NULL OR capacity > 0)
);

CREATE INDEX warehouses_org_idx ON warehouses (organization_id, is_active) WHERE deleted_at IS NULL;

-- Maksimum satu gudang default per organisasi.
CREATE UNIQUE INDEX warehouses_org_default_idx
    ON warehouses (organization_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE TRIGGER warehouses_set_updated_at
    BEFORE UPDATE ON warehouses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE warehouses IS 'Gudang/lokasi fulfillment tenant; alamat asal pengiriman.';

CREATE TABLE stores (
    id               uuid PRIMARY KEY,
    organization_id  uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,

    name             text NOT NULL,
    -- Kanal penjualan. `manual` = input langsung tanpa integrasi.
    channel          text NOT NULL DEFAULT 'manual'
                     CHECK (channel IN ('manual','shopify','woocommerce','tokopedia','shopee','tiktok_shop','custom')),
    adapter_type     text NOT NULL DEFAULT 'mock',   -- kunci registry adapter
    credentials_enc  bytea,                          -- rahasia terenkripsi; tak pernah di-log
    warehouse_id     uuid REFERENCES warehouses (id) ON DELETE SET NULL, -- default fulfillment
    is_active        boolean NOT NULL DEFAULT true,
    deleted_at       timestamptz,

    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT stores_name_not_blank CHECK (length(btrim(name)) > 0)
);

CREATE INDEX stores_org_idx ON stores (organization_id, is_active) WHERE deleted_at IS NULL;

CREATE TRIGGER stores_set_updated_at
    BEFORE UPDATE ON stores
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE stores IS 'Kanal penjualan tenant (manual/marketplace); sumber order.';