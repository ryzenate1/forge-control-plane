package http

import (
	"strings"

	"gamepanel/forge/internal/services/i18n"

	"github.com/gofiber/fiber/v2"
)

var regionToBase = map[string]string{
	"pt-BR": "pt", "pt-PT": "pt",
	"zh-CN": "zh", "zh-TW": "zh", "zh-HK": "zh",
	"es-MX": "es", "es-ES": "es", "es-AR": "es",
	"fr-CA": "fr", "fr-FR": "fr",
	"de-AT": "de", "de-DE": "de", "de-CH": "de",
	"en-US": "en", "en-GB": "en", "en-AU": "en",
	"ja-JP": "ja",
	"ru-RU": "ru", "ru-UA": "ru",
}

func I18nMiddleware(translator *i18n.TranslationService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		locale := resolveLocale(c, translator)
		c.Locals("locale", locale)
		c.Locals("translator", translator)
		return c.Next()
	}
}

func resolveLocale(c *fiber.Ctx, translator *i18n.TranslationService) string {
	available := translator.AvailableLocales()

	if locale := c.Query("locale"); locale != "" {
		for _, l := range available {
			if l == locale {
				return locale
			}
		}
	}

	if locale := c.Cookies("NEXT_LOCALE"); locale != "" {
		for _, l := range available {
			if l == locale {
				return locale
			}
		}
		if base, ok := regionToBase[locale]; ok {
			for _, l := range available {
				if l == base {
					return base
				}
			}
		}
		if base, _, ok := strings.Cut(locale, "-"); ok {
			for _, l := range available {
				if l == base {
					return base
				}
			}
		}
	}

	acceptLang := c.Get("Accept-Language")
	if acceptLang != "" {
		locales := strings.Split(acceptLang, ",")
		if len(locales) > 0 {
			full := strings.TrimSpace(strings.Split(locales[0], ";")[0])
			for _, l := range available {
				if l == full {
					return full
				}
			}
			if base, ok := regionToBase[full]; ok {
				for _, l := range available {
					if l == base {
						return base
					}
				}
			}
			if len(full) >= 2 {
				locale := full[:2]
				for _, l := range available {
					if l == locale {
						return locale
					}
				}
			}
		}
	}

	if len(available) > 0 {
		return available[0]
	}
	return "en"
}

func T(c *fiber.Ctx, key string, args ...any) string {
	translator, ok := c.Locals("translator").(*i18n.TranslationService)
	if !ok {
		return key
	}
	locale, ok := c.Locals("locale").(string)
	if !ok {
		if available := translator.AvailableLocales(); len(available) > 0 {
			locale = available[0]
		} else {
			locale = "en"
		}
	}
	return translator.T(locale, key, args...)
}
