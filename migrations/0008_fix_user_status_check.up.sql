-- 0008 — Perbaiki CHECK constraint status pengguna
--
-- Migrasi 0002 hanya mengizinkan ('active','suspended','deleted'), padahal
-- domain identitas memakai status 'pending' untuk akun yang belum
-- memverifikasi email (lihat dev-docs/modules/identity/models.md).
-- Akibatnya setiap pendaftaran gagal dengan pelanggaran CHECK.
--
-- Perbaikan dilakukan lewat migrasi BARU, bukan dengan mengedit 0002:
-- migrasi yang sudah diterapkan tidak boleh berubah, karena itu akan membuat
-- skema berbeda antara database yang sudah jalan dan yang baru dibuat.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;

ALTER TABLE users
    ADD CONSTRAINT users_status_check
    CHECK (status IN ('pending', 'active', 'suspended', 'deleted'));

COMMENT ON COLUMN users.status IS
    'pending = terdaftar, email belum diverifikasi; active = dapat login; suspended = diblokir; deleted = ditutup.';