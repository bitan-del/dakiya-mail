ALTER TABLE instance_settings DROP COLUMN IF EXISTS postmaster_last_error;
ALTER TABLE instance_settings DROP COLUMN IF EXISTS postmaster_synced_at;
ALTER TABLE instance_settings DROP COLUMN IF EXISTS postmaster_credentials;

DROP TABLE IF EXISTS postmaster_domain_stats;
