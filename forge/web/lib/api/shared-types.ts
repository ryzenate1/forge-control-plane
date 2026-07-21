export type Locale = 'en' | 'fr' | 'de' | 'es' | 'ja' | 'zh' | 'ru' | 'pt-BR';

export const DEFAULT_I18N_CONFIG = {
  defaultLocale: 'en' as Locale,
  supportedLocales: ['en', 'fr', 'de', 'es', 'ja', 'zh', 'ru', 'pt-BR'] as Locale[],
};
