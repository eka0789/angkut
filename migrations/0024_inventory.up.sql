-- 0024 — Inventory & warehouse stock transfer (TASK-037/038).
--
-- Model:
--   products           — katalog produk milik organisasi.
--   product_variants   — SKU yang benar-benar dikelola stoknya.
--   inventory          — saldo per (variant, warehouse): on_hand & reserved,
--                        dengan version untuk optimistic lock.
--   inventory_movements — append-only: setiap perubahan saldo punya jejak
--                        (adjust / transfer_out / transfer_in / reserve /
--                        release). TANPA UPDATE/DELETE.
--   stock_transfers    — transfer stok antar gudang (TASK-038), status
--                        completed saat dibuat (atomik) atau cancelled.

CREATE TABLE products (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text NOT NULL,
    category        text NOT NULL DEFAULT '',
    weight_kg       numeric(10,3),
    length_cm       numeric(10,3),
    width_cm        numeric(10,3),
    height_cm       numeric(10,3),
    is_active       boolean NOT NULL DEFAULT true,
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT products_name_not_blank CHECK (length(btrim(name)) > 0)
);

CREATE INDEX products_org_created_idx ON products (organization_id, created_at DESC);
CREATE INDEX products_org_name_idx ON products (organization_id, lower(name));

COMMENT ON TABLE products IS 'Katalog produk (TASK-037); varian adalah SKU yang dikelola stoknya.';

CREATE TABLE product_variants (
    id                  uuid PRIMARY KEY,
    organization_id     uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    product_id          uuid NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    sku                 text NOT NULL,
    name                text NOT NULL DEFAULT '',
    weight_kg           numeric(10,3),
    length_cm           numeric(10,3),
    width_cm            numeric(10,3),
    height_cm           numeric(10,3),
    low_stock_threshold int NOT NULL DEFAULT 5 CHECK (low_stock_threshold >= 0),
    is_active           boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_variant_sku UNIQUE (organization_id, sku)
);

CREATE INDEX variants_product_idx ON product_variants (product_id);

COMMENT ON TABLE product_variants IS 'SKU varian produk; stok dikelola per varian.';

CREATE TABLE inventory (
    id           uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    variant_id   uuid NOT NULL REFERENCES product_variants (id) ON DELETE CASCADE,
    warehouse_id uuid NOT NULL REFERENCES warehouses (id) ON DELETE CASCADE,
    qty_on_hand  int NOT NULL DEFAULT 0 CHECK (qty_on_hand >= 0),
    qty_reserved int NOT NULL DEFAULT 0 CHECK (qty_reserved >= 0),
    version      int NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_inventory_slot UNIQUE (organization_id, variant_id, warehouse_id),
    CONSTRAINT inventory_reserved_sane CHECK (qty_reserved <= qty_on_hand)
);

CREATE INDEX inventory_wh_idx ON inventory (warehouse_id);

COMMENT ON TABLE inventory IS 'Saldo stok per varian per gudang; available = on_hand - reserved.';

CREATE TRIGGER inventory_set_updated_at
    BEFORE UPDATE ON inventory
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Movement APPEND-ONLY: rekonsiliasi & audit keandalan stok bergantung pada
-- jejak yang tidak bisa diubah. Koreksi = movement baru (adjust).
CREATE TABLE inventory_movements (
    id           uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    variant_id   uuid NOT NULL REFERENCES product_variants (id) ON DELETE CASCADE,
    warehouse_id uuid NOT NULL REFERENCES warehouses (id) ON DELETE CASCADE,
    type         text NOT NULL
        CHECK (type IN ('adjust', 'transfer_out', 'transfer_in', 'reserve', 'release')),
    qty_delta    int NOT NULL,
    qty_after    int NOT NULL,
    reason       text NOT NULL DEFAULT '',
    transfer_id  uuid,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX movements_org_created_idx ON inventory_movements (organization_id, created_at DESC);
CREATE INDEX movements_variant_idx ON inventory_movements (variant_id, created_at DESC);

COMMENT ON TABLE inventory_movements IS 'Jejak perubahan stok, append-only.';

CREATE TABLE stock_transfers (
    id           uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    from_warehouse_id uuid NOT NULL REFERENCES warehouses (id) ON DELETE CASCADE,
    to_warehouse_id   uuid NOT NULL REFERENCES warehouses (id) ON DELETE CASCADE,
    notes        text NOT NULL DEFAULT '',
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT transfers_distinct_wh CHECK (from_warehouse_id <> to_warehouse_id)
);

CREATE INDEX transfers_org_created_idx ON stock_transfers (organization_id, created_at DESC);

COMMENT ON TABLE stock_transfers IS 'Transfer stok antar gudang (TASK-038); atomik saat dibuat.';

CREATE TABLE stock_transfer_items (
    transfer_id uuid NOT NULL REFERENCES stock_transfers (id) ON DELETE CASCADE,
    variant_id  uuid NOT NULL REFERENCES product_variants (id) ON DELETE CASCADE,
    qty         int NOT NULL CHECK (qty > 0),
    PRIMARY KEY (transfer_id, variant_id)
);

COMMENT ON TABLE stock_transfer_items IS 'Item varian dalam satu transfer stok.';
