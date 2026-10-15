-- 0017 down — hapus processed_events lalu tracking_events.

drop index if exists idx_processed_events_consumer_time;
drop table if exists processed_events;

drop index if exists idx_tracking_events_status_time;
drop index if exists idx_tracking_events_org_time;
drop index if exists idx_tracking_events_shipment_time;
drop index if exists uq_tracking_event_natural;
drop table if exists tracking_events;