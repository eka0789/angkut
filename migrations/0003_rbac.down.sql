-- 0003 down — hapus RBAC (seed role ikut terhapus bersama tabel).
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;