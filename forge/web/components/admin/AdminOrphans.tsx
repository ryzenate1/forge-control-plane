"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Database, RefreshCw, Server } from "lucide-react";
import { fetchOrphanRemediations, resolveDatabaseOrphanRemediation, resolveServerOrphanRemediation } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";
import { FreshnessBadge } from "./telemetry-ui";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  AdminSelect,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  SectionHeader,
} from "./admin-ui";

/**
 * The failed-deletion queue.
 *
 * Read the name carefully: this page is *not* an orphan scanner. It renders the
 * `server_orphan_remediations` / `database_orphan_remediations` rows that the
 * control plane writes when a force-delete could not be completed on the daemon.
 * There is no discovery endpoint anywhere in the API that looks for resources
 * with no owning record (`lib/api/drain.ts:60` returns a hardcoded `[]` for
 * exactly that reason), so an empty list evidences only "no failed deletion was
 * reported". Rendering it as "no orphans exist" would be a never-run check
 * dressed up as a clean one. The sidebar label and description still promise a
 * scan; that copy lives in the frozen `admin-registry.ts` and is reported upward.
 */
export function AdminOrphans() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [status, setStatus] = useState<"pending" | "resolved">("pending");
  const q = useQuery({
    queryKey: ["orphan-remediations", status],
    queryFn: () => fetchOrphanRemediations(status),
    retry: false,
  });
  const serverRemediations = useMemo(
    () => (Array.isArray(q.data?.serverRemediations) ? q.data!.serverRemediations : []),
    [q.data],
  );
  const databaseRemediations = useMemo(
    () => (Array.isArray(q.data?.databaseRemediations) ? q.data!.databaseRemediations : []),
    [q.data],
  );
  const resolveDB = useMutation({
    mutationFn: resolveDatabaseOrphanRemediation,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["orphan-remediations"] });
      toast({ tone: "success", title: "Database remediation marked resolved" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Could not resolve database remediation", message: e.message }),
  });
  const resolveSrv = useMutation({
    mutationFn: resolveServerOrphanRemediation,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["orphan-remediations"] });
      toast({ tone: "success", title: "Server remediation marked resolved" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Could not resolve server remediation", message: e.message }),
  });

  const nothingTracked = serverRemediations.length === 0 && databaseRemediations.length === 0;

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={sourceState(q)} />}
        sub="Servers and databases whose remote deletion failed and are waiting for manual cleanup. Nothing on this page scans for orphans — an empty queue means no failed deletion has been reported, not that the fleet has none."
        action={
          <div className="flex flex-wrap items-end gap-2">
            <AdminSelect
              label="Queue status"
              value={status}
              onChange={(v) => setStatus(v as "pending" | "resolved")}
              options={[
                { value: "pending", label: "Pending" },
                { value: "resolved", label: "Resolved" },
              ]}
            />
            <Btn size="sm" tone="ghost" ariaLabel="Refresh the failed-deletion queue" onClick={() => void q.refetch()} disabled={q.isFetching}>
              <RefreshCw size={13} className={q.isFetching ? "animate-spin" : ""} /> {q.isFetching ? "Refreshing…" : "Refresh"}
            </Btn>
          </div>
        }
      />

      {q.isLoading ? (
        <AdminLoadingState label="Loading the failed-deletion queue…" />
      ) : q.isError ? (
        <AdminErrorState
          message={`Could not load the failed-deletion queue: ${q.error instanceof Error ? q.error.message : "request failed"}. The queue state is unknown — this is not an empty queue.`}
          retry={() => void q.refetch()}
        />
      ) : (
        <div className="space-y-6">
          <div className="ui-alert ui-alert-warning flex items-start gap-2">
            <AlertCircle aria-hidden="true" className="mt-0.5 shrink-0" size={14} />
            <span>
              This is a <strong className="font-semibold">failure queue</strong>, not a scan. Forge records a row here only
              when a force-delete could not be completed on the daemon. No endpoint searches the fleet for resources without an
              owning record, so counts below describe reported failures — they cannot tell you whether orphans exist.
            </span>
          </div>

          {nothingTracked ? (
            <EmptyState
              icon={AlertCircle}
              title={status === "pending" ? "No failed deletions reported" : "No resolved deletions"}
              message={
                status === "pending"
                  ? "No server or database deletion has been reported as failed. Unreported is not the same as none: orphan discovery does not exist, so nothing here has been checked."
                  : "No remediation has been marked resolved yet."
              }
            />
          ) : (
            <div className="space-y-6">
              <Card>
                <CardHeader
                  title="Server deletions that failed"
                  icon={Server}
                  action={<Pill tone={serverRemediations.length > 0 ? "yellow" : "neutral"}>{serverRemediations.length}</Pill>}
                />
                {serverRemediations.length === 0 ? (
                  <p className="px-4 py-6 text-center text-sm text-text-subtle">
                    No {status} server deletion failures reported.
                  </p>
                ) : (
                  <div className="divide-y divide-line">
                    {serverRemediations.map((r) => {
                      const isResolving = resolveSrv.isPending && (resolveSrv.variables as string) === r.id;
                      return (
                        <div key={r.id} className="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between">
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-mono text-sm text-text">Server {r.serverId}</span>
                              <Pill tone={r.status === "pending" ? "yellow" : "green"}>{r.status}</Pill>
                            </div>
                            <p className="mt-1 break-all font-mono text-meta text-text-subtle">Node: {r.nodeUrl || "not reported"}</p>
                            <p className="mt-2 break-words text-meta text-danger">{r.daemonError || "No daemon error was reported."}</p>
                            <p className="mt-2 text-meta text-text-muted">Reported {formatDate(r.createdAt, "a date that was not reported")}</p>
                          </div>
                          {r.status === "pending" ? (
                            <Btn
                              size="sm"
                              tone="ghost"
                              disabled={resolveSrv.isPending}
                              onClick={() => {
                                void (async () => {
                                  const ok = await confirm({
                                    title: `Mark server ${r.serverId} as resolved?`,
                                    description: "This only closes the queue row. It does not retry the deletion or touch the remote host — confirm the remote resource has already been cleaned up.",
                                    confirmLabel: "Mark resolved",
                                  });
                                  if (ok) resolveSrv.mutate(r.id);
                                })();
                              }}
                            >
                              {isResolving ? "Resolving…" : "Mark resolved"}
                            </Btn>
                          ) : (
                            <span className="text-meta text-text-muted">Resolved {formatDate(r.resolvedAt, "not reported")}</span>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </Card>

              <Card>
                <CardHeader
                  title="Database deletions that failed"
                  icon={Database}
                  action={<Pill tone={databaseRemediations.length > 0 ? "yellow" : "neutral"}>{databaseRemediations.length}</Pill>}
                />
                {databaseRemediations.length === 0 ? (
                  <p className="px-4 py-6 text-center text-sm text-text-subtle">
                    No {status} database deletion failures reported.
                  </p>
                ) : (
                  <div className="divide-y divide-line">
                    {databaseRemediations.map((r) => {
                      const isResolving = resolveDB.isPending && (resolveDB.variables as string) === r.id;
                      return (
                        <div key={r.id} className="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between">
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-mono text-sm text-text">{r.database}</span>
                              <Pill tone={r.status === "pending" ? "yellow" : "green"}>{r.status}</Pill>
                            </div>
                            <p className="mt-1 break-all font-mono text-meta text-text-subtle">
                              {r.engine} · {r.host}:{r.port} · {r.username}@{r.remote}
                            </p>
                            <p className="mt-2 break-words text-meta text-danger">{r.reason || "No reason was reported."}</p>
                            <p className="mt-2 text-meta text-text-muted">Reported {formatDate(r.createdAt, "a date that was not reported")}</p>
                          </div>
                          {r.status === "pending" ? (
                            <Btn
                              size="sm"
                              tone="ghost"
                              disabled={resolveDB.isPending}
                              onClick={() => {
                                void (async () => {
                                  const ok = await confirm({
                                    title: `Mark ${r.database} as resolved?`,
                                    description: "This only closes the queue row. It does not retry the deletion — confirm the remote database has already been dropped.",
                                    confirmLabel: "Mark resolved",
                                  });
                                  if (ok) resolveDB.mutate(r.id);
                                })();
                              }}
                            >
                              {isResolving ? "Resolving…" : "Mark resolved"}
                            </Btn>
                          ) : (
                            <span className="text-meta text-text-muted">Resolved {formatDate(r.resolvedAt, "not reported")}</span>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </Card>
            </div>
          )}

          <Card className="p-4">
            <p className="text-meta leading-6 text-text-subtle">
              Marking a row resolved records the resolution and an audit entry. It does <strong className="font-semibold text-text">not</strong> retry
              the daemon deletion, so clean the remote host first. The automatic reaper for stale placements and allocations is a
              different job and lives at{" "}
              <a className="font-medium text-[var(--brand)] hover:underline" href="/admin/cleanup">Cleanup</a>.
            </p>
          </Card>
        </div>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
