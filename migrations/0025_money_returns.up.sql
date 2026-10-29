-- 0025 — Money: COD, settlement, ledger append-only, payment, invoice,
-- returns & return policy (TASK-041..046).
--
-- Prinsip yang dipaksa di sini:
--   * ledger_entries TIDAK BISA di-UPDATE/DELETE (trigger) — koreksi = entri
--     pembalik (reversal) yang mereferensikan entri asli;
--   * settlement TIDAK mengubah transaksi COD lama — hanya memindahkan
--     statusnya ke `settling`/`settled` dan menulis entri ledger baru;
--   * refund idempoten lewat unique partial index ledger (org, ref_type,
--     ref_id) per akun refund.

CREATE TABLE cod_transactions (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    shipment_id     uuid NOT NULL REFERENCES shipments (id) ON DELETE CASCADE,
    courier_code    text NOT NULL,
    amount          bigint NOT NULL CHECK (amount > 0),
    fee             bigint NOT NULL DEFAULT 0 CHECK (fee >= 0),
    status          text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'collected', 'settling', 'settled', 'failed')),
    settlement_id   uuid,
    collected_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Satu pengiriman COD = satu transaksi COD (idempotensi alami).
CREATE UNIQUE INDEX uq_cod_shipment ON cod_transactions (shipment_id);
CREATE INDEX cod_org_status_idx ON cod_transactions (organization_id, status, created_at DESC);
CREATE INDEX cod_settlement_idx ON cod_transactions (settlement_id) WHERE settlement_id IS NOT NULL;

COMMENT ON TABLE cod_transactions IS 'Tagihan COD per pengiriman (TASK-041); fee berasal dari pricing engine.';

CREATE TABLE cod_settlements (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    courier_code    text NOT NULL,
    period_start    date NOT NULL,
    period_end      date NOT NULL,
    status          text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'paid')),
    total_amount    bigint NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    total_fee       bigint NOT NULL DEFAULT 0 CHECK (total_fee >= 0),
    net_amount      bigint NOT NULL GENERATED ALWAYS AS (total_amount - total_fee) STORED,
    reference       text,
    paid_at         timestamptz,
    proof_url       text,
    idempotency_key text,
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT settlements_period_sane CHECK (period_end >= period_start)
);

CREATE UNIQUE INDEX uq_cod_settlement_idem
    ON cod_settlements (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX cod_settlements_org_idx ON cod_settlements (organization_id, created_at DESC);

COMMENT ON TABLE cod_settlements IS 'Rekonsiliasi COD per kurir & periode (TASK-042); net = total − fee.';

CREATE TRIGGER cod_settlements_set_updated_at
    BEFORE UPDATE ON cod_settlements
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cod_settlement_items (
    settlement_id    uuid NOT NULL REFERENCES cod_settlements (id) ON DELETE CASCADE,
    cod_transaction_id uuid NOT NULL REFERENCES cod_transactions (id),
    PRIMARY KEY (settlement_id, cod_transaction_id)
);

COMMENT ON TABLE cod_settlement_items IS 'Transaksi COD yang digabung dalam satu settlement.';

-- ── Ledger append-only ──────────────────────────────────────────────────
CREATE TABLE ledger_entries (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    entry_date      date NOT NULL DEFAULT CURRENT_DATE,
    account         text NOT NULL,
    debit           bigint NOT NULL DEFAULT 0 CHECK (debit >= 0),
    credit          bigint NOT NULL DEFAULT 0 CHECK (credit >= 0),
    ref_type        text NOT NULL,
    ref_id          text NOT NULL,
    memo            text NOT NULL DEFAULT '',
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ledger_org_date_idx ON ledger_entries (organization_id, entry_date DESC);
CREATE INDEX ledger_org_account_idx ON ledger_entries (organization_id, account);
-- Refund idempoten: satu entri refund per referensi (mis. satu return).
CREATE UNIQUE INDEX uq_ledger_refund_once
    ON ledger_entries (organization_id, account, ref_type, ref_id)
    WHERE account = 'return_refund';

CREATE FUNCTION ledger_block_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries append-only: % tidak diizinkan (buat entri pembalik)',
        TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_no_update
    BEFORE UPDATE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_block_mutation();
CREATE TRIGGER ledger_no_delete
    BEFORE DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_block_mutation();

COMMENT ON TABLE ledger_entries IS 'Buku besar append-only (TASK-043); koreksi = entri pembalik.';

-- ── Payment & invoice (provider mock) ───────────────────────────────────
CREATE TABLE invoices (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    number          text NOT NULL,
    order_id        uuid REFERENCES orders (id) ON DELETE SET NULL,
    amount          bigint NOT NULL CHECK (amount >= 0),
    status          text NOT NULL DEFAULT 'issued'
        CHECK (status IN ('issued', 'paid', 'void')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_invoice_number UNIQUE (organization_id, number)
);

CREATE INDEX invoices_org_idx ON invoices (organization_id, created_at DESC);

COMMENT ON TABLE invoices IS 'Tagihan per order non-COD (TASK-044, provider mock).';

CREATE TRIGGER invoices_set_updated_at
    BEFORE UPDATE ON invoices
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payments (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    invoice_id      uuid REFERENCES invoices (id) ON DELETE SET NULL,
    provider        text NOT NULL DEFAULT 'mock',
    provider_ref    text NOT NULL,
    amount          bigint NOT NULL CHECK (amount > 0),
    payload         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_payment_provider_ref UNIQUE (organization_id, provider, provider_ref)
);

COMMENT ON TABLE payments IS 'Pembayaran masuk; replay provider_ref sama = payment yang sama (idempoten).';

-- ── Returns (TASK-045/046) ──────────────────────────────────────────────
CREATE TABLE return_policies (
    id                 uuid PRIMARY KEY,
    organization_id    uuid NOT NULL UNIQUE REFERENCES organizations (id) ON DELETE CASCADE,
    window_days        int NOT NULL DEFAULT 7 CHECK (window_days >= 0),
    auto_approve_limit bigint NOT NULL DEFAULT 0 CHECK (auto_approve_limit >= 0),
    conditions         jsonb NOT NULL DEFAULT '[]'::jsonb,
    updated_by         uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE return_policies IS 'Kebijakan retur per organisasi (TASK-046).';

CREATE TRIGGER return_policies_set_updated_at
    BEFORE UPDATE ON return_policies
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE returns (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    order_id        uuid REFERENCES orders (id) ON DELETE SET NULL,
    shipment_id     uuid REFERENCES shipments (id) ON DELETE SET NULL,
    type            text NOT NULL DEFAULT 'refund'
        CHECK (type IN ('refund', 'exchange')),
    reason          text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'requested'
        CHECK (status IN ('requested', 'approved', 'rejected', 'received',
                          'inspected', 'refunded', 'cancelled')),
    refund_amount   bigint NOT NULL DEFAULT 0 CHECK (refund_amount >= 0),
    refund_method   text,
    decided_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    decided_at      timestamptz,
    refunded_at     timestamptz,
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX returns_org_status_idx ON returns (organization_id, status, created_at DESC);

COMMENT ON TABLE returns IS 'Retur/refund (TASK-045); state machine ketat di domain.';

CREATE TRIGGER returns_set_updated_at
    BEFORE UPDATE ON returns
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE return_items (
    id            uuid PRIMARY KEY,
    return_id     uuid NOT NULL REFERENCES returns (id) ON DELETE CASCADE,
    order_item_id uuid REFERENCES order_items (id) ON DELETE SET NULL,
    name_snapshot text NOT NULL DEFAULT '',
    qty           int NOT NULL CHECK (qty > 0),
    condition     text NOT NULL DEFAULT 'unknown'
        CHECK (condition IN ('unknown', 'good', 'damaged', 'lost')),
    restocked     boolean NOT NULL DEFAULT false
);

COMMENT ON TABLE return_items IS 'Item dalam satu retur; kondisi diisi saat inspect.';
