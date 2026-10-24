-- Warmup thread messages: the subject and body of each warmup mail the live
-- writer sends, so the mailbox that answers it reads the message it is
-- answering instead of replaying a pre-written turn, and so a mailbox can be
-- compared against its own recent mail before it repeats itself.
--
-- A thread is opened by one pool mailbox and answered by another, usually in a
-- different workspace, so the text is sealed with the instance credential key
-- (CREDENTIALS_ENCRYPTION_KEY) rather than an organization DEK. Rows are
-- scratch context for warmup traffic, not customer data, and are pruned after
-- 30 days by the warmup generation job.
--
--   thread_id  minted per thread and recorded as warmup_tokens.conversation_id
--   turn       0 for the opening, +1 for each reply
--   direction  initiator: sent by the mailbox that opened the thread
--              responder: sent by the mailbox it was addressed to

CREATE TABLE IF NOT EXISTS warmup_thread_messages (
    thread_id            uuid NOT NULL,
    turn                 integer NOT NULL CHECK (turn >= 0),
    direction            text NOT NULL CHECK (direction IN ('initiator', 'responder')),
    sender_account_id    uuid NOT NULL REFERENCES email_accounts (id) ON DELETE CASCADE,
    recipient_account_id uuid NOT NULL REFERENCES email_accounts (id) ON DELETE CASCADE,
    subject_enc          text NOT NULL,
    body_enc             text NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, turn, direction)
);

CREATE INDEX IF NOT EXISTS idx_warmup_thread_messages_sender
    ON warmup_thread_messages (sender_account_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_warmup_thread_messages_created
    ON warmup_thread_messages (created_at);
