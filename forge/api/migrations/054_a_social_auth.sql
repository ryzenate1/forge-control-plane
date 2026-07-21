-- Social authentication providers configuration
CREATE TABLE IF NOT EXISTS social_providers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    display_name text NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    client_id text NOT NULL DEFAULT '',
    client_secret text NOT NULL DEFAULT '',
    scopes text[] NOT NULL DEFAULT '{}',
    button_style text NOT NULL DEFAULT 'brand_social',
    icon_class text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Social identities linked to user accounts
CREATE TABLE IF NOT EXISTS social_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider text NOT NULL,
    provider_id text NOT NULL,
    provider_name text NOT NULL DEFAULT '',
    avatar_url text,
    profile_url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_id)
);

CREATE INDEX IF NOT EXISTS idx_social_identities_user_id ON social_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_social_identities_provider ON social_identities(provider, provider_id);

-- Seed default social providers
INSERT INTO social_providers (name, display_name, enabled, button_style, icon_class) VALUES
    ('discord', 'Discord', false, 'brand_social', 'discord'),
    ('steam', 'Steam', false, 'brand_social', 'steam'),
    ('authentik', 'Authentik', false, 'primary', 'shield')
ON CONFLICT (name) DO NOTHING;
