"use client";

import { useEffect, useMemo, useState } from "react";
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
  Settings,
  Square,
  Terminal,
} from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type ApiAllocation,
  type ApiServer,
  type ApiStats,
  fetchNode,
  fetchServerActivity,
  fetchServerAllocations,
  fetchServerStartup,
  fetchServerStats,
  sendPowerSignal,
} from "@/lib/api";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import { EmptyState } from "@/components/ui/primitives";
import { chart } from "@/lib/design-tokens";
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

function formatBytes(bytes?: number | null): string {
  if (typeof bytes !== "number" || !Number.isFinite(bytes) || bytes < 0) return "—";
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GiB`;
  if (bytes >= 1024 ** 2) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${Math.round(bytes)} B`;
}

function formatUptime(totalSeconds?: number | null): string {
  if (typeof totalSeconds !== "number" || !Number.isFinite(totalSeconds) || totalSeconds < 0) return "—";
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const mins = Math.floor((totalSeconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function percentOf(used?: number | null, total?: number | null): number | null {
  if (typeof used !== "number" || typeof total !== "number" || !Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return null;
  return Math.min(100, Math.max(0, (used / total) * 100));
}

function Sparkline({ data, color }: { data: number[]; color: string }) {
  if (data.length < 2) return <GhostSparkline color={color} />;
  const min = Math.min(...data);
  const max = Math.max(...data);
  const range = max - min || 1;
  const width = 120;
  const height = 34;
  const points = data.map((val, idx) => {
    const x = (idx / (data.length - 1)) * width;
    const y = height - ((val - min) / range) * (height - 8) - 4;
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });
  const pathD = `M ${points.join(" L ")}`;
  const gid = `ov-${color.replace(/[^a-zA-Z0-9]/g, "")}`;
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="h-full w-full" aria-hidden="true">
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.35} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={`${pathD} L ${width},${height} L 0,${height} Z`} fill={`url(#${gid})`} />
      <path d={pathD} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function GhostSparkline({ color }: { color: string }) {
  const gid = `ov-ghost-${color.replace(/[^a-zA-Z0-9]/g, "")}`;
  return (
    <svg viewBox="0 0 120 34" className="h-full w-full" aria-hidden="true">
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.22} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d="M 0,26 L 20,22 L 40,23 L 60,16 L 80,14 L 100,8 L 120,6 L 120,34 L 0,34 Z" fill={`url(#${gid})`} />
      <path d="M 0,26 L 20,22 L 40,23 L 60,16 L 80,14 L 100,8 L 120,6" fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" opacity={0.5} />
    </svg>
  );
}

function shortImage(image?: string): string {
  if (!image) return "—";
  const noTag = image.split("@")[0];
  const parts = noTag.split("/");
  return parts[parts.length - 1] || image;
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
            try {
              const ta = document.createElement("textarea");
              ta.value = value;
              document.body.appendChild(ta);
              ta.select();
              if (typeof document.execCommand === "function") document.execCommand("copy");
              ta.remove();
            } catch {
              /* clipboard unavailable — leave value visible for manual copy */
            }
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
  { href: "/console", label: "Console", hint: "Live terminal", icon: Terminal, permissions: ["websocket.connect", "control.console"] },
  { href: "/files", label: "Files", hint: "Browse & edit", icon: Folder, permissions: ["file.read"] },
  { href: "/startup", label: "Startup", hint: "Image & variables", icon: Rocket, permissions: ["startup.read"] },
  { href: "/network", label: "Network", hint: "Allocations", icon: Network, permissions: ["allocation.read"] },
  { href: "/backups", label: "Backups", hint: "Snapshots", icon: Archive, permissions: ["backup.read"] },
  { href: "/activity", label: "Activity", hint: "Audit trail", icon: Activity, permissions: ["activity.read"] },
  { href: "/settings", label: "Settings", hint: "Rename & reinstall", icon: Settings, permissions: ["settings.rename", "settings.reinstall", "file.sftp"] },
];

type StatSample = { t: number; cpu: number; mem: number; disk: number; net: number };

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
  const canStats = hasServerPermission(access, "websocket.connect");
  const statsQuery = useQuery({
    queryKey: ["server-stats", server?.id],
    queryFn: () => fetchServerStats(server?.id ?? ""),
    enabled: Boolean(server?.id) && canStats,
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
    retry: 1,
  });
  const startupQuery = useQuery({
    queryKey: ["server-startup", server?.id],
    queryFn: () => fetchServerStartup(server?.id ?? ""),
    enabled: Boolean(server?.id) && hasServerPermission(access, "startup.read"),
    staleTime: 60_000,
    retry: 1,
  });

  const [samples, setSamples] = useState<StatSample[]>([]);
  useEffect(() => { setSamples([]); }, [server?.id]);
  useEffect(() => {
    const s: ApiStats | undefined = statsQuery.data;
    if (!s) return;
    setSamples((prev) => [...prev.slice(-19), {
      t: Date.now(),
      cpu: s.cpuPercent,
      mem: percentOf(s.memoryBytes, s.memoryLimit) ?? 0,
      disk: percentOf(s.diskBytes, s.diskLimit) ?? 0,
      net: s.networkRxBytes + s.networkTxBytes,
    }]);
  }, [statsQuery.data]);

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
  const sortedActivity = useMemo(() => [...(activityQuery.data?.data ?? [])]
    .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()),
  [activityQuery.data]);
  const recentActivity = sortedActivity.slice(0, 5);
  const lastStartEvent = sortedActivity.find((e) => /start/i.test(e.action) && !/restart|install/i.test(e.action));
  const lastActivityAt = sortedActivity[0]?.createdAt;
  const actions = QUICK_ACTIONS.filter((a) => hasServerPermission(access, a.permissions));

  const stats: ApiStats | undefined = statsQuery.data;
  const statsLive = statsQuery.isSuccess && Boolean(stats);
  const memPct = stats ? percentOf(stats.memoryBytes, stats.memoryLimit) : null;
  const diskPct = stats ? percentOf(stats.diskBytes, stats.diskLimit) : null;

  const startupVars = startupQuery.data?.variables ?? [];
  const varValue = (matcher: RegExp): string | null => {
    const v = startupVars.find((item) => matcher.test(item.name ?? "") || matcher.test(item.envVariable ?? item.env_variable ?? ""));
    if (!v) return null;
    const value = (v.serverValue ?? v.server_value ?? v.defaultValue ?? "").trim();
    return value || null;
  };
  const versionValue = varValue(/version/i);
  const jarValue = varValue(/jar/i);

  const imageShort = shortImage(server?.dockerImage);
  const tags = [server?.template, imageShort === "—" ? null : imageShort].filter((t): t is string => Boolean(t)).slice(0, 3);

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
            <p className="text-[10px] font-bold uppercase tracking-widest text-rose-400/80">{server?.template ? "Game server" : "Server"}</p>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-bold text-white" title={server?.name}>{server?.name ?? "Server"}</h1>
              <span className={cn("rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider", statusTone(server?.status, server?.suspended))}>
                {server?.suspended ? "Suspended" : server?.status ?? "Unknown"}
              </span>
            </div>
            {server?.description ? <p className="mt-1 text-sm text-slate-400">{server.description}</p> : null}
            {tags.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1.5">
                {tags.map((tag) => (
                  <span key={tag} className="rounded-md border border-white/[0.08] bg-white/[0.03] px-2 py-0.5 font-mono text-[10px] text-slate-400">{tag}</span>
                ))}
              </div>
            )}
            <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[11px] text-slate-500">
              <span title={node?.fqdn ?? node?.name}>Node: <span className="font-mono text-slate-300">{node?.name ?? server?.node ?? "—"}</span></span>
              <span title="Runtime engine this workload is dispatched to">Engine: <span className="font-mono text-slate-300">{server?.runtimeProvider || "unknown"}</span></span>
              <span title={statsLive && stats ? `Uptime ${stats.uptime}s reported by the beacon` : "Uptime is reported by the beacon while the server runs"}>Uptime: <span className="font-mono text-slate-300">{statsLive && stats ? formatUptime(stats.uptime) : "—"}</span></span>
              <span>Version: <span className="font-mono text-slate-300">{versionValue ?? imageShort}</span></span>
              <span>Last activity: <span className="font-mono text-slate-300">{lastActivityAt ? timeAgo(lastActivityAt) : "—"}</span></span>
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

      <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
        {[
          { key: "cpu", title: "CPU", icon: Cpu, color: chart.sky, iconClass: "text-sky-400",
            value: statsLive && stats ? `${stats.cpuPercent.toFixed(1)}%` : null,
            sub: statsLive ? "live" : statsQuery.isLoading ? "…" : "Offline",
            data: samples.map((s) => s.cpu), bar: statsLive && stats ? Math.min(100, stats.cpuPercent) : null },
          { key: "memory", title: "Memory", icon: MemoryStick, color: chart.violet, iconClass: "text-purple-400",
            value: memPct != null ? `${memPct.toFixed(1)}%` : null,
            sub: stats && statsLive ? `${formatBytes(stats.memoryBytes)} / ${formatBytes(stats.memoryLimit)}` : statsQuery.isLoading ? "…" : "Offline",
            data: samples.map((s) => s.mem), bar: statsLive ? memPct : null },
          { key: "disk", title: "Disk", icon: Database, color: chart.lightOrange, iconClass: "text-orange-400",
            value: diskPct != null ? `${diskPct.toFixed(1)}%` : null,
            sub: stats && statsLive ? `${formatBytes(stats.diskBytes)} / ${formatBytes(stats.diskLimit)}` : statsQuery.isLoading ? "…" : "Offline",
            data: samples.map((s) => s.disk), bar: statsLive ? diskPct : null },
          { key: "network", title: "Network", icon: Network, color: chart.lightCyan, iconClass: "text-cyan-400",
            value: statsLive && stats ? formatBytes(stats.networkRxBytes + stats.networkTxBytes) : null,
            sub: stats && statsLive ? `RX ${formatBytes(stats.networkRxBytes)} · TX ${formatBytes(stats.networkTxBytes)}` : statsQuery.isLoading ? "…" : "Offline",
            data: samples.map((s) => s.net), bar: null },
        ].map((card) => (
          <div key={card.key} className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm">
            <div className="flex items-center gap-2 text-xs font-semibold text-slate-200">
              <card.icon size={15} className={card.iconClass} />
              <span>{card.title}</span>
              {statsLive ? (
                <span className="ml-auto flex items-center gap-1 font-mono text-[10px] font-normal text-emerald-400">
                  <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-400" />live
                </span>
              ) : null}
            </div>
            <div className="mt-2 flex items-end justify-between gap-2">
              <div>
                <p className="font-mono text-xl font-bold text-slate-100">{card.value ?? "— —"}</p>
                <p className="mt-0.5 max-w-36 truncate text-[11px] text-slate-500" title={card.sub}>{card.sub}</p>
              </div>
              <div className="h-9 w-24 shrink-0 overflow-hidden sm:w-28">
                <Sparkline data={card.data} color={card.color} />
              </div>
            </div>
            {card.bar != null ? (
              <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/10">
                <div className="h-full rounded-full transition-all" style={{ width: `${card.bar}%`, backgroundColor: card.color }} />
              </div>
            ) : null}
          </div>
        ))}
      </div>
      {!statsLive && !statsQuery.isLoading && (
        <p className="text-[11px] text-slate-600">Live resource usage is reported by the beacon while the server runs — charts stay empty while it is offline. No per-server history is retained; sparklines cover this session only.</p>
      )}

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
            <InfoRow label="Uptime"><span className="font-mono text-slate-200" title={statsLive && stats ? `Beacon-reported uptime: ${stats.uptime}s` : "Uptime is reported by the beacon while the server runs"}>{statsLive && stats ? formatUptime(stats.uptime) : "—"}</span></InfoRow>
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
            {versionValue ? <InfoRow label="Version"><span className="font-mono text-slate-200">{versionValue}</span></InfoRow> : null}
            {jarValue ? <InfoRow label="Server JAR"><span className="font-mono text-slate-200">{jarValue}</span></InfoRow> : null}
            <InfoRow label="Owner"><span className="text-slate-200">{server?.ownerEmail ?? server?.owner ?? "—"}</span></InfoRow>
            <InfoRow label="Created"><span className="text-slate-200">{server?.createdAt ? new Date(server.createdAt).toLocaleString() : "—"}</span></InfoRow>
            <InfoRow label="Last started"><span className="text-slate-200" title={lastStartEvent ? new Date(lastStartEvent.createdAt).toLocaleString() : "Derived from the server audit trail"}>{lastStartEvent ? timeAgo(lastStartEvent.createdAt) : "—"}</span></InfoRow>
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
