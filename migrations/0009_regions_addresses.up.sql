-- 0009 — Regions (wilayah) & addresses: hierarki provinsi/kota/kecamatan/desa + alamat tenant
-- Wilayah bersifat publik (reference data), alamat bersifat tenant-scoped.

-- Provinsi (level 1)
CREATE TABLE provinces (
    id              text PRIMARY KEY,           -- 2 digit: 31, 32, 33, ...
    name            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX provinces_name_trgm_idx ON provinces USING gin (name gin_trgm_ops);

-- Kota/Kabupaten (level 2)
CREATE TABLE cities (
    id              text PRIMARY KEY,           -- 4 digit: 3101, 3201, 3371, ...
    province_id     text NOT NULL REFERENCES provinces (id),
    name            text NOT NULL,
    type            text NOT NULL               -- kota, kabupaten
                    CHECK (type IN ('kota', 'kabupaten')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cities_province_idx ON cities (province_id);
CREATE INDEX cities_name_trgm_idx ON cities USING gin (name gin_trgm_ops);

-- Kecamatan (level 3)
CREATE TABLE districts (
    id              text PRIMARY KEY,           -- 6 digit: 310101, 320101, ...
    city_id         text NOT NULL REFERENCES cities (id),
    name            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX districts_city_idx ON districts (city_id);
CREATE INDEX districts_name_trgm_idx ON districts USING gin (name gin_trgm_ops);

-- Desa/Kelurahan (level 4)
CREATE TABLE villages (
    id              text PRIMARY KEY,           -- 10 digit: 3101011001, ...
    district_id     text NOT NULL REFERENCES districts (id),
    name            text NOT NULL,
    type            text NOT NULL               -- desa, kelurahan
                    CHECK (type IN ('desa', 'kelurahan')),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX villages_district_idx ON villages (district_id);
CREATE INDEX villages_name_trgm_idx ON villages USING gin (name gin_trgm_ops);

-- Kode pos
CREATE TABLE postal_codes (
    code            text PRIMARY KEY,           -- 5 digit: 50241, 40111, ...
    district_id     text NOT NULL REFERENCES districts (id),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX postal_codes_district_idx ON postal_codes (district_id);

-- Alamat tenant-scoped (pelanggan, gudang, alamat pengirim/penerima)
CREATE TABLE addresses (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    
    -- Identitas penerima/lokasi
    recipient       text NOT NULL,
    phone           text NOT NULL,
    
    -- Region hierarchy
    province_id     text NOT NULL REFERENCES provinces (id),
    city_id         text NOT NULL REFERENCES cities (id),
    district_id     text NOT NULL REFERENCES districts (id),
    village_id      text REFERENCES villages (id),
    postal_code     text,
    
    -- Detail alamat
    street          text NOT NULL,              -- Jalan lengkap
    notes           text,                       -- Patokan/catatan kurir
    
    -- Map pin (optional)
    latitude        numeric(10,7),
    longitude       numeric(10,7),
    
    -- Label & status
    label           text,                       -- "Kantor Pusat", "Gudang Jakarta", etc
    is_default      boolean NOT NULL DEFAULT false,
    status          text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'archived')),
    
    created_by      uuid NOT NULL REFERENCES users (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    
    CONSTRAINT addresses_recipient_not_blank CHECK (length(btrim(recipient)) > 0),
    CONSTRAINT addresses_phone_not_blank CHECK (length(btrim(phone)) > 0),
    CONSTRAINT addresses_street_not_blank CHECK (length(btrim(street)) > 0),
    CONSTRAINT addresses_lat_range CHECK (latitude BETWEEN -90 AND 90),
    CONSTRAINT addresses_lng_range CHECK (longitude BETWEEN -180 AND 180),
    CONSTRAINT addresses_pin_complete CHECK (
        (latitude IS NULL AND longitude IS NULL) OR 
        (latitude IS NOT NULL AND longitude IS NOT NULL)
    )
);

CREATE INDEX addresses_org_idx ON addresses (organization_id, status);
CREATE INDEX addresses_region_idx ON addresses (city_id, district_id);
CREATE INDEX addresses_recipient_trgm_idx ON addresses USING gin (recipient gin_trgm_ops);
CREATE INDEX addresses_street_trgm_idx ON addresses USING gin (street gin_trgm_ops);

CREATE TRIGGER addresses_set_updated_at
    BEFORE UPDATE ON addresses
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE provinces IS 'Reference data publik: provinsi Indonesia.';
COMMENT ON TABLE cities IS 'Reference data publik: kota/kabupaten.';
COMMENT ON TABLE districts IS 'Reference data publik: kecamatan.';
COMMENT ON TABLE villages IS 'Reference data publik: desa/kelurahan.';
COMMENT ON TABLE postal_codes IS 'Reference data publik: kode pos.';
COMMENT ON TABLE addresses IS 'Alamat tenant-scoped: pelanggan, gudang, pengirim/penerima.';
