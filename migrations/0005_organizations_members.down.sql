-- 0005 down — lepas FK tertunda lebih dulu, lalu hapus tabel tenant.
ALTER TABLE IF EXISTS usage_counters
    DROP CONSTRAINT IF EXISTS usage_counters_organization_fk;
ALTER TABLE IF EXISTS subscriptions
    DROP CONSTRAINT IF EXISTS subscriptions_organization_fk;

DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;