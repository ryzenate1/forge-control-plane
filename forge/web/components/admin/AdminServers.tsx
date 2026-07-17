"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle, Ban, Box, Cpu, Database, ExternalLink, HardDrive, Info,
  KeyRound, Layers, Network, Plus, RefreshCw, Trash2, Zap,
} from "lucide-react";
import {
  type ApiServer, type ApiNode, type ApiAllocation, type ApiEgg,
  type ApiUser, type ApiMount, type ApiRegion, type ApiTemplate,
  fetchServer, fetchServers, fetchNodes, fetchEggs, fetchAllocations, fetchUsers,
  fetchTemplates, fetchRegions, fetchMounts, fetchServerMounts, fetchServerDatabases, fetchServerStartup,
  assignServerAllocation, assignServerMount, fetchServerAllocations, removeServerMount, searchUsers, createServer, createServerDatabase,
  rotateServerDatabasePasswordByBody, deleteServerDatabaseWithSuffix, setPrimaryServerAllocation, unassignServerAllocation, updateServerStartupVariable,
  cancelServerTransfer, deleteServer, fetchServerTransferStatus, suspendServer, transferServer, unsuspendServer, reinstallServer, updateServer,
  ApiUserSearchResult,
} from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea, cn } from "./admin-ui";

type ServerTab = "about" | "details" | "build" | "startup" | "allocations" | "database" | "mounts" | "manage" | "delete";
const SERVER_TABS: Array<{ id: ServerTab; label: string; danger?: boolean }> = [
  { id: "about", label: "About" },
  { id: "details", label: "Details" },
  { id: "build", label: "Build Configuration" },
  { id: "startup", label: "Startup" },
  { id: "allocations", label: "Allocations" },
  { id: "database", label: "Database" },
  { id: "mounts", label: "Mounts" },
  { id: "manage", label: "Manage" },
  { id: "delete", label: "Delete", danger: true },
];

export function AdminServers() {
  const { data: servers = [], isLoading } = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
  const { data: users = [] } = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const { data: allocations = [] } = useQuery({ queryKey: ["allocations"], queryFn: fetchAllocations });
  const { data: eggs = [] } = useQuery<ApiEgg[]>({
    queryKey: ["eggs"],
    queryFn: () => fetchEggs("*"),
  });
  const { data: templates = [] } = useQuery({ queryKey: ["templates"], queryFn: fetchTemplates });
  const { data: regions = [] } = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });
  const { data: mounts = [] } = useQuery({ queryKey: ["mounts"], queryFn: fetchMounts });
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [selectedServerId, setSelectedServerId] = useState<string | null>(null);
  const [tab, setTab] = useState<ServerTab>("about");

  const filtered = servers.filter((s) => !search || s.name.toLowerCase().includes(search.toLowerCase()));

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Servers"
        sub="All game server instances across the cluster."
        action={
          <Btn tone="primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> Create New
          </Btn>
        }
      />
      <Card>
        <div className="flex items-center gap-3 p-4">
          <Input placeholder="Search Servers" value={search} onChange={setSearch} />
        </div>
        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading servers…</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Layers} message="No servers yet." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">UUID</th>
                  <th className="px-4 py-3">Owner</th>
                  <th className="px-4 py-3">Node</th>
                  <th className="px-4 py-3">Connection</th>
                  <th className="px-4 py-3">Status</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((s) => (
                  <tr key={s.id} className="border-b border-white/[0.04] transition hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-semibold"><button type="button" className="text-left hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#dc2626]" onClick={() => { setSelectedServerId(s.id); setTab("about"); }}>{s.name}</button></td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">{s.id.slice(0, 8)}…</td>
                    <td className="px-4 py-3 text-slate-400">{s.owner ?? "—"}</td>
                    <td className="px-4 py-3 text-slate-400">{s.node ?? "—"}</td>
                    <td className="px-4 py-3 font-mono text-xs">{s.allocation ?? "—"}</td>
                    <td className="px-4 py-3"><ServerStatusBadge server={s} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate && (
        <CreateServerModal
          users={users}
          nodes={nodes}
          allocations={allocations}
          templates={templates}
          eggs={eggs}
          regions={regions}
          onClose={() => setShowCreate(false)}
        />
      )}

      {selectedServerId && (
        <Modal title="Server" onClose={() => setSelectedServerId(null)} wide>
          <ServerDetailContent
            serverId={selectedServerId}
            tab={tab}
            setTab={setTab}
            users={users}
            nodes={nodes}
            allocations={allocations}
            mounts={mounts}
            onClose={() => setSelectedServerId(null)}
          />
        </Modal>
      )}
    </div>
  );
}

function CreateServerModal({ users, nodes, allocations, templates, eggs, regions, onClose }: {
  users: ApiUser[];
  nodes: ApiNode[];
  allocations: ApiAllocation[];
  templates: any[];
  eggs: ApiEgg[];
  regions: ApiRegion[];
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [ownerId, setOwnerId] = useState("");
  const [nodeId, setNodeId] = useState("");
  const [regionId, setRegionId] = useState("");
  const [templateId, setTemplateId] = useState("");
  const [allocationId, setAllocationId] = useState("");
  const [memoryMb, setMemoryMb] = useState("2048");
  const [cpuShares, setCpuShares] = useState("1024");
  const [diskMb, setDiskMb] = useState("10240");
  const availableAllocations = allocations.filter((allocation) => !allocation.server && (!nodeId || allocation.node === nodeId));
  const templateOptions = [
    ...templates.map((template) => ({
      id: template.id,
      label: eggs.find((egg) => egg.id === template.id)?.name ?? template.name,
    })),
    ...eggs.filter((egg) => !templates.some((template) => template.id === egg.id)).map((egg) => ({
      id: egg.id,
      label: egg.nestName ? `${egg.nestName} / ${egg.name}` : egg.name,
    })),
  ];
  const selectedAllocation = availableAllocations.find((allocation) => allocation.id === allocationId);
  const validationError = !name.trim()
    ? "Server name is required."
    : !ownerId
      ? "Owner is required."
      : !templateId
        ? "Template is required."
        : !nodeId
          ? "Node is required."
          : !allocationId || !selectedAllocation
            ? "An available allocation is required."
            : selectedAllocation.node !== nodeId
              ? "The allocation must belong to the selected node."
              : [memoryMb, cpuShares, diskMb].some((value) => !Number.isFinite(Number(value)) || Number(value) <= 0)
                ? "Memory, CPU shares, and disk must be positive numbers."
                : null;
  const createMut = useMutation({
    mutationFn: () => createServer({
      name: name.trim(),
      ownerId,
      nodeId,
      regionId: regionId || undefined,
      templateId,
      allocationId,
      memoryMb: Number(memoryMb),
      cpuShares: Number(cpuShares),
      diskMb: Number(diskMb),
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["servers"] });
      void qc.invalidateQueries({ queryKey: ["allocations"] });
      onClose();
    },
  });

  return (
    <Modal title="Create Server" onClose={onClose} wide>
      <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (!validationError) createMut.mutate(); }}>
        <div className="grid gap-4 md:grid-cols-2">
          <Input label="Server Name" value={name} onChange={setName} required />
          <label className="block text-sm font-medium text-slate-300">
            <span className="mb-1.5 block">Owner</span>
            <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100" value={ownerId} onChange={(event) => setOwnerId(event.target.value)} required>
              <option value="">Select owner…</option>
              {users.map((user) => <option key={user.id} value={user.id}>{user.email}</option>)}
            </select>
          </label>
          <label className="block text-sm font-medium text-slate-300">
            <span className="mb-1.5 block">Node</span>
            <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100" value={nodeId} onChange={(event) => { setNodeId(event.target.value); setAllocationId(""); }}>
              <option value="">Select node…</option>
              {nodes.filter((node) => !regionId || node.regionId === regionId || node.region === regions.find((region) => region.id === regionId)?.slug).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
            </select>
          </label>
          <label className="block text-sm font-medium text-slate-300">
            <span className="mb-1.5 block">Region</span>
            <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100" value={regionId} onChange={(event) => { setRegionId(event.target.value); setNodeId(""); setAllocationId(""); }}>
              <option value="">Select region…</option>
              {regions.filter((region) => region.enabled).map((region) => <option key={region.id} value={region.id}>{region.name}</option>)}
            </select>
          </label>
          <label className="block text-sm font-medium text-slate-300">
            <span className="mb-1.5 block">Template / Egg</span>
            <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100" value={templateId} onChange={(event) => setTemplateId(event.target.value)} required>
              <option value="">Select template…</option>
              {templateOptions.map((template) => <option key={template.id} value={template.id}>{template.label}</option>)}
            </select>
          </label>
          <label className="block text-sm font-medium text-slate-300">
            <span className="mb-1.5 block">Available Allocation</span>
            <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100" value={allocationId} onChange={(event) => setAllocationId(event.target.value)} required>
              <option value="">Select allocation…</option>
              {availableAllocations.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}
            </select>
          </label>
          <Input label="Memory (MiB)" value={memoryMb} onChange={setMemoryMb} type="number" required />
          <Input label="CPU Shares" value={cpuShares} onChange={setCpuShares} type="number" required />
          <Input label="Disk (MiB)" value={diskMb} onChange={setDiskMb} type="number" required />
        </div>
        {validationError && <p className="text-sm text-amber-300">{validationError}</p>}
        {createMut.error && <p className="text-sm text-red-300">{createMut.error instanceof Error ? createMut.error.message : "Server creation failed."}</p>}
        <ModalFooter onCancel={onClose} onConfirm={() => { if (!validationError) createMut.mutate(); }} confirmLabel={createMut.isPending ? "Creating…" : "Create Server"} disabled={Boolean(validationError) || createMut.isPending} />
      </form>
    </Modal>
  );
}

function ServerStatusBadge({ server }: { server: ApiServer }) {
  if (server.suspended) return <Pill tone="red">Suspended</Pill>;
  if (server.status === "installing") return <Pill tone="yellow">Installing</Pill>;
  if (server.status === "running") return <Pill tone="green">Active</Pill>;
  return <Pill tone="blue">{server.status}</Pill>;
}

function ServerDetailContent({ serverId, tab, setTab, users, nodes, allocations, mounts, onClose }: {
  serverId: string; tab: ServerTab; setTab: (t: ServerTab) => void;
  users: ApiUser[]; nodes: ApiNode[]; allocations: ApiAllocation[];
  mounts: ApiMount[]; onClose: () => void;
}) {
  const qc = useQueryClient();
  const { data: server, isLoading } = useQuery({ queryKey: ["server", serverId], queryFn: () => fetchServer(serverId) });
  const deleteMut = useMutation({ mutationFn: () => deleteServer(serverId, false), onSuccess: () => { qc.invalidateQueries({ queryKey: ["servers"] }); onClose(); } });
  const forceDeleteMut = useMutation({ mutationFn: () => deleteServer(serverId, true), onSuccess: () => { qc.invalidateQueries({ queryKey: ["servers"] }); onClose(); } });
  const suspendMut = useMutation({ mutationFn: () => suspendServer(serverId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server", serverId] }) });
  const unsuspendMut = useMutation({ mutationFn: () => unsuspendServer(serverId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server", serverId] }) });
  const reinstallMut = useMutation({ mutationFn: () => reinstallServer(serverId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server", serverId] }) });

  if (isLoading || !server) return <div className="p-8 text-center text-sm text-slate-500">Loading…</div>;
  const installed = server.status !== "installing";

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-bold text-white">{server.name}</h2>
          <p className="text-xs text-slate-400">{server.description ?? ""}</p>
        </div>
        <a className="flex items-center gap-1 text-xs text-sky-400 hover:underline" href={`/server/${server.id}`} target="_blank" rel="noreferrer">
          Open console <ExternalLink size={12} />
        </a>
      </div>
      <div className="flex gap-1 border-b border-white/[0.06] overflow-x-auto" role="tablist" aria-label="Server sections">
        {SERVER_TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={tab === t.id}
            disabled={!installed && t.id !== "about" && t.id !== "manage" && t.id !== "delete"}
            className={cn(
              "whitespace-nowrap px-3 py-2 text-sm font-medium transition",
              tab === t.id
                ? t.danger
                  ? "border-b-2 border-red-500 text-red-400"
                  : "border-b-2 border-[#dc2626] text-[#dc2626]"
                : "text-slate-400 hover:text-slate-200",
              "disabled:opacity-50"
            )}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === "about" && <ServerAboutTab server={server} users={users} nodes={nodes} allocations={allocations} />}
      {tab === "details" && <ServerDetailsTab server={server} users={users} />}
      {tab === "build" && <ServerBuildTab server={server} users={users} allocations={allocations} />}
      {tab === "startup" && <ServerStartupTab server={server} />}
      {tab === "allocations" && <ServerAllocationsTab server={server} allocations={allocations} />}
      {tab === "database" && <ServerDatabaseTab serverId={serverId} />}
      {tab === "mounts" && <ServerMountsTab server={server} mounts={mounts} />}
      {tab === "manage" && (
        <ServerManageTab
          server={server}
          reinstallMut={reinstallMut}
          suspendMut={suspendMut}
          unsuspendMut={unsuspendMut}
          nodes={nodes}
          allocations={allocations}
        />
      )}
      {tab === "delete" && <ServerDeleteTab deleteMut={deleteMut} forceDeleteMut={forceDeleteMut} serverName={server.name} />}
    </div>
  );
}

function InfoRow({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <li className="flex items-center justify-between px-4 py-3 text-sm">
      <span className="text-slate-400">{label}</span>
      <span className={cn("text-slate-200", mono && "font-mono text-xs")}>{value}</span>
    </li>
  );
}

function ServerAboutTab({ server, users, nodes, allocations }: { server: ApiServer; users: ApiUser[]; nodes: ApiNode[]; allocations: ApiAllocation[] }) {
  const owner = users.find((u) => u.id === server.owner)?.email ?? server.owner ?? "—";
  const node = nodes.find((n) => n.id === server.node)?.name ?? server.node ?? "—";
  const alloc = allocations.find((a) => a.id === server.allocation);
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card>
        <CardHeader title="Information" icon={Info} />
        <ul className="divide-y divide-white/[0.04]">
          <InfoRow label="Internal ID" value={server.id.slice(0, 8) + "…"} mono />
          <InfoRow label="UUID" value={server.uuid ?? server.id} mono />
          <InfoRow label="Server Name" value={server.name} />
          <InfoRow label="Memory" value={server.memoryMb != null ? `${server.memoryMb} MiB` : "—"} />
          <InfoRow label="Disk" value={server.diskMb != null ? `${server.diskMb} MiB` : "—"} />
          <InfoRow label="Default Connection" value={alloc ? `${alloc.ip}:${alloc.port}` : "—"} mono />
        </ul>
      </Card>
      <div className="space-y-3">
        <SmallBox color={server.suspended ? "red" : "blue"} label={server.suspended ? "Suspended" : "Status"} value={server.suspended ? "YES" : server.status} />
        <SmallBox color="purple" label="Owner" value={owner} />
        <SmallBox color="emerald" label="Node" value={node} />
      </div>
    </div>
  );
}

function SmallBox({ color, label, value }: { color: "orange" | "blue" | "purple" | "emerald" | "red"; label: string; value: string }) {
  const map: Record<string, string> = {
    orange: "bg-orange-500/10 text-orange-400 border-orange-500/30",
    blue: "bg-sky-500/10 text-sky-400 border-sky-500/30",
    purple: "bg-purple-500/10 text-purple-400 border-purple-500/30",
    emerald: "bg-emerald-500/10 text-emerald-400 border-emerald-500/30",
    red: "bg-red-500/10 text-red-400 border-red-500/30",
  };
  return (
    <div className={cn("rounded-lg border p-4", map[color])}>
      <div className="text-[10px] font-bold uppercase tracking-widest opacity-70">{label}</div>
      <div className="mt-1 text-lg font-bold">{value}</div>
    </div>
  );
}

function ServerDetailsTab({ server, users }: { server: ApiServer; users: ApiUser[] }) {
  const qc = useQueryClient();
  const currentOwnerId = server.ownerId ?? users.find((user) => user.id === server.owner || user.email === server.owner)?.id ?? "";
  const [name, setName] = useState(server.name);
  const [description, setDescription] = useState(server.description ?? "");
  const [ownerId, setOwnerId] = useState(currentOwnerId);
  const [userSearch, setUserSearch] = useState("");
  const { data: userResults } = useQuery<ApiUser[]>({
    queryKey: ["user-search", userSearch],
    queryFn: () => searchUsers(userSearch),
    enabled: userSearch.length > 0,
  });
  const canSafelyUpdate = Boolean(ownerId && server.memoryMb !== undefined && server.cpuShares !== undefined && server.diskMb !== undefined);
  const saveMut = useMutation({
    mutationFn: () => {
      if (!canSafelyUpdate) throw new Error("Current server resource values are unavailable.");
      return updateServer(server.id, {
        name: name.trim(),
        description,
        ownerId,
        memoryMb: server.memoryMb!,
        cpuShares: server.cpuShares!,
        diskMb: server.diskMb!,
      });
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["server", server.id] }); void qc.invalidateQueries({ queryKey: ["servers"] }); },
  });
  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (canSafelyUpdate && name.trim()) saveMut.mutate(); }}>
      <Card>
        <CardHeader title="Base Information" icon={Info} />
        <div className="space-y-3 p-4">
          <Input label="Server Name" value={name} onChange={setName} required />
          <Textarea label="Description" value={description} onChange={setDescription} rows={3} />
          <Input label="Owner Email Search" value={userSearch} onChange={setUserSearch} placeholder="Search by email or username…" />
          {userResults && userResults.length > 0 && (
            <ul className="max-h-40 overflow-y-auto rounded border border-white/10 bg-[#0a0e16] p-2 text-sm">
              {userResults.map((u) => (
                <li key={u.id}>
                  <button
                    type="button"
                    className={cn("w-full rounded px-2 py-1 text-left hover:bg-white/5", ownerId === u.id && "bg-[#dc2626]/20")}
                    onClick={() => { setOwnerId(u.id); setUserSearch(""); }}
                  >
                    {u.email} <span className="text-xs text-slate-400">({u.username})</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {ownerId && <div className="text-xs text-slate-400">Selected owner ID: <span className="font-mono">{ownerId}</span></div>}
        </div>
      </Card>
      {!canSafelyUpdate && <p className="text-sm text-amber-300">Details cannot be updated safely because the API response is missing the current owner or resource baseline required by the backend PATCH endpoint.</p>}
      {saveMut.error && <p className="text-sm text-red-300">{saveMut.error instanceof Error ? saveMut.error.message : "Server update failed."}</p>}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={!canSafelyUpdate || !name.trim() || saveMut.isPending}>
          {saveMut.isPending ? "Saving…" : "Update Details"}
        </Btn>
      </div>
    </form>
  );
}

function ServerBuildTab({ server, users, allocations }: { server: ApiServer; users: ApiUser[]; allocations: ApiAllocation[] }) {
  const qc = useQueryClient();
  const ownerId = server.ownerId ?? users.find((user) => user.id === server.owner || user.email === server.owner)?.id ?? "";
  const [memory, setMemory] = useState(server.memoryMb === undefined ? "" : String(server.memoryMb));
  const [disk, setDisk] = useState(server.diskMb === undefined ? "" : String(server.diskMb));
  const [cpuShares, setCpuShares] = useState(server.cpuShares === undefined ? "" : String(server.cpuShares));
  const serverAllocs = allocations.filter((allocation) => allocation.server === server.id);
  const currentAllocationId = server.allocationId ?? (serverAllocs.some((allocation) => allocation.id === server.allocation) ? server.allocation : "") ?? "";
  const [allocationId, setAllocationId] = useState(currentAllocationId);
  const numericValues = [memory, disk, cpuShares].map(Number);
  const canSafelyUpdate = Boolean(ownerId && server.name.trim() && numericValues.every((value) => Number.isFinite(value) && value > 0));
  const saveMut = useMutation({
    mutationFn: () => {
      if (!canSafelyUpdate) throw new Error("Current owner and positive resource values are required.");
      return updateServer(server.id, {
        name: server.name,
        description: server.description ?? "",
        ownerId,
        memoryMb: Number(memory),
        cpuShares: Number(cpuShares),
        diskMb: Number(disk),
        primaryAllocationId: allocationId || undefined,
      });
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["server", server.id] }); void qc.invalidateQueries({ queryKey: ["servers"] }); },
  });
  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (canSafelyUpdate) saveMut.mutate(); }}>
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader title="Resource Management" icon={Cpu} />
          <div className="space-y-3 p-4">
            <Input label="CPU Shares" value={cpuShares} onChange={setCpuShares} type="number" />
            <Input label="Memory (MiB)" value={memory} onChange={setMemory} type="number" />
            <Input label="Disk Space (MiB)" value={disk} onChange={setDisk} type="number" />
            <p className="text-xs text-amber-300">CPU percentage limits are unavailable: the backend accepts cpuLimit but does not persist it.</p>
          </div>
        </Card>
        <div className="space-y-4">
          <Card>
            <CardHeader title="Application Feature Limits" icon={Layers} />
            <p className="p-4 text-sm text-amber-300">Database, backup, and allocation limits are read-only here because the current backend PATCH endpoint does not persist them.</p>
          </Card>
          <Card>
            <CardHeader title="Allocation Management" icon={Network} />
            <div className="space-y-3 p-4">
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-slate-400">Default Allocation</span>
                <select className="h-10 w-full rounded-lg border border-white/10 bg-[#141824] px-3 text-slate-100" value={allocationId} onChange={(event) => setAllocationId(event.target.value)}>
                  <option value="">Keep current allocation</option>
                  {serverAllocs.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}
                </select>
              </label>
              <p className="text-xs text-slate-400">Only allocations already assigned to this server can become primary.</p>
            </div>
          </Card>
        </div>
      </div>
      {!canSafelyUpdate && <p className="text-sm text-amber-300">Build settings cannot be updated until the current owner and resource baseline are available.</p>}
      {saveMut.error && <p className="text-sm text-red-300">{saveMut.error instanceof Error ? saveMut.error.message : "Build update failed."}</p>}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={!canSafelyUpdate || saveMut.isPending}>
          {saveMut.isPending ? "Saving…" : "Update Build Configuration"}
        </Btn>
      </div>
    </form>
  );
}

function ServerStartupTab({ server }: { server: ApiServer }) {
  const qc = useQueryClient();
  const { data: startup } = useQuery({ queryKey: ["server-startup", server.id], queryFn: () => fetchServerStartup(server.id) });
  const [vars, setVars] = useState<Record<string, string>>({});
  const imageEntries = Object.entries(startup?.docker_images ?? {});
  const updateVar = (name: string, value: string) => setVars((current) => ({ ...current, [name]: value }));
  const saveMut = useMutation({
    mutationFn: async () => {
      const changedVariables = Object.entries(vars);
      if (changedVariables.length === 0) throw new Error("No startup variable changes to save.");
      for (const [key, value] of changedVariables) await updateServerStartupVariable(server.id, key, value);
    },
    onSuccess: () => {
      setVars({});
      void qc.invalidateQueries({ queryKey: ["server-startup", server.id] });
    },
  });
  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (Object.keys(vars).length > 0) saveMut.mutate(); }}>
      <Card>
        <CardHeader title="Startup Command" icon={Zap} />
        <div className="space-y-3 p-4">
          <label className="block text-sm font-medium text-slate-300">Resolved Startup Command<input className="mt-1.5 h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 font-mono text-xs text-slate-400" readOnly value={startup?.startup_command ?? ""} /></label>
          <label className="block text-sm font-medium text-slate-300">Raw Startup Command<input className="mt-1.5 h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 font-mono text-xs text-slate-400" readOnly value={startup?.raw_startup_command ?? ""} /></label>
          <p className="text-xs text-amber-300">Startup command editing is unavailable because the current backend PATCH endpoint does not persist it safely.</p>
        </div>
      </Card>
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader title="Template" icon={Box} />
          <div className="space-y-3 p-4">
            <p className="text-sm text-slate-300">{server.template || "—"}</p>
          </div>
        </Card>
        <Card>
          <CardHeader title="Available Docker Images" icon={Box} />
          <div className="space-y-2 p-4 text-sm text-slate-300">
            {imageEntries.length === 0 ? <p className="text-slate-500">No Docker images were reported by this template.</p> : imageEntries.map(([label, image]) => <div key={image} className="rounded border border-white/[0.06] bg-[#161b28] px-3 py-2"><span className="block text-xs text-slate-500">{label || "Image"}</span><code className="break-all text-xs">{image}</code></div>)}
          </div>
        </Card>
      </div>
      {startup?.variables && Array.isArray(startup.variables) && startup.variables.length > 0 && (
        <Card>
          <CardHeader title="Service Variables" icon={KeyRound} />
          <div className="space-y-3 p-4">
            {startup.variables.map((variable: any) => (
              <label className="block text-sm font-medium text-slate-300" key={variable.env_variable}>
                <span className="mb-1.5 block">{variable.name} ({variable.env_variable})</span>
                <input
                  className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={!variable.is_editable}
                  onChange={(event) => updateVar(variable.env_variable, event.target.value)}
                  value={vars[variable.env_variable] ?? variable.server_value}
                />
                {variable.description && <span className="mt-1 block text-xs text-slate-500">{variable.description}</span>}
              </label>
            ))}
          </div>
        </Card>
      )}
      {saveMut.error && <p className="text-sm text-red-300">{saveMut.error instanceof Error ? saveMut.error.message : "Startup variable update failed."}</p>}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={Object.keys(vars).length === 0 || saveMut.isPending}>
          {saveMut.isPending ? "Saving…" : "Save Variable Changes"}
        </Btn>
      </div>
    </form>
  );
}

function ServerAllocationsTab({ server, allocations }: { server: ApiServer; allocations: ApiAllocation[] }) {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["server-allocations", server.id], queryFn: () => fetchServerAllocations(server.id) });
  const assigned = query.data ?? [];
  const assignedIds = new Set(assigned.map((allocation) => allocation.id));
  const available = allocations.filter((allocation) => !allocation.server && !assignedIds.has(allocation.id) && (allocation.node === server.nodeId || allocation.node === server.node));
  const [allocationId, setAllocationId] = useState("");
  const refresh = () => { void qc.invalidateQueries({ queryKey: ["server-allocations", server.id] }); void qc.invalidateQueries({ queryKey: ["allocations"] }); void qc.invalidateQueries({ queryKey: ["server", server.id] }); };
  const assignMut = useMutation({ mutationFn: () => assignServerAllocation(server.id, allocationId), onSuccess: () => { setAllocationId(""); refresh(); } });
  const unassignMut = useMutation({ mutationFn: (id: string) => unassignServerAllocation(server.id, id), onSuccess: refresh });
  const primaryMut = useMutation({ mutationFn: (id: string) => setPrimaryServerAllocation(server.id, id), onSuccess: refresh });
  const primaryId = server.primaryAllocationId ?? server.allocationId;
  return <div className="space-y-4"><Card><CardHeader title="Assigned Allocations" icon={Network}/>{assigned.length === 0 ? <EmptyState icon={Network} message="No allocations assigned."/> : <div className="overflow-x-auto"><table className="w-full text-sm"><tbody className="divide-y divide-white/[0.04]">{assigned.map((allocation) => { const primary = allocation.id === primaryId || allocation.primary || allocation.isPrimary; return <tr key={allocation.id}><td className="px-4 py-3 font-mono text-xs">{allocation.ip}:{allocation.port}</td><td className="px-4 py-3">{primary ? <Pill tone="green">Primary</Pill> : <Btn size="sm" tone="ghost" onClick={() => primaryMut.mutate(allocation.id)}>Make primary</Btn>}</td><td className="px-4 py-3 text-right"><Btn size="sm" tone="danger" disabled={Boolean(primary) || unassignMut.isPending} onClick={() => { if (confirm("Unassign this allocation?")) unassignMut.mutate(allocation.id); }}>Unassign</Btn></td></tr>; })}</tbody></table></div>}</Card><Card><CardHeader title="Assign Allocation" icon={Plus}/><div className="flex flex-col gap-3 p-4 sm:flex-row"><select className="h-9 flex-1 rounded border border-white/10 bg-[#161b28] px-3 text-sm" value={allocationId} onChange={(event) => setAllocationId(event.target.value)}><option value="">Select an unassigned allocation…</option>{available.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}</select><Btn disabled={!allocationId || assignMut.isPending} onClick={() => assignMut.mutate()}>Assign</Btn></div></Card></div>;
}

function ServerDatabaseTab({ serverId }: { serverId: string }) {
  const qc = useQueryClient();
  const { data: dbs = [] } = useQuery({ queryKey: ["server-dbs", serverId], queryFn: () => fetchServerDatabases(serverId) });
  const [dbName, setDbName] = useState("");
  const [remote, setRemote] = useState("%");
  const createMut = useMutation({
    mutationFn: () => createServerDatabase(serverId, { database: dbName.trim(), remote: remote.trim() || "%" } as any),
    onSuccess: () => { setDbName(""); void qc.invalidateQueries({ queryKey: ["server-dbs", serverId] }); },
  });
  const rotateMut = useMutation({ mutationFn: (dbId: string) => rotateServerDatabasePasswordByBody(serverId, dbId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server-dbs", serverId] }) });
  const deleteMut = useMutation({ mutationFn: (dbId: string) => deleteServerDatabaseWithSuffix(serverId, dbId), onSuccess: () => qc.invalidateQueries({ queryKey: ["server-dbs", serverId] }) });
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader title="Active Databases" icon={Database} />
        {dbs.length === 0 ? (
          <EmptyState icon={Database} message="No databases for this server." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                <th className="px-4 py-2">Database</th>
                <th className="px-4 py-2">Username</th>
                <th className="px-4 py-2">Host</th>
                <th className="px-4 py-2">Remote</th>
                <th className="px-4 py-2 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {dbs.map((d) => (
                <tr key={d.id} className="border-b border-white/[0.04]">
                  <td className="px-4 py-2 font-mono text-xs">{d.database}</td>
                  <td className="px-4 py-2 font-mono text-xs">{d.username}</td>
                  <td className="px-4 py-2 font-mono text-xs">{d.host ?? "—"}</td>
                  <td className="px-4 py-2 text-xs">{d.remote}</td>
                  <td className="px-4 py-2 text-right space-x-2">
                    <Btn tone="ghost" onClick={() => { if (confirm("Rotate password?")) rotateMut.mutate(d.id); }}>
                      Rotate
                    </Btn>
                    <Btn tone="danger" onClick={() => { if (confirm("Delete database?")) deleteMut.mutate(d.id); }}>
                      Delete
                    </Btn>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      <Card>
        <CardHeader title="Create New Database" icon={Plus} />
        <form
          className="space-y-3 p-4"
          onSubmit={(e) => { e.preventDefault(); createMut.mutate(); }}
        >
          <p className="text-xs text-slate-400">The backend selects the database host automatically; server database creation does not accept a host ID.</p>
          <Input label="Database Name" value={dbName} onChange={setDbName} placeholder={`s${serverId.slice(0, 6)}_name`} required />
          <Input label="Connections From" value={remote} onChange={setRemote} />
          {createMut.error && <p className="text-sm text-red-300">{createMut.error instanceof Error ? createMut.error.message : "Database creation failed."}</p>}
          <Btn tone="primary" type="submit" disabled={!dbName.trim() || createMut.isPending}>
            {createMut.isPending ? "Creating…" : "Create Database"}
          </Btn>
        </form>
      </Card>
    </div>
  );
}

function ServerMountsTab({ server, mounts }: { server: ApiServer; mounts: ApiMount[] }) {
  const qc = useQueryClient();
  const serverId = server.id;
  const { data: serverMounts = [], isError: isMountsError, error: mountsError } = useQuery({ queryKey: ["server-mounts", serverId], queryFn: () => fetchServerMounts(serverId) });
  const mountIds = new Set(serverMounts.map((mount) => mount.id));
  const nodeId = server.nodeId;
  const eggId = server.template;
  const eligibleMounts = nodeId && eggId
    ? mounts.filter((mount) => (mount.nodeIds ?? []).includes(nodeId) && (mount.templateIds ?? []).includes(eggId))
    : [];
  const [syncNotice, setSyncNotice] = useState<string | null>(null);
  const refreshMountState = () => {
    void qc.invalidateQueries({ queryKey: ["server-mounts", serverId] });
    void qc.invalidateQueries({ queryKey: ["server", serverId] });
  };
  const attachMut = useMutation({
    mutationFn: (mountId: string) => assignServerMount(serverId, mountId),
    onSuccess: (result) => { setSyncNotice(result.runtimeSynchronized ? "Mount assignment synchronized with the runtime." : "Mount assignment is pending runtime synchronization."); refreshMountState(); },
    onError: refreshMountState,
  });
  const detachMut = useMutation({
    mutationFn: (mountId: string) => removeServerMount(serverId, mountId),
    onSuccess: (result) => { setSyncNotice(result.runtimeSynchronized ? "Mount removal synchronized with the runtime." : "Mount removal is pending runtime synchronization."); refreshMountState(); },
    onError: refreshMountState,
  });
  const actionError = attachMut.error ?? detachMut.error;
  const errorText = (error: unknown, fallback: string) => error instanceof Error ? error.message : fallback;
  const eligibilityGuidance = !server.nodeId || !server.template
    ? "This server is missing node or egg information, so no mounts can be selected. Assign both before attaching a mount."
    : "Create a mount, then attach this server's node and egg to it before it can be assigned here.";

  return (
    <Card>
      <CardHeader title="Eligible Mounts" icon={HardDrive} />
      <div className="border-b border-white/[0.06] px-4 py-3 text-xs text-slate-400">
        Only mounts attached to this server's node and egg are shown. Assignment makes the mount available to the server; it does not confirm a runtime mount.
      </div>
      {server.configSyncPending ? <div className="m-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-100" role="status">Mount configuration is pending runtime synchronization{server.configSyncError ? `: ${server.configSyncError}` : "."}</div> : null}
      {syncNotice ? <div className="m-4 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 text-sm text-emerald-100" role="status">{syncNotice}</div> : null}
      {isMountsError ? <div className="m-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">{errorText(mountsError, "Server mount assignments could not be loaded.")}</div> : null}
      {actionError ? <div className="m-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">{errorText(actionError, "Mount assignment failed. The persisted change may be pending runtime synchronization.")}</div> : null}
      {eligibleMounts.length === 0 ? (
        <div className="p-6 text-sm text-slate-400">
          <p className="font-medium text-slate-200">No eligible mounts</p>
          <p className="mt-1">{eligibilityGuidance}</p>
        </div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
              <th className="px-4 py-2">ID</th>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Source</th>
              <th className="px-4 py-2">Target</th>
              <th className="px-4 py-2">Assignment</th>
              <th className="px-4 py-2 text-right">Action</th>
            </tr>
          </thead>
          <tbody>
            {eligibleMounts.map((mount) => {
              const assigned = mountIds.has(mount.id);
              return (
                <tr key={mount.id} className="border-b border-white/[0.04]">
                  <td className="px-4 py-2 font-mono text-xs">{mount.id.slice(0, 8)}…</td>
                  <td className="px-4 py-2 font-semibold">{mount.name}</td>
                  <td className="px-4 py-2 font-mono text-xs">{mount.source}</td>
                  <td className="px-4 py-2 font-mono text-xs">{mount.target}</td>
                  <td className="px-4 py-2">
                    {assigned ? <Pill tone="green">Assigned</Pill> : <Pill tone="blue">Not assigned</Pill>}
                  </td>
                  <td className="px-4 py-2 text-right">
                    {assigned ? (
                      <Btn tone="danger" disabled={detachMut.isPending} onClick={() => detachMut.mutate(mount.id)}>{detachMut.isPending ? "Unassigning…" : "Unassign"}</Btn>
                    ) : (
                      <Btn tone="primary" disabled={attachMut.isPending} onClick={() => attachMut.mutate(mount.id)}>{attachMut.isPending ? "Assigning…" : "Assign"}</Btn>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </Card>
  );
}

function ServerManageTab({ server, reinstallMut, suspendMut, unsuspendMut, nodes, allocations }: { server: ApiServer; reinstallMut: { mutate: () => void; isPending: boolean }; suspendMut: { mutate: () => void; isPending: boolean }; unsuspendMut: { mutate: () => void; isPending: boolean }; nodes: ApiNode[]; allocations: ApiAllocation[] }) {
  const qc = useQueryClient();
  const transferQuery = useQuery({ queryKey: ["server-transfer", server.id], queryFn: () => fetchServerTransferStatus(server.id), retry: false, refetchInterval: server.transferring ? 5000 : false });
  const [targetNodeId, setTargetNodeId] = useState(server.transferTargetNodeId ?? "");
  const [primaryAllocationId, setPrimaryAllocationId] = useState("");
  const targetAllocations = allocations.filter((allocation) => allocation.node === targetNodeId && !allocation.server);
  const transferMut = useMutation({ mutationFn: () => transferServer(server.id, { targetNodeId, primaryAllocationId } as any), onSuccess: () => { void transferQuery.refetch(); void qc.invalidateQueries({ queryKey: ["server", server.id] }); } });
  const cancelMut = useMutation({ mutationFn: () => cancelServerTransfer(server.id), onSuccess: () => { void transferQuery.refetch(); void qc.invalidateQueries({ queryKey: ["server", server.id] }); } });
  const transfer = transferQuery.data;
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader title="Reinstall Server" icon={RefreshCw} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-slate-400">Re-runs the egg install script. Container will be destroyed and recreated.</p>
          <Btn tone="danger" onClick={() => { if (confirm("Reinstall this server?")) reinstallMut.mutate(); }} disabled={reinstallMut.isPending}>
            {reinstallMut.isPending ? "Reinstalling…" : "Reinstall Server"}
          </Btn>
        </div>
      </Card>
      <Card>
        <CardHeader title="Suspension" icon={Ban} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-slate-400">Suspending stops the container and blocks user access until unsuspended.</p>
          {server.suspended ? (
            <Btn tone="primary" onClick={() => unsuspendMut.mutate()} disabled={unsuspendMut.isPending}>
              {unsuspendMut.isPending ? "Unsuspending…" : "Unsuspend Server"}
            </Btn>
          ) : (
            <Btn tone="warning" onClick={() => suspendMut.mutate()} disabled={suspendMut.isPending}>
              {suspendMut.isPending ? "Suspending…" : "Suspend Server"}
            </Btn>
          )}
        </div>
      </Card>
      <Card>
        <CardHeader title="Transfer" icon={Network} action={transfer ? <Pill tone={transfer.error ? "red" : transfer.transferring ? "yellow" : "blue"}>{transfer.state}</Pill> : undefined}/>
        <div className="space-y-3 p-4">
          {transfer?.error ? <p className="text-xs text-red-300">{transfer.error}</p> : null}
          <label className="block text-xs text-slate-400">Target node<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100" value={targetNodeId} onChange={(event) => { setTargetNodeId(event.target.value); setPrimaryAllocationId(""); }}><option value="">Select…</option>{nodes.filter((node) => node.id !== server.nodeId && node.name !== server.node).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
          <label className="block text-xs text-slate-400">Primary allocation<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100" value={primaryAllocationId} onChange={(event) => setPrimaryAllocationId(event.target.value)}><option value="">Select…</option>{targetAllocations.map((allocation) => <option key={allocation.id} value={allocation.id}>{allocation.ip}:{allocation.port}</option>)}</select></label>
          {transfer?.transferring ? <Btn tone="danger" disabled={cancelMut.isPending} onClick={() => { if (confirm("Cancel this transfer?")) cancelMut.mutate(); }}>Cancel Transfer</Btn> : <Btn disabled={!targetNodeId || !primaryAllocationId || transferMut.isPending} onClick={() => { if (confirm("Start this server transfer?")) transferMut.mutate(); }}>Start Transfer</Btn>}
        </div>
      </Card>
    </div>
  );
}

function ServerDeleteTab({ deleteMut, forceDeleteMut, serverName }: { deleteMut: { mutate: () => void; isPending: boolean }; forceDeleteMut: { mutate: () => void; isPending: boolean }; serverName: string }) {
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card>
        <CardHeader title="Safely Delete" icon={Trash2} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-slate-400">Removes the server record. Container and files are cleaned up by the daemon asynchronously.</p>
          <Btn tone="danger" onClick={() => { if (confirm(`Safely delete ${serverName}?`)) deleteMut.mutate(); }} disabled={deleteMut.isPending}>
            {deleteMut.isPending ? "Deleting…" : "Safely Delete"}
          </Btn>
        </div>
      </Card>
      <Card>
        <CardHeader title="Force Delete" icon={AlertTriangle} action={<Pill tone="red">Danger</Pill>} />
        <div className="space-y-3 p-4">
          <p className="text-xs text-red-400">Bypasses daemon cleanup. Files may be orphaned on the node.</p>
          <Btn tone="danger" onClick={() => { if (confirm(`FORCE delete ${serverName}? Files may be orphaned.`)) forceDeleteMut.mutate(); }} disabled={forceDeleteMut.isPending}>
            {forceDeleteMut.isPending ? "Deleting…" : "Forcibly Delete"}
          </Btn>
        </div>
      </Card>
    </div>
  );
}
