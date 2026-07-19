"use client";

import { useState } from "react";
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
  fetchMigrations,
  fetchNodes,
  fetchRecoveryPlan,
  fetchRecoveryPlans,
  fetchServers,
  previewEvacuation,
  startRecoveryPlan,
  type ApiEvacuationPlan,
  type ApiEvacuationResult,
  type ApiMigration,
  type ApiRecoveryPlan,
} from "@/modules/infrastructure/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader } from "./admin-ui";

const operationQueryKeys = [["migrations"], ["recovery"], ["nodes"], ["servers"]] as const;
const terminalStatuses = ["completed", "restored", "cancelled", "failed"];

type OperationFeedback = { tone: "success" | "error"; message: string } | null;

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Unknown error";
}

function statusTone(status: string): "green" | "red" | "yellow" | "blue" {
  if (["completed", "restored"].includes(status)) return "green";
  if (["failed", "cancelled"].includes(status)) return "red";
  if (["planned", "pending", "planning"].includes(status)) return "yellow";
  return "blue";
}

function MigrationActions({ migration, onAction }: { migration: ApiMigration; onAction: () => void }) {
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

  return (
    <div className="flex gap-1.5">
      {migration.status === "planned" && <Btn size="sm" tone="success" onClick={() => executeMut.mutate()} disabled={executeMut.isPending}>{executeMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />} Execute</Btn>}
      {!terminalStatuses.includes(migration.status) && <Btn size="sm" tone="danger" onClick={() => cancelMut.mutate()} disabled={cancelMut.isPending}>{cancelMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Ban size={12} />} Cancel</Btn>}
    </div>
  );
}

function RecoveryPlanDetails({ plan }: { plan: ApiRecoveryPlan }) {
  if (plan.items.length === 0) return <p className="mt-2 text-xs text-slate-500">No workloads were included in this recovery plan.</p>;

  return <div className="mt-3 space-y-2">{plan.items.map((item) => (
    <div className="rounded border border-white/[0.06] p-2.5 text-xs" key={item.id}>
      <div className="flex items-center justify-between gap-2"><code>{item.serverId}</code><Pill tone={statusTone(item.status)}>{item.status}</Pill></div>
      <p className="mt-1 text-slate-500">{item.sourceNodeId} → {item.targetNodeId || "No target assigned"}</p>
      {item.sourceBackupName && <p className="mt-1 text-slate-500">Backup: {item.sourceBackupName}</p>}
      {item.reason && <p className="mt-1 text-amber-300">{item.reason}</p>}
    </div>
  ))}</div>;
}

export function AdminOperations() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const migrations = useQuery({ queryKey: ["migrations"], queryFn: fetchMigrations, refetchInterval: 10_000 });
  const recoveries = useQuery({ queryKey: ["recovery"], queryFn: fetchRecoveryPlans, refetchInterval: 10_000 });
  const nodes = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const servers = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
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

  return <div>
    <SectionHeader title="Migrations & Recovery" sub="Preview evacuation capacity, save an explicit plan, then start or recover workloads safely." />
    {feedback && <div className={`mb-4 rounded-lg border p-3 text-sm ${feedback.tone === "error" ? "border-red-700/30 bg-red-900/10 text-red-300" : "border-emerald-700/30 bg-emerald-900/10 text-emerald-300"}`} role={feedback.tone === "error" ? "alert" : "status"}>{feedback.message}</div>}

    <div className="grid gap-5 xl:grid-cols-2">
      <Card><CardHeader title="Create migration record" icon={ArrowRightLeft} /><div className="grid gap-3 p-4">
        <label className="text-sm text-slate-300">Server<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3" value={serverId} onChange={(event) => setServerId(event.target.value)}><option value="">Select server…</option>{(servers.data ?? []).map((server) => <option key={server.id} value={server.id}>{server.name}</option>)}</select></label>
        <label className="text-sm text-slate-300">Target node (optional; planner may choose)<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3" value={targetNodeId} onChange={(event) => setTargetNodeId(event.target.value)}><option value="">Automatic</option>{(nodes.data ?? []).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
        <Btn disabled={!serverId || migrationMut.isPending} onClick={() => migrationMut.mutate()}>{migrationMut.isPending && <Loader2 size={14} className="animate-spin" />}{migrationMut.isPending ? "Creating migration plan…" : "Create migration plan"}</Btn>
      </div></Card>

      <Card><CardHeader title="Evacuation / recovery planning" icon={RotateCcw} /><div className="grid gap-3 p-4">
        <label className="text-sm text-slate-300">Affected node<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3" value={nodeId} onChange={(event) => { setNodeId(event.target.value); setEvacuation(null); }}><option value="">Select node…</option>{(nodes.data ?? []).map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}</select></label>
        <Input label="Recovery reason" value={reason} onChange={setReason} placeholder="For example: node is unavailable" />
        <div className="flex flex-wrap gap-2">
          <Btn tone="ghost" disabled={!nodeId || previewMut.isPending} onClick={() => previewMut.mutate()}>{previewMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Eye size={13} />}{previewMut.isPending ? "Loading preview…" : "Preview evacuation"}</Btn>
          <Btn disabled={!nodeId || evacuationMut.isPending} onClick={() => { if (confirm("Save this evacuation plan? It will not begin moving workloads until you start it.")) evacuationMut.mutate(); }}>{evacuationMut.isPending && <Loader2 size={13} className="animate-spin" />}{evacuationMut.isPending ? "Saving plan…" : "Save evacuation plan"}</Btn>
          <Btn tone="warning" disabled={!nodeId || !reason.trim() || recoveryMut.isPending} onClick={() => { if (confirm("Create this backup-only recovery plan? A restored backup does not move server ownership.")) recoveryMut.mutate(); }}>{recoveryMut.isPending && <Loader2 size={13} className="animate-spin" />}{recoveryMut.isPending ? "Creating recovery plan…" : "Create recovery plan"}</Btn>
        </div>
        <div className="rounded border border-amber-700/30 bg-amber-950/20 p-3 text-xs leading-5 text-amber-100">Recovery only restores a backup independently verified by the destination daemon. <strong>Restored</strong> confirms backup data recovery; it does not move server ownership or allocations.</div>
      </div></Card>
    </div>

    {displayedEvacuation && <div className="mt-5"><Card><CardHeader title={displayedEvacuation.preview ? "Evacuation preview" : "Evacuation plan"} icon={Workflow} /><div className="p-4 text-sm">
      <div className="mb-3 flex flex-wrap items-center gap-2 text-slate-400"><span>{displayedEvacuation.items.length} affected workload(s)</span><Pill tone={statusTone(displayedEvacuation.plan.status)}>{displayedEvacuation.plan.status}</Pill>{displayedEvacuation.preview && <span className="text-xs">Preview only — no plan has been saved.</span>}</div>
      {!displayedEvacuation.preview && <div className="mb-3 flex flex-wrap items-center gap-3">
        {displayedEvacuation.plan.status === "pending" && <Btn tone="warning" disabled={executeEvacuationMut.isPending} onClick={() => { if (confirm("Start workload evacuation for this plan?")) executeEvacuationMut.mutate(displayedEvacuation.plan.id); }}>{executeEvacuationMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Play size={13} />}{executeEvacuationMut.isPending ? "Starting evacuation…" : "Start evacuation"}</Btn>}
        {displayedEvacuation.plan.status === "running" && <Btn tone="danger" disabled={cancelEvacuationMut.isPending} onClick={() => { if (confirm("Cancel this evacuation? Active migrations will be cancelled and no further workloads will start.")) cancelEvacuationMut.mutate(displayedEvacuation.plan.id); }}>{cancelEvacuationMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Ban size={13} />}{cancelEvacuationMut.isPending ? "Cancelling evacuation…" : "Cancel evacuation"}</Btn>}
        {displayedEvacuation.plan.status === "running" && <span className="text-xs text-slate-500">{settledItems}/{displayedEvacuation.items.length} workloads settled ({evacuationProgress}%)</span>}
      </div>}
      {displayedEvacuation.plan.status === "running" && <div className="mb-3 h-1.5 overflow-hidden rounded-full bg-white/[0.06]"><div className="h-full rounded-full bg-blue-500 transition-all" style={{ width: `${evacuationProgress}%` }} /></div>}
      <div className="mt-3 space-y-2">{displayedEvacuation.items.map((item) => <div key={item.id} className="rounded border border-white/[0.06] p-3"><div className="flex items-center justify-between gap-3"><code>{item.serverId}</code><Pill tone={item.eligible ? statusTone(item.status) : "red"}>{item.eligible ? item.status : "Blocked"}</Pill></div><p className="mt-1 text-xs text-slate-500">{item.sourceNodeId} → {item.targetNodeId || "No eligible target"}</p>{item.reason && <p className="mt-1 text-xs text-slate-500">{item.reason}</p>}{item.error && <p className="mt-1 text-xs text-red-300">{item.error}</p>}{item.migrationId && <p className="mt-1 font-mono text-xs text-slate-500">Migration: {item.migrationId}</p>}</div>)}</div>
    </div></Card></div>}

    <div className="mt-5 grid gap-5 xl:grid-cols-2">
      <Card><CardHeader title="Migration history" icon={ArrowRightLeft} />
        {migrations.isLoading ? <div className="py-10 text-center text-sm text-slate-500">Loading migrations...</div> : migrations.isError ? <div className="py-10 text-center text-sm text-red-300">Failed to load migrations. Check API connectivity.</div> : (migrations.data ?? []).length === 0 ? <EmptyState icon={ArrowRightLeft} message="No migrations." /> : <div className="divide-y divide-white/[0.04]">{(migrations.data ?? []).map((migration) => <div className="flex items-center justify-between gap-3 p-4" key={migration.id}><div><p className="font-mono text-xs">{migration.serverId}</p><p className="text-xs text-slate-500">{migration.sourceNodeId} → {migration.targetNodeId}{migration.error || migration.failureReason ? ` · ${migration.error ?? migration.failureReason}` : ""}</p>{migration.progress != null && <div className="mt-1.5 h-1.5 w-32 overflow-hidden rounded-full bg-white/[0.06]"><div className="h-full rounded-full bg-blue-500 transition-all" style={{ width: `${migration.progress}%` }} /></div>}</div><div className="flex items-center gap-2">{migration.progress != null && <span className="text-xs text-slate-500">{migration.progress}%</span>}<Pill tone={statusTone(migration.status)}>{migration.status}</Pill><MigrationActions migration={migration} onAction={() => void refreshOperations()} /></div></div>)}</div>}
      </Card>
      <Card><CardHeader title="Recovery plans" icon={AlertTriangle} />
        {recoveries.isLoading ? <div className="py-10 text-center text-sm text-slate-500">Loading recovery plans...</div> : recoveries.isError ? <div className="py-10 text-center text-sm text-red-300">Failed to load recovery plans. Check API connectivity.</div> : (recoveries.data ?? []).length === 0 ? <EmptyState icon={AlertTriangle} message="No recovery plans." /> : <div className="divide-y divide-white/[0.04]">{(recoveries.data ?? []).map((plan) => <div className="p-4" key={plan.id}><div className="flex items-center justify-between gap-3"><div><p className="font-mono text-xs">{plan.nodeId}</p><p className="text-xs text-slate-500">{plan.reason}</p><p className="mt-1 text-xs text-slate-500">{plan.items.length} workload(s)</p></div><div className="flex flex-wrap items-center justify-end gap-2"><Pill tone={statusTone(plan.status)}>{plan.status}</Pill>{plan.status === "planned" && <Btn size="sm" tone="warning" onClick={() => { if (confirm("Start workload recovery for this plan?")) startRecoveryMut.mutate(plan.id); }} disabled={startRecoveryMut.isPending}>{startRecoveryMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />} Start</Btn>}{!terminalStatuses.includes(plan.status) && <Btn size="sm" tone="danger" onClick={() => { if (confirm("Cancel this recovery plan?")) cancelRecoveryMut.mutate(plan.id); }} disabled={cancelRecoveryMut.isPending}>{cancelRecoveryMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Ban size={12} />} Cancel</Btn>}</div></div><RecoveryPlanDetails plan={plan} /></div>)}</div>}
      </Card>
    </div>

    <div className="mt-5">
      <Card>
        <CardHeader title="Recovery plan details" icon={AlertTriangle} />
        <div className="space-y-3 p-4">
          <label className="block text-sm text-slate-300">Plan
            <select className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3" value={selectedRecoveryPlanId ?? ""} onChange={(event) => setSelectedRecoveryPlanId(event.target.value || null)}>
              <option value="">Select a recovery plan…</option>
              {(recoveries.data ?? []).map((plan) => <option key={plan.id} value={plan.id}>{plan.nodeId} — {plan.status}</option>)}
            </select>
          </label>
          {recoveryPlanQuery.isLoading && <p className="text-xs text-slate-500">Loading recovery plan details…</p>}
          {recoveryPlanQuery.isError && <p className="text-xs text-red-300">Recovery plan details could not be loaded.</p>}
          {recoveryPlanQuery.data && <>
            <div className="rounded border border-white/[0.06] p-3 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2"><span>{recoveryPlanQuery.data.reason}</span><Pill tone={statusTone(recoveryPlanQuery.data.status)}>{recoveryPlanQuery.data.status}</Pill></div>
              <RecoveryPlanDetails plan={recoveryPlanQuery.data} />
            </div>
            {recoveryPlanQuery.data.status === "planned" && <Btn tone="warning" disabled={executeRecoveryMut.isPending} onClick={() => { if (confirm("Start this backup-only recovery plan? Restored does not mean ownership was moved.")) executeRecoveryMut.mutate(recoveryPlanQuery.data!.id); }}>{executeRecoveryMut.isPending ? <Loader2 size={13} className="animate-spin" /> : <Play size={13} />}{executeRecoveryMut.isPending ? "Starting recovery…" : "Start from plan details"}</Btn>}
          </>}
        </div>
      </Card>
    </div>
  </div>;
}
