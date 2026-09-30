"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { HardDrive, Info, ListChecks, MemoryStick, Network, Server } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  AdminSelect,
  AdminTBody,
  AdminTable,
  AdminTabs,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Pill,
  SectionHeader,
  SubsystemHealthMeter,
} from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState, useNodesQuery } from "@/lib/admin/telemetry";
import { ApiError } from "@/lib/api/http";
import { statusTone } from "@/lib/api/status";
import {
  fetchHostDisk,
  fetchHostInfo,
  fetchHostMemory,
  fetchHostNetwork,
  fetchHostProcesses,
  type DiskPartition,
  type NetworkInterface,
  type ProcessEntry,
} from "@/lib/api/host";
import { formatDate } from "@/lib/utils";

type Tab = "info" | "disk" | "memory" | "network" | "processes";

const TABS: { id: Tab; label: string; icon: typeof Info }[] = [
  { id: "info", label: "Info", icon: Info },
  { id: "disk", label: "Disk", icon: HardDrive },
  { id: "memory", label: "Memory", icon: MemoryStick },
  { id: "network", label: "Network", icon: Network },
  { id: "processes", label: "Processes", icon: ListChecks },
];

/** Cadences the page actually polls at, named once so the copy and the badge agree. */
const HOST_REFRESH_MS = 30_000;
const PROCESS_REFRESH_MS = 10_000;

/** Shown whenever a field the node never sent would otherwise read as a measurement. */
const NOT_REPORTED = "Not reported";

function num(value: number | null | undefined): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function orUnknown(value: string | number | null | undefined): string {
  if (typeof value === "number") return Number.isFinite(value) ? String(value) : NOT_REPORTED;
  const text = (value ?? "").trim();
  return text === "" ? NOT_REPORTED : text;
}

function fmtMB(mb: number | null): string {
  if (mb === null) return NOT_REPORTED;
  if (mb >= 1024 * 1024) return `${(mb / 1024 / 1024).toFixed(1)} TB`;
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${Math.round(mb)} MB`;
}

/**
 * Utilisation colour comes from the shared status vocabulary, never from a class
 * picked in this file: >80% is a real warning, >60% deserves attention, and a
 * partition the node never measured is `unknown` (dashed), not a green bar.
 */
function usageTone(pct: number | null) {
  if (pct === null) return "unknown" as const;
  if (pct > 80) return "danger" as const;
  if (pct > 60) return "warn" as const;
  return "ok" as const;
}

function useHostQuery<T>(queryKey: string[], fetchFn: (init?: RequestInit) => Promise<T>, refetchInterval: number) {
  return useQuery({
    queryKey,
    queryFn: ({ signal }) => {
      const timeout = AbortSignal.timeout(15000);
      const combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
      return fetchFn({ signal: combined });
    },
    retry: 1,
    refetchInterval,
    staleTime: 10_000,
  });
}

/**
 * Every failure keeps its own cause. The previous version answered everything
 * outside 408/401/403 with "An unexpected error occurred", which hid the two
 * commonest real reasons — a node that is not registered and a Beacon that is
 * not reachable — behind an unknown error.
 */
function hostErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 504) return "Connection timeout — the Beacon daemon did not respond in time";
    if (error.status === 502) return "Node offline — the Beacon daemon is unreachable";
    if (error.status === 404) return "No node found — this node is no longer registered";
    if (error.status === 503) return "Service unavailable — the daemon proxy is not ready";
    if (error.status === 401 || error.status === 403) {
      return error.message || "Not allowed — your session cannot inspect this host";
    }
    return error.message || `Host inspection failed (HTTP ${error.status})`;
  }
  return error instanceof Error && error.message ? error.message : "Host inspection failed";
}

/** Freshness for one tab's own query — the only liveness claim this page makes. */
function TabFreshness({ query, intervalMs }: { query: Parameters<typeof sourceState>[0]; intervalMs: number }) {
  return (
    <div className="mt-4 flex justify-end">
      <FreshnessBadge state={sourceState(query, intervalMs)} />
    </div>
  );
}

function InfoTab({ nodeId }: { nodeId: string }) {
  const query = useHostQuery(["host-info", nodeId], (init) => fetchHostInfo(nodeId, init), HOST_REFRESH_MS);

  if (query.isPending) return <AdminLoadingState label="Loading host information…" />;
  if (query.isError) return <AdminErrorState message={hostErrorMessage(query.error)} retry={() => void query.refetch()} />;
  const data = query.data;
  if (!data) return <EmptyState icon={Info} title="Host information unavailable" message="The node returned no system information for this read." />;

  return (
    <div>
      <div className="grid gap-3 md:grid-cols-2">
        <StatRow label="Hostname" value={orUnknown(data.hostname)} />
        <StatRow label="CPU model" value={orUnknown(data.cpuModel)} />
        <StatRow label="Operating system" value={orUnknown(data.os)} />
        <StatRow label="CPU cores" value={num(data.cpuCores) === null ? NOT_REPORTED : String(num(data.cpuCores))} />
        <StatRow label="Kernel" value={orUnknown(data.kernel)} />
        <StatRow label="Architecture" value={orUnknown(data.arch)} />
        <StatRow label="Host uptime" value={fmtUptime(data.uptimeSeconds)} />
        <StatRow label="Beacon uptime" value={fmtUptime(data.daemonUptimeSeconds)} />
        <StatRow label="Node clock" value={formatDate(data.time, NOT_REPORTED)} />
      </div>
      <TabFreshness intervalMs={HOST_REFRESH_MS} query={query} />
    </div>
  );
}

function DiskTab({ nodeId }: { nodeId: string }) {
  const query = useHostQuery(["host-disk", nodeId], (init) => fetchHostDisk(nodeId, init), HOST_REFRESH_MS);

  if (query.isPending) return <AdminLoadingState label="Loading disk usage…" />;
  if (query.isError) return <AdminErrorState message={hostErrorMessage(query.error)} retry={() => void query.refetch()} />;
  const data = query.data;
  if (!data || data.length === 0) {
    return <EmptyState icon={HardDrive} title="No disk data reported" message="The node answered this read without any mounted partitions." />;
  }

  return (
    <div>
      <div className="space-y-4">
        {data.map((part: DiskPartition, index) => {
          const pct = num(part.usedPercent);
          const used = fmtMB(num(part.usedMb));
          const total = fmtMB(num(part.totalMb));
          return (
            <div key={`${part.mountPoint}-${index}`}>
              <SubsystemHealthMeter
                icon={HardDrive}
                label={orUnknown(part.mountPoint)}
                percentage={pct}
                tone={usageTone(pct)}
                valueText={pct === null ? NOT_REPORTED : `${pct.toFixed(1)}% used`}
              />
              <p className="mt-1 font-mono text-meta text-text-muted">
                {orUnknown(part.device)} · {orUnknown(part.fsType)} · {used} / {total}
                {pct === null ? " · usage not measured" : ""}
              </p>
            </div>
          );
        })}
      </div>
      <TabFreshness intervalMs={HOST_REFRESH_MS} query={query} />
    </div>
  );
}

function MemoryTab({ nodeId }: { nodeId: string }) {
  const query = useHostQuery(["host-memory", nodeId], (init) => fetchHostMemory(nodeId, init), HOST_REFRESH_MS);

  if (query.isPending) return <AdminLoadingState label="Loading memory usage…" />;
  if (query.isError) return <AdminErrorState message={hostErrorMessage(query.error)} retry={() => void query.refetch()} />;
  const data = query.data;
  if (!data) return <EmptyState icon={MemoryStick} title="No memory data reported" message="The node answered this read without memory usage." />;

  const pct = num(data.usedPercent);
  const swapTotal = num(data.swapTotalMb);
  const swapUsed = num(data.swapUsedMb);
  const swapPct = swapTotal && swapTotal > 0 && swapUsed !== null ? (swapUsed / swapTotal) * 100 : null;

  return (
    <div className="space-y-5">
      <div>
        <SubsystemHealthMeter
          icon={MemoryStick}
          label="Memory"
          percentage={pct}
          tone={usageTone(pct)}
          valueText={pct === null ? NOT_REPORTED : `${pct.toFixed(1)}% used`}
        />
        <p className="mt-1 font-mono text-meta text-text-muted">
          {fmtMB(num(data.usedMb))} used · {fmtMB(num(data.freeMb))} free · {fmtMB(num(data.totalMb))} total
          {pct === null ? " · usage not measured" : ""}
        </p>
      </div>
      <div>
        <SubsystemHealthMeter
          icon={MemoryStick}
          label="Swap"
          percentage={swapPct !== null && Number.isFinite(swapPct) ? Math.min(100, swapPct) : null}
          tone={usageTone(swapPct !== null && Number.isFinite(swapPct) ? swapPct : null)}
          valueText={fmtMB(num(data.swapUsedMb))}
        />
        <p className="mt-1 font-mono text-meta text-text-muted">
          {fmtMB(num(data.swapUsedMb))} used · {fmtMB(num(data.swapFreeMb))} free · {fmtMB(num(data.swapTotalMb))} total
        </p>
      </div>
      <TabFreshness intervalMs={HOST_REFRESH_MS} query={query} />
    </div>
  );
}

function NetworkTab({ nodeId }: { nodeId: string }) {
  const query = useHostQuery(["host-network", nodeId], (init) => fetchHostNetwork(nodeId, init), HOST_REFRESH_MS);

  if (query.isPending) return <AdminLoadingState label="Loading network interfaces…" />;
  if (query.isError) return <AdminErrorState message={hostErrorMessage(query.error)} retry={() => void query.refetch()} />;
  const data = query.data;
  if (!data || data.length === 0) {
    return <EmptyState icon={Network} title="No interfaces reported" message="The node answered this read without network interfaces." />;
  }

  return (
    <div>
      <div className="grid gap-3 md:grid-cols-2">
        {data.map((iface: NetworkInterface) => (
          <Card key={iface.name}>
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold text-text">{orUnknown(iface.name)}</p>
                <p className="mt-1 font-mono text-meta text-text-subtle">{orUnknown(iface.ips)}</p>
                <p className="font-mono text-meta text-text-muted">MAC {orUnknown(iface.mac)}</p>
                <p className="font-mono text-meta text-text-muted">
                  {num(iface.speedMbps) === null ? `Link speed ${NOT_REPORTED}` : `${iface.speedMbps} Mbps link speed`}
                </p>
              </div>
              {/* `status` is free text off the wire: up/down/unknown/lowerlayerdown are
                  four different readings and must not collapse into one colour. */}
              <Pill tone={statusTone(iface.status, "discovery")}>{orUnknown(iface.status)}</Pill>
            </div>
          </Card>
        ))}
      </div>
      <TabFreshness intervalMs={HOST_REFRESH_MS} query={query} />
    </div>
  );
}

const PROCESS_ROWS_SHOWN = 100;

function ProcessesTab({ nodeId }: { nodeId: string }) {
  const [sortBy, setSortBy] = useState<"cpu" | "mem">("cpu");
  const query = useHostQuery(["host-processes", nodeId], (init) => fetchHostProcesses(nodeId, init), PROCESS_REFRESH_MS);

  if (query.isPending) return <AdminLoadingState label="Loading processes…" />;
  if (query.isError) return <AdminErrorState message={hostErrorMessage(query.error)} retry={() => void query.refetch()} />;
  const data = query.data;
  if (!data || data.length === 0) {
    return <EmptyState icon={ListChecks} title="No processes reported" message="The node answered this read without a process list." />;
  }

  const sorted = [...data].sort((a, b) => {
    if (sortBy === "cpu") return (b.cpuPercent ?? 0) - (a.cpuPercent ?? 0);
    return (b.memoryPercent ?? 0) - (a.memoryPercent ?? 0);
  });
  const shown = sorted.slice(0, PROCESS_ROWS_SHOWN);

  return (
    <div>
      <div className="mb-3 flex flex-wrap gap-2">
        <Btn onClick={() => setSortBy("cpu")} size="sm" tone={sortBy === "cpu" ? "primary" : "ghost"}>
          Sort by CPU
        </Btn>
        <Btn onClick={() => setSortBy("mem")} size="sm" tone={sortBy === "mem" ? "primary" : "ghost"}>
          Sort by memory
        </Btn>
      </div>
      <AdminTable label={`Processes on node ${nodeId}`}>
        <AdminTHead>
          <AdminTh>PID</AdminTh>
          <AdminTh>Name</AdminTh>
          <AdminTh>CPU %</AdminTh>
          <AdminTh>Memory %</AdminTh>
          <AdminTh>State</AdminTh>
        </AdminTHead>
        <AdminTBody>
          {shown.map((proc: ProcessEntry) => (
            <AdminTr key={proc.pid}>
              <AdminTd className="font-mono">{orUnknown(proc.pid)}</AdminTd>
              <AdminTd className="font-medium text-text">{orUnknown(proc.name)}</AdminTd>
              <AdminTd className="font-mono">{num(proc.cpuPercent) === null ? NOT_REPORTED : proc.cpuPercent.toFixed(1)}</AdminTd>
              <AdminTd className="font-mono">{num(proc.memoryPercent) === null ? NOT_REPORTED : proc.memoryPercent.toFixed(1)}</AdminTd>
              <AdminTd>{orUnknown(proc.state)}</AdminTd>
            </AdminTr>
          ))}
        </AdminTBody>
      </AdminTable>
      {sorted.length > shown.length ? (
        <p className="mt-2 text-meta text-text-muted">
          Showing the {shown.length} busiest of {sorted.length} reported processes, sorted by {sortBy === "cpu" ? "CPU" : "memory"}.
        </p>
      ) : null}
      <TabFreshness intervalMs={PROCESS_REFRESH_MS} query={query} />
    </div>
  );
}

function StatRow({ label, value }: { label: string; value: string }) {
  const unreported = value === NOT_REPORTED;
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-line bg-overlay-subtle px-3 py-2.5">
      <span className="text-meta font-semibold uppercase tracking-wide text-text-muted">{label}</span>
      <span className={unreported ? "font-mono text-xs text-unknown" : "font-mono text-xs text-text"}>{value}</span>
    </div>
  );
}

/**
 * A node reports an unreadable uptime as a negative value or omits it, which is
 * not the same as a machine that has been up for zero seconds. "0m" or "-1m"
 * would be a made-up reading.
 */
function fmtUptime(seconds: number | undefined) {
  const value = num(seconds);
  if (value === null || value < 0) return NOT_REPORTED;
  const d = Math.floor(value / 86400);
  const h = Math.floor((value % 86400) / 3600);
  const m = Math.floor((value % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export default function AdminHost() {
  const [activeTab, setActiveTab] = useState<Tab>("info");
  // Nothing is fetched until the operator names a node: `/host/*` 400s without
  // `nodeId` because reading an arbitrary machine is not an acceptable default,
  // so the client must not pick one either.
  const [nodeId, setNodeId] = useState("");

  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const nodeOptions = useMemo(
    () => nodes.map((node) => ({ value: node.id, label: node.status === "active" ? node.name : `${node.name} · ${node.status}` })),
    [nodes],
  );

  const activeLabel = TABS.find((t) => t.id === activeTab)?.label ?? "";
  const ActiveIcon = TABS.find((t) => t.id === activeTab)?.icon ?? Info;

  return (
    <AdminPageLayout>
      <SectionHeader
        info={{
          description:
            "Per-node system readings taken from one Beacon daemon. The node has to be named before anything is read.",
          eyebrow: "Architecture & Semantics",
          sections: [
            {
              content:
                "Host endpoints reject a request that does not say which Beacon it targets, so this page shows a prompt instead of data until you pick a node. Switching the node re-reads everything against that machine.",
              icon: Server,
              title: "One named node",
            },
            {
              content: `Polling, not streaming: system, disk, memory and network reads refresh every ${HOST_REFRESH_MS / 1000} seconds and processes every ${PROCESS_REFRESH_MS / 1000}. Each section shows its own last successful read, and a reading older than its interval is marked stale.`,
              icon: Info,
              title: "Read cadence",
            },
            {
              content:
                "A field the node did not send renders as \"Not reported\" and an unmeasured utilisation bar stays dashed. Neither is shown as 0, and neither is shown as healthy.",
              icon: MemoryStick,
              title: "Unmeasured values",
            },
          ],
          title: "Host inspection",
          triggerLabel: "About Host Inspector",
        }}
        action={
          <div className="min-w-56">
            <AdminSelect
              disabled={nodesQuery.isPending || nodeOptions.length === 0}
              label="Target node"
              onChange={(value) => setNodeId(value)}
              options={nodeOptions}
              placeholder={
                nodesQuery.isPending ? "Loading nodes…" : nodeOptions.length === 0 ? "No nodes registered" : "Select a node…"
              }
              value={nodeId}
            />
          </div>
        }
      />

      {nodesQuery.isPending ? (
        <AdminLoadingState label="Loading nodes…" />
      ) : nodesQuery.isError ? (
        <AdminErrorState
          message={hostErrorMessage(nodesQuery.error)}
          retry={() => void nodesQuery.refetch()}
        />
      ) : nodes.length === 0 ? (
        <EmptyState
          icon={Server}
          message="No Beacon daemon is registered, so there is no host to inspect. Add one under Infrastructure → Nodes."
          title="No nodes available"
        />
      ) : !nodeId ? (
        <EmptyState
          icon={Server}
          message="Choose the node above. Disk, memory, network and process readings all come from that one machine, so nothing is fetched until you name it."
          title="No node selected"
        />
      ) : (
        <>
          <AdminTabs active={activeTab} label="Host sections" onChange={(id) => setActiveTab(id as Tab)} tabs={TABS} />
          <div aria-label={`${activeLabel} for the selected node`} role="tabpanel">
            <Card>
              <div className="flex items-center gap-2 border-b border-line pb-3 text-xs font-semibold text-text">
                <ActiveIcon aria-hidden="true" className="text-text-subtle" size={14} />
                {activeLabel}
              </div>
              <div className="pt-4">
                {activeTab === "info" && <InfoTab nodeId={nodeId} />}
                {activeTab === "disk" && <DiskTab nodeId={nodeId} />}
                {activeTab === "memory" && <MemoryTab nodeId={nodeId} />}
                {activeTab === "network" && <NetworkTab nodeId={nodeId} />}
                {activeTab === "processes" && <ProcessesTab nodeId={nodeId} />}
              </div>
            </Card>
          </div>
        </>
      )}
    </AdminPageLayout>
  );
}
