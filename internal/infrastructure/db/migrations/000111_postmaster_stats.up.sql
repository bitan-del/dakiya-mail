-- Google Postmaster Tools: Gmail's own daily view of each sending domain.
--
-- postmaster_domain_stats is one row per organization, domain and day, pulled
-- from the Gmail Postmaster Tools API for the domains of the organization's
-- mailboxes. Ratios are 0..1 and NULL when Gmail reported nothing that day
-- (low-volume domains have no data). raw keeps the full upstream payload
-- (IP reputation, delivery errors, encryption) for fields not yet charted.
--
-- The Google OAuth credentials are instance-wide, not workspace data. They live
-- next to the other instance settings, sealed with CREDENTIALS_ENCRYPTION_KEY,
-- in their own column so a write of the settings document can never clobber
-- them and the document the admin panel reads never carries them.

CREATE TABLE IF NOT EXISTS postmaster_domain_stats (
    organization_id   uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    domain            text NOT NULL,
    date              date NOT NULL,
    domain_reputation text CHECK (domain_reputation IN ('high', 'medium', 'low', 'bad')),
    spam_rate         double precision,
    spf_ratio         double precision,
    dkim_ratio        double precision,
    dmarc_ratio       double precision,
    raw               jsonb NOT NULL DEFAULT '{}'::jsonb,
    synced_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, domain, date)
);

ALTER TABLE instance_settings ADD COLUMN IF NOT EXISTS postmaster_credentials text NOT NULL DEFAULT '';
ALTER TABLE instance_settings ADD COLUMN IF NOT EXISTS postmaster_synced_at timestamptz;
ALTER TABLE instance_settings ADD COLUMN IF NOT EXISTS postmaster_last_error text NOT NULL DEFAULT '';
