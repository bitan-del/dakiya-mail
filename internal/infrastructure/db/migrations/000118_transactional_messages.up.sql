-- Transactional sends made through POST /transactional/send: which template
-- they used and the caller's own track ids, echoed back in delivery webhooks so
-- an integrator can tie every event to its own records.

CREATE TABLE IF NOT EXISTS transactional_messages (
    task_id         uuid PRIMARY KEY REFERENCES tasks (id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    template_id     uuid,
    recipient       text NOT NULL,
    track_ids       text[] NOT NULL DEFAULT '{}' CHECK (cardinality(track_ids) <= 5),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_transactional_messages_org_created
    ON transactional_messages (organization_id, created_at DESC);
