-- 0001 down — cabut ekstensi & fungsi dasar.
-- Hati-hati: mencabut pg_trgm/pgcrypto akan merusak indeks/tabel yang
-- bergantung padanya, sehingga urutan migrasi down yang benar berarti migrasi
-- lain dicabut lebih dulu.

DROP FUNCTION IF EXISTS set_updated_at();
DROP EXTENSION IF EXISTS "unaccent";
DROP EXTENSION IF EXISTS "pg_trgm";
DROP EXTENSION IF EXISTS "pgcrypto";