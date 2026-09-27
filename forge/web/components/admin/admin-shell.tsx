"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { LogOut, AlertTriangle, Menu, X, Search, ChevronDown, ChevronRight, CheckCircle2, Clock, User } from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchCurrentUser, logout, fetchHealthStatus, fetchNotificationLogs, EVENT_LABELS } from "@/lib/api";
import { API_BASE_URL } from "@/lib/api/http";
import { useBranding } from "@/components/branding";
import { useServerStore } from "@/stores/use-server-store";
import { useT } from "@/components/TranslationProvider";
import { useDismissOnOutside } from "@/lib/hooks/use-dismiss-on-outside";
import { navListKeyDown, useNavDrawer } from "@/lib/hooks/use-nav-drawer";
import {
  applyBreadcrumbOverrides,
  BreadcrumbProvider,
  useBreadcrumbOverrides,
} from "@/lib/nav/breadcrumb-context";
import {
  adminActiveHref,
  adminBreadcrumbTrail,
  adminEntryMatches,
  adminSidebarGroups,
  type AdminNavEntry,
  type AdminSidebarGroup,
} from "./admin-registry";
import { ForgeLogoIcon, NotificationBellIcon, SettingsCogIcon } from "@/components/ui/forge-icons";
import { ScopeSwitcher } from "./scope-switcher";

import { CommandPalette } from "./command-palette";

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

function NavStateLaneBadge({ hasPending }: { hasPending?: boolean }) {
  if (!hasPending) return null;
  return (
    <span className="ml-auto flex items-center gap-1" aria-label="Pending generation">
      <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />
      <span className="h-1.5 w-1.5 rounded-full bg-amber-400 motion-safe:animate-pulse" />
    </span>
  );
}

type NavLabeller = (label: string, labelKey: string) => string;

/**
 * One sidebar nav tree, rendered identically on desktop and in the mobile
 * drawer. Both surfaces previously carried their own copy of this markup and
 * had already drifted apart (the drawer lost the "Less" control and the
 * aria-expanded state on "More").
 *
 * Exactly one row is ever marked `aria-current="page"`: `activeHref` comes from
 * `adminActiveHref`, which resolves the single most specific registry entry.
 */
function SidebarNav({
  groups,
  activeHref,
  label,
  searching,
  isGroupCollapsed,
  onToggleGroup,
  isMoreExpanded,
  onToggleMore,
  onNavigate,
}: {
  groups: AdminSidebarGroup[];
  activeHref?: string;
  label: NavLabeller;
  searching: boolean;
  isGroupCollapsed: (title: string) => boolean;
  onToggleGroup: (title: string) => void;
  isMoreExpanded: (title: string) => boolean;
  onToggleMore: (title: string) => void;
  onNavigate?: () => void;
}) {
  const itemClass = (active: boolean) =>
    cn(
      "flex w-full items-center gap-2.5 rounded-md border-l-2 px-2.5 py-1.5 text-left text-xs font-medium transition-colors motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]",
      active
        ? "border-[var(--brand)] bg-[var(--brand)]/10 font-semibold text-[var(--brand)]"
        : "border-transparent text-[var(--text-subtle)] hover:bg-white/[0.04] hover:text-[var(--text)]",
    );

  const renderItem = (item: AdminNavEntry) => {
    const Icon = item.icon;
    const active = item.href === activeHref;
    return (
      <Link
        key={item.href}
        data-nav-item
        aria-current={active ? "page" : undefined}
        className={itemClass(active)}
        href={item.href}
        onClick={onNavigate}
      >
        <Icon size={14} className="shrink-0" />
        <span className="truncate">{label(item.label, item.labelKey)}</span>
        <NavStateLaneBadge hasPending={item.hasPendingGenerations} />
      </Link>
    );
  };

  if (groups.length === 0) {
    return (
      <p className="px-2.5 py-3 text-xs text-[var(--text-muted)]" role="status">
        No navigation entries match that filter.
      </p>
    );
  }

  return (
    <div className="space-y-3" onKeyDown={navListKeyDown}>
      {groups.map((group) => {
        const collapsed = isGroupCollapsed(group.title);
        const showSecondary = searching || isMoreExpanded(group.title);
        const activeIsHidden =
          collapsed && [...group.items, ...group.secondaryItems].some((item) => item.href === activeHref);
        return (
          <div key={group.title} className="space-y-0.5">
            <button
              type="button"
              onClick={() => onToggleGroup(group.title)}
              aria-expanded={!collapsed}
              className="flex w-full items-center justify-between rounded px-2 py-1 text-left text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-muted)] transition-colors hover:bg-white/[0.04] hover:text-[var(--text)] motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            >
              <span className="flex items-center gap-1.5">
                {label(group.title, group.titleKey)}
                {activeIsHidden ? (
                  <span className="h-1 w-1 rounded-full bg-[var(--brand)]" aria-label="Contains the current page" />
                ) : null}
              </span>
              <ChevronDown
                size={12}
                className={cn("text-[var(--text-muted)] transition-transform motion-safe:transition-transform", collapsed && "-rotate-90")}
              />
            </button>

            {!collapsed ? (
              <div className="space-y-0.5 pt-0.5">
                {group.items.map(renderItem)}
                {group.secondaryItems.length > 0 ? (
                  <>
                    {showSecondary ? group.secondaryItems.map(renderItem) : null}
                    {searching ? null : (
                      <button
                        type="button"
                        onClick={() => onToggleMore(group.title)}
                        aria-expanded={showSecondary}
                        className="flex w-full items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[11px] font-medium text-[var(--text-muted)] transition-colors hover:bg-white/[0.04] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                      >
                        {showSecondary ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                        {showSecondary ? "Show less" : `${group.secondaryItems.length} more`}
                      </button>
                    )}
                  </>
                ) : null}
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

/**
 * The admin control-plane frame: sidebar, top bar, breadcrumb trail, drawer.
 *
 * The breadcrumb provider wraps the whole frame so a detail page rendered as
 * `children` can name its own resource in the trail the top bar renders — one
 * trail with real names, rather than the shell's trail plus a second one drawn
 * by the page header.
 */
export function AdminShell({ children }: { children: React.ReactNode }) {
  return (
    <BreadcrumbProvider>
      <AdminShellFrame>{children}</AdminShellFrame>
    </BreadcrumbProvider>
  );
}

function AdminShellFrame({ children }: { children: React.ReactNode }) {
  const t = useT();
  const tOr = (key: string, fallback: string) => { const value = t(key); return value === key ? fallback : value; };
  const pathname = usePathname();
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
  const drawerRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const notificationsRef = useRef<HTMLDivElement>(null);
  const userMenuRef = useRef<HTMLDivElement>(null);

  useDismissOnOutside(notificationsRef, notificationsOpen, () => setNotificationsOpen(false));
  useDismissOnOutside(userMenuRef, userMenuOpen, () => setUserMenuOpen(false));

  const closeDrawer = useCallback(() => setMobileOpen(false), []);
  useNavDrawer({ open: mobileOpen, close: closeDrawer, containerRef: drawerRef, initialFocusRef: closeButtonRef });

  const searching = Boolean(navSearch.trim());
  const navGroups = useMemo(() => {
    const rawGroups = adminSidebarGroups(user?.role);
    if (!searching) return rawGroups;
    return rawGroups
      .map((group) => ({
        ...group,
        items: group.items.filter((item) => adminEntryMatches(item, navSearch)),
        secondaryItems: group.secondaryItems.filter((item) => adminEntryMatches(item, navSearch)),
      }))
      .filter((group) => group.items.length > 0 || group.secondaryItems.length > 0);
  }, [navSearch, searching, user?.role]);

  // Exactly one sidebar row is highlighted, and hidden pages highlight their
  // parent (e.g. /admin/health highlights Monitoring) rather than nothing.
  const activeHref = adminActiveHref(pathname);
  const crumbOverrides = useBreadcrumbOverrides();
  const crumbs = useMemo(
    () => applyBreadcrumbOverrides(adminBreadcrumbTrail(pathname), crumbOverrides),
    [pathname, crumbOverrides],
  );

  useEffect(() => {
    setMobileOpen(false);
    setNotificationsOpen(false);
    setUserMenuOpen(false);
  }, [pathname]);

  const toggleGroup = useCallback(
    (title: string) => setCollapsedGroups((current) => ({ ...current, [title]: !current[title] })),
    [],
  );
  const isGroupCollapsed = useCallback(
    (title: string) => Boolean(collapsedGroups[title]) && !searching,
    [collapsedGroups, searching],
  );
  const toggleMore = useCallback(
    (title: string) => setExpandedMore((current) => ({ ...current, [title]: !current[title] })),
    [],
  );
  const isMoreExpanded = useCallback((title: string) => Boolean(expandedMore[title]), [expandedMore]);
  const navLabel = useCallback<NavLabeller>(
    (fallback, key) => {
      const value = t(key);
      return value === key ? fallback : value;
    },
    [t],
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

  const navSearchInput = (id: string) => (
    <div className="relative">
      <Search size={13} className="absolute left-2.5 top-2.5 text-[var(--text-muted)]" />
      <input
        id={id}
        aria-label={tOr("admin.shell.filterNav", "Filter navigation")}
        value={navSearch}
        onChange={(event) => setNavSearch(event.target.value)}
        placeholder={tOr("admin.shell.filterNavPlaceholder", "Filter navigation…")}
        className="h-8 w-full rounded-md border border-[var(--line)] bg-[var(--surface-input)] pl-8 pr-2.5 text-xs text-[var(--text)] outline-none focus:border-[var(--focus)] focus:ring-1 focus:ring-[var(--focus)]"
      />
    </div>
  );

  return (
    <div className="h-screen overflow-hidden bg-[var(--canvas)]">
      <CommandPalette open={commandPaletteOpen} onOpenChange={setCommandPaletteOpen} />

      <a
        href="#forge-main"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[100] focus:rounded-lg focus:bg-[var(--brand)] focus:px-4 focus:py-2 focus:text-sm focus:font-semibold focus:text-white"
      >
        {tOr("a11y.skipToContent", "Skip to content")}
      </a>

      {/* Top bar — fixed */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-[var(--line)] bg-[var(--surface)] px-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <button
            aria-label={tOr("admin.shell.openNav", "Open admin navigation")}
            aria-expanded={mobileOpen}
            className="hidden rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)] max-[899px]:inline-flex"
            onClick={() => setMobileOpen(true)}
            type="button"
          >
            <Menu size={19} />
          </button>
          <Link
            className="flex shrink-0 items-center gap-2.5 text-base font-bold tracking-tight text-[var(--text)] transition-colors hover:text-[var(--brand)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            href="/admin/overview"
          >
            <ForgeLogoIcon size={24} className="shrink-0" />
            <span className="flex flex-col text-left">
              <span className="text-xs font-extrabold uppercase leading-tight tracking-wider text-[var(--text)]">{companyName || "Forge"}</span>
              <span className="font-mono text-[8px] uppercase tracking-widest text-[var(--text-muted)]">Control Plane</span>
            </span>
          </Link>

          {/* Breadcrumbs — the full trail, with every ancestor clickable. */}
          {crumbs.length > 0 ? (
            <nav aria-label="Breadcrumb" className="hidden min-w-0 items-center border-l border-[var(--line)] pl-3 font-mono text-xs sm:flex">
              <ol className="flex min-w-0 items-center gap-1.5">
                {crumbs.map((crumb, index) => (
                  <li key={`${crumb.label}-${index}`} className="flex min-w-0 items-center gap-1.5">
                    {index > 0 ? <span aria-hidden="true" className="text-[var(--text-muted)]">/</span> : null}
                    {crumb.href ? (
                      <Link
                        href={crumb.href}
                        className="truncate text-[var(--text-subtle)] transition-colors hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                      >
                        {crumb.label}
                      </Link>
                    ) : (
                      <span
                        aria-current={index === crumbs.length - 1 ? "page" : undefined}
                        className={cn("truncate", index === crumbs.length - 1 ? "font-semibold text-[var(--text)]" : "text-[var(--text-muted)]")}
                      >
                        {crumb.label}
                      </span>
                    )}
                  </li>
                ))}
              </ol>
            </nav>
          ) : null}
        </div>

        {/* Center: Command Palette Trigger */}
        <div className="mx-4 hidden max-w-md flex-1 md:block">
          <button
            type="button"
            onClick={() => setCommandPaletteOpen(true)}
            className="flex w-full cursor-pointer items-center justify-between rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-1.5 text-xs text-[var(--text-subtle)] transition-colors hover:border-[var(--line-strong)] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          >
            <span className="flex items-center gap-2">
              <Search size={13} className="text-[var(--text-muted)]" />
              <span className="truncate">{tOr("admin.shell.searchPlaceholder", "Search pages, servers, nodes, deployments…")}</span>
            </span>
            <kbd className="rounded border border-[var(--line)] bg-[var(--surface-raised)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--text-muted)]">
              ⌘K
            </kbd>
          </button>
        </div>

        {/* Right: Actions */}
        <div className="flex items-center gap-2.5 text-xs text-[var(--text-subtle)]">
          <PlatformStatusPill
            status={
              healthQuery.isPending
                ? "pending"
                : healthQuery.isError
                  ? "unknown"
                  : healthQuery.data?.status === "ok" && healthQuery.data?.ok !== false
                    ? "healthy"
                    : "degraded"
            }
            onClick={() => router.push("/admin/health")}
          />

          {/* Quick Notifications Button with Interactive Popover */}
          <div ref={notificationsRef} className="relative">
            <button
              type="button"
              aria-label={tOr("admin.shell.notifications", "Notifications")}
              aria-expanded={notificationsOpen}
              onClick={() => setNotificationsOpen(!notificationsOpen)}
              className="relative cursor-pointer rounded-lg p-2 text-[var(--text-subtle)] transition-colors hover:bg-white/[0.06] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            >
              <NotificationBellIcon size={15} />
              {recentNotifications.length > 0 && (
                <span className="absolute right-1.5 top-1.5 flex h-2 w-2 rounded-full bg-[var(--brand)] ring-2 ring-[var(--surface)]" />
              )}
            </button>

            {notificationsOpen && (
              <div className="absolute right-0 top-full z-50 mt-2 w-80 rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-3 shadow-2xl">
                <div className="flex items-center justify-between border-b border-[var(--line)] pb-2">
                  <span className="text-xs font-bold text-[var(--text)]">Notifications</span>
                  {recentNotifications.length > 0 && (
                    <span className="rounded-full bg-rose-500/20 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-rose-300">{recentNotifications.length} recent</span>
                  )}
                </div>
                {notificationsQuery.isPending ? (
                  <p className="py-3 text-center text-[11px] text-[var(--text-muted)]" role="status">Loading…</p>
                ) : notificationsQuery.isError ? (
                  <p className="py-3 text-center text-[11px] text-[var(--text-muted)]" role="alert">Notifications are unavailable right now.</p>
                ) : recentNotifications.length === 0 ? (
                  <p className="py-3 text-center text-[11px] text-[var(--text-muted)]">No recent notifications.</p>
                ) : (
                  <div className="space-y-2 divide-y divide-[var(--line)] py-2 text-xs">
                    {recentNotifications.map((log) => (
                      <div key={log.id} className="flex items-start gap-2 pt-1.5">
                        <span
                          className={cn(
                            "mt-1 h-1.5 w-1.5 shrink-0 rounded-full",
                            log.status === "failed" ? "bg-rose-400" : log.status === "pending" ? "bg-amber-400" : "bg-sky-400"
                          )}
                        />
                        <div className="min-w-0">
                          <p className="truncate font-semibold text-[var(--text)]">{EVENT_LABELS[log.eventType as keyof typeof EVENT_LABELS] ?? log.eventType}</p>
                          <p className="text-[11px] text-[var(--text-subtle)]">{log.status} · {relativeTime(log.sentAt)}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
                <button
                  type="button"
                  onClick={() => { setNotificationsOpen(false); router.push("/admin/activity"); }}
                  className="mt-2 block w-full border-t border-[var(--line)] pt-1.5 text-center text-[11px] font-semibold text-[var(--text-subtle)] hover:text-[var(--text)]"
                >
                  View activity log →
                </button>
              </div>
            )}
          </div>

          {/* Platform Settings — single entry point (user menu links to Account instead) */}
          <button
            type="button"
            aria-label={tOr("admin.nav.settings", "Platform Settings")}
            onClick={() => router.push("/admin/settings")}
            className="cursor-pointer rounded-lg p-2 text-[var(--text-subtle)] transition-colors hover:bg-white/[0.06] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          >
            <SettingsCogIcon size={15} />
          </button>

          {/* User Avatar Chip with Interactive Dropdown */}
          <div ref={userMenuRef} className="relative flex items-center border-l border-[var(--line)] pl-2">
            <button
              type="button"
              aria-expanded={userMenuOpen}
              aria-haspopup="menu"
              aria-label={tOr("admin.shell.accountMenu", "Account menu")}
              onClick={() => setUserMenuOpen(!userMenuOpen)}
              className="flex cursor-pointer items-center gap-2 rounded-lg p-1 transition hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            >
              <span className="grid h-7 w-7 place-items-center rounded-full bg-gradient-to-tr from-slate-700 to-slate-600 text-xs font-bold uppercase text-white ring-1 ring-white/10">
                {user?.email ? user.email.charAt(0) : "A"}
              </span>
              <span className="hidden flex-col text-left 2xl:flex">
                <span className="max-w-[100px] truncate text-xs font-semibold text-[var(--text)]">
                  {user?.email ? user.email.split("@")[0] : "Admin"}
                </span>
                <span className="text-[10px] capitalize text-[var(--text-muted)]">{user?.role || "Administrator"}</span>
              </span>
              <ChevronDown size={11} className={cn("text-[var(--text-muted)] transition-transform", userMenuOpen && "rotate-180")} />
            </button>

            {userMenuOpen && (
              <div role="menu" className="absolute right-0 top-full z-50 mt-2 w-56 divide-y divide-[var(--line)] rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-1.5 shadow-2xl">
                <div className="px-3 py-2">
                  <p className="truncate text-xs font-bold text-[var(--text)]">{user?.email}</p>
                  <p className="font-mono text-[10px] capitalize text-[var(--text-muted)]">{user?.role}</p>
                </div>
                <div className="space-y-0.5 py-1">
                  {[
                    { label: tOr("nav.account", "Account"), icon: User, href: "/account" },
                    { label: tOr("admin.nav.settings", "Platform Settings"), icon: SettingsCogIcon, href: "/admin/settings" },
                    { label: tOr("admin.nav.health", "Diagnostics"), icon: CheckCircle2, href: "/admin/health" },
                    { label: tOr("admin.nav.activity", "Activity"), icon: Clock, href: "/admin/activity" },
                  ].map(({ label, icon: Icon, href }) => (
                    <button
                      key={href}
                      type="button"
                      role="menuitem"
                      onClick={() => { setUserMenuOpen(false); router.push(href); }}
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-[var(--text-subtle)] hover:bg-white/[0.06] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                    >
                      <Icon size={13} />
                      <span>{label}</span>
                    </button>
                  ))}
                </div>
                <div className="pt-1">
                  <button
                    type="button"
                    role="menuitem"
                    onClick={handleLogout}
                    className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-rose-300 hover:bg-rose-500/10 hover:text-rose-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                  >
                    <LogOut size={13} />
                    <span>{tOr("auth.logout", "Sign Out")}</span>
                  </button>
                </div>
              </div>
            )}
          </div>

          <button
            onClick={() => setCommandPaletteOpen(true)}
            aria-label={tOr("admin.shell.openPalette", "Open command palette")}
            className="rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] hover:text-[var(--text)] md:hidden"
            type="button"
          >
            <Search size={16} />
          </button>
        </div>
      </header>

      <div className="flex h-[calc(100vh-56px)]">
        {/* Sidebar — independently scrollable */}
        <aside
          aria-label={tOr("admin.shell.primaryNav", "Admin navigation")}
          className="flex w-64 shrink-0 flex-col border-r border-[var(--line)] bg-[var(--surface)] max-[899px]:hidden"
        >
          <div className="border-b border-[var(--line)] p-2.5">{navSearchInput("admin-nav-filter")}</div>

          <nav className="scrollbar-thin flex-1 overflow-y-auto px-2.5 py-3">
            <SidebarNav
              groups={navGroups}
              activeHref={activeHref}
              label={navLabel}
              searching={searching}
              isGroupCollapsed={isGroupCollapsed}
              onToggleGroup={toggleGroup}
              isMoreExpanded={isMoreExpanded}
              onToggleMore={toggleMore}
            />
          </nav>

          <div className="shrink-0 space-y-1.5 border-t border-[var(--line)] px-2.5 pb-10 pt-2.5 sm:pb-3">
            <ScopeSwitcher />
            <button
              onClick={handleLogout}
              className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-1.5 text-xs text-[var(--text-subtle)] transition-colors hover:bg-white/[0.04] hover:text-[var(--text)] motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
              type="button"
            >
              <LogOut size={14} />
              {tOr("auth.logout", "Sign Out")}
            </button>
          </div>
        </aside>

        {/* Content — independently scrollable */}
        <main
          id="forge-main"
          tabIndex={-1}
          className="min-w-0 flex-1 overflow-y-auto bg-[radial-gradient(circle_at_top_right,color-mix(in_srgb,var(--brand)_8%,transparent),transparent_28rem)] p-4 sm:p-5 lg:p-6"
        >
          <div className="mx-auto max-w-[1440px]">{children}</div>
        </main>
      </div>

      {/* Mobile Drawer Navigation */}
      {mobileOpen ? (
        <div className="fixed inset-0 z-50 hidden max-[899px]:flex" role="dialog" aria-modal="true" aria-label={tOr("admin.shell.primaryNav", "Admin navigation")}>
          <button
            aria-label={tOr("common.close", "Close")}
            className="absolute inset-0 bg-black/70 backdrop-blur-sm motion-safe:transition-opacity"
            onClick={closeDrawer}
            type="button"
          />
          <aside
            ref={drawerRef}
            className="relative flex h-full w-[min(88vw,320px)] flex-col border-r border-[var(--line)] bg-[var(--surface)] shadow-2xl motion-safe:animate-[slideIn_0.2s_ease-out]"
          >
            <div className="flex items-center justify-between border-b border-[var(--line)] p-3">
              <span className="text-sm font-semibold text-[var(--text)]">{tOr("admin.shell.primaryNav", "Admin navigation")}</span>
              <button
                ref={closeButtonRef}
                aria-label={tOr("common.close", "Close")}
                className="rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                onClick={closeDrawer}
                type="button"
              >
                <X size={18} />
              </button>
            </div>
            <div className="border-b border-[var(--line)] p-2.5">{navSearchInput("admin-nav-filter-mobile")}</div>
            <nav className="flex-1 overflow-y-auto px-2.5 py-3">
              <SidebarNav
                groups={navGroups}
                activeHref={activeHref}
                label={navLabel}
                searching={searching}
                isGroupCollapsed={isGroupCollapsed}
                onToggleGroup={toggleGroup}
                isMoreExpanded={isMoreExpanded}
                onToggleMore={toggleMore}
                onNavigate={closeDrawer}
              />
            </nav>
            <div className="shrink-0 border-t border-[var(--line)] p-2.5">
              <ScopeSwitcher />
            </div>
          </aside>
        </div>
      ) : null}
    </div>
  );
}

/**
 * Platform status pill. "Unknown" is a distinct state from "healthy": when the
 * health endpoint cannot be reached the pill must not claim all systems are
 * operational.
 */
function PlatformStatusPill({
  status,
  onClick,
}: {
  status: "pending" | "healthy" | "degraded" | "unknown";
  onClick: () => void;
}) {
  const config = {
    pending: { text: "Checking status…", dot: "bg-slate-400", chip: "border-[var(--line)] bg-white/[0.03] text-[var(--text-subtle)]", pulse: false },
    healthy: { text: "All Systems Operational", dot: "bg-emerald-500", chip: "border-emerald-500/25 bg-emerald-500/10 text-emerald-300", pulse: true },
    degraded: { text: "Platform Degraded", dot: "bg-amber-500", chip: "border-amber-500/30 bg-amber-500/15 text-amber-300", pulse: true },
    unknown: { text: "Status Unknown", dot: "bg-rose-500", chip: "border-rose-500/30 bg-rose-500/10 text-rose-300", pulse: false },
  }[status];

  return (
    <button
      type="button"
      onClick={onClick}
      title="Inspect platform diagnostics & health checks"
      className={cn(
        "hidden cursor-pointer items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium transition hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] xl:flex",
        config.chip,
      )}
    >
      <span className="relative flex h-2 w-2">
        {config.pulse ? (
          <span className={cn("absolute inline-flex h-full w-full animate-ping rounded-full opacity-75", config.dot)} />
        ) : null}
        <span className={cn("relative inline-flex h-2 w-2 rounded-full", config.dot)} />
      </span>
      <span className="font-mono">{config.text}</span>
    </button>
  );
}
