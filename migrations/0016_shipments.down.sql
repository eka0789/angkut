-- 0016 down — hapus packages, items, lalu shipments.

drop index if exists idx_shipment_packages_shipment;
drop index if exists uq_shipment_package_seq;
drop table if exists shipment_packages;

drop index if exists idx_shipment_items_org;
drop index if exists idx_shipment_items_shipment;
drop table if exists shipment_items;

drop trigger if exists shipments_set_updated_at on shipments;
drop index if exists idx_shipments_org_delivered;
drop index if exists idx_shipments_awb_trgm;
drop index if exists idx_shipments_order;
drop index if exists idx_shipments_org_status;
drop index if exists uq_shipment_idem;
drop index if exists uq_shipment_awb;
drop table if exists shipments;