package http

import (
	"strings"

	"gamepanel/forge/internal/services/i18n"

	"github.com/gofiber/fiber/v2"
)

func I18nMiddleware(translator *i18n.TranslationService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		locale := resolveLocale(c, translator)
		c.Locals("locale", locale)
		c.Locals("translator", translator)
		return c.Next()
	}
}

func resolveLocale(c *fiber.Ctx, translator *i18n.TranslationService) string {
	if locale := c.Query("locale"); locale != "" {
		return locale
	}

	acceptLang := c.Get("Accept-Language")
	if acceptLang != "" {
		locales := strings.Split(acceptLang, ",")
		if len(locales) > 0 {
			lang := strings.TrimSpace(strings.Split(locales[0], ";")[0])
			if len(lang) >= 2 {
				locale := lang[:2]
				for _, l := range translator.AvailableLocales() {
					if l == locale {
						return locale
					}
				}
			}
		}
	}

	return translator.AvailableLocales()[0]
}

func T(c *fiber.Ctx, key string, args ...any) string {
	translator, ok := c.Locals("translator").(*i18n.TranslationService)
	if !ok {
		return key
	}
	locale, ok := c.Locals("locale").(string)
	if !ok {
		locale = translator.AvailableLocales()[0]
	}
	return translator.T(locale, key, args...)
}
