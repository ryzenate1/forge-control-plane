-- Phase 7 (Commerce): ship a GitHub OAuth provider record next to the existing
-- Discord / Steam / Authentik seeds. Credentials are entered by an admin on the
-- Social Login Providers page (admin PUT /admin/social/providers/:id).
INSERT INTO social_providers (name, display_name, enabled, scopes, button_style, icon_class) VALUES
    ('github', 'GitHub', false, '{read:user,user:email}', 'brand_social', 'github')
ON CONFLICT (name) DO NOTHING;