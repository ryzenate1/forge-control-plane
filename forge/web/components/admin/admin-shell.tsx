"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { LogOut, AlertTriangle, Menu, X, Search, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchCurrentUser, logout } from "@/lib/api";
import { API_BASE_URL } from "@/lib/api/http";
import { useBranding } from "@/components/branding";
import { useServerStore } from "@/stores/use-server-store";
import { useT } from "@/components/TranslationProvider";
import { adminPagesForRole, findAdminPage } from "./admin-registry";

export function AdminShell({ children }: { children: React.ReactNode }) {
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

  useEffect(() => {
    if (userQuery.data === null) {
      router.replace("/");
    } else if (user && user.role !== "admin") {
      router.replace("/servers");
    }
  }, [router, user, userQuery.data]);

  const [mobileOpen, setMobileOpen] = useState(false);
  const [navSearch, setNavSearch] = useState("");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const navGroups = useMemo(() => {
    // Group by the administrator's goal, not by the backend table or service
    // that happens to implement the page. This keeps the first scan predictable
    // and puts frequent decisions before specialist controls.
    const plan = [
      { title: "Command Center", titleKey: "admin.navGroup.commandCenter", hrefs: ["/admin/overview", "/admin/monitoring", "/admin/health", "/admin/activity", "/admin/cron-jobs", "/admin/host", "/admin/operations"] },
      { title: "Workloads", titleKey: "admin.navGroup.workloads", hrefs: ["/admin/servers", "/admin/apps", "/admin/deployments", "/admin/preview-deployments", "/admin/source-deployments", "/admin/compose", "/admin/app-store"] },
      { title: "People & Access", titleKey: "admin.navGroup.peopleAccess", hrefs: ["/admin/users", "/admin/roles", "/admin/organizations", "/admin/projects", "/admin/environments", "/admin/oauth-clients"] },
      { title: "Infrastructure & Data", titleKey: "admin.navGroup.infrastructureData", hrefs: ["/admin/regions", "/admin/locations", "/admin/nodes", "/admin/allocations", "/admin/databases", "/admin/database-services", "/admin/mounts", "/admin/files", "/admin/terminal", "/admin/docker", "/admin/cloud", "/admin/backups"] },
      { title: "Networking & Security", titleKey: "admin.navGroup.networkingSecurity", hrefs: ["/admin/endpoints", "/admin/firewall", "/admin/load-balancer", "/admin/traffic", "/admin/domains", "/admin/certificates", "/admin/mtls", "/admin/social", "/admin/git", "/admin/git-providers", "/admin/webhooks"] },
      { title: "Platform Configuration", titleKey: "admin.navGroup.platformConfiguration", hrefs: ["/admin/nests", "/admin/app-templates", "/admin/templates", "/admin/plugins", "/admin/settings", "/admin/api", "/admin/notifications", "/admin/scheduler", "/admin/autoscaler", "/admin/failover"] },
    ];
    const entries = adminPagesForRole(user?.role).flatMap((group) => group.items);
    return plan.map((group) => ({ ...group, items: group.hrefs.map((href) => entries.find((item) => item.href === href)).filter((item): item is (typeof entries)[number] => Boolean(item)) }))
      .filter((group) => group.items.length > 0)
      .map((group) => ({ ...group, items: group.items.filter((item) => !navSearch.trim() || `${item.label} ${item.description}`.toLowerCase().includes(navSearch.toLowerCase())) }))
      .filter((group) => group.items.length > 0);
  }, [navSearch, user?.role]);
  const currentPage = findAdminPage(pathname);
  useEffect(() => { setMobileOpen(false); }, [pathname]);
  const toggleGroup = (title: string) => setCollapsedGroups((current) => ({ ...current, [title]: !current[title] }));
  const isGroupCollapsed = (title: string) => Boolean(collapsedGroups[title] && !navSearch.trim());

  const handleLogout = async () => {
      await logout();
      setCurrentUser(null);
      router.push("/");
    };

  if (userQuery.isPending) {
      return (
        <div className="grid min-h-screen place-items-center bg-[#0f1419] p-4 text-sm text-slate-400">
          {tOr("admin.shell.redirecting", "Redirecting to sign in…")}
        </div>
      );
    }

    if (!userQuery.data) {
    return (
      <div className="grid min-h-screen place-items-center bg-[#0f1419] p-4">
        <div className="w-full max-w-md space-y-4 rounded-xl border border-red-500/30 bg-[#1e2536] p-6 text-center" role="alert">
          <AlertTriangle size={28} className="mx-auto text-amber-400" strokeWidth={1.5} />
          <h1 className="text-xl font-bold text-slate-100">{tOr("admin.shell.verifyFailedTitle", "Unable to verify admin access")}</h1>
          <p className="text-sm text-red-300">
            {userQuery.isError
              ? `${tOr("admin.shell.apiUnreachable", "API not reachable at")} ${API_BASE_URL}. ${tOr("admin.shell.apiUnreachableHint", "Make sure the Go backend is running.")}`
              : tOr("admin.shell.userLoadFailed", "The current user could not be loaded. Admin content remains hidden until the API responds.")
            }
          </p>
          <div className="flex justify-center gap-2">
            <button
              className="rounded-lg bg-[#dc2626] px-4 py-2 text-sm font-bold text-white hover:bg-[#b91c1c] disabled:opacity-60 transition-colors"
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
      <div className="grid min-h-screen place-items-center bg-[#0f1419] p-4 text-sm text-slate-400">
        {tOr("admin.shell.verifying", "Verifying admin access…")}
      </div>
    );
  }

  return (
    <div className="h-screen overflow-hidden bg-[#0a0e14]">
      {/* Top bar — fixed */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/[0.06] bg-[#0f1520] px-4 sm:px-6">
        <button aria-label="Open admin navigation" className="mr-3 rounded-lg p-2 text-slate-300 hover:bg-white/[0.06] max-[899px]:inline-flex hidden" onClick={() => setMobileOpen(true)} type="button"><Menu size={19} /></button>
        <button
          className="text-lg font-bold text-slate-100 tracking-tight"
          onClick={() => router.push("/servers")}
          type="button"
        >
          {companyName}
        </button>
        <div className="flex items-center gap-3 text-sm text-slate-400">
          <button
            className="hover:text-white transition-colors"
            onClick={() => router.push("/servers")}
            type="button"
          >
            {tOr("server.myServers", "My Servers")}
          </button>
        </div>
      </header>

      <div className="flex h-[calc(100vh-56px)]">
        {/* Sidebar — independently scrollable */}
        <aside className="max-[899px]:hidden w-72 shrink-0 flex-col border-r border-white/[0.06] bg-[#11161f] flex">
          <div className="border-b border-white/[0.06] p-3"><div className="relative"><Search size={14} className="absolute left-3 top-2.5 text-slate-600"/><input aria-label="Search admin navigation" value={navSearch} onChange={(event) => setNavSearch(event.target.value)} placeholder="Find a control…" className="h-9 w-full rounded-lg border border-white/[0.08] bg-black/20 pl-9 pr-3 text-xs text-slate-200 outline-none focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"/></div></div>
          <nav className="flex-1 overflow-y-auto px-3 py-5">
            {navGroups.map((group) => (
              <div key={group.title} className="mb-4">
                <button type="button" onClick={() => toggleGroup(group.title)} className="mb-1 flex w-full items-center justify-between rounded-md px-3 py-1.5 text-left text-[10px] font-bold uppercase tracking-[0.12em] text-slate-500 hover:bg-white/[0.04] hover:text-slate-300" aria-expanded={!isGroupCollapsed(group.title)}>
                  <span>{tOr(group.titleKey, group.title)} <span className="ml-1 text-slate-700">{group.items.length}</span></span>
                  <ChevronDown size={13} className={cn("transition-transform", isGroupCollapsed(group.title) && "-rotate-90")} />
                </button>
                {!isGroupCollapsed(group.title) && group.items.map((item) => {
                  const Icon = item.icon;
                  const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
                  return (
                    <button
                      key={item.href}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-all text-left",
                        active
                          ? "border-l-2 border-red-400 bg-red-500/10 pl-2.5 text-red-300 shadow-sm shadow-red-950/20"
                          : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200",
                      )}
                      onClick={() => router.push(item.href)}
                      type="button"
                    >
                      <Icon size={15} className="shrink-0" />
                      <span className="truncate">{tOr(item.labelKey, item.label)}</span>
                    </button>
                  );
                })}
              </div>
            ))}
          </nav>
          <div className="shrink-0 border-t border-white/[0.06] px-3 py-3">
            <button
              onClick={handleLogout}
              className="flex items-center gap-2.5 w-full px-3 py-2 text-sm text-slate-500 hover:text-slate-200 hover:bg-white/[0.04] rounded-lg transition-all"
              type="button"
            >
              <LogOut size={15} />
              {tOr("auth.logout", "Sign Out")}
            </button>
          </div>
        </aside>

        {/* Content — independently scrollable */}
        <main className="min-w-0 flex-1 overflow-y-auto bg-[radial-gradient(circle_at_top_right,rgba(127,29,29,0.10),transparent_32rem)] p-4 sm:p-6 lg:p-8">
          {currentPage ? <div className="mb-5 hidden border-b border-white/[0.06] pb-4 md:block"><p className="text-[10px] font-bold uppercase tracking-[0.16em] text-red-400">{currentPage.label}</p><p className="mt-1 text-sm text-slate-500">{currentPage.description}</p></div> : null}{children}
        </main>
      </div>
      {mobileOpen ? <div className="fixed inset-0 z-50 max-[899px]:flex hidden"><button aria-label="Close navigation overlay" className="absolute inset-0 bg-black/70" onClick={() => setMobileOpen(false)} type="button"/><aside className="relative flex h-full w-[min(88vw,340px)] flex-col border-r border-white/[0.06] bg-[#11161f] shadow-2xl"><div className="flex items-center justify-between border-b border-white/[0.06] p-4"><span className="text-sm font-semibold text-slate-200">Admin navigation</span><button aria-label="Close admin navigation" className="rounded-lg p-2 text-slate-400 hover:bg-white/[0.06]" onClick={() => setMobileOpen(false)} type="button"><X size={18}/></button></div><div className="border-b border-white/[0.06] p-3"><div className="relative"><Search size={14} className="absolute left-3 top-2.5 text-slate-600"/><input aria-label="Search admin navigation" value={navSearch} onChange={(event) => setNavSearch(event.target.value)} placeholder="Find a control…" className="h-9 w-full rounded-lg border border-white/[0.08] bg-black/20 pl-9 pr-3 text-xs text-slate-200 outline-none focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"/></div></div><nav className="flex-1 overflow-y-auto px-3 py-5">{navGroups.map((group) => <div key={group.title} className="mb-4"><button type="button" onClick={() => toggleGroup(group.title)} className="mb-1 flex w-full items-center justify-between rounded-md px-3 py-1.5 text-left text-[10px] font-bold uppercase tracking-[0.12em] text-slate-500" aria-expanded={!isGroupCollapsed(group.title)}><span>{tOr(group.titleKey, group.title)} <span className="ml-1 text-slate-700">{group.items.length}</span></span><ChevronDown size={13} className={cn("transition-transform", isGroupCollapsed(group.title) && "-rotate-90")} /></button>{!isGroupCollapsed(group.title) && group.items.map((item) => { const Icon = item.icon; const active = pathname === item.href || pathname.startsWith(`${item.href}/`); return <button key={item.href} aria-current={active ? "page" : undefined} className={cn("flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm font-medium", active ? "bg-red-500/10 text-red-400" : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200")} onClick={() => router.push(item.href)} type="button"><Icon size={15}/><span className="truncate">{tOr(item.labelKey, item.label)}</span></button>; })}</div>)}</nav></aside></div> : null}
    </div>
  );
}
