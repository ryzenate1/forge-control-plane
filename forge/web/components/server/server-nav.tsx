"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import { Activity, Calendar, ChevronLeft, Database, Folder, HardDrive, Layers, LogOut, Menu, Network, Rocket, Settings, Terminal, User, Users, X } from "lucide-react";
import { type ApiServer, logout } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useT } from "@/components/TranslationProvider";
import { hasServerPermission, type ServerAccess } from "./server-context";

export type ServerTab = "console" | "files" | "databases" | "schedules" | "users" | "backups" | "builds" | "network" | "startup" | "settings" | "activity" | "mounts" | "processes" | "deployments" | "git" | "database" | "transfer";

interface ServerNavProps { serverId: string; server: ApiServer; access: ServerAccess; activeTab?: ServerTab }

const tabs: Array<{ id: ServerTab; labelKey: string; fallback: string; icon: typeof Terminal; permissions: string[] }> = [
  { id: "console", labelKey: "server.console", fallback: "Console", icon: Terminal, permissions: ["websocket.connect", "control.console"] },
  { id: "files", labelKey: "server.files", fallback: "Files", icon: Folder, permissions: ["file.read"] },
  { id: "databases", labelKey: "server.databases", fallback: "Databases", icon: Database, permissions: ["database.read"] },
  { id: "database", labelKey: "server.database", fallback: "Database", icon: Database, permissions: ["database.read"] },
  { id: "schedules", labelKey: "server.schedules", fallback: "Schedules", icon: Calendar, permissions: ["schedule.read"] },
  { id: "users", labelKey: "admin.users", fallback: "Users", icon: Users, permissions: ["user.read"] },
  { id: "backups", labelKey: "server.backups", fallback: "Backups", icon: HardDrive, permissions: ["backup.read"] },
  { id: "builds", labelKey: "server.builds", fallback: "Builds", icon: Rocket, permissions: [] },
  { id: "network", labelKey: "server.network", fallback: "Network", icon: Network, permissions: ["allocation.read"] },
  { id: "startup", labelKey: "server.startup", fallback: "Startup", icon: Rocket, permissions: ["startup.read"] },
  { id: "settings", labelKey: "server.settings", fallback: "Settings", icon: Settings, permissions: ["settings.rename", "settings.reinstall", "file.sftp"] },
  { id: "mounts", labelKey: "server.mounts", fallback: "Mounts", icon: Folder, permissions: ["mount.read"] },
  { id: "activity", labelKey: "admin.activity", fallback: "Activity", icon: Activity, permissions: ["activity.read"] },
  { id: "processes", labelKey: "server.processes", fallback: "Processes", icon: Layers, permissions: ["control.start"] },
  { id: "deployments", labelKey: "server.deployments", fallback: "Deployments", icon: Rocket, permissions: [] },
  { id: "transfer", labelKey: "server.transfer", fallback: "Transfer", icon: Network, permissions: ["settings.reinstall"] },
];

function statusTone(server: ApiServer) {
  if (server.suspended) return "border-rose-500/40 bg-rose-500/15 text-rose-300";
  if (server.transferring) return "border-sky-500/40 bg-sky-500/15 text-sky-300";
  if (server.status === "running") return "border-emerald-500/40 bg-emerald-500/15 text-emerald-300";
  if (server.status === "installing") return "border-amber-500/40 bg-amber-500/15 text-amber-300";
  return "border-slate-500/40 bg-slate-700/50 text-slate-300";
}

export function ServerNav({ serverId, server, access, activeTab }: ServerNavProps) {
  const t = useT();
  const pathname = usePathname();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const visibleTabs = tabs.filter((tab) => hasServerPermission(access, tab.permissions));
  const tr = (key: string, fallback: string) => { const value = t(key); return value === key ? fallback : value; };
  const state = server.suspended ? tr("server.nav.suspended", "Suspended") : server.transferring ? tr("server.nav.transferring", "Transferring") : server.status || tr("server.nav.unknown", "Unknown");
  const signOut = async () => { await logout(); router.push("/"); };

  const content = <>
    <div className="border-b border-white/[0.06] p-4">
      <Link href="/servers" className="mb-3 inline-flex items-center gap-1 text-xs text-slate-400 hover:text-white"><ChevronLeft size={13} /> {tr("server.myServers", "My servers")}</Link>
      <h1 className="truncate text-base font-bold text-white" title={server.name}>{server.name}</h1>
      {server.description ? <p className="mt-1 line-clamp-2 text-xs text-slate-400">{server.description}</p> : null}
      <div className="mt-3 flex flex-wrap items-center gap-2"><span className={cn("rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider", statusTone(server))}>{state}</span>{server.allocation ? <span className="font-mono text-[10px] text-slate-400">{server.allocation}</span> : null}</div>
      <p className="mt-2 truncate text-[10px] text-slate-500">{server.node ? `${tr("server.nav.node", "Node")}: ${server.node}` : tr("server.nav.nodeUnavailable", "Node unavailable")} · {server.id}</p>
    </div>
    <nav aria-label={tr("server.nav.ariaLabel", "Server navigation")} className="flex-1 overflow-y-auto p-2">
      {visibleTabs.map((tab) => { const Icon = tab.icon; const href = tab.id === "console" ? `/server/${serverId}` : `/server/${serverId}/${tab.id}`; const selected = activeTab === tab.id || (tab.id === "console" && pathname === href); const label = t(tab.labelKey); return <Link aria-current={selected ? "page" : undefined} className={cn("mb-0.5 flex items-center gap-2 rounded-lg px-3 py-2.5 text-sm font-medium transition", selected ? "bg-red-600/15 text-red-300" : "text-slate-400 hover:bg-white/[0.05] hover:text-white")} href={href} key={tab.id} onClick={() => setOpen(false)}><Icon size={16} />{label === tab.labelKey ? tab.fallback : label}</Link>; })}
      {!access.isAdmin && !access.isOwner && access.permissions === null ? <p className="m-2 rounded border border-amber-500/30 bg-amber-500/10 p-2 text-xs text-amber-200">{tr("server.nav.permissionsUnverified", "Permissions could not be verified. Navigation is restricted.")}</p> : null}
    </nav>
    <div className="space-y-1 border-t border-white/[0.06] p-2"><Link className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-400 hover:bg-white/[0.05] hover:text-white" href="/account"><User size={15} />{tr("server.nav.account", "Account")}</Link><button className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-400 hover:bg-white/[0.05] hover:text-white" onClick={signOut} type="button"><LogOut size={15} />{tr("server.nav.signOut", "Sign out")}</button></div>
  </>;

  return <>
    <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-white/[0.06] bg-[#0f1419]/95 px-4 backdrop-blur md:hidden"><div className="min-w-0"><p className="truncate text-sm font-bold text-white">{server.name}</p><p className="truncate font-mono text-[10px] text-slate-400">{server.allocation || state}</p></div><button aria-expanded={open} aria-label={tr("server.nav.toggleNav", "Toggle server navigation")} className="rounded-lg p-2 text-slate-300 hover:bg-white/5" onClick={() => setOpen((value) => !value)} type="button">{open ? <X /> : <Menu />}</button></header>
    {open ? <button aria-label={tr("server.nav.closeNav", "Close server navigation")} className="fixed inset-0 z-30 bg-black/60 md:hidden" onClick={() => setOpen(false)} type="button" /> : null}
    <aside className={cn("fixed inset-y-0 left-0 z-40 flex w-72 flex-col border-r border-white/[0.06] bg-[#0f1419] transition-transform md:sticky md:top-0 md:h-screen md:w-64 md:translate-x-0", open ? "translate-x-0" : "-translate-x-full")}>{content}</aside>
  </>;
}
