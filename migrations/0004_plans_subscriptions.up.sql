-- 0004 — Paket langganan: plans, subscriptions, usage_counters
-- Nilai dibaca billing & quota AI. Harga rupiah utuh (BIGINT).

CREATE TABLE plans (
    id              uuid PRIMARY KEY,
    code            text        NOT NULL UNIQUE,     -- free, starter, growth, business, enterprise
    name            text        NOT NULL,
    price_monthly   bigint      NOT NULL DEFAULT 0,  -- rupiah / bulan
    price_yearly    bigint      NOT NULL DEFAULT 0,
    limits          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    features        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    is_active       boolean     NOT NULL DEFAULT true,
    sort_order      int         NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT plans_price_nonneg CHECK (price_monthly >= 0 AND price_yearly >= 0)
);

CREATE INDEX plans_active_idx ON plans (is_active, sort_order);

CREATE TRIGGER plans_set_updated_at
    BEFORE UPDATE ON plans
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Satu organisasi memiliki paling banyak satu langganan aktif (dijaga UNIQUE
-- parsial). Riwayat langganan lama tetap tersimpan.
CREATE TABLE subscriptions (
    id                     uuid PRIMARY KEY,
    organization_id        uuid        NOT NULL,     -- FK ditambah di 0005
    plan_id                uuid        NOT NULL REFERENCES plans (id),
    status                 text        NOT NULL DEFAULT 'trialing'
                           CHECK (status IN ('trialing', 'active', 'past_due', 'cancelled', 'expired')),
    current_period_start   timestamptz NOT NULL,
    current_period_end     timestamptz NOT NULL,
    trial_ends_at          timestamptz,
    cancelled_at           timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    version                int         NOT NULL DEFAULT 1,
    CONSTRAINT subscriptions_period_valid CHECK (current_period_end > current_period_start)
);

CREATE UNIQUE INDEX subscriptions_one_active_per_org
    ON subscriptions (organization_id)
    WHERE status IN ('trialing', 'active', 'past_due');

CREATE INDEX subscriptions_org_idx ON subscriptions (organization_id, created_at DESC);

CREATE TRIGGER subscriptions_set_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Metering kuota (shipments, ai_messages, api_calls) per periode bulan.
CREATE TABLE usage_counters (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL,            -- FK ditambah di 0005
    metric          text        NOT NULL
                    CHECK (metric IN ('shipments', 'ai_messages', 'api_calls', 'storage_bytes')),
    period          text        NOT NULL,            -- format YYYY-MM
    value           bigint      NOT NULL DEFAULT 0 CHECK (value >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT usage_counters_period_format CHECK (period ~ '^\d{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT usage_counters_unique UNIQUE (organization_id, metric, period)
);

CREATE TRIGGER usage_counters_set_updated_at
    BEFORE UPDATE ON usage_counters
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Seed paket awal. Angka limit sengaja konservatif dan dapat diubah admin tanpa
-- menyentuh kode (aturan: batas dari config/DB, bukan hardcode di logika).
INSERT INTO plans (id, code, name, price_monthly, price_yearly, limits, features, sort_order) VALUES
('01920000-0000-7000-8000-000000000101', 'free', 'Gratis', 0, 0,
 '{"shipments_per_month": 50, "stores": 1, "warehouses": 1, "users": 2, "ai_messages": 20, "automation_rules": 0, "api_access": false}',
 '{"bulk_shipping": false, "cod_ledger": true, "public_tracking": true}', 10),
('01920000-0000-7000-8000-000000000102', 'starter', 'Starter', 149000, 1490000,
 '{"shipments_per_month": 500, "stores": 2, "warehouses": 1, "users": 5, "ai_messages": 200, "automation_rules": 3, "api_access": false}',
 '{"bulk_shipping": true, "cod_ledger": true, "public_tracking": true}', 20),
('01920000-0000-7000-8000-000000000103', 'growth', 'Growth', 449000, 4490000,
 '{"shipments_per_month": 3000, "stores": 5, "warehouses": 3, "users": 15, "ai_messages": 1000, "automation_rules": 20, "api_access": true}',
 '{"bulk_shipping": true, "cod_ledger": true, "public_tracking": true, "returns": true}', 30),
('01920000-0000-7000-8000-000000000104', 'business', 'Business', 1290000, 12900000,
 '{"shipments_per_month": 15000, "stores": 15, "warehouses": 10, "users": 50, "ai_messages": 5000, "automation_rules": 100, "api_access": true}',
 '{"bulk_shipping": true, "cod_ledger": true, "public_tracking": true, "returns": true, "sso": true}', 40),
('01920000-0000-7000-8000-000000000105', 'enterprise', 'Enterprise', 0, 0,
 '{"shipments_per_month": -1, "stores": -1, "warehouses": -1, "users": -1, "ai_messages": -1, "automation_rules": -1, "api_access": true}',
 '{"bulk_shipping": true, "cod_ledger": true, "public_tracking": true, "returns": true, "sso": true, "sla": true}', 50);