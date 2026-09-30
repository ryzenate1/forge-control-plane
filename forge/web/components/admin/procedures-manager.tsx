"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Play, Plus, Trash2, Workflow } from "lucide-react";
import { OfflineBanner } from "@/components/shared/states-offline";
import * as api from "@/lib/api/procedures";
import { sanitizeError } from "@/lib/sanitize";
import { sourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { FreshnessBadge } from "./telemetry-ui";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Pill,
  SectionHeader,
  Textarea,
} from "./admin-ui";

// Procedures (multi-step runbooks) panel. Server state (procedure list,
// executions) lives in react-query; request-driven reads (execution detail,
// step logs) and writes (create/delete/execute/cancel/approve/reject) are
// mutations. Nothing is announced until the corresponding call resolves, and
// destructive actions are confirmed first.

function executionTone(status: string): "green" | "red" | "yellow" | "neutral" {
  const s = (status ?? "").toLowerCase();
  if (s === "succeeded" || s === "completed" || s === "success") return "green";
  if (s === "failed" || s === "error") return "red";
  if (s === "running" || s === "waiting_approval" || s === "pending") return "yellow";
  return "neutral";
}

export function ProceduresManager() {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const [selected, setSelected] = useState<api.Procedure | null>(null);
  const [selectedExec, setSelectedExec] = useState<api.ProcedureExecution | null>(null);
  const [logs, setLogs] = useState<Record<string, api.ProcedureStepLog[]>>({});
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const pageSize = 20;

  const [form, setForm] = useState<api.CreateProcedureRequest>({
    name: "",
    description: "",
    tenantId: null,
    enabled: true,
    steps: [
      { position: 0, name: "step-1", action: "run_command", config: { command: "echo hello" }, maxRetries: 3, timeoutSeconds: 300, requiresApproval: false, continueOnFailure: false, rollbackEnabled: false },
    ],
    schedule: null,
  });
  const [cron, setCron] = useState("");

  const proceduresQuery = useQuery({
    queryKey: ["procedures"],
    queryFn: () => api.listProcedures(),
    refetchInterval: 15_000,
  });
  const procedures = useMemo(() => proceduresQuery.data ?? [], [proceduresQuery.data]);

  const executionsQuery = useQuery({
    queryKey: ["procedure-executions", selected?.id],
    queryFn: () => api.listExecutions(selected!.id, 20),
    enabled: Boolean(selected?.id),
    refetchInterval: 10_000,
  });
  const executions = useMemo(() => executionsQuery.data ?? [], [executionsQuery.data]);

  useEffect(() => {
    if (!selected && procedures.length > 0) {
      const first = procedures[0];
      if (first) setSelected(first);
    }
  }, [procedures, selected]);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["procedures"] });
    if (selected) void queryClient.invalidateQueries({ queryKey: ["procedure-executions", selected.id] });
  };

  const createMut = useMutation({
    mutationFn: (payload: api.CreateProcedureRequest) => api.createProcedure(payload),
    onSuccess: (created) => {
      setSuccess(`Created ${created.name}`);
      setForm((prev) => ({ ...prev, name: "", description: "" }));
      setCron("");
      invalidate();
    },
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Create failed")),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteProcedure(id),
    onSuccess: () => {
      setSuccess("Deleted");
      setSelected(null);
      setSelectedExec(null);
      invalidate();
    },
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Delete failed")),
  });

  const executeMut = useMutation({
    mutationFn: (id: string) => api.executeProcedure(id),
    onSuccess: (exec) => {
      setSuccess(`Execution ${exec.id.slice(0, 8)} queued`);
      invalidate();
    },
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Execute failed")),
  });

  const cancelMut = useMutation({
    mutationFn: (execId: string) => api.cancelExecution(execId),
    onSuccess: () => {
      setSuccess("Cancelled");
      invalidate();
    },
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Cancel failed")),
  });

  const detailMut = useMutation({
    mutationFn: (execId: string) => api.getExecution(execId),
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Load execution failed")),
  });

  const stepLogsMut = useMutation({
    mutationFn: (stepId: string) => api.listStepLogs(stepId),
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Load logs failed")),
  });

  const approveMut = useMutation({
    mutationFn: (stepExecId: string) => api.approveStep(stepExecId),
    onSuccess: async () => {
      setSuccess("Approved");
      if (selectedExec) {
        try {
          const fresh = await api.getExecution(selectedExec.id);
          setSelectedExec(fresh);
        } catch (e) {
          setError(sanitizeError(e instanceof Error ? e.message : "Reload failed"));
        }
      }
    },
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Approve failed")),
  });

  const rejectMut = useMutation({
    mutationFn: (stepExecId: string) => api.rejectStep(stepExecId),
    onSuccess: async () => {
      setSuccess("Rejected");
      if (selectedExec) {
        try {
          const fresh = await api.getExecution(selectedExec.id);
          setSelectedExec(fresh);
        } catch (e) {
          setError(sanitizeError(e instanceof Error ? e.message : "Reload failed"));
        }
      }
    },
    onError: (err) => setError(sanitizeError(err instanceof Error ? err.message : "Reject failed")),
  });

  function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const payload: api.CreateProcedureRequest = {
      ...form,
      schedule: cron.trim() ? { cronExpression: cron.trim(), timezone: "UTC", enabled: true } : null,
    };
    createMut.mutate(payload);
  }

  async function handleDelete(id: string, name: string) {
    const ok = await confirm({
      title: `Delete procedure “${name}”?`,
      description: "The procedure, its steps and schedule are removed. Past executions are kept as audit history.",
      danger: true,
      confirmLabel: "Delete",
    });
    if (ok) deleteMut.mutate(id);
  }

  function handleExecute() {
    if (!selected) return;
    executeMut.mutate(selected.id);
  }

  async function handleViewExec(execId: string) {
    setError(null);
    try {
      const exec = await detailMut.mutateAsync(execId);
      setSelectedExec(exec);
      for (const step of exec.steps) {
        try {
          const stepLogs = await stepLogsMut.mutateAsync(step.id);
          setLogs((prev) => ({ ...prev, [step.id]: stepLogs }));
        } catch {
          // Per-step log failures must not hide the execution detail.
        }
      }
    } catch {
      // detailMut.onError already surfaced the failure.
    }
  }

  const paged = procedures.slice(page * pageSize, (page + 1) * pageSize);

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Procedures"
        sub="Multi-step runbooks with approval gates, cron scheduling, retry/rollback, and audit logging."
        status={<FreshnessBadge state={sourceState(proceduresQuery, 30_000)} />}
      />
      <OfflineBanner onRetry={() => void proceduresQuery.refetch()} />
      {error && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-danger-line bg-danger-subtle p-4 text-sm text-danger">
          <span>{error}</span> <button onClick={() => setError(null)} className="rounded px-2 py-1 text-xs underline hover:bg-overlay-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}
      {success && (
        <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-ok-line bg-ok-subtle p-4 text-sm text-ok">
          <span>{success}</span> <button onClick={() => setSuccess(null)} className="rounded px-2 py-1 text-xs underline hover:bg-overlay-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        <Card>
          <CardHeader title={proceduresQuery.isSuccess ? `${procedures.length} procedures` : "Procedures"} icon={Workflow} />
          {proceduresQuery.isLoading ? (
            <AdminLoadingState label="Loading procedures…" />
          ) : proceduresQuery.isError ? (
            <AdminErrorState message={sanitizeError(proceduresQuery.error instanceof Error ? proceduresQuery.error.message : "Failed to load procedures")} retry={() => void proceduresQuery.refetch()} />
          ) : procedures.length === 0 ? (
            <EmptyState icon={Workflow} title="No procedures" sub="Create one with steps (run_command, sleep, run_procedure, etc.)." />
          ) : (
            <>
              <div className="space-y-2 max-h-[520px] overflow-auto">
                {paged.map((p) => (
                  <button
                    key={p.id}
                    onClick={() => setSelected(p)}
                    aria-label={`Select procedure ${p.name}`}
                    className={`w-full text-left rounded-lg border p-3 motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)] ${selected?.id === p.id ? "border-brand bg-brand-subtle" : "border-line bg-surface hover:bg-surface-hover"}`}
                  >
                    <p className="text-sm font-bold text-text">{p.name} {p.enabled ? "" : "(disabled)"}</p>
                    <p className="text-xs text-text-subtle truncate">{p.description || "—"} · {p.steps?.length ?? 0} steps {p.schedule ? `· cron ${p.schedule.cronExpression}` : ""}</p>
                  </button>
                ))}
              </div>
              {procedures.length > pageSize && (
                <div className="mt-3 flex items-center justify-between border-t border-line pt-3">
                  <span className="font-mono text-xs text-text-subtle">Page {page + 1} of {Math.ceil(procedures.length / pageSize)} · {procedures.length} total</span>
                  <div className="flex gap-2">
                    <Btn size="sm" tone="ghost" disabled={page === 0} onClick={() => setPage((prev) => Math.max(0, prev - 1))}>Prev</Btn>
                    <Btn size="sm" tone="ghost" disabled={(page + 1) * pageSize >= procedures.length} onClick={() => setPage((prev) => prev + 1)}>Next</Btn>
                  </div>
                </div>
              )}
            </>
          )}
          <div className="mt-3">
            <Btn size="sm" tone="ghost" onClick={() => void proceduresQuery.refetch()} loading={proceduresQuery.isFetching} ariaLabel="Refresh procedures">Refresh</Btn>
          </div>
        </Card>

        <Card>
          <CardHeader title={selected ? selected.name : "Detail"} icon={History} />
          {!selected ? (
            <p className="text-sm text-text-subtle">Select a procedure to inspect steps and schedule.</p>
          ) : (
            <div className="space-y-3">
              <p className="text-xs text-text-subtle">Enabled: {String(selected.enabled)} · {selected.schedule ? `Schedule ${selected.schedule.cronExpression} (${selected.schedule.timezone})` : "No schedule"}</p>
              <div className="space-y-2">
                {selected.steps.map((s, idx) => (
                  <div key={s.id || idx} className="rounded-lg border border-line bg-surface p-3 text-xs">
                    <p className="font-bold text-text">{idx + 1}. {s.name} <span className="font-normal text-text-subtle">· {s.action}</span></p>
                    <p className="font-mono text-[11px] text-text-subtle break-all">{JSON.stringify(s.config)}</p>
                    <p className="text-text-subtle">retries {s.maxRetries} · timeout {s.timeoutSeconds}s {s.requiresApproval ? "· requires approval" : ""} {s.rollbackEnabled ? "· rollback" : ""} {s.continueOnFailure ? "· continue on failure" : ""}</p>
                  </div>
                ))}
              </div>
              <div className="flex gap-2">
                <Btn size="sm" onClick={handleExecute} loading={executeMut.isPending}><Play size={13} /> Execute</Btn>
                <Btn size="sm" tone="danger" onClick={() => void handleDelete(selected.id, selected.name)} loading={deleteMut.isPending}><Trash2 size={13} /> Delete</Btn>
              </div>
              <div>
                <p className="text-xs font-bold uppercase text-text-subtle">Executions (last 20)</p>
                {executionsQuery.isLoading ? (
                  <div className="mt-2"><AdminLoadingState label="Loading executions…" /></div>
                ) : executionsQuery.isError ? (
                  <div className="mt-2"><AdminErrorState message={sanitizeError(executionsQuery.error instanceof Error ? executionsQuery.error.message : "Failed to load executions")} retry={() => void executionsQuery.refetch()} /></div>
                ) : executions.length === 0 ? (
                  <p className="mt-1 text-xs text-text-subtle">No executions.</p>
                ) : (
                  <div className="mt-2">
                    <AdminTable label="Procedure executions">
                      <AdminTHead>
                        <AdminTh>Execution</AdminTh>
                        <AdminTh>Status</AdminTh>
                        <AdminTh className="text-right">Actions</AdminTh>
                      </AdminTHead>
                      <AdminTBody>
                        {executions.map((e) => (
                          <AdminTr key={e.id}>
                            <AdminTd>
                              <span className="font-mono font-bold">{e.id.slice(0, 8)}</span>
                              <span className="block text-text-subtle">{new Date(e.createdAt).toLocaleString()} · {e.trigger}</span>
                            </AdminTd>
                            <AdminTd><Pill tone={executionTone(e.status)}>{e.status}</Pill></AdminTd>
                            <AdminTd>
                              <div className="flex justify-end gap-1.5">
                                <Btn size="sm" tone="ghost" onClick={() => void handleViewExec(e.id)} loading={detailMut.isPending && detailMut.variables === e.id}>View</Btn>
                                <Btn size="sm" tone="ghost" onClick={() => cancelMut.mutate(e.id)} loading={cancelMut.isPending && cancelMut.variables === e.id}>Cancel</Btn>
                              </div>
                            </AdminTd>
                          </AdminTr>
                        ))}
                      </AdminTBody>
                    </AdminTable>
                  </div>
                )}
              </div>
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="Create Procedure" icon={Plus} />
          <form onSubmit={handleCreate} className="space-y-3">
            <Input label="Name" value={form.name} onChange={(v) => setForm({ ...form, name: v })} placeholder="Name" required />
            <Input label="Description" value={form.description} onChange={(v) => setForm({ ...form, description: v })} placeholder="Description" />
            <Input label="Cron schedule" value={cron} onChange={setCron} placeholder="Cron (e.g. 0 2 * * *) — leave empty for none" mono />
            <p className="text-xs text-text-subtle">Cron is standard 5-field (minute hour dom month dow). Validated at admission (422).</p>
            <div className="space-y-2">
              <p className="text-xs font-bold uppercase text-text-subtle">Steps JSON</p>
              <Textarea
                value={JSON.stringify(form.steps, null, 2)}
                onChange={(v) => {
                  try {
                    const parsed = JSON.parse(v) as typeof form.steps;
                    setForm({ ...form, steps: parsed });
                  } catch {
                    // keep raw invalid for user to fix; don't update
                  }
                }}
                rows={10}
                placeholder="Steps JSON"
              />
              <p className="text-xs text-text-subtle">Actions: run_command (allowlist), sleep, run_procedure, deploy_stack, send_webhook, run_build. Approval gates pause execution until the step is approved.</p>
            </div>
            <label className="flex gap-2 items-center text-xs"><input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> Enabled</label>
            <Btn type="submit" loading={createMut.isPending}>Create</Btn>
          </form>
        </Card>
      </div>

      {selectedExec && (
        <Card>
          <CardHeader title={`Execution ${selectedExec.id.slice(0, 8)}`} icon={History} />
          <p className="mb-3 text-xs text-text-subtle">Status {selectedExec.status} · trigger {selectedExec.trigger} · created {new Date(selectedExec.createdAt).toLocaleString()}</p>
          <div className="space-y-2">
            {selectedExec.steps.map((step) => (
              <div key={step.id} className="rounded-lg border border-line bg-surface p-3">
                <div className="flex items-center justify-between">
                  <p className="text-sm font-bold text-text">#{step.position + 1} {step.id.slice(0, 8)} · {step.status} <span className="text-xs font-normal text-text-subtle">attempt {step.attempt}/{step.maxAttempts}</span></p>
                  <div className="flex gap-1">
                    <Btn size="sm" tone="success" disabled={step.status !== "waiting_approval"} onClick={() => approveMut.mutate(step.id)} loading={approveMut.isPending}>Approve</Btn>
                    <Btn size="sm" tone="danger" disabled={step.status !== "waiting_approval"} onClick={() => rejectMut.mutate(step.id)} loading={rejectMut.isPending}>Reject</Btn>
                  </div>
                </div>
                {step.output && <p className="mt-2 text-xs">Output: {step.output}</p>}
                {step.error && <p className="mt-1 text-xs text-danger">Error: {step.error}</p>}
                <div className="mt-2">
                  <p className="text-xs font-bold uppercase text-text-subtle">Logs</p>
                  <div className="mt-1 max-h-32 overflow-auto rounded border border-line bg-surface-input p-2 font-mono text-[11px]">
                    {(logs[step.id] ?? []).length === 0 ? <span className="text-text-subtle">No logs.</span> : (logs[step.id] ?? []).map((l) => <div key={l.id} className="flex gap-2"><span className="text-text-subtle">{l.level}</span><span>{l.message}</span><span className="ml-auto text-text-subtle">{new Date(l.createdAt).toLocaleTimeString()}</span></div>)}
                  </div>
                  <div className="mt-2">
                    <Btn
                      size="sm"
                      tone="ghost"
                      loading={stepLogsMut.isPending && stepLogsMut.variables === step.id}
                      onClick={() => {
                        void (async () => {
                          try {
                            const fetched = await stepLogsMut.mutateAsync(step.id);
                            setLogs((prev) => ({ ...prev, [step.id]: fetched }));
                          } catch {
                            // stepLogsMut.onError already surfaced the failure.
                          }
                        })();
                      }}
                    >
                      Reload logs
                    </Btn>
                  </div>
                </div>
              </div>
            ))}
            <Btn size="sm" tone="ghost" onClick={() => setSelectedExec(null)}>Close</Btn>
          </div>
        </Card>
      )}
      {renderConfirm()}
    </div>
  );
}
