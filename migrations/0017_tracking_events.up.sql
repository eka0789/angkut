-- 0017 — Tracking events (TASK-030).
--
-- Tabel ini APPEND-ONLY: satu baris = satu kejadian nyata di lapangan.
-- Koreksi dilakukan dengan menambah baris baru, bukan mengubah yang lama —
-- jejak pengiriman harus bisa dipercaya sebagai bukti.
--
-- Idempotensi: UNIQUE (shipment_id, status, occurred_at, source) mencegah
-- event yang sama masuk dua kali ketika webhook kurir dikirim berulang
-- (kurir nyata memang sering mengulang kiriman yang sama).

CREATE TABLE tracking_events (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    shipment_id     uuid NOT NULL REFERENCES shipments (id) ON DELETE CASCADE,

    status          text NOT NULL
                    CHECK (status IN ('created', 'pickup_requested', 'picked_up', 'sorting',
                                      'in_transit', 'out_for_delivery', 'delivered',
                                      'failed', 'returned')),
    description     text,
    location        text,
    latitude        numeric(10,7),
    longitude       numeric(10,7),

    occurred_at     timestamptz NOT NULL,
    source          text NOT NULL DEFAULT 'system'
                    CHECK (source IN ('webhook', 'manual', 'system', 'sync')),

    -- Traceability ke event mentah webhook (TASK-031). Nullable: event manual
    -- dan sistem tidak berasal dari webhook.
    raw_event_id    uuid,

    created_at      timestamptz NOT NULL DEFAULT now(),

    -- Koordinat harus lengkap berpasangan; satu saja tidak berarti apa-apa.
    CONSTRAINT tracking_events_coords_paired
        CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CONSTRAINT tracking_events_lat_range CHECK (latitude IS NULL OR (latitude BETWEEN -90 AND 90)),
    CONSTRAINT tracking_events_lng_range CHECK (longitude IS NULL OR (longitude BETWEEN -180 AND 180))
);

-- Kunci idempotensi alami: satu status pada satu waktu dari satu sumber.
CREATE UNIQUE INDEX uq_tracking_event_natural
    ON tracking_events (shipment_id, status, occurred_at, source);

-- Timeline: event terbaru dulu per shipment.
CREATE INDEX idx_tracking_events_shipment_time
    ON tracking_events (shipment_id, occurred_at DESC);

CREATE INDEX idx_tracking_events_org_time
    ON tracking_events (organization_id, occurred_at DESC);

-- Analitik SLA per status (mis. berapa lama dari picked_up ke delivered).
CREATE INDEX idx_tracking_events_status_time
    ON tracking_events (status, occurred_at DESC);

COMMENT ON TABLE tracking_events IS
    'APPEND-ONLY. Satu baris = satu kejadian pengiriman; koreksi lewat baris baru.';

-- Kunci idempotensi event yang SUDAH diproses consumer. Dipakai worker
-- (TASK-033) untuk menjamin "webhook duplikat diproses sekali" tanpa
-- bergantung pada broker.
CREATE TABLE processed_events (
    event_id     text PRIMARY KEY,
    consumer     text NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_processed_events_consumer_time ON processed_events (consumer, processed_at DESC);

COMMENT ON TABLE processed_events IS
    'Dedup consumer: event_id yang sudah diproses tidak diproses ulang (at-least-once → efektif once).';