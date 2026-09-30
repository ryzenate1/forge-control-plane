import { readFileSync, existsSync, promises as fsPromises } from "node:fs";
import { resolve, join } from "node:path";

type Messages = Record<string, unknown>;

function resolveNested(obj: unknown, path: string): unknown {
  return path.split(".").reduce<unknown>((acc, key) => {
    if (acc && typeof acc === "object" && key in acc) {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, obj);
}

function interpolate(str: string, args?: Record<string, string | number> | (string | number)[]): string {
  if (!args) return str;
  if (Array.isArray(args)) {
    return str.replace(/\{(\d+)\}/g, (_, idx) => {
      const val = args[parseInt(idx)];
      return val != null ? String(val) : `{${idx}}`;
    });
  }
  return str.replace(/\{(\w+)\}/g, (_, key) => {
    const val = (args as Record<string, string | number>)[key];
    return val != null ? String(val) : `{${key}}`;
  });
}

const serverCache = new Map<string, Messages>();

// `lang/` lives at the repo root. The web server's cwd varies (forge/web in
// dev, /app in the Docker standalone runner which copies `lang/` next to the
// server), so probe candidates instead of assuming `../../lang`.
function findLocaleFile(locale: string): string | null {
  if (!/^[a-z]{2}(-[A-Z]{2})?$/.test(locale)) return null;
  const candidates = [
    resolve(process.cwd(), "lang", `${locale}.json`),
    resolve(process.cwd(), "..", "lang", `${locale}.json`),
    resolve(process.cwd(), "..", "..", "lang", `${locale}.json`),
    resolve(process.cwd(), "..", "..", "..", "lang", `${locale}.json`),
    join("/app", "lang", `${locale}.json`),
  ];
  for (const filePath of candidates) {
    try {
      if (existsSync(filePath)) return filePath;
    } catch {
      // Try the next candidate.
    }
  }
  return null;
}

function logMissingLocale(locale: string): void {
  if (typeof console !== "undefined") {
    console.error(`[i18n] locale "${locale}" not found (cwd=${process.cwd()}); returning null so callers fall back to English`);
  }
}

export async function loadLocaleAsync(locale: string): Promise<Messages | null> {
  if (serverCache.has(locale)) return serverCache.get(locale)!;
  const filePath = findLocaleFile(locale);
  if (!filePath) {
    logMissingLocale(locale);
    return null;
  }
  try {
    const content = await fsPromises.readFile(filePath, "utf-8");
    const messages = JSON.parse(content) as Messages;
    serverCache.set(locale, messages);
    return messages;
  } catch (err) {
    if (typeof console !== "undefined") {
      console.error(`[i18n] failed to load locale "${locale}" from ${filePath}:`, err);
    }
    return null;
  }
}

export function loadLocale(locale: string): Messages | null {
  if (serverCache.has(locale)) return serverCache.get(locale)!;
  const filePath = findLocaleFile(locale);
  if (!filePath) {
    logMissingLocale(locale);
    return null;
  }
  try {
    const content = readFileSync(filePath, "utf-8");
    const messages = JSON.parse(content) as Messages;
    serverCache.set(locale, messages);
    return messages;
  } catch (err) {
    if (typeof console !== "undefined") {
      console.error(`[i18n] failed to load locale "${locale}" from ${filePath}:`, err);
    }
    return null;
  }
}

export function t(key: string, locale: string, args?: Record<string, string | number> | (string | number)[]): string {
  const messages = loadLocale(locale);
  if (!messages) return key;
  const value = resolveNested(messages, key);
  if (typeof value !== "string") return key;
  return interpolate(value, args);
}
