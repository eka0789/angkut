-- 0015 down — hapus order_items lalu orders.

drop trigger if exists orders_set_updated_at on orders;

drop index if exists idx_order_items_org;
drop index if exists idx_order_items_order;
drop table if exists order_items;

drop index if exists idx_orders_customer;
drop index if exists idx_orders_org_status_created;
drop index if exists uq_order_external;
drop index if exists uq_order_number;
drop table if exists orders;