-- Normalize Authentik configuration. Earlier versions overloaded client_id,
-- client_secret, and scopes to hold its issuer URL and OAuth credentials.
ALTER TABLE social_providers
    ADD COLUMN IF NOT EXISTS issuer_url text NOT NULL DEFAULT '';

UPDATE social_providers
SET
    issuer_url = client_id,
    client_id = client_secret,
    client_secret = CASE
        WHEN cardinality(scopes) > 0 THEN scopes[cardinality(scopes)]
        ELSE ''
    END,
    scopes = ARRAY['openid', 'email', 'profile']
WHERE name = 'authentik'
  AND issuer_url = '';
