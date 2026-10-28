-- Brands: one workspace sending as several identities (an agency's clients, or
-- a company's product lines). A mailbox belongs to at most one brand, and its
-- campaign mail carries that brand's footer: the name, the postal address
-- CAN-SPAM requires, an optional line, and an unsubscribe link.

CREATE TABLE IF NOT EXISTS brands (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name             text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    website          text NOT NULL DEFAULT '',
    physical_address text NOT NULL DEFAULT '',
    footer_text      text NOT NULL DEFAULT '',
    footer_enabled   boolean NOT NULL DEFAULT true,
    unsubscribe_link boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_brands_org_name ON brands (organization_id, lower(name));

ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS brand_id uuid REFERENCES brands (id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_email_accounts_brand ON email_accounts (brand_id) WHERE brand_id IS NOT NULL;
