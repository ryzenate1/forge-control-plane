import { NextResponse } from "next/server";
import de from "../../../../../../lang/de.json";
import en from "../../../../../../lang/en.json";
import es from "../../../../../../lang/es.json";
import fr from "../../../../../../lang/fr.json";
import ja from "../../../../../../lang/ja.json";
import pt from "../../../../../../lang/pt.json";
import ru from "../../../../../../lang/ru.json";
import zh from "../../../../../../lang/zh.json";

const MESSAGES = { de, en, es, fr, ja, pt, ru, zh } as const;

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ locale: string }> },
) {
  const { locale } = await params;

  if (!(locale in MESSAGES)) {
    return NextResponse.json({ error: `Unsupported locale: ${locale}` }, { status: 400 });
  }

  const messages = MESSAGES[locale as keyof typeof MESSAGES];

  return NextResponse.json(messages, {
    // Translation edits must be visible immediately, including through shared
    // proxies. The client keeps a per-locale in-memory cache for navigation.
    headers: { "Cache-Control": "no-store" },
  });
}
