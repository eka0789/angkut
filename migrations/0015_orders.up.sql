-- 0015 — Orders & order items (TASK-026).
--
-- Order adalah niat jual pelanggan; shipment (TASK-027) adalah eksekusinya.
-- Satu order dapat melahirkan beberapa shipment (partial fulfillment).
--
-- Nomor order di-generate di application layer (ORD-YYYY-NNNNNN) dan unik
-- per organisasi. Snapshot nama pada order_items menjaga histori tetap benar
-- walau produk berubah (produk sendiri menyusul di Phase 4 — kolom variant_id
-- nullable sampai tabel products ada).

CREATE TABLE orders (
    id                  uuid PRIMARY KEY,
    organization_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    store_id            uuid REFERENCES stores (id) ON DELETE SET NULL,
    customer_id         uuid NOT NULL REFERENCES customers (id) ON DELETE RESTRICT,

    order_number        text NOT NULL,               -- ORD-2026-000123
    external_order_id   text,                        -- id marketplace (sync Phase 4)

    status              text NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft','confirmed','processing','shipped','delivered','cancelled','returned')),
    payment_type        text NOT NULL DEFAULT 'prepaid'
                        CHECK (payment_type IN ('prepaid','cod')),

    -- Uang dalam rupiah utuh (bigint).
    subtotal            bigint NOT NULL DEFAULT 0,
    shipping_cost       bigint NOT NULL DEFAULT 0,
    discount            bigint NOT NULL DEFAULT 0,
    total_amount        bigint NOT NULL DEFAULT 0,

    shipping_address_id uuid REFERENCES addresses (id) ON DELETE SET NULL,
    notes               text,
    cancelled_reason    text,

    deleted_at          timestamptz,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT orders_number_not_blank CHECK (length(btrim(order_number)) > 0),
    CONSTRAINT orders_amounts_non_negative
        CHECK (subtotal >= 0 AND shipping_cost >= 0 AND discount >= 0 AND total_amount >= 0)
);

-- Nomor order unik per organisasi (termasuk baris terhapus: nomor tidak didaur ulang).
CREATE UNIQUE INDEX uq_order_number ON orders (organization_id, order_number);

-- Idempotency sync marketplace: satu external_order_id hanya boleh sekali per store.
CREATE UNIQUE INDEX uq_order_external
    ON orders (store_id, external_order_id)
    WHERE external_order_id IS NOT NULL;

CREATE INDEX idx_orders_org_status_created
    ON orders (organization_id, status, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_orders_customer
    ON orders (customer_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE TRIGGER orders_set_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE orders IS 'Order pelanggan tenant; sumber pembuatan shipment.';

CREATE TABLE order_items (
    id                     uuid PRIMARY KEY,
    organization_id        uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    order_id               uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,

    -- Nullable: produk/variant menyusul (Phase 4). Snapshot nama tetap wajib
    -- agar histori order tidak pernah bergantung pada katalog saat ini.
    variant_id             uuid,

    product_name_snapshot  text NOT NULL,
    variant_name_snapshot  text,

    qty                    int NOT NULL,
    unit_price             bigint NOT NULL,
    total_price            bigint NOT NULL,

    created_at             timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT order_items_qty_positive CHECK (qty > 0),
    CONSTRAINT order_items_unit_price_non_negative CHECK (unit_price >= 0),
    CONSTRAINT order_items_total_price_non_negative CHECK (total_price >= 0),
    CONSTRAINT order_items_name_not_blank CHECK (length(btrim(product_name_snapshot)) > 0)
);

CREATE INDEX idx_order_items_order ON order_items (order_id);
CREATE INDEX idx_order_items_org ON order_items (organization_id);

COMMENT ON TABLE order_items IS 'Baris item order dengan snapshot nama produk.';