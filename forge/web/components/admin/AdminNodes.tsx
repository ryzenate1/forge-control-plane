"use client";
import { REFRESH, hasDrift, mbLabel, nodeStatus, relativeTime, sourceState, useNodesQuery } from "@/lib/admin/telemetry";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity, AlertTriangle, ArrowDownUp, ChevronRight, Cpu, Database, Eye, EyeOff, Globe, HardDrive, History, Layers, GitCompare, KeyRound, Lock, Mail,
  MemoryStick, Network, Plus, Server, Settings as SettingsIcon, Shield, Trash2, Unlock, Wrench, Zap,
} from "lucide-react";
import { useRouter } from "next/navigation";
import {
  createNode, deleteNode, fetchServers, fetchLocations, fetchRegions, fetchNode, updateNode, rotateNodeToken,
  fetchNodeAllocations, fetchNodeServers, fetchNodeLifecycle,
  fetchNodeSystemInformation, setAllocationAlias, deleteAllocationsBulk, getBeaconAPIURL,
  type ApiNode, type ApiAllocation, type ApiLocation, type ApiRegion, type ApiServer,
  type CreateNodeInput, type UpdateNodeInput,
} from "@/lib/api";
import { fetchCapability, fetchCapabilityDelta, fetchCapabilityHistory, probeCapabilities } from "@/lib/api/capabilities";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { copySecret } from "@/lib/clipboard";
import { AdminTabs, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, Textarea, cn, Pill, AdminLoadingState, AdminErrorState, AdminPageLayout, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr } from "./admin-ui";
import { FreshnessBadge, NotReported } from "./telemetry-ui";
import { toneStyles } from "@/components/ui/forge/status";
import { InfoCard, KpiGrid, QuickActionsCard, type KpiDatum, type QuickAction } from "./dashboard-cards";
import { formatDate } from "@/lib/utils";
import { chart } from "@/lib/design-tokens";

type Tab = "about" | "settings" | "configuration" | "allocation" | "servers" | "capabilities";

const ADMIN_TABS: Array<{ id: Tab; label: string }> = [
  { id: "about", label: "About" },
  { id: "settings", label: "Settings" },
  { id: "configuration", label: "Configuration" },
  { id: "allocation", label: "Allocation" },
  { id: "servers", label: "Servers" },
  { id: "capabilities", label: "Capabilities" },
];

function validateNodeForm(name: string, locationId: string, fqdn: string, scheme: string, memoryMb: string, diskMb: string, daemonListen: string, daemonSftp: string): string | null {
  if (!name.trim()) return "Node name is required.";
  if (!locationId) return "Select a location.";
  const host = fqdn.trim().toLowerCase();
  if (!host) return "FQDN is required.";
  try {
    const endpoint = new URL(`${scheme}://${host}`);
    if ((endpoint.protocol !== "http:" && endpoint.protocol !== "https:") || endpoint.hostname.toLowerCase() !== host) return "Enter a valid FQDN or IP address.";
  } catch { return "Enter a valid FQDN or IP address."; }
  for (const [label, value, minimum, maximum] of [["Memory", memoryMb, 0, Number.MAX_SAFE_INTEGER], ["Disk", diskMb, 0, Number.MAX_SAFE_INTEGER], ["Daemon port", daemonListen, 1, 65535], ["SFTP port", daemonSftp, 1, 65535]] as const) {
    const number = Number(value);
    if (!Number.isInteger(number) || number < minimum || number > maximum) return `${label} must be an integer between ${minimum} and ${maximum}.`;
  }
  if (Number(daemonListen) === Number(daemonSftp)) return "Daemon and SFTP ports must be different.";
  return null;
}

/**
 * Total capacity from a reported allocated/available pair.
 *
 * Returns `undefined` unless **both** halves are real numbers. Summing with
 * `?? 0` turned "the node never reported" into a plausible-looking total, which
 * is how an unmeasured value ends up rendered as a measured one. This mirrors
 * the same rule `beacon-workspace.tsx` applies to a single host.
 */
function capacityTotal(allocated: number | undefined | null, available: number | undefined | null): number | undefined {
  if (typeof allocated !== "number" || !Number.isFinite(allocated)) return undefined;
  if (typeof available !== "number" || !Number.isFinite(available)) return undefined;
  return allocated + available;
}

type NodeSortKey = "name" | "state" | "heartbeat" | "location" | "memory" | "disk" | "servers";

export function AdminNodes() {
  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => Array.isArray(nodesQuery.data) ? nodesQuery.data : [], [nodesQuery.data]);
  const locationsQuery = useQuery({ queryKey: ["locations"], queryFn: fetchLocations });
  const locations = useMemo(() => Array.isArray(locationsQuery.data) ? locationsQuery.data : [], [locationsQuery.data]);
  const regionsQuery = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });
  const regions = useMemo(() => Array.isArray(regionsQuery.data) ? regionsQuery.data : [], [regionsQuery.data]);
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
  const servers = useMemo(() => Array.isArray(serversQuery.data) ? serversQuery.data : [], [serversQuery.data]);
  const [search, setSearch] = useState("");
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [sortKey, setSortKey] = useState<NodeSortKey>("name");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");

  const router = useRouter();

  // Real freshness for the frame's `status` slot. Every claim on this page that
  // says "current" is derived from this, not from "a response arrived".
  const nodesSource = sourceState(nodesQuery, REFRESH.inventory);
  const serversSource = sourceState(serversQuery, REFRESH.inventory);

  // Servers per node, resolved once for the whole table. Servers may reference
  // their host by id *or* by name, so a row counts either form — but a single
  // pass, so a node is never counted twice. The map backs both the cell and the
  // sort, so the two can never disagree.
  const serverCountByNode = useMemo(() => {
    const counts = new Map<string, number>();
    for (const node of nodes) {
      let total = 0;
      for (const server of servers) {
        if (server.nodeId === node.id || server.node === node.id || (node.name ? server.node === node.name : false)) total += 1;
      }
      counts.set(node.id, total);
    }
    return counts;
  }, [nodes, servers]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = nodes.filter((n) => !q
      || n.name.toLowerCase().includes(q)
      || (n.fqdn ?? "").toLowerCase().includes(q)
      || (n.displayName ?? "").toLowerCase().includes(q));
    const dir = sortDir === "asc" ? 1 : -1;
    const locationName = (node: ApiNode) => locations.find((l) => l.id === node.locationId)?.short ?? "";
    return [...list].sort((a, b) => {
      switch (sortKey) {
        case "state": return dir * nodeStatus(a).label.localeCompare(nodeStatus(b).label);
        case "heartbeat": return dir * (a.heartbeatState ?? "").localeCompare(b.heartbeatState ?? "");
        case "location": return dir * locationName(a).localeCompare(locationName(b));
        case "memory": return dir * ((a.memoryMb ?? 0) - (b.memoryMb ?? 0));
        case "disk": return dir * ((a.diskMb ?? 0) - (b.diskMb ?? 0));
        case "servers": return dir * ((serverCountByNode.get(a.id) ?? 0) - (serverCountByNode.get(b.id) ?? 0));
        default: return dir * a.name.localeCompare(b.name);
      }
    });
  }, [nodes, search, sortKey, sortDir, locations, serverCountByNode]);

  const toggleSort = (key: NodeSortKey) => {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("asc"); }
  };

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={nodesSource} />}
        info={{
          title: "Nodes",
          triggerLabel: "About Nodes",
          eyebrow: "Architecture & Semantics",
          description: "Beacon hosts, heartbeat status and capacity across the fleet.",
          sections: [
            { title: "Beacon heartbeat & state", content: "Each node runs the Beacon agent. Operational state and persisted heartbeat are shown separately: state is what the node reports, heartbeat is the monitoring evidence. A node we cannot reach reads Unknown, never Offline or Healthy." },
            { title: "Desired vs reported", content: "Memory and Disk in this list are the capacity an operator declared; the node detail workspace shows what the host actually reports. Where the two disagree, the detail view is the measurement and this column is the intent." },
            { title: "Placement", content: "Declared capacity and server counts feed placement scoring. Open a row for the detail workspace (overview, allocations, servers, capabilities)." },
          ],
        }}
        action={
          <Btn tone="primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> Create Node
          </Btn>
        }
      />
      {locationsQuery.isError ? (
        <AdminErrorState message={`Could not load locations: ${locationsQuery.error.message}`} retry={() => void locationsQuery.refetch()} />
      ) : null}
      {serversQuery.isError ? (
        <AdminErrorState message={`Could not load server counts: ${serversQuery.error.message}`} retry={() => void serversQuery.refetch()} />
      ) : null}

      <Card>
        <div className="flex flex-wrap items-end gap-3 p-4">
          <div className="min-w-[220px] flex-1">
            <Input label="Search nodes" placeholder="Name, display name or FQDN" value={search} onChange={setSearch} />
          </div>
          {/* The count is withheld until the query has actually answered, so a
              fleet that has not been read yet cannot read as "0 / 0". */}
          {nodesSource.status === "ready" ? (
            <span className="pb-2 text-xs text-text-subtle" role="status">
              {filtered.length} of {nodes.length} nodes
            </span>
          ) : (
            <span className="pb-2 text-xs text-text-subtle">{nodesSource.status === "error" ? "Count unavailable" : "Count pending"}</span>
          )}
        </div>
        {nodesQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading nodes…" /></div>
        ) : nodesQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message="Nodes could not be loaded from the API." retry={() => void nodesQuery.refetch()} />
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Network} title="No nodes" message={search ? "No nodes match your search." : "Setup required — create a node before hosting workloads."} />
        ) : (
          <AdminTable label="Nodes">
            <AdminTHead>
              <AdminTh><span className="sr-only">Status</span></AdminTh>
              <SortableTh active={sortKey === "name"} dir={sortDir} onClick={() => toggleSort("name")}>Name</SortableTh>
              <SortableTh active={sortKey === "state"} dir={sortDir} onClick={() => toggleSort("state")}>State</SortableTh>
              <SortableTh active={sortKey === "heartbeat"} dir={sortDir} onClick={() => toggleSort("heartbeat")}>Heartbeat</SortableTh>
              <SortableTh active={sortKey === "location"} dir={sortDir} onClick={() => toggleSort("location")}>Location</SortableTh>
              <AdminTh>Region</AdminTh>
              <SortableTh active={sortKey === "memory"} dir={sortDir} onClick={() => toggleSort("memory")}>Memory</SortableTh>
              <SortableTh active={sortKey === "disk"} dir={sortDir} onClick={() => toggleSort("disk")}>Disk</SortableTh>
              <SortableTh active={sortKey === "servers"} dir={sortDir} onClick={() => toggleSort("servers")}>Servers</SortableTh>
              <AdminTh>SSL</AdminTh>
              <AdminTh>Public</AdminTh>
              <AdminTh><span className="sr-only">Open</span></AdminTh>
            </AdminTHead>
            <AdminTBody>
                {filtered.map((node) => (
                  <NodeRow
                    key={node.id}
                    node={node}
                    locations={locations}
                    regions={regions}
                    onClick={() => router.push(`/admin/nodes/${encodeURIComponent(node.id)}`)}
                    onQuick={() => setSelectedNodeId(node.id)}
                    serverCount={serversSource.status === "ready" ? serverCountByNode.get(node.id) ?? 0 : null}
                  />
                ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {selectedNodeId && (
        <NodeDetailView
          nodeId={selectedNodeId}
          onClose={() => setSelectedNodeId(null)}
        />
      )}

      <CreateNodeModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        locations={locations}
        regions={regions}
        locationsError={locationsQuery.isError ? locationsQuery.error : null}
        onRetryLocations={() => void locationsQuery.refetch()}
      />
    </AdminPageLayout>
  );
}

/** Header cell that sorts. The direction is spoken, not just drawn. */
function SortableTh({ children, active, dir, onClick }: {
  children: React.ReactNode;
  active: boolean;
  dir: "asc" | "desc";
  onClick: () => void;
}) {
  return (
    <AdminTh>
      <button
        className="inline-flex items-center gap-1 text-left font-semibold uppercase tracking-wider focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
        onClick={onClick}
        type="button"
      >
        {children}
        <ArrowDownUp
          aria-label={active ? (dir === "asc" ? "sorted ascending" : "sorted descending") : "not sorted"}
          className={active ? "text-brand" : "opacity-30"}
          size={11}
        />
      </button>
    </AdminTh>
  );
}

function NodeRow({ node, locations, regions, onClick, onQuick, serverCount }: {
  node: ApiNode;
  locations: ApiLocation[];
  regions: ApiRegion[];
  onClick: () => void;
  onQuick?: () => void;
  /** `null` when the server inventory has not answered — unknown, not zero. */
  serverCount: number | null;
}) {
  // One verdict from the canonical reader, so the dot, the pill and every other
  // surface in the product say the same word about the same node.
  const verdict = nodeStatus(node);
  // `actualState` is the backend's canonical operational state. Heartbeat is
  // shown separately because it is persisted monitoring evidence, not a probe.
  const actualState = node.actualState ?? "unknown";
  const heartbeatState = node.heartbeatState ?? "unknown";
  const drifted = hasDrift(node);
  const location = locations.find((candidate) => candidate.id === node.locationId);
  const region = regions.find((candidate) => candidate.id === node.regionId);
  // An unset scheme is unknown, not https: defaulting it drew a green lock on a
  // node whose transport nobody had actually chosen.
  const scheme = node.scheme?.trim().toLowerCase() || "";
  const isPublic = node.public ?? node.isPublic;
  return (
    <AdminTr>
      <AdminTd>
        <span className="flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className={cn(
              "inline-block h-2.5 w-2.5 shrink-0 rounded-full",
              toneStyles[verdict.tone].dot,
              verdict.tone === "unknown" && "rounded-full border border-dashed border-unknown-line bg-transparent",
            )}
          />
          <span className="sr-only">{verdict.label}</span>
          {drifted ? (
            <span title={`Desired ${node.desiredState ?? "unknown"}, observed ${node.actualState ?? "unknown"} — the node has not reconciled`}>
              <AlertTriangle aria-hidden="true" className={toneStyles.warn.fg} size={12} />
              <span className="sr-only">Desired and observed state disagree</span>
            </span>
          ) : null}
        </span>
      </AdminTd>
      <AdminTd>
        <div className="flex items-center gap-2">
          {node.maintenanceMode ? <Wrench aria-hidden="true" size={12} className={toneStyles.info.fg} /> : null}
          <button type="button" className="text-left font-semibold hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" onClick={onClick}>{node.name}</button>
        </div>
        {node.fqdn ? <div className="truncate font-mono text-[11px] text-text-muted">{node.fqdn}</div> : null}
      </AdminTd>
      {/* Observed state stays the raw word; the verdict pill above is the
          interpretation. Showing both is the point — they are different
          measurements and merging them into "Healthy" would hide drift. */}
      <AdminTd className="font-mono text-xs capitalize">
        <Pill tone={verdict.tone}>{actualState}</Pill>
      </AdminTd>
      <AdminTd className="font-mono text-xs capitalize text-text-subtle">
        {node.heartbeatState ? heartbeatState : <NotReported reason="No heartbeat recorded for this node" />}
      </AdminTd>
      <AdminTd className="text-text-subtle">
        {location ? <><div>{location.short}</div><div className="text-xs text-text-subtle">{location.long}</div></> : <NotReported reason="No location assigned" />}
      </AdminTd>
      <AdminTd className="text-text-subtle">
        {region ? (
          <><div>{region.name}</div><div className="font-mono text-xs text-text-subtle">{region.slug}</div></>
        ) : node.region ? (
          // A declared region name with no foreign key is what the Regions page
          // cannot count, so it is labelled as the legacy string it is.
          <span title="Free-text region recorded at creation; this node is not linked to a Regions entry, so Regions does not count it.">
            <div>{node.region}</div>
            <div className="text-xs text-text-muted">not linked</div>
          </span>
        ) : <NotReported reason="No region assigned" />}
      </AdminTd>
      <AdminTd className="font-mono text-xs">{mbLabel(node.memoryMb) ?? <NotReported reason="Memory limit not set on this node" />}</AdminTd>
      <AdminTd className="font-mono text-xs">{mbLabel(node.diskMb) ?? <NotReported reason="Disk limit not set on this node" />}</AdminTd>
      <AdminTd className="text-text-subtle">
        {serverCount === null ? <NotReported reason="Server inventory not loaded" /> : serverCount}
      </AdminTd>
      <AdminTd>
        {scheme === "https" ? (
          <span className="flex items-center gap-1.5"><Lock aria-hidden="true" size={14} className={toneStyles.ok.fg} /><span className="sr-only">TLS enabled</span></span>
        ) : scheme === "http" ? (
          <span className="flex items-center gap-1.5"><Unlock aria-hidden="true" size={14} className={toneStyles.warn.fg} /><span className="sr-only">No TLS — plain HTTP</span></span>
        ) : (
          <NotReported reason="No scheme recorded on this node" />
        )}
      </AdminTd>
      <AdminTd>
        {isPublic === undefined ? (
          <NotReported reason="Visibility not reported" />
        ) : isPublic ? (
          <span className="flex items-center gap-1.5"><Eye aria-hidden="true" size={14} className="text-info" /><span className="sr-only">Public</span></span>
        ) : (
          <span className="flex items-center gap-1.5"><EyeOff aria-hidden="true" size={14} className={toneStyles.neutral.fg} /><span className="sr-only">Private</span></span>
        )}
      </AdminTd>
      <AdminTd className="text-right">
        <div className="flex items-center justify-end gap-1">
          {onQuick ? <button type="button" onClick={(e) => { e.stopPropagation(); onQuick(); }} className="rounded px-2 py-1 text-xs text-text-subtle hover:bg-overlay hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Quick</button> : null}
          <ChevronRight aria-hidden="true" size={14} className="text-text-subtle" />
        </div>
      </AdminTd>
    </AdminTr>
  );
}

export function NodeDetailView({ nodeId, onClose }: { nodeId: string; onClose: () => void }) {
  const nodeQuery = useQuery({ queryKey: ["node", nodeId], queryFn: () => fetchNode(nodeId) });
  const { data: node, isLoading } = nodeQuery;
  const allocQuery = useQuery({ queryKey: ["node-allocations", nodeId], queryFn: () => fetchNodeAllocations(nodeId) });
  const allocations = useMemo(() => Array.isArray(allocQuery.data) ? allocQuery.data : [], [allocQuery.data]);
  const [tab, setTab] = useState<Tab>("about");
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const deleteMut = useMutation({
    mutationFn: async () => {
      const result = await deleteNode(nodeId);
      if (!result.ok) throw new Error("The server reported the node was not deleted.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["nodes"] }); onClose(); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete node", message: e.message }),
  });
  const requestDelete = () => {
    void (async () => {
      if (await confirm({ title: `Delete ${node?.name ?? "this node"}?`, description: "This is only allowed after its servers and allocations are removed. This action cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate();
    })();
  };

  if (isLoading) {
    return <Modal title="Node" onClose={onClose}><div className="p-4"><AdminLoadingState label="Loading node…" /></div></Modal>;
  }

  if (nodeQuery.isError || !node) {
    return (
      <Modal title="Node" onClose={onClose}>
        <div className="p-4">
          <AdminErrorState message={nodeQuery.isError ? `Could not load this node: ${nodeQuery.error.message}` : "This node is no longer available."} retry={() => void nodeQuery.refetch()} />
        </div>
      </Modal>
    );
  }

  // The canonical verdict, shared with the list row: maintenance/draining win
  // the label, and a node we have not observed stays Unknown rather than
  // reading "Active" the way this modal's own three-way branch used to.
  const verdict = nodeStatus(node);
  const drifted = hasDrift(node);

  return (
    <Modal title={node.name} description="Heartbeat status, capacity and settings for this host." onClose={onClose} wide className="max-w-6xl">
      <div className="space-y-4">
        {/* Replaces the `DashHeader` hero this modal used to render: that
            component is a sub-heading primitive that also emitted an
            unmeasured pulsing "live" badge and an <h2> competing with the
            dialog title. Same information, no fabricated claim. */}
        <div className="rounded-xl border border-line bg-overlay-subtle p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <Server aria-hidden="true" size={16} className="shrink-0 text-text-subtle" />
                <span className="truncate text-sm font-bold text-text">{node.name}</span>
                <Pill tone={verdict.tone}>{verdict.label}</Pill>
                {drifted ? (
                  <span title={`Desired ${node.desiredState ?? "unknown"}, observed ${node.actualState ?? "unknown"}`}>
                    <Pill tone="warn">Not reconciled</Pill>
                  </span>
                ) : null}
              </div>
              {node.description ? <p className="mt-1 max-w-prose text-xs leading-5 text-text-muted">{node.description}</p> : null}
              <div className="mt-2 flex flex-wrap gap-1.5">
                {[node.schedulerType ?? "docker", node.runtimeProvider].filter((t): t is string => Boolean(t)).map((tag) => (
                  <span className="rounded-md border border-line bg-overlay px-2 py-0.5 font-mono text-[10px] text-text-muted" key={tag}>{tag}</span>
                ))}
              </div>
            </div>
            <Btn tone="danger" size="sm" type="button" disabled={deleteMut.isPending} onClick={requestDelete}>
              <Trash2 size={14} /> {deleteMut.isPending ? "Deleting…" : "Delete Node"}
            </Btn>
          </div>
          <dl className="mt-3 grid grid-cols-2 gap-x-6 gap-y-1.5 text-[11px] sm:grid-cols-4">
            <div className="flex items-center gap-1.5"><dt className="text-text-muted">FQDN</dt><dd className="truncate font-mono font-semibold text-text">{node.fqdn || "not reported"}</dd></div>
            <div className="flex items-center gap-1.5"><dt className="text-text-muted">Daemon ports</dt><dd className="font-mono font-semibold text-text">{node.daemonListen ?? 9090} / {node.daemonSftp ?? 2022}</dd></div>
            <div className="flex items-center gap-1.5"><dt className="text-text-muted">Visibility</dt><dd className="font-semibold text-text">{node.public ?? node.isPublic ? "Public" : "Private"}</dd></div>
            <div className="flex items-center gap-1.5"><dt className="text-text-muted">Declared memory</dt><dd className="font-mono font-semibold text-text">{mbLabel(node.memoryMb) ?? "not set"}</dd></div>
          </dl>
        </div>
        <AdminTabs tabs={ADMIN_TABS} active={tab} onChange={(id) => setTab(id as Tab)} label="Node sections" />
        {tab === "about" && <NodeAboutTab nodeId={nodeId} setTab={setTab} />}
        {tab === "settings" && <NodeSettingsTab node={node} />}
        {tab === "configuration" && <NodeConfigurationTab node={node} />}
        {tab === "allocation" && <NodeAllocationTab node={node} allocations={allocations} />}
        {tab === "servers" && <NodeServersTab nodeId={nodeId} />}
        {tab === "capabilities" && <NodeCapabilitiesTab nodeId={nodeId} />}
      </div>
      {renderConfirm()}
    </Modal>
  );
}

function NodeAboutTab({ nodeId, setTab }: { nodeId: string; setTab: (t: Tab) => void }) {
  const lifecycleQuery = useQuery({
    queryKey: ["node-lifecycle", nodeId],
    queryFn: () => fetchNodeLifecycle(nodeId),
    refetchInterval: 10_000,
  });
  const sysQuery = useQuery({
    queryKey: ["node-sysinfo", nodeId],
    queryFn: () => fetchNodeSystemInformation(nodeId),
    refetchInterval: 10_000,
  });
  const nodeQuery = useQuery({ queryKey: ["node", nodeId], queryFn: () => fetchNode(nodeId) });
  const serversQuery = useQuery<ApiServer[]>({
    queryKey: ["node-servers", nodeId],
    queryFn: () => fetchNodeServers(nodeId),
  });
  const lifecycle = lifecycleQuery.data;
  const sys = sysQuery.data;
  const node = nodeQuery.data;
  const isLifecycleError = lifecycleQuery.isError;
  const isLifecycleLoading = lifecycleQuery.isLoading;
  const filteredServers = useMemo(() => Array.isArray(serversQuery.data) ? serversQuery.data : [], [serversQuery.data]);

  // Freshness, per source. A tile's pulsing "live" marker is now derived from
  // this rather than from `Boolean(cap)` — an object that merely arrived is not
  // a reading that is current, and a stale one must not claim to be.
  const lifecycleSource = sourceState(lifecycleQuery, 10_000);
  const sysSource = sourceState(sysQuery, 10_000);
  const serversSource = sourceState(serversQuery, REFRESH.inventory);
  const lifecycleLive = lifecycleSource.status === "ready" && !lifecycleSource.stale;

  const cap = lifecycle?.capacity;
  // Both halves must be reported before a total exists; see `capacityTotal`.
  const memTotal = capacityTotal(cap?.allocated_memory, cap?.available_memory);
  const memPct = cap && memTotal && typeof cap.allocated_memory === "number" && memTotal > 0
    ? (cap.allocated_memory / memTotal) * 100
    : null;
  const diskTotal = capacityTotal(cap?.allocated_disk, cap?.available_disk);
  const diskPct = cap && diskTotal && typeof cap.allocated_disk === "number" && diskTotal > 0
    ? (cap.allocated_disk / diskTotal) * 100
    : null;
  const score = lifecycle?.healthScore.total;

  const capAge = relativeTime(lifecycleSource.updatedAt);
  const serversLive = serversSource.status === "ready" && !serversSource.stale;
  const kpis: KpiDatum[] = [
    { key: "servers", title: "Servers", icon: Layers, color: chart.sky, iconClass: "text-info", valueClass: "text-info",
      value: serversSource.status === "ready" ? String(filteredServers.length) : null,
      sub: serversSource.status === "loading" ? "Reading inventory…"
        : serversSource.status === "error" ? "Inventory unavailable"
        : `on this node · read ${capAge ?? "just now"}`,
      live: serversLive },
    { key: "memory", title: "Memory allocated", icon: MemoryStick, color: chart.violet, iconClass: "text-text-muted", valueClass: "text-text",
      value: memPct != null ? `${memPct.toFixed(1)}%` : null,
      sub: memPct == null
        ? (isLifecycleLoading ? "Reading capacity…" : "Capacity not reported")
        : `${mbLabel(cap?.allocated_memory) ?? "—"} of ${mbLabel(memTotal) ?? "an unreported total"} · ${lifecycleLive ? `read ${capAge ?? "just now"}` : "read at an unknown time"}`,
      live: lifecycleLive, bar: memPct },
    { key: "disk", title: "Disk allocated", icon: HardDrive, color: chart.lightOrange, iconClass: "text-text-muted", valueClass: "text-text",
      value: diskPct != null ? `${diskPct.toFixed(1)}%` : null,
      sub: diskPct == null
        ? (isLifecycleLoading ? "Reading capacity…" : "Capacity not reported")
        : `${mbLabel(cap?.allocated_disk) ?? "—"} of ${mbLabel(diskTotal) ?? "an unreported total"} · ${lifecycleLive ? `read ${capAge ?? "just now"}` : "read at an unknown time"}`,
      live: lifecycleLive, bar: diskPct },
    { key: "readiness", title: "Readiness", icon: Activity, color: chart.lightEmerald, iconClass: "text-text-muted", valueClass: "text-text",
      value: typeof score === "number" ? `${score}/100` : null,
      sub: lifecycle ? (lifecycle.placementEligible ? "Eligible for placement" : lifecycle.placementBlockedReason ?? "Not eligible")
        : isLifecycleError ? "Lifecycle unavailable"
        : "Reading lifecycle…",
      live: lifecycleLive, bar: typeof score === "number" ? score : null },
  ];

  const quickActions: QuickAction[] = [
    { label: "Servers", hint: "Workloads on node", icon: Layers, onSelect: () => setTab("servers") },
    { label: "Allocations", hint: "Addresses & ports", icon: Network, onSelect: () => setTab("allocation") },
    { label: "Capabilities", hint: "Probes & deltas", icon: Shield, onSelect: () => setTab("capabilities") },
    { label: "Settings", hint: "Name & limits", icon: SettingsIcon, onSelect: () => setTab("settings") },
  ];

  return (
    <div className="space-y-4">
      {serversQuery.isError ? (
        <AdminErrorState message={`Could not load servers on this node: ${serversQuery.error.message}`} retry={() => void serversQuery.refetch()} />
      ) : null}
      <KpiGrid kpis={kpis} />

      <div className="grid gap-4 xl:grid-cols-5">
        <InfoCard wide icon={Activity} title="Information" rows={[
          /* A failed read names itself as a failed read. It used to answer
             "Offline", which is a statement about the machine that nothing on
             this page has actually measured. */
          ["Daemon version", sys?.version ? <span className="font-mono text-text" key="v">{sys.version}</span> : <NotReported key="v" reason={sysSource.status === "error" ? `Host report unavailable: ${sysSource.message ?? "the request failed"}` : sysSource.status === "restricted" ? "Not visible to your account" : "Not read yet"} />],
          ["System", sys?.os || sys?.architecture ? <span className="font-mono text-text" key="sys">{`${sys.os ?? "unknown"} (${sys.architecture ?? "unknown"})`}</span> : <NotReported key="sys" reason="Host has not reported its OS or architecture" />],
          ["CPU threads", <span className="font-mono text-text" key="cpu">{sys?.cpuThreads ?? <NotReported reason="Host has not reported its CPU" />}</span>],
          ["Docker", <span className={cn("font-mono", sys ? (sys.dockerAvailable ? toneStyles.ok.fg : toneStyles.danger.fg) : toneStyles.unknown.fg)} key="docker">{sys ? (sys.dockerAvailable ? "Available" : "Unavailable") : <NotReported reason={sysSource.status === "error" ? "Runtime report unavailable" : "Not read yet"} />}</span>],
          ["FQDN", <span className="font-mono text-text" key="fqdn">{node?.fqdn ?? <NotReported reason="No FQDN recorded" />}</span>],
          ["Runtime / scheduler", <span className="font-mono text-text" key="rt">{node?.runtimeProvider ?? node?.schedulerType ?? <NotReported reason="Neither the host nor the panel has reported a runtime" />}</span>],
          ["Beacon version", <span className="font-mono text-text" key="bv">{sys?.version ?? node?.version ?? <NotReported reason="Not reported" />}</span>],
          ["Last seen", <span className="font-mono text-text" key="seen">{node?.lastSeenAt ? formatDate(node.lastSeenAt) : <NotReported reason="No heartbeat observed yet" />}</span>],
          ["Labels", <span className="font-mono text-xs text-text" key="labels">{node?.labels?.length ? node.labels.map((label) => `${label.key}=${label.value}`).join(", ") : <span className="text-text-muted">none attached</span>}</span>],
          /* `desiredState ?? draining ? … : …` parsed as `(desiredState ??
             draining) ? …` — any non-empty state, including "active", was
             truthy, so every healthy node in this panel read "draining". */
          ["Desired state", <span className="font-mono text-text" key="ds">{node ? nodeStatus(node).label : <NotReported reason="Node record not loaded" />}</span>],
          ["Observed state", <span className="font-mono text-text" key="os2">{node?.actualState ? <span className="capitalize">{node.actualState}</span> : <NotReported reason="Not observed" />}</span>],
          ["Daemon ports", <span className="font-mono text-text" key="ports">{node?.daemonListen ?? 9090} / {node?.daemonSftp ?? 2022}</span>],
          ["Behind proxy", <span className="font-mono text-text" key="proxy">{node ? (node.behindProxy ? "Yes" : "No") : <NotReported reason="Node record not loaded" />}</span>],
          ["Public", <span className="font-mono text-text" key="pub">{node?.public ?? node?.isPublic ? "Yes" : node ? "No" : <NotReported reason="Node record not loaded" />}</span>],
          ["Public hostname", <span className="font-mono text-text" key="ph">{node?.publicHostname || <NotReported reason="No public hostname set" />}</span>],
          ["Display name", <span className="font-mono text-text" key="dn">{node?.displayName || <NotReported reason="No display name set" />}</span>],
          ["Scheduler", <span className="font-mono capitalize text-text" key="sched">{node?.schedulerType ? node.schedulerType : <NotReported reason="Not reported" />}</span>],
          ["Upload limit", <span className="font-mono text-text" key="ul">{node?.uploadSizeMb ? `${node.uploadSizeMb} MiB` : <span className="text-text-muted">panel default</span>}</span>],
          ["Memory overallocation", <span className="font-mono text-text" key="mo">{node?.memoryOverallocate != null ? `${node.memoryOverallocate}%` : <NotReported reason="Not recorded on this node" />}</span>],
          ["Disk overallocation", <span className="font-mono text-text" key="do">{node?.diskOverallocate != null ? `${node.diskOverallocate}%` : <NotReported reason="Not recorded on this node" />}</span>],
          ["CPU overallocation", <span className="font-mono text-text" key="co">{node?.cpuOverallocate != null ? `${node.cpuOverallocate}%` : <NotReported reason="Not recorded on this node" />}</span>],
          ["Tags", <span className="font-mono text-xs text-text" key="tags">{node?.tags?.length ? node.tags.join(", ") : <span className="text-text-muted">none</span>}</span>],
        ]} />

        <div className="rounded-xl border border-line bg-overlay-subtle p-5 xl:col-span-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="flex items-center gap-2 text-sm font-bold text-text"><Activity aria-hidden="true" size={15} className="text-text-subtle" /> Lifecycle</h3>
            <FreshnessBadge state={lifecycleSource} />
          </div>
          <div className={cn("mt-3 rounded-lg border p-3", !lifecycle ? toneStyles.unknown.border : lifecycle.placementEligible ? toneStyles.ok.border : toneStyles.warn.border, !lifecycle ? toneStyles.unknown.bg : lifecycle.placementEligible ? toneStyles.ok.bg : toneStyles.warn.bg)}>
            <p className="flex items-center gap-1.5 text-sm font-bold text-text">
              <span aria-hidden="true" className={cn("h-2 w-2 rounded-full", !lifecycle ? toneStyles.unknown.dot : lifecycle.placementEligible ? toneStyles.ok.dot : toneStyles.warn.dot)} />
              {lifecycle ? (lifecycle.placementEligible ? "Eligible for placement" : "Not eligible") : isLifecycleError ? "Lifecycle unavailable" : "Reading lifecycle…"}
            </p>
            <p className="mt-0.5 font-mono text-[11px] text-text-subtle">
              {lifecycle
                ? (!lifecycle.placementEligible && lifecycle.placementBlockedReason
                    ? lifecycle.placementBlockedReason
                    : typeof lifecycle.healthScore.total === "number" ? `Readiness ${lifecycle.healthScore.total}/100` : "Readiness not scored")
                : isLifecycleError ? (lifecycleSource.message ?? "The host did not answer") : ""}
            </p>
          </div>
          <dl className="mt-2 divide-y divide-line text-xs">
            <div className="flex items-center justify-between gap-3 py-2">
              <dt className="text-text-subtle">Actual state / heartbeat</dt>
              <dd className="font-mono capitalize text-text">{lifecycle ? `${lifecycle.node.actualState ?? "unknown"} / ${lifecycle.node.heartbeatState ?? "unknown"}` : <NotReported reason="Lifecycle not read" />}</dd>
            </div>
            <div className="flex items-center justify-between gap-3 py-2">
              <dt className="text-text-subtle">Placement</dt>
              <dd className={cn("font-mono", lifecycle ? (lifecycle.placementEligible ? toneStyles.ok.fg : toneStyles.warn.fg) : toneStyles.unknown.fg)}>
                {lifecycle ? (lifecycle.placementEligible ? "Eligible" : lifecycle.placementBlockedReason ?? "Not eligible") : "Not reported"}
              </dd>
            </div>
            <div className="flex items-center justify-between gap-3 py-2">
              <dt className="text-text-subtle">Memory allocated / free</dt>
              <dd className="font-mono text-text">{lifecycle ? `${mbLabel(lifecycle.capacity?.allocated_memory) ?? "not reported"} / ${mbLabel(lifecycle.capacity?.available_memory) ?? "not reported"}` : <NotReported reason="Lifecycle not read" />}</dd>
            </div>
            <div className="flex items-center justify-between gap-3 py-2">
              <dt className="text-text-subtle">Disk allocated / free</dt>
              <dd className="font-mono text-text">{lifecycle ? `${mbLabel(lifecycle.capacity?.allocated_disk) ?? "not reported"} / ${mbLabel(lifecycle.capacity?.available_disk) ?? "not reported"}` : <NotReported reason="Lifecycle not read" />}</dd>
            </div>
            <div className="flex items-center justify-between gap-3 py-2">
              <dt className="text-text-subtle">CPU / servers</dt>
              <dd className="font-mono text-text">
                {lifecycle?.capacity ? `${lifecycle.capacity.allocated_cpu} / ${lifecycle.capacity.available_cpu} · ${lifecycle.capacity.server_count ?? "an unreported number"} servers` : <NotReported reason="Lifecycle not read" />}
              </dd>
            </div>
          </dl>
        </div>
      </div>

      {node?.description && (
        <Card>
          <CardHeader title="Description" icon={Mail} />
          <pre className="whitespace-pre-wrap px-4 py-3 text-xs text-text">{node.description}</pre>
        </Card>
      )}

      <QuickActionsCard icon={Zap} title="Quick Actions" actions={quickActions} />
    </div>
  );
}

function NodeSettingsTab({ node }: { node: ApiNode }) {
  const qc = useQueryClient();
  const locationsQuery = useQuery({ queryKey: ["locations"], queryFn: fetchLocations });
  const locations = useMemo(() => Array.isArray(locationsQuery.data) ? locationsQuery.data : [], [locationsQuery.data]);
  const regionsQuery = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });
  const regions = useMemo(() => Array.isArray(regionsQuery.data) ? regionsQuery.data : [], [regionsQuery.data]);
  const region = regions.find((candidate) => candidate.id === node.regionId);
  /** Marks the backend this host actually reported, inside the option itself. */
  const reported = (backend: string) => {
    const actual = (node.runtimeProvider ?? node.schedulerType ?? "").toLowerCase();
    return actual === backend ? " — reported by this host" : "";
  };
  const [name, setName] = useState(node.name);
  const [description, setDescription] = useState(node.description ?? "");
  const [locationId, setLocationId] = useState(node.locationId ?? "");
  const [fqdn, setFqdn] = useState(node.fqdn ?? "");
  const [scheme, setScheme] = useState(node.scheme ?? "https");
  const [behindProxy, setBehindProxy] = useState(node.behindProxy ?? false);
  const [desiredState, setDesiredState] = useState(node.desiredState ?? (node.draining ? "draining" : node.maintenanceMode ? "maintenance" : "active"));
  const [rotatedToken, setRotatedToken] = useState<string | null>(null);
  const [credentialCopied, setCredentialCopied] = useState(false);
  const [credentialMasked, setCredentialMasked] = useState(false);
  const [confirm, renderConfirm] = useConfirm();
  useEffect(() => {
    const hide = () => setCredentialMasked(true);
    const onVisibilityChange = () => { if (document.visibilityState === "hidden") hide(); };
    window.addEventListener("blur", hide);
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      window.removeEventListener("blur", hide);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);
  const [memoryMb, setMemoryMb] = useState(String(node.memoryMb));
  const [diskMb, setDiskMb] = useState(String(node.diskMb));
  const [daemonListen, setDaemonListen] = useState(String(node.daemonListen ?? 9090));
  const [daemonSftp, setDaemonSftp] = useState(String(node.daemonSftp ?? 2022));
  const [schedulerType, setSchedulerType] = useState(node.schedulerType ?? "docker");
  const { toast } = useToast();
  const saveMut = useMutation({
    mutationFn: () => {
      const validationError = validateNodeForm(name, locationId, fqdn, scheme, memoryMb, diskMb, daemonListen, daemonSftp);
      if (validationError) throw new Error(validationError);
      return updateNode(node.id, {
      name,
      description,
      locationId,
      baseUrl: `${scheme}://${fqdn.trim()}`,
      fqdn,
      scheme,
      behindProxy,
      desiredState,
      memoryMb: Number(memoryMb),
      diskMb: Number(diskMb),
      uploadSizeMb: node.uploadSizeMb,
      daemonBase: node.daemonBase,
      daemonListen: Number(daemonListen),
      daemonSftp: Number(daemonSftp),
      schedulerType,
      } as UpdateNodeInput);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["node", node.id] });
      void qc.invalidateQueries({ queryKey: ["nodes"] });
      void qc.invalidateQueries({ queryKey: ["node-lifecycle", node.id] });
      toast({ tone: "success", title: "Node settings saved" });
    },
    onError: (error: Error) => toast({ tone: "error", title: "Failed to save node settings", message: error.message }),
  });

  const rotateMut = useMutation({
    mutationFn: () => rotateNodeToken(node.id),
    onSuccess: (result) => {
      setRotatedToken(result.token);
      void qc.invalidateQueries({ queryKey: ["node", node.id] });
      toast({ tone: "success", title: "Node token rotated" });
    },
    onError: (error: Error) => toast({ tone: "error", title: "Failed to rotate node token", message: error.message }),
  });

  return (
    <form
      className="grid gap-4 md:grid-cols-2"
      onSubmit={(e) => { e.preventDefault(); saveMut.mutate(); }}
    >
      <Card>
        <CardHeader title="Settings" icon={SettingsIcon} />
        <div className="space-y-3 p-4">
          <Input label="Name" value={name} onChange={setName} />
          <Textarea label="Description" value={description} onChange={setDescription} rows={3} />
          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Location</span>
            <select className="h-10 w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 text-[var(--text)]" value={locationId} onChange={(e) => setLocationId(e.target.value)} required disabled={locationsQuery.isPending || locationsQuery.isError}>
              <option value="">Select…</option>
              {locations.map((location) => <option key={location.id} value={location.id}>{location.short} — {location.long}</option>)}
            </select>
            {locationsQuery.isError ? (
              <div className="mt-2 flex items-start justify-between gap-3 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
                <span>Could not load locations: {locationsQuery.error.message}</span>
                <Btn size="sm" tone="ghost" type="button" onClick={() => void locationsQuery.refetch()}>Retry</Btn>
              </div>
            ) : null}
          </label>
          <Input label="FQDN" value={fqdn} onChange={setFqdn} />
          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">SSL</span>
            <select className="h-10 w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 text-[var(--text)]" value={scheme} onChange={(e) => setScheme(e.target.value)}>
              <option value="https">https (SSL)</option>
              <option value="http">http (no SSL)</option>
            </select>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={behindProxy} onChange={(e) => setBehindProxy(e.target.checked)} className="accent-[var(--brand)]" />
            <span>Behind Proxy</span>
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Lifecycle state</span>
            <select className="h-10 w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 text-[var(--text)]" value={desiredState} onChange={(e) => setDesiredState(e.target.value as "active" | "draining" | "maintenance")}>
              <option value="active">Active — eligible when healthy</option>
              <option value="draining">Draining — exclude from placement</option>
              <option value="maintenance">Maintenance — exclude from placement</option>
            </select>
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-text-subtle">Scheduler backend</span>
            <select className="ui-input w-full cursor-pointer" value={schedulerType} onChange={(e) => setSchedulerType(e.target.value)}>
              <option value="docker">Docker{reported("docker")}</option>
              <option value="k3s">K3s (Kubernetes){reported("k3s")}</option>
              <option value="nomad">Nomad (HashiCorp){reported("nomad")}</option>
            </select>
            {/* Capability gating as stated in the product rules: the option is
                never hidden and never silently offered. The node's own reported
                runtime is named beside it, so choosing Nomad on a host that has
                never reported Nomad is a visible decision rather than a default. */}
            <span className="mt-1.5 block text-xs leading-5 text-text-muted">
              {node.runtimeProvider
                ? `This host currently reports the "${node.runtimeProvider}" runtime. Anything else is unverified until the next capability probe.`
                : "This host has not reported a runtime, so no backend choice here is verified against it."}
            </span>
          </label>
          <div className="rounded-lg border border-line bg-overlay-subtle p-3 text-xs leading-5 text-text-muted">
            <p className="font-semibold uppercase tracking-wide text-text-subtle">Region</p>
            <p className="mt-1 text-text">
                {region ? `${region.name} (${region.slug})` : node.region ? `${node.region} — free text, not linked to a region` : "No region assigned"}
            </p>
            <p className="mt-1">
              A node&apos;s region is chosen when it is created and cannot be reassigned from this
              panel; the API has no field for it on update. Create the node again, or change it
              where the control plane supports it.
            </p>
          </div>

        </div>
      </Card>
      <div className="space-y-4">
        <Card>
          <CardHeader title="Resource Limits" icon={Cpu} />
          <div className="space-y-3 p-4">
            <Input label="Memory (MiB)" value={memoryMb} onChange={setMemoryMb} type="number" />
            <Input label="Disk (MiB)" value={diskMb} onChange={setDiskMb} type="number" />
          </div>
        </Card>
        <Card>
          <CardHeader title="Daemon Configuration" icon={Network} />
          <div className="space-y-3 p-4">
            <Input label="Daemon Port" value={daemonListen} onChange={setDaemonListen} type="number" />
            <Input label="Daemon SFTP Port" value={daemonSftp} onChange={setDaemonSftp} type="number" />
          </div>
        </Card>
        <div className="flex justify-between">
          <Btn tone="ghost" onClick={() => { void (async () => { if (await confirm({ title: "Rotate node token?", description: "The current daemon credential will stop working immediately. This action cannot be undone.", danger: true, confirmLabel: "Rotate" })) rotateMut.mutate(); })(); }} type="button">
            <KeyRound size={14} /> Rotate Token
          </Btn>
          <Btn tone="primary" type="submit" disabled={saveMut.isPending || !locationId || locationsQuery.isPending || locationsQuery.isError}>
            {saveMut.isPending ? "Saving…" : "Save"}
          </Btn>
        </div>
      </div>
      {rotatedToken ? <div className={cn("md:col-span-2 rounded-lg border p-4", toneStyles.warn.border, toneStyles.warn.bg)}><p className={cn("text-sm font-semibold", toneStyles.warn.fg)}>New complete credential — shown once</p><pre className="mt-2 overflow-auto rounded bg-overlay-strong p-3 font-mono text-xs text-text">{credentialMasked ? "••••••••••••••••••••••••" : rotatedToken}</pre><div className="mt-3 flex gap-2"><Btn size="sm" tone="ghost" type="button" onClick={() => setCredentialMasked((masked) => !masked)}>{credentialMasked ? <><Eye size={14} /> Reveal credential</> : <><EyeOff size={14} /> Hide credential</>}</Btn><Btn size="sm" tone="ghost" type="button" onClick={async () => { if (await copySecret(rotatedToken)) { setCredentialCopied(true); setTimeout(() => setCredentialCopied(false), 2000); } }}>{credentialCopied ? "Credential copied" : "Copy credential"}</Btn><Btn size="sm" tone="ghost" type="button" onClick={() => setRotatedToken(null)}>I stored it</Btn></div><p className="mt-2 text-xs text-text-muted">Copied credentials are wiped from the clipboard 15s after copying and when this window loses focus.</p></div> : null}
      {renderConfirm()}
    </form>
  );
}

function NodeConfigurationTab({ node }: { node: ApiNode }) {
  const panelURL = getBeaconAPIURL();
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader title="Beacon environment" icon={Globe} />
        <div className="space-y-3 p-4 text-sm text-[var(--text)]">
          <p>Beacon reads its panel connection from environment variables. It does not load the legacy YAML file or support <code>beacon configure</code>.</p>
          <p>Use the full credential shown when this node was created or when its token was rotated. If it was not retained, rotate the token in Settings.</p>
          <pre className="overflow-auto rounded bg-[var(--canvas)] p-4 text-[11px] leading-relaxed text-[var(--text)]">{`# /etc/forge/beacon.env (mode 0600)
APP_ENV=production
DAEMON_NODE_ID=${node.id}
DAEMON_NODE_TOKEN=<token-id>.<secret>
PANEL_API_URL=${panelURL}
DAEMON_ADDR=:${node.daemonListen ?? 9090}
DAEMON_SFTP_ADDR=:${node.daemonSftp ?? 2022}
DAEMON_DATA_DIR=${node.daemonBase ?? "/srv/game-panel/servers"}
DAEMON_ALLOW_INSECURE_NO_AUTH=false

# Restart the beacon systemd service or container after installing this file.`}</pre>
        </div>
      </Card>
    </div>
  );
}

function NodeAllocationTab({ node, allocations }: { node: ApiNode; allocations: ApiAllocation[] }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [aliases, setAliases] = useState<Record<string, string>>({});

  const safeAllocations = useMemo(() => Array.isArray(allocations) ? allocations : [], [allocations]);
  const filtered = useMemo(() =>
    safeAllocations.filter((a) => !filter || a.ip.includes(filter) || a.port.toString().includes(filter)),
    [safeAllocations, filter],
  );
  const deletable = filtered.filter((allocation) => !allocation.server);
  const toggle = (id: string) => {
    const next = new Set(selected);
    if (next.has(id)) next.delete(id); else next.add(id);
    setSelected(next);
  };
  const allFiltered = deletable.length > 0 && deletable.every((allocation) => selected.has(allocation.id));
  const toggleAll = () => {
    const next = new Set(selected);
    if (allFiltered) deletable.forEach((allocation) => next.delete(allocation.id));
    else deletable.forEach((allocation) => next.add(allocation.id));
    setSelected(next);
  };

  const deleteBulkMut = useMutation({
    mutationFn: () => deleteAllocationsBulk(node.id, Array.from(selected)),
    onSuccess: () => {
      const count = selected.size;
      setSelected(new Set());
      void qc.invalidateQueries({ queryKey: ["node-allocations", node.id] });
      toast({ tone: "success", title: `${count} allocation${count === 1 ? "" : "s"} deleted` });
    },
    onError: (error: Error) => toast({ tone: "error", title: "Failed to delete allocations", message: error.message }),
  });
  const setAliasMut = useMutation({
    mutationFn: ({ id, alias }: { id: string; alias: string }) => setAllocationAlias(node.id, id, alias),
    onSuccess: (_, { id, alias }) => {
      setAliases((current) => ({ ...current, [id]: alias }));
      void qc.invalidateQueries({ queryKey: ["node-allocations", node.id] });
      toast({ tone: "success", title: alias ? "Allocation alias updated" : "Allocation alias cleared" });
    },
    onError: (error: Error, { id }) => {
      const allocation = allocations.find((candidate) => candidate.id === id);
      setAliases((current) => ({ ...current, [id]: allocation?.alias ?? "" }));
      toast({ tone: "error", title: "Failed to update allocation alias", message: error.message });
    },
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Input placeholder="Filter IP or port" value={filter} onChange={setFilter} />
        {selected.size > 0 && (
          <Btn tone="danger" disabled={deleteBulkMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Delete ${selected.size} allocation${selected.size === 1 ? "" : "s"}?`, description: "Free allocations only — allocations attached to a server are excluded. This action cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteBulkMut.mutate(); })(); }}>
            <Trash2 size={14} /> {deleteBulkMut.isPending ? "Deleting…" : `Delete ${selected.size}`}
          </Btn>
        )}
      </div>
      <Card>
        <AdminTable label="Allocations">
          <AdminTHead>
            <AdminTh>
                <input type="checkbox" checked={allFiltered} onChange={toggleAll} disabled={deletable.length === 0 || deleteBulkMut.isPending} className="accent-[var(--brand)]" />
              </AdminTh>
              <AdminTh>IP</AdminTh>
              <AdminTh>Alias</AdminTh>
              <AdminTh>Port</AdminTh>
              <AdminTh>Server</AdminTh>
          </AdminTHead>
          <AdminTBody>
            {filtered.map((a) => (
              <AdminTr key={a.id}>
                <AdminTd>
                  <input type="checkbox" disabled={!!a.server || deleteBulkMut.isPending} checked={selected.has(a.id)} onChange={() => toggle(a.id)} className="accent-[var(--brand)]" />
                </AdminTd>
                <AdminTd className="font-mono text-xs">{a.ip}</AdminTd>
                <AdminTd>
                  <input
                    className="h-8 w-32 rounded border border-[var(--line)] bg-[var(--surface)] px-2 text-xs disabled:cursor-not-allowed disabled:opacity-60"
                    value={aliases[a.id] ?? a.alias ?? ""}
                    disabled={setAliasMut.isPending}
                    onChange={(e) => setAliases((current) => ({ ...current, [a.id]: e.target.value }))}
                    onBlur={(e) => {
                      const alias = e.target.value.trim();
                      if ((a.alias ?? "") !== alias) setAliasMut.mutate({ id: a.id, alias });
                    }}
                  />
                </AdminTd>
                <AdminTd className="font-mono text-xs">{a.port}</AdminTd>
                <AdminTd className="text-[var(--text-subtle)]">{a.server ?? "—"}</AdminTd>
              </AdminTr>
            ))}
          </AdminTBody>
        </AdminTable>
      </Card>
      {renderConfirm()}
    </div>
  );
}

function NodeServersTab({ nodeId }: { nodeId: string }) {
  const serversQuery = useQuery<ApiServer[]>({
    queryKey: ["node-servers-list", nodeId],
    queryFn: () => fetchNodeServers(nodeId),
  });
  const filtered = useMemo(() => Array.isArray(serversQuery.data) ? serversQuery.data : [], [serversQuery.data]);
  return (
    <Card>
      <CardHeader title={`Servers (${filtered.length})`} icon={Database} />
      {serversQuery.isError ? (
        <div className="p-4">
          <AdminErrorState message={`Could not load servers on this node: ${serversQuery.error.message}`} retry={() => void serversQuery.refetch()} />
        </div>
      ) : filtered.length === 0 ? (
        <EmptyState icon={Database} title="No servers" message="No servers on this node." />
      ) : (
        <AdminTable label="Node servers">
          <AdminTHead>
            <AdminTh>Name</AdminTh>
            <AdminTh>UUID</AdminTh>
            <AdminTh>Status</AdminTh>
          </AdminTHead>
          <AdminTBody>
            {filtered.map((s) => (
              <AdminTr key={s.id}>
                <AdminTd className="font-semibold">{s.name}</AdminTd>
                <AdminTd className="font-mono text-xs text-[var(--text-subtle)]">{s.id.slice(0, 8)}…</AdminTd>
                <AdminTd>{s.status}</AdminTd>
              </AdminTr>
            ))}
          </AdminTBody>
        </AdminTable>
      )}
    </Card>
  );
}

function NodeCapabilitiesTab({ nodeId }: { nodeId: string }) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const capQ = useQuery({
    queryKey: ["node-capability", nodeId],
    queryFn: () => fetchCapability(nodeId),
    retry: false,
  });
  const histQ = useQuery({
    queryKey: ["node-capability-history", nodeId],
    queryFn: () => fetchCapabilityHistory(nodeId, 10),
    retry: false,
  });
  const deltaQ = useQuery({
    queryKey: ["node-capability-delta", nodeId],
    queryFn: () => fetchCapabilityDelta(nodeId),
    retry: false,
  });
  const probeMut = useMutation({
    mutationFn: () => probeCapabilities(nodeId),
    onSuccess: (data) => {
      if (!data.online) toast({ tone: "error", title: "Node offline", message: (data as { error?: string }).error ?? "beacon unreachable" });
      else toast({ tone: "success", title: "Probe succeeded", message: `Capabilities refreshed @ ${new Date().toLocaleTimeString()}` });
      void qc.invalidateQueries({ queryKey: ["node-capability"] });
      void qc.invalidateQueries({ queryKey: ["node-capability-history"] });
      void qc.invalidateQueries({ queryKey: ["node-capability-delta"] });
      void qc.invalidateQueries({ queryKey: ["admin-capabilities-global"] });
      void deltaQ.refetch();
      void histQ.refetch();
      void capQ.refetch();
    },
    onError: (e: Error) => toast({ tone: "error", title: "Probe failed", message: e.message }),
  });
  const d = deltaQ.data;
  const drifted = d ? d.added.length > 0 || d.removed.length > 0 || d.changed.length > 0 : false;
  /* The server returns three empty buckets when the two newest snapshots both
     unmarshal to no entries at all, so "stable" requires a positive reading —
     an empty comparison is unknown, not an all-clear. */
  const stable = drifted === false && (d?.unchanged.length ?? 0) > 0;

  return (
    <div className="space-y-4">
      {drifted ? (
        <div className={cn("flex items-start gap-2 rounded-lg border p-3 text-sm", toneStyles.warn.chip)} role="alert">
          <AlertTriangle size={14} className={cn("mt-0.5 shrink-0", toneStyles.warn.fg)} />
          <span>Capability drift detected — {d?.added.length ?? 0} added · {d?.removed.length ?? 0} removed · {d?.changed.length ?? 0} changed since last snapshot. Use Probe to refresh or compare the two newest history rows.</span>
        </div>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-text-subtle">Per-node capability detail, history and drift for this host.</p>
        <Btn size="sm" tone="primary" loading={probeMut.isPending} onClick={() => probeMut.mutate()}>
          <Zap size={12} /> Probe live
        </Btn>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="border border-[var(--line)] bg-[var(--surface)]">
          <CardHeader title="Current snapshot" icon={Layers} action={<Btn size="sm" tone="ghost" onClick={() => void capQ.refetch()}>Reload</Btn>} />
          {capQ.isLoading ? (
            <div className="p-4"><AdminLoadingState label="Loading capability…" /></div>
          ) : capQ.isError ? (
            <div className="p-4"><AdminErrorState message={(capQ.error as Error).message} retry={() => void capQ.refetch()} /></div>
          ) : capQ.data ? (
            <div className="space-y-3 p-4">
              <div className="grid gap-2 text-xs">
                <div className="flex justify-between"><span className="text-[var(--text-subtle)]">Beacon</span><span className="font-mono text-[var(--text)]">{capQ.data.beaconVersion || "—"}</span></div>
                <div className="flex justify-between"><span className="text-[var(--text-subtle)]">OS / Arch</span><span className="font-mono text-[var(--text)]">{capQ.data.os} / {capQ.data.architecture}</span></div>
                <div className="flex justify-between"><span className="text-[var(--text-subtle)]">CPU / Memory</span><span className="font-mono text-[var(--text)]">{capQ.data.cpuThreads} threads · {capQ.data.memoryMb} MiB</span></div>
                <div className="flex justify-between"><span className="text-[var(--text-subtle)]">Runtime</span><span className={cn("font-mono", capQ.data.runtimeAvailable ? toneStyles.ok.fg : toneStyles.danger.fg)}>{capQ.data.runtimeAvailable ? capQ.data.runtimeStatus || "available" : "unavailable"}</span></div>
                <div className="flex justify-between"><span className="text-[var(--text-subtle)]">Fetched</span><span className="font-mono text-[var(--text)]">{capQ.data.fetchedAt ? `${formatDate(capQ.data.fetchedAt)} · ${relativeTime(Date.parse(capQ.data.fetchedAt)) ?? "age unknown"}` : <NotReported reason="Snapshot has no timestamp" />}</span></div>
              </div>
              <div className="flex flex-wrap gap-1">
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
              <p className="text-[11px] font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Raw report (diagnostic)</p>
              <pre className="max-h-40 overflow-auto rounded bg-overlay-subtle p-2 font-mono text-[11px] leading-5 text-[var(--text-subtle)]">{JSON.stringify(capQ.data.rawReport ?? capQ.data, null, 2)}</pre>
            </div>
          ) : (
            <div className="p-4 text-sm text-[var(--text-subtle)]">No capability snapshot yet — probe the node or wait for heartbeat.</div>
          )}
        </Card>

        <Card className="border border-[var(--line)] bg-[var(--surface)]">
          <CardHeader title="Delta" icon={GitCompare} action={<Btn size="sm" tone="ghost" onClick={() => void deltaQ.refetch()}>Recompute</Btn>} />
          {deltaQ.isLoading ? (
            <div className="p-4"><AdminLoadingState label="Computing delta…" /></div>
          ) : deltaQ.isError ? (
            <div className="p-4"><AdminErrorState message={(deltaQ.error as Error).message} retry={() => void deltaQ.refetch()} /></div>
          ) : d ? (
            <div className="space-y-3 p-4">
              <div className="flex flex-wrap gap-2 text-xs">
                <Pill tone="green">+ {d.added.length} added</Pill>
                <Pill tone="red">− {d.removed.length} removed</Pill>
                <Pill tone="yellow">~ {d.changed.length} changed</Pill>
                <Pill tone="neutral">= {d.unchanged.length} unchanged</Pill>
                <span className="ml-auto font-mono text-[11px] text-[var(--text-subtle)]">fetchedAt {d.fetchedAt ? new Date(d.fetchedAt).toLocaleString() : "—"}</span>
              </div>
              {drifted ? null : stable ? (
                <div className={cn("rounded-lg border p-3 text-sm", toneStyles.ok.chip)}>No drift — {d?.unchanged.length} capabilities match the previous snapshot.</div>
              ) : (
                <div className={cn("rounded-lg border border-dashed p-3 text-sm", toneStyles.unknown.chip)}>
                  Nothing to compare — neither snapshot carried capability entries, so stability is unknown rather than confirmed.
                </div>
              )}
              <div className="grid gap-2">
                <DeltaSection title="Added" items={d.added} tone="green" />
                <DeltaSection title="Removed" items={d.removed} tone="red" />
                <DeltaSection title="Changed" items={d.changed} tone="yellow" />
                <DeltaSection title="Unchanged" items={d.unchanged} tone="neutral" />
              </div>
              <p className="text-xs leading-5 text-[var(--text-subtle)]">
                Drift is derived from <code className="font-mono text-[11px]">node_capability_history</code> (newest 2 rows). When only one snapshot exists the delta reports everything as <code className="font-mono">added</code> — the honest “no baseline” signal.
              </p>
            </div>
          ) : (
            <div className="p-4 text-sm text-[var(--text-subtle)]">No delta yet — probe or wait for a second snapshot.</div>
          )}
        </Card>
      </div>

      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title="History" icon={History} action={<Btn size="sm" tone="ghost" onClick={() => void histQ.refetch()}>Reload</Btn>} />
        {histQ.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading history…" /></div>
        ) : histQ.isError ? (
          <div className="p-4"><AdminErrorState message={(histQ.error as Error).message} retry={() => void histQ.refetch()} /></div>
        ) : (histQ.data?.length ?? 0) === 0 ? (
          <div className="p-4 text-sm text-[var(--text-subtle)]">No history — probe or wait for heartbeat to generate snapshots.</div>
        ) : (
          <ol className="space-y-2 p-4">
            {(histQ.data ?? []).map((h) => {
              const text = JSON.stringify(h.capabilities, null, 2) ?? "";
              const shown = text.slice(0, 600);
              return (
                <li className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2" key={h.id}>
                  <div className="flex items-center gap-2 font-mono text-xs">
                    <span className="font-medium text-[var(--text)]">{formatDate(h.observedAt)}</span>
                    <span className="text-[var(--text-subtle)]">· {h.beaconVersion || "no version"}</span>
                  </div>
                  <pre className="mt-1 overflow-auto font-mono text-[11px] leading-5 text-[var(--text-subtle)]">{shown}{shown.length < text.length ? `\n… truncated, ${text.length - shown.length} characters not shown` : ""}</pre>
                </li>
              );
            })}
          </ol>
        )}
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
      <div className="text-[11px] font-bold uppercase tracking-widest text-[var(--text-subtle)]">{title} · {items.length}</div>
      {items.length === 0 ? <div className="mt-2 text-xs text-[var(--text-subtle)]">none in this bucket</div> : (
        <ul className="mt-2 space-y-1">
          {items.map((it, i) => {
            const text = JSON.stringify(it, null, 2);
            const shown = text.slice(0, 400);
            return (
              <li key={i} className="rounded border border-[var(--line)] bg-overlay-subtle px-2 py-1 font-mono text-[11px] leading-5 text-[var(--text-subtle)]">
                {shown}
                {/* Say so when the dump is cut: a truncated entry must not read
                    as a complete one. */}
                {shown.length < text.length ? <span className="mt-0.5 block text-info">… truncated, {text.length - shown.length} characters not shown</span> : null}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function CreateNodeModal({ open, onClose, locations, regions, locationsError, onRetryLocations }: {
  open: boolean;
  onClose: () => void;
  locations: ApiLocation[];
  regions: ApiRegion[];
  locationsError: Error | null;
  onRetryLocations: () => void;
}) {
  const qc = useQueryClient();

  // — Basic Details
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [locationId, setLocationId] = useState("");
  const [regionId, setRegionId] = useState("");
  const [publicNode, setPublicNode] = useState(true);

  // — Network
  const [fqdn, setFqdn] = useState("");
  const [scheme, setScheme] = useState("https");
  const [behindProxy, setBehindProxy] = useState(false);
  const [publicHostname, setPublicHostname] = useState("");
  const [allowedIps, setAllowedIps] = useState("");
  const [networkInterface, setNetworkInterface] = useState("");

  // — Resource Limits
  const [memoryMb, setMemoryMb] = useState("0");
  const [diskMb, setDiskMb] = useState("0");
  const [memoryOverallocate, setMemoryOverallocate] = useState("0");
  const [diskOverallocate, setDiskOverallocate] = useState("0");
  const [cpuOverallocate, setCpuOverallocate] = useState("0");
  const [uploadSizeMb, setUploadSizeMb] = useState("100");
  const [reservedMemoryMb, setReservedMemoryMb] = useState("0");
  const [reservedDiskMb, setReservedDiskMb] = useState("0");

  // — Daemon
  const [daemonBase, setDaemonBase] = useState("/var/lib/beacon/servers");
  const [daemonListen, setDaemonListen] = useState("9090");
  const [daemonSftp, setDaemonSftp] = useState("2022");
  const [daemonSftpAlias, setDaemonSftpAlias] = useState("");
  const [daemonConnect, setDaemonConnect] = useState("8080");

  // — Allocation
  const [defaultAllocationIp, setDefaultAllocationIp] = useState("0.0.0.0");
  const [allocationPortMin, setAllocationPortMin] = useState("25565");
  const [allocationPortMax, setAllocationPortMax] = useState("26565");
  const [autoAllocate, setAutoAllocate] = useState(false);

  // — Scheduler
  const [schedulerType, setSchedulerType] = useState("docker");

  // — Monitoring
  const [enableHealthChecks, setEnableHealthChecks] = useState(true);
  const [enableMetrics, setEnableMetrics] = useState(true);
  const [prometheusEndpoint, setPrometheusEndpoint] = useState("");
  const [alertThresholdCpu, setAlertThresholdCpu] = useState("90");
  const [alertThresholdMemory, setAlertThresholdMemory] = useState("90");
  const [alertThresholdDisk, setAlertThresholdDisk] = useState("90");

  // — Maintenance
  const [maintenanceMode, setMaintenanceMode] = useState(false);
  const [maintenanceMessage, setMaintenanceMessage] = useState("");
  const [drainBeforeMaintenance, setDrainBeforeMaintenance] = useState(false);

  // — Security
  const [tokenRotationPolicy, setTokenRotationPolicy] = useState("manual");
  const [tlsSetting, setTlsSetting] = useState("auto");
  const [tags, setTags] = useState("");

  const [onboarding, setOnboarding] = useState<{ id: string; token: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [credentialMasked, setCredentialMasked] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const panelURL = getBeaconAPIURL();

  useEffect(() => {
    const hide = () => setCredentialMasked(true);
    const onVisibilityChange = () => { if (document.visibilityState === "hidden") hide(); };
    window.addEventListener("blur", hide);
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      window.removeEventListener("blur", hide);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);

  const createMut = useMutation({
    mutationFn: () => {
      const validationError = validateNodeForm(name, locationId, fqdn, scheme, memoryMb, diskMb, daemonListen, daemonSftp);
      if (validationError) throw new Error(validationError);
      const location = locations.find((candidate) => candidate.id === locationId);
      if (!location) throw new Error("Select a valid location before creating the node.");
      const region = regions.find((candidate) => candidate.id === regionId);
      if (regionId && !region) throw new Error("Select a valid region, or leave the region unset.");
      const tagsArr = tags.split(",").map((t) => t.trim()).filter(Boolean);
      const allowedIpsArr = allowedIps.split(",").map((t) => t.trim()).filter(Boolean);
      return createNode({
        name: name.trim(),
        // `region` is the legacy free-text column the node list still shows;
        // `regionId` is the foreign key the Regions page counts and guards
        // deletion on. Both are written now, so the two pages stop disagreeing
        // about where a node lives.
        region: region ? region.name : location.short,
        regionId: region?.id,
        locationId: location.id,
        description: description.trim(),
        displayName: displayName.trim() || undefined,
        public: publicNode,
        baseUrl: `${scheme}://${fqdn.trim()}`,
        fqdn: fqdn.trim(),
        scheme,
        behindProxy,
        publicHostname: publicHostname.trim() || undefined,
        allowedIps: allowedIpsArr.length > 0 ? allowedIpsArr : undefined,
        networkInterface: networkInterface.trim() || undefined,
        memoryMb: Number(memoryMb),
        diskMb: Number(diskMb),
        memoryOverallocate: Number(memoryOverallocate),
        diskOverallocate: Number(diskOverallocate),
        cpuOverallocate: Number(cpuOverallocate),
        uploadSizeMb: Number(uploadSizeMb),
        reservedMemoryMb: Number(reservedMemoryMb),
        reservedDiskMb: Number(reservedDiskMb),
        daemonBase,
        daemonListen: Number(daemonListen),
        daemonSftp: Number(daemonSftp),
        daemonSftpAlias: daemonSftpAlias.trim() || undefined,
        daemonConnect: Number(daemonConnect),
        defaultAllocationIp: defaultAllocationIp.trim() || undefined,
        allocationPortMin: Number(allocationPortMin),
        allocationPortMax: Number(allocationPortMax),
        autoAllocate,
        schedulerType,
        enableHealthChecks,
        enableMetrics,
        prometheusEndpoint: prometheusEndpoint.trim() || undefined,
        alertThresholdCpu: Number(alertThresholdCpu),
        alertThresholdMemory: Number(alertThresholdMemory),
        alertThresholdDisk: Number(alertThresholdDisk),
        maintenanceMode,
        maintenanceMessage: maintenanceMessage.trim() || undefined,
        drainBeforeMaintenance,
        tokenRotationPolicy,
        tlsSetting,
        tags: tagsArr.length > 0 ? tagsArr : undefined,
      } as CreateNodeInput);
    },
    onSuccess: ({ node, token }) => { qc.invalidateQueries({ queryKey: ["nodes"] }); setOnboarding({ id: node.id, token }); setCreateError(null); },
    onError: (e: Error) => { console.error("Failed to create node:", e); setCreateError(e.message || "Unknown error"); },
  });

  if (!open) return null;
  return (
    <Modal title="New Node" onClose={onClose} className="max-w-6xl">
      {onboarding ? (
        <div className="space-y-4">
          <div className={cn("rounded-lg border p-4 text-sm", toneStyles.warn.chip)}>Save this credential now. Forge will not show it again; rotate the token if it is lost. Revealed values are hidden automatically when this window loses focus.</div>
          <pre className="overflow-auto rounded bg-[var(--canvas)] p-4 font-mono text-xs leading-relaxed text-text">{`# /etc/forge/beacon.env (mode 0600)
APP_ENV=production
DAEMON_NODE_ID=${onboarding.id}
DAEMON_NODE_TOKEN=${credentialMasked ? "••••••••••••••••" : onboarding.token}
PANEL_API_URL=${panelURL}
DAEMON_ADDR=:${daemonListen}
DAEMON_SFTP_ADDR=:${daemonSftp}
DAEMON_DATA_DIR=${daemonBase}
DAEMON_ALLOW_INSECURE_NO_AUTH=false

# Configure Beacon's systemd EnvironmentFile= or container env_file, then restart Beacon.`}</pre>
          <div className="flex justify-end gap-2">
            <Btn tone="ghost" onClick={() => setCredentialMasked((masked) => !masked)}>{credentialMasked ? <><Eye size={14} /> Reveal credential</> : <><EyeOff size={14} /> Hide credential</>}</Btn>
            <Btn tone="ghost" onClick={async () => { if (await copySecret(onboarding.token)) { setCopied(true); setTimeout(() => setCopied(false), 2000); } }}>{copied ? "Credential copied" : "Copy credential"}</Btn>
            <Btn tone="primary" onClick={onClose}>I stored this credential</Btn>
          </div>
        </div>
      ) : <form
        className="space-y-4"
        onSubmit={(e) => { e.preventDefault(); createMut.mutate(); }}
      >
        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader title="Basic Details" icon={Shield} />
            <div className="space-y-4 p-5">
              <Input label="Name" value={name} onChange={setName} placeholder="nyc-dal-01" required />
              <Input label="Display Name" value={displayName} onChange={setDisplayName} placeholder="NYC Dallas Node 1" />
              <Textarea label="Description" value={description} onChange={setDescription} rows={2} placeholder="Optional description for this node" />
              <label className="block text-sm">
                <span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Location</span>
                <select className="h-10 w-full cursor-pointer rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" value={locationId} onChange={(e) => setLocationId(e.target.value)} required disabled={locations.length === 0 || locationsError !== null}>
                  <option value="">Select…</option>
                  {locations.map((location) => <option key={location.id} value={location.id}>{location.short} — {location.long}</option>)}
                </select>
                {locationsError ? (
                  <div className="mt-2"><AdminErrorState message={`Could not load locations: ${locationsError.message}`} retry={onRetryLocations} /></div>
                ) : locations.length === 0 ? <p className="mt-1 text-xs text-warn">Create a location first before adding a node.</p> : null}
              </label>
              {/* Region used to be synthesised from the location code, so the
                  Regions page counted zero nodes for a region the Nodes page
                  displayed, and its delete guard let a "populated" region be
                  removed. The link is chosen explicitly here or not at all. */}
              <label className="block text-sm">
                <span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Region</span>
                <select className="h-10 w-full cursor-pointer rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" value={regionId} onChange={(e) => setRegionId(e.target.value)}>
                  <option value="">No region</option>
                  {regions.map((region) => <option key={region.id} value={region.id}>{region.name} ({region.slug})</option>)}
                </select>
                <span className="mt-1.5 block text-xs leading-5 text-text-muted">
                  {regions.length === 0
                    ? "No regions exist yet, so this node cannot be linked to one; the Regions page will not count it."
                    : regionId
                      ? "Links this node to a region: it will be counted there and can only be scheduled with it."
                      : "Without a region this node is not counted by the Regions page and cannot be placed by region."}
                </span>
              </label>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-[var(--text)]">
                <input type="checkbox" checked={publicNode} onChange={(e) => setPublicNode(e.target.checked)} className="h-4 w-4 accent-[var(--brand)]" />
                <span>Public node</span>
              </label>
            </div>
          </Card>

          <Card>
            <CardHeader title="Network" icon={Globe} />
            <div className="space-y-4 p-5">
              <Input label="FQDN" value={fqdn} onChange={setFqdn} placeholder="node1.example.com" required />
              <Input label="Public Hostname" value={publicHostname} onChange={setPublicHostname} placeholder="Optional public-facing hostname" />
              <label className="block text-sm">
                <span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">SSL</span>
                <select className="h-10 w-full cursor-pointer rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" value={scheme} onChange={(e) => setScheme(e.target.value)}>
                  <option value="https">https</option>
                  <option value="http">http</option>
                </select>
              </label>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-[var(--text)]">
                <input type="checkbox" checked={behindProxy} onChange={(e) => setBehindProxy(e.target.checked)} className="h-4 w-4 accent-[var(--brand)]" />
                <span>Behind Proxy</span>
              </label>
              <Input label="Allowed IPs" value={allowedIps} onChange={setAllowedIps} placeholder="Comma-separated, e.g. 10.0.0.0/8, 192.168.1.0/24" />
              <Input label="Network Interface" value={networkInterface} onChange={setNetworkInterface} placeholder="e.g. eth0, bond0" />
            </div>
          </Card>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader title="Resource Limits" icon={Cpu} />
            <div className="space-y-4 p-5">
              <div className="grid grid-cols-2 gap-4">
                <Input label="Total Memory (MiB)" value={memoryMb} onChange={setMemoryMb} type="number" />
                <Input label="Total Disk (MiB)" value={diskMb} onChange={setDiskMb} type="number" />
              </div>
              <div className="grid grid-cols-3 gap-4">
                <Input label="Memory Overalloc. %" value={memoryOverallocate} onChange={setMemoryOverallocate} type="number" />
                <Input label="Disk Overalloc. %" value={diskOverallocate} onChange={setDiskOverallocate} type="number" />
                <Input label="CPU Overalloc. %" value={cpuOverallocate} onChange={setCpuOverallocate} type="number" />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <Input label="Upload Size (MiB)" value={uploadSizeMb} onChange={setUploadSizeMb} type="number" />
                <Input label="Reserved Memory (MiB)" value={reservedMemoryMb} onChange={setReservedMemoryMb} type="number" />
              </div>
              <Input label="Reserved Disk (MiB)" value={reservedDiskMb} onChange={setReservedDiskMb} type="number" />
            </div>
          </Card>

          <Card>
            <CardHeader title="Daemon" icon={Wrench} />
            <div className="space-y-4 p-5">
              <Input label="Server File Directory" value={daemonBase} onChange={setDaemonBase} />
              <div className="grid grid-cols-2 gap-4">
                <Input label="Daemon Port" value={daemonListen} onChange={setDaemonListen} type="number" />
                <Input label="SFTP Port" value={daemonSftp} onChange={setDaemonSftp} type="number" />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <Input label="SFTP Alias" value={daemonSftpAlias} onChange={setDaemonSftpAlias} placeholder="Optional SFTP hostname alias" />
                <Input label="Connect Port" value={daemonConnect} onChange={setDaemonConnect} type="number" />
              </div>
            </div>
          </Card>
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader title="Allocation" icon={Network} />
            <div className="space-y-4 p-5">
              <Input label="Default IP" value={defaultAllocationIp} onChange={setDefaultAllocationIp} />
              <div className="grid grid-cols-2 gap-4">
                <Input label="Port Min" value={allocationPortMin} onChange={setAllocationPortMin} type="number" />
                <Input label="Port Max" value={allocationPortMax} onChange={setAllocationPortMax} type="number" />
              </div>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-[var(--text)]">
                <input type="checkbox" checked={autoAllocate} onChange={(e) => setAutoAllocate(e.target.checked)} className="h-4 w-4 accent-[var(--brand)]" />
                <span>Auto-allocate ports</span>
              </label>
            </div>
          </Card>

          <Card>
            <CardHeader title="Scheduler" icon={SettingsIcon} />
            <div className="space-y-4 p-5">
              <label className="block text-sm">
                <span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Backend</span>
                <select className="h-10 w-full cursor-pointer rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" value={schedulerType} onChange={(e) => setSchedulerType(e.target.value)}>
                  <option value="docker">Docker</option>
                  <option value="k3s">K3s (Kubernetes)</option>
                  <option value="nomad">Nomad (HashiCorp)</option>
                </select>
                {/* A node that has never reported cannot be gated on a capability
                    it has not stated — so the choice is offered with the reason
                    it is unverified, rather than looking authoritative. */}
                <span className="mt-1.5 block text-xs leading-5 text-text-muted">
                  A new node has not reported a runtime yet, so nothing here is verified against it.
                  Docker is the only path confirmed end to end; K3s and Nomad stay unverified until the
                  first capability probe, which you can read in the node&apos;s Capabilities tab.
                </span>
              </label>
            </div>
          </Card>

          <Card>
            <CardHeader title="Tags" icon={Activity} />
            <div className="space-y-4 p-5">
              <Input label="Tags" value={tags} onChange={setTags} placeholder="ssd, gpu, low-latency" />
              <p className="text-xs text-[var(--text-subtle)]">Tags let you filter and group nodes for scheduling constraints.</p>
            </div>
          </Card>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader title="Monitoring & Alerts" icon={Activity} />
            <div className="space-y-4 p-5">
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-[var(--text)]">
                <input type="checkbox" checked={enableHealthChecks} onChange={(e) => setEnableHealthChecks(e.target.checked)} className="h-4 w-4 accent-[var(--brand)]" />
                <span>Enable health checks</span>
              </label>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-[var(--text)]">
                <input type="checkbox" checked={enableMetrics} onChange={(e) => setEnableMetrics(e.target.checked)} className="h-4 w-4 accent-[var(--brand)]" />
                <span>Enable metrics collection</span>
              </label>
              <Input label="Prometheus Endpoint" value={prometheusEndpoint} onChange={setPrometheusEndpoint} placeholder="Optional Prometheus scrape URL" />
              <div>
                <p className="mb-3 text-xs font-medium text-[var(--text-subtle)]">Alert thresholds (0–100%)</p>
                <div className="grid grid-cols-3 gap-4">
                  <Input label="CPU %" value={alertThresholdCpu} onChange={setAlertThresholdCpu} type="number" />
                  <Input label="Memory %" value={alertThresholdMemory} onChange={setAlertThresholdMemory} type="number" />
                  <Input label="Disk %" value={alertThresholdDisk} onChange={setAlertThresholdDisk} type="number" />
                </div>
              </div>
              {/* Stated plainly rather than implied: these are stored on the
                  node record, but the node detail views never read them back and
                  Settings has no field for them, so nothing here can be reviewed
                  or changed after the node exists. */}
              <p className="text-xs leading-5 text-warn">
                These monitoring settings are recorded with the node and cannot be shown or edited
                anywhere in this panel afterwards — the node views do not read them back.
              </p>
            </div>
          </Card>

          <Card>
            <CardHeader title="Maintenance & Security" icon={Lock} />
            <div className="space-y-4 p-5">
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-[var(--text)]">
                <input type="checkbox" checked={maintenanceMode} onChange={(e) => setMaintenanceMode(e.target.checked)} className="h-4 w-4 accent-[var(--brand)]" />
                <span>Maintenance mode</span>
              </label>
              {maintenanceMode && (
                <div className="space-y-4 rounded-lg border border-[var(--line)] bg-[var(--surface-hover)] p-4">
                  <label className="flex cursor-pointer items-center gap-2.5 text-sm text-[var(--text)]">
                    <input type="checkbox" checked={drainBeforeMaintenance} onChange={(e) => setDrainBeforeMaintenance(e.target.checked)} className="h-4 w-4 accent-[var(--brand)]" />
                    <span>Drain before maintenance</span>
                  </label>
                  <Input label="Maintenance Message" value={maintenanceMessage} onChange={setMaintenanceMessage} placeholder="Displayed to users during maintenance" />
                </div>
              )}
              <label className="block text-sm">
                <span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Token Rotation</span>
                <select className="h-10 w-full cursor-pointer rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" value={tokenRotationPolicy} onChange={(e) => setTokenRotationPolicy(e.target.value)}>
                  <option value="manual">Manual</option>
                  <option value="auto">Auto</option>
                </select>
                {/* "Auto" here is a recorded preference, not a scheduler. The
                    panel has no way to show that a rotation ran, so the choice
                    says so rather than implying an active policy. */}
                <span className="mt-1.5 block text-xs leading-5 text-warn">
                  Recorded on the node only. This panel neither runs nor reports rotations — rotating
                  is a manual action in Settings, and no view shows whether the policy was honoured.
                </span>
              </label>
              <label className="block text-sm">
                <span className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">TLS Setting</span>
                <select className="h-10 w-full cursor-pointer rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" value={tlsSetting} onChange={(e) => setTlsSetting(e.target.value)}>
                  <option value="auto">Auto</option>
                  <option value="manual">Manual</option>
                  <option value="disabled">Disabled</option>
                </select>
              </label>
              <p className="text-xs leading-5 text-warn">
                Security and monitoring choices on this card and the monitoring card are stored with
                the node but are never read back into the node views, so they cannot be reviewed or
                changed hereafter.
              </p>
            </div>
          </Card>
        </div>

        {createError ? <div className="mt-2"><AdminErrorState message={createError} /></div> : null}
        {/* The reason a disabled primary action cannot fire sits directly above
            it, instead of in a note elsewhere on a 40-field form. */}
        {!createMut.isPending && locations.length === 0 && !locationsError ? (
          <p className="text-xs leading-5 text-warn">No locations exist yet, so this form cannot be submitted — create a location first.</p>
        ) : !createMut.isPending && !locationId ? (
          <p className="text-xs leading-5 text-warn">Choose a location to enable creation.</p>
        ) : null}
        <ModalFooter onCancel={onClose} onConfirm={() => createMut.mutate()} confirmLabel={createMut.isPending ? "Creating…" : "Create Node"} disabled={createMut.isPending || !locationId || locationsError !== null} />
      </form>}
    </Modal>
  );
}
