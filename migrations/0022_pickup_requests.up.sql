-- 0022 — Pickup requests: permintaan penjemputan paket oleh kurir dari gudang.
--
-- Sebuah pickup request mengagregasi satu atau lebih pengiriman yang akan
-- dijemput kurir pada waktu terjadwal. Status: requested → scheduled →
-- completed / cancelled. Tabel penghubung pickup_shipments mencatat pengiriman
-- yang tergabung dalam request (banyak-ke-banyak).

CREATE TABLE pickup_requests (
    id               uuid PRIMARY KEY,
    organization_id  uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    warehouse_id     uuid NOT NULL REFERENCES warehouses (id) ON DELETE RESTRICT,
    courier_code     text NOT NULL,
    scheduled_at     timestamptz,
    status           text NOT NULL DEFAULT 'requested'
                     CHECK (status IN ('requested','scheduled','completed','cancelled')),
    notes            text NOT NULL DEFAULT '',
    created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX pickup_requests_org_idx ON pickup_requests (organization_id, status, created_at DESC);
CREATE INDEX pickup_requests_warehouse_idx ON pickup_requests (organization_id, warehouse_id);

CREATE TRIGGER pickup_requests_set_updated_at
    BEFORE UPDATE ON pickup_requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE pickup_requests IS 'Permintaan penjemputan paket oleh kurir dari gudang (TASK-036).';

CREATE TABLE pickup_shipments (
    pickup_id    uuid NOT NULL REFERENCES pickup_requests (id) ON DELETE CASCADE,
    shipment_id  uuid NOT NULL REFERENCES shipments (id) ON DELETE CASCADE,
    added_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (pickup_id, shipment_id)
);

CREATE INDEX pickup_shipments_shipment_idx ON pickup_shipments (shipment_id);

COMMENT ON TABLE pickup_shipments IS 'Pengiriman yang tergabung dalam satu pickup request.';