-- 0007 down
DROP TABLE IF EXISTS user_oauth_accounts;
DROP TABLE IF EXISTS twofa_recovery_codes;
DROP TABLE IF EXISTS email_verification_tokens;
DROP TABLE IF EXISTS password_reset_tokens;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_twofa_needs_secret;

ALTER TABLE users
    DROP COLUMN IF EXISTS password_changed_at,
    DROP COLUMN IF EXISTS twofa_confirmed_at,
    DROP COLUMN IF EXISTS twofa_secret_enc,
    DROP COLUMN IF EXISTS twofa_enabled;