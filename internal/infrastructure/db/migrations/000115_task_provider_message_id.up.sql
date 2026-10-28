-- The id the sending provider assigned to a message when it differs from our
-- own Message-ID. Amazon SES answers the SMTP DATA command with "250 Ok <id>"
-- and its delivery, bounce and complaint notifications name that id, not ours,
-- so this is what an SES event resolves its task through.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS provider_message_id text;

CREATE INDEX IF NOT EXISTS idx_tasks_provider_message_id
    ON tasks (provider_message_id) WHERE provider_message_id IS NOT NULL;
