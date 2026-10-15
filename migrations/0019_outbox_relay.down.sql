-- 0019 down — kembalikan outbox_events ke bentuk migrasi 0006.

drop index if exists outbox_events_stale_claim_idx;

alter table outbox_events drop column if exists claimed_by;
alter table outbox_events drop column if exists claimed_at;

-- Baris yang sedang 'processing' saat rollback tidak dikenal skema lama;
-- kembalikan ke 'pending' agar tidak menjadi baris yang tak pernah terbit.
update outbox_events set status = 'pending' where status = 'processing';

alter table outbox_events drop constraint if exists outbox_events_status_check;
alter table outbox_events add constraint outbox_events_status_check
    check (status in ('pending', 'published', 'failed', 'dead'));