"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowRightLeft, Eye, Loader2, RotateCcw, Workflow, Ban, CheckCircle2, Play } from "lucide-react";
import {
  cancelEvacuationPlan,
  cancelMigration,
  cancelRecoveryPlan,
  createEvacuationPlan,
  createMigration,
  createRecoveryPlan,
  executeEvacuationPlan,
  executeMigration,
  executeRecoveryPlan,
  fetchEvacuationPlan,
  fetchMigrationExecutorStatus,
  fetchMigrations,
  fetchRecoveryPlan,
  fetchRecoveryPlans,
  fetchServers,
  previewEvacuation,
  startRecoveryPlan,
  type ApiEvacuationPlan,
  type ApiEvacuationResult,
  type ApiMigration,
  type ApiRecoveryPlan,
} from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { AdminConfirmDialog, AdminErrorState, AdminPageHeader, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Pill } from "./admin-ui";
import { adminPageGuides } from "./admin-page-guides";
import { deploymentStatusTone } from "@/lib/api/status";
import { TableSkeleton } from "@/components/ui/loading-skeleton";
import { listRecentInstallWorkflows, executeInstallWorkflow } from "@/lib/api/installer";
import { createInstallWSManager } from "@/lib/api/install-ws";

const operationQueryKeys = [["migrations"], ["recovery"], ["nodes"], ["servers"]] as const;
const terminalStatuses = ["completed", "restored", "cancelled", "failed"];

type OperationFeedback = { tone: "success" | "error"; message: string } | null;

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Unknown error";
}

function MigrationActions({ migration, onAction, executorAvailable }: { migration: ApiMigration; onAction: () => void; executorAvailable?: boolean }) {
  const { toast } = useToast();
  const executeMut = useMutation({
    mutationFn: () => executeMigration(migration.id),
    onSuccess: () => { toast({ tone: "success", title: "Migration started" }); onAction(); },
    onError: (error) => toast({ tone: "error", title: "Execute failed", message: errorMessage(error) }),
  });
  const cancelMut = useMutation({
    mutationFn: () => cancelMigration(migration.id),
    onSuccess: () => { toast({ tone: "success", title: "Migration cancelled" }); onAction(); },
    onError: (error) => toast({ tone: "error", title: "Cancel failed", message: errorMessage(error) }),
  });

  const executeDisabled = !executorAvailable || executeMut.isPending;

  return (
    <div className="flex gap-1.5">
      {migration.status === "planned" && <Btn size="sm" tone="success" onClick={() => executeMut.mutate()} disabled={executeDisabled} title={executorAvailable ? undefined : "Workload transfer runtime not available"}>{executeMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />} Execute</Btn>}
      {!terminalStatuses.includes(migration.status) && <Btn size="sm" tone="danger" onClick={() => cancelMut.mutate()} disabled={cancelMut.isPending}>{cancelMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Ban size={12} />} Cancel</Btn>}
    </div>
  );
}

function RecoveryPlanDetails({ plan }: { plan: ApiRecoveryPlan }) {
  if (plan.items.length === 0) return <p className="mt-2 text-xs text-[var(--text-muted)]">No workloads were included in this recovery plan.</p>;

  return <div className="mt-3 space-y-2">{plan.items.map((item) => (
    <div className="rounded border border-[var(--line)] p-2.5 text-xs" key={item.id}>
      <div className="flex items-center justify-between gap-2"><code>{item.serverId}</code><Pill tone={deploymentStatusTone(item.status)}>{item.status}</Pill></div>
      <p className="mt-1 text-[var(--text-muted)]">{item.sourceNodeId} → {item.targetNodeId || "No target assigned"}</p>
      {item.sourceBackupName && <p className="mt-1 text-[var(--text-muted)]">Backup: {item.sourceBackupName}</p>}
      {item.reason && <p className="mt-1 text-amber-300">{item.reason}</p>}
    </div>
  ))}</div>;
}

export function AdminOperations() {
  const { toast } = useToast();
  const qc = useQueryClient();
  // Operations lists poll every 10s: migrations, recovery plans and installer
  // workflows are operator-watched queues, so they tick faster than the 30s
  // backup/cron surfaces.
  const migrations = useQuery({ queryKey: ["migrations"], queryFn: fetchMigrations, refetchInterval: 10_000 });
  const recoveries = useQuery({ queryKey: ["recovery"], queryFn: fetchRecoveryPlans, refetchInterval: 10_000 });
  const nodes = useNodesQuery();
  const servers = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
  const executorStatus = useQuery({ queryKey: ["migrations-executor"], queryFn: fetchMigrationExecutorStatus, staleTime: 30_000 });
  const executorAvailable = executorStatus.data?.available === true;
  const installerQ = useQuery({ queryKey: ["installer-workflows"], queryFn: () => listRecentInstallWorkflows(20), refetchInterval: 10_000 });

  const [serverId, setServerId] = useState("");
  const [targetNodeId, setTargetNodeId] = useState("");
  const [nodeId, setNodeId] = useState("");
  const [reason, setReason] = useState("");
  const [evacuation, setEvacuation] = useState<ApiEvacuationResult | null>(null);
  const [selectedRecoveryPlanId, setSelectedRecoveryPlanId] = useState<string | null>(null);
  const recoveryPlanQuery = useQuery({
    queryKey: ["recovery-plan", selectedRecoveryPlanId],
    queryFn: () => fetchRecoveryPlan(selectedRecoveryPlanId!),
    enabled: selectedRecoveryPlanId !== null,
  });
  const [evacuationPlanId, setEvacuationPlanId] = useState<string | null>(null);
  const evacuationPlanQuery = useQuery({
    queryKey: ["evacuation-plan", evacuationPlanId],
    queryFn: () => fetchEvacuationPlan(evacuationPlanId!),
    enabled: evacuationPlanId !== null,
    refetchInterval: (query) => query.state.data?.status === "running" ? 2_000 : false,
  });
  const [feedback, setFeedback] = useState<OperationFeedback>(null);

  const [evacuationPlanConfirmOpen, setEvacuationPlanConfirmOpen] = useState(false);
  const [recoveryPlanConfirmOpen, setRecoveryPlanConfirmOpen] = useState(false);
  const [evacuationStartConfirmOpen, setEvacuationStartConfirmOpen] = useState(false);
  const [evacuationCancelConfirmOpen, setEvacuationCancelConfirmOpen] = useState(false);
  const [recoveryStartConfirmOpen, setRecoveryStartConfirmOpen] = useState<string | null>(null);
  const [recoveryCancelConfirmOpen, setRecoveryCancelConfirmOpen] = useState<string | null>(null);
  const [recoveryPlanDetailsStartConfirmOpen, setRecoveryPlanDetailsStartConfirmOpen] = useState(false);

  const refreshOperations = () => Promise.all(operationQueryKeys.map((queryKey) => qc.invalidateQueries({ queryKey })));
  const reportSuccess = (message: string) => { setFeedback({ tone: "success", message }); toast({ tone: "success", title: message }); };
  const reportError = (title: string, error: unknown) => {
    const message = errorMessage(error);
    setFeedback({ tone: "error", message: `${title}: ${message}` });
    toast({ tone: "error", title, message });
  };

  const migrationMut = useMutation({
    mutationFn: () => createMigration({ serverId, targetNodeId: targetNodeId || undefined }),
    onSuccess: () => { setServerId(""); setTargetNodeId(""); reportSuccess("Migration plan created."); void refreshOperations(); },
    onError: (error) => reportError("Migration creation failed", error),
  });
  const previewMut = useMutation({
    mutationFn: () => previewEvacuation(nodeId),
    onSuccess: (result) => { setEvacuation(result); reportSuccess(`Evacuation preview loaded for ${result.items.length} workload(s).`); },
    onError: (error) => reportError("Evacuation preview failed", error),
  });
  const evacuationMut = useMutation({
    mutationFn: () => createEvacuationPlan(nodeId),
    onSuccess: (result) => { setEvacuation(result); setEvacuationPlanId(result.plan.id); qc.setQueryData<ApiEvacuationPlan>(["evacuation-plan", result.plan.id], result.plan); reportSuccess(`Evacuation plan ${result.plan.id} created.`); void refreshOperations(); },
    onError: (error) => reportError("Evacuation plan creation failed", error),
  });
  const executeEvacuationMut = useMutation({
    mutationFn: executeEvacuationPlan,
    onSuccess: (plan) => { setEvacuation({ plan, items: plan.items, preview: false }); setEvacuationPlanId(plan.id); qc.setQueryData<ApiEvacuationPlan>(["evacuation-plan", plan.id], plan); reportSuccess(`Evacuation plan ${plan.id} started.`); void refreshOperations(); },
    onError: (error) => reportError("Evacuation execution failed", error),
  });
  const cancelEvacuationMut = useMutation({
    mutationFn: cancelEvacuationPlan,
    onSuccess: (plan) => { setEvacuation({ plan, items: plan.items, preview: false }); qc.setQueryData<ApiEvacuationPlan>(["evacuation-plan", plan.id], plan); reportSuccess(`Evacuation plan ${plan.id} cancelled.`); },
    onError: (error) => reportError("Evacuation cancellation failed", error),
  });
  const recoveryMut = useMutation({
    mutationFn: () => createRecoveryPlan({ nodeId, reason: reason.trim() }),
    onSuccess: (plan) => { setReason(""); reportSuccess(`Recovery plan ${plan.id} created.`); void refreshOperations(); },
    onError: (error) => reportError("Recovery plan creation failed", error),
  });
  const executeRecoveryMut = useMutation({
    mutationFn: executeRecoveryPlan,
    onSuccess: (plan) => { reportSuccess(`Recovery plan ${plan.id} started.`); void refreshOperations(); void qc.invalidateQueries({ queryKey: ["recovery-plan"] }); },
    onError: (error) => reportError("Recovery start failed", error),
  });
  const startRecoveryMut = useMutation({
    mutationFn: startRecoveryPlan,
    onSuccess: (plan) => { reportSuccess(`Recovery plan ${plan.id} started.`); void refreshOperations(); },
    onError: (error) => reportError("Recovery start failed", error),
  });
  const cancelRecoveryMut = useMutation({
    mutationFn: cancelRecoveryPlan,
    onSuccess: (plan) => { reportSuccess(`Recovery plan ${plan.id} cancelled.`); void refreshOperations(); },
    onError: (error) => reportError("Recovery cancellation failed", error),
  });

  const displayedEvacuation = evacuationPlanQuery.data ? { plan: evacuationPlanQuery.data, items: evacuationPlanQuery.data.items, preview: false } : evacuation;
  const settledItems = displayedEvacuation?.items.filter((item) => ["completed", "failed", "cancelled"].includes(item.status)).length ?? 0;
  const evacuationProgress = displayedEvacuation?.items.length ? Math.round((settledItems / displayedEvacuation.items.length) * 100) : 0;
  const blockedItems = displayedEvacuation?.items.filter((i) => !i.eligible) ?? [];
  const eligibleItems = displayedEvacuation?.items.filter((i) => i.eligible) ?? [];
  const eligibleTargets = [...new Set(eligibleItems.map((i) => i.targetNodeId).filter(Boolean))];

  const startEvacuationDisabled = !executorAvailable || executeEvacuationMut.isPending;

  // Breadcrumbs come from the AdminShell registry trail; width comes from the
  // AdminPageLayout in app/admin/operations/page.tsx.
  return <div className="space-y-6">
    <AdminPageHeader
      title="Operations"
      description="Control-plane operation history and manual controls."
      info={adminPageGuides.operations}
    />
    {feedback && <div className={`mb-4 rounded-lg border p-3 text-sm ${feedback.tone === "error" ? "border-red-700/30 bg-red-900/10 text-red-300" : "border-emerald-700/30 bg-emerald-900/10 text-emerald-300"}`} role={feedback.tone === "error" ? "alert" : "status"}>{feedback.message}</div>}

    {!executorAvailable && executorStatus.isSuccess && (
      <div className="mb-4 rounded-lg border border-amber-700/30 bg-amber-900/10 p-3 text-sm text-amber-200">
        <strong>Planning-only mode:</strong> Workload transfer runtime is not available. Migration plans can be created but execution is disabled.
      </div>
    )}

    <div className="grid gap-5 xl:grid-cols-2">
      <Card><CardHeader title="Create migration record" icon={ArrowRightLeft} /><div className="grid gap-3 p-4">
        <label className="text-sm text-[var(--text-subtle)]">Server<select className="mt-1 h-9 w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3" value={serverId} onChange={(event) => setServerId(event.target.value)}><option value="">Select server…</option>{(servers.data ?? []).map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}</select></label>
        <label className="text-sm text-[var(--text-subtle)]">Target node (optional; planner may choose)<select className="mt-1 h-9 w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3" value={targetNodeId} onChange={(event) => setTargetNodeId(event.target.value)}><option value="">Automatic</option>{(nodes.data ?? []).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
        <Btn disabled={!serverId || migrationMut.isPending} onClick={() => migrationMut.mutate()}>{migrationMut.isPending && <Loader2 size={14} className="animate-spin" />}{migrationMut.isPending ? "Creating migration plan…" : "Create migration plan"}</Btn>
        {migrationMut.error && (
          <div className="rounded border border-red-700/30 bg-red-900/10 p-3 text-xs text-red-200">
            <p className="mb-1 font-semibold">Migration creation failed</p>
            <p>{errorMessage(migrationMut.error)}</p>
          </div>
        )}
      </div></Card>

      <Card><CardHeader title="Evacuation / recovery planning" icon={RotateCcw} /><div className="grid gap-3 p-4">
        <label className="text-sm text-[var(--text-subtle)]">Affected node<select className="mt-1 h-9 w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3" value={nodeId} onChange={(event) => { setNodeId(event.target.value); setEvacuation(null); }}><option value="">Select node…</option>{(nodes.data ?? []).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
        <Input label="Recovery reason" value={reason} onChange={setReason} placeholder="For example: node is unavailable" />
        <div className="flex flex-wrap gap-2">
          <Btn tone="ghost" disabled={!nodeId || previewMut.isPending} onClick={() => previewMut.mutate()}>{previewMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Eye size={13} />}{previewMut.isPending ? "Loading preview…" : "Preview evacuation"}</Btn>
          <Btn disabled={!nodeId || evacuationMut.isPending} onClick={() => setEvacuationPlanConfirmOpen(true)}>{evacuationMut.isPending && <Loader2 size={13} className="animate-spin" />}{evacuationMut.isPending ? "Saving plan…" : "Save evacuation plan"}</Btn>
          <Btn tone="warning" disabled={!nodeId || !reason.trim() || recoveryMut.isPending} onClick={() => setRecoveryPlanConfirmOpen(true)}>{recoveryMut.isPending && <Loader2 size={13} className="animate-spin" />}{recoveryMut.isPending ? "Creating recovery plan…" : "Create recovery plan"}</Btn>
        </div>
        <div className="rounded border border-amber-700/30 bg-amber-950/20 p-3 text-xs leading-5 text-amber-100">
          <p className="mb-1 font-semibold">What recovery restores</p>
          <ul className="list-disc space-y-1 pl-4">
            <li>Restores backup data to a destination daemon (independent verification by daemon)</li>
            <li><strong>Does not</strong> migrate server ownership or move allocations</li>
            <li>A verified backup must exist before recovery can proceed</li>
            <li>The destination node must be independently confirmed as reachable</li>
            <li>Allocation and ownership remain on the source node after restore</li>
            <li>Expected state after recovery: backup data present on destination, server record unchanged on source</li>
          </ul>
        </div>
      </div></Card>
    </div>

    <InstallerStreamingManager executionEnabled={Boolean(installerQ.data?.executionEnabled)} />

    {displayedEvacuation && <div className="mt-5"><Card><CardHeader title={displayedEvacuation.preview ? "Evacuation preview" : "Evacuation plan"} icon={Workflow} /><div className="p-4 text-sm">
      <div className="mb-3 flex flex-wrap items-center gap-2 text-[var(--text-subtle)]">
        <span>{displayedEvacuation.items.length} affected workload(s)</span>
        <Pill tone={deploymentStatusTone(displayedEvacuation.plan.status)}>{displayedEvacuation.plan.status}</Pill>
        {displayedEvacuation.preview && <span className="text-xs">Preview only — no plan has been saved.</span>}
      </div>
      <div className="mb-3 grid grid-cols-2 gap-3 text-xs sm:grid-cols-4">
        <div className="rounded border border-[var(--line)] p-2"><span className="text-[var(--text-muted)]">Affected node</span><p className="font-semibold">{displayedEvacuation.plan.nodeId}</p></div>
        <div className="rounded border border-[var(--line)] p-2"><span className="text-[var(--text-muted)]">Eligible workloads</span><p className="font-semibold text-emerald-300">{eligibleItems.length}</p></div>
        <div className="rounded border border-[var(--line)] p-2"><span className="text-[var(--text-muted)]">Blocked workloads</span><p className="font-semibold text-red-300">{blockedItems.length}</p></div>
        <div className="rounded border border-[var(--line)] p-2"><span className="text-[var(--text-muted)]">Destination nodes</span><p className="font-semibold">{eligibleTargets.length > 0 ? eligibleTargets.join(", ") : "None"}</p></div>
      </div>
      {blockedItems.length > 0 && (
        <div className="mb-3 rounded border border-red-700/30 bg-red-950/10 p-2 text-xs text-red-200">
          <p className="mb-1 font-semibold">Blocked workloads</p>
          {blockedItems.map((item) => (
            <p key={item.id} className="ml-2">{item.serverId}: {item.reason || "No eligible target"}</p>
          ))}
        </div>
      )}
      {!displayedEvacuation.preview && <div className="mb-3 flex flex-wrap items-center gap-3">
        {displayedEvacuation.plan.status === "pending" && <Btn tone="warning" disabled={startEvacuationDisabled} onClick={() => setEvacuationStartConfirmOpen(true)} title={executorAvailable ? undefined : "Workload transfer runtime not available"}>{executeEvacuationMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Play size={13} />}{executeEvacuationMut.isPending ? "Starting evacuation…" : "Start evacuation"}</Btn>}
        {displayedEvacuation.plan.status === "running" && <Btn tone="danger" disabled={cancelEvacuationMut.isPending} onClick={() => setEvacuationCancelConfirmOpen(true)}>{cancelEvacuationMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Ban size={13} />}{cancelEvacuationMut.isPending ? "Cancelling evacuation…" : "Cancel evacuation"}</Btn>}
        {displayedEvacuation.plan.status === "running" && <span className="text-xs text-[var(--text-muted)]">{settledItems}/{displayedEvacuation.items.length} workloads settled ({evacuationProgress}%)</span>}
      </div>}
      {displayedEvacuation.plan.status === "running" && <div className="mb-3 h-1.5 overflow-hidden rounded-full bg-white/[0.06]"><div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${evacuationProgress}%` }} /></div>}
      <div className="mt-3 space-y-2">{displayedEvacuation.items.map((item) => <div key={item.id} className="rounded border border-[var(--line)] p-3"><div className="flex items-center justify-between gap-3"><code>{item.serverId}</code><Pill tone={item.eligible ? deploymentStatusTone(item.status) : "red"}>{item.eligible ? item.status : "Blocked"}</Pill></div><p className="mt-1 text-xs text-[var(--text-muted)]">{item.sourceNodeId} → {item.targetNodeId || "No eligible target"}</p>{item.reason && <p className="mt-1 text-xs text-[var(--text-muted)]">{item.reason}</p>}{item.error && <p className="mt-1 text-xs text-red-300">{item.error}</p>}{item.migrationId && <p className="mt-1 font-mono text-xs text-[var(--text-muted)]">Migration: {item.migrationId}</p>}</div>)}</div>
    </div></Card></div>}

    <div className="mt-5 grid gap-5 xl:grid-cols-2">
      <Card><CardHeader title="Migration history" icon={ArrowRightLeft} />
        {migrations.isLoading ? <TableSkeleton rows={3} /> : migrations.isError ? <div className="p-4"><AdminErrorState message={`Could not load migrations: ${migrations.error.message}`} retry={() => void migrations.refetch()} /></div> : (migrations.data ?? []).length === 0 ? <EmptyState icon={ArrowRightLeft} title="No migrations" message="No migration plans have been created yet." /> : (
          <AdminTable label="Migration history">
            <AdminTHead>
              <AdminTh>Server</AdminTh>
              <AdminTh>Route</AdminTh>
              <AdminTh>Progress</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh><span /></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {(migrations.data ?? []).map((migration) => (
                <AdminTr key={migration.id}>
                  <AdminTd className="font-mono text-xs">{migration.serverId}</AdminTd>
                  <AdminTd className="text-xs text-[var(--text-subtle)]">{migration.sourceNodeId} → {migration.targetNodeId}{migration.error || migration.failureReason ? ` · ${migration.error ?? migration.failureReason}` : ""}</AdminTd>
                  <AdminTd>
                    {migration.progress != null ? (
                      <span className="flex items-center gap-2">
                        <span className="h-1.5 w-32 overflow-hidden rounded-full bg-white/[0.06]"><span className="block h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${migration.progress}%` }} /></span>
                        <span className="text-xs text-[var(--text-muted)]">{migration.progress}%</span>
                      </span>
                    ) : <span className="text-[var(--text-muted)]">—</span>}
                  </AdminTd>
                  <AdminTd><Pill tone={deploymentStatusTone(migration.status)}>{migration.status}</Pill></AdminTd>
                  <AdminTd><MigrationActions migration={migration} executorAvailable={executorAvailable} onAction={() => void refreshOperations()} /></AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
      <Card><CardHeader title="Recovery plans" icon={AlertTriangle} />
        {recoveries.isLoading ? <TableSkeleton rows={3} /> : recoveries.isError ? <div className="p-4"><AdminErrorState message={`Could not load recovery plans: ${recoveries.error.message}`} retry={() => void recoveries.refetch()} /></div> : (recoveries.data ?? []).length === 0 ? <EmptyState icon={AlertTriangle} title="No recovery plans" message="No recovery plans have been created yet." /> : <div className="divide-y divide-[var(--line)]">{(recoveries.data ?? []).map((plan) => <div className="p-4" key={plan.id}><div className="flex items-center justify-between gap-3"><div><p className="font-mono text-xs">{plan.nodeId}</p><p className="text-xs text-[var(--text-muted)]">{plan.reason}</p><p className="mt-1 text-xs text-[var(--text-muted)]">{plan.items.length} workload(s)</p></div><div className="flex flex-wrap items-center justify-end gap-2"><Pill tone={deploymentStatusTone(plan.status)}>{plan.status}</Pill>{plan.status === "planned" && <Btn size="sm" tone="warning" onClick={() => setRecoveryStartConfirmOpen(plan.id)} disabled={startRecoveryMut.isPending}>{startRecoveryMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />} Start</Btn>}{!terminalStatuses.includes(plan.status) && <Btn size="sm" tone="danger" onClick={() => setRecoveryCancelConfirmOpen(plan.id)} disabled={cancelRecoveryMut.isPending}>{cancelRecoveryMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Ban size={12} />} Cancel</Btn>}</div></div><RecoveryPlanDetails plan={plan} /></div>)}</div>}
      </Card>
    </div>

    <div className="mt-5">
      <Card>
        <CardHeader title="Recovery plan details" icon={AlertTriangle} />
        <div className="space-y-3 p-4">
          <label className="block text-sm text-[var(--text-subtle)]">Plan
            <select className="mt-1 h-9 w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3" value={selectedRecoveryPlanId ?? ""} onChange={(event) => setSelectedRecoveryPlanId(event.target.value || null)}>
              <option value="">Select a recovery plan…</option>
              {(recoveries.data ?? []).map((plan) => <option key={plan.id} value={plan.id}>{plan.nodeId} — {plan.status}</option>)}
            </select>
          </label>
          {recoveryPlanQuery.isLoading && <p className="text-xs text-[var(--text-muted)]">Loading recovery plan details…</p>}
          {recoveryPlanQuery.isError && <p className="text-xs text-red-300">Recovery plan details could not be loaded.</p>}
          {recoveryPlanQuery.data && <>
            <div className="rounded border border-[var(--line)] p-3 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2"><span>{recoveryPlanQuery.data.reason}</span><Pill tone={deploymentStatusTone(recoveryPlanQuery.data.status)}>{recoveryPlanQuery.data.status}</Pill></div>
              <RecoveryPlanDetails plan={recoveryPlanQuery.data} />
            </div>
            {recoveryPlanQuery.data.status === "planned" && <Btn tone="warning" disabled={executeRecoveryMut.isPending} onClick={() => setRecoveryPlanDetailsStartConfirmOpen(true)}>{executeRecoveryMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Play size={13} />}{executeRecoveryMut.isPending ? "Starting recovery…" : "Start from plan details"}</Btn>}
          </>}
        </div>
      </Card>
    </div>

    <section className="mt-8">
      <div className="flex items-center gap-3">
        <h3 className="text-sm font-semibold">Install workflows</h3>
        <span className="rounded-full border border-[var(--line)] bg-white/[0.03] px-2 py-0.5 text-[11px] font-medium text-[var(--text-subtle)]">
          {installerQ.data?.executionEnabled ? "execution on (INSTALLER_WORKFLOW_ENABLED=1)" : "visibility only — execution deferred"}
        </span>
      </div>
      <p className="mt-1 text-xs text-[var(--text-subtle)]">
        6-step installer history persisted at <code className="rounded bg-white/10 px-1 py-0.5 font-mono text-[11px]">install_workflows</code> (create·setup·download·script·config·start). Now surfaced even when execution is manual.
      </p>
      {!installerQ.data?.executionEnabled ? (
        <div className="mt-3 flex gap-2 rounded-lg border border-amber-500/25 bg-amber-500/[0.08] px-3 py-2.5 text-xs leading-5 text-amber-200">
          <AlertTriangle size={14} className="shrink-0 mt-0.5" />
          <span>Execution is <b>intentionally deferred</b> (INSTALLER_WORKFLOW_ENABLED=0). Workflows are visible for audit/history; actual installs still run via the canonical Beacon path. When enabled, install streaming uses <code className="font-mono">GET /servers/:id/install/ws</code> (Beacon, admin-scoped) proxied via Forge <code className="font-mono">GET /servers/:id/ws/install</code> + ticket — see <code className="font-mono">install-ws.ts</code>.</span>
        </div>
      ) : null}
      <div className="mt-3 overflow-hidden rounded-xl border border-[var(--line)]">
        <table className="w-full text-sm">
          <thead className="bg-white/[0.02] text-left text-xs uppercase tracking-wider text-[var(--text-subtle)]"><tr className="border-b border-[var(--line)]"><th className="px-4 py-2.5 font-medium">Workflow</th><th className="px-4 py-2.5">Server</th><th className="px-4 py-2.5">Type</th><th className="px-4 py-2.5">Status</th><th className="px-4 py-2.5">Steps</th><th className="px-4 py-2.5">Created</th><th className="px-4 py-2.5">Action</th></tr></thead>
          <tbody className="divide-y divide-[var(--line)]">
            {(installerQ.data?.data ?? []).slice(0, 10).map((wf) => (
              <tr key={wf.id} className="hover:bg-white/[0.02]">
                <td className="px-4 py-3 font-mono text-xs">{wf.id.slice(0, 8)}</td>
                <td className="px-4 py-3 font-mono text-xs">{wf.serverId.slice(0, 8)}</td>
                <td className="px-4 py-3 text-xs">{wf.type}</td>
                <td className="px-4 py-3"><span className={`rounded-full border px-2 py-0.5 text-[11px] font-semibold ${wf.status === "completed" ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300" : wf.status === "failed" ? "border-red-500/30 bg-red-500/10 text-red-300" : wf.status === "running" ? "border-blue-500/30 bg-blue-500/10 text-blue-300" : "border-amber-500/30 bg-amber-500/10 text-amber-300"}`}>{wf.status}</span></td>
                <td className="px-4 py-3">
                  <div className="flex gap-1">
                    {wf.steps?.map((s) => (
                      <span key={s.id} title={`${s.name}: ${s.status}`} className={`h-2 w-2 rounded-full ${s.status === "completed" ? "bg-emerald-500" : s.status === "running" ? "bg-blue-500 animate-pulse" : s.status === "failed" ? "bg-red-500" : "bg-[var(--text-muted)]"}`}/>
                    ))}
                  </div>
                </td>
                <td className="px-4 py-3 text-xs text-[var(--text-subtle)]">{new Date(wf.createdAt).toLocaleString()}</td>
                <td className="px-4 py-3">
                  {installerQ.data?.executionEnabled ? (
                    <button onClick={() => executeInstallWorkflow(wf.id).then(() => qc.invalidateQueries({ queryKey: ["installer-workflows"] }))} className="text-xs underline">Execute</button>
                  ) : (
                    <span className="text-xs text-[var(--text-subtle)]">manual</span>
                  )}
                </td>
              </tr>
            ))}
            {(installerQ.data?.data?.length ?? 0) === 0 ? <tr><td colSpan={7} className="py-8 text-center text-sm text-[var(--text-subtle)]">No workflows yet — create one from a server&apos;s install action.</td></tr> : null}
          </tbody>
        </table>
      </div>
    </section>

    <AdminConfirmDialog
      open={evacuationPlanConfirmOpen}
      title="Save evacuation plan?"
      description="Save this evacuation plan? It will not begin moving workloads until you start it."
      confirmLabel="Save plan"
      onCancel={() => setEvacuationPlanConfirmOpen(false)}
      onConfirm={() => { setEvacuationPlanConfirmOpen(false); evacuationMut.mutate(); }}
      loading={evacuationMut.isPending}
    />

    <AdminConfirmDialog
      open={recoveryPlanConfirmOpen}
      title="Create recovery plan?"
      description="Create this backup-only recovery plan? A restored backup does not move server ownership."
      confirmLabel="Create plan"
      onCancel={() => setRecoveryPlanConfirmOpen(false)}
      onConfirm={() => { setRecoveryPlanConfirmOpen(false); recoveryMut.mutate(); }}
      loading={recoveryMut.isPending}
    />

    <AdminConfirmDialog
      open={evacuationStartConfirmOpen}
      title="Start evacuation?"
      description="Start workload evacuation for this plan?"
      confirmLabel="Start evacuation"
      onCancel={() => setEvacuationStartConfirmOpen(false)}
      onConfirm={() => { setEvacuationStartConfirmOpen(false); executeEvacuationMut.mutate(displayedEvacuation!.plan.id); }}
      loading={executeEvacuationMut.isPending}
    />

    <AdminConfirmDialog
      open={evacuationCancelConfirmOpen}
      title="Cancel evacuation?"
      description="Cancel this evacuation? Active migrations will be cancelled and no further workloads will start."
      confirmLabel="Cancel evacuation"
      destructive
      onCancel={() => setEvacuationCancelConfirmOpen(false)}
      onConfirm={() => { setEvacuationCancelConfirmOpen(false); cancelEvacuationMut.mutate(displayedEvacuation!.plan.id); }}
      loading={cancelEvacuationMut.isPending}
    />

    <AdminConfirmDialog
      open={recoveryStartConfirmOpen !== null}
      title="Start recovery?"
      description="Start workload recovery for this plan?"
      confirmLabel="Start recovery"
      onCancel={() => setRecoveryStartConfirmOpen(null)}
      onConfirm={() => { const id = recoveryStartConfirmOpen; setRecoveryStartConfirmOpen(null); if (id) startRecoveryMut.mutate(id); }}
      loading={startRecoveryMut.isPending}
    />

    <AdminConfirmDialog
      open={recoveryCancelConfirmOpen !== null}
      title="Cancel recovery plan?"
      description="Cancel this recovery plan?"
      confirmLabel="Cancel plan"
      destructive
      onCancel={() => setRecoveryCancelConfirmOpen(null)}
      onConfirm={() => { const id = recoveryCancelConfirmOpen; setRecoveryCancelConfirmOpen(null); if (id) cancelRecoveryMut.mutate(id); }}
      loading={cancelRecoveryMut.isPending}
    />

    <AdminConfirmDialog
      open={recoveryPlanDetailsStartConfirmOpen}
      title="Start backup-only recovery plan?"
      description="Start this backup-only recovery plan? Restored does not mean ownership was moved."
      confirmLabel="Start recovery"
      onCancel={() => setRecoveryPlanDetailsStartConfirmOpen(false)}
      onConfirm={() => { const id = selectedRecoveryPlanId; setRecoveryPlanDetailsStartConfirmOpen(false); if (id) executeRecoveryMut.mutate(id); }}
      loading={executeRecoveryMut.isPending}
    />
  </div>;
}

function InstallerStreamingManager({ executionEnabled }: { executionEnabled: boolean }) {
  const [serverId, setServerId] = useState("");
  const [status, setStatus] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const mgrRef = useRef<ReturnType<typeof createInstallWSManager> | null>(null);
  const serversQ = useQuery({ queryKey: ["servers", "install-ws"], queryFn: fetchServers, enabled: executionEnabled });
  const start = () => {
    if (!serverId) return;
    setLogs([]);
    setStatus("Connecting to install stream…");
    const mgr = createInstallWSManager(serverId, {
      onLog: (line) => setLogs((p) => [...p.slice(-200), line]),
      onStatus: setStatus,
      onComplete: (success, code) => { setStatus(success ? `Complete (exit ${code ?? 0})` : `Failed (exit ${code ?? 1})`); setConnected(false); },
      onError: (e) => setStatus(`Error: ${e}`),
    });
    const origOnStatus = (mgr as unknown as { config: { onStatusChange?: (s: string) => void } }).config.onStatusChange;
    (mgr as unknown as { config: Record<string, unknown> }).config = {
      ...((mgr as unknown as { config: Record<string, unknown> }).config),
      onStatusChange: (s: string) => { setConnected(s === "connected"); origOnStatus?.(s as never); },
    };
    mgrRef.current?.disconnect();
    mgrRef.current = mgr;
    void mgr.connect();
    setConnected(true);
  };
  const stop = () => {
    mgrRef.current?.disconnect();
    mgrRef.current = null;
    setConnected(false);
    setStatus("Disconnected");
  };
  if (!executionEnabled) {
    return (
      <section className="mt-6 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
        <h3 className="text-sm font-semibold">Install streaming — ticket + WS manager</h3>
        <p className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">
          Beacon exposes <code className="font-mono">GET /servers/:id/install/ws</code> (server.go:1600, admin-scoped token). Forge proxies via <code className="font-mono">GET /servers/:id/ws/install</code> + <code className="font-mono">GET /servers/:id/install/ws</code> alias (server.go realtimeProxy, daemon MintAdminToken). Frontend manager: <code className="font-mono">forge/web/lib/api/install-ws.ts</code> — <code className="font-mono">connectInstallWebSocket</code>/<code className="font-mono">createInstallWSManager</code> (log/status/complete frames). Visible even when <code className="font-mono">INSTALLER_WORKFLOW_ENABLED=0</code> (docs-only), live once enabled.
        </p>
      </section>
    );
  }
  return (
    <section className="mt-6 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
      <h3 className="text-sm font-semibold">Install streaming — live</h3>
      <p className="mt-1 text-xs text-[var(--text-subtle)]">Admin-only: streams installer logs via <code className="font-mono">GET /servers/:id/install/ws</code> ticket + <code className="font-mono">createInstallWSManager</code>. Requires <code className="font-mono">INSTALLER_WORKFLOW_ENABLED=1</code> and admin role (beacon ScopeAdmin).</p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <select value={serverId} onChange={(e) => setServerId(e.target.value)} className="h-9 min-w-[200px] rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-sm">
          <option value="">Select server…</option>
          {(serversQ.data ?? []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </select>
        {!connected ? <button onClick={start} disabled={!serverId} className="rounded-lg bg-[var(--brand)] px-3 py-1.5 text-xs font-bold text-white disabled:opacity-40">Stream install</button> : <button onClick={stop} className="rounded-lg border border-[var(--line)] px-3 py-1.5 text-xs">Disconnect</button>}
        <span className="text-xs text-[var(--text-subtle)]">{status || (connected ? "Connected" : "Idle")}</span>
      </div>
      {logs.length ? <pre className="mt-3 max-h-40 overflow-auto rounded-lg bg-[var(--surface-input)] p-3 font-mono text-xs leading-5 text-[var(--text-subtle)]">{logs.slice(-30).join("\n")}</pre> : null}
    </section>
  );
}
