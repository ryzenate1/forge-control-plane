const rtlLocales = new Set(["ar", "he", "fa", "ur", "yi"]);

export function isRtl(locale: string): boolean {
  return rtlLocales.has(locale.split("-")[0]);
}

export function getDir(locale: string): "ltr" | "rtl" {
  return isRtl(locale) ? "rtl" : "ltr";
}

export function formatNumber(value: number, locale: string, options?: Intl.NumberFormatOptions): string {
  try {
    return new Intl.NumberFormat(locale.replace("-", "_"), options).format(value);
  } catch {
    return String(value);
  }
}

export function formatDate(value: Date | number | string, locale: string, options?: Intl.DateTimeFormatOptions): string {
  try {
    return new Intl.DateTimeFormat(locale.replace("-", "_"), options).format(new Date(value));
  } catch {
    return String(value);
  }
}

export function formatRelativeTime(value: number, unit: Intl.RelativeTimeFormatUnit, locale: string, options?: Intl.RelativeTimeFormatOptions): string {
  try {
    return new Intl.RelativeTimeFormat(locale.replace("-", "_"), options).format(value, unit);
  } catch {
    return String(value);
  }
}

export function formatBytes(bytes: number, locale: string): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const value = bytes / Math.pow(1024, i);
  return `${formatNumber(value, locale, { maximumFractionDigits: 2 })} ${units[i]}`;
}

export function pluralize(count: number, locale: string, singular: string, plural: string, zero?: string): string {
  if (count === 0 && zero) return zero;
  try {
    const rules = new Intl.PluralRules(locale.replace("-", "_"));
    return rules.select(count) === "one" ? singular : plural;
  } catch {
    return count === 1 ? singular : plural;
  }
}
