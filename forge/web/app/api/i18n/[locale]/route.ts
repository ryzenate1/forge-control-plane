import { NextResponse } from "next/server";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { basename, join, resolve } from "node:path";

type Messages = Record<string, unknown>;

// The `lang/` catalog lives at the repository root (sibling of `forge/`).
// Resolve it robustly across dev (`cwd = forge/web`), repo-root tooling
// (`cwd = repo root`) and the Docker standalone runner (`cwd = /app` with
// `lang/` copied next to the server — see forge/web/Dockerfile).
function findLangDir(): string | null {
  const candidates = [
    resolve(process.cwd(), "lang"),
    resolve(process.cwd(), "..", "lang"),
    resolve(process.cwd(), "..", "..", "lang"),
    resolve(process.cwd(), "..", "..", "..", "lang"),
    "/app/lang",
    resolve(process.cwd(), "forge", "web", "..", "..", "lang"),
  ];
  for (const dir of candidates) {
    try {
      if (existsSync(dir) && existsSync(join(dir, "en.json"))) return dir;
    } catch {
      // Try the next candidate.
    }
  }
  return null;
}

function listSupportedLocales(langDir: string): string[] {
  try {
    return readdirSync(langDir)
      .filter((f) => f.endsWith(".json"))
      .map((f) => basename(f, ".json"))
      .filter((l) => /^[a-z]{2}(-[A-Z]{2})?$/.test(l))
      .sort();
  } catch {
    return ["en"];
  }
}

const messageCache = new Map<string, Messages>();

function loadMessages(locale: string): Messages | null {
  if (messageCache.has(locale)) return messageCache.get(locale)!;
  const langDir = findLangDir();
  if (!langDir) {
    console.error(`[i18n] lang directory not found (cwd=${process.cwd()}); cannot serve locale "${locale}"`);
    return null;
  }
  const filePath = join(langDir, `${locale}.json`);
  try {
    if (!existsSync(filePath)) return null;
    const messages = JSON.parse(readFileSync(filePath, "utf-8")) as Messages;
    messageCache.set(locale, messages);
    return messages;
  } catch (err) {
    console.error(`[i18n] failed to load locale "${locale}" from ${filePath}:`, err);
    return null;
  }
}

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ locale: string }> },
) {
  const { locale } = await params;

  // Reject path traversal before touching the filesystem.
  if (!/^[a-z]{2}(-[A-Z]{2})?$/.test(locale)) {
    return NextResponse.json({ error: `Unsupported locale: ${locale}` }, { status: 400 });
  }

  const langDir = findLangDir();
  const supported = langDir ? listSupportedLocales(langDir) : ["en"];
  if (!supported.includes(locale)) {
    return NextResponse.json({ error: `Unsupported locale: ${locale}` }, { status: 400 });
  }

  const messages = loadMessages(locale);
  if (!messages) {
    console.error(`[i18n] locale "${locale}" not available (supported: ${supported.join(",")})`);
    return NextResponse.json({ error: `Locale unavailable: ${locale}` }, { status: 404 });
  }

  return NextResponse.json(messages, {
    // Translation edits must be visible immediately, including through shared
    // proxies. The client keeps a per-locale in-memory cache for navigation.
    headers: { "Cache-Control": "no-store" },
  });
}
