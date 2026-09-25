"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LogOut, Menu, User, X } from "lucide-react";
import { fetchCurrentUser, logout } from "@/lib/api";
import { useBranding } from "@/components/branding";
import { useServerStore } from "@/stores/use-server-store";
import { ConsoleNav } from "@/components/console/console-nav";
import { OfflineBanner } from "@/components/shared/states-offline";

export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { companyName } = useBranding();
  const { currentUser } = useServerStore();
  const [mobileOpen, setMobileOpen] = useState(false);

  const userQuery = useQuery({
    queryKey: ["current-user"],
    queryFn: fetchCurrentUser,
    staleTime: 30_000,
    retry: 1,
  });

  useEffect(() => {
    if (userQuery.data === null) {
      router.replace("/?reason=session-expired&next=/console");
    }
  }, [router, userQuery.data]);

  useEffect(() => { setMobileOpen(false); }, [pathname]);

  const handleLogout = async () => {
    await logout();
    router.push("/");
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
        className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-[60] focus:rounded-lg focus:bg-red-600 focus:px-4 focus:py-2 focus:text-sm focus:font-bold focus:text-white"
      >
        Skip to content
      </a>
      {/* Top bar — fixed */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/[0.06] bg-[var(--surface)] px-4 sm:px-6">
        <div className="flex items-center gap-1">
          <button aria-label="Open console navigation" className="hidden rounded-lg p-2 text-slate-300 hover:bg-white/[0.06] max-[899px]:inline-flex" onClick={() => setMobileOpen(true)} type="button"><Menu size={19} /></button>
          <Link className="text-lg font-bold tracking-tight text-slate-100" href="/console">{companyName}</Link>
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
        <main id="forge-main" className="min-w-0 flex-1 overflow-y-auto p-4 sm:p-6 lg:p-7">
          <OfflineBanner onRetry={() => window.location.reload()} />
          {children}
        </main>
      </div>

      {/* Mobile navigation drawer */}
      {mobileOpen ? (
        <div className="fixed inset-0 z-50 hidden max-[899px]:flex">
          <button aria-label="Close navigation overlay" className="absolute inset-0 bg-black/70" onClick={() => setMobileOpen(false)} type="button" />
          <aside className="relative flex h-full w-[min(88vw,340px)] flex-col border-r border-white/[0.06] bg-[var(--surface)] shadow-2xl">
            <div className="flex items-center justify-between border-b border-white/[0.06] p-4">
              <span className="text-sm font-semibold text-slate-200">Console navigation</span>
              <button aria-label="Close console navigation" className="rounded-lg p-2 text-slate-400 hover:bg-white/[0.06]" onClick={() => setMobileOpen(false)} type="button"><X size={18} /></button>
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
        </div>
      ) : null}
    </div>
  );
}