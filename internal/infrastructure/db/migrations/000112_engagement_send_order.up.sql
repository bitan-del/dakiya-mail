-- Per-campaign send order (engagement-first) and sunset policy; defaults keep today's behavior.
-- No index: the engagement lookup reads campaign_contact_progress by contact_id (idx_campaign_progress_contact).
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS send_order text NOT NULL DEFAULT 'default'
    CHECK (send_order IN ('default', 'engaged_first'));
-- 0 = off; N skips contacts with N+ campaign emails and no human open, click or reply.
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS sunset_after_sends integer NOT NULL DEFAULT 0
    CHECK (sunset_after_sends >= 0 AND sunset_after_sends <= 50);
