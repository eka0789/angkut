-- 0013 — Seed kurir mock: zona tarif, kurir, layanan, tarif (TASK-022).
-- Semua adapter_type='mock' — provider sungguhan (JNE, J&T, dst.) menyusul
-- di Phase 4 dengan kredensial masing-masing.

-- ── Zona tarif (berbasis provinsi yang sudah di-seed di 0010) ────────────────
INSERT INTO zones (code, name, type, "values") VALUES
('ZONA_JABODETABEK', 'Jabodetabek', 'province', ARRAY['31','32','36']),
('ZONA_JAWA',        'Jawa (tengah-timur)', 'province', ARRAY['33','34','35']),
('ZONA_SUMATERA',    'Sumatera (utara)', 'province', ARRAY['11','12','13','14']);

-- ── Kurir mock ────────────────────────────────────────────────────────────────
INSERT INTO couriers (id, code, name, adapter_type, is_active) VALUES
('01930000-0000-7000-8000-000000000101', 'jne',     'JNE',     'mock', true),
('01930000-0000-7000-8000-000000000102', 'jnt',     'J&T Express', 'mock', true),
('01930000-0000-7000-8000-000000000103', 'sicepat', 'SiCepat', 'mock', true);

-- ── Layanan ───────────────────────────────────────────────────────────────────
INSERT INTO courier_services (
    id, courier_id, code, name,
    supports_cod, supports_pickup, supports_insurance,
    eta_min_days, eta_max_days, volumetric_divisor, reliability_score
) VALUES
('01930000-0000-7000-8000-000000000201', '01930000-0000-7000-8000-000000000101', 'REG', 'Reguler',
 true, true, true, 2, 3, 6000, 92.00),
('01930000-0000-7000-8000-000000000202', '01930000-0000-7000-8000-000000000101', 'YES', 'Yakin Esok Sampai',
 false, true, true, 1, 1, 6000, 90.00),
('01930000-0000-7000-8000-000000000203', '01930000-0000-7000-8000-000000000102', 'EZ', 'Economy Express',
 true, false, false, 2, 4, 6000, 88.00),
('01930000-0000-7000-8000-000000000204', '01930000-0000-7000-8000-000000000102', 'REG', 'Reguler',
 true, true, true, 1, 3, 6000, 89.00),
('01930000-0000-7000-8000-000000000205', '01930000-0000-7000-8000-000000000103', 'BEST', 'Best',
 true, true, true, 1, 2, 6000, 95.00),
('01930000-0000-7000-8000-000000000206', '01930000-0000-7000-8000-000000000103', 'REG', 'SiCepat Reguler',
 true, false, true, 2, 3, 6000, 91.00);

-- ── Tarif: matriks pasangan zona × tier berat × faktor kurir ─────────────────
-- Tier: [0,1) kg, [1,5) kg, ≥5 kg (max NULL = tanpa batas). Harga dibulatkan
-- ke ratusan terdekat agar mirip tarif nyata.
WITH matrix (origin_zone, destination_zone, base) AS (
    VALUES
        ('ZONA_JABODETABEK', 'ZONA_JABODETABEK', 9000),
        ('ZONA_JABODETABEK', 'ZONA_JAWA',        12000),
        ('ZONA_JABODETABEK', 'ZONA_SUMATERA',    22000),
        ('ZONA_JAWA',        'ZONA_JABODETABEK', 12000),
        ('ZONA_JAWA',        'ZONA_JAWA',        10000),
        ('ZONA_JAWA',        'ZONA_SUMATERA',    24000),
        ('ZONA_SUMATERA',    'ZONA_JABODETABEK', 22000),
        ('ZONA_SUMATERA',    'ZONA_JAWA',        24000),
        ('ZONA_SUMATERA',    'ZONA_SUMATERA',    18000)
),
tiers (tier, min_w, max_w, adder) AS (
    VALUES
        (1, 0::numeric,  1::numeric,       0::bigint),
        (2, 1::numeric,  5::numeric,    4000::bigint),
        (3, 5::numeric,  NULL::numeric, 9000::bigint)
),
factors (courier_code, factor) AS (
    VALUES
        ('jne',     1.00::numeric),
        ('jnt',     0.93::numeric),
        ('sicepat', 0.96::numeric)
)
INSERT INTO courier_rates (
    id, courier_service_id, origin_zone, destination_zone,
    min_weight, max_weight, price, effective_from
)
SELECT
    gen_random_uuid(),
    s.id,
    m.origin_zone,
    m.destination_zone,
    t.min_w,
    t.max_w,
    (round((m.base + t.adder) * f.factor / 100) * 100)::bigint,
    now()
FROM courier_services s
JOIN couriers c ON c.id = s.courier_id
CROSS JOIN matrix m
CROSS JOIN tiers t
JOIN factors f ON f.courier_code = c.code
WHERE c.adapter_type = 'mock' AND s.is_active;
