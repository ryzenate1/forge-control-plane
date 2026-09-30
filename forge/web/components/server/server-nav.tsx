"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useCallback, useRef, useState } from "react";
import { ChevronLeft, LogOut, Menu, User, X, type LucideIcon } from "lucide-react";
import { type ApiServer, logout } from "@/lib/api";
import { useServerStore } from "@/stores/use-server-store";
import { useTenancyStore } from "@/stores/use-tenancy-store";
import { cn } from "@/lib/utils";
import { useT } from "@/components/TranslationProvider";
import { resolveActiveHref } from "@/lib/nav/active";
import { navListKeyDown, useNavDrawer } from "@/lib/hooks/use-nav-drawer";
import {
  WORKLOAD_TAB_PARENTS,
  workloadTabGroups,
  workloadTabHref,
  workloadTabs,
  type WorkloadTabId,
} from "@/components/console/console-registry";
import { hasServerPermission, type ServerAccess } from "./server-context";

/**
 * A tab of the server console.
 *
 * Derived from the shared workload registry rather than declared again here.
 * This file used to carry its own 19-entry array, which had drifted: it was
 * missing the `git` tab that has a real page, and it listed both `database` and
 * `databases` as sibling rows.
 */
export type ServerTab = WorkloadTabId;

interface ServerNavProps {
  serverId: string;
  server: ApiServer;
  access: ServerAccess;
  /**
   * Optional hint from the page. Active state is normally resolved from the
   * pathname — which handles sub-paths the last-segment heuristic got wrong —
   * and this is consulted only when the pathname matches no known tab.
   */
  activeTab?: ServerTab;
}

function statusTone(server: ApiServer) {
  if (server.suspended) return "border-[var(--danger-line)] bg-[var(--danger-subtle)] text-[var(--danger)]";
  if (server.transferring) return "border-[var(--info-line)] bg-[var(--info-subtle)] text-[var(--info)]";
  if (server.status === "running") return "border-[var(--ok-line)] bg-[var(--ok-subtle)] text-[var(--ok)]";
  if (server.status === "installing") return "border-[var(--warn-line)] bg-[var(--warn-subtle)] text-[var(--warn)]";
  return "border-[var(--unknown-line)] bg-[var(--unknown-subtle)] text-[var(--unknown)]";
}

export function ServerNav({ serverId, server, access, activeTab }: ServerNavProps) {
  const t = useT();
  const pathname = usePathname();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const drawerRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useNavDrawer({ open, close, containerRef: drawerRef, initialFocusRef: closeButtonRef });

  const tr = (key: string, fallback: string) => { const value = t(key); return value === key ? fallback : value; };
  const visibleTabs = workloadTabs.filter((tab) => (!tab.adminOnly || access.isAdmin) && hasServerPermission(access, tab.permissions));

  // Longest match wins, so /files/sub/dir still highlights Files and
  // /databases/services highlights Managed Services rather than Databases.
  const resolved = resolveActiveHref(pathname, visibleTabs.map((tab) => workloadTabHref(serverId, tab.id)));
  const activeHref = resolved ?? (activeTab ? workloadTabHref(serverId, activeTab) : undefined);

  const state = server.suspended
    ? tr("server.nav.suspended", "Suspended")
    : server.transferring
      ? tr("server.nav.transferring", "Transferring")
      : server.status || tr("server.nav.unknown", "Unknown");
  const signOut = async () => {
    try {
      await logout();
    } finally {
      useServerStore.getState().reset();
      useTenancyStore.getState().reset();
      router.push("/");
    }
  };

  const content = (
    <>
      <div className="border-b border-[var(--line)] p-4">
        <Link
          href="/servers"
          className="mb-3 inline-flex items-center gap-1 text-xs text-[var(--text-muted)] transition-colors hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          onClick={close}
        >
          <ChevronLeft size={13} /> {tr("server.myServers", "My servers")}
        </Link>
        <h1 className="truncate text-base font-bold text-[var(--text)]" title={server.name}>{server.name}</h1>
        {server.description ? <p className="mt-1 line-clamp-2 text-xs text-[var(--text-muted)]">{server.description}</p> : null}
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className={cn("rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider", statusTone(server))}>{state}</span>
          {server.allocation ? <span className="font-mono text-[10px] text-[var(--text-muted)]">{server.allocation}</span> : null}
        </div>
        <p className="mt-2 truncate text-[10px] text-[var(--text-muted)]">
          {server.node ? `${tr("server.nav.node", "Node")}: ${server.node}` : tr("server.nav.nodeUnavailable", "Node unavailable")} · {server.id}
        </p>
      </div>

      <nav
        aria-label={tr("server.nav.ariaLabel", "Server navigation")}
        className="flex-1 space-y-3 overflow-y-auto p-2"
        onKeyDown={navListKeyDown}
      >
        {/*
          Grouped rather than a flat run of 19 rows. Daily work first,
          configuration next, deploy and lifecycle operations last — the same
          grouping the console sidebar uses, from the same source.
        */}
        {workloadTabGroups.map((group) => {
          const groupTabs = group.tabs
            .map((id) => visibleTabs.find((tab) => tab.id === id))
            .filter((tab): tab is (typeof workloadTabs)[number] => Boolean(tab));
          if (groupTabs.length === 0) return null;
          return (
            <div key={group.title}>
              <p className="mb-1 px-3 text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-muted)]">
                {group.title}
              </p>
              {groupTabs.map((tab) => {
                const Icon = tab.icon as LucideIcon;
                const href = workloadTabHref(serverId, tab.id);
                const selected = href === activeHref;
                const nested = Boolean(WORKLOAD_TAB_PARENTS[tab.id]);
                const label = t(tab.labelKey);
                return (
                  <Link
                    key={tab.id}
                    data-nav-item
                    aria-current={selected ? "page" : undefined}
                    className={cn(
                      "mb-0.5 flex items-center gap-2 rounded-lg py-2.5 text-sm font-medium transition-colors motion-safe:transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]",
                      nested ? "ml-3 border-l border-[var(--line)] pl-3 text-[13px]" : "px-3",
                      selected
                        ? "bg-[color-mix(in_srgb,var(--brand)_10%,transparent)] font-semibold text-[var(--brand)]"
                        : "text-[var(--text-subtle)] hover:bg-white/[0.05] hover:text-[var(--text)]",
                    )}
                    href={href}
                    onClick={close}
                  >
                    <Icon size={nested ? 14 : 16} className="shrink-0" />
                    <span className="truncate">{label === tab.labelKey ? tab.fallback : label}</span>
                  </Link>
                );
              })}
            </div>
          );
        })}

        {!access.isAdmin && !access.isOwner && access.permissions === null ? (
          <p className="m-2 rounded border border-amber-500/30 bg-amber-500/10 p-2 text-xs text-amber-200" role="alert">
            {tr("server.nav.permissionsUnverified", "Permissions could not be verified. Navigation is restricted.")}
          </p>
        ) : null}
      </nav>

      <div className="space-y-1 border-t border-[var(--line)] p-2">
        <Link
          className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-[var(--text-subtle)] transition-colors hover:bg-white/[0.05] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          href="/account"
          onClick={close}
        >
          <User size={15} />{tr("server.nav.account", "Account")}
        </Link>
        <button
          className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-[var(--text-subtle)] transition-colors hover:bg-white/[0.05] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          onClick={signOut}
          type="button"
        >
          <LogOut size={15} />{tr("server.nav.signOut", "Sign out")}
        </button>
      </div>
    </>
  );

  return (
    <>
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-[var(--line)] bg-[color-mix(in_srgb,var(--surface)_95%,transparent)] px-4 backdrop-blur md:hidden">
        <div className="min-w-0">
          <p className="truncate text-sm font-bold text-[var(--text)]">{server.name}</p>
          <p className="truncate font-mono text-[10px] text-[var(--text-muted)]">{server.allocation || state}</p>
        </div>
        <button
          ref={closeButtonRef}
          aria-expanded={open}
          aria-label={tr("server.nav.toggleNav", "Toggle server navigation")}
          className="rounded-lg p-2 text-[var(--text-subtle)] hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          onClick={() => setOpen((value) => !value)}
          type="button"
        >
          {open ? <X /> : <Menu />}
        </button>
      </header>

      {open ? (
        <button
          aria-label={tr("server.nav.closeNav", "Close server navigation")}
          className="fixed inset-0 z-30 bg-black/60 md:hidden"
          onClick={close}
          type="button"
        />
      ) : null}

      {/*
        One element serves as both the static desktop rail and the mobile
        drawer. It only takes dialog semantics while it is an overlay — marking
        the permanently visible desktop rail as a modal dialog would trap focus
        for every keyboard user on every page.
      */}
      <aside
        ref={drawerRef}
        {...(open ? { role: "dialog" as const, "aria-modal": true, "aria-label": tr("server.nav.ariaLabel", "Server navigation") } : {})}
        className={cn(
          "fixed inset-y-0 left-0 z-40 flex w-72 flex-col border-r border-[var(--line)] bg-[var(--surface)] transition-transform motion-safe:transition-transform md:sticky md:top-0 md:h-screen md:w-64 md:translate-x-0",
          open ? "translate-x-0" : "-translate-x-full",
        )}
      >
        {content}
      </aside>
    </>
  );
}
