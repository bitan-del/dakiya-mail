-- Saved segments: a named contacts filter (the same shape POST /contacts/search
-- takes) that is evaluated each time it is used, so a segment always reflects
-- the contacts that match it now rather than a frozen list.

CREATE TABLE IF NOT EXISTS contact_segments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    description     text NOT NULL DEFAULT '',
    filter          jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_segments_org_name
    ON contact_segments (organization_id, lower(name));
