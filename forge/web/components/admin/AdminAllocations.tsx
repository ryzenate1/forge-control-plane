"use client";

import { useState, useMemo, useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, ArrowDownUp, Check, Copy, Edit3, Globe, Network, Plus, Server, Trash2 } from "lucide-react";
import type { ApiAllocation, ApiAllocationNode } from "@/lib/api";
import { createAllocation, deleteAllocations, fetchAllocationNodes, fetchAllocations, setAdminAllocationAlias } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, StatsRow } from "./admin-ui";

const EMPTY_NODES: ApiAllocationNode[] = [];
const EMPTY_ALLOCATIONS: ApiAllocation[] = [];

type SortKey = "node" | "ip" | "port" | "alias" | "server";

function validateCreateInput(ip: string, ports: string): string | null {
  const address = ip.trim();
  const isIPv4 = address.split(".").length === 4 && address.split(".").every((part) => /^\d+$/.test(part) && Number(part) <= 255);
  const isIPv6 = address.includes(":") && /^[0-9a-fA-F:.]+$/.test(address);
  if (!isIPv4 && !isIPv6) return "Enter a valid IPv4 or IPv6 address.";
  const values = ports.trim().split(/[\s,]+/).filter(Boolean);
  if (values.length === 0) return "Enter at least one port.";
  let count = 0;
  for (const value of values) {
    const match = value.match(/^(\d+)(?:-(\d+))?$/);
    if (!match) return `Invalid port expression: ${value}`;
    const start = Number(match[1]);
    const end = Number(match[2] ?? match[1]);
    if (start < 1 || end > 65535 || end < start) return `Invalid port range: ${value}`;
    count += end - start + 1;
    if (count > 2000) return "A request can contain at most 2,000 ports.";
  }
  return null;
}

export function AdminAllocations() {
  const qc = useQueryClient();
  const { toast } = useToast();

  const [modal, setModal] = useState(false);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "free" | "used">("all");
  const [nodeFilter, setNodeFilter] = useState("all");
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [sortKey, setSortKey] = useState<SortKey>("port");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");

  const [editing, setEditing] = useState<ApiAllocation | null>(null);
  const [editAlias, setEditAlias] = useState("");
  const [editError, setEditError] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [createError, setCreateError] = useState<string | null>(null);

  const [nodeId, setNodeId] = useState("");
  const [ip, setIp] = useState("0.0.0.0");
  const [ports, setPorts] = useState("25565");
  const [containerPort, setContainerPort] = useState("");
  const [protocol, setProtocol] = useState<"tcp" | "udp">("tcp");
  const [alias, setAlias] = useState("");
  const [notes, setNotes] = useState("");

  const nodesQuery = useQuery({ queryKey: ["allocation-nodes"], queryFn: fetchAllocationNodes });
  const allocationsQuery = useQuery({ queryKey: ["allocations"], queryFn: fetchAllocations });
  const nodes = nodesQuery.data ?? EMPTY_NODES;
  const allocations = allocationsQuery.data ?? EMPTY_ALLOCATIONS;

  const createMut = useMutation({
    mutationFn: () => createAllocation({ nodeId: nodeId || nodes[0]?.id || "", ip, ports, containerPort: containerPort ? Number(containerPort) : undefined, protocol, alias, notes }),
    onSuccess: (created) => { qc.invalidateQueries({ queryKey: ["allocations"] }); setModal(false); setCreateError(null); toast({ tone: "success", title: `${created.length} allocation${created.length === 1 ? "" : "s"} created` }); },
    onError: (e: Error) => { setCreateError(e.message || "Unknown error"); toast({ tone: "error", title: "Failed to create allocations", message: e.message || "Unknown error" }); },
  });

  const editMut = useMutation({
    mutationFn: ({ id, alias }: { id: string; alias: string }) => setAdminAllocationAlias(id, alias),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["allocations"] }); setEditing(null); setEditError(null); toast({ tone: "success", title: "Allocation alias updated" }); },
    onError: (error: Error) => { const message = error.message || "Unknown error"; setEditError(message); toast({ tone: "error", title: "Failed to update allocation alias", message }); },
  });

  const bulkDeleteMut = useMutation({
    mutationFn: (ids: string[]) => deleteAllocations(ids),
    onSuccess: (_, ids) => { void qc.invalidateQueries({ queryKey: ["allocations"] }); setSelectedIds((current) => current.filter((id) => !ids.includes(id))); setDeleteError(null); toast({ tone: "success", title: `${ids.length} allocation${ids.length === 1 ? "" : "s"} deleted` }); },
    onError: (error: Error) => { const message = error.message || "Unknown error"; setDeleteError(message); toast({ tone: "error", title: "Failed to delete allocations", message }); },
  });

  const free = allocations.filter((a) => !a.server);
  const used = allocations.filter((a) => a.server);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = allocations.filter((a) => {
      const matchesSearch = !q || a.ip.includes(q) || String(a.port).includes(q) || (a.node ?? "").toLowerCase().includes(q) || (a.server ?? "").toLowerCase().includes(q) || (a.alias ?? "").toLowerCase().includes(q);
      const matchesStatus = statusFilter === "all" || (statusFilter === "free" ? !a.server : Boolean(a.server));
      const matchesNode = nodeFilter === "all" || (nodes.find((node) => node.id === nodeFilter)?.name === a.node);
      return matchesSearch && matchesStatus && matchesNode;
    });
    list.sort((a, b) => {
      let cmp = 0;
      if (sortKey === "port") cmp = a.port - b.port;
      else if (sortKey === "ip") cmp = a.ip.localeCompare(b.ip);
      else if (sortKey === "node") cmp = (a.node ?? "").localeCompare(b.node ?? "");
      else if (sortKey === "alias") cmp = (a.alias ?? "").localeCompare(b.alias ?? "");
      else if (sortKey === "server") cmp = (a.server ?? "").localeCompare(b.server ?? "");
      return sortDir === "asc" ? cmp : -cmp;
    });
    return list;
  }, [allocations, search, statusFilter, nodeFilter, nodes, sortKey, sortDir]);

  const visibleFreeIds = useMemo(() => filtered.filter((a) => !a.server).map((a) => a.id), [filtered]);
  const selectedFreeIds = selectedIds.filter((id) => !allocations.find((a) => a.id === id)?.server);
  const allVisibleFreeSelected = visibleFreeIds.length > 0 && visibleFreeIds.every((id) => selectedIds.includes(id));

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    else { setSortKey(key); setSortDir("asc"); }
  };

  const toggleSelected = (id: string) => setSelectedIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);

  const toggleSelectAllVisible = () => {
    setDeleteError(null);
    if (allVisibleFreeSelected) setSelectedIds((current) => current.filter((id) => !visibleFreeIds.includes(id)));
    else setSelectedIds((current) => [...new Set([...current, ...visibleFreeIds])]);
  };

  const openEdit = (allocation: ApiAllocation) => { setEditing(allocation); setEditAlias(allocation.alias ?? ""); setEditError(null); };
  const confirmDelete = (ids: string[]) => {
    if (ids.length === 0 || !window.confirm(`Delete ${ids.length} free allocation${ids.length === 1 ? "" : "s"}? This cannot be undone.`)) return;
    setDeleteError(null);
    bulkDeleteMut.mutate(ids);
  };
  const confirmSingleDelete = (id: string) => confirmDelete([id]);

  const exportCSV = useCallback(() => {
    const rows = filtered.map((a) => `"${a.node}","${a.ip}",${a.port},"${a.protocol ?? "tcp"}","${a.containerPort ?? ""}","${a.alias ?? ""}","${a.server ?? ""}"`).join("\n");
    const text = `Node,IP,Port,Protocol,Container Port,Alias,Server\n${rows}`;
    navigator.clipboard.writeText(text).then(() => toast({ tone: "success", title: "Copied to clipboard" }), () => toast({ tone: "error", title: "Failed to copy" }));
  }, [filtered, toast]);

  const SortIcon = ({ column }: { column: SortKey }) => {
    if (sortKey !== column) return <ArrowDownUp size={10} className="ml-1 opacity-0 group-hover:opacity-40" />;
    return <ArrowDownUp size={10} className={`ml-1 ${sortDir === "asc" ? "rotate-180" : ""} text-brand`} />;
  };

  const selectStyle = "h-10 w-full rounded-lg border border-white/10 bg-surface-card-header px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15";

  const thClass = "sticky top-0 bg-surface-card-header px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer select-none group";

  return (
    <div>
      <SectionHeader
        title="Allocations"
        sub="IP:port bindings available to servers."
        action={<Btn disabled={nodesQuery.isLoading || nodesQuery.isError} onClick={() => { setNodeId(nodes[0]?.id ?? ""); setCreateError(null); setModal(true); }}><Plus size={14} /> Create Allocations</Btn>}
      />

      {nodesQuery.isError ? (
        <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
          <span>Could not load allocation nodes: {nodesQuery.error.message}</span>
          <Btn size="sm" tone="ghost" onClick={() => nodesQuery.refetch()}>Retry</Btn>
        </div>
      ) : null}

      <StatsRow items={[
        { label: "Total", value: allocations.length, icon: Network, tone: "neutral" },
        { label: "In use", value: used.length, icon: Server, tone: "blue" },
        { label: "Free", value: free.length, icon: Globe, tone: "green" },
      ]} />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-[1fr_200px_180px_auto_auto]">
        <Input value={search} onChange={setSearch} placeholder="Search by IP, port, node, server..." />
        <select className={selectStyle} value={nodeFilter} onChange={(e) => { setNodeFilter(e.target.value); setSelectedIds([]); }}>
          <option value="all">All nodes</option>
          {nodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
        </select>
        <select className={selectStyle} value={statusFilter} onChange={(e) => { setStatusFilter(e.target.value as "all" | "free" | "used"); setSelectedIds([]); }}>
          <option value="all">All allocations</option>
          <option value="free">Free only</option>
          <option value="used">In use only</option>
        </select>
        <Btn tone="danger" disabled={selectedFreeIds.length === 0 || bulkDeleteMut.isPending} onClick={() => confirmDelete(selectedFreeIds)}>
          <Trash2 size={13} /> {bulkDeleteMut.isPending ? "Deleting…" : `Delete (${selectedFreeIds.length})`}
        </Btn>
        <Btn tone="ghost" disabled={filtered.length === 0} onClick={exportCSV}>
          <Copy size={13} /> Export
        </Btn>
      </div>

      {deleteError ? (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200" role="alert">
          <AlertCircle size={16} className="mt-0.5 shrink-0" />
          <span>Could not delete allocations: {deleteError}</span>
        </div>
      ) : null}

      <Card className="overflow-hidden">
        <CardHeader
          title={`All allocations${search ? ` (${filtered.length} of ${allocations.length})` : ""}`}
          icon={Network}
        />
        {allocationsQuery.isLoading ? (
          <EmptyState icon={Network} message="Loading allocations…" />
        ) : allocationsQuery.isError ? (
          <div className="p-5">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load allocations: {allocationsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => allocationsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Network} message="No allocations found." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06]">
                  <th className={thClass} onClick={() => toggleSort("node")}>
                    Node <SortIcon column="node" />
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-slate-400">
                    <input
                      type="checkbox"
                      checked={allVisibleFreeSelected}
                      onChange={toggleSelectAllVisible}
                      className="h-4 w-4 accent-[#dc2626] cursor-pointer"
                      disabled={visibleFreeIds.length === 0}
                    />
                  </th>
                  <th className={thClass} onClick={() => toggleSort("ip")}>
                    IP : Port / Protocol <SortIcon column="ip" />
                  </th>
                  <th className={thClass} onClick={() => toggleSort("alias")}>
                    Alias <SortIcon column="alias" />
                  </th>
                  <th className={thClass} onClick={() => toggleSort("server")}>
                    Server <SortIcon column="server" />
                  </th>
                  <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-slate-400" />
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((alloc) => (
                  <tr key={alloc.id} className="transition-colors hover:bg-white/[0.02]">
                    <td className="px-4 py-3 text-xs text-slate-400">{alloc.node}</td>
                    <td className="px-4 py-3">
                      <input
                        type="checkbox"
                        disabled={Boolean(alloc.server)}
                        checked={selectedIds.includes(alloc.id)}
                        onChange={() => toggleSelected(alloc.id)}
                        className="h-4 w-4 accent-[#dc2626] cursor-pointer disabled:cursor-not-allowed disabled:opacity-30"
                      />
                    </td>
                    <td className="px-4 py-3 font-mono text-sm whitespace-nowrap">
                      <span className="text-slate-300">{alloc.ip}</span>
                      <span className="text-slate-600">:</span>
                      <span className="text-brand font-bold">{alloc.port}</span>
                      <span className="ml-2 inline-flex items-center gap-1 rounded bg-white/[0.04] px-1.5 py-0.5 text-[10px] font-bold uppercase text-slate-500">
                        {alloc.protocol ?? "tcp"}
                        {alloc.containerPort && alloc.containerPort !== alloc.port ? <span className="text-slate-600">→{alloc.containerPort}</span> : null}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500 max-w-[200px] truncate">{alloc.alias || <span className="text-slate-600">—</span>}</td>
                    <td className="px-4 py-3">
                      {alloc.server
                        ? <Pill tone="blue">{alloc.server}</Pill>
                        : <Pill tone="green">free</Pill>}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-1">
                        <Btn size="sm" tone="ghost" onClick={() => openEdit(alloc)}><Edit3 size={12} /></Btn>
                        {!alloc.server && (
                          <Btn size="sm" tone="danger" onClick={() => confirmSingleDelete(alloc.id)} disabled={bulkDeleteMut.isPending}>
                            <Trash2 size={12} />
                          </Btn>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {editing ? (
        <Modal title="Edit Allocation Alias" onClose={() => { setEditing(null); setEditError(null); }}>
          <div className="space-y-5">
            <div className="flex items-center gap-3 rounded-lg border border-white/[0.06] bg-surface-card-header p-4 font-mono text-sm">
              <Network size={16} className="shrink-0 text-slate-500" />
              <span className="text-slate-300">{editing.ip}</span>
              <span className="text-slate-600">:</span>
              <span className="text-brand font-bold">{editing.port}</span>
              <Pill tone="neutral">{editing.protocol ?? "tcp"}</Pill>
              {editing.server ? <Pill tone="blue">{editing.server}</Pill> : <Pill tone="green">free</Pill>}
            </div>
            <Input label="Alias" value={editAlias} onChange={(value) => { setEditAlias(value); setEditError(null); }} placeholder="minecraft.example.com" />
            {editError ? (
              <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200" role="alert">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>Could not update alias: {editError}</span>
              </div>
            ) : null}
          </div>
          <ModalFooter
            onCancel={() => { setEditing(null); setEditError(null); }}
            onConfirm={() => { setEditError(null); editMut.mutate({ id: editing.id, alias: editAlias.trim() }); }}
            disabled={editMut.isPending}
            confirmLabel={editMut.isPending ? "Saving…" : "Save Alias"}
          />
        </Modal>
      ) : null}

      {modal ? (
        <Modal title="Create Allocations" onClose={() => { setModal(false); setCreateError(null); }} className="max-w-2xl">
          {nodesQuery.isError ? (
            <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load allocation nodes: {nodesQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => nodesQuery.refetch()}>Retry</Btn>
            </div>
          ) : null}
          <div className="space-y-5">
            {/* Node Selection */}
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">Node</label>
              <select className={selectStyle} value={nodeId} onChange={(e) => setNodeId(e.target.value)}>
                {nodes.length === 0 ? <option value="">No nodes available</option> : null}
                {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
              </select>
              {nodesQuery.isLoading ? <p className="mt-1 text-xs text-slate-400">Loading nodes…</p> : nodes.length === 0 ? <p className="mt-1 text-xs text-amber-300">Create a node before adding allocations.</p> : null}
            </div>

            {/* IP & Ports */}
            <div className="grid gap-5 sm:grid-cols-2">
              <div>
                <Input label="IP address" value={ip} onChange={setIp} placeholder="0.0.0.0" mono />
                <p className="mt-1.5 text-xs text-slate-500">Use <code className="text-slate-400">0.0.0.0</code> to bind on every interface, or a specific IP to limit exposure.</p>
              </div>
              <div>
                <Input label="Ports" value={ports} onChange={setPorts} placeholder="25565" mono />
                <p className="mt-1.5 text-xs text-slate-500">Single port (<code className="text-slate-400">25565</code>), range (<code className="text-slate-400">25565-25580</code>), or comma list.</p>
              </div>
            </div>

            {/* Protocol, Container Port, Alias, Notes */}
            <div className="grid gap-5 sm:grid-cols-2">
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Protocol</label>
                <select className={selectStyle} value={protocol} onChange={(e) => setProtocol(e.target.value as "tcp" | "udp")}>
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                </select>
              </div>
              <Input label="Container port (optional)" value={containerPort} onChange={setContainerPort} placeholder="Same as host port" mono />
              <Input label="Alias (optional)" value={alias} onChange={setAlias} placeholder="minecraft.local" />
              <Input label="Notes (optional)" value={notes} onChange={setNotes} placeholder="Additional notes..." />
            </div>

            {createError ? (
              <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>{createError}</span>
              </div>
            ) : null}
          </div>
          <ModalFooter
            onCancel={() => { setModal(false); setCreateError(null); }}
            onConfirm={() => {
              const validationError = validateCreateInput(ip, ports);
              if (validationError) { setCreateError(validationError); return; }
              setCreateError(null);
              createMut.mutate();
            }}
            disabled={!nodeId || ip.trim() === "" || ports.trim() === "" || createMut.isPending || nodesQuery.isLoading || nodesQuery.isError}
            confirmLabel="Create"
          />
        </Modal>
      ) : null}
    </div>
  );
}
