-- Sender domains: a domain the workspace sends from, the provider it sends
-- through, and the last DNS check of its SPF, DKIM and DMARC records. The
-- dashboard turns these into the exact records to publish.
--
--   provider       google_workspace, microsoft_365, amazon_ses or other
--   dkim_selectors selectors to probe; for Amazon SES, the three Easy DKIM tokens

CREATE TABLE IF NOT EXISTS sending_domains (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    domain          text NOT NULL,
    provider        text NOT NULL DEFAULT 'other'
                    CHECK (provider IN ('google_workspace', 'microsoft_365', 'amazon_ses', 'other')),
    dkim_selectors  text[] NOT NULL DEFAULT '{}',
    state           text NOT NULL DEFAULT 'pending'
                    CHECK (state IN ('pending', 'passing', 'failing', 'unknown')),
    spf_found       boolean NOT NULL DEFAULT false,
    spf_record      text NOT NULL DEFAULT '',
    dkim_found      boolean NOT NULL DEFAULT false,
    dmarc_found     boolean NOT NULL DEFAULT false,
    dmarc_policy    text NOT NULL DEFAULT '',
    check_summary   text NOT NULL DEFAULT '',
    last_checked_at timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, domain)
);
