-- WebAuthn/FIDO2 passwordless authentication credentials
CREATE TABLE IF NOT EXISTS webauthn_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id bytea NOT NULL,
    public_key bytea NOT NULL,
    attestation_type text NOT NULL DEFAULT '',
    aaguid bytea NOT NULL DEFAULT '\x00000000000000000000000000000000',
    sign_count bigint NOT NULL DEFAULT 0,
    clone_warning boolean NOT NULL DEFAULT false,
    name text NOT NULL DEFAULT 'Security Key',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user_id ON webauthn_credentials(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_webauthn_credentials_credential_id ON webauthn_credentials(credential_id);
