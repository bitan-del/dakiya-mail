-- Hashed suppression lists: advertiser and partner suppression files shipped as
-- MD5 or SHA-256 digests of the lowercased, trimmed address, so the plaintext
-- never has to be shared. Every send path matches a recipient against these
-- as well as suppressed_recipients.
--
--   algo       md5 or sha256
--   hash       lowercase hex digest of lower(trim(email))
--   list_name  the file or advertiser the digest came from, for removal

CREATE TABLE IF NOT EXISTS suppressed_hashes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    algo            text NOT NULL CHECK (algo IN ('md5', 'sha256')),
    hash            text NOT NULL,
    list_name       text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, algo, hash)
);

CREATE INDEX IF NOT EXISTS idx_suppressed_hashes_org_list
    ON suppressed_hashes (organization_id, list_name, algo);
