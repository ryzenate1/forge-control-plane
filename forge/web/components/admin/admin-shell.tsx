"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { LogOut, AlertTriangle, Menu, X, Search, ChevronDown, CheckCircle2, Clock, User } from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchCurrentUser, logout, fetchHealthStatus, fetchNotificationLogs, EVENT_LABELS } from "@/lib/api";
import { API_BASE_URL } from "@/lib/api/http";
import { useBranding } from "@/components/branding";
import { useServerStore } from "@/stores/use-server-store";
import { useT } from "@/components/TranslationProvider";
import { adminPagesForRole, adminSidebarGroups, findAdminPage, ADMIN_ALIAS_ROUTES } from "./admin-registry";
import {
  ForgeLogoIcon,
  PlanetDefaultIcon,
  NotificationBellIcon,
  SettingsCogIcon,
} from "@/components/ui/forge-icons";

import { CommandPalette } from "./command-palette";

function resolveAlias(pathname: string): string {
  return ADMIN_ALIAS_ROUTES[pathname] ?? pathname;
}

function relativeTime(value?: string): string {
  if (!value) return "recently";
  const then = new Date(value).getTime();
  if (Number.isNaN(then)) return "recently";
  const diffMin = Math.round((then - Date.now()) / 60_000);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (Math.abs(diffMin) < 60) return rtf.format(diffMin, "minute");
  if (Math.abs(diffMin) < 60 * 24) return rtf.format(Math.round(diffMin / 60), "hour");
  return rtf.format(Math.round(diffMin / (60 * 24)), "day");
}

/** Closes a popover when the user clicks outside it or presses Escape. */
function useDismissOnOutside(ref: React.RefObject<HTMLElement | null>, isOpen: boolean, close: () => void) {
  useEffect(() => {
    if (!isOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) close();
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [ref, isOpen, close]);
}

function NavStateLaneBadge({ hasPending }: { hasPending?: boolean }) {
  if (!hasPending) return null;
  return (
    <span className="ml-auto flex items-center gap-1" aria-label="Pending generation">
      <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
      <span className="h-1.5 w-1.5 rounded-full bg-amber-400 motion-safe:animate-pulse" />
    </span>
  );
}

export function AdminShell({ children }: { children: React.ReactNode }) {
  const t = useT();
  const tOr = (key: string, fallback: string) => { const value = t(key); return value === key ? fallback : value; };
  const pathname = usePathname();
  const resolvedPath = resolveAlias(pathname);
  const router = useRouter();
  const { companyName } = useBranding();
  const { currentUser, setCurrentUser } = useServerStore();
  const userQuery = useQuery({
    queryKey: ["current-user"],
    queryFn: fetchCurrentUser,
    staleTime: 30_000,
    retry: 1,
  });
  const user = userQuery.data === null ? null : userQuery.data ?? currentUser;

  const healthQuery = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealthStatus,
    staleTime: 30_000,
    retry: 1,
  });
  const isHealthy = healthQuery.data?.status === "ok" && healthQuery.data?.ok !== false;

  const notificationsQuery = useQuery({
    queryKey: ["notification-logs", "recent"],
    queryFn: () => fetchNotificationLogs(undefined, 5, 0),
    staleTime: 30_000,
    retry: 1,
  });
  const recentNotifications = notificationsQuery.data ?? [];

  useEffect(() => {
    if (userQuery.data === null) {
      router.replace("/");
    } else if (user && user.role !== "admin") {
      router.replace("/servers");
    }
  }, [router, user, userQuery.data]);

  const [mobileOpen, setMobileOpen] = useState(false);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const [navSearch, setNavSearch] = useState("");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [expandedMore, setExpandedMore] = useState<Record<string, boolean>>({});
  const [notificationsOpen, setNotificationsOpen] = useState(false);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const [projectMenuOpen, setProjectMenuOpen] = useState(false);
  const drawerRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const notificationsRef = useRef<HTMLDivElement>(null);
  const userMenuRef = useRef<HTMLDivElement>(null);
  const projectMenuRef = useRef<HTMLDivElement>(null);

  useDismissOnOutside(notificationsRef, notificationsOpen, () => setNotificationsOpen(false));
  useDismissOnOutside(userMenuRef, userMenuOpen, () => setUserMenuOpen(false));
  useDismissOnOutside(projectMenuRef, projectMenuOpen, () => setProjectMenuOpen(false));

  const navGroups = useMemo(() => {
    const rawGroups = adminSidebarGroups(user?.role);
    if (!navSearch.trim()) return rawGroups;
    const query = navSearch.toLowerCase().trim();
    const matches = (label: string, description: string, href: string) =>
      `${label} ${description} ${href}`.toLowerCase().includes(query);
    return rawGroups
      .map((group) => ({
        ...group,
        items: group.items.filter((item) => matches(item.label, item.description, item.href)),
        secondaryItems: group.secondaryItems.filter((item) => matches(item.label, item.description, item.href)),
      }))
      .filter((group) => group.items.length > 0 || group.secondaryItems.length > 0);
  }, [navSearch, user?.role]);

  const currentPage = findAdminPage(resolvedPath);

  // Find active group for breadcrumbs
  const activeGroup = useMemo(() => {
    return adminPagesForRole(user?.role).find((g) =>
      g.items.some((item) => resolvedPath === item.href || resolvedPath.startsWith(`${item.href}/`))
    );
  }, [resolvedPath, user?.role]);

  useEffect(() => {
    setMobileOpen(false);
    setNotificationsOpen(false);
    setUserMenuOpen(false);
    setProjectMenuOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (mobileOpen) {
      document.body.style.overflow = "hidden";
      closeButtonRef.current?.focus();
      const handleKey = (e: KeyboardEvent) => {
        if (e.key === "Escape") setMobileOpen(false);
        if (e.key === "Tab" && drawerRef.current) {
          const focusable = drawerRef.current.querySelectorAll<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
          if (focusable.length === 0) return;
          const first = focusable[0];
          const last = focusable[focusable.length - 1];
          if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
          else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
        }
      };
      document.addEventListener("keydown", handleKey);
      return () => {
        document.body.style.overflow = "";
        document.removeEventListener("keydown", handleKey);
      };
    } else {
      document.body.style.overflow = "";
    }
  }, [mobileOpen]);

  const toggleGroup = (title: string) => setCollapsedGroups((current) => ({ ...current, [title]: !current[title] }));
  const isGroupCollapsed = (title: string) => Boolean(collapsedGroups[title] && !navSearch.trim());
  const toggleMore = (title: string) => setExpandedMore((current) => ({ ...current, [title]: !current[title] }));
  const isMoreExpanded = (title: string) => Boolean(expandedMore[title] || navSearch.trim());

  const navItemClass = (active: boolean) =>
    cn(
      "flex w-full items-center gap-2.5 rounded-md border-l-2 px-2.5 py-1.5 text-xs font-medium transition-colors text-left motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]",
      active
        ? "border-[var(--brand)] bg-[var(--brand)]/10 text-[var(--brand)] font-semibold"
        : "border-transparent text-[var(--text-subtle)] hover:bg-white/[0.04] hover:text-[var(--text)]"
    );

  const handleLogout = async () => {
    try {
      await logout();
    } catch {
      // Session may already be gone server-side; still clear local state and leave.
    } finally {
      setCurrentUser(null);
      router.push("/");
    }
  };

  if (userQuery.isPending) {
      return (
        <div className="grid min-h-screen place-items-center bg-[var(--canvas)] p-4 text-sm text-[var(--text-subtle)]">
          {tOr("admin.shell.redirecting", "Redirecting to sign in…")}
        </div>
      );
    }

    if (!userQuery.data) {
    return (
      <div className="grid min-h-screen place-items-center bg-[var(--canvas)] p-4">
        <div className="w-full max-w-md space-y-4 rounded-xl border border-[var(--danger)]/30 bg-[var(--surface-raised)] p-6 text-center" role="alert">
          <AlertTriangle size={28} className="mx-auto text-amber-400" strokeWidth={1.5} />
          <h1 className="text-xl font-bold text-[var(--text)]">{tOr("admin.shell.verifyFailedTitle", "Unable to verify admin access")}</h1>
          <p className="text-sm text-red-300">
            {userQuery.isError
              ? `${tOr("admin.shell.apiUnreachable", "API not reachable at")} ${API_BASE_URL}. ${tOr("admin.shell.apiUnreachableHint", "Make sure the Go backend is running.")}`
              : tOr("admin.shell.userLoadFailed", "The current user could not be loaded. Admin content remains hidden until the API responds.")
            }
          </p>
          <div className="flex justify-center gap-2">
            <button
              className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white hover:bg-[var(--brand-hover)] disabled:opacity-60 transition-colors motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)]"
              disabled={userQuery.isFetching}
              onClick={() => void userQuery.refetch()}
              type="button"
            >
              {userQuery.isFetching ? tOr("admin.shell.retrying", "Retrying…") : tOr("common.retry", "Retry")}
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="grid min-h-screen place-items-center bg-[var(--canvas)] p-4 text-sm text-[var(--text-subtle)]">
        {tOr("admin.shell.verifying", "Verifying admin access…")}
      </div>
    );
  }

  return (
    <div className="h-screen overflow-hidden bg-[var(--canvas)]">
      <CommandPalette open={commandPaletteOpen} onOpenChange={setCommandPaletteOpen} />

      {/* Top bar — fixed */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-[var(--line)] bg-[var(--surface)] px-4 sm:px-6">
        <div className="flex items-center gap-3">
          <button
            aria-label="Open admin navigation"
            className="rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] max-[899px]:inline-flex hidden focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)]"
            onClick={() => setMobileOpen(true)}
            type="button"
          >
            <Menu size={19} />
          </button>
          <Link
            className="text-base font-bold text-[var(--text)] tracking-tight hover:text-[var(--brand)] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] flex items-center gap-2.5"
            href="/admin/overview"
          >
            <ForgeLogoIcon size={24} className="shrink-0" />
            <div className="flex flex-col text-left">
              <span className="leading-tight font-extrabold text-xs tracking-wider uppercase text-slate-100">{companyName || "Forge"}</span>
              <span className="text-[8px] font-mono tracking-widest text-slate-500 uppercase">Infrastructure Control Plane</span>
            </div>
          </Link>

          {/* Breadcrumbs — group is plain text (groups have no landing page) */}
          {currentPage && (
            <div className="hidden sm:flex items-center gap-2 pl-3 border-l border-[var(--line)] text-xs font-mono">
              {activeGroup && (
                <>
                  <span className="text-[var(--text-muted)]">
                    {activeGroup.title}
                  </span>
                  <span className="text-[var(--text-muted)]">/</span>
                </>
              )}
              <span className="font-semibold text-slate-200">{tOr(currentPage.labelKey, currentPage.label)}</span>
            </div>
          )}
        </div>

        {/* Center: Command Palette Trigger */}
        <div className="flex-1 max-w-md mx-4 hidden md:block">
          <button
            type="button"
            onClick={() => setCommandPaletteOpen(true)}
            className="flex w-full items-center justify-between rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-1.5 text-xs text-[var(--text-subtle)] hover:border-[var(--line-strong)] hover:text-[var(--text)] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] cursor-pointer"
          >
            <span className="flex items-center gap-2">
              <Search size={13} className="text-[var(--text-muted)]" />
              <span className="truncate">Search servers, nodes, deployments, domains…</span>
            </span>
            <kbd className="rounded border border-[var(--line)] bg-[var(--surface-raised)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--text-muted)]">
              ⌘K
            </kbd>
          </button>
        </div>

        {/* Right: Actions */}
        <div className="flex items-center gap-2.5 text-xs text-[var(--text-subtle)]">
          {/* Global System Status Pill — Clickable to Health */}
          <button
            type="button"
            onClick={() => router.push("/admin/health")}
            title="Inspect platform diagnostics & health checks"
            className={cn(
              "hidden xl:flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium transition cursor-pointer hover:opacity-90",
              isHealthy
                ? "border-emerald-500/25 bg-emerald-500/10 text-emerald-300"
                : "border-amber-500/30 bg-amber-500/15 text-amber-300"
            )}
          >
            <span className="relative flex h-2 w-2">
              <span className={cn("animate-ping absolute inline-flex h-full w-full rounded-full opacity-75", isHealthy ? "bg-emerald-400" : "bg-amber-400")} />
              <span className={cn("relative inline-flex rounded-full h-2 w-2", isHealthy ? "bg-emerald-500" : "bg-amber-500")} />
            </span>
            <span className="font-mono">{isHealthy ? "All Systems Operational" : "Platform Degraded"}</span>
          </button>

          {/* Quick Notifications Button with Interactive Popover */}
          <div ref={notificationsRef} className="relative">
            <button
              type="button"
              aria-label="Notifications"
              aria-expanded={notificationsOpen}
              onClick={() => setNotificationsOpen(!notificationsOpen)}
              className="relative rounded-lg p-2 text-slate-400 hover:bg-white/[0.06] hover:text-white transition-colors cursor-pointer"
            >
              <NotificationBellIcon size={15} />
              {recentNotifications.length > 0 && (
                <span className="absolute top-1.5 right-1.5 flex h-2 w-2 rounded-full bg-[var(--brand)] ring-2 ring-[var(--surface)]" />
              )}
            </button>

            {notificationsOpen && (
              <div className="absolute right-0 top-full mt-2 w-80 rounded-xl border border-white/[0.1] bg-[var(--surface-raised)] p-3 shadow-2xl z-50">
                <div className="flex items-center justify-between pb-2 border-b border-white/[0.06]">
                  <span className="text-xs font-bold text-slate-100">Notifications</span>
                  {recentNotifications.length > 0 && (
                    <span className="rounded-full bg-rose-500/20 px-1.5 py-0.5 text-[10px] font-mono font-semibold text-rose-300">{recentNotifications.length} recent</span>
                  )}
                </div>
                {notificationsQuery.isPending ? (
                  <p className="py-3 text-center text-[11px] text-slate-500">Loading…</p>
                ) : notificationsQuery.isError ? (
                  <p className="py-3 text-center text-[11px] text-slate-500">Notifications are unavailable right now.</p>
                ) : recentNotifications.length === 0 ? (
                  <p className="py-3 text-center text-[11px] text-slate-500">No recent notifications.</p>
                ) : (
                  <div className="py-2 space-y-2 text-xs divide-y divide-white/[0.04]">
                    {recentNotifications.map((log) => (
                      <div key={log.id} className="pt-1.5 flex items-start gap-2">
                        <span
                          className={cn(
                            "mt-1 h-1.5 w-1.5 rounded-full shrink-0",
                            log.status === "failed" ? "bg-rose-400" : log.status === "pending" ? "bg-amber-400" : "bg-sky-400"
                          )}
                        />
                        <div className="min-w-0">
                          <p className="font-semibold text-slate-200 truncate">{EVENT_LABELS[log.eventType as keyof typeof EVENT_LABELS] ?? log.eventType}</p>
                          <p className="text-[11px] text-slate-400">{log.status} · {relativeTime(log.sentAt)}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
                <button
                  type="button"
                  onClick={() => { setNotificationsOpen(false); router.push("/admin/activity"); }}
                  className="mt-2 block w-full text-center text-[11px] font-semibold text-slate-400 hover:text-white pt-1.5 border-t border-white/[0.06]"
                >
                  View activity log →
                </button>
              </div>
            )}
          </div>

          {/* Platform Settings — single entry point (user menu links to Account instead) */}
          <button
            type="button"
            aria-label="Platform settings"
            onClick={() => router.push("/admin/settings")}
            className="rounded-lg p-2 text-slate-400 hover:bg-white/[0.06] hover:text-white transition-colors cursor-pointer"
          >
            <SettingsCogIcon size={15} />
          </button>

          {/* User Avatar Chip with Interactive Dropdown */}
          <div ref={userMenuRef} className="relative flex items-center pl-2 border-l border-white/[0.08]">
            <button
              type="button"
              onClick={() => setUserMenuOpen(!userMenuOpen)}
              className="flex items-center gap-2 rounded-lg p-1 hover:bg-white/[0.04] transition cursor-pointer"
            >
              <div className="grid h-7 w-7 place-items-center rounded-full bg-gradient-to-tr from-slate-700 to-slate-600 font-bold text-xs text-white uppercase ring-1 ring-white/10">
                {user?.email ? user.email.charAt(0) : "A"}
              </div>
              <div className="hidden 2xl:flex flex-col text-left">
                <span className="truncate max-w-[100px] text-xs font-semibold text-slate-200">
                  {user?.email ? user.email.split("@")[0] : "Admin"}
                </span>
                <span className="text-[10px] text-slate-500 capitalize">{user?.role || "Administrator"}</span>
              </div>
              <ChevronDown size={11} className={cn("text-slate-400 transition-transform", userMenuOpen && "rotate-180")} />
            </button>

            {userMenuOpen && (
              <div className="absolute right-0 top-full mt-2 w-56 rounded-xl border border-white/[0.1] bg-[var(--surface-raised)] p-1.5 shadow-2xl z-50 divide-y divide-white/[0.06]">
                <div className="px-3 py-2">
                  <p className="truncate text-xs font-bold text-slate-200">{user?.email || "admin@example.com"}</p>
                  <p className="text-[10px] font-mono text-slate-500 capitalize">{user?.role || "administrator"} · Default Org</p>
                </div>
                <div className="py-1 space-y-0.5">
                  <button
                    type="button"
                    onClick={() => { setUserMenuOpen(false); router.push("/account"); }}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-white"
                  >
                    <User size={13} />
                    <span>Account</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => { setUserMenuOpen(false); router.push("/admin/settings"); }}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-white"
                  >
                    <SettingsCogIcon size={13} />
                    <span>Platform Settings</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => { setUserMenuOpen(false); router.push("/admin/health"); }}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-white"
                  >
                    <CheckCircle2 size={13} />
                    <span>Diagnostics & Health</span>
                  </button>
                  <button
                    type="button"
                    onClick={() => { setUserMenuOpen(false); router.push("/admin/activity"); }}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-300 hover:bg-white/[0.06] hover:text-white"
                  >
                    <Clock size={13} />
                    <span>Activity Audit</span>
                  </button>
                </div>
                <div className="pt-1">
                  <button
                    type="button"
                    onClick={handleLogout}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-rose-300 hover:bg-rose-500/10 hover:text-rose-200"
                  >
                    <LogOut size={13} />
                    <span>Sign Out</span>
                  </button>
                </div>
              </div>
            )}
          </div>

          <button
            onClick={() => setCommandPaletteOpen(true)}
            aria-label="Open command palette"
            className="md:hidden rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] hover:text-white"
            type="button"
          >
            <Search size={16} />
          </button>
        </div>
      </header>

      <div className="flex h-[calc(100vh-56px)]">
        {/* Sidebar — independently scrollable */}
        <aside className="max-[899px]:hidden w-64 shrink-0 flex-col border-r border-[var(--line)] bg-[var(--surface)] flex">
          <div className="border-b border-[var(--line)] p-2.5">
            <div className="relative">
              <Search size={13} className="absolute left-2.5 top-2.5 text-[var(--text-muted)]" />
              <input
                aria-label="Search admin navigation"
                value={navSearch}
                onChange={(event) => setNavSearch(event.target.value)}
                placeholder="Filter navigation…"
                className="h-8 w-full rounded-md border border-[var(--line)] bg-[var(--surface-input)] pl-8 pr-2.5 text-xs text-[var(--text)] outline-none focus:border-[var(--focus)] focus:ring-1 focus:ring-[var(--focus)]"
              />
            </div>
          </div>

          <nav className="flex-1 overflow-y-auto px-2.5 py-3 space-y-3 scrollbar-thin">
            {navGroups.map((group) => {
              const searching = Boolean(navSearch.trim());
              const showSecondary = searching || isMoreExpanded(group.title);
              return (
              <div key={group.title} className="space-y-0.5">
                <button
                  type="button"
                  onClick={() => toggleGroup(group.title)}
                  className="flex w-full items-center justify-between rounded px-2 py-1 text-left text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-muted)] hover:bg-white/[0.04] hover:text-[var(--text)] motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                  aria-expanded={!isGroupCollapsed(group.title)}
                >
                  <span>
                    {tOr(group.titleKey, group.title)}
                  </span>
                  <ChevronDown
                    size={12}
                    className={cn(
                      "transition-transform motion-safe:transition-transform text-[var(--text-muted)]",
                      isGroupCollapsed(group.title) && "-rotate-90"
                    )}
                  />
                </button>

                {!isGroupCollapsed(group.title) && (
                  <div className="space-y-0.5 pt-0.5">
                    {group.items.map((item) => {
                      const Icon = item.icon;
                      const active = resolvedPath === item.href || resolvedPath.startsWith(`${item.href}/`);
                      return (
                        <Link
                          key={item.href}
                          aria-current={active ? "page" : undefined}
                          className={navItemClass(active)}
                          href={item.href}
                        >
                          <Icon size={14} className="shrink-0" />
                          <span className="truncate">{tOr(item.labelKey, item.label)}</span>
                          <NavStateLaneBadge hasPending={item.hasPendingGenerations} />
                        </Link>
                      );
                    })}
                    {group.secondaryItems.length > 0 && !showSecondary ? (
                      <button
                        type="button"
                        onClick={() => toggleMore(group.title)}
                        aria-expanded={false}
                        className="flex w-full items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[11px] font-medium text-[var(--text-muted)] hover:bg-white/[0.04] hover:text-[var(--text)]"
                      >
                        <ChevronDown size={12} className="-rotate-90" />
                        More ({group.secondaryItems.length})
                      </button>
                    ) : null}
                    {group.secondaryItems.length > 0 && showSecondary ? (
                      <>
                        {!searching ? (
                          <button
                            type="button"
                            onClick={() => toggleMore(group.title)}
                            aria-expanded={true}
                            className="flex w-full items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[11px] font-medium text-[var(--text-muted)] hover:bg-white/[0.04] hover:text-[var(--text)]"
                          >
                            <ChevronDown size={12} />
                            Less
                          </button>
                        ) : null}
                        {group.secondaryItems.map((item) => {
                          const Icon = item.icon;
                          const active = resolvedPath === item.href || resolvedPath.startsWith(`${item.href}/`);
                          return (
                            <Link
                              key={item.href}
                              aria-current={active ? "page" : undefined}
                              className={navItemClass(active)}
                              href={item.href}
                            >
                              <Icon size={14} className="shrink-0" />
                              <span className="truncate">{tOr(item.labelKey, item.label)}</span>
                              <NavStateLaneBadge hasPending={item.hasPendingGenerations} />
                            </Link>
                          );
                        })}
                      </>
                    ) : null}
                  </div>
                )}
              </div>
              );
            })}
          </nav>

          <div className="shrink-0 border-t border-[var(--line)] px-2.5 pt-2.5 pb-10 sm:pb-3 space-y-1.5">
            {/* Project Switcher Pill Matching Reference Screenshot */}
            <div ref={projectMenuRef} className="relative">
              <button
                type="button"
                onClick={() => setProjectMenuOpen(!projectMenuOpen)}
                className="flex items-center justify-between w-full px-2.5 py-1.5 rounded-lg border border-white/[0.08] bg-white/[0.02] hover:bg-white/[0.06] transition text-left cursor-pointer"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <PlanetDefaultIcon size={16} className="shrink-0" />
                  <div className="min-w-0">
                    <span className="block text-[9px] font-mono uppercase tracking-wider text-slate-500">Project</span>
                    <span className="block text-xs font-semibold text-slate-200 truncate">Default</span>
                  </div>
                </div>
                <ChevronDown size={12} className={cn("text-slate-400 transition-transform", projectMenuOpen && "rotate-180")} />
              </button>

              {projectMenuOpen && (
                <div className="absolute bottom-full left-0 mb-1.5 w-full rounded-xl border border-white/[0.1] bg-[var(--surface-raised)] p-1.5 shadow-2xl z-50 divide-y divide-white/[0.06]">
                  <div className="px-2 py-1 text-[10px] font-mono text-slate-500 uppercase tracking-wider">Switch Environment</div>
                  <div className="py-1 space-y-0.5">
                    <button
                      type="button"
                      onClick={() => setProjectMenuOpen(false)}
                      className="flex items-center justify-between w-full rounded-lg px-2 py-1.5 text-xs text-slate-200 hover:bg-white/[0.06] font-medium transition"
                    >
                      <span className="flex items-center gap-1.5">
                        <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
                        <span>Default (Production)</span>
                      </span>
                      <span className="text-[10px] font-mono text-emerald-400 font-semibold">Active</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => { setProjectMenuOpen(false); router.push("/admin/environments"); }}
                      className="flex items-center justify-between w-full rounded-lg px-2 py-1.5 text-xs text-slate-400 hover:bg-white/[0.06] hover:text-slate-200 transition"
                    >
                      <span className="flex items-center gap-1.5">
                        <span className="h-1.5 w-1.5 rounded-full bg-amber-400" />
                        <span>Staging</span>
                      </span>
                    </button>
                  </div>
                  <div className="pt-1">
                    <button
                      type="button"
                      onClick={() => { setProjectMenuOpen(false); router.push("/admin/projects"); }}
                      className="flex items-center justify-between w-full rounded-lg px-2 py-1 text-[11px] text-slate-400 hover:bg-white/[0.06] hover:text-white transition"
                    >
                      <span>Manage Projects…</span>
                    </button>
                  </div>
                </div>
              )}
            </div>

            <button
              onClick={handleLogout}
              className="flex items-center gap-2.5 w-full px-2.5 py-1.5 text-xs text-[var(--text-subtle)] hover:text-[var(--text)] hover:bg-white/[0.04] rounded-md transition-colors motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] cursor-pointer"
              type="button"
            >
              <LogOut size={14} />
              {tOr("auth.logout", "Sign Out")}
            </button>
          </div>
        </aside>

        {/* Content — independently scrollable */}
        <main className="min-w-0 flex-1 overflow-y-auto bg-[radial-gradient(circle_at_top_right,color-mix(in_srgb,var(--brand)_8%,transparent),transparent_28rem)] p-4 sm:p-5 lg:p-6">
          <div className="mx-auto max-w-[1440px]">
            {children}
          </div>
        </main>
      </div>

      {/* Mobile Drawer Navigation */}
      {mobileOpen ? (
        <div className="fixed inset-0 z-50 max-[899px]:flex hidden" role="dialog" aria-modal="true" aria-label="Admin navigation">
          <button
            aria-label="Close navigation overlay"
            className="absolute inset-0 bg-black/70 backdrop-blur-sm motion-safe:transition-opacity"
            onClick={() => setMobileOpen(false)}
            type="button"
          />
          <aside
            ref={drawerRef}
            className="relative flex h-full w-[min(88vw,320px)] flex-col border-r border-[var(--line)] bg-[var(--surface)] shadow-2xl motion-safe:animate-[slideIn_0.2s_ease-out]"
          >
            <div className="flex items-center justify-between border-b border-[var(--line)] p-3">
              <span className="text-sm font-semibold text-[var(--text)]">Admin navigation</span>
              <button
                ref={closeButtonRef}
                aria-label="Close admin navigation"
                className="rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                onClick={() => setMobileOpen(false)}
                type="button"
              >
                <X size={18} />
              </button>
            </div>
            <div className="border-b border-[var(--line)] p-2.5">
              <div className="relative">
                <Search size={13} className="absolute left-2.5 top-2.5 text-[var(--text-muted)]" />
                <input
                  aria-label="Search admin navigation"
                  value={navSearch}
                  onChange={(event) => setNavSearch(event.target.value)}
                  placeholder="Filter navigation…"
                  className="h-8 w-full rounded-md border border-[var(--line)] bg-[var(--surface-input)] pl-8 pr-2.5 text-xs text-[var(--text)] outline-none focus:border-[var(--focus)] focus:ring-1 focus:ring-[var(--focus)]"
                />
              </div>
            </div>
            <nav className="flex-1 overflow-y-auto px-2.5 py-3 space-y-3">
              {navGroups.map((group) => {
                const searching = Boolean(navSearch.trim());
                const showSecondary = searching || isMoreExpanded(group.title);
                return (
                <div key={group.title} className="space-y-0.5">
                  <button
                    type="button"
                    onClick={() => toggleGroup(group.title)}
                    className="flex w-full items-center justify-between rounded px-2 py-1 text-left text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-muted)]"
                    aria-expanded={!isGroupCollapsed(group.title)}
                  >
                    <span>
                      {tOr(group.titleKey, group.title)}
                    </span>
                    <ChevronDown
                      size={12}
                      className={cn(
                        "transition-transform motion-safe:transition-transform",
                        isGroupCollapsed(group.title) && "-rotate-90"
                      )}
                    />
                  </button>
                  {!isGroupCollapsed(group.title) && (
                    <div className="space-y-0.5 pt-0.5">
                      {[...group.items, ...(showSecondary ? group.secondaryItems : [])].map((item) => {
                        const Icon = item.icon;
                        const active = resolvedPath === item.href || resolvedPath.startsWith(`${item.href}/`);
                        return (
                          <Link
                            key={item.href}
                            aria-current={active ? "page" : undefined}
                            className={navItemClass(active)}
                            href={item.href}
                            onClick={() => setMobileOpen(false)}
                          >
                            <Icon size={14} className="shrink-0" />
                            <span className="truncate">{tOr(item.labelKey, item.label)}</span>
                            <NavStateLaneBadge hasPending={item.hasPendingGenerations} />
                          </Link>
                        );
                      })}
                      {group.secondaryItems.length > 0 && !showSecondary ? (
                        <button
                          type="button"
                          onClick={() => toggleMore(group.title)}
                          className="flex w-full items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[11px] font-medium text-[var(--text-muted)]"
                        >
                          <ChevronDown size={12} className="-rotate-90" />
                          More ({group.secondaryItems.length})
                        </button>
                      ) : null}
                    </div>
                  )}
                </div>
                );
              })}
            </nav>
          </aside>
        </div>
      ) : null}
    </div>
  );
}
