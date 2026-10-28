-- Hosted signup forms: a public page at /f/<slug> that adds the people who fill
-- it in as contacts, optionally only after they confirm by email (double opt-in).
--
--   fields          optional fields shown besides email: first_name, last_name, company, phone
--   email_account_id the mailbox that sends double opt-in confirmations
--   created_by      the member the contacts are created under

CREATE TABLE IF NOT EXISTS signup_forms (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    name             text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    slug             text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{2,62}$'),
    title            text NOT NULL DEFAULT '',
    description      text NOT NULL DEFAULT '',
    fields           text[] NOT NULL DEFAULT '{}',
    category_ids     uuid[] NOT NULL DEFAULT '{}',
    double_opt_in    boolean NOT NULL DEFAULT false,
    email_account_id uuid REFERENCES email_accounts (id) ON DELETE SET NULL,
    success_message  text NOT NULL DEFAULT '',
    redirect_url     text NOT NULL DEFAULT '',
    enabled          boolean NOT NULL DEFAULT true,
    submissions      integer NOT NULL DEFAULT 0,
    confirmations    integer NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_signup_forms_org ON signup_forms (organization_id);

-- A double opt-in submission waiting for its confirmation click. token_hash is
-- the sha256 of the emailed token; the token itself is never stored.
CREATE TABLE IF NOT EXISTS signup_form_pending (
    token_hash   text PRIMARY KEY,
    form_id      uuid NOT NULL REFERENCES signup_forms (id) ON DELETE CASCADE,
    email        text NOT NULL,
    data         jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    confirmed_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_signup_form_pending_created ON signup_form_pending (created_at);

ALTER TABLE public.contacts DROP CONSTRAINT IF EXISTS contacts_source_check;
ALTER TABLE public.contacts
    ADD CONSTRAINT contacts_source_check
    CHECK (source IN ('unknown', 'manual', 'campaign', 'import', 'sheet_sync', 'api', 'ai_assistant', 'form'));
