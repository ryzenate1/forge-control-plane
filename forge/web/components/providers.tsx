"use client";

import { QueryClient, QueryClientProvider, useQuery, useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { type ReactNode, useEffect, useState } from "react";
import { fetchCurrentUser, migrateToCookieSession, refreshSession } from "@/lib/api";
import { useServerStore } from "@/stores/use-server-store";
import { BrandingProvider } from "@/components/branding";
import { Button } from "@/components/ui/primitives";
import { ToastProvider, useToast } from "@/components/ui/toast";
import { ThemeProvider } from "@/components/theme-provider";
import { ErrorBoundary } from "@/components/ui/error-boundary";

const SESSION_KEEPALIVE_MS = 10 * 60 * 1000;
const PROTECTED_PATH_PREFIXES = ["/servers", "/server", "/account", "/admin"];

function requiresSession(pathname: string) {
  return PROTECTED_PATH_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

function SessionLoader({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const { currentUser, setCurrentUser } = useServerStore();
  const sessionQuery = useQuery({
    queryKey: ["current-user"],
    queryFn: fetchCurrentUser,
    retry: 1,
    staleTime: SESSION_KEEPALIVE_MS,
    refetchInterval: SESSION_KEEPALIVE_MS,
    refetchOnWindowFocus: true,
  });

  useEffect(() => {
    const onVisibility = () => {
      if (document.visibilityState === "visible" && currentUser) {
        void migrateToCookieSession().finally(() => queryClient.invalidateQueries({ queryKey: ["current-user"] }));
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, [currentUser, queryClient]);

  useEffect(() => {
    void migrateToCookieSession();
  }, []);

  useEffect(() => {
    if (!currentUser) return;
    const interval = setInterval(() => {
      void refreshSession().catch(() => undefined);
    }, SESSION_KEEPALIVE_MS);
    return () => clearInterval(interval);
  }, [currentUser]);

  useEffect(() => {
    if (sessionQuery.data === null) {
      setCurrentUser(null);
      queryClient.removeQueries({ queryKey: ["current-user"] });
      if (requiresSession(pathname)) {
        toast({ tone: "error", title: "Session expired", message: "Sign in again to continue." });
        router.replace(`/?reason=session-expired&next=${encodeURIComponent(pathname)}`);
      }
      return;
    }
    if (sessionQuery.data && !currentUser) setCurrentUser(sessionQuery.data);
  }, [currentUser, pathname, queryClient, router, sessionQuery.data, setCurrentUser, toast]);

  return <>
    {sessionQuery.isFetching && !sessionQuery.data ? <div aria-label="Verifying session" className="fixed inset-x-0 top-0 z-[55] h-0.5 overflow-hidden bg-red-950"><div className="h-full w-1/2 animate-pulse bg-red-500" /></div> : null}
    {sessionQuery.isError ? <div className="flex flex-wrap items-center justify-center gap-3 border-b border-amber-500/25 bg-amber-500/10 px-4 py-2 text-sm text-amber-100" role="alert"><span>Session verification is temporarily unavailable. Your local session has not been removed.</span><Button className="min-h-8 px-3 py-1" disabled={sessionQuery.isFetching} onClick={() => void sessionQuery.refetch()} variant="secondary">{sessionQuery.isFetching ? "Retrying…" : "Retry"}</Button></div> : null}
    {children}
  </>;
}

export function Providers({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { staleTime: 30_000, gcTime: 5 * 60_000, refetchOnWindowFocus: false, retry: 1 }, mutations: { retry: false } } }));
  return <ThemeProvider><QueryClientProvider client={queryClient}><ToastProvider><BrandingProvider><ErrorBoundary><SessionLoader>{children}</SessionLoader></ErrorBoundary></BrandingProvider></ToastProvider></QueryClientProvider></ThemeProvider>;
}
