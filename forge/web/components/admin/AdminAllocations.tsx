"use client";

import { useState, useMemo, useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, ArrowDownUp, Cable, Copy, Edit3, Network, Plus, Server, Trash2 } from "lucide-react";
import type { ApiAllocation, ApiAllocationNode } from "@/lib/api";
import { createAllocation, deleteAllocations, fetchAllocationNodes, fetchAllocations, setAdminAllocationAlias } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminErrorState, AdminLoadingState, AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, StatsRow } from "./admin-ui";

const EMPTY_NODES: ApiAllocationNode[] = [];
const EMPTY_ALLOCATIONS: ApiAllocation[] = [];

type SortKey = "node" | "ip" | "port" | "alias" | "server";

function isIPv6Address(address: string): boolean {
  if (!address.includes(":")) return false;
  if (!/^[0-9a-fA-F:.]+$/.test(address)) return false;
  // The previous branch was `includes(":") && /[0-9a-fA-F:.]+/`, which accepted
  // `::`, `1:2:3` and `ffff:`. An allocation bound to a malformed address is
  // invisible until traffic fails, so the shape is checked properly: at most one
  // `::` elision, hextets of 1-4 hex digits, and eight groups when not elided.
  if (address.split("::").length - 1 > 1) return false;
  if (address.includes("::")) {
    const groups = address.split(":").filter(Boolean);
    return groups.length >= 1 && groups.length <= 7 && groups.every((g) => /^[0-9a-fA-F]{1,4}$/.test(g));
  }
  const groups = address.split(":");
  return groups.length === 8 && groups.every((g) => /^[0-9a-fA-F]{1,4}$/.test(g));
}

function validateCreateInput(ip: string, ports: string): string | null {
  const address = ip.trim();
  const isIPv4 = address.split(".").length === 4 && address.split(".").every((part) => /^\d+$/.test(part) && Number(part) <= 255);
  if (!isIPv4 && !isIPv6Address(address)) return "Enter a valid IPv4 address (four octets) or a full IPv6 address.";
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
  const [confirm, renderConfirm] = useConfirm();

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
  // No default address: `0.0.0.0` was the initial value of a control whose help
  // text says a specific IP limits exposure, so the widest exposure was the
  // value an operator got without choosing it.
  const [ip, setIp] = useState("");
  const [ports, setPorts] = useState("25565");
  const [containerPort, setContainerPort] = useState("");
  const [protocol, setProtocol] = useState<"tcp" | "udp">("tcp");
  const [alias, setAlias] = useState("");
  const [notes, setNotes] = useState("");

  const nodesQuery = useQuery({ queryKey: ["allocation-nodes"], queryFn: fetchAllocationNodes });
  const allocationsQuery = useQuery({ queryKey: ["allocations"], queryFn: fetchAllocations });
  const nodes = nodesQuery.data ?? EMPTY_NODES;
  const allocations = allocationsQuery.data ?? EMPTY_ALLOCATIONS;

  // AGENTS.md: a request that omits the node must be rejected, not answered with
  // the first node that happens to have credentials. Both the modal opening and
  // the mutation used to fall back to `nodes[0]`.
  const createMut = useMutation({
    mutationFn: () => {
      if (!nodeId) throw new Error("Select the node to bind this allocation on — no node was chosen.");
      return createAllocation({ nodeId, ip: ip.trim(), ports, containerPort: containerPort ? Number(containerPort) : undefined, protocol, alias, notes });
    },
    onSuccess: (created) => { qc.invalidateQueries({ queryKey: ["allocations"] }); setModal(false); setCreateError(null); toast({ tone: "success", title: `${created.length} allocation${created.length === 1 ? "" : "s"} created` }); },
    onError: (e: Error) => { setCreateError(e.message || "Unknown error"); toast({ tone: "error", title: "Failed to create allocations", message: e.message || "Unknown error" }); },
  });

  const editMut = useMutation({
    mutationFn: async ({ id, alias }: { id: string; alias: string }) => {
      const result = await setAdminAllocationAlias(id, alias);
      if (!result.ok) throw new Error("The server reported the allocation alias was not updated.");
      return result;
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["allocations"] }); setEditing(null); setEditError(null); toast({ tone: "success", title: "Allocation alias updated" }); },
    onError: (error: Error) => { const message = error.message || "Unknown error"; setEditError(message); toast({ tone: "error", title: "Failed to update allocation alias", message }); },
  });

  const bulkDeleteMut = useMutation({
    mutationFn: async (ids: string[]) => {
      const result = await deleteAllocations(ids);
      if (!result.ok) throw new Error("The server reported the allocations were not deleted.");
      return result;
    },
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
  const confirmDelete = async (ids: string[]) => {
    if (ids.length === 0) return;
    const confirmed = await confirm({ title: `Delete ${ids.length} free allocation${ids.length === 1 ? "" : "s"}?`, description: "Only free (unassigned) allocations will be deleted. This cannot be undone.", danger: true, confirmLabel: "Delete" });
    if (!confirmed) return;
    setDeleteError(null);
    bulkDeleteMut.mutate(ids);
  };
  const confirmSingleDelete = (id: string) => confirmDelete([id]);

  const exportCSV = useCallback(() => {
    // Raw interpolation corrupted the file on any name containing a quote.
    const esc = (value: string | number | undefined | null) => `"${String(value ?? "").replace(/"/g, '""')}"`;
    const rows = filtered.map((a) => [esc(a.node), esc(a.ip), a.port, esc(a.protocol ?? "tcp"), a.containerPort ?? "", esc(a.alias ?? ""), esc(a.server ?? "")].join(","));
    const text = `Node,IP,Port,Protocol,Container Port,Alias,Server\n${rows.join("\n")}`;
    navigator.clipboard.writeText(text).then(() => toast({ tone: "success", title: `Copied ${filtered.length} allocation${filtered.length === 1 ? "" : "s"} to the clipboard`, message: "The rows currently matching your search and filters, as CSV text." }), () => toast({ tone: "error", title: "Failed to copy" }));
  }, [filtered, toast]);

  const SortIcon = ({ column }: { column: SortKey }) => {
    if (sortKey !== column) return <ArrowDownUp size={10} className="ml-1 opacity-0 group-hover:opacity-40" />;
    return <ArrowDownUp size={10} className={`ml-1 ${sortDir === "asc" ? "rotate-180" : ""} text-brand`} />;
  };

  const selectStyle = "h-10 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3.5 text-sm text-text outline-none transition hover:border-line-strong focus:border-brand focus:ring-2 focus:ring-brand/20";

  // `sticky top-0` was inert (no vertical scroll container), so it is gone; the
  // header is now a button so sorting is keyboard-operable and disclosed.
  const thClass = "bg-overlay-subtle px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-text-subtle select-none group";

  // Counting a list that has not arrived yet is not a measurement: while the
  // query is in flight or has failed the tiles read "—" / "Unavailable" instead
  // of a zero the page never measured.
  const statValue = (n: number) => (allocationsQuery.isPending ? "—" : allocationsQuery.isError ? "Unavailable" : n);

  return (
    <AdminPageLayout>
      <SectionHeader
        action={<Btn disabled={nodesQuery.isPending || nodesQuery.isError || nodes.length === 0} title={nodesQuery.isError ? "Allocation nodes could not be loaded" : nodes.length === 0 ? "No nodes are available to bind an allocation to" : undefined} onClick={() => { setNodeId(""); setCreateError(null); setModal(true); }}><Plus size={14} /> Create Allocations</Btn>}
      />

      {nodesQuery.isError ? (
        <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger" role="alert">
          <span>Could not load allocation nodes: {nodesQuery.error instanceof Error ? nodesQuery.error.message : "unknown error"}</span>
          <Btn size="sm" tone="ghost" onClick={() => void nodesQuery.refetch()}>Retry</Btn>
        </div>
      ) : null}

      <StatsRow items={[
        { label: "Total", value: statValue(allocations.length), icon: Network, tone: "neutral" },
        { label: "In use", value: statValue(used.length), icon: Server, tone: "blue" },
        { label: "Free", value: statValue(free.length), icon: Cable, tone: "green" },
      ]} />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-[1fr_200px_180px_auto_auto]">
        <Input aria-label="Search allocations by IP, port, node, server or alias" value={search} onChange={setSearch} placeholder="Search by IP, port, node, server..." />
        <select aria-label="Filter by node" className={selectStyle} value={nodeFilter} onChange={(e) => { setNodeFilter(e.target.value); setSelectedIds([]); }}>
          <option value="all">All nodes</option>
          {nodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
        </select>
        <select aria-label="Filter by assignment state" className={selectStyle} value={statusFilter} onChange={(e) => { setStatusFilter(e.target.value as "all" | "free" | "used"); setSelectedIds([]); }}>
          <option value="all">All allocations</option>
          <option value="free">Free only</option>
          <option value="used">In use only</option>
        </select>
        <Btn tone="danger" disabled={selectedFreeIds.length === 0 || bulkDeleteMut.isPending} onClick={() => confirmDelete(selectedFreeIds)}>
          <Trash2 size={13} /> {bulkDeleteMut.isPending ? "Deleting…" : `Delete (${selectedFreeIds.length})`}
        </Btn>
        <Btn tone="ghost" disabled={filtered.length === 0} onClick={exportCSV}>
          <Copy size={13} /> Copy as CSV
        </Btn>
      </div>

      {deleteError ? (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger" role="alert">
          <AlertCircle size={16} className="mt-0.5 shrink-0" />
          <span>Could not delete allocations: {deleteError}</span>
        </div>
      ) : null}

      <Card className="overflow-hidden">
        <CardHeader
          title={`All allocations${search ? ` (${filtered.length} of ${allocations.length})` : ""}`}
          icon={Network}
        />
        {allocationsQuery.isPending ? (
          <div className="p-4"><AdminLoadingState label="Loading allocations…" /></div>
        ) : allocationsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Allocations could not be loaded: ${allocationsQuery.error instanceof Error ? allocationsQuery.error.message : "request error"}. This is a failed read, not an empty fleet.`}
              retry={() => void allocationsQuery.refetch()}
            />
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={Network}
            message={search || statusFilter !== "all" || nodeFilter !== "all"
              ? "No allocation matches the search and filters in effect. Clear them to see the rest."
              : "No allocation has been recorded yet. Create one to bind an IP:port to a node."}
            title={search || statusFilter !== "all" || nodeFilter !== "all" ? "No allocations match the filters" : "No allocations"}
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-line">
                  <th className={thClass}>
                    <button className="flex items-center font-semibold uppercase tracking-wider text-text-subtle hover:text-text" onClick={() => toggleSort("node")} type="button">Node <SortIcon column="node" /></button>
                  </th>
                  <th className="bg-overlay-subtle px-4 py-3">
                    <input
                      type="checkbox"
                      checked={allVisibleFreeSelected}
                      onChange={toggleSelectAllVisible}
                      className="h-4 w-4 cursor-pointer rounded border-line bg-[var(--surface-input)] text-brand focus:ring-brand/30"
                      aria-label="Select all visible free allocations"
                      disabled={visibleFreeIds.length === 0}
                      title={visibleFreeIds.length === 0 ? "No free allocation is visible to select" : undefined}
                    />
                  </th>
                  {([
                    ["ip", "IP / Port / Protocol"],
                    ["alias", "Alias"],
                    ["server", "Server"],
                  ] as [SortKey, string][]).map(([key, label]) => (
                    <th key={key} aria-sort={sortKey === key ? (sortDir === "asc" ? "ascending" : "descending") : "none"} className={thClass}>
                      {/* Sorting was mouse-only (`cursor-pointer` on a `th` with no
                          handler) and the active direction appeared only on hover. */}
                      <button className="flex items-center font-semibold uppercase tracking-wider text-text-subtle hover:text-text" onClick={() => toggleSort(key)} type="button">{label} <SortIcon column={key} /></button>
                    </th>
                  ))}
                  <th className="bg-overlay-subtle px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((alloc) => (
                  <tr key={alloc.id} className="transition-colors hover:bg-overlay-subtle">
                    <td className="px-4 py-3 text-xs text-text-subtle">{alloc.node}</td>
                    <td className="px-4 py-3">
                      <input
                        type="checkbox"
                        disabled={Boolean(alloc.server)}
                        checked={selectedIds.includes(alloc.id)}
                        onChange={() => toggleSelected(alloc.id)}
                        className="h-4 w-4 cursor-pointer rounded border-line bg-[var(--surface-input)] text-brand focus:ring-brand/30 disabled:cursor-not-allowed disabled:opacity-30"
                        aria-label={`Select ${alloc.ip}:${alloc.port}`}
                        title={alloc.server ? "Assigned to a server, so it cannot be deleted in bulk" : undefined}
                      />
                    </td>
                    <td className="px-4 py-3 font-mono text-sm whitespace-nowrap">
                      <span className="text-text">{alloc.ip}</span>
                      <span className="text-text-muted">:</span>
                      <span className="text-brand font-bold">{alloc.port}</span>
                      <span className="ml-2 inline-flex items-center gap-1 rounded bg-overlay-subtle px-1.5 py-0.5 text-[10px] font-bold uppercase text-text-subtle">
                        {alloc.protocol ?? "tcp"}
                        {alloc.containerPort && alloc.containerPort !== alloc.port ? <span className="text-text-muted">→{alloc.containerPort}</span> : null}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-text-subtle max-w-[200px] truncate" title={alloc.alias || undefined}>{alloc.alias || <span className="text-text-muted">Not named</span>}</td>
                    <td className="px-4 py-3">
                      {alloc.server
                        ? <Pill tone="blue">{alloc.server}</Pill>
                        : <Pill tone="neutral">Free</Pill>}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-1">
                        <Btn ariaLabel={`Edit the alias of ${alloc.ip}:${alloc.port}`} size="sm" tone="ghost" onClick={() => openEdit(alloc)}><Edit3 size={12} /></Btn>
                        {!alloc.server && (
                          <Btn ariaLabel={`Delete the allocation ${alloc.ip}:${alloc.port}`} size="sm" tone="danger" onClick={() => confirmSingleDelete(alloc.id)} disabled={bulkDeleteMut.isPending}>
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
          <div className="space-y-4">
            <div className="flex items-center gap-3 rounded-lg border border-line bg-overlay-subtle p-4 font-mono text-sm">
              <Network size={16} className="shrink-0 text-text-subtle" />
              <span className="text-text">{editing.ip}</span>
              <span className="text-text-muted">:</span>
              <span className="text-brand font-bold">{editing.port}</span>
              <Pill tone="neutral">{editing.protocol ?? "tcp"}</Pill>
              {editing.server ? <Pill tone="blue">{editing.server}</Pill> : <Pill tone="neutral">Free</Pill>}
            </div>
            <Input label="Alias" value={editAlias} onChange={(value) => { setEditAlias(value); setEditError(null); }} placeholder="A short name for this binding, e.g. minecraft-lobby" />
            {editError ? (
              <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs text-danger" role="alert">
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
          <div className="space-y-4">
          {nodesQuery.isError ? (
            <div className="flex items-start justify-between gap-4 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger" role="alert">
              <span>Could not load allocation nodes: {nodesQuery.error instanceof Error ? nodesQuery.error.message : "unknown error"}</span>
              <Btn size="sm" tone="ghost" onClick={() => void nodesQuery.refetch()}>Retry</Btn>
            </div>
          ) : null}
            {/* Node Selection — no pre-selected node. A public IP:port bound on
                the wrong host is exactly the request that must not be guessed. */}
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle" htmlFor="allocation-node">Node</label>
              <select aria-label="Node to bind the allocation on" className={selectStyle} id="allocation-node" value={nodeId} onChange={(e) => { setNodeId(e.target.value); setCreateError(null); }}>
                <option value="">{nodesQuery.isPending ? "Loading nodes…" : nodes.length === 0 ? "No nodes available" : "Select a node"}</option>
                {nodes.map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
              </select>
              {!nodeId ? <p className="mt-1 text-xs text-text-subtle">{nodesQuery.isPending ? "Nodes are still loading." : nodes.length === 0 ? "Create a node before adding allocations." : "Choose which node owns this allocation — nothing is picked for you."}</p> : null}
            </div>

            {/* IP & Ports */}
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <Input label="IP address" value={ip} onChange={setIp} placeholder="e.g. 10.0.4.21 — or 0.0.0.0 for every interface" mono />
                <p className="mt-1.5 text-xs text-text-subtle"><code className="text-text-subtle">0.0.0.0</code> binds on every interface. Name a specific address unless the port is meant to be reachable on all of them.</p>
              </div>
              <div>
                <Input label="Ports" value={ports} onChange={setPorts} placeholder="25565" mono />
                <p className="mt-1.5 text-xs text-text-subtle">Single port (<code className="text-text-subtle">25565</code>), range (<code className="text-text-subtle">25565-25580</code>), or comma list.</p>
              </div>
            </div>

            {/* Protocol, Container Port, Alias, Notes */}
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle" htmlFor="allocation-protocol">Protocol</label>
                <select aria-label="Allocation protocol" className={selectStyle} id="allocation-protocol" value={protocol} onChange={(e) => setProtocol(e.target.value as "tcp" | "udp")}>
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                </select>
              </div>
              <Input label="Container port (optional)" value={containerPort} onChange={setContainerPort} placeholder="Same as host port" mono />
              <Input label="Alias (optional)" value={alias} onChange={setAlias} placeholder="minecraft.local" />
              <Input label="Notes (optional)" value={notes} onChange={setNotes} placeholder="Additional notes..." />
            </div>

            {createError ? (
              <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs text-danger" role="alert">
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
              if (!nodeId) { setCreateError("Select the node to bind this allocation on."); return; }
              setCreateError(null);
              createMut.mutate();
            }}
            disabled={!nodeId || ip.trim() === "" || ports.trim() === "" || !!validateCreateInput(ip, ports) || createMut.isPending || nodesQuery.isPending || nodesQuery.isError}
            confirmLabel={createMut.isPending ? "Creating…" : "Create"}
          />
        </Modal>
      ) : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
