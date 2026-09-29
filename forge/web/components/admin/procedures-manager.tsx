"use client";

import { useEffect, useState } from "react";
import { Clock, ListOrdered, Play, Plus, Trash2, Workflow } from "lucide-react";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageHeader,
  AdminPageLayout,
  AdminErrorState,
  AdminLoadingState,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Pill,
  AdminSelect,
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { formatDate } from "@/lib/utils";
import * as api from "@/lib/api/procedures";
import { sanitizeError } from "@/lib/sanitize";

const STEP_ACTIONS = [
  "run_command",
  "sleep",
  "run_procedure",
  "deploy_stack",
  "send_webhook",
  "run_build",
] as const;

type StepAction = (typeof STEP_ACTIONS)[number];

/** The primary config key each action reads, from internal/services/procedure/service.go. */
const ACTION_CONFIG_KEY: Record<StepAction, { key: string; label: string; placeholder: string; numeric?: boolean }> = {
  run_command: { key: "command", label: "Command", placeholder: "echo hello" },
  sleep: { key: "duration_seconds", label: "Duration (seconds)", placeholder: "10", numeric: true },
  run_procedure: { key: "procedure_id", label: "Procedure ID", placeholder: "procedure UUID" },
  deploy_stack: { key: "stack", label: "Stack", placeholder: "web" },
  send_webhook: { key: "url", label: "Webhook URL", placeholder: "https://…" },
  run_build: { key: "build_id", label: "Build ID", placeholder: "build UUID" },
};

function isStepAction(value: string): value is StepAction {
  return (STEP_ACTIONS as readonly string[]).includes(value);
}

type StepDraft = {
  name: string;
  action: StepAction;
  configValue: string;
  extraConfig: Array<{ key: string; value: string }>;
  maxRetries: number;
  timeoutSeconds: number;
  requiresApproval: boolean;
  continueOnFailure: boolean;
  rollbackEnabled: boolean;
};

const defaultStepDraft: StepDraft = {
  name: "step-1",
  action: "run_command",
  configValue: "echo hello",
  extraConfig: [],
  maxRetries: 3,
  timeoutSeconds: 300,
  requiresApproval: false,
  continueOnFailure: false,
  rollbackEnabled: false,
};

function draftToPayload(step: StepDraft, position: number) {
  const spec = ACTION_CONFIG_KEY[step.action];
  const config: Record<string, unknown> = {};
  const primary = step.configValue.trim();
  if (primary) {
    config[spec.key] = spec.numeric ? Number(primary) || 0 : primary;
  }
  for (const entry of step.extraConfig) {
    const key = entry.key.trim();
    if (!key) continue;
    const value = entry.value.trim();
    config[key] = /^-?\d+(\.\d+)?$/.test(value) ? Number(value) : value;
  }
  return {
    position,
    name: step.name.trim() || `step-${position + 1}`,
    action: step.action,
    config,
    maxRetries: step.maxRetries,
    timeoutSeconds: step.timeoutSeconds,
    requiresApproval: step.requiresApproval,
    continueOnFailure: step.continueOnFailure,
    rollbackEnabled: step.rollbackEnabled,
  };
}

function statusTone(status: string): "green" | "red" | "yellow" | "blue" | "neutral" | "unknown" {
  const s = status.toLowerCase();
  if (s === "succeeded" || s === "completed" || s === "approved") return "green";
  if (s === "failed" || s === "rejected" || s === "cancelled") return "red";
  if (s === "waiting_approval" || s === "pending") return "yellow";
  if (s === "running" || s === "in_progress") return "blue";
  // An unrecognised status is not neutral: the UI does not know what it means.
  return "unknown";
}

/**
 * Statuses the runner treats as finished. `procedure/service.go` only ever writes
 * `succeeded`, `failed` or `cancelled` on completion (plus `waiting_approval` and
 * `running` while in flight), so anything else is unknown rather than terminal.
 */
function isTerminalExecution(status: string): boolean {
  return ["succeeded", "completed", "failed", "cancelled", "rejected"].includes(status.toLowerCase());
}

/**
 * A verdict the backend cannot express.
 *
 * With `continueOnFailure` the execution is completed as `succeeded` even when red
 * `failed` steps sit underneath it (`service.go:300-301`), and steps after a
 * halting failure stay `pending` forever because no `skipped` status is ever
 * written (`:325-438`). Both are computed here from the steps so the list pill and
 * the step rows stop contradicting each other.
 */
function executionVerdict(exec: { status: string; steps: { status: string }[] }): { label: string; tone: ReturnType<typeof statusTone>; note: string } {
  const failed = exec.steps.filter((s) => s.status.toLowerCase() === "failed").length;
  const pending = exec.steps.filter((s) => s.status.toLowerCase() === "pending").length;
  const base = statusTone(exec.status);
  if (base === "green" && failed > 0) {
    return {
      label: `succeeded with ${failed} failed step${failed === 1 ? "" : "s"}`,
      tone: "yellow",
      note: "The procedure reached its end, but steps failed along the way because it is configured to continue on failure.",
    };
  }
  if (isTerminalExecution(exec.status) && pending > 0) {
    return {
      label: `${exec.status} · ${pending} step${pending === 1 ? "" : "s"} never ran`,
      tone: base,
      note: "These steps were left behind when the run halted. The control plane records them as pending, not skipped, so they are shown as never run.",
    };
  }
  return { label: exec.status, tone: base, note: "" };
}

/** Per-step log read: rows, or the reason there are no rows. */
type LogRead = { rows: api.ProcedureStepLog[]; error: string | null; loaded: boolean };

const LOGS_UNLOADED: LogRead = { rows: [], error: null, loaded: false };

export function ProceduresManager() {
  const [confirm, renderConfirm] = useConfirm();
  const [procedures, setProcedures] = useState<api.Procedure[]>([]);
  const [selected, setSelected] = useState<api.Procedure | null>(null);
  const [executions, setExecutions] = useState<api.ProcedureExecution[]>([]);
  const [selectedExec, setSelectedExec] = useState<api.ProcedureExecution | null>(null);
  const [logs, setLogs] = useState<Record<string, LogRead>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const pageSize = 20;
  const [listRefreshedAt, setListRefreshedAt] = useState<number | null>(null);
  const [executionsLoading, setExecutionsLoading] = useState(false);
  const [executionsError, setExecutionsError] = useState<string | null>(null);

  const [formName, setFormName] = useState("");
  const [formDescription, setFormDescription] = useState("");
  const [formEnabled, setFormEnabled] = useState(true);
  const [steps, setSteps] = useState<StepDraft[]>([{ ...defaultStepDraft }]);
  const [cron, setCron] = useState("");

  async function load() {
    setLoading(true);
    setError(null);
    try {
      const list = await api.listProcedures();
      setProcedures(list);
      setListRefreshedAt(Date.now());
      if (list.length > 0 && !selected) {
        const first = list[0];
        if (first) setSelected(first);
      }
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Failed to load procedures"));
    } finally {
      setLoading(false);
    }
  }

  async function loadExecutions(procId: string) {
    setExecutionsLoading(true);
    setExecutionsError(null);
    try {
      const execs = await api.listExecutions(procId, 20);
      setExecutions(execs);
    } catch (e) {
      setExecutions([]);
      setExecutionsError(sanitizeError(e instanceof Error ? e.message : "Failed to load executions"));
    } finally {
      setExecutionsLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (selected) void loadExecutions(selected.id);
  }, [selected]);

  /**
   * An execution that is still in flight is not a static record. `service.go:238`
   * runs the steps in a goroutine, so without this the panel showed a `running` row
   * frozen at "running" until the page was reloaded — a stale reading presented as
   * current. Polling stops as soon as nothing is in flight.
   */
  const hasInFlight =
    executions.some((e) => !isTerminalExecution(e.status)) ||
    (selectedExec !== null && !isTerminalExecution(selectedExec.status));

  useEffect(() => {
    if (!selected || !hasInFlight) return;
    const timer = setInterval(() => {
      void loadExecutions(selected.id);
      if (selectedExec) {
        void api.getExecution(selectedExec.id).then((fresh) => {
          setSelectedExec(fresh);
          for (const step of fresh.steps) {
            if (!logs[step.id]?.loaded) void loadStepLogs(step.id);
          }
        }).catch(() => {
          // The interval keeps retrying; a failed tick is surfaced by the next
          // successful read, and the per-step log box shows its own error state.
        });
      }
    }, 5_000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasInFlight, selected, selectedExec?.id]);

  /**
   * `PUT /procedures/:id` takes the same body as create
   * (`createProcedureRequest`, handlers_procedures.go:54), so an existing runbook
   * can now be enabled or disabled instead of being retyped. The whole definition is
   * replaced, which is why the payload round-trips every field.
   */
  async function handleToggleEnabled(proc: api.Procedure) {
    const ok = await confirm({
      title: `${proc.enabled ? "Disable" : "Enable"} “${proc.name}”?`,
      description: proc.enabled
        ? "Its schedule stops firing and it can no longer be run manually until it is re-enabled. Existing history is kept."
        : "Its cron schedule becomes eligible to fire again and manual runs are allowed.",
      confirmLabel: proc.enabled ? "Disable" : "Enable",
      danger: proc.enabled,
    });
    if (!ok) return;
    try {
      await api.updateProcedure(proc.id, {
        name: proc.name,
        description: proc.description,
        tenantId: proc.tenantId ?? null,
        enabled: !proc.enabled,
        steps: proc.steps.map((s, index) => ({
          position: index,
          name: s.name,
          action: s.action,
          config: s.config ?? {},
          maxRetries: s.maxRetries,
          timeoutSeconds: s.timeoutSeconds,
          requiresApproval: s.requiresApproval,
          continueOnFailure: s.continueOnFailure,
          rollbackEnabled: s.rollbackEnabled,
        })),
        schedule: proc.schedule
          ? { cronExpression: proc.schedule.cronExpression, timezone: proc.schedule.timezone, enabled: proc.schedule.enabled }
          : null,
      });
      setSuccess(`“${proc.name}” ${proc.enabled ? "disabled" : "enabled"}`);
      await load();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Update failed"));
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const payload: api.CreateProcedureRequest = {
      name: formName.trim(),
      description: formDescription.trim(),
      tenantId: null,
      enabled: formEnabled,
      steps: steps.map((step, index) => draftToPayload(step, index)),
      schedule: cron.trim() ? { cronExpression: cron.trim(), timezone: "UTC", enabled: true } : null,
    };
    try {
      const created = await api.createProcedure(payload);
      setSuccess(`Created ${created.name}`);
      setFormName("");
      setFormDescription("");
      setCron("");
      setSteps([{ ...defaultStepDraft }]);
      await load();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Create failed"));
    }
  }

  async function handleDelete(id: string, name: string) {
    const ok = await confirm({
      title: `Delete procedure “${name}”?`,
      description: "Its steps, schedule and history will stop here. This cannot be undone.",
      danger: true,
      confirmLabel: "Delete",
    });
    if (!ok) return;
    try {
      await api.deleteProcedure(id);
      setSuccess("Deleted");
      setSelected(null);
      await load();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Delete failed"));
    }
  }

  /**
   * Fetch one step's logs and *record whether the read worked*.
   *
   * The old code swallowed every per-step failure ("ignore per-step log failures")
   * and the log box then printed "No logs." — an unreadable log store reported as an
   * empty one. Failures land in `LogRead.error` and render as a retry prompt.
   */
  async function loadStepLogs(stepExecId: string) {
    try {
      const rows = await api.listStepLogs(stepExecId);
      setLogs((prev) => ({ ...prev, [stepExecId]: { rows, error: null, loaded: true } }));
    } catch (e) {
      setLogs((prev) => ({
        ...prev,
        [stepExecId]: { rows: [], error: sanitizeError(e instanceof Error ? e.message : "Log request failed"), loaded: false },
      }));
    }
  }

  async function handleViewExec(execId: string) {
    try {
      const exec = await api.getExecution(execId);
      setSelectedExec(exec);
      const targets = exec.steps.filter((step) => !logs[step.id]?.loaded);
      for (const step of targets) {
        await loadStepLogs(step.id);
      }
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Load execution failed"));
    }
  }

  /**
   * `POST /procedures/:id/execute` returns 202 and runs the steps in a background
   * goroutine (`service.go:238`), so "queued" is accurate here — unlike
   * Reconciliation's confirm endpoint, which executes inline. The confirmation names
   * what will run because a procedure executes commands and deploys stacks.
   */
  async function handleExecute() {
    if (!selected) return;
    const ok = await confirm({
      title: `Run procedure “${selected.name}”?`,
      description: `${selected.steps.length} step(s) will run now, including any commands, deployments and webhooks they define. Steps marked as approval gates pause until approved.`,
      confirmLabel: "Run procedure",
      danger: selected.steps.some((s) => s.action === "run_command" || s.action === "deploy_stack"),
    });
    if (!ok) return;
    try {
      const exec = await api.executeProcedure(selected.id);
      setSuccess(`Execution ${exec.id.slice(0, 8)} accepted — it runs in the background and refreshes below while it is in flight`);
      await loadExecutions(selected.id);
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Execute failed"));
    }
  }

  async function handleCancel(execId: string) {
    const ok = await confirm({
      title: "Cancel this execution?",
      description: "Steps already run are not rolled back; the runbook stops at its current step.",
      confirmLabel: "Cancel execution",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.cancelExecution(execId);
      setSuccess("Cancellation requested");
      if (selected) await loadExecutions(selected.id);
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Cancel failed"));
    }
  }

  async function handleApprove(stepExecId: string) {
    try {
      await api.approveStep(stepExecId);
      setSuccess("Approved");
      if (selectedExec) {
        const fresh = await api.getExecution(selectedExec.id);
        setSelectedExec(fresh);
      }
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Approve failed"));
    }
  }

  async function handleReject(stepExecId: string) {
    try {
      await api.rejectStep(stepExecId);
      setSuccess("Rejected");
      if (selectedExec) {
        const fresh = await api.getExecution(selectedExec.id);
        setSelectedExec(fresh);
      }
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Reject failed"));
    }
  }

  const updateStep = (index: number, patch: Partial<StepDraft>) => {
    setSteps((prev) => prev.map((step, i) => (i === index ? { ...step, ...patch } : step)));
  };

  const changeStepAction = (index: number, action: StepAction) => {
    setSteps((prev) =>
      prev.map((step, i) => (i === index ? { ...step, action, configValue: "", extraConfig: [] } : step)),
    );
  };

  return (
    <AdminPageLayout>
      <AdminPageHeader
        status={
          listRefreshedAt && !error ? (
            <span className="font-mono text-eyebrow text-text-muted">List refreshed {formatDate(listRefreshedAt)}</span>
          ) : error ? (
            <span className="font-mono text-eyebrow text-unknown">List not loaded</span>
          ) : null
        }
      />
      <OfflineBanner onRetry={() => void load()} />
      {error && (
        <AdminErrorState message={`The procedure list could not be loaded: ${error}. An empty list below is not evidence that none exist.`} retry={() => void load()} />
      )}
      {success && (
        <div role="status" className="ui-alert ui-alert-success items-center justify-between">
          <span>{success}</span> <Btn size="sm" tone="ghost" onClick={() => setSuccess(null)}>Dismiss</Btn>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        <Card>
          <CardHeader title="Procedures" icon={Workflow} action={procedures.length > 0 ? <Pill tone="neutral">{procedures.length} total</Pill> : undefined} />
          {loading ? (
            <AdminLoadingState label="Loading procedures…" />
          ) : error ? (
            <AdminErrorState message="No procedure list to show — the read failed." retry={() => void load()} />
          ) : procedures.length === 0 ? (
            <EmptyState icon={Workflow} title="No procedures defined" message="The list loaded successfully and contains nothing. Create one with steps such as a command, a pause, or a nested procedure." />
          ) : (
            <>
              <div className="space-y-2 max-h-[520px] overflow-auto">
                {procedures.slice(page * pageSize, (page + 1) * pageSize).map((p) => (
                  <button
                    key={p.id}
                    onClick={() => setSelected(p)}
                    aria-label={`Select procedure ${p.name}`}
                    className={`w-full text-left rounded-lg border p-3 motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)] ${selected?.id === p.id ? "border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]" : "border-[var(--line)] bg-[var(--surface)] hover:bg-[var(--surface-hover)]"}`}
                  >
                    <p className="flex items-center gap-2 text-sm font-bold text-[var(--text)]">
                      {p.name}
                      <Pill tone={p.enabled ? "green" : "neutral"}>{p.enabled ? "Enabled" : "Disabled"}</Pill>
                    </p>
                    <p className="mt-0.5 truncate text-xs text-[var(--text-subtle)]">{p.description || "—"} · {p.steps?.length ?? 0} steps {p.schedule ? `· cron ${p.schedule.cronExpression}` : ""}</p>
                  </button>
                ))}
              </div>
              {procedures.length > pageSize && (
                <div className="mt-3 flex items-center justify-between border-t border-[var(--line)] pt-3">
                  <span className="font-mono text-xs text-[var(--text-subtle)]">Page {page + 1} of {Math.ceil(procedures.length / pageSize)} · {procedures.length} total</span>
                  <div className="flex gap-2">
                    <Btn size="sm" tone="ghost" disabled={page === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>Prev</Btn>
                    <Btn size="sm" tone="ghost" disabled={(page + 1) * pageSize >= procedures.length} onClick={() => setPage((p) => p + 1)}>Next</Btn>
                  </div>
                </div>
              )}
            </>
          )}
          <div className="mt-3">
            <Btn size="sm" tone="ghost" onClick={() => void load()}>Refresh</Btn>
          </div>
        </Card>

        <Card>
          <CardHeader title={selected ? `Detail: ${selected.name}` : "Detail"} icon={ListOrdered} />
          {!selected ? (
            <EmptyState icon={ListOrdered} title="No procedure selected" message="Select a procedure to inspect its steps and schedule." />
          ) : (
            <div className="space-y-3">
              <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--text-subtle)]">
                <Pill tone={selected.enabled ? "green" : "neutral"}>{selected.enabled ? "Enabled" : "Disabled"}</Pill>
                {selected.schedule ? (
                  <span className="inline-flex items-center gap-1"><Clock size={12} /> {selected.schedule.cronExpression} ({selected.schedule.timezone})</span>
                ) : (
                  <span>No schedule</span>
                )}
              </div>
              <div className="space-y-2">
                {selected.steps.map((s, idx) => (
                  <div key={s.id || idx} className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs">
                    <p className="flex flex-wrap items-center gap-2 font-bold text-[var(--text)]">
                      <span>{idx + 1}. {s.name}</span>
                      <Pill tone="blue">{s.action}</Pill>
                    </p>
                    {Object.entries(s.config ?? {}).length > 0 ? (
                      <dl className="mt-2 space-y-1">
                        {Object.entries(s.config ?? {}).map(([key, value]) => (
                          <div key={key} className="flex items-center justify-between gap-2">
                            <dt className="font-mono text-eyebrow text-[var(--text-subtle)]">{key}</dt>
                            <dd className="max-w-[55%] break-all font-mono text-eyebrow text-[var(--text)]">{typeof value === "object" ? JSON.stringify(value) : String(value)}</dd>
                          </div>
                        ))}
                      </dl>
                    ) : (
                      <p className="mt-1 text-eyebrow text-[var(--text-subtle)]">This step declares no configuration.</p>
                    )}
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      <Pill tone="neutral">retries {s.maxRetries}</Pill>
                      <Pill tone="neutral">timeout {s.timeoutSeconds}s</Pill>
                      {s.requiresApproval ? <Pill tone="yellow">approval gate</Pill> : null}
                      {s.rollbackEnabled ? <Pill tone="blue">rollback</Pill> : null}
                      {s.continueOnFailure ? <Pill tone="neutral">continue on failure</Pill> : null}
                    </div>
                  </div>
                ))}
              </div>
              <div className="flex flex-wrap gap-2">
                <Btn size="sm" tone="primary" onClick={() => void handleExecute()}><Play size={12} /> Run procedure</Btn>
                <Btn size="sm" tone={selected.enabled ? "warning" : "primary"} onClick={() => void handleToggleEnabled(selected)}>
                  {selected.enabled ? "Disable" : "Enable"}
                </Btn>
                <Btn size="sm" tone="danger" onClick={() => { void handleDelete(selected.id, selected.name); }}><Trash2 size={12} /> Delete</Btn>
              </div>
              <div>
                <p className="text-xs font-bold uppercase text-[var(--text-subtle)]">Executions (last 20)</p>
                {executionsLoading ? (
                  <AdminLoadingState label="Loading executions…" />
                ) : executionsError ? (
                  <AdminErrorState message={`Execution history could not be loaded: ${executionsError}. “No executions” would be a guess.`} retry={() => void loadExecutions(selected.id)} />
                ) : executions.length === 0 ? (
                  <EmptyState icon={Clock} title="No executions" message="This procedure has never been run — the history loaded and is empty." />
                ) : (
                  <div className="mt-2 space-y-1 max-h-[260px] overflow-auto">
                    {executions.map((e) => {
                      const verdict = executionVerdict(e);
                      const terminal = isTerminalExecution(e.status);
                      return (
                        <div key={e.id} className="flex items-center justify-between gap-2 rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-xs">
                          <div className="min-w-0">
                            <p className="flex flex-wrap items-center gap-2 font-bold">
                              {e.id.slice(0, 8)}
                              <Pill tone={verdict.tone}>{verdict.label}</Pill>
                              <span className="font-normal text-[var(--text-subtle)]">· {e.trigger}</span>
                            </p>
                            <p className="text-[var(--text-subtle)]">{formatDate(e.createdAt, "no recorded time")}</p>
                          </div>
                          <div className="flex shrink-0 gap-1">
                            <Btn size="sm" tone="ghost" onClick={() => void handleViewExec(e.id)}>View</Btn>
                            <Btn
                              size="sm"
                              tone="ghost"
                              disabled={terminal}
                              title={terminal ? "This execution has already finished, so there is nothing to cancel." : undefined}
                              onClick={() => void handleCancel(e.id)}
                            >
                              Cancel
                            </Btn>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="Create procedure" icon={Plus} />
          <form onSubmit={handleCreate} className="space-y-3">
            <Input label="Name" value={formName} onChange={setFormName} placeholder="Nightly backup check" required />
            <Input label="Description" value={formDescription} onChange={setFormDescription} placeholder="What this runbook does" />
            <Input label="Cron (optional)" value={cron} onChange={setCron} placeholder="0 2 * * *" mono />
            <p className="-mt-2 text-xs text-[var(--text-subtle)]">Standard 5-field cron. Empty means manual runs only.</p>
            <div className="space-y-2">
              <p className="text-xs font-bold uppercase text-[var(--text-subtle)]">Steps</p>
              {steps.map((step, index) => {
                const spec = ACTION_CONFIG_KEY[step.action];
                return (
                  <div key={index} className="space-y-2 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3">
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-bold text-[var(--text)]">Step {index + 1}</span>
                      {steps.length > 1 ? (
                        <Btn size="sm" tone="ghost" onClick={() => setSteps((prev) => prev.filter((_, i) => i !== index))}>Remove</Btn>
                      ) : null}
                    </div>
                    <Input label="Step name" value={step.name} onChange={(v) => updateStep(index, { name: v })} placeholder={`step-${index + 1}`} mono />
                    <div>
                      <AdminSelect
                        label="Action"
                        value={step.action}
                        onChange={(value) => { if (isStepAction(value)) changeStepAction(index, value); }}
                        options={STEP_ACTIONS.map((action) => ({ value: action, label: action }))}
                      />
                    </div>
                    <Input
                      label={spec.label}
                      value={step.configValue}
                      onChange={(v) => updateStep(index, { configValue: v })}
                      placeholder={spec.placeholder}
                      type={spec.numeric ? "number" : "text"}
                      mono
                    />
                    {step.extraConfig.map((entry, entryIndex) => (
                      <div key={entryIndex} className="flex items-end gap-2">
                        <div className="flex-1">
                          <Input label="Key" value={entry.key} onChange={(v) => updateStep(index, { extraConfig: step.extraConfig.map((en, i) => (i === entryIndex ? { ...en, key: v } : en)) })} placeholder="key" mono />
                        </div>
                        <div className="flex-1">
                          <Input label="Value" value={entry.value} onChange={(v) => updateStep(index, { extraConfig: step.extraConfig.map((en, i) => (i === entryIndex ? { ...en, value: v } : en)) })} placeholder="value" mono />
                        </div>
                        <Btn size="sm" tone="ghost" ariaLabel={`Remove extra config entry ${entryIndex + 1} from step ${index + 1}`} onClick={() => updateStep(index, { extraConfig: step.extraConfig.filter((_, i) => i !== entryIndex) })}>✕</Btn>
                      </div>
                    ))}
                    <Btn size="sm" tone="ghost" onClick={() => updateStep(index, { extraConfig: [...step.extraConfig, { key: "", value: "" }] })}>+ Config entry</Btn>
                    <div className="grid grid-cols-2 gap-2">
                      <Input label="Max retries" type="number" value={String(step.maxRetries)} onChange={(v) => updateStep(index, { maxRetries: Number(v) || 0 })} />
                      <Input label="Timeout (s)" type="number" value={String(step.timeoutSeconds)} onChange={(v) => updateStep(index, { timeoutSeconds: Number(v) || 0 })} />
                    </div>
                    <div className="flex flex-wrap gap-3 text-xs text-[var(--text)]">
                      <label className="flex items-center gap-1.5"><input type="checkbox" checked={step.requiresApproval} onChange={(e) => updateStep(index, { requiresApproval: e.target.checked })} /> Approval gate</label>
                      <label className="flex items-center gap-1.5"><input type="checkbox" checked={step.continueOnFailure} onChange={(e) => updateStep(index, { continueOnFailure: e.target.checked })} /> Continue on failure</label>
                      <label className="flex items-center gap-1.5"><input type="checkbox" checked={step.rollbackEnabled} onChange={(e) => updateStep(index, { rollbackEnabled: e.target.checked })} /> Rollback</label>
                    </div>
                  </div>
                );
              })}
              <Btn size="sm" tone="ghost" onClick={() => setSteps((prev) => [...prev, { ...defaultStepDraft, name: `step-${prev.length + 1}` }])}><Plus size={12} /> Add step</Btn>
              <p className="text-xs text-[var(--text-subtle)]">Commands run from an allowlist. Approval gates pause execution until a step is approved.</p>
            </div>
            <label className="flex gap-2 items-center text-xs text-[var(--text)]"><input type="checkbox" checked={formEnabled} onChange={(e) => setFormEnabled(e.target.checked)} /> Enabled</label>
            <Btn type="submit" tone="primary">Create</Btn>
          </form>
        </Card>
      </div>

      {selectedExec && (
        <Card>
          <CardHeader
            title={`Execution ${selectedExec.id.slice(0, 8)}`}
            icon={Clock}
            action={<Pill tone={executionVerdict(selectedExec).tone}>{executionVerdict(selectedExec).label}</Pill>}
          />
          <p className="mb-1 text-xs text-[var(--text-subtle)]">
            trigger {selectedExec.trigger} · created {formatDate(selectedExec.createdAt, "no recorded time")}
            {selectedExec.completedAt ? ` · finished ${formatDate(selectedExec.completedAt)}` : " · no completion recorded"}
          </p>
          {executionVerdict(selectedExec).note ? (
            <p className="ui-alert ui-alert-warning mb-3">{executionVerdict(selectedExec).note}</p>
          ) : null}
          {!isTerminalExecution(selectedExec.status) ? (
            <p className="mb-3 text-xs text-text-subtle">Still in flight — this panel refreshes every 5 s until the run finishes.</p>
          ) : null}
          <div className="space-y-2">
            {selectedExec.steps.map((step) => {
              const stepStatus = step.status.toLowerCase();
              const neverRan = isTerminalExecution(selectedExec.status) && stepStatus === "pending";
              const read = logs[step.id] ?? LOGS_UNLOADED;
              return (
                <div key={step.id} className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <p className="flex flex-wrap items-center gap-2 text-sm font-bold text-[var(--text)]">
                      #{step.position + 1} {step.id.slice(0, 8)}
                      <Pill tone={neverRan ? "unknown" : statusTone(step.status)}>{neverRan ? "never ran" : step.status}</Pill>
                      <span className="text-xs font-normal text-[var(--text-subtle)]">
                        attempt {typeof step.attempt === "number" ? step.attempt : "—"}/{typeof step.maxAttempts === "number" ? step.maxAttempts : "not reported"}
                      </span>
                    </p>
                    <div className="flex gap-1">
                      <Btn size="sm" tone="success" disabled={step.status !== "waiting_approval"} onClick={() => void handleApprove(step.id)}>Approve</Btn>
                      <Btn size="sm" tone="danger" disabled={step.status !== "waiting_approval"} onClick={() => void handleReject(step.id)}>Reject</Btn>
                    </div>
                  </div>
                  {step.output && <p className="mt-2 font-mono text-xs text-[var(--text)]">Output: {step.output}</p>}
                  {step.error && <p className="mt-1 text-xs text-danger">Error: {step.error}</p>}
                  <div className="mt-2">
                    <p className="text-xs font-bold uppercase text-[var(--text-subtle)]">Logs</p>
                    {!read.loaded && read.error ? (
                      <div className="mt-1">
                        <AdminErrorState message={`Logs could not be read: ${read.error}. This is not “no logs”.`} retry={() => void loadStepLogs(step.id)} />
                      </div>
                    ) : !read.loaded ? (
                      <div className="mt-1 max-h-32 overflow-auto rounded border border-dashed border-[var(--line)] bg-[var(--surface-input)] p-2 font-mono text-eyebrow text-unknown">
                        Logs not fetched for this step yet.
                      </div>
                    ) : read.rows.length === 0 ? (
                      <div className="mt-1 max-h-32 overflow-auto rounded border border-[var(--line)] bg-[var(--surface-input)] p-2 font-mono text-eyebrow text-text-subtle">
                        The log read succeeded and returned no lines.
                      </div>
                    ) : (
                      <div className="mt-1 max-h-32 overflow-auto rounded border border-[var(--line)] bg-[var(--surface-input)] p-2 font-mono text-eyebrow">
                        {read.rows.map((l) => (
                          <div key={l.id} className="flex gap-2">
                            <span className="text-[var(--text-subtle)]">{l.level}</span>
                            <span>{l.message}</span>
                            <span className="ml-auto text-[var(--text-subtle)]">{formatDate(l.createdAt, "no time")}</span>
                          </div>
                        ))}
                      </div>
                    )}
                    <Btn size="sm" tone="ghost" onClick={() => void loadStepLogs(step.id)}>
                      {read.loaded ? "Reload logs" : "Fetch logs"}
                    </Btn>
                  </div>
                </div>
              );
            })}
            <Btn size="sm" tone="ghost" onClick={() => setSelectedExec(null)}>Close</Btn>
          </div>
        </Card>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
