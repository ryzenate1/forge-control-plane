package config

type Services struct {
	Discord   OAuthConfig
	Steam     SteamConfig
	Authentik OAuthConfig
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Enabled      bool
}

type SteamConfig struct {
	APIKey  string
	Enabled bool
}

func ServicesConfig() Services {
	return Services{
		Discord: OAuthConfig{
			ClientID:     env("DISCORD_CLIENT_ID", ""),
			ClientSecret: env("DISCORD_CLIENT_SECRET", ""),
			RedirectURL:  env("DISCORD_REDIRECT_URL", ""),
			Enabled:      envBool("DISCORD_ENABLED", false),
		},
		Steam: SteamConfig{
			APIKey:  env("STEAM_API_KEY", ""),
			Enabled: envBool("STEAM_ENABLED", false),
		},
		Authentik: OAuthConfig{
			ClientID:     env("AUTHENTIK_CLIENT_ID", ""),
			ClientSecret: env("AUTHENTIK_CLIENT_SECRET", ""),
			RedirectURL:  env("AUTHENTIK_REDIRECT_URL", ""),
			Enabled:      envBool("AUTHENTIK_ENABLED", false),
		},
	}
}
