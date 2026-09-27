"use client";

import Link from "next/link";
import { Fragment } from "react";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchCurrentUser, fetchServer } from "@/lib/api";
import { useT } from "@/components/TranslationProvider";
import { resolveActiveHref } from "@/lib/nav/active";
import { navListKeyDown } from "@/lib/hooks/use-nav-drawer";
import { computeServerAccess, hasServerPermission } from "@/components/server/server-context";
import {
  SERVERS_LIST_HREF,
  consoleNavGroups,
  workloadTabs,
  workloadTabGroups,
  workloadTabHref,
  type WorkloadTabId,
} from "@/components/console/console-registry";

export type { ConsoleNavItem, ConsoleNavGroup } from "@/components/console/console-registry";

/**
 * Which server's tabs the sidebar should expand, if any.
 *
 * Workload detail pages live at `/server/[id]/*` (see `workloadTabHref`), so
 * that — not `/console/servers/[id]` — is what we match. `/servers` (the list)
 * deliberately does not match: there is no single server to expand there.
 */
function matchServerId(pathname: string): string | null {
  const match = pathname.match(/^\/server\/([^/]+)(?:\/|$)/);
  return match?.[1] ?? null;
}

/**
 * Every href this nav can highlight, in one list.
 *
 * Active state is resolved once, across contextual server tabs *and* top-level
 * destinations, by the shared longest-match resolver. Resolving the two lists
 * separately is what produced the old split behaviour: the top level used its
 * own prefix sort while the server tabs used `pathname === href`, so any
 * deeper path (a file browser sub-path, a backup detail) highlighted nothing.
 */
function navHrefs(serverId: string | null): string[] {
  return [
    ...consoleNavGroups.flatMap((group) => group.items.map((item) => item.href)),
    ...(serverId ? workloadTabs.map((tab) => workloadTabHref(serverId, tab.id as WorkloadTabId)) : []),
  ];
}

function statusDot(server: { suspended?: boolean; transferring?: boolean; status?: string }) {
  if (server.suspended) return "bg-rose-400";
  if (server.transferring) return "bg-sky-400";
  if (server.status === "running") return "bg-emerald-400";
  if (server.status === "installing") return "bg-amber-400";
  return "bg-slate-500";
}

function navItemClass(active: boolean, minHeight: string) {
  return cn(
    "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]",
    minHeight,
    active
      ? "border-l-2 border-[var(--brand)] bg-[var(--brand)]/10 pl-2.5 font-semibold text-[var(--brand)]"
      : "text-[var(--text-subtle)] hover:bg-white/[0.04] hover:text-[var(--text)]",
  );
}

function ServerSection({ activeHref, onNavigate }: { activeHref?: string; onNavigate?: () => void }) {
  const t = useT();
  const pathname = usePathname();
  const serverId = matchServerId(pathname);

  const serverQuery = useQuery({
    queryKey: ["server", serverId],
    queryFn: () => fetchServer(serverId as string),
    enabled: Boolean(serverId),
    staleTime: 30_000,
    retry: 1,
  });
  const userQuery = useQuery({
    queryKey: ["current-user"],
    queryFn: fetchCurrentUser,
    staleTime: 30_000,
    retry: 1,
  });

  if (!serverId) return null;

  const server = serverQuery.data;
  const user = userQuery.data ?? null;
  const access = server ? computeServerAccess(server, user) : null;
  const tr = (key: string, fallback: string) => { const value = t(key); return value === key ? fallback : value; };
  const visibleTabs = access
    ? workloadTabs.filter((tab) => hasServerPermission(access, tab.permissions))
    : [];
  const state = server
    ? (server.suspended ? tr("server.nav.suspended", "Suspended")
      : server.transferring ? tr("server.nav.transferring", "Transferring")
      : server.status || tr("server.nav.unknown", "Unknown"))
    : null;

  return (
    <div>
      <Link
        className="mb-2 flex items-center gap-1 px-3 text-[11px] font-medium text-[var(--text-muted)] transition-colors hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
        href={SERVERS_LIST_HREF}
        onClick={onNavigate}
      >
        <ChevronLeft size={12} /> {tr("server.nav.allServers", "All servers")}
      </Link>
      <div className="mb-2 flex items-center gap-2 rounded-lg bg-white/[0.03] px-3 py-2.5">
        <span className={cn("h-1.5 w-1.5 shrink-0 rounded-full", server ? statusDot(server) : "motion-safe:animate-pulse bg-slate-600")} />
        <p className="min-w-0 truncate text-xs font-bold text-[var(--text)]">{server?.name ?? serverId}</p>
        {state ? <span className="ml-auto shrink-0 text-[10px] text-[var(--text-muted)]">{state}</span> : null}
      </div>
      <div className="space-y-3" onKeyDown={navListKeyDown}>
        {workloadTabGroups.map((group) => {
          const groupVisible = group.tabs
            .map((id) => visibleTabs.find((tab) => tab.id === id))
            .filter(Boolean) as typeof visibleTabs;
          if (groupVisible.length === 0) return null;
          return (
            <div key={group.title}>
              <p className="mb-1 px-3 text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">{group.title}</p>
              <ul className="space-y-0.5">
                {groupVisible.map((tab) => {
                  const Icon = tab.icon as LucideIcon;
                  const href = workloadTabHref(serverId, tab.id);
                  const active = href === activeHref;
                  const label = t(tab.labelKey);
                  return (
                    <li key={tab.id}>
                      <Link
                        data-nav-item
                        aria-current={active ? "page" : undefined}
                        className={navItemClass(active, "min-h-9")}
                        href={href}
                        onClick={onNavigate}
                      >
                        <Icon size={14} className="shrink-0" />
                        <span className="truncate">{label === tab.labelKey ? tab.fallback : label}</span>
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          );
        })}
        {!access && serverQuery.isPending ? (
          <div className="flex items-center gap-2.5 rounded-lg px-3 py-2.5 text-sm text-[var(--text-subtle)]" role="status">
            <span className="h-3.5 w-3.5 rounded bg-white/[0.06] motion-safe:animate-pulse" />
            <span className="text-xs motion-safe:animate-pulse">{tr("common.loading", "Loading server…")}</span>
          </div>
        ) : null}
        {serverQuery.isError ? (
          <p className="px-3 text-xs text-[var(--text-muted)]" role="alert">
            {tr("server.nav.unavailable", "Server unavailable — tabs hidden.")}
          </p>
        ) : null}
      </div>
    </div>
  );
}

export function ConsoleNav({ onNavigate }: { onNavigate?: () => void } = {}) {
  const pathname = usePathname();
  const t = useT();

  const serverId = matchServerId(pathname);
  const activeHref = resolveActiveHref(pathname, navHrefs(serverId));

  return (
    <div className="space-y-5">
      {serverId ? <ServerSection activeHref={activeHref} onNavigate={onNavigate} /> : null}
      <div className="space-y-5" onKeyDown={navListKeyDown}>
        {consoleNavGroups.map((group) => {
          // No availability filter: every registry href is asserted to resolve
          // by test/route-integrity.test.ts, so filtering here would only ever
          // hide a working link. The previous hand-maintained allowlist did the
          // opposite — it green-lit /console/servers and /console/backups, which
          // had no pages at all.
          if (group.items.length === 0) return null;
          return (
            <Fragment key={group.title}>
              <div>
                <p className="mb-1 px-3 text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-muted)]">
                  {group.title}
                </p>
                <ul className="space-y-0.5">
                  {group.items.map((item) => {
                    const Icon = item.icon;
                    const active = item.href === activeHref;
                    const label = item.labelKey ? t(item.labelKey) : item.label;
                    return (
                      <li key={item.href}>
                        <Link
                          data-nav-item
                          aria-current={active ? "page" : undefined}
                          className={navItemClass(active, "min-h-11")}
                          href={item.href}
                          onClick={onNavigate}
                        >
                          <Icon size={15} className="shrink-0" />
                          <span className="truncate">{label === item.labelKey ? item.label : label}</span>
                          {item.badge ? (
                            <span className="ml-auto rounded-full bg-white/[0.06] px-2 py-0.5 text-[10px] font-bold text-[var(--text-subtle)]">{item.badge}</span>
                          ) : null}
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </div>
            </Fragment>
          );
        })}
      </div>
    </div>
  );
}
