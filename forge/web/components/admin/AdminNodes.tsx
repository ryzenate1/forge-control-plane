"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity, AlertCircle, ChevronRight, Cpu, Database, Eye, EyeOff, Globe, KeyRound, Lock, Mail,
  Network, Plus, Search, Settings as SettingsIcon, Shield, Trash2, Unlock, Wrench,
} from "lucide-react";
import {
  fetchNodes, createNode, deleteNode, fetchServers, fetchLocations, fetchRegions, fetchNode, updateNode, rotateNodeToken,
  fetchNodeAllocations, fetchNodeServers, fetchNodeLifecycle,
  fetchNodeSystemInformation, setAllocationAlias, deleteAllocationsBulk, getBeaconPanelURL,
  type ApiNode, type ApiAllocation, type ApiLocation, type ApiRegion, type ApiServer,
  type CreateNodeInput, type UpdateNodeInput,
} from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, SectionHeader, Textarea, cn } from "./admin-ui";

type Tab = "about" | "settings" | "configuration" | "allocation" | "servers";

const ADMIN_TABS: Array<{ id: Tab; label: string }> = [
  { id: "about", label: "About" },
  { id: "settings", label: "Settings" },
  { id: "configuration", label: "Configuration" },
  { id: "allocation", label: "Allocation" },
  { id: "servers", label: "Servers" },
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

export function AdminNodes() {
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = nodesQuery.data ?? [];
  const locationsQuery = useQuery({ queryKey: ["locations"], queryFn: fetchLocations });
  const locations = locationsQuery.data ?? [];
  const regionsQuery = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });
  const regions = regionsQuery.data ?? [];
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
  const servers = serversQuery.data ?? [];
  const [search, setSearch] = useState("");
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);

  const filtered = nodes.filter((n) =>
    !search || n.name.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Nodes"
        sub="Machines that run game servers. Each node runs the beacon agent."
        action={
          <Btn tone="primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> Create New
          </Btn>
        }
      />

      {locationsQuery.isError ? (
        <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
          <span>Could not load locations: {locationsQuery.error.message}</span>
          <Btn size="sm" tone="ghost" onClick={() => void locationsQuery.refetch()}>Retry</Btn>
        </div>
      ) : null}
      {serversQuery.isError ? (
        <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
          <span>Could not load server counts: {serversQuery.error.message}</span>
          <Btn size="sm" tone="ghost" onClick={() => void serversQuery.refetch()}>Retry</Btn>
        </div>
      ) : null}

      <Card>
        <div className="flex items-center gap-3 p-4">
          <Search size={14} className="text-slate-500" />
          <Input placeholder="Search Nodes" value={search} onChange={setSearch} />
        </div>
        {nodesQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading nodes…</div>
        ) : nodesQuery.isError ? (
          <div className="p-8 text-center text-sm text-red-300">
            <AlertCircle className="mx-auto mb-2" size={20} />
            <p>Nodes could not be loaded from the API.</p>
            <div className="mt-3"><Btn size="sm" onClick={() => void nodesQuery.refetch()}>Retry</Btn></div>
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Network} message={search ? "No nodes match your search." : "Setup required — create a node before hosting workloads."} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-slate-200">
              <thead>
                <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3"></th>
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">State</th>
                  <th className="px-4 py-3">Heartbeat</th>
                  <th className="px-4 py-3">Location</th>
                  <th className="px-4 py-3">Region</th>
                  <th className="px-4 py-3">Memory</th>
                  <th className="px-4 py-3">Disk</th>
                  <th className="px-4 py-3">Servers</th>
                  <th className="px-4 py-3">SSL</th>
                  <th className="px-4 py-3">Public</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((node) => (
                  <NodeRow
                    key={node.id}
                    node={node}
                    locations={locations}
                    regions={regions}
                    onClick={() => setSelectedNodeId(node.id)}
                    serverCount={servers.filter((server) => server.nodeId === node.id || server.node === node.id || server.node === node.name).length}
                  />
                ))}
              </tbody>
            </table>
          </div>
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
        locationsError={locationsQuery.isError ? locationsQuery.error : null}
        onRetryLocations={() => void locationsQuery.refetch()}
      />
    </div>
  );
}

function NodeRow({ node, locations, regions, onClick, serverCount }: {
  node: ApiNode;
  locations: ApiLocation[];
  regions: ApiRegion[];
  onClick: () => void;
  serverCount: number;
}) {
  // `actualState` is the backend's canonical operational state. Heartbeat is
  // shown separately because it is persisted monitoring evidence, not a probe.
  const actualState = node.actualState ?? "unknown";
  const heartbeatState = node.heartbeatState ?? "unknown";
  const isOnline = actualState === "online";
  const isDegraded = actualState === "degraded";
  const location = locations.find((candidate) => candidate.id === node.locationId);
  const region = regions.find((candidate) => candidate.id === node.regionId);
  const ssl = (node.scheme ?? "https") === "https";
  return (
    <tr className="border-b border-white/[0.04] transition hover:bg-white/[0.02]">
      <td className="px-4 py-3">
        <span
          className={cn(
            "inline-block h-2.5 w-2.5 rounded-full",
            isOnline ? "bg-emerald-500" : isDegraded ? "bg-amber-400" : actualState === "offline" ? "bg-red-500" : "bg-slate-500"
          )}
          title={`Actual state: ${actualState}; heartbeat: ${heartbeatState}`}
        />
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          {node.maintenanceMode && <Wrench size={12} className="text-amber-400" />}
          <button type="button" className="font-semibold text-left hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#dc2626]" onClick={onClick}>{node.name}</button>
        </div>
      </td>
      <td className="px-4 py-3 font-mono text-xs capitalize">{actualState}</td>
      <td className="px-4 py-3 font-mono text-xs capitalize text-slate-400">{heartbeatState}</td>
      <td className="px-4 py-3 text-slate-400">
        {location ? <><div>{location.short}</div><div className="text-xs text-slate-500">{location.long}</div></> : "—"}
      </td>
      <td className="px-4 py-3 text-slate-400">
        {region ? <><div>{region.name}</div><div className="text-xs text-slate-500">{region.slug}</div></> : node.region || "—"}
      </td>
      <td className="px-4 py-3 font-mono text-xs">{node.memoryMb} MiB</td>
      <td className="px-4 py-3 font-mono text-xs">{node.diskMb} MiB</td>
      <td className="px-4 py-3 text-slate-400">{serverCount}</td>
      <td className="px-4 py-3">{ssl ? <Lock size={14} className="text-emerald-500" /> : <Unlock size={14} className="text-red-400" />}</td>
      <td className="px-4 py-3">{node.public ?? node.isPublic ? <Eye size={14} className="text-sky-500" /> : <EyeOff size={14} className="text-slate-500" />}</td>
      <td className="px-4 py-3 text-right">
        <ChevronRight size={14} className="text-slate-500" />
      </td>
    </tr>
  );
}

function NodeDetailView({ nodeId, onClose }: { nodeId: string; onClose: () => void }) {
  const nodeQuery = useQuery({ queryKey: ["node", nodeId], queryFn: () => fetchNode(nodeId) });
  const { data: node, isLoading } = nodeQuery;
  const allocQuery = useQuery({ queryKey: ["node-allocations", nodeId], queryFn: () => fetchNodeAllocations(nodeId) });
  const allocations = allocQuery.data ?? [];
  const [tab, setTab] = useState<Tab>("about");
  const qc = useQueryClient();
  const { toast } = useToast();
  const deleteMut = useMutation({
    mutationFn: () => deleteNode(nodeId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["nodes"] }); onClose(); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete node", message: e.message }),
  });

  if (isLoading) {
    return <Modal title="Node" onClose={onClose}><div className="p-8 text-center text-sm text-slate-500">Loading…</div></Modal>;
  }

  if (nodeQuery.isError || !node) {
    return (
      <Modal title="Node" onClose={onClose}>
        <div className="space-y-3 p-8 text-center text-sm text-red-300">
          <AlertCircle className="mx-auto" size={20} />
          <p>{nodeQuery.isError ? `Could not load this node: ${nodeQuery.error.message}` : "This node is no longer available."}</p>
          <div><Btn size="sm" onClick={() => void nodeQuery.refetch()}>Retry</Btn></div>
        </div>
      </Modal>
    );
  }

  return (
    <Modal title={node.name} onClose={onClose} wide>
      <div className="space-y-6">
        <div className="flex justify-end"><Btn tone="danger" size="sm" type="button" disabled={deleteMut.isPending} onClick={() => { if (confirm(`Delete ${node.name}? This is only allowed after its servers and allocations are removed.`)) deleteMut.mutate(); }}><Trash2 size={14} /> {deleteMut.isPending ? "Deleting…" : "Delete Node"}</Btn></div>
        <div className="flex gap-1 border-b border-white/[0.06]" role="tablist" aria-label="Node sections">
          {ADMIN_TABS.map((t) => (
            <button
              key={t.id}
              type="button"
              role="tab"
              aria-selected={tab === t.id}
              className={cn(
                "px-3 py-2 text-sm font-medium transition",
                tab === t.id ? "border-b-2 border-[#dc2626] text-[#dc2626]" : "text-slate-400 hover:text-slate-200"
              )}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>
        {tab === "about" && <NodeAboutTab nodeId={nodeId} />}
        {tab === "settings" && <NodeSettingsTab node={node} />}
        {tab === "configuration" && <NodeConfigurationTab node={node} />}
        {tab === "allocation" && <NodeAllocationTab node={node} allocations={allocations} />}
        {tab === "servers" && <NodeServersTab nodeId={nodeId} />}
      </div>
    </Modal>
  );
}

function NodeAboutTab({ nodeId }: { nodeId: string }) {
  const { data: lifecycle, isError: isLifecycleError } = useQuery({
    queryKey: ["node-lifecycle", nodeId],
    queryFn: () => fetchNodeLifecycle(nodeId),
    refetchInterval: 10_000,
  });
  const { data: sys, isError } = useQuery({
    queryKey: ["node-sysinfo", nodeId],
    queryFn: () => fetchNodeSystemInformation(nodeId),
    refetchInterval: 10_000,
  });
  const { data: node } = useQuery({ queryKey: ["node", nodeId], queryFn: () => fetchNode(nodeId) });
  const serversQuery = useQuery<ApiServer[]>({
    queryKey: ["node-servers", nodeId],
    queryFn: () => fetchNodeServers(nodeId),
  });
  const filteredServers = serversQuery.data ?? [];

  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <div className="lg:col-span-2 space-y-4">
        {serversQuery.isError ? (
          <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
            <span>Could not load servers on this node: {serversQuery.error.message}</span>
            <Btn size="sm" tone="ghost" onClick={() => void serversQuery.refetch()}>Retry</Btn>
          </div>
        ) : null}
        <Card>
          <CardHeader title="Information" icon={Activity} />
          <ul className="divide-y divide-white/[0.04]">
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">Daemon Version</span>
              <span className="font-mono text-slate-200" data-attr="info-version">{sys?.version ?? (isError ? "Offline" : "Probing…")}</span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">System</span>
              <span className="font-mono text-slate-200" data-attr="info-system">
                {sys ? `${sys.os ?? "?"} (${sys.architecture ?? "?"})` : "—"}
              </span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">CPU Threads</span>
              <span className="font-mono text-slate-200" data-attr="info-cpus">{sys?.cpuThreads ?? "—"}</span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">Docker</span>
              <span className={cn("font-mono", sys?.dockerAvailable ? "text-emerald-400" : "text-red-400")}>
                {sys?.dockerStatus ?? "unknown"}
              </span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">FQDN</span>
              <span className="font-mono text-slate-200">{node?.fqdn ?? "—"}</span>
            </li>
          </ul>
        </Card>
        <Card>
          <CardHeader title="Lifecycle" icon={Activity} />
          <ul className="divide-y divide-white/[0.04]">
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">Readiness</span>
              <span className={cn("font-mono", lifecycle?.placementEligible ? "text-emerald-400" : "text-amber-400")}>
                {lifecycle ? `${lifecycle.healthScore.total}/100` : isLifecycleError ? "Unavailable" : "Loading…"}
              </span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">Actual State / Heartbeat</span>
              <span className="font-mono text-slate-200 capitalize">
                {lifecycle ? `${lifecycle.node.actualState ?? "unknown"} / ${lifecycle.node.heartbeatState ?? "unknown"}` : "—"}
              </span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">Placement</span>
              <span className={cn("font-mono", lifecycle?.placementEligible ? "text-emerald-400" : "text-amber-400")}>
                {lifecycle?.placementEligible ? "Eligible" : lifecycle?.placementBlockedReason ?? "Not eligible"}
              </span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">Capacity (allocated / available)</span>
              <span className="font-mono text-slate-200">
                {lifecycle ? `${lifecycle.capacity.allocated_memory} / ${lifecycle.capacity.available_memory} MiB memory · ${lifecycle.capacity.allocated_disk} / ${lifecycle.capacity.available_disk} MiB disk` : "—"}
              </span>
            </li>
            <li className="flex justify-between px-4 py-3 text-sm">
              <span className="text-slate-400">Allocated CPU / Servers</span>
              <span className="font-mono text-slate-200">
                {lifecycle ? `${lifecycle.capacity.allocated_cpu} / ${lifecycle.capacity.available_cpu} available · ${lifecycle.capacity.server_count} servers` : "—"}
              </span>
            </li>
          </ul>
        </Card>
        {node?.description && (
          <Card>
            <CardHeader title="Description" icon={Mail} />
            <pre className="px-4 py-3 text-xs text-slate-300 whitespace-pre-wrap">{node.description}</pre>
          </Card>
        )}
      </div>
      <div className="space-y-3">
        <SmallBox color="orange" label="Maintenance" value={node?.maintenanceMode ? "ENABLED" : "Normal"} />
        <SmallBox color="blue" label="Total Servers" value={String(filteredServers.length)} />
        <SmallBox color="purple" label="Memory Limit" value={`${node?.memoryMb ?? 0} MiB`} />
        <SmallBox color="emerald" label="Disk Limit" value={`${node?.diskMb ?? 0} MiB`} />
      </div>
    </div>
  );
}

function SmallBox({ color, label, value }: { color: "orange" | "blue" | "purple" | "emerald"; label: string; value: string }) {
  const map: Record<string, string> = {
    orange: "bg-orange-500/10 text-orange-400 border-orange-500/30",
    blue: "bg-sky-500/10 text-sky-400 border-sky-500/30",
    purple: "bg-purple-500/10 text-purple-400 border-purple-500/30",
    emerald: "bg-emerald-500/10 text-emerald-400 border-emerald-500/30",
  };
  return (
    <div className={cn("rounded-lg border p-4", map[color])}>
      <div className="text-[10px] font-bold uppercase tracking-widest opacity-70">{label}</div>
      <div className="mt-1 text-lg font-bold">{value}</div>
    </div>
  );
}

function NodeSettingsTab({ node }: { node: ApiNode }) {
  const qc = useQueryClient();
  const locationsQuery = useQuery({ queryKey: ["locations"], queryFn: fetchLocations });
  const locations = locationsQuery.data ?? [];
  const [name, setName] = useState(node.name);
  const [description, setDescription] = useState(node.description ?? "");
  const [locationId, setLocationId] = useState(node.locationId ?? "");
  const [fqdn, setFqdn] = useState(node.fqdn ?? "");
  const [scheme, setScheme] = useState(node.scheme ?? "https");
  const [behindProxy, setBehindProxy] = useState(node.behindProxy ?? false);
  const [desiredState, setDesiredState] = useState(node.desiredState ?? (node.draining ? "draining" : node.maintenanceMode ? "maintenance" : "active"));
  const [rotatedToken, setRotatedToken] = useState<string | null>(null);
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
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-slate-400">Location</span>
            <select className="h-10 w-full rounded-lg border border-white/10 bg-[#141824] px-3 text-slate-100" value={locationId} onChange={(e) => setLocationId(e.target.value)} required disabled={locationsQuery.isPending || locationsQuery.isError}>
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
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-slate-400">SSL</span>
            <select className="h-10 w-full rounded-lg border border-white/10 bg-[#141824] px-3 text-slate-100" value={scheme} onChange={(e) => setScheme(e.target.value)}>
              <option value="https">https (SSL)</option>
              <option value="http">http (no SSL)</option>
            </select>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={behindProxy} onChange={(e) => setBehindProxy(e.target.checked)} className="accent-[#dc2626]" />
            <span>Behind Proxy</span>
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-slate-400">Lifecycle state</span>
            <select className="h-10 w-full rounded-lg border border-white/10 bg-[#141824] px-3 text-slate-100" value={desiredState} onChange={(e) => setDesiredState(e.target.value)}>
              <option value="active">Active — eligible when healthy</option>
              <option value="draining">Draining — exclude from placement</option>
              <option value="maintenance">Maintenance — exclude from placement</option>
            </select>
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-slate-400">Scheduler Backend</span>
            <select className="h-10 w-full rounded-lg border border-white/10 bg-[#141824] px-3 text-slate-100" value={schedulerType} onChange={(e) => setSchedulerType(e.target.value)}>
              <option value="docker">Docker (default)</option>
              <option value="k3s">K3s (Kubernetes)</option>
              <option value="nomad">Nomad (HashiCorp)</option>
            </select>
          </label>

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
          <Btn tone="ghost" onClick={() => { if (confirm("Rotate the node token? The current daemon credential will stop working.")) rotateMut.mutate(); }} type="button">
            <KeyRound size={14} /> Rotate Token
          </Btn>
          <Btn tone="primary" type="submit" disabled={saveMut.isPending || !locationId || locationsQuery.isPending || locationsQuery.isError}>
            {saveMut.isPending ? "Saving…" : "Save"}
          </Btn>
        </div>
      </div>
      {rotatedToken ? <div className="md:col-span-2 rounded-lg border border-amber-500/30 bg-amber-950/20 p-4"><p className="text-sm font-semibold text-amber-200">New complete credential — shown once</p><pre className="mt-2 overflow-auto rounded bg-black/30 p-3 text-xs text-emerald-300">{rotatedToken}</pre><div className="mt-3 flex gap-2"><Btn size="sm" tone="ghost" type="button" onClick={() => { void navigator.clipboard?.writeText(rotatedToken); toast({ tone: "success", title: "Credential copied" }); }}>Copy credential</Btn><Btn size="sm" tone="ghost" type="button" onClick={() => setRotatedToken(null)}>I stored it</Btn></div></div> : null}
    </form>
  );
}

function NodeConfigurationTab({ node }: { node: ApiNode }) {
  const panelURL = getBeaconPanelURL();
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader title="Beacon environment" icon={Globe} />
        <div className="space-y-3 p-4 text-sm text-slate-300">
          <p>Beacon reads its panel connection from environment variables. It does not load the legacy YAML file or support <code>beacon configure</code>.</p>
          <p>Use the full credential shown when this node was created or when its token was rotated. If it was not retained, rotate the token in Settings.</p>
          <pre className="overflow-auto rounded bg-[#0a0e16] p-4 text-[11px] leading-relaxed text-slate-300">{`# /etc/forge/beacon.env (mode 0600)
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
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [aliases, setAliases] = useState<Record<string, string>>({});

  const filtered = allocations.filter((a) => !filter || a.ip.includes(filter) || a.port.toString().includes(filter));
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
          <Btn tone="danger" disabled={deleteBulkMut.isPending} onClick={() => { if (confirm(`Delete ${selected.size} allocation(s)?`)) deleteBulkMut.mutate(); }}>
            <Trash2 size={14} /> {deleteBulkMut.isPending ? "Deleting…" : `Delete ${selected.size}`}
          </Btn>
        )}
      </div>
      <Card>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
              <th className="px-4 py-2">
                <input type="checkbox" checked={allFiltered} onChange={toggleAll} disabled={deletable.length === 0 || deleteBulkMut.isPending} className="accent-[#dc2626]" />
              </th>
              <th className="px-4 py-2">IP</th>
              <th className="px-4 py-2">Alias</th>
              <th className="px-4 py-2">Port</th>
              <th className="px-4 py-2">Server</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((a) => (
              <tr key={a.id} className="border-b border-white/[0.04]">
                <td className="px-4 py-2">
                  <input type="checkbox" disabled={!!a.server || deleteBulkMut.isPending} checked={selected.has(a.id)} onChange={() => toggle(a.id)} className="accent-[#dc2626]" />
                </td>
                <td className="px-4 py-2 font-mono text-xs">{a.ip}</td>
                <td className="px-4 py-2">
                  <input
                    className="h-8 w-32 rounded border border-white/10 bg-[#141824] px-2 text-xs disabled:cursor-not-allowed disabled:opacity-60"
                    value={aliases[a.id] ?? a.alias ?? ""}
                    disabled={setAliasMut.isPending}
                    onChange={(e) => setAliases((current) => ({ ...current, [a.id]: e.target.value }))}
                    onBlur={(e) => {
                      const alias = e.target.value.trim();
                      if ((a.alias ?? "") !== alias) setAliasMut.mutate({ id: a.id, alias });
                    }}
                  />
                </td>
                <td className="px-4 py-2 font-mono text-xs">{a.port}</td>
                <td className="px-4 py-2 text-slate-400">{a.server ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}

function NodeServersTab({ nodeId }: { nodeId: string }) {
  const serversQuery = useQuery<ApiServer[]>({
    queryKey: ["node-servers-list", nodeId],
    queryFn: () => fetchNodeServers(nodeId),
  });
  const filtered = serversQuery.data ?? [];
  return (
    <Card>
      <CardHeader title={`Servers (${filtered.length})`} icon={Database} />
      {serversQuery.isError ? (
        <div className="p-4">
          <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
            <span>Could not load servers on this node: {serversQuery.error.message}</span>
            <Btn size="sm" tone="ghost" onClick={() => void serversQuery.refetch()}>Retry</Btn>
          </div>
        </div>
      ) : filtered.length === 0 ? (
        <EmptyState icon={Database} message="No servers on this node." />
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">UUID</th>
              <th className="px-4 py-2">Status</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((s) => (
              <tr key={s.id} className="border-b border-white/[0.04]">
                <td className="px-4 py-2 font-semibold">{s.name}</td>
                <td className="px-4 py-2 font-mono text-xs text-slate-400">{s.id.slice(0, 8)}…</td>
                <td className="px-4 py-2">{s.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}

function CreateNodeModal({ open, onClose, locations, locationsError, onRetryLocations }: {
  open: boolean;
  onClose: () => void;
  locations: ApiLocation[];
  locationsError: Error | null;
  onRetryLocations: () => void;
}) {
  const qc = useQueryClient();

  // — Basic Details
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [locationId, setLocationId] = useState("");
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
  const [createError, setCreateError] = useState<string | null>(null);
  const panelURL = getBeaconPanelURL();

  const createMut = useMutation({
    mutationFn: () => {
      const validationError = validateNodeForm(name, locationId, fqdn, scheme, memoryMb, diskMb, daemonListen, daemonSftp);
      if (validationError) throw new Error(validationError);
      const location = locations.find((candidate) => candidate.id === locationId);
      if (!location) throw new Error("Select a valid location before creating the node.");
      const tagsArr = tags.split(",").map((t) => t.trim()).filter(Boolean);
      const allowedIpsArr = allowedIps.split(",").map((t) => t.trim()).filter(Boolean);
      return createNode({
        name: name.trim(),
        region: location.short,
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
          <div className="rounded-lg border border-amber-500/30 bg-amber-950/20 p-4 text-sm text-amber-100">Save this credential now. Forge will not show it again; rotate the token if it is lost.</div>
          <pre className="overflow-auto rounded bg-[#0a0e16] p-4 text-xs leading-relaxed text-emerald-300">{`# /etc/forge/beacon.env (mode 0600)
APP_ENV=production
DAEMON_NODE_ID=${onboarding.id}
DAEMON_NODE_TOKEN=${onboarding.token}
PANEL_API_URL=${panelURL}
DAEMON_ADDR=:${daemonListen}
DAEMON_SFTP_ADDR=:${daemonSftp}
DAEMON_DATA_DIR=${daemonBase}
DAEMON_ALLOW_INSECURE_NO_AUTH=false

# Configure Beacon's systemd EnvironmentFile= or container env_file, then restart Beacon.`}</pre>
          <div className="flex justify-end gap-2">
            <Btn tone="ghost" onClick={() => { if (navigator.clipboard) void navigator.clipboard.writeText(onboarding.token).then(() => setCopied(true)); }}>{copied ? "Credential copied" : "Copy credential"}</Btn>
            <Btn tone="primary" onClick={onClose}>I stored this credential</Btn>
          </div>
        </div>
      ) : <form
        className="space-y-8"
        onSubmit={(e) => { e.preventDefault(); createMut.mutate(); }}
      >
        <div className="grid gap-6 md:grid-cols-2">
          <Card>
            <CardHeader title="Basic Details" icon={Shield} />
            <div className="space-y-5 p-5">
              <Input label="Name" value={name} onChange={setName} placeholder="nyc-dal-01" required />
              <Input label="Display Name" value={displayName} onChange={setDisplayName} placeholder="NYC Dallas Node 1" />
              <Textarea label="Description" value={description} onChange={setDescription} rows={2} placeholder="Optional description for this node" />
              <label className="block text-sm">
                <span className="mb-1.5 block text-sm font-medium text-slate-300">Location</span>
                <select className="h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15" value={locationId} onChange={(e) => setLocationId(e.target.value)} required disabled={locations.length === 0 || locationsError !== null}>
                  <option value="">Select…</option>
                  {locations.map((location) => <option key={location.id} value={location.id}>{location.short} — {location.long}</option>)}
                </select>
                {locationsError ? (
                  <div className="mt-2 flex items-start justify-between gap-3 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
                    <span>Could not load locations: {locationsError.message}</span>
                    <Btn size="sm" tone="ghost" type="button" onClick={onRetryLocations}>Retry</Btn>
                  </div>
                ) : locations.length === 0 ? <p className="mt-1 text-xs text-amber-300">Create a location first before adding a node.</p> : null}
              </label>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-slate-300">
                <input type="checkbox" checked={publicNode} onChange={(e) => setPublicNode(e.target.checked)} className="h-4 w-4 accent-[#dc2626]" />
                <span>Public node</span>
              </label>
            </div>
          </Card>

          <Card>
            <CardHeader title="Network" icon={Globe} />
            <div className="space-y-5 p-5">
              <Input label="FQDN" value={fqdn} onChange={setFqdn} placeholder="node1.example.com" required />
              <Input label="Public Hostname" value={publicHostname} onChange={setPublicHostname} placeholder="Optional public-facing hostname" />
              <label className="block text-sm">
                <span className="mb-1.5 block text-sm font-medium text-slate-300">SSL</span>
                <select className="h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15" value={scheme} onChange={(e) => setScheme(e.target.value)}>
                  <option value="https">https</option>
                  <option value="http">http</option>
                </select>
              </label>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-slate-300">
                <input type="checkbox" checked={behindProxy} onChange={(e) => setBehindProxy(e.target.checked)} className="h-4 w-4 accent-[#dc2626]" />
                <span>Behind Proxy</span>
              </label>
              <Input label="Allowed IPs" value={allowedIps} onChange={setAllowedIps} placeholder="Comma-separated, e.g. 10.0.0.0/8, 192.168.1.0/24" />
              <Input label="Network Interface" value={networkInterface} onChange={setNetworkInterface} placeholder="e.g. eth0, bond0" />
            </div>
          </Card>
        </div>

        <div className="grid gap-6 md:grid-cols-2">
          <Card>
            <CardHeader title="Resource Limits" icon={Cpu} />
            <div className="space-y-5 p-5">
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
            <div className="space-y-5 p-5">
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

        <div className="grid gap-6 md:grid-cols-3">
          <Card>
            <CardHeader title="Allocation" icon={Network} />
            <div className="space-y-5 p-5">
              <Input label="Default IP" value={defaultAllocationIp} onChange={setDefaultAllocationIp} />
              <div className="grid grid-cols-2 gap-4">
                <Input label="Port Min" value={allocationPortMin} onChange={setAllocationPortMin} type="number" />
                <Input label="Port Max" value={allocationPortMax} onChange={setAllocationPortMax} type="number" />
              </div>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-slate-300">
                <input type="checkbox" checked={autoAllocate} onChange={(e) => setAutoAllocate(e.target.checked)} className="h-4 w-4 accent-[#dc2626]" />
                <span>Auto-allocate ports</span>
              </label>
            </div>
          </Card>

          <Card>
            <CardHeader title="Scheduler" icon={SettingsIcon} />
            <div className="space-y-5 p-5">
              <label className="block text-sm">
                <span className="mb-1.5 block text-sm font-medium text-slate-300">Backend</span>
                <select className="h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15" value={schedulerType} onChange={(e) => setSchedulerType(e.target.value)}>
                  <option value="docker">Docker</option>
                  <option value="k3s">K3s (Kubernetes)</option>
                  <option value="nomad">Nomad (HashiCorp)</option>
                </select>
              </label>
            </div>
          </Card>

          <Card>
            <CardHeader title="Tags" icon={Activity} />
            <div className="space-y-5 p-5">
              <Input label="Tags" value={tags} onChange={setTags} placeholder="ssd, gpu, low-latency" />
              <p className="text-xs text-slate-500">Tags let you filter and group nodes for scheduling constraints.</p>
            </div>
          </Card>
        </div>

        <div className="grid gap-6 md:grid-cols-2">
          <Card>
            <CardHeader title="Monitoring & Alerts" icon={Activity} />
            <div className="space-y-5 p-5">
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-slate-300">
                <input type="checkbox" checked={enableHealthChecks} onChange={(e) => setEnableHealthChecks(e.target.checked)} className="h-4 w-4 accent-[#dc2626]" />
                <span>Enable health checks</span>
              </label>
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-slate-300">
                <input type="checkbox" checked={enableMetrics} onChange={(e) => setEnableMetrics(e.target.checked)} className="h-4 w-4 accent-[#dc2626]" />
                <span>Enable metrics collection</span>
              </label>
              <Input label="Prometheus Endpoint" value={prometheusEndpoint} onChange={setPrometheusEndpoint} placeholder="Optional Prometheus scrape URL" />
              <div>
                <p className="mb-3 text-xs font-medium text-slate-400">Alert thresholds (0–100%)</p>
                <div className="grid grid-cols-3 gap-4">
                  <Input label="CPU %" value={alertThresholdCpu} onChange={setAlertThresholdCpu} type="number" />
                  <Input label="Memory %" value={alertThresholdMemory} onChange={setAlertThresholdMemory} type="number" />
                  <Input label="Disk %" value={alertThresholdDisk} onChange={setAlertThresholdDisk} type="number" />
                </div>
              </div>
            </div>
          </Card>

          <Card>
            <CardHeader title="Maintenance & Security" icon={Lock} />
            <div className="space-y-5 p-5">
              <label className="flex cursor-pointer items-center gap-2.5 text-sm text-slate-300">
                <input type="checkbox" checked={maintenanceMode} onChange={(e) => setMaintenanceMode(e.target.checked)} className="h-4 w-4 accent-[#dc2626]" />
                <span>Maintenance mode</span>
              </label>
              {maintenanceMode && (
                <div className="space-y-4 rounded-lg border border-white/[0.06] bg-white/[0.02] p-4">
                  <label className="flex cursor-pointer items-center gap-2.5 text-sm text-slate-300">
                    <input type="checkbox" checked={drainBeforeMaintenance} onChange={(e) => setDrainBeforeMaintenance(e.target.checked)} className="h-4 w-4 accent-[#dc2626]" />
                    <span>Drain before maintenance</span>
                  </label>
                  <Input label="Maintenance Message" value={maintenanceMessage} onChange={setMaintenanceMessage} placeholder="Displayed to users during maintenance" />
                </div>
              )}
              <label className="block text-sm">
                <span className="mb-1.5 block text-sm font-medium text-slate-300">Token Rotation</span>
                <select className="h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15" value={tokenRotationPolicy} onChange={(e) => setTokenRotationPolicy(e.target.value)}>
                  <option value="manual">Manual</option>
                  <option value="auto">Auto</option>
                </select>
              </label>
              <label className="block text-sm">
                <span className="mb-1.5 block text-sm font-medium text-slate-300">TLS Setting</span>
                <select className="h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15" value={tlsSetting} onChange={(e) => setTlsSetting(e.target.value)}>
                  <option value="auto">Auto</option>
                  <option value="manual">Manual</option>
                  <option value="disabled">Disabled</option>
                </select>
              </label>
            </div>
          </Card>
        </div>

        <div className="flex items-start justify-between gap-4 border-t border-white/[0.06] pt-6">
          <div className="min-w-0 flex-1">
            {createError ? <div className="inline-flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200"><AlertCircle size={14} className="mt-0.5 shrink-0" /> <span>{createError}</span></div> : null}
          </div>
          <div className="flex shrink-0 gap-3">
            <Btn tone="ghost" type="button" onClick={onClose}>Cancel</Btn>
            <Btn tone="primary" type="submit" disabled={createMut.isPending || !locationId || locationsError !== null}>
              {createMut.isPending ? "Creating…" : "Create Node"}
            </Btn>
          </div>
        </div>
      </form>}
    </Modal>
  );
}
