-- 0016 — Shipments, items, packages (TASK-027).
--
-- Shipment adalah eksekusi fisik sebuah order: satu order bisa melahirkan
-- beberapa shipment (fulfillment sebagian). Sematan denormalisasi
-- (courier_code, service_code) di sini SENGAJA disimpan sebagai kolom teks
-- juga agar riwayat shipment tidak berubah bila registry kurir berganti
-- (mengikuti pola snapshot nama produk di order_items).

CREATE TABLE shipments (
    id                    uuid PRIMARY KEY,
    organization_id       uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    order_id              uuid REFERENCES orders (id) ON DELETE SET NULL,
    store_id              uuid REFERENCES stores (id) ON DELETE SET NULL,
    warehouse_id          uuid REFERENCES warehouses (id) ON DELETE SET NULL,

    courier_id            uuid REFERENCES couriers (id) ON DELETE SET NULL,
    courier_service_id    uuid REFERENCES courier_services (id) ON DELETE SET NULL,
    courier_code          text NOT NULL,
    service_code          text NOT NULL,

    awb                   text NOT NULL,               -- nomor resi
    status                text NOT NULL DEFAULT 'draft'
                          CHECK (status IN ('draft','pending_pickup','picked_up','in_transit',
                                            'out_for_delivery','delivered','failed','returned','cancelled')),

    origin_address_id      uuid REFERENCES addresses (id) ON DELETE SET NULL,
    destination_address_id uuid REFERENCES addresses (id) ON DELETE SET NULL,

    actual_weight          numeric(10,3) NOT NULL DEFAULT 0,
    volumetric_weight      numeric(10,3) NOT NULL DEFAULT 0,
    chargeable_weight      numeric(10,3) NOT NULL DEFAULT 0,
    length                 numeric(10,2),
    width                  numeric(10,2),
    height                 numeric(10,2),
    package_type           text,

    declared_value         bigint NOT NULL DEFAULT 0,
    insurance_amount       bigint NOT NULL DEFAULT 0,
    shipping_price         bigint NOT NULL DEFAULT 0,  -- harga ke customer
    cost_price             bigint NOT NULL DEFAULT 0,  -- harga dari kurir (margin)

    is_cod                 boolean NOT NULL DEFAULT false,
    cod_amount             bigint NOT NULL DEFAULT 0,
    cod_fee                bigint NOT NULL DEFAULT 0,

    eta_min_days           int NOT NULL DEFAULT 0,     -- snapshot saat booking
    eta_max_days           int NOT NULL DEFAULT 0,

    notes                  text,
    failure_reason         text,
    booked_at              timestamptz,
    delivered_at           timestamptz,

    -- Optimistic lock: proteksi konflik status bersamaan.
    version                int NOT NULL DEFAULT 1,

    -- Idempotency: retry dengan key sama tidak membuat baris kedua.
    idempotency_key        text,

    label_url              text,

    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT shipments_weights_non_negative
        CHECK (actual_weight >= 0 AND volumetric_weight >= 0 AND chargeable_weight >= 0),
    CONSTRAINT shipments_money_non_negative
        CHECK (declared_value >= 0 AND insurance_amount >= 0 AND shipping_price >= 0
               AND cost_price >= 0 AND cod_amount >= 0 AND cod_fee >= 0),
    CONSTRAINT shipments_cod_amount_requires_flag CHECK (NOT is_cod OR cod_amount > 0),
    CONSTRAINT shipments_awb_not_blank CHECK (length(btrim(awb)) > 0)
);

-- Resi unik per organisasi; duplikat global antar tenant tidak apa-apa.
CREATE UNIQUE INDEX uq_shipment_awb ON shipments (organization_id, awb);

-- Idempotency: hanya berlaku bila key diisi (partial).
CREATE UNIQUE INDEX uq_shipment_idem
    ON shipments (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX idx_shipments_org_status ON shipments (organization_id, status, created_at DESC);
CREATE INDEX idx_shipments_order ON shipments (order_id);
-- Pencarian resi parsial (pg_trgm dari migrasi 0001).
CREATE INDEX idx_shipments_awb_trgm ON shipments USING gin (awb gin_trgm_ops);
-- Analitik SLA: hanya baris terkirim.
CREATE INDEX idx_shipments_org_delivered
    ON shipments (organization_id, delivered_at)
    WHERE delivered_at IS NOT NULL;

CREATE TRIGGER shipments_set_updated_at
    BEFORE UPDATE ON shipments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE shipments IS 'Pengiriman fisik: resi, kurir, berat, harga, status, idempotency.';

CREATE TABLE shipment_items (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    shipment_id     uuid NOT NULL REFERENCES shipments (id) ON DELETE CASCADE,
    order_item_id   uuid REFERENCES order_items (id) ON DELETE SET NULL,
    variant_id      uuid,
    qty             int NOT NULL,
    name_snapshot   text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT shipment_items_qty_positive CHECK (qty > 0),
    CONSTRAINT shipment_items_name_not_blank CHECK (length(btrim(name_snapshot)) > 0)
);

CREATE INDEX idx_shipment_items_shipment ON shipment_items (shipment_id);
CREATE INDEX idx_shipment_items_org ON shipment_items (organization_id);

COMMENT ON TABLE shipment_items IS 'Isi paket sebuah shipment (snapshot nama).';

CREATE TABLE shipment_packages (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    shipment_id     uuid NOT NULL REFERENCES shipments (id) ON DELETE CASCADE,
    sequence        int NOT NULL,
    weight          numeric(10,3) NOT NULL DEFAULT 0,
    length          numeric(10,2),
    width           numeric(10,2),
    height          numeric(10,2),
    awb             text,                              -- bila kurir memberi resi per paket
    qr_payload      text NOT NULL,
    barcode_value   text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT shipment_packages_seq_positive CHECK (sequence > 0),
    CONSTRAINT shipment_packages_weight_non_negative CHECK (weight >= 0)
);

CREATE UNIQUE INDEX uq_shipment_package_seq ON shipment_packages (shipment_id, sequence);
CREATE INDEX idx_shipment_packages_shipment ON shipment_packages (shipment_id);

COMMENT ON TABLE shipment_packages IS 'Rincian paket per shipment + payload label (QR/barcode).';