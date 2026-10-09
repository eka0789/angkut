-- 0005 — Organisasi & keanggotaan: organizations, organization_members
-- Di sinilah kolom organization_id lahir; FK dari tabel 0004 dipasang di sini
-- (urutan migrasi: users → roles → plans → organizations).
--
-- Isolasi tenant (ADR-008): SETIAP query domain WAJIB memfilter
-- organization_id. Nilai diambil dari context (middleware tenant),
-- TIDAK PERNAH dari input pengguna.

CREATE TABLE organizations (
    id              uuid PRIMARY KEY,
    name            text        NOT NULL,
    slug            text        NOT NULL,
    legal_name      text,
    business_type   text,                            -- umkm, toko_online, brand_d2c, warehouse
    phone           text,
    email           text,
    logo_url        text,
    timezone        text        NOT NULL DEFAULT 'Asia/Jakarta',
    currency        text        NOT NULL DEFAULT 'IDR',
    owner_user_id   uuid        NOT NULL REFERENCES users (id),
    status          text        NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended', 'closed')),
    settings        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz,
    CONSTRAINT organizations_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT organizations_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    CONSTRAINT organizations_currency_iso CHECK (currency ~ '^[A-Z]{3}$')
);

-- Slug unik antar organisasi aktif (soft-deleted tidak memblokir pemakaian).
CREATE UNIQUE INDEX organizations_slug_unique_active
    ON organizations (slug)
    WHERE deleted_at IS NULL;

CREATE INDEX organizations_owner_idx ON organizations (owner_user_id);
CREATE INDEX organizations_name_trgm_idx ON organizations USING gin (name gin_trgm_ops);

CREATE TRIGGER organizations_set_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Keanggotaan: user ↔ organisasi dengan role.
CREATE TABLE organization_members (
    id              uuid PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id         uuid        NOT NULL REFERENCES roles (id),
    status          text        NOT NULL DEFAULT 'active'
                    CHECK (status IN ('invited', 'active', 'suspended', 'removed')),
    invited_by      uuid        REFERENCES users (id),
    invited_email   text,                            -- untuk undangan sebelum user ada
    invite_token_hash text,                          -- hash token undangan
    invite_expires_at timestamptz,
    joined_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    -- Satu user hanya boleh punya satu baris keanggotaan per organisasi.
    CONSTRAINT organization_members_unique UNIQUE (organization_id, user_id)
);

CREATE INDEX organization_members_user_idx ON organization_members (user_id, status);
CREATE INDEX organization_members_org_idx  ON organization_members (organization_id, status);
CREATE INDEX organization_members_invite_idx ON organization_members (invite_token_hash)
    WHERE invite_token_hash IS NOT NULL;

CREATE TRIGGER organization_members_set_updated_at
    BEFORE UPDATE ON organization_members
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Anggota terakhir yang masih owner tidak boleh dihapus: bisnis harus selalu
-- punya pemilik. Dijaga di application layer (butuh query lintas baris),
-- dengan indeks berikut mempercepat pemeriksaannya.
CREATE INDEX organization_members_org_role_idx
    ON organization_members (organization_id, role_id)
    WHERE status = 'active';

-- FK tertunda dari migrasi 0004.
ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_organization_fk
    FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE;

ALTER TABLE usage_counters
    ADD CONSTRAINT usage_counters_organization_fk
    FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE;

COMMENT ON TABLE organizations IS 'Tenant utama. Semua data bisnis terikat ke organization_id (ADR-008).';