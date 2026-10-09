-- 0006 — Audit & outbox (append-only) + api_keys
-- Dua tabel append-only paling penting:
--   audit_logs    — siapa melakukan apa, kapan (termasuk aksi AI).
--   outbox_events — pola transactional outbox (ADR-005): event ditulis di
--                   transaksi yang sama dengan perubahan data, lalu direlay
--                   ke RabbitMQ oleh worker. Tidak ada event hilang.

CREATE TABLE audit_logs (
    id              bigserial PRIMARY KEY,           -- urut waktu, murah
    organization_id uuid        REFERENCES organizations (id) ON DELETE SET NULL,
    actor_user_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    actor_type      text        NOT NULL DEFAULT 'user'
                    CHECK (actor_type IN ('user', 'system', 'ai', 'api_key')),
    action          text        NOT NULL,            -- mis. shipment.created
    entity_type     text,                            -- mis. shipment
    entity_id       uuid,
    changes         jsonb,                           -- {before, after} bila relevan
    metadata        jsonb,                           -- request_id, ip, user_agent
    ip_address      inet,
    user_agent      text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_org_time_idx ON audit_logs (organization_id, created_at DESC);
CREATE INDEX audit_logs_entity_idx   ON audit_logs (entity_type, entity_id);
CREATE INDEX audit_logs_actor_idx    ON audit_logs (actor_user_id, created_at DESC);
CREATE INDEX audit_logs_action_idx   ON audit_logs (action);

COMMENT ON TABLE audit_logs IS 'APPEND-ONLY. Tidak ada UPDATE/DELETE; koreksi lewat baris baru.';

-- Outbox: ditulis dalam transaksi bisnis, dikonsumsi relay worker.
CREATE TABLE outbox_events (
    id              bigserial PRIMARY KEY,
    aggregate_type  text        NOT NULL,            -- mis. shipment
    aggregate_id    uuid        NOT NULL,
    organization_id uuid        REFERENCES organizations (id) ON DELETE CASCADE,
    event_type      text        NOT NULL,            -- mis. shipment.cancelled
    payload         jsonb       NOT NULL,
    headers         jsonb       NOT NULL DEFAULT '{}'::jsonb,
    status          text        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'published', 'failed', 'dead')),
    attempts        int         NOT NULL DEFAULT 0,
    last_error      text,
    available_at    timestamptz NOT NULL DEFAULT now(),  -- dukung backoff
    published_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Relay worker memilih baris pending yang sudah waktunya, paling lama dulu.
CREATE INDEX outbox_events_pending_idx
    ON outbox_events (available_at, id)
    WHERE status = 'pending';

CREATE INDEX outbox_events_aggregate_idx ON outbox_events (aggregate_type, aggregate_id);
CREATE INDEX outbox_events_org_time_idx  ON outbox_events (organization_id, created_at DESC);

COMMENT ON TABLE outbox_events IS 'Transactional outbox: ditulis dalam transaksi bisnis, direlay ke RabbitMQ (ADR-005).';

-- Kunci API publik (Public API / integrasi pihak ketiga).
CREATE TABLE api_keys (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text        NOT NULL,
    key_prefix      text        NOT NULL,            -- bagian tampil (mis. ak_live_ab12)
    key_hash        text        NOT NULL,            -- hash kunci; kunci mentah hanya sekali
    scopes          jsonb       NOT NULL DEFAULT '[]'::jsonb,
    rate_limit_per_min int      NOT NULL DEFAULT 120,
    last_used_at    timestamptz,
    expires_at      timestamptz,
    revoked_at      timestamptz,
    created_by      uuid        REFERENCES users (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX api_keys_hash_unique ON api_keys (key_hash);
CREATE INDEX api_keys_org_active_idx ON api_keys (organization_id)
    WHERE revoked_at IS NULL;

CREATE TRIGGER api_keys_set_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON COLUMN api_keys.key_hash IS 'Hash kunci API. Kunci mentah hanya ditampilkan sekali saat dibuat.';