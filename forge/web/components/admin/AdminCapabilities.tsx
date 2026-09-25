"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, AlertTriangle, Cpu, HardDrive, Network, RefreshCw, Search, Zap, Layers, History, GitCompare } from "lucide-react";
import {
  fetchCapabilities,
  fetchCapability,
  fetchCapabilityHistory,
  fetchCapabilityDelta,
  probeCapabilities,
  type NodeCapability,
} from "@/lib/api/capabilities";
import { fetchNodes } from "@/lib/api";
import { useT } from "@/components/TranslationProvider";
import { useToast } from "@/components/ui/toast";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageLayout,
  SectionHeader,
  Card,
  CardHeader,
  Btn,
  Pill,
  AdminLoadingState,
  AdminErrorState,
  AdminTabs,
  EmptyState,
  Input,
} from "./admin-ui";

type Tab = "inventory" | "detail" | "drift";

export function AdminCapabilities() {
  const t = useT();
  const { toast } = useToast();
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>("inventory");
  const [offset, setOffset] = useState(0);
  const limit = 20;
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");
  const [search, setSearch] = useState("");

  const capsQ = useQuery({
    queryKey: ["admin-capabilities-global", offset, limit],
    queryFn: () => fetchCapabilities(offset, limit),
    retry: false,
  });

  const nodesQ = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, retry: false });

  const caps = useMemo(() => capsQ.data ?? [], [capsQ.data]);
  const filtered = useMemo(() => {
    if (!search.trim()) return caps;
    const s = search.toLowerCase();
    return caps.filter((c) => c.nodeId.toLowerCase().includes(s) || c.os.toLowerCase().includes(s) || c.beaconVersion.toLowerCase().includes(s) || c.architecture.toLowerCase().includes(s));
  }, [caps, search]);

  const tabs: Array<{ id: string; label: string }> = [
    { id: "inventory", label: `Inventory · ${caps.length}` },
    { id: "detail", label: "Detail" },
    { id: "drift", label: "Drift / Delta" },
  ];

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { void capsQ.refetch(); void nodesQ.refetch(); }} />
      <SectionHeader
        title="Capabilities"
        sub="Global node capability inventory and drift delta — GET /capabilities, GET /capabilities/:nodeId/delta, POST /capabilities/:nodeId/probe (handlers_capabilities.go:15)"
        action={
          <div className="flex gap-2">
            <Btn size="sm" tone="ghost" onClick={() => { void capsQ.refetch(); void nodesQ.refetch(); }}>
              <RefreshCw size={14} /> Refresh
            </Btn>
            {selectedNodeId && (
              <ProbeButton nodeId={selectedNodeId} onDone={() => { void capsQ.refetch(); }} />
            )}
          </div>
        }
      />

      <AdminTabs tabs={tabs} active={tab} onChange={(id) => setTab(id as Tab)} />

      {tab === "inventory" && (
        <Card className="border border-[var(--line)] bg-[var(--surface)]">
          <CardHeader
            title="Inventory — GET /capabilities"
            icon={Cpu}
            action={
              <span className="flex items-center gap-2">
                <span className="text-xs text-[var(--text-subtle)]">{filtered.length} nodes</span>
                <Pill tone="neutral" className="font-mono text-[10px]">offset {offset} · limit {limit}</Pill>
              </span>
            }
          />
          <div className="flex flex-wrap items-end gap-3 border-b border-[var(--line)] p-4">
            <div className="flex-1 min-w-[200px]"><Input label="Search node / OS / arch / version" value={search} onChange={setSearch} placeholder="e.g. ubuntu, x86_64, 1.2.3" /></div>
            <Btn size="sm" tone="ghost" onClick={() => void capsQ.refetch()}>Reload</Btn>
            <div className="flex gap-2">
              <Btn size="sm" tone="ghost" disabled={offset === 0} onClick={() => setOffset((o) => Math.max(0, o - limit))}>Prev</Btn>
              <Btn size="sm" tone="ghost" disabled={caps.length < limit} onClick={() => setOffset((o) => o + limit)}>Next</Btn>
            </div>
          </div>

          {capsQ.isLoading ? (
            <AdminLoadingState label="Loading capabilities…" />
          ) : capsQ.isError ? (
            <div className="p-4"><AdminErrorState message={(capsQ.error as Error).message} retry={() => void capsQ.refetch()} /></div>
          ) : filtered.length === 0 ? (
            <EmptyState icon={Cpu} title="No capabilities" message="No node capabilities reported yet. Beacons report via POST /nodes/capabilities or via probe." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--line)] bg-[var(--surface-raised)] text-left text-[10px] uppercase tracking-[0.12em] text-[var(--text-subtle)]">
                    <th className="px-4 py-3">Node</th>
                    <th className="px-4 py-3">Beacon</th>
                    <th className="px-4 py-3">Runtime</th>
                    <th className="px-4 py-3">Build</th>
                    <th className="px-4 py-3">Compose</th>
                    <th className="px-4 py-3">Storage</th>
                    <th className="px-4 py-3">Updated</th>
                    <th className="px-4 py-3"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--line)]">
                  {filtered.map((c) => (
                    <CapabilityRow key={c.nodeId} cap={c} selected={selectedNodeId === c.nodeId} onSelect={() => { setSelectedNodeId(c.nodeId); setTab("detail"); }} />
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div className="border-t border-[var(--line)] p-3 text-xs text-[var(--text-subtle)]">
            Live-probe any node to refresh its capability snapshot. Drift is computed from the last two history entries (added / removed / changed).
          </div>
        </Card>
      )}

      {tab === "detail" && (
        <DetailPanel selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} caps={caps} />
      )}

      {tab === "drift" && (
        <DriftPanel selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} caps={caps} />
      )}
    </AdminPageLayout>
  );
}

function CapabilityRow({ cap, selected, onSelect }: { cap: NodeCapability; selected: boolean; onSelect: () => void }) {
  return (
    <tr className={`hover:bg-[var(--surface-hover)] motion-safe:transition-colors ${selected ? "bg-[var(--brand)]/5" : ""}`}>
      <td className="px-4 py-3">
        <div className="font-mono text-xs font-medium text-[var(--text)]">{cap.nodeId.slice(0, 12)}…</div>
        <div className="text-xs text-[var(--text-subtle)]">{cap.os} · {cap.architecture} · {cap.cpuThreads} threads · {cap.memoryMb} MiB</div>
      </td>
      <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{cap.beaconVersion || "—"}</td>
      <td className="px-4 py-3">
        <Pill tone={cap.runtimeAvailable ? "green" : "red"}>{cap.runtimeAvailable ? cap.runtimeStatus || "available" : "unavailable"}</Pill>
        <div className="mt-1 text-[11px] text-[var(--text-subtle)]">{cap.runtimeProvider || ""} {cap.runtimeVersion || ""}</div>
      </td>
      <td className="px-4 py-3">
        <div className="flex gap-1"><Pill tone={cap.dockerBuildEnabled ? "green" : "neutral"}>docker</Pill><Pill tone={cap.nixpacksEnabled ? "green" : "neutral"}>nixpacks</Pill></div>
      </td>
      <td className="px-4 py-3">
        <Pill tone={cap.composeEnabled ? "blue" : "neutral"}>{cap.composeEnabled ? `compose ${cap.composeVersion ?? ""}` : "off"}</Pill>
        <div className="text-[11px] text-[var(--text-subtle)]">{cap.stackCount} stacks</div>
      </td>
      <td className="px-4 py-3">
        <div className="flex flex-wrap gap-1"><Pill tone={cap.localBackups ? "green" : "neutral"}>local</Pill><Pill tone={cap.s3Backups ? "blue" : "neutral"}>s3</Pill><Pill tone={cap.transferEnabled ? "green" : "neutral"}>transfer</Pill><Pill tone={cap.sftpEnabled ? "blue" : "neutral"}>sftp</Pill></div>
      </td>
      <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{cap.fetchedAt ? new Date(cap.fetchedAt).toLocaleString() : "—"}</td>
      <td className="px-4 py-3 text-right">
        <Btn size="sm" tone={selected ? "primary" : "ghost"} onClick={onSelect}>View</Btn>
      </td>
    </tr>
  );
}

function DetailPanel({ selectedNodeId, onSelectNode, caps }: { selectedNodeId: string; onSelectNode: (id: string) => void; caps: NodeCapability[] }) {
  const capQ = useQuery({
    queryKey: ["admin-capability-detail", selectedNodeId],
    queryFn: () => fetchCapability(selectedNodeId),
    enabled: !!selectedNodeId,
    retry: false,
  });
  const histQ = useQuery({
    queryKey: ["admin-capability-history", selectedNodeId],
    queryFn: () => fetchCapabilityHistory(selectedNodeId, 10),
    enabled: !!selectedNodeId,
    retry: false,
  });

  if (!selectedNodeId) {
    return (
      <Card className="border border-[var(--line)] bg-[var(--surface)] p-4">
        <NodePicker caps={caps} onPick={onSelectNode} />
        <p className="mt-3 text-sm text-[var(--text-subtle)]">Select a node to view its capability detail and history.</p>
      </Card>
    );
  }

  return (
    <div className="grid gap-6">
      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title={`Detail — GET /capabilities/${selectedNodeId.slice(0, 12)}…`} icon={Layers} action={<Btn size="sm" tone="ghost" onClick={() => { void capQ.refetch(); void histQ.refetch(); }}>Reload</Btn>} />
        <div className="p-4">
          <NodePicker caps={caps} onPick={onSelectNode} value={selectedNodeId} />
        </div>
        {capQ.isLoading ? <AdminLoadingState label="Loading capability…" /> : capQ.isError ? <div className="p-4"><AdminErrorState message={(capQ.error as Error).message} retry={() => void capQ.refetch()} /></div> : capQ.data ? (
          <div className="space-y-4 p-4">
            <div className="grid gap-3 md:grid-cols-3">
              <Stat label="Beacon" value={capQ.data.beaconVersion || "—"} />
              <Stat label="OS / Arch" value={`${capQ.data.os} / ${capQ.data.architecture}`} />
              <Stat label="CPU / Memory" value={`${capQ.data.cpuThreads} threads · ${capQ.data.memoryMb} MiB`} />
              <Stat label="Runtime" value={`${capQ.data.runtimeAvailable ? "✓" : "✗"} ${capQ.data.runtimeStatus}`} tone={capQ.data.runtimeAvailable ? "green" : "red"} />
              <Stat label="Uptime" value={`${Math.floor((capQ.data.uptimeSeconds ?? 0) / 3600)}h`} />
              <Stat label="Fetched" value={new Date(capQ.data.fetchedAt).toLocaleString()} />
            </div>
            <div className="grid gap-2 md:grid-cols-2 text-xs">
              <div className="rounded-lg border border-[var(--line)] bg-[var(--canvas)] p-3">
                <div className="font-semibold uppercase tracking-widest text-[var(--text-subtle)]">Flags</div>
                <div className="mt-2 flex flex-wrap gap-1">
                  <Pill tone={capQ.data.dockerBuildEnabled ? "green" : "neutral"}>dockerBuild</Pill>
                  <Pill tone={capQ.data.nixpacksEnabled ? "green" : "neutral"}>nixpacks</Pill>
                  <Pill tone={capQ.data.composeEnabled ? "blue" : "neutral"}>compose</Pill>
                  <Pill tone={capQ.data.localBackups ? "green" : "neutral"}>localBackups</Pill>
                  <Pill tone={capQ.data.s3Backups ? "blue" : "neutral"}>s3Backups</Pill>
                  <Pill tone={capQ.data.transferEnabled ? "green" : "neutral"}>transfer</Pill>
                  <Pill tone={capQ.data.sftpEnabled ? "blue" : "neutral"}>sftp</Pill>
                  <Pill tone={capQ.data.webSocketEnabled ? "blue" : "neutral"}>websocket</Pill>
                  <Pill tone={capQ.data.consoleEnabled ? "blue" : "neutral"}>console</Pill>
                  <Pill tone={capQ.data.databaseProvisioningEnabled ? "green" : "neutral"}>dbProvisioning</Pill>
                </div>
              </div>
              <div className="rounded-lg border border-[var(--line)] bg-[var(--canvas)] p-3">
                <div className="font-semibold uppercase tracking-widest text-[var(--text-subtle)]">Raw report</div>
                <pre className="mt-2 max-h-40 overflow-auto rounded bg-black/20 p-2 font-mono text-[11px] leading-5 text-[var(--text-subtle)]">{JSON.stringify(capQ.data.rawReport ?? capQ.data, null, 2)}</pre>
              </div>
            </div>
          </div>
        ) : null}

        <div className="border-t border-[var(--line)] p-4">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]">
            <History size={12} /> History — GET /capabilities/:nodeId/history
          </div>
          {histQ.isLoading ? <div className="mt-2 text-xs text-[var(--text-subtle)]">Loading history…</div> : histQ.isError ? <div className="mt-2 text-xs text-red-300">{(histQ.error as Error).message}</div> : (histQ.data?.length ?? 0) === 0 ? <div className="mt-2 text-xs text-[var(--text-subtle)]">No history — probe or wait for heartbeat to generate snapshots.</div> : (
            <ol className="mt-3 space-y-2">
              {(histQ.data ?? []).map((h) => (
                <li key={h.id} className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2">
                  <div className="flex items-center gap-2 font-mono text-xs">
                    <span className="font-medium text-[var(--text)]">{new Date(h.observedAt).toLocaleString()}</span>
                    <span className="text-[var(--text-subtle)]">· {h.beaconVersion}</span>
                  </div>
                  <pre className="mt-1 overflow-auto text-[11px] leading-5 text-[var(--text-subtle)]">{JSON.stringify(h.capabilities, null, 2)?.slice(0, 600)}</pre>
                </li>
              ))}
            </ol>
          )}
        </div>
      </Card>
    </div>
  );
}

function DriftPanel({ selectedNodeId, onSelectNode, caps }: { selectedNodeId: string; onSelectNode: (id: string) => void; caps: NodeCapability[] }) {
  const deltaQ = useQuery({
    queryKey: ["admin-capability-delta", selectedNodeId],
    queryFn: () => fetchCapabilityDelta(selectedNodeId),
    enabled: !!selectedNodeId,
    retry: false,
  });

  if (!selectedNodeId) {
    return (
      <Card className="border border-[var(--line)] bg-[var(--surface)] p-4">
        <NodePicker caps={caps} onPick={onSelectNode} />
        <p className="mt-3 text-sm text-[var(--text-subtle)]">Select a node to compute drift between its last two snapshots.</p>
      </Card>
    );
  }

  const d = deltaQ.data;

  return (
    <div className="grid gap-6">
      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title={`Delta — GET /capabilities/${selectedNodeId.slice(0, 12)}…/delta`} icon={GitCompare} action={<Btn size="sm" tone="ghost" onClick={() => void deltaQ.refetch()}>Recompute</Btn>} />
        <div className="p-4">
          <NodePicker caps={caps} onPick={onSelectNode} value={selectedNodeId} />
        </div>
        {deltaQ.isLoading ? <AdminLoadingState label="Computing delta…" /> : deltaQ.isError ? <div className="p-4"><AdminErrorState message={(deltaQ.error as Error).message} retry={() => void deltaQ.refetch()} /></div> : d ? (
          <div className="space-y-4 p-4">
            <div className="flex flex-wrap gap-2 text-xs">
              <Pill tone="green">+ {d.added.length} added</Pill>
              <Pill tone="red">− {d.removed.length} removed</Pill>
              <Pill tone="yellow">~ {d.changed.length} changed</Pill>
              <Pill tone="neutral">= {d.unchanged.length} unchanged</Pill>
              <span className="ml-auto font-mono text-[11px] text-[var(--text-subtle)]">fetchedAt {new Date(d.fetchedAt).toLocaleString()}</span>
            </div>
            {(d.added.length > 0 || d.removed.length > 0 || d.changed.length > 0) ? (
              <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-200" role="alert">
                <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                <span>Capability drift detected — {d.added.length} added · {d.removed.length} removed · {d.changed.length} changed since last snapshot. Probe the node or compare the two newest history rows to triage. One-snapshot deltas report everything as <code className="rounded bg-black/20 px-1 font-mono text-xs">added</code> (no baseline).</span>
              </div>
            ) : null}
            <div className="grid gap-4 md:grid-cols-2">
              <DeltaSection title="Added" items={d.added} tone="green" />
              <DeltaSection title="Removed" items={d.removed} tone="red" />
              <DeltaSection title="Changed" items={d.changed} tone="yellow" />
              <DeltaSection title="Unchanged" items={d.unchanged} tone="neutral" />
            </div>
            {(d.added.length === 0 && d.removed.length === 0 && d.changed.length === 0) && (
              <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-3 text-sm text-emerald-200">No drift — capability snapshot is stable.</div>
            )}
          </div>
        ) : null}
        <div className="border-t border-[var(--line)] p-4">
          <p className="text-xs leading-5 text-[var(--text-subtle)]">
            Drift is derived from <code className="font-mono text-[11px]">node_capability_history</code> (newest 2 rows, type-keyed compare). When only one snapshot exists the delta reports everything as <code className="font-mono">added</code> — the honest “no baseline” signal (handlers_capabilities.go:113).
          </p>
        </div>
      </Card>
    </div>
  );
}

function DeltaSection({ title, items, tone }: { title: string; items: unknown[]; tone: "green" | "red" | "yellow" | "neutral" }) {
  const map: Record<string, string> = { green: "border-emerald-500/20 bg-emerald-500/10", red: "border-red-500/20 bg-red-500/10", yellow: "border-amber-500/20 bg-amber-500/10", neutral: "border-[var(--line)] bg-[var(--surface-raised)]" };
  return (
    <div className={`rounded-xl border p-3 ${map[tone]}`}>
      <div className="text-[11px] font-bold uppercase tracking-widest text-[var(--text-subtle)]">{title} · {items.length}</div>
      {items.length === 0 ? <div className="mt-2 text-xs text-[var(--text-subtle)]">—</div> : (
        <ul className="mt-2 space-y-1">
          {items.map((it, i) => (
            <li key={i} className="rounded border border-white/[0.06] bg-black/20 px-2 py-1 font-mono text-[11px] leading-5 text-[var(--text-subtle)]">
              {JSON.stringify(it, null, 2).slice(0, 400)}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function NodePicker({ caps, onPick, value }: { caps: NodeCapability[]; onPick: (id: string) => void; value?: string }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Node</span>
      <select className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm text-[var(--text)]" value={value ?? ""} onChange={(e) => onPick(e.target.value)}>
        <option value="">Select node…</option>
        {caps.map((c) => <option key={c.nodeId} value={c.nodeId}>{c.nodeId.slice(0, 12)}… · {c.os} · {c.beaconVersion || "no version"}</option>)}
      </select>
    </label>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: "green" | "red" }) {
  const color = tone === "green" ? "text-emerald-300" : tone === "red" ? "text-red-300" : "text-[var(--text)]";
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--canvas)] p-3">
      <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">{label}</div>
      <div className={`mt-1 truncate font-mono text-xs font-semibold ${color}`} title={value}>{value || "—"}</div>
    </div>
  );
}

function ProbeButton({ nodeId, onDone }: { nodeId: string; onDone?: () => void }) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const mut = useMutation({
    mutationFn: () => probeCapabilities(nodeId),
    onSuccess: (data) => {
      if (!data.online) toast({ tone: "error", title: "Node offline", message: (data as { error?: string }).error ?? "beacon unreachable" });
      else toast({ tone: "success", title: "Probe succeeded", message: `Capabilities refreshed @ ${new Date().toLocaleTimeString()}` });
      void qc.invalidateQueries({ queryKey: ["admin-capabilities-global"] });
      void qc.invalidateQueries({ queryKey: ["admin-capability"] });
      onDone?.();
    },
    onError: (e: Error) => toast({ tone: "error", title: "Probe failed", message: e.message }),
  });
  return (
    <Btn size="sm" tone="primary" loading={mut.isPending} onClick={() => mut.mutate()}>
      <Zap size={12} /> Probe live
    </Btn>
  );
}
