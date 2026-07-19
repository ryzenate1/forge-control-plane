import { NextResponse } from "next/server";
import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";

const SUPPORTED_LOCALES = ["en", "de", "es", "fr", "ja", "pt", "ru", "zh"];

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ locale: string }> },
) {
  const { locale } = await params;

  if (!SUPPORTED_LOCALES.includes(locale)) {
    return NextResponse.json({ error: `Unsupported locale: ${locale}` }, { status: 400 });
  }

  // TODO: Bridge solution - replace with proper next-intl integration
  const filePath = resolve(process.cwd(), "../../lang", `${locale}.json`);

  if (!existsSync(filePath)) {
    return NextResponse.json({ error: `Locale file not found: ${locale}` }, { status: 404 });
  }

  const content = readFileSync(filePath, "utf-8");
  const messages = JSON.parse(content);

  return NextResponse.json(messages);
}
