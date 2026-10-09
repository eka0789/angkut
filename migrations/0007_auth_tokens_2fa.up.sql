-- 0007 — Autentikasi lanjutan: 2FA, reset password, verifikasi email
--
-- Tiga kelas token sekali-pakai yang semuanya disimpan sebagai HASH, bukan
-- nilai mentah. Token mentah hanya pernah ada di email/response ke pengguna.
-- Setiap tabel menyimpan `expires_at` + `used_at` sehingga replay dapat ditolak.

-- ── 2FA pada users ────────────────────────────────────────────────────────
ALTER TABLE users
    ADD COLUMN twofa_enabled     boolean NOT NULL DEFAULT false,
    ADD COLUMN twofa_secret_enc  bytea,        -- TOTP secret, terenkripsi AES-GCM
    ADD COLUMN twofa_confirmed_at timestamptz,
    ADD COLUMN password_changed_at timestamptz;

COMMENT ON COLUMN users.twofa_secret_enc IS
    'TOTP secret terenkripsi (AES-GCM). Kunci enkripsi dari env, bukan dari DB.';
COMMENT ON COLUMN users.password_changed_at IS
    'Dipakai untuk invalidasi token/sesi yang diterbitkan sebelum perubahan password.';

-- 2FA hanya boleh aktif bila secret benar-benar ada.
ALTER TABLE users
    ADD CONSTRAINT users_twofa_needs_secret
    CHECK (NOT twofa_enabled OR twofa_secret_enc IS NOT NULL);

-- ── Token reset password ─────────────────────────────────────────────────
CREATE TABLE password_reset_tokens (
    id          uuid PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  text        NOT NULL,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    requested_ip inet,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Lookup token saat dipakai; unik agar hash tidak pernah bertabrakan.
CREATE UNIQUE INDEX password_reset_tokens_hash_unique
    ON password_reset_tokens (token_hash);
CREATE INDEX password_reset_tokens_user_idx
    ON password_reset_tokens (user_id, created_at DESC);
-- Hanya satu token aktif per user yang boleh ada pada satu waktu.
CREATE UNIQUE INDEX password_reset_tokens_one_active_per_user
    ON password_reset_tokens (user_id)
    WHERE used_at IS NULL;

-- ── Token verifikasi email ───────────────────────────────────────────────
CREATE TABLE email_verification_tokens (
    id          uuid PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email       text        NOT NULL,            -- email yang diverifikasi saat itu
    token_hash  text        NOT NULL,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX email_verification_tokens_hash_unique
    ON email_verification_tokens (token_hash);
CREATE INDEX email_verification_tokens_user_idx
    ON email_verification_tokens (user_id, created_at DESC);
CREATE UNIQUE INDEX email_verification_tokens_one_active_per_user
    ON email_verification_tokens (user_id)
    WHERE used_at IS NULL;

-- ── Recovery codes 2FA ───────────────────────────────────────────────────
-- Kode juga di-hash: siapa pun yang membaca DB tidak bisa memakai 2FA user.
CREATE TABLE twofa_recovery_codes (
    id         uuid PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  text        NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX twofa_recovery_codes_user_idx
    ON twofa_recovery_codes (user_id)
    WHERE used_at IS NULL;

-- ── Identitas OAuth (Google) ─────────────────────────────────────────────
-- Dipisah dari users agar satu akun bisa punya beberapa penyedia, dan agar
-- linking (email sudah ada -> sambungkan) dapat diaudit.
CREATE TABLE user_oauth_accounts (
    id                uuid PRIMARY KEY,
    user_id           uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider          text        NOT NULL,       -- google
    provider_user_id  text        NOT NULL,       -- 'sub' dari id_token
    email             text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_login_at     timestamptz,
    -- Satu identitas penyedia hanya boleh terhubung ke satu akun.
    CONSTRAINT user_oauth_accounts_unique UNIQUE (provider, provider_user_id)
);

CREATE INDEX user_oauth_accounts_user_idx ON user_oauth_accounts (user_id);