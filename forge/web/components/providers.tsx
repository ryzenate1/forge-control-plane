"use client";

import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { refreshSession } from "@/lib/api";
import { useCurrentUser } from "@/lib/api/use-current-user";
import { queryKeys } from "@/lib/api/query-keys";
import { useServerStore } from "@/stores/use-server-store";
import { useTenancyStore } from "@/stores/use-tenancy-store";
import { TenancyHydrator } from "@/lib/api/tenancy-hydrate";
import { BrandingProvider } from "@/components/branding";
import { Button } from "@/components/ui/primitives";
import { ToastProvider, useToast } from "@/components/ui/toast";
import { ThemeProvider } from "@/components/theme-provider";
import { ErrorBoundary } from "@/components/ui/error-boundary";
import { TranslationProvider } from "@/components/TranslationProvider";
import { isProtectedPath } from "@/lib/auth/protected-paths";
import { ApiError } from "@/lib/api";

const SESSION_KEEPALIVE_MS = 10 * 60 * 1000;

function requiresSession(pathname: string) {
  return isProtectedPath(pathname);
}

function SessionLoader({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const currentUser = useServerStore((s) => s.currentUser);
  const setCurrentUser = useServerStore((s) => s.setCurrentUser);
  const resetTenancy = useTenancyStore((s) => s.reset);
  const resetServer = useServerStore((s) => s.reset);
  const sessionQuery = useCurrentUser();

  // A single expiry surfaces through two independent signals — the global
  // `forge:session-expired` event (401 on any non-suppressed request) and the
  // current-user query resolving to `null` (401 on /auth/me, which suppresses
  // the event and reports via `null`). Both fire for the same expiry, so the
  // reset + toast + redirect must run exactly once per expiry.
  const logoutDoneRef = useRef(false);

  // Single session keepalive: one interval owns both the sliding refresh and
  // the revalidation of the current-user query. The query itself does not set
  // refetchInterval, so there is exactly one timer per mounted tree.
  useEffect(() => {
    if (!currentUser) return;
    let interval: ReturnType<typeof setInterval> | null = null;
    const tick = () => {
      void refreshSession()
        .then(() => queryClient.invalidateQueries({ queryKey: queryKeys.session.currentUser() }))
        .catch(() => undefined);
    };
    const start = () => {
      if (interval !== null) return;
      interval = setInterval(tick, SESSION_KEEPALIVE_MS);
    };
    const stop = () => {
      if (interval !== null) {
        clearInterval(interval);
        interval = null;
      }
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") start();
      else stop();
    };
    start();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      stop();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [currentUser, queryClient]);

  useEffect(() => {
    const onSessionExpired = () => {
      if (logoutDoneRef.current) return;
      logoutDoneRef.current = true;
      resetServer();
      resetTenancy();
      // Selective teardown: drop only user-scoped caches. A bare
      // `removeQueries()` wipes every cache including app-static data and
      // forces a refetch storm on next navigation; scoping keeps the
      // teardown to the session identity, tenancy subtrees and per-user
      // server/node views (zustand stores are reset above).
      queryClient.removeQueries({ queryKey: queryKeys.session.currentUser() });
      queryClient.removeQueries({ queryKey: queryKeys.tenancy.all });
      queryClient.removeQueries({ queryKey: queryKeys.servers.all });
      queryClient.removeQueries({ queryKey: queryKeys.nodes.all });
      if (requiresSession(pathname)) {
        toast({ tone: "error", title: "Session expired", message: "Sign in again to continue." });
        router.replace(`/?reason=session-expired&next=${encodeURIComponent(pathname)}`);
      }
    };
    window.addEventListener("forge:session-expired", onSessionExpired);
    return () => window.removeEventListener("forge:session-expired", onSessionExpired);
  }, [pathname, queryClient, router, resetServer, resetTenancy, toast]);

  useEffect(() => {
    if (sessionQuery.data) {
      // Session is valid again (re-login without a full reload re-uses this
      // tree) — re-arm the once-guard so a future expiry is handled.
      logoutDoneRef.current = false;
      // Sync on identity/role change, not only on first load: `!currentUser`
      // alone keeps a stale snapshot (e.g. a promoted role) for the whole
      // page lifetime.
      const fresh = sessionQuery.data;
      if (
        !currentUser ||
        currentUser.id !== fresh.id ||
        currentUser.email !== fresh.email ||
        currentUser.role !== fresh.role
      ) {
        setCurrentUser(fresh);
      }
      return;
    }
    if (sessionQuery.data === null) {
      if (logoutDoneRef.current) return;
      logoutDoneRef.current = true;
      resetServer();
      resetTenancy();
      queryClient.removeQueries({ queryKey: queryKeys.session.currentUser() });
      if (requiresSession(pathname)) {
        toast({ tone: "error", title: "Session expired", message: "Sign in again to continue." });
        router.replace(`/?reason=session-expired&next=${encodeURIComponent(pathname)}`);
      }
    }
  }, [currentUser, pathname, queryClient, router, sessionQuery.data, setCurrentUser, resetServer, resetTenancy, toast]);

  return <>
    {sessionQuery.isFetching && !sessionQuery.data ? <div aria-label="Verifying session" className="fixed inset-x-0 top-0 z-[55] h-0.5 overflow-hidden bg-[var(--danger-subtle)]"><div className="h-full w-1/2 animate-pulse bg-[var(--danger)]" /></div> : null}
    {sessionQuery.isError ? <div className="flex flex-wrap items-center justify-center gap-3 border-b border-[var(--warn-line)] bg-[var(--warn-subtle)] px-4 py-2 text-sm text-[var(--text)]" role="alert"><span>Session verification is temporarily unavailable. Your local session has not been removed.</span><Button className="min-h-8 px-3 py-1" disabled={sessionQuery.isFetching} onClick={() => void sessionQuery.refetch()} variant="secondary">{sessionQuery.isFetching ? "Retrying…" : "Retry"}</Button></div> : null}
    <TenancyHydrator />
    {children}
  </>;
}

export function Providers({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { staleTime: 30_000, gcTime: 5 * 60_000, refetchOnWindowFocus: false, retry: (failureCount, error) => { if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false; return failureCount < 1; } }, mutations: { retry: false } } }));
  return <ThemeProvider><QueryClientProvider client={queryClient}><ToastProvider><BrandingProvider><ErrorBoundary><TranslationProvider><SessionLoader>{children}</SessionLoader></TranslationProvider></ErrorBoundary></BrandingProvider></ToastProvider></QueryClientProvider></ThemeProvider>;
}
