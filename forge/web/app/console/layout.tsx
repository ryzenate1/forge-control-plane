"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LogOut, Menu, User, X } from "lucide-react";
import { logout } from "@/lib/api";
import { useCurrentUser } from "@/lib/api/use-current-user";
import { useBranding } from "@/components/branding";
import { useServerStore } from "@/stores/use-server-store";
import { useTenancyStore } from "@/stores/use-tenancy-store";
import { ConsoleNav } from "@/components/console/console-nav";
import { consoleNavGroups } from "@/components/console/console-registry";
import { humanizeSegment, resolveActive } from "@/lib/nav/active";
import { useNavDrawer } from "@/lib/hooks/use-nav-drawer";
import {
  applyBreadcrumbOverrides,
  BreadcrumbProvider,
  useBreadcrumbOverrides,
} from "@/lib/nav/breadcrumb-context";
import { OfflineBanner } from "@/components/shared/states-offline";

/**
 * The customer console frame.
 *
 * The breadcrumb provider wraps the frame so a detail page rendered as
 * `children` can name its own resource in the trail this bar renders. Before
 * that, detail pages drew a second trail inside their own page header.
 */
export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  return (
    <BreadcrumbProvider>
      <ConsoleFrame>{children}</ConsoleFrame>
    </BreadcrumbProvider>
  );
}

function ConsoleFrame({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { companyName } = useBranding();
  const { currentUser } = useServerStore();
  const [mobileOpen, setMobileOpen] = useState(false);
  const drawerRef = useRef<HTMLElement>(null);
  const drawerCloseRef = useRef<HTMLButtonElement>(null);
  const closeDrawer = useCallback(() => setMobileOpen(false), []);
  useNavDrawer({ open: mobileOpen, close: closeDrawer, containerRef: drawerRef, initialFocusRef: drawerCloseRef });

  /**
   * Page context for the console top bar. Derived from the same registry the
   * sidebar renders, so the label in the bar and the highlighted row can never
   * disagree; unregistered paths fall back to a humanized final segment rather
   * than showing nothing.
   */
  const overrides = useBreadcrumbOverrides();
  const crumbs = useMemo<Array<{ label: string; href?: string }>>(() => {
    const entry = resolveActive(pathname, consoleNavGroups.flatMap((group) => group.items));
    if (!entry) {
      const last = pathname.split("/").filter(Boolean).at(-1);
      return last && last !== "console" ? [{ label: humanizeSegment(last) }] : [];
    }

    const trail: Array<{ label: string; href?: string }> = [{ label: entry.label, href: entry.href }];
    // Segments below the matched nav entry become their own crumbs, so a
    // detail page reads as "Applications / My API" instead of collapsing onto
    // its list page. A page names its own segment with `useBreadcrumbLabel`.
    if (pathname.startsWith(entry.href) && pathname !== entry.href) {
      const rest = pathname.slice(entry.href.length).split("/").filter(Boolean);
      let href = entry.href;
      rest.forEach((segment, index) => {
        href = `${href}/${segment}`;
        trail.push({
          label: humanizeSegment(segment),
          href: index === rest.length - 1 ? undefined : href,
        });
      });
    }
    const last = trail.at(-1);
    if (last) delete last.href;
    return applyBreadcrumbOverrides(trail, overrides);
  }, [pathname, overrides]);

  const userQuery = useCurrentUser();

  useEffect(() => {
    if (userQuery.data === null) {
      router.replace("/?reason=session-expired&next=/console");
    }
  }, [router, userQuery.data]);

  useEffect(() => { setMobileOpen(false); }, [pathname]);

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      useServerStore.getState().reset();
      useTenancyStore.getState().reset();
      router.push("/");
    }
  };

  if (userQuery.isPending || userQuery.data === undefined) {
    return (
      <div className="grid min-h-screen place-items-center bg-[var(--canvas)] p-4 text-sm text-slate-300">
        Preparing Console…
      </div>
    );
  }

  if (!userQuery.data) {
    return (
      <div className="grid min-h-screen place-items-center bg-[var(--canvas)] p-4">
        <div className="w-full max-w-md space-y-4 rounded-xl border border-red-500/30 bg-[var(--surface)] p-6 text-center" role="alert">
          <h1 className="text-xl font-bold text-slate-100">Session unavailable</h1>
          <p className="text-sm text-slate-300">The current user could not be loaded. Sign in again to continue.</p>
          <button
            className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white transition-colors hover:bg-[var(--brand-dark)] disabled:opacity-60"
            disabled={userQuery.isFetching}
            onClick={() => void userQuery.refetch()}
            type="button"
          >
            {userQuery.isFetching ? "Retrying…" : "Retry"}
          </button>
        </div>
      </div>
    );
  }

  const email = userQuery.data.email ?? currentUser?.email;

  return (
    <div className="h-screen overflow-hidden bg-[var(--canvas)]">
      <a
        href="#forge-main"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[100] focus:rounded-lg focus:bg-[var(--brand)] focus:px-4 focus:py-2 focus:text-sm focus:font-semibold focus:text-white"
      >
        Skip to content
      </a>
      {/* Top bar — fixed */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/[0.06] bg-[var(--surface)] px-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-1">
          <button
            aria-label="Open console navigation"
            aria-expanded={mobileOpen}
            className="hidden rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] max-[899px]:inline-flex"
            onClick={() => setMobileOpen(true)}
            type="button"
          >
            <Menu size={19} />
          </button>
          <Link className="shrink-0 text-lg font-bold tracking-tight text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" href="/console">{companyName}</Link>
          {crumbs.length > 0 ? (
            <nav aria-label="Breadcrumb" className="ml-3 hidden min-w-0 items-center border-l border-[var(--line)] pl-3 font-mono text-xs sm:flex">
              <ol className="flex min-w-0 items-center gap-1.5">
                <li>
                  <Link className="text-[var(--text-subtle)] transition-colors hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" href="/console">Console</Link>
                </li>
                {crumbs.map((entry, index) => (
                  <li key={`${entry.label}-${index}`} className="flex min-w-0 items-center gap-1.5">
                    <span aria-hidden="true" className="text-[var(--text-muted)]">/</span>
                    {entry.href ? (
                      <Link
                        className="truncate text-[var(--text-subtle)] transition-colors hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                        href={entry.href}
                      >
                        {entry.label}
                      </Link>
                    ) : (
                      <span aria-current="page" className="truncate font-semibold text-[var(--text)]">{entry.label}</span>
                    )}
                  </li>
                ))}
              </ol>
            </nav>
          ) : null}
        </div>
        <div className="flex items-center gap-1 text-sm">
          <Link className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-white" href="/account">
            <User size={15} />
            <span className="hidden max-w-48 truncate sm:inline">{email}</span>
          </Link>
          <button className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-slate-400 transition-colors hover:bg-white/[0.06] hover:text-white" onClick={handleLogout} type="button">
            <LogOut size={15} />
            Sign out
          </button>
        </div>
      </header>

      <div className="flex h-[calc(100vh-56px)]">
        {/* Sidebar — independently scrollable */}
        <aside className="max-[899px]:hidden w-64 shrink-0 flex-col border-r border-white/[0.06] bg-[var(--surface)] flex">
          <div className="border-b border-white/[0.06] px-4 py-3">
            <p className="text-[10px] font-bold uppercase tracking-[0.12em] text-slate-400">Console</p>
          </div>
          <nav className="flex-1 overflow-y-auto px-3 py-4">
            <ConsoleNav />
          </nav>
          <div className="shrink-0 border-t border-white/[0.06] px-3 py-3">
            <Link className="flex items-center gap-2.5 w-full rounded-lg px-3 py-2 text-sm text-slate-300 transition-all hover:bg-white/[0.04] hover:text-slate-200" href="/account">
              <User size={15} />
              Account settings
            </Link>
          </div>
        </aside>

        {/* Content — independently scrollable */}
        <main id="forge-main" tabIndex={-1} className="min-w-0 flex-1 overflow-y-auto p-4 sm:p-6 lg:p-7">
          <OfflineBanner onRetry={() => window.location.reload()} />
          {children}
        </main>
      </div>

      {/* Mobile navigation drawer */}
      {mobileOpen ? (
        <div className="fixed inset-0 z-50 hidden max-[899px]:flex" role="dialog" aria-modal="true" aria-label="Console navigation">
          <button aria-label="Close navigation overlay" className="absolute inset-0 bg-black/70" onClick={closeDrawer} type="button" />
          <aside ref={drawerRef} className="relative flex h-full w-[min(88vw,340px)] flex-col border-r border-white/[0.06] bg-[var(--surface)] shadow-2xl">
            <div className="flex items-center justify-between border-b border-white/[0.06] p-4">
              <span className="text-sm font-semibold text-[var(--text)]">Console navigation</span>
              <button ref={drawerCloseRef} aria-label="Close console navigation" className="rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" onClick={closeDrawer} type="button"><X size={18} /></button>
            </div>
            <nav className="flex-1 overflow-y-auto px-3 py-4">
              <ConsoleNav onNavigate={closeDrawer} />
            </nav>
            <div className="shrink-0 border-t border-white/[0.06] px-3 py-3">
              <Link className="flex items-center gap-2.5 w-full rounded-lg px-3 py-2 text-sm text-slate-300 transition-all hover:bg-white/[0.04] hover:text-slate-200" href="/account">
                <User size={15} />
                Account settings
              </Link>
            </div>
          </aside>
        </div>
      ) : null}
    </div>
  );
}