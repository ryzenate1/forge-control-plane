"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import { Activity, Calendar, ChevronLeft, Database, Folder, HardDrive, Layers, LogOut, Menu, Network, Rocket, Settings, Terminal, User, Users, X } from "lucide-react";
import { type ApiServer, logout } from "@/lib/api";
import { cn } from "@/lib/utils";
import { hasServerPermission, type ServerAccess } from "./server-context";

export type ServerTab = "console" | "files" | "databases" | "schedules" | "users" | "backups" | "builds" | "network" | "startup" | "settings" | "activity" | "mounts" | "processes" | "deployments" | "git" | "database";

interface ServerNavProps { serverId: string; server: ApiServer; access: ServerAccess; activeTab?: ServerTab }

const tabs: Array<{ id: ServerTab; label: string; icon: typeof Terminal; permissions: string[] }> = [
  { id: "console", label: "Console", icon: Terminal, permissions: ["websocket.connect", "control.console"] },
  { id: "files", label: "Files", icon: Folder, permissions: ["file.read"] },
  { id: "databases", label: "Databases", icon: Database, permissions: ["database.read"] },
  { id: "schedules", label: "Schedules", icon: Calendar, permissions: ["schedule.read"] },
  { id: "users", label: "Users", icon: Users, permissions: ["user.read"] },
  { id: "backups", label: "Backups", icon: HardDrive, permissions: ["backup.read"] },
  { id: "builds", label: "Builds", icon: Rocket, permissions: [] },
  { id: "network", label: "Network", icon: Network, permissions: ["allocation.read"] },
  { id: "startup", label: "Startup", icon: Rocket, permissions: ["startup.read"] },
  { id: "settings", label: "Settings", icon: Settings, permissions: ["settings.rename", "settings.reinstall", "file.sftp"] },
  { id: "mounts", label: "Mounts", icon: Folder, permissions: ["mount.read"] },
  { id: "activity", label: "Activity", icon: Activity, permissions: ["activity.read"] },
  { id: "processes", label: "Processes", icon: Layers, permissions: ["control.start"] },
  { id: "deployments", label: "Deployments", icon: Rocket, permissions: [] },
];

function statusTone(server: ApiServer) {
  if (server.suspended) return "border-rose-500/40 bg-rose-500/15 text-rose-300";
  if (server.transferring) return "border-sky-500/40 bg-sky-500/15 text-sky-300";
  if (server.status === "running") return "border-emerald-500/40 bg-emerald-500/15 text-emerald-300";
  if (server.status === "installing") return "border-amber-500/40 bg-amber-500/15 text-amber-300";
  return "border-slate-500/40 bg-slate-700/50 text-slate-300";
}

export function ServerNav({ serverId, server, access, activeTab }: ServerNavProps) {
  const pathname = usePathname();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const visibleTabs = tabs.filter((tab) => hasServerPermission(access, tab.permissions));
  const state = server.suspended ? "Suspended" : server.transferring ? "Transferring" : server.status || "Unknown";
  const signOut = async () => { await logout(); router.push("/"); };

  const content = <>
    <div className="border-b border-white/[0.06] p-4">
      <Link href="/servers" className="mb-3 inline-flex items-center gap-1 text-xs text-slate-400 hover:text-white"><ChevronLeft size={13} /> My servers</Link>
      <h1 className="truncate text-base font-bold text-white" title={server.name}>{server.name}</h1>
      {server.description ? <p className="mt-1 line-clamp-2 text-xs text-slate-400">{server.description}</p> : null}
      <div className="mt-3 flex flex-wrap items-center gap-2"><span className={cn("rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider", statusTone(server))}>{state}</span>{server.allocation ? <span className="font-mono text-[10px] text-slate-400">{server.allocation}</span> : null}</div>
      <p className="mt-2 truncate text-[10px] text-slate-500">{server.node ? `Node: ${server.node}` : "Node unavailable"} · {server.id}</p>
    </div>
    <nav aria-label="Server navigation" className="flex-1 overflow-y-auto p-2">
      {visibleTabs.map((tab) => { const Icon = tab.icon; const href = tab.id === "console" ? `/server/${serverId}` : `/server/${serverId}/${tab.id}`; const selected = activeTab === tab.id || (tab.id === "console" && pathname === href); return <Link aria-current={selected ? "page" : undefined} className={cn("mb-0.5 flex items-center gap-2 rounded-lg px-3 py-2.5 text-sm font-medium transition", selected ? "bg-red-600/15 text-red-300" : "text-slate-400 hover:bg-white/[0.05] hover:text-white")} href={href} key={tab.id} onClick={() => setOpen(false)}><Icon size={16} />{tab.label}</Link>; })}
      {!access.isAdmin && !access.isOwner && access.permissions === null ? <p className="m-2 rounded border border-amber-500/30 bg-amber-500/10 p-2 text-xs text-amber-200">Permissions could not be verified. Navigation is restricted.</p> : null}
    </nav>
    <div className="space-y-1 border-t border-white/[0.06] p-2"><Link className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-400 hover:bg-white/[0.05] hover:text-white" href="/account"><User size={15} />Account</Link><button className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-400 hover:bg-white/[0.05] hover:text-white" onClick={signOut} type="button"><LogOut size={15} />Sign out</button></div>
  </>;

  return <>
    <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-white/[0.06] bg-[#0f1419]/95 px-4 backdrop-blur md:hidden"><div className="min-w-0"><p className="truncate text-sm font-bold text-white">{server.name}</p><p className="truncate font-mono text-[10px] text-slate-400">{server.allocation || state}</p></div><button aria-expanded={open} aria-label="Toggle server navigation" className="rounded-lg p-2 text-slate-300 hover:bg-white/5" onClick={() => setOpen((value) => !value)} type="button">{open ? <X /> : <Menu />}</button></header>
    {open ? <button aria-label="Close server navigation" className="fixed inset-0 z-30 bg-black/60 md:hidden" onClick={() => setOpen(false)} type="button" /> : null}
    <aside className={cn("fixed inset-y-0 left-0 z-40 flex w-72 flex-col border-r border-white/[0.06] bg-[#0f1419] transition-transform md:sticky md:top-0 md:h-screen md:w-64 md:translate-x-0", open ? "translate-x-0" : "-translate-x-full")}>{content}</aside>
  </>;
}
