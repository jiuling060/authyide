-- Email registration requires inbox verification before the account can sign in
-- with a password. Pre-existing accounts created by admin or through OIDC /
-- OAuth already hold a trusted identity, so they ship as verified.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified boolean DEFAULT true NOT NULL;
