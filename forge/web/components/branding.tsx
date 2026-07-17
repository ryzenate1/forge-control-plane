"use client";

import { createContext, useContext, useEffect, useMemo, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchPublicPanelSettings } from "@/lib/api";
import { Button } from "@/components/ui/primitives";

type BrandingContextValue = { companyName: string; browserTitle?: string; footerText?: string; logoUrl?: string; faviconUrl?: string; loginBackgroundUrl?: string; themePreset?: string; defaultLocale: string };
const DEFAULT_BRANDING: BrandingContextValue = { companyName: "Forge Control Plane", defaultLocale: "en" };
const BrandingContext = createContext<BrandingContextValue>(DEFAULT_BRANDING);

function safePublicUrl(value?: string) {
  if (!value) return undefined;
  try {
    const url = new URL(value, typeof window === "undefined" ? "http://localhost" : window.location.origin);
    return url.protocol === "http:" || url.protocol === "https:" || url.protocol === "data:" ? url.href : undefined;
  } catch { return undefined; }
}

export function BrandingProvider({ children }: { children: ReactNode }) {
  const settingsQuery = useQuery({ queryKey: ["public-panel-settings"], queryFn: fetchPublicPanelSettings, staleTime: 5 * 60_000, retry: 1 });
  const data = settingsQuery.data;
  const branding: BrandingContextValue = useMemo(() => data ? { ...data, companyName: data.companyName?.trim() || DEFAULT_BRANDING.companyName, defaultLocale: data.defaultLocale || "en", logoUrl: safePublicUrl(data.logoUrl), faviconUrl: safePublicUrl(data.faviconUrl), loginBackgroundUrl: safePublicUrl(data.loginBackgroundUrl) } : DEFAULT_BRANDING, [data]);

  useEffect(() => {
    document.title = branding.browserTitle?.trim() || branding.companyName;
    document.documentElement.lang = branding.defaultLocale;
    let link = document.querySelector<HTMLLinkElement>("link[rel='icon']");
    if (branding.faviconUrl) {
      if (!link) { link = document.createElement("link"); link.rel = "icon"; link.dataset.dynamic = "true"; document.head.appendChild(link); }
      link.href = branding.faviconUrl;
    } else if (link?.dataset.dynamic === "true") link.remove();
  }, [branding.browserTitle, branding.companyName, branding.defaultLocale, branding.faviconUrl]);

  return <BrandingContext.Provider value={branding}>{settingsQuery.isError ? <div className="flex flex-wrap items-center justify-center gap-3 border-b border-amber-500/25 bg-amber-500/10 px-4 py-2 text-sm text-amber-100" role="alert"><span>Branding settings could not be loaded. Default branding is shown; other API requests are unaffected.</span><Button className="min-h-8 px-3 py-1" disabled={settingsQuery.isFetching} onClick={() => void settingsQuery.refetch()} variant="secondary">{settingsQuery.isFetching ? "Retrying…" : "Retry"}</Button></div> : null}{children}</BrandingContext.Provider>;
}

export function useBranding() { return useContext(BrandingContext); }
