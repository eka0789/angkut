-- 0008 down — kembalikan constraint ke bentuk sebelumnya.
-- CATATAN: rollback ini akan gagal bila sudah ada baris berstatus 'pending'.
-- Itu memang benar: membalikkan migrasi tidak boleh diam-diam menghapus data.
-- Untuk rollback di lingkungan berisi data, promosikan akun pending lebih dulu.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;

ALTER TABLE users
    ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'suspended', 'deleted'));