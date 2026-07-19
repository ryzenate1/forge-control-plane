export type Locale = 'en' | 'es' | 'fr' | 'de' | 'zh' | 'ja' | 'ru' | 'pt';

export interface I18nConfig {
  defaultLocale: Locale;
  supportedLocales: Locale[];
}

export const DEFAULT_I18N_CONFIG: I18nConfig = {
  defaultLocale: 'en',
  supportedLocales: ['en', 'es', 'fr', 'de', 'zh', 'ja', 'ru', 'pt'],
};
