"use client";

import { useState, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Edit3, Globe, Network, Plus, Server, Trash2 } from "lucide-react";
import type { ApiAllocation, ApiAllocationNode } from "@/lib/api";
import { createAllocation, deleteAllocations, fetchAllocationNodes, fetchAllocations, setAdminAllocationAlias } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, StatsRow } from "./admin-ui";

const EMPTY_NODES: ApiAllocationNode[] = [];
const EMPTY_ALLOCATIONS: ApiAllocation[] = [];

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
 const [editing, setEditing] = useState<ApiAllocation | null>(null);
 const [editAlias, setEditAlias] = useState("");
 const [editError, setEditError] = useState<string | null>(null);
 const [deleteError, setDeleteError] = useState<string | null>(null);
 const [nodeId, setNodeId] = useState("");
 const [ip, setIp] = useState("0.0.0.0");
 const [ports, setPorts] = useState("25565");
 const [alias, setAlias] = useState("");
 const [notes, setNotes] = useState("");
 const [createError, setCreateError] = useState<string | null>(null);

 const nodesQuery = useQuery({ queryKey: ["allocation-nodes"], queryFn: fetchAllocationNodes });
 const allocationsQuery = useQuery({ queryKey: ["allocations"], queryFn: fetchAllocations });
 const nodes = nodesQuery.data ?? EMPTY_NODES;
 const allocations = allocationsQuery.data ?? EMPTY_ALLOCATIONS;

  const createMut = useMutation({
    mutationFn: () => {
      return createAllocation({ nodeId: nodeId || nodes[0]?.id || "", ip, ports, alias, notes });
    },
    onSuccess: (created) => { qc.invalidateQueries({ queryKey: ["allocations"] }); setModal(false); setCreateError(null); toast({ tone: "success", title: `${created.length} allocation${created.length === 1 ? "" : "s"} created` }); },
    onError: (e: Error) => { console.error("Failed to create allocations:", e); setCreateError(e.message || "Unknown error"); toast({ tone: "error", title: "Failed to create allocations", message: e.message || "Unknown error" }); },
  });



 const editMut = useMutation({
 mutationFn: ({ id, alias }: { id: string; alias: string }) => setAdminAllocationAlias(id, alias),
 onSuccess: () => {
 void qc.invalidateQueries({ queryKey: ["allocations"] });
 setEditing(null);
 setEditError(null);
 toast({ tone: "success", title: "Allocation alias updated" });
 },
 onError: (error: Error) => {
 const message = error.message || "Unknown error";
 setEditError(message);
 toast({ tone: "error", title: "Failed to update allocation alias", message });
 },
 });
 const bulkDeleteMut = useMutation({
 mutationFn: (ids: string[]) => deleteAllocations(ids),
 onSuccess: (_, ids) => {
 void qc.invalidateQueries({ queryKey: ["allocations"] });
 setSelectedIds((current) => current.filter((id) => !ids.includes(id)));
 setDeleteError(null);
 toast({ tone: "success", title: `${ids.length} allocation${ids.length === 1 ? "" : "s"} deleted` });
 },
 onError: (error: Error) => {
 const message = error.message || "Unknown error";
 setDeleteError(message);
 toast({ tone: "error", title: "Failed to delete allocations", message });
 },
 });

 const free = allocations.filter((a) => !a.server);
 const used = allocations.filter((a) => a.server);

 const filtered = useMemo(() => {
 const q = search.trim().toLowerCase();
 return allocations.filter((a) => {
 const matchesSearch = !q || a.ip.includes(q) || String(a.port).includes(q) || (a.node ?? "").toLowerCase().includes(q) || (a.server ?? "").toLowerCase().includes(q) || (a.alias ?? "").toLowerCase().includes(q);
 const matchesStatus = statusFilter === "all" || (statusFilter === "free" ? !a.server : Boolean(a.server));
 const matchesNode = nodeFilter === "all" || a.node === nodes.find((node) => node.id === nodeFilter)?.name;
 return matchesSearch && matchesStatus && matchesNode;
 });
 }, [allocations, search, statusFilter, nodeFilter, nodes]);
 const selectedFreeIds = selectedIds.filter((id) => !allocations.find((allocation) => allocation.id === id)?.server);
 const toggleSelected = (id: string) => {
 setDeleteError(null);
 setSelectedIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
 };
 const openEdit = (allocation: ApiAllocation) => {
 setEditing(allocation);
 setEditAlias(allocation.alias ?? "");
 setEditError(null);
 };
 const confirmDelete = (ids: string[]) => {
 if (ids.length === 0 || !window.confirm(`Delete ${ids.length} free allocation${ids.length === 1 ? "" : "s"}? This cannot be undone.`)) return;
 setDeleteError(null);
 bulkDeleteMut.mutate(ids);
 };

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

 <div className="mb-4 grid gap-3 lg:grid-cols-[1fr_180px_180px_auto]">
 <Input value={search} onChange={setSearch} placeholder="Search by IP, port, node, server" />
 <select className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100" value={nodeFilter} onChange={(event) => setNodeFilter(event.target.value)}>
 <option value="all">All nodes</option>
 {nodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
 </select>
 <select className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as "all" | "free" | "used")}>
 <option value="all">All allocations</option>
 <option value="free">Free only</option>
 <option value="used">In use only</option>
 </select>
 <Btn tone="danger" disabled={selectedFreeIds.length === 0 || bulkDeleteMut.isPending} onClick={() => confirmDelete(selectedFreeIds)}>
 <Trash2 size={13} /> {bulkDeleteMut.isPending ? "Deleting…" : `Delete Selected (${selectedFreeIds.length})`}
 </Btn>
 </div>

 {deleteError ? <div className="mb-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200" role="alert"><AlertCircle size={16} className="mt-0.5 shrink-0" /><span>Could not delete allocations: {deleteError}</span></div> : null}

 <Card>
 <CardHeader title="All allocations" icon={Network} />
 {allocationsQuery.isLoading ? (
 <EmptyState icon={Network} message="Loading allocations…" />
 ) : allocationsQuery.isError ? (
 <div className="p-4">
 <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
 <span>Could not load allocations: {allocationsQuery.error.message}</span>
 <Btn size="sm" tone="ghost" onClick={() => allocationsQuery.refetch()}>Retry</Btn>
 </div>
 </div>
 ) : filtered.length === 0 ? (
 <EmptyState icon={Network} message="No allocations found." />
 ) : (
 <table className="w-full text-sm">
 <thead>
 <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
 <th className="px-4 py-3">Node</th>
 <th className="px-4 py-3">Select</th>
 <th className="px-4 py-3">IP : Port</th>
 <th className="px-4 py-3">Alias</th>
 <th className="px-4 py-3">Server</th>
 <th className="px-4 py-3" />
 </tr>
 </thead>
 <tbody className="divide-y divide-white/[0.04]">
 {filtered.map((alloc) => (
 <tr key={alloc.id} className="hover:bg-white/[0.02]">
 <td className="px-4 py-3 text-slate-400 text-xs">{alloc.node}</td>
 <td className="px-4 py-3">
 <input type="checkbox" disabled={Boolean(alloc.server)} checked={selectedIds.includes(alloc.id)} onChange={() => toggleSelected(alloc.id)} />
 </td>
 <td className="px-4 py-3 font-mono text-sm">
 <span className="text-slate-300">{alloc.ip}</span>
 <span className="text-slate-500">:</span>
 <span className="text-[#dc2626] font-bold">{alloc.port}</span>
 </td>
 <td className="px-4 py-3 text-xs text-slate-500">{alloc.alias || "-"}</td>
 <td className="px-4 py-3">
 {alloc.server
 ? <Pill tone="blue">{alloc.server}</Pill>
 : <Pill tone="green">free</Pill>}
 </td>
 <td className="px-4 py-3">
 <div className="flex gap-2">
 <Btn size="sm" tone="ghost" onClick={() => openEdit(alloc)}><Edit3 size={12} /> Edit alias</Btn>
 {!alloc.server && (
 <Btn size="sm" tone="danger" onClick={() => confirmDelete([alloc.id])} disabled={bulkDeleteMut.isPending}><Trash2 size={12} /> {bulkDeleteMut.isPending ? "Deleting…" : "Delete"}</Btn>
 )}
 </div>
 </td>
 </tr>
 ))}
 </tbody>
 </table>
 )}
 </Card>

 {editing ? (
 <Modal title="Edit Allocation Alias" onClose={() => { setEditing(null); setEditError(null); }}>
 <div className="grid gap-4">
 <div className="rounded-lg border border-white/[0.06] bg-[#161b28] p-3 font-mono text-sm text-slate-200">{editing.ip}:{editing.port}</div>
 <Input label="Alias" value={editAlias} onChange={(value) => { setEditAlias(value); setEditError(null); }} placeholder="minecraft.example.com" />
 {editError ? <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200" role="alert"><AlertCircle size={14} className="mt-0.5 shrink-0" /><span>Could not update alias: {editError}</span></div> : null}
 </div>
 <ModalFooter onCancel={() => { setEditing(null); setEditError(null); }} onConfirm={() => { setEditError(null); editMut.mutate({ id: editing.id, alias: editAlias.trim() }); }} disabled={editMut.isPending} confirmLabel={editMut.isPending ? "Saving…" : "Save Alias"} />
 </Modal>
 ) : null}

 {modal ? (
 <Modal title="Create Allocations" onClose={() => { setModal(false); setCreateError(null); }} wide>
 {nodesQuery.isError ? (
 <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
 <span>Could not load allocation nodes: {nodesQuery.error.message}</span>
 <Btn size="sm" tone="ghost" onClick={() => nodesQuery.refetch()}>Retry</Btn>
 </div>
 ) : null}
 <div className="grid gap-4 md:grid-cols-2">
 <div className="md:col-span-2">
 <label className="block text-sm font-medium text-slate-300 mb-1.5">Node</label>
 <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100 text-sm" value={nodeId} onChange={(e) => setNodeId(e.target.value)}>
 {nodes.length === 0 ? <option value="">No nodes available</option> : null}
 {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
 </select>
 {nodesQuery.isLoading ? <p className="mt-1 text-xs text-slate-400">Loading nodes…</p> : nodes.length === 0 ? <p className="mt-1 text-xs text-amber-300">Create a node before adding allocations.</p> : null}
 </div>
 <div>
 <Input label="IP address" value={ip} onChange={setIp} placeholder="0.0.0.0" mono />
 <p className="mt-1 text-xs text-slate-500"><code>0.0.0.0</code> binds the port on every node interface. Use a specific interface IP to limit exposure.</p>
 </div>
 <div>
 <Input label="Ports (single, range 25565-25580, or comma list)" value={ports} onChange={setPorts} placeholder="25565" mono />
 <p className="mt-1 text-xs text-slate-500">e.g. 25565 or 25565-25580 or 25565,25566,25570</p>
 </div>
 <Input label="Alias (optional)" value={alias} onChange={setAlias} placeholder="minecraft.local" />
 <Input label="Notes (optional)" value={notes} onChange={setNotes} placeholder="" />
 {createError ? <div className="md:col-span-2 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200"><AlertCircle size={14} className="mt-0.5 shrink-0" /> <span>{createError}</span></div> : null}
 </div>
 <ModalFooter
 onCancel={() => { setModal(false); setCreateError(null); }}
 onConfirm={() => {
 const validationError = validateCreateInput(ip, ports);
 if (validationError) {
 setCreateError(validationError);
 return;
 }
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
