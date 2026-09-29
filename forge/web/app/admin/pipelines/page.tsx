"use client";

import { useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useRouter, useSearchParams } from "next/navigation";
import {
  Play,
  RotateCcw,
  XCircle,
  FileText,
  Clock3,
  Layers,
  CheckCircle2,
  CircleAlert,
  Download,
  GitBranch,
  ShieldCheck,
  ShieldX,
} from "lucide-react";
import {
  listPipelines,
  listPipelineRuns,
  triggerPipelineRun,
  cancelPipelineRun,
  retryPipelineRun,
  listPipelineRunLogs,
  listPipelineArtifacts,
  pipelineArtifactDownloadUrl,
  approvePipelineStage,
  rejectPipelineStage,
  getPipelineRun,
  type PipelineRun,
  type PipelineStageRun,
} from "@/lib/api/pipelines";
import {
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminToolbar,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Pill,
  StatsRow,
  Modal,
  AdminLoadingState,
  AdminLoadingRows,
  AdminErrorState,
  Input,
} from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { OfflineBanner } from "@/components/shared/states-offline";
import { formatDate, formatBytes, errorMessage } from "@/lib/utils";
import { getApiBaseUrl } from "@/lib/api/http";
import { statusTone } from "@/lib/api/status";
import { useConfirm } from "@/components/ui/confirm-dialog";

/**
 * The run states the pipeline service can put a run in —
 * `internal/services/pipeline/model.go:38-44`:
 * `queued | running | completed | failed | cancelled | awaiting_approval`.
 *
 * Note `awaiting_approval`, spelled in full. This page used to carry its own
 * set `["queued","running","await_approval","in_progress"]`: `await_approval` is
 * not a state the service emits, so approval-gated runs were never counted as
 * active and never offered a cancel, and `in_progress` matched nothing either.
 * The old set also had no `completed`, which is the success state the service
 * actually writes (`succeeded` is a *stage* status, not a run status).
 */
const RUN_STATUSES = ["queued", "running", "awaiting_approval", "completed", "failed", "cancelled"];

/** Cancelling is accepted by the service in these three states only (`service.go:317-330`). */
const CANCELLABLE = new Set(["queued", "running", "awaiting_approval"]);
/** Retry is rejected everywhere else (`service.go:252-254`). */
const RETRYABLE = new Set(["failed", "cancelled"]);
/** A run in these states may still move, so its logs are worth polling. */
const IN_FLIGHT = new Set(["queued", "running", "awaiting_approval"]);

function runTone(status: string) {
  return statusTone(status, "deployment");
}

function humanToken(value: string): string {
  return value.replace(/_/g, " ");
}

function timeOrDash(value?: string | null): string {
  return value ? formatDate(value) : "—";
}

export default function AdminPipelinesPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const router = useRouter();
  const [confirm, renderConfirm] = useConfirm();
  const searchParams = useSearchParams();

  const selectedPipeline = searchParams.get("pipeline") ?? "";
  const statusFilter = searchParams.get("status") ?? "";
  const search = searchParams.get("q") ?? "";
  const openRunId = searchParams.get("run") ?? "";

  const clearFilters = () => {
    // One replace: three sequential `setParam` calls would each start from the
    // same stale `searchParams` and only the last change would survive.
    const params = new URLSearchParams(searchParams.toString());
    params.delete("q");
    params.delete("status");
    params.delete("pipeline");
    const query = params.toString();
    router.replace(query ? `/admin/pipelines?${query}` : "/admin/pipelines", { scroll: false });
  };

  const setParam = (key: string, value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (value) params.set(key, value);
    else params.delete(key);
    const query = params.toString();
    router.replace(query ? `/admin/pipelines?${query}` : "/admin/pipelines", { scroll: false });
  };

  const defsQuery = useQuery({
    queryKey: ["pipelines-defs"],
    queryFn: () => listPipelines(),
  });

  const runsQuery = useQuery({
    queryKey: ["pipeline-runs", selectedPipeline || "all", statusFilter || "any"],
    queryFn: () =>
      listPipelineRuns({
        ...(selectedPipeline ? { pipelineId: selectedPipeline } : {}),
        ...(statusFilter ? { status: statusFilter } : {}),
      }),
    refetchInterval: 10_000,
  });

  const triggerMutation = useMutation({
    mutationFn: (pipelineId: string) => triggerPipelineRun(pipelineId, "manual"),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["pipeline-runs"] });
      toast({ tone: "success", title: "Run queued", message: "The pipeline was queued for execution." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to trigger pipeline", message: errorMessage(err) }),
  });

  const cancelMutation = useMutation({
    mutationFn: async (runId: string) => {
      const result = await cancelPipelineRun(runId);
      if (!result.ok) throw new Error("The server reported the pipeline run was not cancelled.");
      return result;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pipeline-runs"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to cancel run", message: errorMessage(err) }),
  });

  const retryMutation = useMutation({
    mutationFn: (runId: string) => retryPipelineRun(runId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["pipeline-runs"] });
      toast({ tone: "success", title: "Retry queued", message: "A new run was queued as a retry of this one." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to retry run", message: errorMessage(err) }),
  });

  const openRunQuery = useQuery({
    queryKey: ["pipeline-run", openRunId],
    queryFn: () => getPipelineRun(openRunId),
    enabled: Boolean(openRunId),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && IN_FLIGHT.has(status) ? 5_000 : false;
    },
  });

  const openRun = openRunId ? (openRunQuery.data ?? null) : null;

  const logsQuery = useQuery({
    queryKey: ["pipeline-run-logs", openRunId],
    queryFn: () => listPipelineRunLogs(openRunId),
    enabled: Boolean(openRunId),
    refetchInterval: openRun && IN_FLIGHT.has(openRun.status) ? 4_000 : false,
  });

  const artifactsQuery = useQuery({
    queryKey: ["pipeline-run-artifacts", openRunId],
    queryFn: () => listPipelineArtifacts(openRunId),
    enabled: Boolean(openRunId),
  });

  function decide(runId: string, stage: PipelineStageRun, decision: "approve" | "reject") {
    const mutation = decision === "approve" ? approveMutation : rejectMutation;
    void (async () => {
      const ok = await confirm({
        title: decision === "approve" ? `Approve “${stage.name}”?` : `Reject “${stage.name}”?`,
        description:
          decision === "approve"
            ? "The run resumes and executes this approval gate, then continues to the remaining stages."
            : "Rejecting fails this stage and ends the whole run. Later stages do not execute.",
        danger: decision === "reject",
        confirmLabel: decision === "approve" ? "Approve stage" : "Reject stage",
      });
      if (ok) mutation.mutate({ runId, stageId: stage.id });
    })();
  }

  const approveMutation = useMutation({
    mutationFn: ({ runId, stageId }: { runId: string; stageId: string }) => approvePipelineStage(runId, stageId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["pipeline-run", openRunId] });
      qc.invalidateQueries({ queryKey: ["pipeline-runs"] });
      toast({ tone: "success", title: "Stage approved", message: "The run has resumed." });
    },
    onError: (err) => toast({ tone: "error", title: "Approval failed", message: errorMessage(err) }),
  });

  const rejectMutation = useMutation({
    mutationFn: ({ runId, stageId }: { runId: string; stageId: string }) => rejectPipelineStage(runId, stageId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["pipeline-run", openRunId] });
      qc.invalidateQueries({ queryKey: ["pipeline-runs"] });
      toast({ tone: "warning", title: "Stage rejected", message: "The run failed at the approval gate." });
    },
    onError: (err) => toast({ tone: "error", title: "Rejection failed", message: errorMessage(err) }),
  });

  const defs = useMemo(() => defsQuery.data ?? [], [defsQuery.data]);
  const runs = useMemo(() => runsQuery.data ?? [], [runsQuery.data]);

  const visibleRuns = useMemo(() => {
    const needle = search.toLowerCase();
    if (!needle) return runs;
    return runs.filter((r) =>
      `${r.pipelineName ?? ""} ${r.pipelineId} ${r.id}`.toLowerCase().includes(needle),
    );
  }, [runs, search]);

  const stats = useMemo(() => {
    return {
      succeeded: runs.filter((r) => r.status === "completed").length,
      failed: runs.filter((r) => r.status === "failed").length,
      active: runs.filter((r) => IN_FLIGHT.has(r.status)).length,
      awaiting: runs.filter((r) => r.status === "awaiting_approval").length,
    };
  }, [runs]);

  const scopeSuffix = selectedPipeline ? " (this pipeline)" : "";
  const runsKnown = runsQuery.data !== undefined;
  const defsKnown = defsQuery.data !== undefined;

  const logs = logsQuery.data ?? [];
  const artifacts = artifactsQuery.data ?? [];
  const stages = useMemo(
    () =>
      [...(openRun?.stages ?? [])].sort((a, b) => (a.position ?? 0) - (b.position ?? 0)),
    [openRun?.stages],
  );
  const awaitingStage = stages.find((s) => s.status === "awaiting_approval");

  return (
    <AdminPageLayout>
      <AdminPageHeader
        status={<FreshnessBadge state={sourceState(runsQuery, 10_000)} />}
      />
      <OfflineBanner onRetry={() => void runsQuery.refetch()} />

      <StatsRow
        items={[
          { label: "Definitions", value: defsKnown ? defs.length : "—", icon: Layers },
          { label: `Runs${scopeSuffix}`, value: runsKnown ? runs.length : "—", icon: Clock3 },
          { label: `Active${scopeSuffix}`, value: runsKnown ? stats.active : "—", icon: Play, tone: stats.active ? "pending" : "neutral" },
          { label: `Awaiting approval${scopeSuffix}`, value: runsKnown ? stats.awaiting : "—", icon: ShieldCheck, tone: stats.awaiting ? "warn" : "neutral" },
          { label: `Succeeded${scopeSuffix}`, value: runsKnown ? stats.succeeded : "—", icon: CheckCircle2, tone: stats.succeeded ? "ok" : "neutral" },
          { label: `Failed${scopeSuffix}`, value: runsKnown ? stats.failed : "—", icon: CircleAlert, tone: stats.failed ? "danger" : "neutral" },
        ]}
      />

      <AdminToolbar>
        <Input label="Search runs" placeholder="Pipeline name or run id" value={search} onChange={(v) => setParam("q", v)} />
        <AdminSelect
          label="Status"
          value={statusFilter}
          onChange={(v) => setParam("status", v)}
          placeholder="All statuses"
          options={RUN_STATUSES.map((s) => ({ value: s, label: humanToken(s) }))}
        />
        {selectedPipeline ? (
          <AdminSelect
            label="Pipeline"
            value={selectedPipeline}
            onChange={(v) => setParam("pipeline", v)}
            options={defs.map((d) => ({ value: d.id, label: d.name }))}
          />
        ) : null}
        {search || statusFilter || selectedPipeline ? (
          <Btn tone="ghost" size="sm" onClick={clearFilters}>
            Clear filters
          </Btn>
        ) : null}
      </AdminToolbar>

      {selectedPipeline && stats.awaiting > 0 ? (
        <p className="ui-alert ui-alert-warning" role="status">
          {stats.awaiting.toLocaleString()} run{stats.awaiting === 1 ? "" : "s"} here is waiting at an approval
          gate — open the run to approve or reject the stage.
        </p>
      ) : null}

      <AdminSection
        title="Definitions"
        description="Each definition is the ordered set of stages a run executes."
        action={
          <div className="flex items-center gap-2">
            {defsQuery.isFetching ? <span className="text-meta text-text-subtle">Refreshing…</span> : null}
            {selectedPipeline ? (
              <Btn size="sm" tone="ghost" onClick={() => setParam("pipeline", "")}>
                Show all pipelines&apos;s runs
              </Btn>
            ) : null}
          </div>
        }
      >
        <Card>
          {defsQuery.isPending ? (
            <AdminLoadingRows cols={3} rows={3} label="Loading pipelines…" />
          ) : defsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Pipeline definitions could not be loaded: ${errorMessage(defsQuery.error)}`}
                retry={() => void defsQuery.refetch()}
              />
            </div>
          ) : defs.length === 0 ? (
            <EmptyState
              icon={Layers}
              title="No pipeline definitions"
              message="Pipelines build and release workloads from Git, compose or scripted stages."
            />
          ) : (
            <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
              {defs.map((d) => {
                const active = selectedPipeline === d.id;
                const rowBusy = triggerMutation.isPending && triggerMutation.variables === d.id;
                return (
                  // A card that is *not* a button: it used to nest a Trigger button
                  // inside a card-sized <button>, which is invalid HTML and made the
                  // card's whole text run the button's accessible name.
                  <div
                    className={`flex flex-col gap-3 rounded-xl border p-4 ${
                      active ? "border-[var(--focus)] bg-overlay-strong" : "border-line bg-overlay-subtle"
                    }`}
                    key={d.id}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <h3 className="flex items-center gap-2 text-sm font-semibold text-text">
                          <GitBranch aria-hidden="true" className="shrink-0 text-text-muted" size={14} />
                          <span className="truncate">{d.name}</span>
                        </h3>
                        {d.description ? (
                          <p className="mt-1 text-meta text-text-subtle">{d.description}</p>
                        ) : (
                          <p className="mt-1 text-meta text-text-muted">No description recorded.</p>
                        )}
                      </div>
                      <Pill tone="neutral">
                        {d.trigger?.type ? humanToken(d.trigger.type) : "manual"}
                      </Pill>
                    </div>
                    <div className="flex items-center justify-between text-meta text-text-subtle">
                      <span>
                        {Array.isArray(d.stages) ? d.stages.length.toLocaleString() : "no stage count reported"}{" "}
                        stage{Array.isArray(d.stages) && d.stages.length === 1 ? "" : "s"}
                      </span>
                      {d.trigger?.type === "schedule" && d.trigger.cron ? (
                        <span className="font-mono">{d.trigger.cron}</span>
                      ) : null}
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <Btn
                        size="sm"
                        tone="primary"
                        onClick={() => triggerMutation.mutate(d.id)}
                        disabled={rowBusy}
                        loading={rowBusy}
                      >
                        <Play aria-hidden="true" size={12} /> Trigger
                      </Btn>
                      <Btn size="sm" tone="ghost" onClick={() => setParam("pipeline", active ? "" : d.id)}>
                        {active ? "Showing its runs" : "Filter runs to this pipeline"}
                      </Btn>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </Card>
      </AdminSection>

      <AdminSection
        title="Runs"
        description={
          selectedPipeline
            ? "Runs for the selected pipeline, as the control plane returns them."
            : "Runs across all pipelines, newest first as the control plane returns them."
        }
        action={
          runsQuery.isFetching ? <span className="text-meta text-text-subtle">Refreshing…</span> : null
        }
      >
        <Card>
          {runsQuery.isPending ? (
            <AdminLoadingRows cols={5} rows={4} label="Loading runs…" />
          ) : runsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Pipeline runs could not be loaded: ${errorMessage(runsQuery.error)}`}
                retry={() => void runsQuery.refetch()}
              />
            </div>
          ) : runs.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={FileText}
                title="No runs"
                message={
                  selectedPipeline || statusFilter
                    ? "No run matches the selected pipeline or status. Clear the filters above to see every run."
                    : "No pipeline has run yet. Trigger one above, or wait for a schedule or webhook to start one."
                }
              />
            </div>
          ) : visibleRuns.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={FileText}
                title="No run matches that search"
                message="No run in the current result set matches the search text."
              />
            </div>
          ) : (
            <AdminTable label="Pipeline runs">
              <AdminTHead>
                <AdminTh>Run</AdminTh>
                <AdminTh>Pipeline</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Progress</AdminTh>
                <AdminTh>Current stage</AdminTh>
                <AdminTh>Created</AdminTh>
                <AdminTh>Started</AdminTh>
                <AdminTh>Finished</AdminTh>
                <AdminTh>Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {visibleRuns.map((r) => {
                  const rowBusy =
                    (cancelMutation.isPending && cancelMutation.variables === r.id) ||
                    (retryMutation.isPending && retryMutation.variables === r.id);
                  const canCancel = CANCELLABLE.has(r.status);
                  const canRetry = RETRYABLE.has(r.status);
                  return (
                    <AdminTr key={r.id}>
                      <AdminTd className="font-mono text-meta">
                        <Btn size="sm" tone="ghost" onClick={() => setParam("run", r.id)}>
                          {r.id.slice(0, 8)}
                          <span className="sr-only">open run details</span>
                        </Btn>
                      </AdminTd>
                      <AdminTd className="text-meta">
                        {r.pipelineName ?? <span className="font-mono">{r.pipelineId.slice(0, 8)}</span>}
                      </AdminTd>
                      <AdminTd>
                        <Pill tone={runTone(r.status)}>{humanToken(r.status) || "unknown"}</Pill>
                      </AdminTd>
                      <AdminTd className="max-w-40 text-meta">
                        {typeof r.progressPct === "number" && Number.isFinite(r.progressPct)
                          ? `${r.progressPct}%`
                          : "Not reported"}
                      </AdminTd>
                      <AdminTd className="text-meta">
                        {r.currentStage ? humanToken(r.currentStage) : "Not reported"}
                      </AdminTd>
                      <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(r.createdAt)}</AdminTd>
                      <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(r.startedAt)}</AdminTd>
                      <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(r.finishedAt)}</AdminTd>
                      <AdminTd>
                        <div className="flex flex-wrap items-center gap-1">
                          <Btn size="sm" tone="ghost" onClick={() => setParam("run", r.id)}>
                            <FileText aria-hidden="true" size={12} /> Logs
                          </Btn>
                          {r.status === "awaiting_approval" ? (
                            <Btn size="sm" tone="primary" onClick={() => setParam("run", r.id)} title="Open the run to approve or reject its gated stage">
                              <ShieldCheck aria-hidden="true" size={12} /> Approve
                            </Btn>
                          ) : null}
                          <Btn
                            size="sm"
                            tone="danger"
                            onClick={() => void askCancel(r)}
                            disabled={!canCancel || rowBusy}
                            title={
                              canCancel
                                ? undefined
                                : `The service only cancels queued, running or approval-gated runs; this one is “${humanToken(r.status)}”.`
                            }
                          >
                            <XCircle aria-hidden="true" size={12} /> Cancel
                          </Btn>
                          <Btn
                            size="sm"
                            tone="ghost"
                            onClick={() => void askRetry(r)}
                            disabled={!canRetry || rowBusy}
                            title={
                              canRetry
                                ? undefined
                                : `Only failed or cancelled runs can be retried; this one is “${humanToken(r.status)}”.`
                            }
                          >
                            <RotateCcw aria-hidden="true" size={12} /> Retry
                          </Btn>
                        </div>
                      </AdminTd>
                    </AdminTr>
                  );
                })}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      </AdminSection>

      {openRunId ? (
        <Modal
          // `DashHeader` used to sit inside this dialog, repeating the run id as a
          // second heading, the status as a second pill, the pipeline name a second
          // time and the artifact count a second time — in a red 10px eyebrow.
          title={<span className="font-mono">Run {openRunId}</span>}
          description={openRun ? `${openRun.pipelineName ?? openRun.pipelineId} · ${humanToken(openRun.status)}` : "Loading this run…"}
          onClose={() => setParam("run", "")}
          wide
        >
          {openRunQuery.isPending ? (
            <AdminLoadingState label="Loading run…" />
          ) : openRunQuery.isError ? (
            <AdminErrorState
              message={`This run could not be read: ${errorMessage(openRunQuery.error)}`}
              retry={() => void openRunQuery.refetch()}
            />
          ) : openRun ? (
            <div className="space-y-4">
              <dl className="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <dt className="text-meta uppercase tracking-wider text-text-muted">Status</dt>
                  <dd>
                    <Pill tone={runTone(openRun.status)}>{humanToken(openRun.status) || "unknown"}</Pill>
                  </dd>
                </div>
                <div>
                  <dt className="text-meta uppercase tracking-wider text-text-muted">Trigger</dt>
                  <dd className="text-text">{humanToken(openRun.trigger) || "Not reported"}</dd>
                </div>
                <div>
                  <dt className="text-meta uppercase tracking-wider text-text-muted">Started</dt>
                  <dd className="text-text">{timeOrDash(openRun.startedAt)}</dd>
                </div>
                <div>
                  <dt className="text-meta uppercase tracking-wider text-text-muted">Finished</dt>
                  <dd className="text-text">{timeOrDash(openRun.finishedAt)}</dd>
                </div>
                {typeof openRun.retryCount === "number" && openRun.retryCount > 0 ? (
                  <div>
                    <dt className="text-meta uppercase tracking-wider text-text-muted">Retry</dt>
                    <dd className="text-text">attempt {openRun.retryCount + 1}</dd>
                  </div>
                ) : null}
              </dl>

              {openRun.error ? (
                <p className="ui-alert ui-alert-danger" role="alert">
                  <span className="font-semibold">Run failure:</span> {openRun.error}
                </p>
              ) : null}

              {awaitingStage ? (
                <div className="space-y-2">
                  <p className="ui-alert ui-alert-warning" role="status">
                    Stage “{awaitingStage.name}” is waiting for approval. Until it is decided the run does not
                    continue.
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Btn
                      size="sm"
                      tone="primary"
                      onClick={() => decide(openRun.id, awaitingStage, "approve")}
                      disabled={approveMutation.isPending || rejectMutation.isPending}
                    >
                      <ShieldCheck aria-hidden="true" size={12} /> Approve stage
                    </Btn>
                    <Btn
                      size="sm"
                      tone="danger"
                      onClick={() => decide(openRun.id, awaitingStage, "reject")}
                      disabled={approveMutation.isPending || rejectMutation.isPending}
                    >
                      <ShieldX aria-hidden="true" size={12} /> Reject stage
                    </Btn>
                  </div>
                </div>
              ) : null}

              <section aria-labelledby="run-stages" className="space-y-2">
                <h3 className="text-sm font-semibold text-text" id="run-stages">
                  Stages
                </h3>
                {stages.length === 0 ? (
                  <p className="text-meta text-text-subtle">
                    The run reported no stages, so there is nothing to list — this is not a load failure.
                  </p>
                ) : (
                  <AdminTable label="Run stages">
                    <AdminTHead>
                      <AdminTh>#</AdminTh>
                      <AdminTh>Stage</AdminTh>
                      <AdminTh>Status</AdminTh>
                      <AdminTh>Attempts</AdminTh>
                      <AdminTh>Started</AdminTh>
                      <AdminTh>Finished</AdminTh>
                    </AdminTHead>
                    <AdminTBody>
                      {stages.map((s) => (
                        <AdminTr key={s.id}>
                          <AdminTd className="font-mono text-meta">{s.position}</AdminTd>
                          <AdminTd className="text-sm">
                            {s.name}
                            <span className="mt-0.5 block font-mono text-meta text-text-muted">{s.action}</span>
                          </AdminTd>
                          <AdminTd>
                            <Pill tone={statusTone(s.status, "deployment")}>
                              {humanToken(s.status) || "unknown"}
                            </Pill>
                          </AdminTd>
                          <AdminTd className="font-mono text-meta">
                            {typeof s.attempts === "number" ? s.attempts : "—"}
                          </AdminTd>
                          <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(s.startedAt)}</AdminTd>
                          <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(s.finishedAt)}</AdminTd>
                        </AdminTr>
                      ))}
                    </AdminTBody>
                  </AdminTable>
                )}
              </section>

              <section aria-labelledby="run-artifacts" className="space-y-2">
                <h3 className="text-sm font-semibold text-text" id="run-artifacts">
                  Artifacts
                </h3>
                {artifactsQuery.isError ? (
                  <AdminErrorState
                    message={`Artifacts could not be listed: ${errorMessage(artifactsQuery.error)}`}
                    retry={() => void artifactsQuery.refetch()}
                  />
                ) : artifactsQuery.isPending ? (
                  <p className="text-meta text-text-subtle" role="status">Loading artifacts…</p>
                ) : artifacts.length === 0 ? (
                  <p className="text-meta text-text-subtle">This run produced no artifacts.</p>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {artifacts.map((a) => (
                      <a
                        className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-overlay-subtle px-2.5 py-1.5 text-meta text-text transition-colors hover:border-line-strong hover:bg-overlay-strong"
                        href={`${getApiBaseUrl()}${pipelineArtifactDownloadUrl(a.id)}`}
                        key={a.id}
                      >
                        <Download aria-hidden="true" size={12} /> {a.name}
                        <span className="text-text-muted">
                          {typeof a.sizeBytes === "number" ? formatBytes(a.sizeBytes) : "size not reported"}
                        </span>
                      </a>
                    ))}
                  </div>
                )}
              </section>

              <section aria-labelledby="run-logs" className="space-y-2">
                <h3 className="text-sm font-semibold text-text" id="run-logs">
                  Logs
                </h3>
                {logsQuery.isError ? (
                  <AdminErrorState
                    message={`Logs could not be read: ${errorMessage(logsQuery.error)}`}
                    retry={() => void logsQuery.refetch()}
                  />
                ) : logsQuery.isPending ? (
                  <AdminLoadingState label="Loading logs…" />
                ) : logs.length === 0 ? (
                  <p className="text-meta text-text-subtle">
                    The log request completed and returned no lines for this run.
                  </p>
                ) : (
                  <div className="max-h-125 space-y-3 overflow-auto rounded-lg border border-line bg-overlay-subtle p-3">
                    {stages.map((s) => {
                      const stageLogs = logs.filter((l) => l.stageId === s.id);
                      if (stageLogs.length === 0) return null;
                      return (
                        <div key={s.id}>
                          <h4 className="mb-1 font-mono text-meta text-text-muted">
                            {`${s.position}. ${s.name}`} · {humanToken(s.status)}
                          </h4>
                          <LogLines lines={stageLogs} />
                        </div>
                      );
                    })}
                    {(() => {
                      const loose = logs.filter((l) => !l.stageId || !stages.some((s) => s.id === l.stageId));
                      if (loose.length === 0) return null;
                      return (
                        <div>
                          <h4 className="mb-1 font-mono text-meta text-text-muted">Run-level output</h4>
                          <LogLines lines={loose} />
                        </div>
                      );
                    })()}
                  </div>
                )}
              </section>
            </div>
          ) : (
            <AdminErrorState
              message="The run is not open. Close this dialog and select the run id from the list."
              retry={() => void openRunQuery.refetch()}
            />
          )}
        </Modal>
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );

  async function askCancel(run: PipelineRun) {
    const ok = await confirm({
      title: "Cancel this run?",
      description: `Run ${run.id} of “${run.pipelineName ?? run.pipelineId}” will stop at the next stage boundary. Stages already finished are not undone.`,
      danger: true,
      confirmLabel: "Cancel run",
    });
    if (ok) cancelMutation.mutate(run.id);
  }

  async function askRetry(run: PipelineRun) {
    const ok = await confirm({
      title: "Retry this run?",
      description: `A new run of “${run.pipelineName ?? run.pipelineId}” is queued as a retry of ${run.id}. This run stays as it is, for the record.`,
      confirmLabel: "Retry",
    });
    if (ok) retryMutation.mutate(run.id);
  }
}

function LogLines({ lines }: { lines: Array<{ id: number; level: string; message: string; timestamp?: string }> }) {
  return (
    <div className="space-y-0.5 font-mono text-meta leading-5">
      {lines.map((line) => (
        <div className="flex gap-2" key={line.id}>
          <span
            className={`shrink-0 uppercase ${
              line.level === "error"
                ? "text-danger"
                : line.level === "warn"
                  ? "text-warn"
                  : "text-text-muted"
            }`}
          >
            {line.level || "log"}
          </span>
          {line.timestamp ? (
            <span className="shrink-0 text-text-muted">{formatDate(line.timestamp)}</span>
          ) : null}
          <span className={`whitespace-pre-wrap break-words ${line.level === "error" ? "text-danger" : "text-text"}`}>
            {line.message}
          </span>
        </div>
      ))}
    </div>
  );
}
