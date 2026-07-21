"use client";

import { useCallback, useEffect, useState } from "react";
import { DEFAULT_I18N_CONFIG, type Locale } from "@/lib/api/shared-types";

// TODO: Bridge solution - replace with proper next-intl integration
// See: https://next-intl.dev/docs/getting-started/app-router

type Messages = Record<string, unknown>;

function resolveNested(obj: unknown, path: string): unknown {
  return path.split(".").reduce<unknown>((acc, key) => {
    if (acc && typeof acc === "object" && key in acc) {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, obj);
}

function interpolate(str: string, args?: Record<string, string | number>): string {
  if (!args) return str;
  return str.replace(/\{(\w+)\}/g, (_, key) => {
    const val = args[key];
    return val != null ? String(val) : `{${key}}`;
  });
}

function getInitialLocale(): Locale {
  if (typeof window === "undefined") return DEFAULT_I18N_CONFIG.defaultLocale;
  const cookie = document.cookie.split("; ").find((c) => c.startsWith("NEXT_LOCALE="));
  if (cookie) {
    const val = cookie.split("=")[1] as Locale;
    if (DEFAULT_I18N_CONFIG.supportedLocales.includes(val)) return val;
  }
  return DEFAULT_I18N_CONFIG.defaultLocale;
}

export function useTranslation() {
  const [locale, setLocale] = useState<Locale>(getInitialLocale);
  const [messages, setMessages] = useState<Messages | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    fetch(`/api/i18n/${locale}`)
      .then((res) => {
        if (!res.ok) throw new Error(`Failed to load locale: ${locale}`);
        return res.json();
      })
      .then((data) => {
        setMessages(data as Messages);
      })
      .catch(() => {
        // Fallback: try English
        fetch("/api/i18n/en")
          .then((res) => res.json())
          .then((data) => setMessages(data as Messages))
          .catch(() => setMessages({}));
      })
      .finally(() => setLoading(false));
  }, [locale]);

  const t = useCallback(
    (key: string, args?: Record<string, string | number>): string => {
      if (!messages) return key;
      const value = resolveNested(messages, key);
      if (typeof value !== "string") return key;
      return interpolate(value, args);
    },
    [messages],
  );

  const changeLocale = useCallback((newLocale: Locale) => {
    document.cookie = `NEXT_LOCALE=${newLocale};path=/;max-age=31536000;SameSite=Lax`;
    setLocale(newLocale);
  }, []);

  return { t, locale, changeLocale, loading, supportedLocales: DEFAULT_I18N_CONFIG.supportedLocales };
}
