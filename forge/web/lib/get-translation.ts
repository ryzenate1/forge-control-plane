import { readFileSync, existsSync, promises as fsPromises } from "node:fs";
import { resolve } from "node:path";

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

export async function loadLocaleAsync(locale: string): Promise<Messages | null> {
  if (serverCache.has(locale)) return serverCache.get(locale)!;
  const filePath = resolve(process.cwd(), "../../lang", `${locale}.json`);
  if (!existsSync(filePath)) return null;
  try {
    const content = await fsPromises.readFile(filePath, "utf-8");
    const messages = JSON.parse(content) as Messages;
    serverCache.set(locale, messages);
    return messages;
  } catch {
    return null;
  }
}

export function loadLocale(locale: string): Messages | null {
  if (serverCache.has(locale)) return serverCache.get(locale)!;
  const filePath = resolve(process.cwd(), "../../lang", `${locale}.json`);
  if (!existsSync(filePath)) return null;
  const content = readFileSync(filePath, "utf-8");
  const messages = JSON.parse(content) as Messages;
  serverCache.set(locale, messages);
  return messages;
}

export function t(key: string, locale: string, args?: Record<string, string | number> | (string | number)[]): string {
  const messages = loadLocale(locale);
  if (!messages) return key;
  const value = resolveNested(messages, key);
  if (typeof value !== "string") return key;
  return interpolate(value, args);
}
