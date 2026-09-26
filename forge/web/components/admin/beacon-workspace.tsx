"use client";

/**
 * Beacon workspace — first-class full-page resource view for one compute
 * machine running the Forge agent (audit ARCH §7).
 *
 * Data sources (all existing, verified endpoints):
 *   GET /nodes/:id                    → identity + desired/actual state
 *   GET /nodes/:id/lifecycle          → live health vitals + score + placement eligibility (10s poll)
 *   GET /nodes/:id/system-information  → OS/arch/kernel/daemon/docker/uptime (10s poll)
 *   GET /nodes/:id/capacity           → allocated vs available cpu/memory/disk
 *   GET /nodes/:id/servers            → workloads hosted here
 *   GET /nodes/:id/allocations        → network ports bound here
 */

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import {
  Activity, Boxes, Cable, ChevronRight, Cpu, Gauge,
  HardDrive, MemoryStick, Network, Settings2, ShieldQuestion, Terminal, Wrench, Wifi,
} from "lucide-react";
import {
  AdminBackButton, AdminErrorState, AdminLoadingState, AdminPageHeader, AdminSection,
  AdminTabs, Btn, Card, CardHeader, EmptyState, Pill, cn,
} from "./admin-ui";
import { NodeDetailView } from "./AdminNodes";
import {
  fetchNode, fetchNodeAllocations, fetchNodeCapacity, fetchNodeLifecycle,
  fetchNodeServers, fetchNodeSystemInformation,
  type ApiAllocation, type ApiNode, type ApiServer,
} from "@/lib/api";

type Tab = "overview" | "workloads" | "network" | "capacity" | "hardware" | "firewall" | "terminal" | "maintenance";

const TABS = [
  { id: "overview", label: "Overview", icon: Activity },
  { id: "workloads", label: "Workloads", icon: Boxes },
  { id: "network", label: "Network", icon: Cable },
  { id: "capacity", label: "Capacity", icon: Gauge },
  { id: "hardware", label: "Hardware", icon: Cpu },
  { id: "firewall", label: "Firewall", icon: ShieldQuestion },
  { id: "terminal", label: "Terminal", icon: Terminal },
  { id: "maintenance", label: "Maintenance", icon: Wrench },
] as const;

function fmtMiB(mib?: number): string {
  if (mib === undefined || mib === null) return "—";
  if (mib >= 1024) return `${(mib / 1024).toFixed(1)} GiB`;
  return `${Math.round(mib)} MiB`;
}

function fmtUptime(seconds?: number): string {
  if (!seconds || seconds <= 0) return "—";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${m}m` : `${m}m`;
}

function heartbeatAge(iso?: string): string {
  if (!iso) return "—";
  const ms = Date.now() - new Date(iso).getTime();
  if (!Number.isFinite(ms)) return "—";
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s ago`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m ago`;
  return `${Math.round(m / 60)}h ago`;
}

function healthTone(value?: string): "green" | "yellow" | "red" | "neutral" {
  switch ((value ?? "").toLowerCase()) {
    case "healthy": case "ok": case "good": return "green";
    case "warning": case "degraded": case "elevated": return "yellow";
    case "critical": case "failing": case "unhealthy": return "red";
    default: return "neutral";
  }
}

function stateTone(node: ApiNode): "green" | "yellow" | "red" | "neutral" {
  const actual = node.actualState ?? "unknown";
  if (actual === "online") return "green";
  if (actual === "degraded") return "yellow";
  if (actual === "offline") return "red";
  return "neutral";
}

/** Allocated-vs-total capacity bar. */
function CapacityBar({ label, used, total, unit, icon: Icon }: {
  label: string; used: number; total: number; unit: string; icon?: typeof Cpu;
}) {
  const pct = total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0;
  const tone = pct >= 90 ? "bg-red-500" : pct >= 70 ? "bg-amber-400" : "bg-emerald-500";
  return (
    <div>
      <div className="mb-1 flex items-center justify-between text-xs">
        <span className="flex items-center gap-1.5 text-slate-400">{Icon ? <Icon size={12} /> : null}{label}</span>
        <span className="font-mono text-slate-300">{used.toLocaleString()} / {total.toLocaleString()} {unit}</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-white/[0.06]" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}>
        <div className={cn("h-full rounded-full transition-all motion-safe:transition-all", tone)} style={{ width: `${pct}%` }} />
      </div>
      <div className="mt-0.5 text-right text-[10px] text-slate-500">{pct}% allocated</div>
    </div>
  );
}

function VitalsCard({ label, value, tone, icon: Icon }: {
  label: string; value: string; tone: "green" | "yellow" | "red" | "neutral"; icon?: typeof Cpu;
}) {
  const toneRing = tone === "green" ? "text-emerald-400" : tone === "yellow" ? "text-amber-400" : tone === "red" ? "text-red-400" : "text-slate-400";
  return (
    <Card className="flex items-center gap-3 p-4">
      {Icon ? <Icon size={16} className={cn("shrink-0", toneRing)} /> : null}
      <div className="min-w-0">
        <div className="text-[10px] font-bold uppercase tracking-widest text-slate-500">{label}</div>
        <div className={cn("truncate text-sm font-semibold capitalize", toneRing)}>{value}</div>
      </div>
    </Card>
  );
}

export function BeaconWorkspace() {
  const params = useParams();
  const router = useRouter();
  const nodeId = String(params.id ?? "");
  const [tab, setTab] = useState<Tab>("overview");
  const [configureOpen, setConfigureOpen] = useState(false);

  const nodeQuery = useQuery({ queryKey: ["node", nodeId], queryFn: () => fetchNode(nodeId), enabled: Boolean(nodeId) });
  const lifecycleQuery = useQuery({
    queryKey: ["node-lifecycle", nodeId],
    queryFn: () => fetchNodeLifecycle(nodeId),
    refetchInterval: 10_000,
    enabled: Boolean(nodeId),
    retry: 1,
  });
  const sysQuery = useQuery({
    queryKey: ["node-sysinfo", nodeId],
    queryFn: () => fetchNodeSystemInformation(nodeId),
    refetchInterval: 10_000,
    enabled: Boolean(nodeId),
    retry: 1,
  });
  const capacityQuery = useQuery({
    queryKey: ["node-capacity", nodeId],
    queryFn: () => fetchNodeCapacity(nodeId),
    enabled: Boolean(nodeId),
  });
  const serversQuery = useQuery({
    queryKey: ["node-servers", nodeId],
    queryFn: () => fetchNodeServers(nodeId),
    enabled: Boolean(nodeId),
  });
  const allocQuery = useQuery({
    queryKey: ["node-allocations", nodeId],
    queryFn: () => fetchNodeAllocations(nodeId),
    enabled: Boolean(nodeId),
  });

  const node = nodeQuery.data;
  const lifecycle = lifecycleQuery.data;
  const servers = useMemo(() => (Array.isArray(serversQuery.data) ? serversQuery.data : []), [serversQuery.data]);
  const allocations = useMemo(() => (Array.isArray(allocQuery.data) ? allocQuery.data : []), [allocQuery.data]);

  if (nodeQuery.isLoading) {
    return (
      <div className="space-y-4">
        <AdminBackButton onClick={() => router.push("/admin/nodes")} label="Beacons" />
        <AdminLoadingState label="Loading Beacon…" />
      </div>
    );
  }
  if (nodeQuery.isError || !node) {
    return (
      <div className="space-y-4">
        <AdminBackButton onClick={() => router.push("/admin/nodes")} label="Beacons" />
        <AdminErrorState message={`Beacon could not be loaded: ${nodeQuery.error?.message ?? "not found"}`} retry={() => void nodeQuery.refetch()} />
      </div>
    );
  }

  const actualState = node.actualState ?? "unknown";
  const capacity = capacityQuery.data;
  const sys = sysQuery.data;
  const healthScore = lifecycle?.healthScore;

  return (
    <div className="space-y-5">
      <AdminPageHeader
        breadcrumb="Infrastructure / Beacons"
        title={node.name}
        description={[
          node.fqdn ?? node.baseUrl ?? "—",
          `Heartbeat ${heartbeatAge(node.lastHeartbeatAt)}`,
          sys ? `${sys.os} · ${sys.architecture}` : undefined,
          `${actualState}${node.maintenanceMode ? " · Maintenance" : ""}${node.draining ? " · Draining" : ""}`,
        ].filter(Boolean).join("  ·  ")}
        backAction={() => router.push("/admin/nodes")}
        backLabel="Beacons"
        action={
          <Btn tone="primary" onClick={() => setConfigureOpen(true)}>
            <Settings2 size={14} /> Configure
          </Btn>
        }
      />

      {/* Live vitals — answers "is this machine healthy?" before anything else */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <VitalsCard label="CPU" value={lifecycle?.health.cpu ?? (lifecycleQuery.isError ? "Offline" : "Probing…")} tone={healthTone(lifecycle?.health.cpu)} icon={Cpu} />
        <VitalsCard label="Memory" value={lifecycle?.health.memory ?? (lifecycleQuery.isError ? "Offline" : "Probing…")} tone={healthTone(lifecycle?.health.memory)} icon={MemoryStick} />
        <VitalsCard label="Disk" value={lifecycle?.health.disk ?? (lifecycleQuery.isError ? "Offline" : "Probing…")} tone={healthTone(lifecycle?.health.disk)} icon={HardDrive} />
        <VitalsCard label="Network" value={lifecycle?.health.network ?? (lifecycleQuery.isError ? "Offline" : "Probing…")} tone={healthTone(lifecycle?.health.network)} icon={Wifi} />
        <VitalsCard label="Runtime" value={lifecycle?.health.runtime ?? (lifecycleQuery.isError ? "Offline" : "Probing…")} tone={healthTone(lifecycle?.health.runtime)} icon={Boxes} />
        <VitalsCard
          label="Health score"
          value={healthScore ? `${healthScore.total}/100` : lifecycleQuery.isError ? "Offline" : "Probing…"}
          tone={healthScore ? (healthScore.total >= 80 ? "green" : healthScore.total >= 50 ? "yellow" : "red") : "neutral"}
          icon={ShieldQuestion}
        />
      </div>

      {lifecycle && !lifecycle.placementEligible ? (
        <div className="rounded-xl border border-amber-500/25 bg-amber-950/20 p-3 text-sm text-amber-200">
          Not eligible for new placements{lifecycle.placementBlockedReason ? ` — ${lifecycle.placementBlockedReason}` : ""}.
        </div>
      ) : null}

      <AdminTabs tabs={[...TABS]} active={tab} onChange={(id) => setTab(id as Tab)} label="Beacon sections" />

      {tab === "overview" ? (
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="lg:col-span-2">
            <CardHeader title="System identity" icon={Activity} />
            <dl className="grid grid-cols-2 gap-x-6 gap-y-1 p-4 text-sm sm:grid-cols-3">
              {[
                ["Operating system", sys?.os ?? "—"],
                ["Architecture", sys?.architecture ?? "—"],
                ["Kernel", sys?.kernelVersion ?? "—"],
                ["Daemon version", sys?.version ?? "—"],
                ["Docker", sys ? `${sys.dockerAvailable ? "Available" : "Unavailable"}${sys.dockerStatus ? ` · ${sys.dockerStatus}` : ""}` : "—"],
                ["Uptime", fmtUptime(sys?.daemonUptimeSeconds ?? sys?.uptime)],
                ["CPU threads", sys?.cpuThreads != null ? String(sys.cpuThreads) : node.cpuCores != null ? String(node.cpuCores) : "—"],
                ["Memory limit", fmtMiB(node.memoryMb)],
                ["Disk limit", fmtMiB(node.diskMb)],
                ["FQDN", node.fqdn ?? "—"],
                ["Scheme", (node.scheme ?? "https").toUpperCase()],
                ["Region", node.region ?? "—"],
              ].map(([label, value]) => (
                <div key={label} className="border-b border-white/[0.04] py-2">
                  <dt className="text-[10px] font-bold uppercase tracking-widest text-slate-500">{label}</dt>
                  <dd className="truncate font-mono text-[13px] text-slate-200" title={String(value)}>{value}</dd>
                </div>
              ))}
            </dl>
          </Card>
          <div className="space-y-4">
            <Card className="p-4">
              <div className="mb-3 flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-500">Capacity</span>
                <button type="button" className="text-xs text-slate-400 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400" onClick={() => setTab("capacity")}>Details <ChevronRight size={10} className="inline" /></button>
              </div>
              <div className="space-y-4">
                <CapacityBar label="Memory" used={capacity?.allocated_memory ?? 0} total={(capacity?.available_memory ?? 0) + (capacity?.allocated_memory ?? 0)} unit="MiB" icon={MemoryStick} />
                <CapacityBar label="Disk" used={capacity?.allocated_disk ?? 0} total={(capacity?.available_disk ?? 0) + (capacity?.allocated_disk ?? 0)} unit="MiB" icon={HardDrive} />
                <CapacityBar label="CPU" used={capacity?.allocated_cpu ?? 0} total={(capacity?.available_cpu ?? 0) + (capacity?.allocated_cpu ?? 0)} unit="%" icon={Cpu} />
              </div>
            </Card>
            <Card className="p-4">
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-slate-500">Workloads</span>
                <button type="button" className="text-xs text-slate-400 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400" onClick={() => setTab("workloads")}>View all <ChevronRight size={10} className="inline" /></button>
              </div>
              <div className="text-2xl font-bold text-slate-100">{capacity?.server_count ?? servers.length}</div>
              <div className="mt-1 text-xs text-slate-500">hosted on this Beacon</div>
            </Card>
          </div>
        </div>
      ) : null}

      {tab === "workloads" ? (
        <AdminSection title="Workloads on this Beacon" description="Game servers and applications scheduled here.">
          {serversQuery.isLoading ? (
            <AdminLoadingState label="Loading workloads…" />
          ) : serversQuery.isError ? (
            <AdminErrorState message={`Could not load workloads: ${serversQuery.error.message}`} retry={() => void serversQuery.refetch()} />
          ) : servers.length === 0 ? (
            <EmptyState icon={Boxes} title="No workloads here yet" message="Newly placed workloads will appear on this Beacon automatically." />
          ) : (
            <Card>
              <div className="overflow-x-auto">
                <table className="w-full text-sm text-slate-200">
                  <thead>
                    <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                      <th className="px-4 py-3">Name</th>
                      <th className="px-4 py-3">State</th>
                      <th className="px-4 py-3">Memory</th>
                      <th className="px-4 py-3"></th>
                    </tr>
                  </thead>
                  <tbody>
                    {servers.map((server: ApiServer) => (
                      <tr key={server.id} className="border-b border-white/[0.04] transition last:border-0 hover:bg-white/[0.02]">
                        <td className="px-4 py-3 font-medium">{server.name}</td>
                        <td className="px-4 py-3"><Pill tone={server.status === "running" ? "green" : server.status === "starting" || server.status === "stopping" ? "yellow" : server.status === "offline" || server.status === "stopped" ? "neutral" : "red"}>{server.status}</Pill></td>
                        <td className="px-4 py-3 font-mono text-xs text-slate-400">{fmtMiB(server.memoryMb)}</td>
                        <td className="px-4 py-3 text-right">
                          <button
                            type="button"
                            className="text-xs text-slate-400 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-400"
                            onClick={() => router.push(`/server/${server.id}/console`)}
                          >
                            Open terminal <ChevronRight size={10} className="inline" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          )}
        </AdminSection>
      ) : null}

      {tab === "network" ? (
        <AdminSection title="Network allocations" description="IP ports bound to this Beacon and available to its workloads.">
          {allocQuery.isLoading ? (
            <AdminLoadingState label="Loading allocations…" />
          ) : allocQuery.isError ? (
            <AdminErrorState message={`Could not load allocations: ${allocQuery.error.message}`} retry={() => void allocQuery.refetch()} />
          ) : allocations.length === 0 ? (
            <EmptyState icon={Network} title="No allocations yet" message="Create allocations from Infrastructure → Networking → IP Allocations; deployment assigns them to workloads automatically." />
          ) : (
            <Card>
              <div className="overflow-x-auto">
                <table className="w-full text-sm text-slate-200">
                  <thead>
                    <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                      <th className="px-4 py-3">Address</th>
                      <th className="px-4 py-3">Protocol</th>
                      <th className="px-4 py-3">Alias</th>
                      <th className="px-4 py-3">Assigned to</th>
                    </tr>
                  </thead>
                  <tbody>
                    {allocations.map((alloc: ApiAllocation) => (
                      <tr key={alloc.id} className="border-b border-white/[0.04] last:border-0">
                        <td className="px-4 py-3 font-mono text-xs">{alloc.ip}:{alloc.port}</td>
                        <td className="px-4 py-3 font-mono text-xs uppercase text-slate-400">{alloc.protocol ?? "tcp"}</td>
                        <td className="px-4 py-3 font-mono text-xs text-slate-400">{alloc.alias ?? "—"}</td>
                        <td className="px-4 py-3 text-slate-400">{alloc.server ?? "unassigned"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          )}
        </AdminSection>
      ) : null}

      {tab === "capacity" ? (
        <AdminSection title="Placement capacity" description="What this Beacon has committed versus what it can still accept. Used by the placement scheduler.">
          {capacityQuery.isLoading ? (
            <AdminLoadingState label="Loading capacity…" />
          ) : capacityQuery.isError || !capacity ? (
            <AdminErrorState message={capacityQuery.error ? `Could not load capacity: ${capacityQuery.error.message}` : "No capacity snapshot available."} retry={() => void capacityQuery.refetch()} />
          ) : (
            <div className="grid gap-4 lg:grid-cols-3">
              <Card className="p-5"><CapacityBar label="Memory" used={capacity.allocated_memory} total={capacity.allocated_memory + capacity.available_memory} unit="MiB" icon={MemoryStick} /></Card>
              <Card className="p-5"><CapacityBar label="Disk" used={capacity.allocated_disk} total={capacity.allocated_disk + capacity.available_disk} unit="MiB" icon={HardDrive} /></Card>
              <Card className="p-5"><CapacityBar label="CPU" used={capacity.allocated_cpu} total={capacity.allocated_cpu + capacity.available_cpu} unit="%" icon={Cpu} /></Card>
              <Card className="p-4 lg:col-span-3">
                <div className="flex items-center justify-between text-sm text-slate-400">
                  <span>Snapshot updated {new Date(capacity.updated_at).toLocaleString()}</span>
                  <Btn size="sm" tone="ghost" onClick={() => void capacityQuery.refetch()}><Gauge size={13} /> Refresh</Btn>
                </div>
              </Card>
            </div>
          )}
        </AdminSection>
      ) : null}

      {/* ─── Hardware Tab ──────────────────────────────────────────────────── */}
      {tab === "hardware" ? (
        <AdminSection title="Hardware & Virtualization">
          <div className="grid gap-4 lg:grid-cols-2">
            <Card className="p-5">
              <h3 className="mb-3 text-sm font-semibold text-white">CPU &amp; Kernel</h3>
              <dl className="space-y-2 text-xs">
                <div className="flex justify-between"><dt className="text-slate-400">Threads</dt><dd className="font-mono text-slate-200">{typeof sysQuery.data?.cpuThreads === "number" ? sysQuery.data.cpuThreads : "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">Architecture</dt><dd className="font-mono text-slate-200">{sysQuery.data?.architecture ?? "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">Kernel</dt><dd className="font-mono text-slate-200">{sysQuery.data?.kernelVersion ?? "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">OS</dt><dd className="font-mono text-slate-200">{sysQuery.data?.os ?? "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">Container engine</dt><dd className="font-mono text-slate-200">{sysQuery.data?.dockerStatus ?? "—"}</dd></div>
              </dl>
            </Card>
            <Card className="p-5">
              <h3 className="mb-3 text-sm font-semibold text-white">Memory, Storage &amp; Uptime</h3>
              <dl className="space-y-2 text-xs">
                <div className="flex justify-between"><dt className="text-slate-400">Total Memory</dt><dd className="font-mono text-slate-200">{sysQuery.data?.memoryMb ? fmtMiB(sysQuery.data.memoryMb) : "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">Total Disk</dt><dd className="font-mono text-slate-200">{capacity ? fmtMiB((capacity.available_disk ?? 0) + (capacity.allocated_disk ?? 0)) : "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">Host Uptime</dt><dd className="font-mono text-slate-200">{sysQuery.data?.uptime ? fmtUptime(sysQuery.data.uptime) : "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">Agent Uptime</dt><dd className="font-mono text-slate-200">{sysQuery.data?.daemonUptimeSeconds ? fmtUptime(sysQuery.data.daemonUptimeSeconds) : "—"}</dd></div>
                <div className="flex justify-between"><dt className="text-slate-400">Beacon Version</dt><dd className="font-mono text-slate-200">{sysQuery.data?.version ?? "—"}</dd></div>
              </dl>
            </Card>
            <Card className="p-5 lg:col-span-2">
              <h3 className="mb-3 text-sm font-semibold text-white">Virtualization Capabilities</h3>
              <p className="text-xs text-slate-400">Nested virt detection (vmx/svm), hugepages, IOMMU groups, and GPU passthrough require the Beacon host-capabilities endpoint. This section will populate from <code className="rounded bg-white/[0.06] px-1">GET /nodes/:id/host/capabilities</code> once available.</p>
            </Card>
          </div>
        </AdminSection>
      ) : null}

      {/* ─── Firewall Tab ───────────────────────────────────────────────────── */}
      {tab === "firewall" ? (
        <AdminSection title="Firewall & Port Forwards">
          <Card className="p-5">
            <p className="text-xs text-slate-400">Manage iptables rules and DNAT port-forwards for this node via the dedicated <Link href={`/admin/firewall?node=${nodeId}`} className="text-red-400 hover:underline">Firewall page</Link>. The firewall supports allow-rules (INPUT chain) and port-forwards (PREROUTING DNAT).</p>
          </Card>
        </AdminSection>
      ) : null}

      {/* ─── Terminal Tab ───────────────────────────────────────────────────── */}
      {tab === "terminal" ? (
        <AdminSection title="Host Terminal">
          <Card className="p-5">
            <p className="text-xs text-slate-400">Interactive shell on this node requires the PTY host terminal implementation in Beacon. Use <Link href="/admin/terminal" className="text-red-400 hover:underline">Admin Terminal</Link> when it becomes live. Container exec is available via <Link href="/admin/docker" className="text-red-400 hover:underline">Docker</Link>.</p>
          </Card>
        </AdminSection>
      ) : null}

      {/* ─── Maintenance Tab ───────────────────────────────────────────────── */}
      {tab === "maintenance" ? (
        <AdminSection title="Maintenance Operations">
          <div className="grid gap-4 sm:grid-cols-2">
            <Card className="p-5">
              <h3 className="mb-2 text-sm font-semibold text-white">Drain & Evacuate</h3>
              <p className="text-xs text-slate-400">Set lifecycle to draining/maintenance via Configure. Evacuation previews at <Link href="/admin/drain" className="text-red-400 hover:underline">/admin/drain</Link>.</p>
            </Card>
            <Card className="p-5">
              <h3 className="mb-2 text-sm font-semibold text-white">Beacon Upgrade</h3>
              <p className="text-xs text-slate-400">Self-upgrade endpoints exist on Beacon but the API service currently simulates them. Fleet upgrade orchestration is planned.</p>
            </Card>
            <Card className="p-5">
              <h3 className="mb-2 text-sm font-semibold text-white">Reconciliation</h3>
              <p className="text-xs text-slate-400">Desired-vs-actual drift detection at <Link href="/admin/reconciliation" className="text-red-400 hover:underline">/admin/reconciliation</Link>. Plan→confirm→execute pattern reusable for dangerous VM ops.</p>
            </Card>
            <Card className="p-5">
              <h3 className="mb-2 text-sm font-semibold text-white">Operations Timeline</h3>
              <p className="text-xs text-slate-400">Recent migrations, recovery plans, install workflows at <Link href="/admin/operations" className="text-red-400 hover:underline">/admin/operations</Link>.</p>
            </Card>
          </div>
        </AdminSection>
      ) : null}

      {/* Contextual configuration task reuses the existing full editor */}
      {configureOpen ? (
        <NodeDetailView nodeId={nodeId} onClose={() => setConfigureOpen(false)} />
      ) : null}
    </div>
  );
}
