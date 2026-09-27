"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Database, RefreshCw, Server } from "lucide-react";
import { fetchOrphanRemediations, resolveDatabaseOrphanRemediation, resolveServerOrphanRemediation } from "@/lib/api";
import { toast } from "@/components/ui/sonner";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { Btn, Card, CardHeader, Pill, AdminSelect, SectionHeader } from "./admin-ui";

export function AdminOrphans() {
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const [status, setStatus] = useState<"pending" | "resolved">("pending");
  const q = useQuery({ queryKey: ["orphan-remediations", status], queryFn: () => fetchOrphanRemediations(status) });
  const serverRemediations = useMemo(() => Array.isArray(q.data?.serverRemediations) ? q.data!.serverRemediations : [], [q.data]);
  const databaseRemediations = useMemo(() => Array.isArray(q.data?.databaseRemediations) ? q.data!.databaseRemediations : [], [q.data]);
  const resolveDB = useMutation({
    mutationFn: resolveDatabaseOrphanRemediation,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["orphan-remediations"] }); toast.success("Database orphan remediation resolved"); },
    onError: (e: Error) => toast.error(e.message || "Could not resolve database remediation"),
  });
  const resolveSrv = useMutation({
    mutationFn: resolveServerOrphanRemediation,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["orphan-remediations"] }); toast.success("Server orphan remediation resolved"); },
    onError: (e: Error) => toast.error(e.message || "Could not resolve server remediation"),
  });

  return (
    <div className="mx-auto w-full max-w-[1280px] space-y-6">
      <SectionHeader
        title="Orphan Remediation"
        sub="Force-deleted servers and databases that could not be removed remotely are tracked here. Resolve after manually confirming remote cleanup is complete."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <AdminSelect
              label=""
              value={status}
              onChange={(v) => setStatus(v as "pending" | "resolved")}
              options={[
                { value: "pending", label: "Pending" },
                { value: "resolved", label: "Resolved" },
              ]}
            />
            <Btn size="sm" tone="ghost" onClick={() => void q.refetch()} disabled={q.isFetching}>
              <RefreshCw size={13} className={q.isFetching ? "animate-spin" : ""} /> {q.isFetching ? "Refreshing…" : "Refresh"}
            </Btn>
          </div>
        }
      />

      {q.isLoading ? (
        <div className="mt-6 rounded-xl border border-[var(--line)] p-8 text-center text-sm text-[var(--text-subtle)]">Loading remediation tasks…</div>
      ) : q.isError ? (
        <div className="mt-6 rounded-xl border border-red-500/20 bg-red-950/10 p-4 text-sm text-red-200 flex items-start justify-between gap-4">
          <span>Could not load orphan remediation tasks: {q.error.message}</span>
          <Btn size="sm" tone="ghost" onClick={() => void q.refetch()}>Retry</Btn>
        </div>
      ) : (
        <div className="mt-6 space-y-6">
          <Card className="overflow-hidden">
            <CardHeader title="Server resources" icon={Server} />
            <div className="flex items-center gap-2 border-b border-white/[0.06] bg-[color-mix(in_srgb,var(--surface-input)_50%,transparent)] px-5 py-3 text-xs font-semibold uppercase tracking-widest text-slate-400">
              <Server size={14} /> Server orphans <Pill>{serverRemediations.length}</Pill> <span className="ml-auto font-normal normal-case tracking-normal text-[11px] text-slate-500">server_orphan_remediations {status}</span>
            </div>
            {serverRemediations.length === 0 ? (
              <div className="px-5 py-8 text-center text-sm text-slate-300">No {status} server orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-white/[0.06]">
                {serverRemediations.map((r) => {
                  const isResolving = resolveSrv.isPending && (resolveSrv.variables as string) === r.id;
                  return (
                    <div key={r.id} className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-slate-200">Server {r.serverId}</span>
                          <Pill tone={r.status === "pending" ? "yellow" : "green"}>{r.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-slate-400">Node: {r.nodeUrl}</p>
                        <p className="mt-2 break-words text-xs text-red-200">{r.daemonError}</p>
                        <p className="mt-2 text-xs text-slate-400">Reported {new Date(r.createdAt).toLocaleString()}</p>
                      </div>
                      {r.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveSrv.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Mark server ${r.serverId} as resolved?`, description: "Only do this after confirming its remote resource has been cleaned up.", confirmLabel: "Mark resolved" })) resolveSrv.mutate(r.id); })(); }}>
                          {isResolving ? "Resolving…" : "Mark resolved"}
                        </Btn>
                      ) : <span className="text-xs text-slate-400">Resolved {r.resolvedAt ? new Date(r.resolvedAt).toLocaleString() : ""}</span>}
                    </div>
                  );
                })}
              </div>
            )}
          </Card>

          <Card className="overflow-hidden">
            <CardHeader title="Database resources" icon={Database} />
            <div className="flex items-center gap-2 border-b border-white/[0.06] bg-[color-mix(in_srgb,var(--surface-input)_50%,transparent)] px-5 py-3 text-xs font-semibold uppercase tracking-widest text-slate-400">
              <Database size={14} /> Database orphans <Pill>{databaseRemediations.length}</Pill> <span className="ml-auto font-normal normal-case tracking-normal text-[11px] text-slate-500">database_orphan_remediations {status}</span>
            </div>
            {databaseRemediations.length === 0 ? (
              <div className="px-5 py-8 text-center text-sm text-slate-300">No {status} database orphan remediation tasks.</div>
            ) : (
              <div className="divide-y divide-white/[0.06]">
                {databaseRemediations.map((r) => {
                  const isResolving = resolveDB.isPending && (resolveDB.variables as string) === r.id;
                  return (
                    <div key={r.id} className="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center lg:justify-between">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-slate-200">{r.database}</span>
                          <Pill tone={r.status === "pending" ? "yellow" : "green"}>{r.status}</Pill>
                        </div>
                        <p className="mt-1 break-all font-mono text-xs text-slate-400">{r.engine} · {r.host}:{r.port} · {r.username}@{r.remote}</p>
                        <p className="mt-2 break-words text-xs text-red-200">{r.reason}</p>
                        <p className="mt-2 text-xs text-slate-400">Reported {new Date(r.createdAt).toLocaleString()}</p>
                      </div>
                      {r.status === "pending" ? (
                        <Btn size="sm" tone="ghost" disabled={resolveDB.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Mark ${r.database} as resolved?`, description: "Only do this after confirming its remote resource has been cleaned up.", confirmLabel: "Mark resolved" })) resolveDB.mutate(r.id); })(); }}>
                          {isResolving ? "Resolving…" : "Mark resolved"}
                        </Btn>
                      ) : <span className="text-xs text-slate-400">Resolved {r.resolvedAt ? new Date(r.resolvedAt).toLocaleString() : ""}</span>}
                    </div>
                  );
                })}
              </div>
            )}
          </Card>

          <div className="flex items-start gap-2 rounded-lg border border-amber-500/20 bg-amber-950/10 p-3 text-xs text-amber-200">
            <AlertCircle size={14} className="mt-0.5 shrink-0" />
            <span>Resolving marks the row as <code className="rounded bg-white/10 px-1">resolved</code> and appends an <code className="rounded bg-white/10 px-1">audit_events</code> row (target_type=orphan_remediation). It does not retry daemon deletion — clean the remote host first.</span>
          </div>
        </div>
      )}
      {renderConfirm()}
    </div>
  );
}
