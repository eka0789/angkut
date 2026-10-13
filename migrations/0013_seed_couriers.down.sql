-- 0013 — rollback seed kurir mock
DELETE FROM courier_rates WHERE courier_service_id IN (
    SELECT id FROM courier_services WHERE courier_id IN (
        SELECT id FROM couriers WHERE adapter_type = 'mock'
    )
);
DELETE FROM courier_services WHERE courier_id IN (
    SELECT id FROM couriers WHERE adapter_type = 'mock'
);
DELETE FROM couriers WHERE adapter_type = 'mock';
DELETE FROM zones WHERE code IN ('ZONA_JABODETABEK', 'ZONA_JAWA', 'ZONA_SUMATERA');
