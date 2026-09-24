"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Activity,
  Archive,
  ArrowRight,
  Check,
  Copy,
  Cpu,
  Database,
  Folder,
  HardDrive,
  MemoryStick,
  Network,
  Play,
  Rocket,
  RotateCw,
  Server,
  Square,
  Terminal,
} from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type ApiAllocation,
  type ApiServer,
  fetchNode,
  fetchServerActivity,
  fetchServerAllocations,
  sendPowerSignal,
} from "@/lib/api";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import { EmptyState } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

function timeAgo(iso?: string | null): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const mins = Math.max(0, Math.round((Date.now() - t) / 60000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function statusTone(status?: string, suspended = false): string {
  if (suspended) return "border-rose-500/40 bg-rose-500/15 text-rose-300";
  if (status === "running") return "border-emerald-500/40 bg-emerald-500/15 text-emerald-300";
  if (status === "installing" || status === "starting") return "border-amber-500/40 bg-amber-500/15 text-amber-300";
  if (status === "crashed") return "border-red-500/40 bg-red-500/15 text-red-300";
  return "border-slate-500/40 bg-slate-700/50 text-slate-300";
}

function isPrimaryAllocation(allocation: ApiAllocation, server?: ApiServer) {
  return (
    allocation.isPrimary === true ||
    allocation.primary === true ||
    server?.primaryAllocationId === allocation.id ||
    server?.allocationId === allocation.id
  );
}

function CopyValue({ value, label }: { value?: string | null; label: string }) {
  const [copied, setCopied] = useState(false);
  if (!value) return <span className="text-slate-500">—</span>;
  return (
    <span className="inline-flex min-w-0 items-center gap-1.5">
      <span className="truncate font-mono text-xs text-slate-200" title={value}>{value}</span>
      <button
        type="button"
        aria-label={`Copy ${label}`}
        title={`Copy ${label}`}
        className="shrink-0 rounded p-1 text-slate-500 transition hover:bg-white/[0.06] hover:text-white"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
          } catch {
            const ta = document.createElement("textarea");
            ta.value = value;
            document.body.appendChild(ta);
            ta.select();
            document.execCommand("copy");
            ta.remove();
          }
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        }}
      >
        {copied ? <Check size={12} className="text-emerald-400" /> : <Copy size={12} />}
      </button>
    </span>
  );
}

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3 py-2">
      <span className="shrink-0 text-xs text-slate-500">{label}</span>
      <span className="min-w-0 text-right text-xs">{children}</span>
    </div>
  );
}

const QUICK_ACTIONS: Array<{ href: string; label: string; hint: string; icon: typeof Terminal; permissions: string[] }> = [
  { href: "", label: "Console", hint: "Live terminal", icon: Terminal, permissions: ["websocket.connect", "control.console"] },
  { href: "/files", label: "Files", hint: "Browse & edit", icon: Folder, permissions: ["file.read"] },
  { href: "/startup", label: "Startup", hint: "Image & variables", icon: Rocket, permissions: ["startup.read"] },
  { href: "/network", label: "Network", hint: "Allocations", icon: Network, permissions: ["allocation.read"] },
  { href: "/backups", label: "Backups", hint: "Snapshots", icon: Archive, permissions: ["backup.read"] },
  { href: "/activity", label: "Activity", hint: "Audit trail", icon: Activity, permissions: ["activity.read"] },
];

export function OverviewView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const refreshServer = context?.refreshServer ?? (() => {});
  const queryClient = useQueryClient();

  const nodeQuery = useQuery({
    queryKey: ["server-overview-node", server?.nodeId],
    queryFn: () => fetchNode(server?.nodeId ?? ""),
    enabled: Boolean(server?.nodeId),
    refetchInterval: 30_000,
    retry: 1,
  });
  const allocationsQuery = useQuery({
    queryKey: ["server-allocations", server?.id],
    queryFn: () => fetchServerAllocations(server?.id ?? ""),
    enabled: Boolean(server?.id),
    retry: 1,
  });
  const activityQuery = useQuery({
    queryKey: ["server-activity", server?.id],
    queryFn: () => fetchServerActivity(server?.id ?? ""),
    enabled: Boolean(server?.id),
    refetchInterval: 30_000,
    retry: 1,
  });

  const canStart = hasServerPermission(access, "control.start");
  const canStop = hasServerPermission(access, "control.stop");
  const canRestart = hasServerPermission(access, "control.restart");
  const power = useMutation({
    mutationFn: (signal: "start" | "stop" | "restart") => sendPowerSignal(server?.id ?? "", signal),
    onSuccess: () => {
      refreshServer();
      void queryClient.invalidateQueries({ queryKey: ["server-activity", server?.id] });
    },
  });

  const allocations = allocationsQuery.data ?? [];
  const primary = allocations.find((a) => isPrimaryAllocation(a, server)) ?? allocations[0];
  const connection = primary ? `${primary.ip}:${primary.port}` : (server?.allocation ?? null);
  const node = nodeQuery.data;
  const recentActivity = [...(activityQuery.data?.data ?? [])]
    .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())
    .slice(0, 5);
  const actions = QUICK_ACTIONS.filter((a) => hasServerPermission(access, a.permissions));

  const powerButtons: Array<{ signal: "start" | "stop" | "restart"; label: string; icon: typeof Play; allowed: boolean; primary?: boolean }> = [
    { signal: "start", label: "Start", icon: Play, allowed: canStart },
    { signal: "stop", label: "Stop", icon: Square, allowed: canStop },
    { signal: "restart", label: "Restart", icon: RotateCw, allowed: canRestart },
  ];

  return (
    <div className="space-y-4">
      <section className="ui-card">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-bold text-white" title={server?.name}>{server?.name ?? "Server"}</h1>
              <span className={cn("rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider", statusTone(server?.status, server?.suspended))}>
                {server?.suspended ? "Suspended" : server?.status ?? "Unknown"}
              </span>
            </div>
            {server?.description ? <p className="mt-1 text-sm text-slate-400">{server.description}</p> : null}
            <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 font-mono text-[11px] text-slate-500">
              {node ? <span title={node.fqdn ?? node.name}>Node: <span className="text-slate-300">{node.name}</span></span> : server?.node ? <span>Node: <span className="text-slate-300">{server.node}</span></span> : null}
              {connection ? <span>Connection: <span className="text-slate-300">{connection}</span></span> : null}
              {server?.dockerImage ? <span className="truncate" title={server.dockerImage}>Image: <span className="text-slate-300">{server.dockerImage}</span></span> : null}
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            {powerButtons.map(({ signal, label, icon: Icon, allowed }) => (
              <button
                key={signal}
                type="button"
                disabled={!allowed || !server?.id || power.isPending || (signal === "start" ? server?.status === "running" : server?.status !== "running")}
                onClick={() => power.mutate(signal)}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-lg px-3.5 py-2 text-xs font-bold transition disabled:cursor-not-allowed disabled:opacity-40",
                  signal === "start"
                    ? "bg-emerald-600 text-white hover:bg-emerald-500"
                    : "border border-white/10 bg-white/[0.04] text-slate-200 hover:bg-white/[0.08]",
                )}
              >
                <Icon size={13} />{power.isPending && power.variables === signal ? "…" : label}
              </button>
            ))}
          </div>
        </div>
        {power.error ? <p className="mt-3 rounded-lg border border-red-500/25 bg-red-950/20 p-2.5 text-xs text-red-200" role="alert">{power.error instanceof Error ? power.error.message : "Power action failed."}</p> : null}
      </section>

      <div className="grid gap-4 xl:grid-cols-3">
        <section className="ui-card">
          <h2 className="flex items-center gap-2 text-sm font-bold text-white"><Server size={15} className="text-slate-400" /> Status</h2>
          <div className="mt-2 divide-y divide-white/[0.05]">
            <InfoRow label="Desired state"><span className="font-semibold capitalize text-slate-200">{server?.desiredState ?? "—"}</span></InfoRow>
            <InfoRow label="Actual state"><span className="font-semibold capitalize text-slate-200">{server?.actualState ?? server?.status ?? "—"}</span></InfoRow>
            <InfoRow label="Node heartbeat">
              {nodeQuery.isLoading ? <span className="text-slate-500">…</span> : node ? (
                <span className={cn("font-semibold", node.heartbeatState === "healthy" ? "text-emerald-300" : "text-amber-300")}>
                  {node.heartbeatState ?? "unknown"} · {timeAgo(node.lastSeenAt ?? node.lastHeartbeatAt)}
                </span>
              ) : <span className="text-slate-500">Unavailable</span>}
            </InfoRow>
            <InfoRow label="Allocations"><span className="font-mono text-slate-200">{allocationsQuery.isLoading ? "…" : allocations.length}{typeof server?.allocationLimit === "number" && server.allocationLimit > 0 ? ` / ${server.allocationLimit}` : ""}</span></InfoRow>
            {server?.transferring ? <InfoRow label="Transfer"><span className="font-semibold text-sky-300">In progress</span></InfoRow> : null}
            {server?.installing ? <InfoRow label="Install"><span className="font-semibold text-amber-300">In progress</span></InfoRow> : null}
          </div>
        </section>

        <section className="ui-card">
          <h2 className="flex items-center gap-2 text-sm font-bold text-white"><HardDrive size={15} className="text-slate-400" /> Server information</h2>
          <div className="mt-2 divide-y divide-white/[0.05]">
            <InfoRow label="Internal ID"><CopyValue value={server?.id} label="server ID" /></InfoRow>
            {server?.uuid ? <InfoRow label="UUID"><CopyValue value={server.uuid} label="UUID" /></InfoRow> : null}
            <InfoRow label="Node">{node ? <Link className="text-sky-300 hover:text-sky-200" href={`/admin/nodes`}>{node.name}</Link> : <span className="text-slate-300">{server?.node ?? "—"}</span>}</InfoRow>
            <InfoRow label="Connection">{connection ? <CopyValue value={connection} label="connection address" /> : <span className="text-slate-500">No allocation</span>}</InfoRow>
            <InfoRow label="Owner"><span className="text-slate-200">{server?.ownerEmail ?? server?.owner ?? "—"}</span></InfoRow>
            <InfoRow label="Created"><span className="text-slate-200">{server?.createdAt ? new Date(server.createdAt).toLocaleString() : "—"}</span></InfoRow>
          </div>
        </section>

        <section className="ui-card">
          <h2 className="flex items-center gap-2 text-sm font-bold text-white"><Cpu size={15} className="text-slate-400" /> Limits</h2>
          <div className="mt-2 divide-y divide-white/[0.05]">
            <InfoRow label="Memory"><span className="font-mono text-slate-200"><MemoryStick size={11} className="mr-1 inline text-slate-500" />{typeof server?.memoryMb === "number" ? `${server.memoryMb.toLocaleString()} MiB` : "—"}</span></InfoRow>
            <InfoRow label="Disk"><span className="font-mono text-slate-200"><Database size={11} className="mr-1 inline text-slate-500" />{typeof server?.diskMb === "number" ? `${server.diskMb.toLocaleString()} MiB` : "—"}</span></InfoRow>
            <InfoRow label="CPU"><span className="font-mono text-slate-200">{typeof server?.cpuLimit === "number" ? `${server.cpuLimit}%` : typeof server?.cpuShares === "number" ? `${server.cpuShares} shares` : "—"}</span></InfoRow>
            <InfoRow label="Databases"><span className="font-mono text-slate-200">{server?.databaseLimit ?? "—"}</span></InfoRow>
            <InfoRow label="Backups"><span className="font-mono text-slate-200">{server?.backupLimit ?? "—"}</span></InfoRow>
            <InfoRow label="Allocations"><span className="font-mono text-slate-200">{server?.allocationLimit ?? "—"}</span></InfoRow>
          </div>
        </section>
      </div>

      <div className="grid gap-4 xl:grid-cols-3">
        <section className="ui-card xl:col-span-2">
          <h2 className="text-sm font-bold text-white">Quick actions</h2>
          <p className="mt-0.5 text-xs text-slate-500">Jump to the section you need. Unavailable sections are hidden by your permissions.</p>
          {actions.length === 0 ? (
            <p className="mt-3 text-xs text-slate-500">No quick actions available for your permission set.</p>
          ) : (
            <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
              {actions.map(({ href, label, hint, icon: Icon }) => (
                <Link
                  key={label}
                  href={`/server/${server?.id}${href}`}
                  className="group flex items-center gap-2.5 rounded-xl border border-white/[0.07] bg-white/[0.02] p-3 transition hover:border-white/20 hover:bg-white/[0.05]"
                >
                  <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-300 transition group-hover:text-white">
                    <Icon size={15} />
                  </span>
                  <span className="min-w-0">
                    <span className="block truncate text-xs font-bold text-slate-200">{label}</span>
                    <span className="block truncate text-[10px] text-slate-500">{hint}</span>
                  </span>
                  <ArrowRight size={13} className="ml-auto shrink-0 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-slate-300" />
                </Link>
              ))}
            </div>
          )}
        </section>

        <section className="ui-card">
          <div className="flex items-center justify-between">
            <h2 className="flex items-center gap-2 text-sm font-bold text-white"><Activity size={15} className="text-slate-400" /> Recent activity</h2>
            {hasServerPermission(access, "activity.read") ? (
              <Link href={`/server/${server?.id}/activity`} className="text-[11px] font-semibold text-slate-400 hover:text-white">View all</Link>
            ) : null}
          </div>
          {activityQuery.isLoading ? <div className="mt-3"><CardSkeleton /></div> : null}
          {activityQuery.isError ? <p className="mt-3 text-xs text-slate-500">Activity is unavailable with your permissions.</p> : null}
          {!activityQuery.isLoading && !activityQuery.isError && recentActivity.length === 0 ? (
            <div className="mt-3"><EmptyState title="No recent activity" description="Events will appear here as this server is managed." /></div>
          ) : null}
          {recentActivity.length > 0 ? (
            <ul className="mt-3 space-y-2.5">
              {recentActivity.map((event) => (
                <li key={event.id} className="flex items-start justify-between gap-2 text-xs">
                  <div className="min-w-0">
                    <p className="truncate font-semibold text-slate-200" title={event.action}>{event.action.replace(/^server[:.]/, "").replace(/[._:-]+/g, " ")}</p>
                    <p className="truncate text-[10px] text-slate-500">{event.actorEmail || "System"}</p>
                  </div>
                  <time className="shrink-0 font-mono text-[10px] text-slate-500" title={new Date(event.createdAt).toLocaleString()}>{timeAgo(event.createdAt)}</time>
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      </div>
    </div>
  );
}
