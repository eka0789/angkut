-- 0014 down — hapus stores lalu warehouses (urutan terbalik dari up karena
-- stores.warehouse_id mereferensi warehouses).

drop trigger if exists stores_set_updated_at on stores;
drop index if exists stores_org_idx;
drop table if exists stores;

drop trigger if exists warehouses_set_updated_at on warehouses;
drop index if exists warehouses_org_default_idx;
drop index if exists warehouses_org_idx;
drop table if exists warehouses;