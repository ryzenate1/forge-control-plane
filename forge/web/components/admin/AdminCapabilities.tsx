"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Cpu, RefreshCw, Zap, Layers, History, GitCompare } from "lucide-react";
import {
  fetchCapabilities,
  fetchCapability,
  fetchCapabilityHistory,
  fetchCapabilityDelta,
  probeCapabilities,
  type NodeCapability,
} from "@/lib/api/capabilities";
import { REFRESH, relativeTime, sourceState, useNodesQuery } from "@/lib/admin/telemetry";
import { FreshnessBadge, NotReported } from "@/components/admin/telemetry-ui";
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
  AdminSelect,
  AdminTable,
  AdminTabs,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  EmptyState,
  Input,
  cn,
} from "./admin-ui";
import { formatDate } from "@/lib/utils";
import { toneStyles } from "@/components/ui/forge/status";

type Tab = "inventory" | "detail" | "drift";

export function AdminCapabilities() {
  const [tab, setTab] = useState<Tab>("inventory");
  const [offset, setOffset] = useState(0);
  const limit = 20;
  const [selectedNodeId, setSelectedNodeId] = useState<string>("");
  const [search, setSearch] = useState("");

  const capsQ = useQuery({
    queryKey: ["admin-capabilities-global", offset, limit],
    queryFn: () => fetchCapabilities(offset, limit),
    // A snapshot page that never re-reads cannot show drift arriving; the page
    // is about freshness, so it polls at the inventory cadence like its siblings.
    refetchInterval: REFRESH.inventory,
    retry: false,
  });

  // Was a `fetchNodes` query nobody used while the picker identified hosts by
  // truncated hex id. It now supplies the names, and the shared hook gives the
  // same cache key and cadence the Nodes page uses.
  const nodesQ = useNodesQuery();
  const nodes = useMemo(() => nodesQ.data ?? [], [nodesQ.data]);
  const nodeName = useMemo(() => {
    const map = new Map<string, string>();
    for (const node of nodes) map.set(node.id, node.name);
    return map;
  }, [nodes]);

  const capsSource = sourceState(capsQ, REFRESH.inventory);
  const caps = useMemo(() => capsQ.data ?? [], [capsQ.data]);
  const searching = search.trim().length > 0;
  const filtered = useMemo(() => {
    if (!searching) return caps;
    const s = search.toLowerCase();
    return caps.filter((c) => c.nodeId.toLowerCase().includes(s)
      || (nodeName.get(c.nodeId) ?? "").toLowerCase().includes(s)
      || c.os.toLowerCase().includes(s)
      || c.beaconVersion.toLowerCase().includes(s)
      || c.architecture.toLowerCase().includes(s));
  }, [caps, search, searching, nodeName]);

  // `caps.length` is one window of an unknown population: the endpoint reports
  // no total. Nothing on this page may call a page count a fleet count.
  const pageFull = caps.length >= limit;

  const tabs: Array<{ id: string; label: string }> = [
    { id: "inventory", label: "Inventory" },
    { id: "detail", label: "Detail" },
    { id: "drift", label: "Drift / Delta" },
  ];

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { void capsQ.refetch(); void nodesQ.refetch(); }} />
      <SectionHeader
        status={<FreshnessBadge state={capsSource} />}
        info={{
          title: "Node Capabilities",
          triggerLabel: "About Node Capabilities",
          eyebrow: "Architecture & Semantics",
          description: "Per-node capability inventory and drift across the fleet.",
          sections: [
            { title: "Inventory & probe", content: "Each Beacon reports its runtime, build and storage capabilities. Probing refreshes the snapshot live; pick a node explicitly — nothing is inferred." },
            { title: "Drift delta", content: "Delta compares the two newest snapshots per node. Added, removed and changed entries signal drift worth triaging. Drift is per node here: the control plane reports no fleet-wide delta, so 'did anything change?' still means opening a node." },
            { title: "Reported, not enforced", content: "These flags are what a Beacon says it can do. The create and settings forms do not gate on them yet — a node that has never reported Nomad can still be assigned Nomad — so treat this page as the evidence and the placement forms as the decision." },
            { title: "What this page counts", content: "The inventory is a window of snapshots, and the endpoint returns no total, so every count here is 'this page', never 'the fleet'." },
          ],
        }}
        action={
          <div className="flex gap-2">
            <Btn size="sm" tone="ghost" onClick={() => { void capsQ.refetch(); void nodesQ.refetch(); }}>
              <RefreshCw size={14} /> Refresh
            </Btn>
            {/* Previously this button did not exist at all until a node was
                selected — the headline action vanished rather than explaining
                why it could not run. */}
            {selectedNodeId ? (
              <ProbeButton nodeId={selectedNodeId} onDone={() => { void capsQ.refetch(); }} />
            ) : (
              <Btn disabled size="sm" tone="primary" title="Probe acts on one node; select one in the inventory first.">
                <Zap size={12} /> Probe live — select a node first
              </Btn>
            )}
          </div>
        }
      />

      <AdminTabs label="Capability sections" tabs={tabs} active={tab} onChange={(id) => setTab(id as Tab)} />

      {tab === "inventory" && (
        <Card className="border border-line bg-overlay-subtle">
          <CardHeader
            icon={Cpu}
            title={capsSource.status === "ready"
              ? `${filtered.length} of ${caps.length} snapshots on this page`
              : "Capability snapshots"}
            action={
              <span className="flex items-center gap-2">
                <Pill tone="neutral" className="font-mono text-[10px]">offset {offset} · limit {limit}</Pill>
                <FreshnessBadge state={capsSource} />
              </span>
            }
          />
          <div className="flex flex-wrap items-end gap-3 border-b border-line p-4">
            <div className="min-w-[200px] flex-1">
              <Input label="Search this page" value={search} onChange={setSearch} placeholder="Node name, id, OS, arch or version" />
              {/* The honest scope of the filter: it narrows the loaded window,
                  not the fleet, and a miss here is not an absence of reports. */}
              <p className="mt-1.5 text-xs leading-5 text-text-muted">
                Searches the {caps.length} snapshot{caps.length === 1 ? "" : "s"} loaded on this page only
                {pageFull ? " — the node you want may be on another page; use Next to move on." : "."}
              </p>
            </div>
            <Btn size="sm" tone="ghost" onClick={() => void capsQ.refetch()}>Reload</Btn>
            <div className="flex gap-2">
              <Btn size="sm" tone="ghost" disabled={offset === 0} onClick={() => setOffset((o) => Math.max(0, o - limit))}>Prev</Btn>
              <Btn size="sm" tone="ghost" disabled={!pageFull} onClick={() => setOffset((o) => o + limit)}>Next</Btn>
            </div>
          </div>

          {capsQ.isPending ? (
            <AdminLoadingState label="Loading capabilities…" />
          ) : capsQ.isError ? (
            <div className="p-4"><AdminErrorState message={(capsQ.error as Error).message} retry={() => void capsQ.refetch()} /></div>
          ) : filtered.length === 0 && searching ? (
            <EmptyState
              icon={Cpu}
              title="No match on this page"
              message={`None of the ${caps.length} snapshots loaded here match “${search.trim()}”. This is not a fleet-wide absence of reports — page forward, or clear the search.`}
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={Cpu}
              title="No capability snapshots at this offset"
              message="No rows returned for this page. Beacons report their capabilities on their own; probe a node from the Nodes page to force one."
            />
          ) : (
            <AdminTable label="Capability inventory">
              <AdminTHead>
                <AdminTh>Node</AdminTh>
                <AdminTh>Beacon</AdminTh>
                <AdminTh>Runtime</AdminTh>
                <AdminTh>Build</AdminTh>
                <AdminTh>Compose</AdminTh>
                <AdminTh>Storage</AdminTh>
                <AdminTh>Updated</AdminTh>
                <AdminTh><span className="sr-only">Actions</span></AdminTh>
              </AdminTHead>
              <AdminTBody>
                {filtered.map((c) => (
                  <CapabilityRow
                    key={c.nodeId}
                    cap={c}
                    nodeName={nodeName.get(c.nodeId)}
                    selected={selectedNodeId === c.nodeId}
                    onSelect={() => { setSelectedNodeId(c.nodeId); setTab("detail"); }}
                  />
                ))}
              </AdminTBody>
            </AdminTable>
          )}
          <div className="border-t border-line p-3 text-xs leading-5 text-text-subtle">
            {pageFull
              ? `Showing one page of ${limit} snapshots. The inventory endpoint reports no total, so the fleet size is unknown here; use Next to read further pages.`
              : "Last page of the snapshot window read so far. The endpoint reports no total, so no fleet count is implied by these rows."}
            Drift is computed from the last two history entries per node (added / removed / changed).
          </div>
        </Card>
      )}

      {tab === "detail" && (
        <DetailPanel selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} caps={caps} nodeName={nodeName} />
      )}

      {tab === "drift" && (
        <DriftPanel selectedNodeId={selectedNodeId} onSelectNode={setSelectedNodeId} caps={caps} nodeName={nodeName} />
      )}
    </AdminPageLayout>
  );
}

function CapabilityRow({ cap, selected, onSelect, nodeName }: { cap: NodeCapability; selected: boolean; onSelect: () => void; nodeName?: string }) {
  return (
    <AdminTr className={`${selected ? "bg-overlay-subtle" : ""}`}>
      <AdminTd>
        <div className="text-xs font-medium text-text">{nodeName ?? "Unnamed node"}</div>
        <div className="font-mono text-[11px] text-text-subtle">{cap.nodeId.slice(0, 12)}…</div>
        <div className="text-xs text-text-subtle">{cap.os} · {cap.architecture} · {cap.cpuThreads} threads · {cap.memoryMb} MiB</div>
      </AdminTd>
      <AdminTd className="font-mono text-xs text-text-subtle">{cap.beaconVersion || <NotReported reason="Beacon reported no version" />}</AdminTd>
      <AdminTd>
        <Pill tone={cap.runtimeAvailable ? "green" : "red"}>{cap.runtimeAvailable ? cap.runtimeStatus || "available" : "unavailable"}</Pill>
        <div className="mt-1 text-[11px] text-text-subtle">{cap.runtimeProvider || "no provider reported"} {cap.runtimeVersion || ""}</div>
      </AdminTd>
      <AdminTd>
        <div className="flex gap-1"><Pill tone={cap.dockerBuildEnabled ? "green" : "neutral"}>docker</Pill><Pill tone={cap.nixpacksEnabled ? "green" : "neutral"}>nixpacks</Pill></div>
      </AdminTd>
      <AdminTd>
        <Pill tone={cap.composeEnabled ? "blue" : "neutral"}>{cap.composeEnabled ? `compose ${cap.composeVersion ?? ""}` : "off"}</Pill>
        <div className="text-[11px] text-text-subtle">{cap.stackCount} stacks</div>
      </AdminTd>
      <AdminTd>
        <div className="flex flex-wrap gap-1"><Pill tone={cap.localBackups ? "green" : "neutral"}>local</Pill><Pill tone={cap.s3Backups ? "blue" : "neutral"}>s3</Pill><Pill tone={cap.transferEnabled ? "green" : "neutral"}>transfer</Pill><Pill tone={cap.sftpEnabled ? "blue" : "neutral"}>sftp</Pill></div>
      </AdminTd>
      {/* Absolute time plus its age: a snapshot from yesterday and one from ten
          seconds ago used to carry identical weight in an unqualified timestamp. */}
      <AdminTd className="font-mono text-xs text-text-subtle">
        {cap.fetchedAt ? (
          <>
            <div>{formatDate(cap.fetchedAt)}</div>
            <div className={cn("text-[11px]", isStaleSnapshot(cap.fetchedAt) ? "text-warn" : "text-text-muted")}>
              {relativeTime(Date.parse(cap.fetchedAt)) ?? "age unknown"}
            </div>
          </>
        ) : <NotReported reason="Snapshot has no timestamp" />}
      </AdminTd>
      <AdminTd className="text-right">
        <Btn size="sm" tone={selected ? "primary" : "ghost"} onClick={onSelect}>View</Btn>
      </AdminTd>
    </AdminTr>
  );
}

/** Older than the inventory cadence twice over — the same rule `sourceState` uses. */
function isStaleSnapshot(fetchedAt: string): boolean {
  const at = Date.parse(fetchedAt);
  return Number.isFinite(at) && Date.now() - at > REFRESH.inventory * 2;
}

function DetailPanel({ selectedNodeId, onSelectNode, caps, nodeName }: { selectedNodeId: string; onSelectNode: (id: string) => void; caps: NodeCapability[]; nodeName: Map<string, string> }) {
  const capQ = useQuery({
    queryKey: ["admin-capability-detail", selectedNodeId],
    queryFn: () => fetchCapability(selectedNodeId),
    enabled: !!selectedNodeId,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
  const histQ = useQuery({
    queryKey: ["admin-capability-history", selectedNodeId],
    queryFn: () => fetchCapabilityHistory(selectedNodeId, 10),
    enabled: !!selectedNodeId,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });

  if (!selectedNodeId) {
    return (
      <Card className="border border-line bg-overlay-subtle p-4">
        <NodePicker caps={caps} nodeName={nodeName} onPick={onSelectNode} />
        <p className="mt-3 text-sm text-text-subtle">Select a node to view its capability detail and history.</p>
      </Card>
    );
  }

  return (
    <div className="grid gap-6">
      <Card className="border border-line bg-overlay-subtle">
        <CardHeader title="Detail" icon={Layers} action={<Btn size="sm" tone="ghost" onClick={() => { void capQ.refetch(); void histQ.refetch(); }}>Reload</Btn>} />
        <div className="p-4">
          <NodePicker caps={caps} nodeName={nodeName} onPick={onSelectNode} value={selectedNodeId} />
        </div>
        {capQ.isPending ? <AdminLoadingState label="Loading capability…" /> : capQ.isError ? <div className="p-4"><AdminErrorState message={(capQ.error as Error).message} retry={() => void capQ.refetch()} /></div> : capQ.data ? (
          <div className="space-y-4 p-4">
            <div className="grid gap-3 md:grid-cols-3">
              <Stat label="Beacon" value={capQ.data.beaconVersion || ""} />
              <Stat label="OS / Arch" value={`${capQ.data.os} / ${capQ.data.architecture}`} />
              <Stat label="CPU / Memory" value={`${capQ.data.cpuThreads} threads · ${capQ.data.memoryMb} MiB`} />
              <Stat label="Runtime" value={capQ.data.runtimeAvailable ? capQ.data.runtimeStatus || "available" : "unavailable"} tone={capQ.data.runtimeAvailable ? "green" : "red"} />
              {/* Unreported uptime used to render "0h" — a real reading. */}
              <Stat label="Uptime" value={typeof capQ.data.uptimeSeconds === "number" && Number.isFinite(capQ.data.uptimeSeconds) ? `${Math.floor(capQ.data.uptimeSeconds / 3600)}h` : ""} />
              <Stat label="Fetched" value={capQ.data.fetchedAt ? `${formatDate(capQ.data.fetchedAt)} · ${relativeTime(Date.parse(capQ.data.fetchedAt)) ?? "age unknown"}` : ""} />
            </div>
            <div className="grid gap-2 md:grid-cols-2 text-xs">
              <div className="rounded-lg border border-line bg-overlay-subtle p-3">
                <div className="font-semibold uppercase tracking-widest text-text-subtle">Flags</div>
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
              <div className="rounded-lg border border-line bg-overlay-subtle p-3">
                <div className="font-semibold uppercase tracking-widest text-text-subtle">Raw report (diagnostic)</div>
                <pre className="mt-2 max-h-40 overflow-auto rounded bg-overlay-strong p-2 font-mono text-[11px] leading-5 text-text-subtle">{JSON.stringify(capQ.data.rawReport ?? capQ.data, null, 2)}</pre>
              </div>
            </div>
          </div>
        ) : null}

        <div className="border-t border-line p-4">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.12em] text-text-subtle">
            <History size={12} /> History
          </div>
          {histQ.isPending ? <div className="mt-2 text-xs text-text-subtle">Loading history…</div> : histQ.isError ? <div className="mt-2"><AdminErrorState message={(histQ.error as Error).message} retry={() => void histQ.refetch()} /></div> : (histQ.data?.length ?? 0) === 0 ? <div className="mt-2 text-xs text-text-subtle">No history for this node yet — probe it or wait for a heartbeat to store a snapshot.</div> : (
            <ol className="mt-3 space-y-2">
              {(histQ.data ?? []).map((h) => {
                const text = JSON.stringify(h.capabilities, null, 2) ?? "";
                const shown = text.slice(0, 600);
                return (
                  <li key={h.id} className="rounded-lg border border-line bg-overlay-subtle px-3 py-2">
                    <div className="flex items-center gap-2 font-mono text-xs">
                      <span className="font-medium text-text">{h.observedAt ? formatDate(h.observedAt) : "untimestamped"}</span>
                      <span className="text-text-subtle">· {h.beaconVersion || "no version"}</span>
                    </div>
                    <pre className="mt-1 overflow-auto font-mono text-[11px] leading-5 text-text-subtle">{shown}{shown.length < text.length ? `\n… truncated, ${text.length - shown.length} characters not shown` : ""}</pre>
                  </li>
                );
              })}
            </ol>
          )}
        </div>
      </Card>
    </div>
  );
}

function DriftPanel({ selectedNodeId, onSelectNode, caps, nodeName }: { selectedNodeId: string; onSelectNode: (id: string) => void; caps: NodeCapability[]; nodeName: Map<string, string> }) {
  const deltaQ = useQuery({
    queryKey: ["admin-capability-delta", selectedNodeId],
    queryFn: () => fetchCapabilityDelta(selectedNodeId),
    enabled: !!selectedNodeId,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });

  if (!selectedNodeId) {
    return (
      <Card className="border border-line bg-overlay-subtle p-4">
        <NodePicker caps={caps} nodeName={nodeName} onPick={onSelectNode} />
        <p className="mt-3 text-sm text-text-subtle">Select a node to compute drift between its last two snapshots.</p>
      </Card>
    );
  }

  const d = deltaQ.data;
  const drifted = !!d && (d.added.length > 0 || d.removed.length > 0 || d.changed.length > 0);

  return (
    <div className="grid gap-6">
      <Card className="border border-line bg-overlay-subtle">
        <CardHeader title="Delta" icon={GitCompare} action={<Btn size="sm" tone="ghost" onClick={() => void deltaQ.refetch()}>Recompute</Btn>} />
        <div className="p-4">
          <NodePicker caps={caps} nodeName={nodeName} onPick={onSelectNode} value={selectedNodeId} />
        </div>
        {deltaQ.isPending ? <AdminLoadingState label="Computing delta…" /> : deltaQ.isError ? <div className="p-4"><AdminErrorState message={(deltaQ.error as Error).message} retry={() => void deltaQ.refetch()} /></div> : d ? (
          <div className="space-y-4 p-4">
            <div className="flex flex-wrap gap-2 text-xs">
              <Pill tone="green">+ {d.added.length} added</Pill>
              <Pill tone="red">− {d.removed.length} removed</Pill>
              <Pill tone="yellow">~ {d.changed.length} changed</Pill>
              <Pill tone="neutral">= {d.unchanged.length} unchanged</Pill>
              <span className="ml-auto font-mono text-[11px] text-text-subtle">fetchedAt {d.fetchedAt ? `${formatDate(d.fetchedAt)} · ${relativeTime(Date.parse(d.fetchedAt)) ?? "age unknown"}` : "not reported"}</span>
            </div>
            {drifted ? (
              <div className={cn("flex items-start gap-2 rounded-lg border p-3 text-sm", toneStyles.warn.chip)} role="alert">
                <AlertTriangle size={14} className={cn("mt-0.5 shrink-0", toneStyles.warn.fg)} />
                <span>Capability drift detected — {d.added.length} added · {d.removed.length} removed · {d.changed.length} changed since last snapshot. Probe the node or compare the two newest history rows to triage. One-snapshot deltas report everything as <code className="rounded bg-overlay-strong px-1 font-mono text-xs">added</code> (no baseline).</span>
              </div>
            ) : null}
            <div className="grid gap-4 md:grid-cols-2">
              <DeltaSection title="Added" items={d.added} tone="green" />
              <DeltaSection title="Removed" items={d.removed} tone="red" />
              <DeltaSection title="Changed" items={d.changed} tone="yellow" />
              <DeltaSection title="Unchanged" items={d.unchanged} tone="neutral" />
            </div>
            {!drifted && (d.unchanged.length > 0 ? (
              <div className={cn("rounded-lg border p-3 text-sm", toneStyles.ok.chip)}>
                No drift — {d.unchanged.length} capabilit{d.unchanged.length === 1 ? "y" : "ies"} identical across the two newest snapshots.
              </div>
            ) : (
              /* All three buckets empty with nothing unchanged is what the server
                 returns when both snapshots unmarshal to zero entries. That is not
                 stability, and it used to render a green all-clear. */
              <div className={cn("rounded-lg border border-dashed p-3 text-sm", toneStyles.unknown.chip)}>
                Nothing to compare — neither of the two newest snapshots carried capability entries, so stability is unknown, not confirmed.
              </div>
            ))}
          </div>
        ) : null}
        <div className="border-t border-line p-4">
          <p className="text-xs leading-5 text-text-subtle">
            Drift compares the two newest snapshots for this node. With a single snapshot everything reports as added — the honest no-baseline signal. No fleet-wide delta exists: the control plane answers per node, so &ldquo;did anything change?&rdquo; needs one node at a time.
          </p>
        </div>
      </Card>
    </div>
  );
}

function DeltaSection({ title, items, tone }: { title: string; items: unknown[]; tone: "green" | "red" | "yellow" | "neutral" }) {
  const map: Record<string, string> = {
    green: cn(toneStyles.ok.border, toneStyles.ok.bg),
    red: cn(toneStyles.danger.border, toneStyles.danger.bg),
    yellow: cn(toneStyles.warn.border, toneStyles.warn.bg),
    neutral: "border-line bg-overlay-subtle",
  };
  return (
    <div className={`rounded-xl border p-3 ${map[tone]}`}>
      <div className="text-[11px] font-bold uppercase tracking-widest text-text-subtle">{title} · {items.length}</div>
      {items.length === 0 ? <div className="mt-2 text-xs text-text-subtle">none in this bucket</div> : (
        <ul className="mt-2 space-y-1">
          {items.map((it, i) => {
            const text = JSON.stringify(it, null, 2);
            const shown = text.slice(0, 400);
            return (
              <li key={i} className="rounded border border-line bg-overlay-strong px-2 py-1 font-mono text-[11px] leading-5 text-text-subtle">
                {shown}
                {shown.length < text.length ? <span className="mt-0.5 block text-info">… truncated, {text.length - shown.length} characters not shown</span> : null}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function NodePicker({ caps, onPick, value, nodeName }: { caps: NodeCapability[]; onPick: (id: string) => void; value?: string; nodeName: Map<string, string> }) {
  return (
    <div>
      <AdminSelect
        label="Node"
        value={value ?? ""}
        onChange={onPick}
        placeholder="Select node…"
        /* Nodes are named where a name exists; the truncated id stays as the
           disambiguator rather than being the whole label. */
        options={caps.map((c) => ({ value: c.nodeId, label: `${nodeName.get(c.nodeId) ?? c.nodeName ?? "Unnamed node"} · ${c.nodeId.slice(0, 8)} · ${c.os} · ${c.beaconVersion || "no version"}` }))}
      />
      <p className="mt-1.5 text-xs leading-5 text-text-muted">
        Only nodes with a capability snapshot on the loaded inventory page can be chosen here.
      </p>
    </div>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: "green" | "red" }) {
  const color = tone === "green" ? toneStyles.ok.fg : tone === "red" ? toneStyles.danger.fg : "text-text";
  const missing = !value;
  return (
    <div className="rounded-xl border border-line bg-overlay-subtle p-3">
      <div className="text-[11px] uppercase tracking-widest text-text-subtle">{label}</div>
      <div className={cn("mt-1 truncate font-mono text-xs font-semibold", missing ? "text-text-subtle" : color)} title={missing ? "Not reported" : value}>
        {missing ? <NotReported reason="This field is absent from the snapshot" /> : value}
      </div>
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
