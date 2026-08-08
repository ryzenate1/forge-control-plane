"use client";

import { useState } from "react";
import { Network, Plus, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { type ApiAllocation, type ApiServer, assignServerAllocation, fetchServerAllocations, setPrimaryServerAllocation, unassignServerAllocation, updateServerAllocationAlias } from "@/lib/api";
import { Alert, Card, ConfirmDialog, EmptyState } from "@/components/ui/primitives";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import { useToast } from "@/components/ui/toast";

function isPrimaryAllocation(allocation: ApiAllocation, server?: ApiServer) {
  return allocation.isPrimary === true || allocation.primary === true || server?.primaryAllocationId === allocation.id || server?.allocationId === allocation.id;
}

function errorText(error: unknown) {
  return error instanceof Error ? error.message : "The allocation action failed. Check your permissions and try again.";
}

export function NetworkView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canCreate = hasServerPermission(access, "allocation.create");
  const canUpdate = hasServerPermission(access, "allocation.update");
  const canDelete = hasServerPermission(access, "allocation.delete");
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [allocationId, setAllocationId] = useState("");
  const [unassignTarget, setUnassignTarget] = useState<ApiAllocation | null>(null);
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
    onError: (error) => toast({ tone: "error", title: "Assignment failed", message: errorText(error) }),
  });
  const unassignMutation = useMutation({
    mutationFn: (id: string) => unassignServerAllocation(server?.id ?? "", id),
    onSuccess: () => { setUnassignTarget(null); refresh(); },
    onError: (error) => toast({ tone: "error", title: "Unassign failed", message: errorText(error) }),
  });
  const aliasMutation = useMutation({ mutationFn: ({ id, alias, notes }: { id: string; alias: string; notes: string }) => updateServerAllocationAlias(server?.id ?? "", id, { alias, notes }), onSuccess: refresh });
  const rows = allocationsQuery.data ?? [];
  const limit = server?.allocationLimit;
  const limitReached = typeof limit === "number" && limit > 0 && rows.length >= limit;
  const actionError = allocationsQuery.error ?? primaryMutation.error ?? assignMutation.error ?? unassignMutation.error ?? aliasMutation.error;

  return (
    <div className="space-y-4">
      <Card title="Assign allocation" description="Attach an existing allocation to this server by ID." icon={<Network size={18} />}>
        <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); if (allocationId.trim()) assignMutation.mutate(allocationId.trim()); }}>
          <label className="ui-label" htmlFor="allocation-id">Allocation ID</label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <input id="allocation-id" className="ui-input min-w-0 flex-1 font-mono" onChange={(event) => setAllocationId(event.target.value)} placeholder="Allocation UUID" value={allocationId} />
            <button className="ui-button ui-button-primary" disabled={!canCreate || !server?.id || !allocationId.trim() || assignMutation.isPending || limitReached} type="submit"><Plus size={16} />{assignMutation.isPending ? "Assigning…" : limitReached ? "Limit reached" : "Assign allocation"}</button>
          </div>
          <p className="ui-hint">The API assigns existing allocations by ID and does not expose a server-scoped catalog of free allocations. {typeof limit === "number" && limit > 0 ? `${rows.length} of ${limit} slots are used.` : "No allocation quota was provided."}</p>
        </form>
      </Card>
      {actionError ? <Alert className="mt-4" tone="error" title="Allocation action failed">{errorText(actionError)} Check the allocation ID and your server permissions, then try again.</Alert> : null}
      {allocationsQuery.isLoading ? <CardSkeleton /> : null}
      {!allocationsQuery.isLoading && !allocationsQuery.isError && rows.length === 0 ? <EmptyState icon={<Network size={20} />} title="No allocations assigned" description="Assign an allocation by ID above to give this server a public address and port." /> : null}
      {rows.map((allocation) => {
        const primary = isPrimaryAllocation(allocation, server);
        const pending = primaryMutation.isPending || unassignMutation.isPending;
        return (
          <div className="ui-card grid gap-4 md:grid-cols-[42px_140px_90px_1fr_auto] md:items-center" key={allocation.id}>
            <Network className="text-slate-500" size={22} />
            <div><p className="rounded-lg bg-white/[0.04] px-2 py-1 font-mono text-base text-slate-100">{allocation.ip}</p><p className="mt-1 text-xs uppercase text-slate-500">IP Address</p></div>
            <div><p className="rounded-lg bg-white/[0.04] px-2 py-1 font-mono text-base text-slate-100">{allocation.port}</p><p className="mt-1 text-xs uppercase text-slate-500">Port</p></div>
            <div className="min-w-0"><p className="rounded-xl bg-white/[0.03] px-4 py-3 font-semibold text-slate-200">{allocation.alias || allocation.notes || "No alias or notes"}</p><p className="mt-1 break-all font-mono text-xs text-slate-500">{allocation.id}</p></div>
            <div className="flex flex-wrap gap-2 md:justify-end">
              <button className="ui-button ui-button-secondary" disabled={!canUpdate || pending || aliasMutation.isPending} onClick={() => { const alias = window.prompt("Allocation alias", allocation.alias ?? ""); if (alias === null) return; const notes = window.prompt("Allocation notes", allocation.notes ?? ""); if (notes === null) return; aliasMutation.mutate({ id: allocation.id, alias: alias.trim(), notes: notes.trim() }); }} type="button">Edit alias</button>
              {primary ? <span className="w-fit rounded bg-emerald-600 px-3 py-2 text-sm font-bold text-white">Primary</span> : <button className="ui-button ui-button-secondary" disabled={!canUpdate || pending} onClick={() => primaryMutation.mutate(allocation.id)} type="button">Make primary</button>}
              <button aria-label={`Unassign ${allocation.ip}:${allocation.port}`} className="ui-button ui-button-danger" disabled={!canDelete || primary || pending} onClick={() => setUnassignTarget(allocation)} title={primary ? "Choose another primary allocation before unassigning this one" : "Unassign allocation"} type="button"><Trash2 size={14} /> Unassign</button>
            </div>
          </div>
        );
      })}
      <ConfirmDialog
        confirmAction={() => { if (unassignTarget) unassignMutation.mutate(unassignTarget.id); }}
        confirmLabel={unassignTarget ? `Unassign ${unassignTarget.ip}:${unassignTarget.port}` : "Unassign"}
        description={`Unassigning this allocation frees its IP and port. The server will keep running on its remaining allocations.`}
        destructive
        loading={unassignMutation.isPending}
        closeAction={() => { if (!unassignMutation.isPending) setUnassignTarget(null); }}
        open={Boolean(unassignTarget)}
        title={unassignTarget ? `Unassign ${unassignTarget.ip}:${unassignTarget.port}?` : ""}
      />
    </div>
  );
}
