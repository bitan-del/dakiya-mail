DROP INDEX IF EXISTS idx_email_accounts_brand;
ALTER TABLE email_accounts DROP COLUMN IF EXISTS brand_id;
DROP TABLE IF EXISTS brands;
