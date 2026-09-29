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
  latestReconcileActivityAt,
  triggerReconcileAll,
  type ReconcileDiff,
  type DriftRecord,
  type ReconcilePlanRow,
} from "@/lib/api/reconciliation";
import { useToast } from "@/components/ui/toast";
import { AdminConfirmDialog, AdminErrorState, AdminPageHeader, AdminStatCard, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Pill } from "./admin-ui";
import { adminPageGuides } from "./admin-page-guides";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { deploymentStatusTone } from "@/lib/api/status";
import { formatDate } from "@/lib/utils";
import { TableSkeleton } from "@/components/ui/loading-skeleton";

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Unknown error";
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
  if (diffs.length === 0) return <p className="text-xs text-[var(--text-muted)]">No diffs recorded for this plan.</p>;
  return (
    <div className="space-y-1.5">
      {diffs.map((diff, i) => (
        <div key={i} className="flex items-start gap-2 rounded border border-[var(--line)] p-2 text-xs">
          <Pill tone={diffTone(diff.diffType)} className="shrink-0">{diff.diffType}</Pill>
          <div className="min-w-0 flex-1">
            <p className="font-mono text-[var(--text)] break-all">{diff.resourceId}</p>
            <p className="text-[var(--text-subtle)]">{diff.description}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

function PlanDrifts({ drifts }: { drifts: DriftRecord[] }) {
  // An empty list here is only meaningful because the plan itself was produced by
  // a run: the plan is the evidence. `PlanRow` is never rendered for a run that
  // did not happen.
  if (drifts.length === 0) return <p className="text-xs text-[var(--text-muted)]">No drift recorded against this plan.</p>;
  return (
    <div className="space-y-1.5">
      {drifts.map((drift, i) => (
        <div key={i} className="flex items-start gap-2 rounded border border-warn-line bg-warn-subtle p-2 text-xs">
          <Pill tone={driftTone(drift.severity)} className="shrink-0">{drift.severity}</Pill>
          <div className="min-w-0 flex-1">
            <p className="font-mono text-text break-all">{drift.resourceId}</p>
            <p className="text-text-subtle">{drift.driftKind}</p>
            <p className="mt-0.5 text-text-muted">Desired: {drift.desired} · Observed: {drift.observed}</p>
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

  // `POST /plans/:id/confirm` is `ConfirmAndExecute` (handlers_reconcile.go:50) —
  // it confirms *and runs* the plan, so the dialog says that instead of
  // promising a queue that does not exist.
  const handleConfirm = async () => {
    setConfirmOpen(false);
    setExecuting(true);
    try {
      await confirmReconcilePlan(plan.id);
      toast({ tone: "success", title: "Plan confirmed and executed" });
      onAction();
    } catch (error) {
      toast({ tone: "error", title: "Confirm and execute failed", message: errorMessage(error) });
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
    <div className="border-b border-[var(--line)] last:border-0">
      <div className="flex items-center gap-3 px-4 py-3 hover:bg-overlay-subtle">
        <button
          aria-label={expanded ? "Collapse plan details" : "Expand plan details"}
          aria-expanded={expanded}
          className="shrink-0 rounded text-text-muted hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          onClick={() => setExpanded(!expanded)}
          type="button"
        >
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-[var(--text)]">{plan.resourceId}</span>
            <Pill tone={deploymentStatusTone(plan.state)}>{plan.state}</Pill>
            {plan.destructive && <Pill tone="red">Destructive</Pill>}
          </div>
          <p className="mt-0.5 text-xs text-[var(--text-muted)]">
            {plan.resourceKind} · {plan.diffCount} diff(s) · {plan.driftCount} drift(s)
            {plan.error ? <span className="ml-2 text-danger">Error: {plan.error}</span> : null}
          </p>
          <p className="mt-0.5 text-xs text-[var(--text-subtle)]">Recorded {formatDate(plan.createdAt)}</p>
        </div>
        <div className="flex shrink-0 gap-1.5">
          {canConfirm && (
            <Btn size="sm" tone="primary" disabled={executing} onClick={() => setConfirmOpen(true)}>
              {executing ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />}
              Confirm &amp; Execute
            </Btn>
          )}
          {canExecute && !isTerminal && (
            <Btn size="sm" tone="warning" disabled={executing} onClick={() => setExecuteOpen(true)}>
              {executing ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
              Execute
            </Btn>
          )}
          {!canConfirm && !canExecute && !isTerminal && <Pill tone="neutral">No action available</Pill>}
        </div>
      </div>

      {expanded && (
        <div className="space-y-3 border-t border-[var(--line)] bg-overlay-subtle px-8 py-3">
          <div>
            <h3 className="mb-1.5 text-xs font-semibold text-[var(--text-subtle)] uppercase tracking-wider">Diffs ({plan.diffs.length})</h3>
            <PlanDiffs diffs={plan.diffs} />
          </div>
          <div>
            <h3 className="mb-1.5 text-xs font-semibold text-[var(--text-subtle)] uppercase tracking-wider">Drifts ({plan.drifts.length})</h3>
            <PlanDrifts drifts={plan.drifts} />
          </div>
        </div>
      )}

      <AdminConfirmDialog
        open={confirmOpen}
        title={plan.destructive ? "Confirm and run destructive plan?" : "Confirm and run plan?"}
        description={
          plan.destructive
            ? "This plan contains destructive changes (deletes). Confirming runs it immediately — the resources named in the diffs will be changed or removed."
            : "Confirming runs this reconciliation plan immediately: the diffs below are applied to the live resource. It is not queued for later review."
        }
        confirmLabel="Confirm and run"
        onCancel={() => setConfirmOpen(false)}
        onConfirm={handleConfirm}
        destructive={plan.destructive}
      />
      <AdminConfirmDialog
        open={executeOpen}
        title="Execute plan?"
        description="This applies the plan's changes to the live resource now."
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

  // Summary and plans poll every 15s — slower than the 10s operations queues,
  // faster than the 30s backup/cron surfaces. Events load on demand.
  const summary = useQuery({
    queryKey: ["reconcile-summary"],
    queryFn: fetchReconcileSummary,
    retry: false,
    refetchInterval: 15_000,
  });

  const plans = useQuery({
    queryKey: ["reconcile-plans"],
    queryFn: () => fetchReconcilePlans(0, 100),
    retry: false,
    refetchInterval: 15_000,
  });

  const events = useQuery({
    queryKey: ["reconcile-events"],
    queryFn: () => fetchReconcileEvents(undefined, 20),
    retry: false,
  });

  const triggerMut = useMutation({
    mutationFn: () => triggerReconcileAll(triggerKind),
    onSuccess: (results) => {
      toast({ tone: "success", title: "Reconciliation run finished", message: `${results.length} resource(s) produced a plan or result` });
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

  // The summary endpoint reports counts over `reconcile_plans` and nothing else:
  // no `lastRunAt`, no "has scanned" flag. So "has a reconciliation ever been
  // recorded?" can only be evidenced by the plans and events themselves. Until
  // every one of those three reads has settled we genuinely do not know, and the
  // KPIs render as unknown rather than as a calm all-zero board.
  const evidenceSettled =
    !summary.isPending && !summary.isError &&
    !plans.isPending && !plans.isError &&
    !events.isPending && !events.isError;
  const lastRecordedAt = evidenceSettled
    ? latestReconcileActivityAt(planRows, eventRows)
    : null;
  const neverRecorded = evidenceSettled && lastRecordedAt === null;
  const countersKnown = evidenceSettled && !neverRecorded;

  const kpi = (value: number | undefined, known: boolean) => (known && typeof value === "number" ? value : "—");

  return (
    <>
      <AdminPageHeader
        status={<FreshnessBadge state={sourceState(summary, 15_000)} />}
        info={adminPageGuides.reconciliation}
        action={
          <div className="flex flex-wrap items-center gap-2">
            <label className="sr-only" htmlFor="reconcile-trigger-kind">Reconciliation scope</label>
            <select
              aria-label="Reconciliation scope"
              className="ui-input h-9 w-auto"
              id="reconcile-trigger-kind"
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
              loading={triggerMut.isPending}
              onClick={() => triggerMut.mutate()}
            >
              {triggerMut.isPending ? <Loader2 size={14} className="animate-spin" /> : <FlaskConical size={14} />}
              {triggerMut.isPending ? "Reconciling…" : "Run reconciliation"}
            </Btn>
            <Btn ariaLabel="Refresh reconciliation data" tone="ghost" onClick={refreshAll}>
              <RefreshCw size={14} />
            </Btn>
          </div>
        }
      />

      {evidenceSettled && lastRecordedAt ? (
        <p className="text-meta text-text-subtle">
          Latest reconciliation record <span className="font-mono text-text">{formatDate(lastRecordedAt)}</span>.
        </p>
      ) : null}

      {neverRecorded ? (
        <div role="status" className="ui-alert ui-alert-warning flex items-start gap-2">
          <AlertTriangle aria-hidden="true" className="mt-0.5 shrink-0" size={14} />
          <span>
            No reconciliation plan or event has ever been recorded. The zero counters below mean{" "}
            <strong className="font-semibold">nothing has been checked</strong>, not that the fleet is in sync.
            Run a reconciliation to produce a drift reading.
          </span>
        </div>
      ) : null}

      {summary.isError ? (
        <AdminErrorState message={`Could not load the reconciliation summary: ${errorMessage(summary.error)}`} retry={() => void summary.refetch()} />
      ) : (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
          <AdminStatCard label="Recorded plans" value={evidenceSettled ? (summaryData?.totalPlans ?? "—") : "—"} icon={Clock} tone={neverRecorded ? "unknown" : "neutral"} />
          <AdminStatCard label="Pending" value={kpi(summaryData?.pendingPlans, countersKnown)} icon={Clock} tone={countersKnown ? "yellow" : "unknown"} />
          <AdminStatCard label="Failed" value={kpi(summaryData?.failedPlans, countersKnown)} icon={AlertTriangle} tone={countersKnown ? "red" : "unknown"} />
          <AdminStatCard label="Drifts" value={kpi(summaryData?.totalDrifts, countersKnown)} icon={FileWarning} tone={countersKnown ? "yellow" : "unknown"} />
          <AdminStatCard label="Unresolved" value={kpi(summaryData?.unresolved, countersKnown)} icon={ShieldAlert} tone={countersKnown ? "blue" : "unknown"} />
        </div>
      )}

      <Card>
        <CardHeader title="Reconciliation Plans" icon={FlaskConical} />
        {plans.isLoading ? (
          <TableSkeleton rows={3} />
        ) : plans.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Could not load plans: ${errorMessage(plans.error)}`} retry={() => void plans.refetch()} />
          </div>
        ) : planRows.length === 0 ? (
          <EmptyState
            icon={FlaskConical}
            title="No reconciliation plans recorded"
            message={
              neverRecorded
                ? "Nothing has run a reconciliation yet, so no drift reading exists. Trigger one above to generate plans."
                : "Reconciliation has run before, but no plans are currently recorded. Run a reconciliation to generate plans."
            }
          />
        ) : (
          <div className="divide-y divide-[var(--line)]">
            {planRows.map((plan) => (
              <PlanRow key={plan.id} plan={plan} onAction={refreshAll} />
            ))}
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title="Recent Events" icon={AlertTriangle} />
        {events.isLoading ? (
          <TableSkeleton rows={3} />
        ) : events.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Could not load events: ${errorMessage(events.error)}`} retry={() => void events.refetch()} />
          </div>
        ) : eventRows.length === 0 ? (
          <EmptyState
            icon={AlertTriangle}
            title="No reconciliation events"
            message="No reconciliation events have been reported. An empty event list is not evidence that resources are in sync."
          />
        ) : (
          <AdminTable label="Recent reconciliation events">
            <AdminTHead>
              <AdminTh>Event</AdminTh>
              <AdminTh>Summary</AdminTh>
              <AdminTh>Resource</AdminTh>
              <AdminTh>Observed</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {eventRows.map((event) => (
                <AdminTr key={event.id}>
                  <AdminTd><Pill tone={deploymentStatusTone(event.eventType)}>{event.eventType}</Pill></AdminTd>
                  <AdminTd className="text-xs text-[var(--text)]">{event.summary}</AdminTd>
                  <AdminTd className="font-mono text-xs text-[var(--text-subtle)]">{event.resourceKind}/{event.resourceId}</AdminTd>
                  <AdminTd className="whitespace-nowrap text-xs text-[var(--text-muted)]">{formatDate(event.createdAt)}</AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
    </>
  );
}
