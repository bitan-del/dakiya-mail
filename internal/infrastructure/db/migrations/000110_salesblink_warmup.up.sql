-- SalesBlink warmup: hand warmup for connected mailboxes to a SalesBlink
-- account instead of (or alongside) the local pool.
--
-- Shape mirrors cloud_link deliberately: one row holds the account-level link,
-- a second table maps each local mailbox to its remote SalesBlink sender.
--
--   salesblink_link       the single API key, sealed with
--                         CREDENTIALS_ENCRYPTION_KEY, plus the warmup defaults
--                         applied to every mailbox we enroll
--   salesblink_mailboxes  local mailboxes enrolled in SalesBlink warmup and
--                         the sender id SalesBlink gave back

CREATE TABLE IF NOT EXISTS salesblink_link (
    id             boolean PRIMARY KEY DEFAULT true CHECK (id),
    base_url       text NOT NULL DEFAULT 'https://run.salesblink.io/api/public/v1.0.0',
    -- Sealed with the mailbox credential key, never stored in plaintext.
    api_key        text NOT NULL,
    -- When true, every eligible mailbox is enrolled as soon as it connects.
    auto_enroll    boolean NOT NULL DEFAULT true,

    -- Warmup defaults applied on enrollment. SalesBlink caps each of these at
    -- 50/day per sender, so the CHECKs match its schema rather than our own.
    warmup_per_day      integer NOT NULL DEFAULT 5  CHECK (warmup_per_day BETWEEN 1 AND 50),
    max_daily_frequency integer NOT NULL DEFAULT 30 CHECK (max_daily_frequency BETWEEN 1 AND 50),
    ramp_up_frequency   integer NOT NULL DEFAULT 1  CHECK (ramp_up_frequency BETWEEN 1 AND 50),
    auto_ramp_up        boolean NOT NULL DEFAULT true,

    connected_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    connected_at   timestamptz NOT NULL DEFAULT now(),
    last_synced_at timestamptz,
    last_error     text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS salesblink_mailboxes (
    email_account_id uuid PRIMARY KEY REFERENCES email_accounts (id) ON DELETE CASCADE,
    -- SalesBlink's own sender id; opaque string, not a uuid.
    sender_id        text NOT NULL,
    enrolled_at      timestamptz NOT NULL DEFAULT now(),
    -- Last values pulled from /senders/{id}/health and /warmup-stats.
    health_score     integer,
    last_stats_at    timestamptz,
    last_error       text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_salesblink_mailboxes_sender ON salesblink_mailboxes (sender_id);
