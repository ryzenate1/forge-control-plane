"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { DEFAULT_I18N_CONFIG, type Locale } from "@forge/shared-types";
import defaultMessages from "../../../lang/en.json";
import { requestJSON } from "./api/http";

type Messages = Record<string, unknown>;

/**
 * Locale bundles are served by the panel's own Next.js route handler (not the Go
 * API), so `sameOrigin` keeps the canonical client from prefixing the path with
 * the API base URL. The 401 session-expiry signal is suppressed: a failed
 * translation load is a cosmetic fallback (English), never a reason to log the
 * user out.
 */
function loadLocaleMessages(target: Locale, signal: AbortSignal): Promise<Messages> {
  return requestJSON<Messages>(`/api/i18n/${target}`, { signal }, { sameOrigin: true, suppressSessionExpired: true });
}

function resolveNested(obj: unknown, path: string): unknown {
  return path.split(".").reduce<unknown>((acc, key) => {
    if (acc && typeof acc === "object" && key in acc) {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, obj);
}

export function interpolate(str: string, args?: Record<string, string | number> | (string | number)[]): string {
  if (!args) return str;
  if (Array.isArray(args)) {
    return str.replace(/\{(\d+)\}/g, (_, index: string) => {
      const value = args[Number(index)];
      return value == null ? `{${index}}` : String(value);
    });
  }
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

const warnedMissingKeys = new Set<string>();

export function useTranslation() {
  const [locale, setLocale] = useState<Locale>(getInitialLocale);
  const [messages, setMessages] = useState<Messages | null>(null);
  const [loading, setLoading] = useState(true);
  const translationCache = useRef(new Map<Locale, Messages>()).current;

  useEffect(() => {
    const cached = translationCache.get(locale);
    if (cached) {
      setMessages(cached);
      setLoading(false);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    loadLocaleMessages(locale, controller.signal)
      .then((data) => {
        const nextMessages = data as Messages;
        translationCache.set(locale, nextMessages);
        setMessages(nextMessages);
      })
      .catch(async () => {
        if (controller.signal.aborted) return;
        try {
          const data = await loadLocaleMessages("en", controller.signal);
          if (!controller.signal.aborted) setMessages(data as Messages);
        } catch {
          if (!controller.signal.aborted) setMessages((prev) => prev ?? {});
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => { controller.abort(); };
  }, [locale, translationCache]);

  const t = useCallback(
    (key: string, args?: Record<string, string | number> | (string | number)[]): string => {
      const localized = resolveNested(messages, key);
      // Missing keys fall back to English, then to the raw key string so the
      // UI never renders "undefined".
      const value = typeof localized === "string" ? localized : resolveNested(defaultMessages, key);
      if (typeof value !== "string") {
        if (Array.isArray(args) && typeof args[0] === "string") {
          return args[0];
        }
        if (process.env.NODE_ENV === "development" && !warnedMissingKeys.has(key)) {
          warnedMissingKeys.add(key);
          console.warn(`[i18n] Missing translation key in lang/en.json: "${key}"`);
        }
        return key;
      }
      return interpolate(value, args);
    },
    [messages],
  );

  const changeLocale = useCallback((newLocale: Locale) => {
    document.cookie = `NEXT_LOCALE=${newLocale};path=/;max-age=31536000;SameSite=Lax`;
    document.documentElement.lang = newLocale;
    setLocale(newLocale);
  }, []);

  const preloadAbortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    return () => { preloadAbortRef.current?.abort(); };
  }, []);

  const preloadLocale = useCallback((localeToPreload: Locale) => {
    if (translationCache.has(localeToPreload) || localeToPreload === locale) return;
    preloadAbortRef.current?.abort();
    const controller = new AbortController();
    preloadAbortRef.current = controller;
    loadLocaleMessages(localeToPreload, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted) {
          translationCache.set(localeToPreload, data as Messages);
        }
      })
      .catch((err: unknown) => {
        if (err instanceof Error && err.name === "AbortError") return;
        console.warn("[i18n] preloadLocale failed:", err);
      });
  }, [locale, translationCache]);

  return { t, locale, changeLocale, loading, supportedLocales: DEFAULT_I18N_CONFIG.supportedLocales, preloadLocale };
}
