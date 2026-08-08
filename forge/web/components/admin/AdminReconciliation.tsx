"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, ChevronDown, ChevronRight, Clock, FileWarning, FlaskConical, Loader2, Play, RefreshCw, ShieldAlert } from "lucide-react";
import {
  confirmReconcilePlan,
  executeReconcilePlan,
  fetchReconcileEvents,
  fetchReconcilePlans,
  fetchReconcileSummary,
  triggerReconcileAll,
  type ReconcileDiff,
  type DriftRecord,
  type ReconcilePlanRow,
} from "@/lib/api/reconciliation";
import { useToast } from "@/components/ui/toast";
import { AdminConfirmDialog, AdminPageHeader, Btn, Card, CardHeader, EmptyState, Pill } from "./admin-ui";
import { TableSkeleton } from "@/components/ui/loading-skeleton";

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Unknown error";
}

function stateTone(state: string): "green" | "red" | "yellow" | "blue" | "neutral" {
  if (["succeeded", "confirmed"].includes(state)) return "green";
  if (["failed", "cancelled"].includes(state)) return "red";
  if (["pending", "executing"].includes(state)) return "yellow";
  return "blue";
}

function diffTone(diffType: string): "green" | "red" | "yellow" | "blue" | "neutral" {
  if (diffType === "noop") return "green";
  if (diffType === "create") return "blue";
  if (diffType === "update") return "yellow";
  if (diffType === "delete") return "red";
  return "neutral";
}

function driftTone(severity: string): "green" | "red" | "yellow" | "blue" | "neutral" {
  if (severity === "critical") return "red";
  if (severity === "warning") return "yellow";
  if (severity === "info") return "blue";
  return "neutral";
}

function PlanDiffs({ diffs }: { diffs: ReconcileDiff[] }) {
  if (diffs.length === 0) return <p className="text-xs text-slate-500">No diffs.</p>;
  return (
    <div className="space-y-1.5">
      {diffs.map((diff, i) => (
        <div key={i} className="flex items-start gap-2 rounded border border-white/[0.06] p-2 text-xs">
          <Pill tone={diffTone(diff.diffType)} className="shrink-0">{diff.diffType}</Pill>
          <div className="min-w-0 flex-1">
            <p className="font-mono text-slate-200 break-all">{diff.resourceId}</p>
            <p className="text-slate-400">{diff.description}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

function PlanDrifts({ drifts }: { drifts: DriftRecord[] }) {
  if (drifts.length === 0) return <p className="text-xs text-slate-500">No drifts detected.</p>;
  return (
    <div className="space-y-1.5">
      {drifts.map((drift, i) => (
        <div key={i} className="flex items-start gap-2 rounded border border-amber-700/30 bg-amber-950/10 p-2 text-xs">
          <Pill tone={driftTone(drift.severity)} className="shrink-0">{drift.severity}</Pill>
          <div className="min-w-0 flex-1">
            <p className="font-mono text-amber-200 break-all">{drift.resourceId}</p>
            <p className="text-amber-300/80">{drift.driftKind}</p>
            <p className="mt-0.5 text-amber-400/60">Desired: {drift.desired} | Observed: {drift.observed}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

function PlanRow({ plan, onAction }: { plan: ReconcilePlanRow; onAction: () => void }) {
  const { toast } = useToast();
  const [expanded, setExpanded] = useState(false);
  const [executing, setExecuting] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [executeOpen, setExecuteOpen] = useState(false);

  const handleConfirm = async () => {
    setConfirmOpen(false);
    setExecuting(true);
    try {
      await confirmReconcilePlan(plan.id);
      toast({ tone: "success", title: "Plan confirmed and executed" });
      onAction();
    } catch (error) {
      toast({ tone: "error", title: "Confirm failed", message: errorMessage(error) });
    } finally {
      setExecuting(false);
    }
  };

  const handleExecute = async () => {
    setExecuteOpen(false);
    setExecuting(true);
    try {
      await executeReconcilePlan(plan.id);
      toast({ tone: "success", title: "Plan execution started" });
      onAction();
    } catch (error) {
      toast({ tone: "error", title: "Execute failed", message: errorMessage(error) });
    } finally {
      setExecuting(false);
    }
  };

  const canConfirm = plan.state === "pending" && !plan.confirmed;
  const canExecute = plan.state === "confirmed" || (plan.state === "pending" && plan.confirmed);
  const isTerminal = ["succeeded", "failed", "cancelled"].includes(plan.state);

  return (
    <div className="border-b border-white/[0.04] last:border-0">
      <div className="flex items-center gap-3 px-4 py-3 hover:bg-white/[0.02]">
        <button
          aria-label={expanded ? "Collapse details" : "Expand details"}
          className="shrink-0 text-slate-500 hover:text-slate-200"
          onClick={() => setExpanded(!expanded)}
          type="button"
        >
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-slate-200">{plan.resourceId}</span>
            <Pill tone={stateTone(plan.state)}>{plan.state}</Pill>
            {plan.destructive && <Pill tone="red">Destructive</Pill>}
          </div>
          <p className="mt-0.5 text-xs text-slate-500">
            {plan.resourceKind} · {plan.diffCount} diff(s) · {plan.driftCount} drift(s)
            {plan.error ? <span className="ml-2 text-red-300">Error: {plan.error}</span> : null}
          </p>
        </div>
        <div className="flex shrink-0 gap-1.5">
          {canConfirm && (
            <Btn size="sm" tone="primary" disabled={executing} onClick={() => setConfirmOpen(true)}>
              {executing ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />}
              Confirm & Execute
            </Btn>
          )}
          {canExecute && !isTerminal && (
            <Btn size="sm" tone="warning" disabled={executing} onClick={() => setExecuteOpen(true)}>
              {executing ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
              Execute
            </Btn>
          )}
        </div>
      </div>

      {expanded && (
        <div className="space-y-3 border-t border-white/[0.04] bg-white/[0.01] px-8 py-3">
          <div>
            <h4 className="mb-1.5 text-xs font-semibold text-slate-400 uppercase tracking-wider">Diffs ({plan.diffs.length})</h4>
            <PlanDiffs diffs={plan.diffs} />
          </div>
          <div>
            <h4 className="mb-1.5 text-xs font-semibold text-slate-400 uppercase tracking-wider">Drifts ({plan.drifts.length})</h4>
            <PlanDrifts drifts={plan.drifts} />
          </div>
        </div>
      )}

      <AdminConfirmDialog
        open={confirmOpen}
        title={plan.destructive ? "Confirm destructive plan?" : "Confirm plan?"}
        description={plan.destructive ? "This plan contains destructive changes (deletes). Review the diffs carefully before proceeding." : "Confirm this reconciliation plan. It will be queued for execution."}
        confirmLabel="Confirm"
        onCancel={() => setConfirmOpen(false)}
        onConfirm={handleConfirm}
        destructive={plan.destructive}
      />
      <AdminConfirmDialog
        open={executeOpen}
        title="Execute plan?"
        description="Execute this reconciliation plan now?"
        confirmLabel="Execute"
        onCancel={() => setExecuteOpen(false)}
        onConfirm={handleExecute}
      />
    </div>
  );
}

export function AdminReconciliation() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [triggerKind, setTriggerKind] = useState("all");

  const summary = useQuery({
    queryKey: ["reconcile-summary"],
    queryFn: fetchReconcileSummary,
    refetchInterval: 15_000,
  });

  const plans = useQuery({
    queryKey: ["reconcile-plans"],
    queryFn: () => fetchReconcilePlans(0, 100),
    refetchInterval: 15_000,
  });

  const events = useQuery({
    queryKey: ["reconcile-events"],
    queryFn: () => fetchReconcileEvents(undefined, 20),
  });

  const triggerMut = useMutation({
    mutationFn: () => triggerReconcileAll(triggerKind),
    onSuccess: (results) => {
      toast({ tone: "success", title: `Reconciliation triggered`, message: `${results.length} resource(s) processed` });
      void qc.invalidateQueries({ queryKey: ["reconcile-summary"] });
      void qc.invalidateQueries({ queryKey: ["reconcile-plans"] });
      void qc.invalidateQueries({ queryKey: ["reconcile-events"] });
    },
    onError: (error) => toast({ tone: "error", title: "Trigger failed", message: errorMessage(error) }),
  });

  const refreshAll = () => {
    void qc.invalidateQueries({ queryKey: ["reconcile-summary"] });
    void qc.invalidateQueries({ queryKey: ["reconcile-plans"] });
    void qc.invalidateQueries({ queryKey: ["reconcile-events"] });
  };

  const summaryData = summary.data;
  const planRows = plans.data?.data ?? [];
  const eventRows = events.data ?? [];

  return (
    <div>
      <AdminPageHeader
        title="Reconciliation"
        description="Detect drift between desired and observed resource state, review diffs, and apply corrections."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <select
              className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-200"
              value={triggerKind}
              onChange={(e) => setTriggerKind(e.target.value)}
            >
              <option value="all">All resources</option>
              <option value="servers">Servers only</option>
              <option value="nodes">Nodes only</option>
              <option value="compose_stacks">Compose stacks only</option>
            </select>
            <Btn
              tone="warning"
              disabled={triggerMut.isPending}
              onClick={() => triggerMut.mutate()}
            >
              {triggerMut.isPending ? <Loader2 size={14} className="animate-spin" /> : <FlaskConical size={14} />}
              {triggerMut.isPending ? "Reconciling…" : "Trigger reconciliation"}
            </Btn>
            <Btn tone="ghost" onClick={refreshAll}>
              <RefreshCw size={14} />
            </Btn>
          </div>
        }
      />

      {summary.isLoading ? (
        <div className="mb-6 grid grid-cols-2 gap-3 md:grid-cols-5">
          {Array.from({ length: 5 }, (_, i) => (
            <div key={i} className="h-24 animate-pulse rounded-xl bg-white/[0.04]" />
          ))}
        </div>
      ) : summary.isError ? (
        <div className="mb-4 rounded-lg border border-red-700/30 bg-red-900/10 p-3 text-sm text-red-200">
          Could not load summary: {errorMessage(summary.error)}
        </div>
      ) : summaryData ? (
        <div className="mb-6 grid grid-cols-2 gap-3 md:grid-cols-5">
          <div className="rounded-xl border border-white/[0.09] bg-[#111722] p-4">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500">
              <Clock size={12} /> Total Plans
            </div>
            <div className="text-2xl font-bold tracking-tight text-slate-100">{summaryData.totalPlans}</div>
          </div>
          <div className="rounded-xl border border-white/[0.09] bg-[#111722] p-4">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500">
              <Clock size={12} /> Pending
            </div>
            <div className="text-2xl font-bold tracking-tight text-amber-400">{summaryData.pendingPlans}</div>
          </div>
          <div className="rounded-xl border border-white/[0.09] bg-[#111722] p-4">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500">
              <AlertTriangle size={12} /> Failed
            </div>
            <div className="text-2xl font-bold tracking-tight text-red-400">{summaryData.failedPlans}</div>
          </div>
          <div className="rounded-xl border border-white/[0.09] bg-[#111722] p-4">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500">
              <FileWarning size={12} /> Drifts
            </div>
            <div className="text-2xl font-bold tracking-tight text-yellow-400">{summaryData.totalDrifts}</div>
          </div>
          <div className="rounded-xl border border-white/[0.09] bg-[#111722] p-4">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500">
              <ShieldAlert size={12} /> Unresolved
            </div>
            <div className="text-2xl font-bold tracking-tight text-blue-400">{summaryData.unresolved}</div>
          </div>
        </div>
      ) : null}

      <Card>
        <CardHeader title="Reconciliation Plans" icon={FlaskConical} />
        {plans.isLoading ? (
          <TableSkeleton rows={3} />
        ) : plans.isError ? (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load plans: {errorMessage(plans.error)}</span>
              <Btn size="sm" tone="ghost" onClick={() => void plans.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : planRows.length === 0 ? (
          <EmptyState icon={FlaskConical} message="No reconciliation plans yet. Trigger one above." />
        ) : (
          <div className="divide-y divide-white/[0.04]">
            {planRows.map((plan) => (
              <PlanRow key={plan.id} plan={plan} onAction={refreshAll} />
            ))}
          </div>
        )}
      </Card>

      <div className="mt-5">
        <Card>
          <CardHeader title="Recent Events" icon={AlertTriangle} />
          {events.isLoading ? (
            <TableSkeleton rows={3} />
          ) : events.isError ? (
            <div className="p-4">
              <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                Could not load events: {errorMessage(events.error)}
              </div>
            </div>
          ) : eventRows.length === 0 ? (
            <EmptyState icon={AlertTriangle} message="No reconciliation events yet." />
          ) : (
            <div className="divide-y divide-white/[0.04]">
              {eventRows.map((event) => (
                <div key={event.id} className="flex items-start gap-3 px-4 py-3">
                  <Pill tone={stateTone(event.eventType)} className="shrink-0 mt-0.5">{event.eventType}</Pill>
                  <div className="min-w-0 flex-1">
                    <p className="text-xs text-slate-200">{event.summary}</p>
                    <p className="mt-0.5 text-xs text-slate-500">
                      {event.resourceKind}/{event.resourceId}
                    </p>
                  </div>
                  <span className="shrink-0 text-xs text-slate-500">
                    {new Date(event.createdAt).toLocaleString()}
                  </span>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}
