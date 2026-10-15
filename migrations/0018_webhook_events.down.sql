-- 0018 down — hapus webhook_secrets lalu webhook_events.

drop trigger if exists webhook_secrets_set_updated_at on webhook_secrets;
drop table if exists webhook_secrets;

drop index if exists idx_webhook_events_invalid_sig;
drop index if exists idx_webhook_events_provider_time;
drop index if exists idx_webhook_events_awb;
drop index if exists idx_webhook_events_status;
drop index if exists uq_webhook_provider_hash;
drop index if exists uq_webhook_provider_event;
drop table if exists webhook_events;