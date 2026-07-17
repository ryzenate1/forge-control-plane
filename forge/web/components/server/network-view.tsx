"use client";

import { useState } from "react";
import { Network, Plus, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { type ApiAllocation, type ApiServer, assignServerAllocation, fetchServerAllocations, setPrimaryServerAllocation, unassignServerAllocation, updateAllocation } from "@/lib/api";

function isPrimaryAllocation(allocation: ApiAllocation, server?: ApiServer) {
  return allocation.isPrimary === true || allocation.primary === true || server?.primaryAllocationId === allocation.id || server?.allocationId === allocation.id;
}

function errorText(error: unknown) {
  return error instanceof Error ? error.message : "Allocation action failed.";
}

export function NetworkView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canCreate = hasServerPermission(access, "allocation.create");
  const canUpdate = hasServerPermission(access, "allocation.update");
  const canDelete = hasServerPermission(access, "allocation.delete");
  const queryClient = useQueryClient();
  const [allocationId, setAllocationId] = useState("");
  const allocationsQuery = useQuery({
    queryKey: ["server-allocations", server?.id],
    queryFn: () => fetchServerAllocations(server?.id ?? ""),
    enabled: Boolean(server?.id),
  });
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["server-allocations", server?.id] });
    void queryClient.invalidateQueries({ queryKey: ["server", server?.id] });
    void queryClient.invalidateQueries({ queryKey: ["servers"] });
  };
  const primaryMutation = useMutation({ mutationFn: (id: string) => setPrimaryServerAllocation(server?.id ?? "", id), onSuccess: refresh });
  const assignMutation = useMutation({
    mutationFn: (id: string) => assignServerAllocation(server?.id ?? "", id),
    onSuccess: () => { setAllocationId(""); refresh(); },
  });
  const unassignMutation = useMutation({ mutationFn: (id: string) => unassignServerAllocation(server?.id ?? "", id), onSuccess: refresh });
  const aliasMutation = useMutation({ mutationFn: ({ id, alias, notes }: { id: string; alias: string; notes: string }) => updateAllocation(id, { alias, notes }), onSuccess: refresh });
  const rows = allocationsQuery.data ?? [];
  const limit = server?.allocationLimit;
  const limitReached = typeof limit === "number" && limit > 0 && rows.length >= limit;
  const actionError = allocationsQuery.error ?? primaryMutation.error ?? assignMutation.error ?? unassignMutation.error ?? aliasMutation.error;

  return (
    <div className="space-y-4">
      <form className="rounded-xl border border-white/[0.08] bg-[#1e2536] p-4" onSubmit={(event) => { event.preventDefault(); if (allocationId.trim()) assignMutation.mutate(allocationId.trim()); }}>
        <label className="text-sm font-semibold text-slate-200" htmlFor="allocation-id">Assign allocation by ID</label>
        <div className="mt-2 flex flex-col gap-2 sm:flex-row">
          <input id="allocation-id" className="h-10 min-w-0 flex-1 rounded border border-[#4b5563] bg-[#141824] px-3 font-mono text-sm text-slate-100" onChange={(event) => setAllocationId(event.target.value)} placeholder="Allocation UUID" value={allocationId} />
          <button className="inline-flex items-center justify-center gap-2 rounded bg-[#dc2626] px-4 py-2 text-sm font-bold text-white disabled:opacity-50" disabled={!canCreate || !server?.id || !allocationId.trim() || assignMutation.isPending || limitReached} type="submit"><Plus size={16} />{assignMutation.isPending ? "Assigning…" : limitReached ? "Limit reached" : "Assign"}</button>
        </div>
        <p className="mt-2 text-xs text-slate-400">The API assigns existing allocations by ID and does not expose a server-scoped catalog of free allocations. {typeof limit === "number" && limit > 0 ? `${rows.length} of ${limit} slots are used.` : "No allocation quota was provided."}</p>
      </form>
      {actionError ? <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200" role="alert">{errorText(actionError)}</div> : null}
      {allocationsQuery.isLoading ? <div className="rounded-xl bg-[#1e2536] px-6 py-5 text-sm font-semibold text-[#94a3b8]">Loading allocations…</div> : null}
      {!allocationsQuery.isLoading && !allocationsQuery.isError && rows.length === 0 ? <div className="rounded-xl bg-[#1e2536] px-6 py-5 text-sm font-semibold text-[#94a3b8]">No allocations are assigned to this server.</div> : null}
      {rows.map((allocation) => {
        const primary = isPrimaryAllocation(allocation, server);
        const pending = primaryMutation.isPending || unassignMutation.isPending;
        return (
          <div className="grid gap-4 rounded-xl bg-[#1e2536] px-4 py-5 text-[#94a3b8] md:grid-cols-[42px_140px_90px_1fr_auto] md:items-center md:px-6" key={allocation.id}>
            <Network size={22} />
            <div><p className="rounded bg-[#141824] px-2 py-1 font-mono text-base text-slate-100">{allocation.ip}</p><p className="mt-1 text-xs uppercase">IP Address</p></div>
            <div><p className="rounded bg-[#141824] px-2 py-1 font-mono text-base text-slate-100">{allocation.port}</p><p className="mt-1 text-xs uppercase">Port</p></div>
            <div><p className="rounded-xl bg-[#252d3f] px-4 py-3 font-semibold text-slate-200">{allocation.alias || allocation.notes || "No alias or notes"}</p><p className="mt-1 break-all font-mono text-xs">{allocation.id}</p></div>
            <div className="flex flex-wrap gap-2 md:justify-end">
              <button className="rounded-xl border border-[#4b5563] px-3 py-2 text-xs font-bold uppercase hover:bg-[#374151] disabled:opacity-60" disabled={!canUpdate || pending || aliasMutation.isPending} onClick={() => { const alias = window.prompt("Allocation alias", allocation.alias ?? ""); if (alias === null) return; const notes = window.prompt("Allocation notes", allocation.notes ?? ""); if (notes === null) return; aliasMutation.mutate({ id: allocation.id, alias: alias.trim(), notes: notes.trim() }); }} type="button">Edit alias</button>
              {primary ? <span className="w-fit rounded bg-[#059669] px-3 py-2 text-sm font-bold text-white">Primary</span> : <button className="rounded-xl border border-[#4b5563] px-4 py-2 text-xs font-bold uppercase hover:bg-[#374151] disabled:opacity-60" disabled={!canUpdate || pending} onClick={() => primaryMutation.mutate(allocation.id)} type="button">Make Primary</button>}
              <button aria-label={`Unassign ${allocation.ip}:${allocation.port}`} className="inline-flex items-center gap-1 rounded-xl border border-red-500/60 px-3 py-2 text-xs font-bold uppercase text-red-200 hover:bg-red-500/10 disabled:opacity-40" disabled={!canDelete || primary || pending} onClick={() => { if (window.confirm(`Unassign ${allocation.ip}:${allocation.port}?`)) unassignMutation.mutate(allocation.id); }} title={primary ? "Choose another primary allocation before unassigning this one" : "Unassign allocation"} type="button"><Trash2 size={14} /> Unassign</button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
