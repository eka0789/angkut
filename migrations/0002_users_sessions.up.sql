-- 0002 — Identitas: users, user_sessions
-- UUID v7 dihasilkan application layer (platform.NewUUID) agar berurutan waktu.
-- Tidak ada kolom rahasia plaintext: password hanya hash Argon2id.

CREATE TABLE users (
    id                  uuid PRIMARY KEY,
    email               text        NOT NULL,
    email_verified_at   timestamptz,
    password_hash       text,                        -- NULL bila hanya OAuth
    name                text        NOT NULL,
    phone               text,
    avatar_url          text,
    locale              text        NOT NULL DEFAULT 'id',
    timezone            text        NOT NULL DEFAULT 'Asia/Jakarta',
    status              text        NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'suspended', 'deleted')),
    last_login_at       timestamptz,
    failed_login_count  int         NOT NULL DEFAULT 0,
    locked_until        timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz,
    CONSTRAINT users_email_not_blank CHECK (length(btrim(email)) > 0),
    CONSTRAINT users_name_not_blank  CHECK (length(btrim(name)) > 0)
);

-- Email unik tanpa membedakan huruf besar/kecil, hanya untuk akun aktif.
CREATE UNIQUE INDEX users_email_unique_active
    ON users (lower(email))
    WHERE deleted_at IS NULL;

-- Pencarian nama pengguna (admin) memakai trigram.
CREATE INDEX users_name_trgm_idx ON users USING gin (name gin_trgm_ops);

CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Sesi = refresh token yang dapat dicabut per perangkat (ADR-007).
CREATE TABLE user_sessions (
    id              uuid PRIMARY KEY,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_token_hash text     NOT NULL,            -- hash, BUKAN token mentah
    device_name     text,
    user_agent      text,
    ip_address      inet,
    expires_at      timestamptz NOT NULL,
    revoked_at      timestamptz,
    replaced_by_id  uuid        REFERENCES user_sessions (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    last_used_at    timestamptz
);

-- Rotasi refresh: satu token hanya boleh dipakai sekali; cari hash dengan cepat.
CREATE UNIQUE INDEX user_sessions_token_hash_unique ON user_sessions (refresh_token_hash);
CREATE INDEX user_sessions_user_active_idx ON user_sessions (user_id, expires_at)
    WHERE revoked_at IS NULL;

COMMENT ON COLUMN user_sessions.refresh_token_hash IS 'SHA-256 dari refresh token; token mentah tidak pernah disimpan.';