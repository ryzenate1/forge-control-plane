"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import {
  Play,
  RotateCcw,
  XCircle,
  FileText,
  Clock3,
  Layers,
  Workflow,
  CheckCircle2,
  CircleAlert,
  Download,
  GitBranch,
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
  type PipelineRun,
} from "@/lib/api/pipelines";
import {
  AdminPageHeader,
  AdminPageLayout,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  StatsRow,
  Modal,
  AdminLoadingState,
  AdminErrorState,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { formatDate, cn } from "@/lib/utils";
import { API_BASE_URL } from "@/lib/api/http";
import { statusTone, type StatusTone } from "@/lib/api/status";

const TONE_PILL: Record<StatusTone, "green" | "red" | "yellow" | "blue" | "neutral"> = {
  green: "green",
  red: "red",
  yellow: "yellow",
  blue: "blue",
  neutral: "neutral",
};

const ACTIVE = new Set(["queued", "running", "await_approval", "in_progress"]);

function runTone(status: string): "green" | "red" | "yellow" | "blue" | "neutral" {
  return TONE_PILL[statusTone(status, "deployment")] ?? "neutral";
}

function ProgressBar({ pct, tone }: { pct: number; tone: string }) {
  const clamped = Math.max(0, Math.min(100, pct || 0));
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-white/[0.06]" role="progressbar" aria-valuenow={clamped} aria-valuemin={0} aria-valuemax={100}>
      <div
        className={cn(
          "h-full rounded-full transition-all",
          tone === "green" && "bg-emerald-400/70",
          tone === "red" && "bg-red-400/70",
          tone === "yellow" && "bg-amber-400/70",
          tone === "blue" && "bg-blue-400/70",
          tone === "neutral" && "bg-slate-400/60",
        )}
        style={{ width: `${clamped}%` }}
      />
    </div>
  );
}

export default function AdminPipelinesPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [selectedPipeline, setSelectedPipeline] = useState<string>("");
  const [logRun, setLogRun] = useState<PipelineRun | null>(null);

  const defsQuery = useQuery({
    queryKey: ["pipelines-defs"],
    queryFn: () => listPipelines(),
  });

  const runsQuery = useQuery({
    queryKey: ["pipeline-runs", selectedPipeline || "all"],
    queryFn: () => listPipelineRuns(selectedPipeline ? { pipelineId: selectedPipeline } : undefined),
    refetchInterval: 10_000,
  });

  const triggerMutation = useMutation({
    mutationFn: (pipelineId: string) => triggerPipelineRun(pipelineId, "manual"),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pipeline-runs"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to trigger pipeline", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const cancelMutation = useMutation({
    mutationFn: async (runId: string) => {
      const result = await cancelPipelineRun(runId);
      if (!result.ok) throw new Error("The server reported the pipeline run was not cancelled.");
      return result;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pipeline-runs"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to cancel run", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const retryMutation = useMutation({
    mutationFn: (runId: string) => retryPipelineRun(runId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pipeline-runs"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to retry run", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const logsQuery = useQuery({
    queryKey: ["pipeline-run-logs", logRun?.id],
    queryFn: () => listPipelineRunLogs(logRun!.id),
    enabled: Boolean(logRun),
    refetchInterval: logRun && ACTIVE.has(logRun.status) ? 4_000 : false,
  });
  const artifactsQuery = useQuery({
    queryKey: ["pipeline-run-artifacts", logRun?.id],
    queryFn: () => listPipelineArtifacts(logRun!.id),
    enabled: Boolean(logRun),
  });

  const defs = useMemo(() => defsQuery.data ?? [], [defsQuery.data]);
  const runs = useMemo(() => runsQuery.data ?? [], [runsQuery.data]);

  const stats = useMemo(() => {
    const succeeded = runs.filter((r) => r.status === "succeeded" || r.status === "completed").length;
    const failed = runs.filter((r) => r.status === "failed").length;
    const active = runs.filter((r) => ACTIVE.has(r.status)).length;
    return { succeeded, failed, active };
  }, [runs]);

  const logs = logsQuery.data ?? [];
  const artifacts = artifactsQuery.data ?? [];

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminPageHeader
        title="Pipelines"
        description="CI/CD delivery workflows — definitions trigger runs (manual, schedule or webhook) that execute ordered stages with live logs, approvals and retries."
      />

      <StatsRow
        items={[
          { label: "Definitions", value: defs.length, icon: Layers },
          { label: "Runs", value: runs.length, icon: Clock3 },
          { label: "Active", value: stats.active, icon: Workflow, tone: stats.active ? "yellow" : "neutral" },
          { label: "Succeeded", value: stats.succeeded, icon: CheckCircle2, tone: "green" },
          { label: "Failed", value: stats.failed, icon: CircleAlert, tone: stats.failed ? "red" : "neutral" },
        ]}
      />

      <Card>
        <CardHeader
          title="Definitions"
          icon={Layers}
          action={
            selectedPipeline ? (
              <Btn size="sm" tone="ghost" onClick={() => setSelectedPipeline("")}>
                Clear filter
              </Btn>
            ) : undefined
          }
        />
        {defsQuery.isLoading ? (
          <AdminLoadingState label="Loading pipelines…" />
        ) : defsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={defsQuery.error instanceof Error ? defsQuery.error.message : "Failed to load pipelines"}
              retry={() => void defsQuery.refetch()}
            />
          </div>
        ) : defs.length === 0 ? (
          <EmptyState icon={Layers} title="No pipeline definitions" message="Pipelines build and release workloads from Git, compose or scripted stages." />
        ) : (
          <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
            {defs.map((d) => {
              const active = selectedPipeline === d.id;
              return (
                <button
                  type="button"
                  key={d.id}
                  onClick={() => setSelectedPipeline(active ? "" : d.id)}
                  className={cn(
                    "group flex flex-col gap-3 rounded-xl border p-4 text-left transition",
                    active
                      ? "border-blue-500/40 bg-blue-500/[0.06] shadow-[0_0_0_1px_rgba(59,130,246,0.25)]"
                      : "border-white/[0.07] bg-white/[0.015] hover:border-white/[0.14] hover:bg-white/[0.03]",
                  )}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 text-sm font-semibold text-slate-100">
                        <GitBranch size={14} className="shrink-0 text-slate-500" />
                        <span className="truncate">{d.name}</span>
                      </div>
                      {d.description ? (
                        <p className="mt-1 line-clamp-2 text-xs leading-5 text-slate-500">{d.description}</p>
                      ) : null}
                    </div>
                    <Pill tone={d.trigger?.enabled ? "green" : "neutral"}>{d.trigger?.type ?? "manual"}</Pill>
                  </div>
                  <div className="flex items-center justify-between text-[11px] text-slate-500">
                    <span>{d.stages?.length ?? 0} stages</span>
                    {d.trigger?.type === "schedule" && d.trigger.cron ? (
                      <span className="font-mono">{d.trigger.cron}</span>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                    <Btn size="sm" tone="primary" onClick={() => triggerMutation.mutate(d.id)} disabled={triggerMutation.isPending}>
                      <Play size={12} /> Trigger
                    </Btn>
                    <span className={cn("text-[11px]", active ? "text-blue-300" : "text-slate-600 group-hover:text-slate-400")}>
                      {active ? "showing its runs" : "click to filter runs"}
                    </span>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title={`Runs${selectedPipeline ? " · filtered" : ""}`} icon={Clock3} />
        {runsQuery.isLoading ? (
          <AdminLoadingState label="Loading runs…" />
        ) : runsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={runsQuery.error instanceof Error ? runsQuery.error.message : "Failed to load runs"}
              retry={() => void runsQuery.refetch()}
            />
          </div>
        ) : runs.length === 0 ? (
          <EmptyState icon={FileText} title="No runs yet" message="Trigger a pipeline above, or wait for a schedule or webhook to start one." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Run</th>
                  <th className="px-4 py-3">Pipeline</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3 w-40">Progress</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {runs.map((r) => {
                  const tone = runTone(r.status);
                  return (
                    <tr key={r.id} className="transition hover:bg-white/[0.02]">
                      <td className="px-4 py-3">
                        <button type="button" onClick={() => setLogRun(r)} className="font-mono text-[11px] text-slate-300 hover:text-blue-300">
                          {r.id.slice(0, 8)}…
                        </button>
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-300">{r.pipelineName ?? <span className="font-mono">{r.pipelineId.slice(0, 8)}</span>}</td>
                      <td className="px-4 py-3">
                        <Pill tone={tone}>{r.status.replace(/_/g, " ")}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-col gap-1">
                          <ProgressBar pct={r.progressPct ?? 0} tone={tone} />
                          <span className="text-[10px] text-slate-500">
                            {r.currentStage ? r.currentStage.replace(/_/g, " ") : ""}
                            {typeof r.progressPct === "number" ? ` · ${r.progressPct}%` : ""}
                          </span>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-500">{formatDate(r.createdAt)}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          <Btn size="sm" tone="ghost" onClick={() => setLogRun(r)}>
                            <FileText size={12} /> Logs
                          </Btn>
                          {ACTIVE.has(r.status) && (
                            <Btn size="sm" tone="danger" onClick={() => cancelMutation.mutate(r.id)} disabled={cancelMutation.isPending}>
                              <XCircle size={12} /> Cancel
                            </Btn>
                          )}
                          {(r.status === "failed" || r.status === "cancelled") && (
                            <Btn size="sm" tone="ghost" onClick={() => retryMutation.mutate(r.id)} disabled={retryMutation.isPending}>
                              <RotateCcw size={12} /> Retry
                            </Btn>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {logRun && (
        <Modal title={`Run ${logRun.id.slice(0, 8)}`} onClose={() => setLogRun(null)} wide description={logRun.pipelineName ? `${logRun.pipelineName} · ${logRun.status}` : logRun.status}>
          <div className="flex flex-col gap-4">
            {artifacts.length > 0 && (
              <div>
                <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-widest text-slate-500">Artifacts</h4>
                <div className="flex flex-wrap gap-2">
                  {artifacts.map((a) => (
                    <a
                      key={a.id}
                      href={`${API_BASE_URL}${pipelineArtifactDownloadUrl(a.id)}`}
                      className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/[0.03] px-2.5 py-1.5 text-xs text-slate-300 transition hover:border-white/20 hover:bg-white/[0.06]"
                    >
                      <Download size={12} /> {a.name}
                      <span className="text-slate-600">{(a.sizeBytes / 1024).toFixed(0)} KB</span>
                    </a>
                  ))}
                </div>
              </div>
            )}
            <div>
              <h4 className="mb-2 text-[11px] font-semibold uppercase tracking-widest text-slate-500">Logs</h4>
              {logsQuery.isLoading ? (
                <AdminLoadingState label="Loading logs…" />
              ) : logs.length === 0 ? (
                <div className="rounded-lg border border-dashed border-white/10 p-4 text-center text-xs text-slate-500">No log output yet.</div>
              ) : (
                <div className="max-h-[50vh] overflow-auto rounded-lg border border-white/[0.06] bg-black/40 p-3 font-mono text-[11px] leading-5">
                  {logs.map((line) => (
                    <div key={line.id} className="flex gap-2">
                      <span
                        className={cn(
                          "shrink-0 uppercase",
                          line.level === "error" ? "text-red-400" : line.level === "warn" ? "text-amber-400" : "text-slate-600",
                        )}
                      >
                        {line.level}
                      </span>
                      <span className={cn("whitespace-pre-wrap break-words", line.level === "error" ? "text-red-200" : "text-slate-300")}>{line.message}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </Modal>
      )}
    </AdminPageLayout>
  );
}
