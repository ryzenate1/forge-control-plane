import { NextResponse } from "next/server";
import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";

const SUPPORTED_LOCALES = ["en", "de", "es", "fr", "ja", "pt", "ru", "zh"];

function loadMessages(locale: string): unknown | null {
  const filePath = resolve(process.cwd(), "../../lang", `${locale}.json`);
  if (!existsSync(filePath)) return null;
  const content = readFileSync(filePath, "utf-8");
  const messages = JSON.parse(content);
  return messages;
}

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ locale: string }> },
) {
  const { locale } = await params;

  if (!SUPPORTED_LOCALES.includes(locale)) {
    return NextResponse.json({ error: `Unsupported locale: ${locale}` }, { status: 400 });
  }

  const messages = loadMessages(locale);
  if (!messages) {
    return NextResponse.json({ error: `Locale file not found: ${locale}` }, { status: 404 });
  }

  return NextResponse.json(messages, {
    // Translation edits must be visible immediately, including through shared
    // proxies. The client keeps a per-locale in-memory cache for navigation.
    headers: { "Cache-Control": "no-store" },
  });
}
