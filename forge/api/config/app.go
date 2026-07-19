package config

type App struct {
	Env            string
	Name           string
	URL            string
	Debug          bool
	Version        string
	Key            string
	Cipher         string
	Locale         string
	FallbackLocale string
}

func AppConfig() App {
	return App{
		Env:            env("APP_ENV", "development"),
		Name:           env("APP_NAME", "GamePanel"),
		URL:            env("APP_URL", "http://localhost:3000"),
		Debug:          envBool("APP_DEBUG", true),
		Version:        env("APP_VERSION", "0.1.0"),
		Key:            env("APP_KEY", ""),
		Cipher:         env("APP_CIPHER", "AES-256-GCM"),
		Locale:         env("APP_LOCALE", "en"),
		FallbackLocale: env("APP_FALLBACK_LOCALE", "en"),
	}
}
