"use client";

import { createContext, useContext } from "react";
import { useTranslation } from "@/lib/use-translation";
import type { Locale } from "@forge/shared-types";
import defaultMessages from "../../../lang/en.json";

type TranslationContextType = {
  t: (key: string, args?: Record<string, string | number> | (string | number)[]) => string;
  locale: Locale;
  changeLocale: (locale: Locale) => void;
  loading: boolean;
  supportedLocales: Locale[];
  preloadLocale: (locale: Locale) => void;
};

const TranslationContext = createContext<TranslationContextType | null>(null);

export function TranslationProvider({ children }: { children: React.ReactNode }) {
  const translation = useTranslation();
  return (
    <TranslationContext.Provider value={translation}>
      {children}
    </TranslationContext.Provider>
  );
}

export function useT(): TranslationContextType["t"] {
  const ctx = useContext(TranslationContext);
  if (!ctx) {
    // Components are also rendered in isolated tests and error fallbacks.
    // Keep those paths readable instead of exposing implementation keys.
    return (key: string, args?: Record<string, string | number> | (string | number)[]) => {
      const value = key.split(".").reduce<unknown>((current, segment) => (
        current && typeof current === "object" ? (current as Record<string, unknown>)[segment] : undefined
      ), defaultMessages);
      if (typeof value !== "string") return key;
      if (Array.isArray(args)) return value.replace(/\{(\d+)\}/g, (_, index: string) => String(args[Number(index)] ?? `{${index}}`));
      return value.replace(/\{(\w+)\}/g, (_, name: string) => String(args?.[name] ?? `{${name}}`));
    };
  }
  return ctx.t;
}

export function useTranslationContext(): TranslationContextType {
  const ctx = useContext(TranslationContext);
  if (!ctx) {
    throw new Error("useTranslationContext must be used within a TranslationProvider");
  }
  return ctx;
}

export { TranslationContext };
