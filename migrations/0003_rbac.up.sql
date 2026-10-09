-- 0003 — RBAC: roles, permissions, role_permissions
-- Role bersifat platform-level (bukan per organisasi) untuk MVP, sehingga
-- definisi izin seragam. Role default: owner, admin, staff, finance,
-- warehouse, viewer.

CREATE TABLE roles (
    id          uuid PRIMARY KEY,
    code        text        NOT NULL UNIQUE,
    name        text        NOT NULL,
    description text,
    level       int         NOT NULL DEFAULT 0,   -- makin tinggi makin berwenang
    is_system   boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT roles_code_format CHECK (code ~ '^[a-z][a-z0-9_]{1,31}$')
);

CREATE TABLE permissions (
    id          uuid PRIMARY KEY,
    code        text        NOT NULL UNIQUE,      -- mis. shipment.create
    module      text        NOT NULL,
    description text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT permissions_code_format CHECK (code ~ '^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$')
);

CREATE INDEX permissions_module_idx ON permissions (module);

-- Tabel pivot role ↔ permission.
CREATE TABLE role_permissions (
    role_id       uuid NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX role_permissions_permission_idx ON role_permissions (permission_id);

CREATE TRIGGER roles_set_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO roles (id, code, name, description, level, is_system) VALUES
    ('01920000-0000-7000-8000-000000000001', 'owner',     'Owner',     'Pemilik organisasi; akses penuh.',        100, true),
    ('01920000-0000-7000-8000-000000000002', 'admin',     'Admin',     'Administrator operasional.',               90, true),
    ('01920000-0000-7000-8000-000000000003', 'finance',   'Keuangan',  'COD, settlement, dan laporan keuangan.',   70, true),
    ('01920000-0000-7000-8000-000000000004', 'warehouse', 'Gudang',    'Stok, transfer, dan pickup.',             60, true),
    ('01920000-0000-7000-8000-000000000005', 'staff',     'Staf',      'Membuat & mengelola pengiriman.',         50, true),
    ('01920000-0000-7000-8000-000000000006', 'viewer',    'Pengamat',  'Hanya melihat data.',                     10, true);