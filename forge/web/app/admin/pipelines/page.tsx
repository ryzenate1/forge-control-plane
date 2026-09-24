"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Play, RotateCcw, XCircle, FileText, Clock3, Layers } from "lucide-react";
import {
  listPipelines,
  listPipelineRuns,
  triggerPipelineRun,
  cancelPipelineRun,
  retryPipelineRun,
} from "@/lib/api/pipelines";
import {
  AdminPageHeader,
  AdminPageLayout,
  AdminToolbar,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  AdminErrorState,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { formatDate } from "@/lib/utils";
import { statusTone } from "@/lib/api/status";

function pipelineStatusTone(s: string) {
  return statusTone(s, "deployment");
}

export default function AdminPipelinesPage() {
  const qc = useQueryClient();
  const [selectedPipeline, setSelectedPipeline] = useState<string>("");

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
  });

  const cancelMutation = useMutation({
    mutationFn: (runId: string) => cancelPipelineRun(runId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pipeline-runs"] }),
  });

  const retryMutation = useMutation({
    mutationFn: (runId: string) => retryPipelineRun(runId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pipeline-runs"] }),
  });

  const defs = useMemo(() => defsQuery.data ?? [], [defsQuery.data]);
  const runs = useMemo(() => runsQuery.data ?? [], [runsQuery.data]);

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminPageHeader
        title="Pipelines"
        description="Deploy — CI/CD pipelines that build and release workloads. Definitions → triggered runs (manual/schedule/webhook) → stages with logs, approvals and retries. Distinct from one-off Deployments and Compose stacks."
      />
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.015] px-4 py-2 text-xs leading-5 text-slate-400">
        <span className="font-semibold text-slate-300">DEPLOY</span> · Pipelines is the delivery workflow surface (Deployments · <span className="font-semibold text-slate-200">Pipelines</span> · Compose · Git). Service <code className="font-mono text-[11px]">pipeline/service.go:181 scheduleLoop</code> evaluates cron every minute; queue worker claims via <code className="font-mono">SKIP LOCKED</code> <code className="font-mono">store.go:314</code>. Webhook: <code className="font-mono">POST /pipelines/webhook/:id</code> (<code className="font-mono">PIPELINE_WEBHOOK_SECRET</code>). See also Deployments for release history.
      </div>

      <Card>
        <CardHeader title="Definitions" icon={Layers} />
        <AdminToolbar>
          <span className="text-xs text-slate-400">
            API: <code className="font-mono">GET /api/v1/pipelines</code> · webhook{" "}
            <code className="font-mono">POST /api/v1/pipelines/webhook/:id</code> (
            <code className="font-mono">PIPELINE_WEBHOOK_SECRET</code>)
          </span>
        </AdminToolbar>
        {defsQuery.isLoading ? (
          <div className="p-6 text-center text-sm text-slate-300">Loading pipelines…</div>
        ) : defsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={defsQuery.error instanceof Error ? defsQuery.error.message : "Failed to load pipelines"}
              retry={() => void defsQuery.refetch()}
            />
            <p className="mt-2 text-xs text-slate-500">
              If this returns 404 the pipeline registrar did not mount — ensure Postgres is up and
              <code className="font-mono"> phase5-pipelines</code> registrar ran (phase5_registrar.go:29).
              Without a DB pool the service logs “pipeline routes skipped” and mounts no routes.
            </p>
          </div>
        ) : defs.length === 0 ? (
          <EmptyState icon={Layers} message="No pipeline definitions yet." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-400">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Trigger</th>
                  <th className="px-4 py-3">Stages</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {defs.map((d) => (
                  <tr
                    key={d.id}
                    className={`hover:bg-white/[0.02] ${selectedPipeline === d.id ? "bg-white/[0.04]" : ""}`}
                    onClick={() => setSelectedPipeline(d.id)}
                  >
                    <td className="px-4 py-3 text-xs font-medium text-slate-200">{d.name}</td>
                    <td className="px-4 py-3">
                      <Pill tone={d.trigger?.enabled ? "green" : "neutral"}>
                        {d.trigger?.type ?? "manual"}
                        {d.trigger?.type === "schedule" && d.trigger.cron ? ` · ${d.trigger.cron}` : ""}
                      </Pill>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">{d.stages?.length ?? 0}</td>
                    <td className="px-4 py-3">
                      <div onClick={(e) => e.stopPropagation()}>
                        <Btn
                          size="sm"
                          tone="primary"
                          onClick={() => triggerMutation.mutate(d.id)}
                          disabled={triggerMutation.isPending}
                        >
                          <Play size={12} /> Trigger
                        </Btn>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {selectedPipeline && (
          <div className="border-t border-white/[0.06] px-4 py-2 text-xs text-slate-400">
            Filtering runs by pipeline <code className="font-mono">{selectedPipeline}</code>{" "}
            <Btn size="sm" tone="ghost" onClick={() => setSelectedPipeline("")}>
              Clear
            </Btn>
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title={`Runs${selectedPipeline ? " (filtered)" : ""} — ${runs.length}`} icon={Clock3} />
        <AdminToolbar>
          <span className="text-xs text-slate-400">
            <code className="font-mono">GET /api/v1/pipeline-runs?pipelineId=&status=</code> ·{" "}
            <code className="font-mono">GET /pipeline-runs/:id/logs?after=</code> · approve/reject
            stages at <code className="font-mono">POST /pipeline-runs/:id/stages/:stageId/approve</code>
          </span>
        </AdminToolbar>
        {runsQuery.isLoading ? (
          <div className="p-6 text-center text-sm text-slate-300">Loading runs…</div>
        ) : runsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={runsQuery.error instanceof Error ? runsQuery.error.message : "Failed to load runs"}
              retry={() => void runsQuery.refetch()}
            />
          </div>
        ) : runs.length === 0 ? (
          <EmptyState icon={FileText} message="No runs yet. Trigger a pipeline above or wait for the schedule/webhook." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-400">
                  <th className="px-4 py-3">Run</th>
                  <th className="px-4 py-3">Pipeline</th>
                  <th className="px-4 py-3">Trigger</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Progress</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {runs.map((r) => {
                  const tone = pipelineStatusTone(r.status) ?? "neutral";
                  return (
                    <tr key={r.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-[11px] text-slate-300" title={r.id}>
                        {r.id.slice(0, 8)}…
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-300">
                        {r.pipelineName ?? r.pipelineId.slice(0, 8)}
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone="neutral">{r.trigger}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone={tone}>{r.status}</Pill>
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-400">
                        {r.currentStage ? `${r.currentStage} ` : ""}
                        {typeof r.progressPct === "number" ? `${r.progressPct}%` : "—"}
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-400">{formatDate(r.createdAt)}</td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1">
                          {(r.status === "queued" || r.status === "running" || r.status === "await_approval") && (
                            <Btn size="sm" tone="danger" onClick={() => cancelMutation.mutate(r.id)} disabled={cancelMutation.isPending}>
                              <XCircle size={12} /> Cancel
                            </Btn>
                          )}
                          {(r.status === "failed" || r.status === "cancelled") && (
                            <Btn size="sm" tone="ghost" onClick={() => retryMutation.mutate(r.id)} disabled={retryMutation.isPending}>
                              <RotateCcw size={12} /> Retry
                            </Btn>
                          )}
                          <a
                            href={`/api/v1/pipeline-runs/${encodeURIComponent(r.id)}/logs`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1 rounded-lg px-2 py-1 text-xs text-slate-400 hover:text-slate-200"
                          >
                            <FileText size={12} /> Logs
                          </a>
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

      <Card className="p-4">
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-widest text-slate-400">Wiring notes</h4>
        <ul className="list-disc space-y-1 pl-5 text-xs leading-5 text-slate-400">
          <li>
            Service: <code className="font-mono">services/pipeline/service.go</code> queue loop +{" "}
            <code className="font-mono">scheduleLoop:181</code> (cron every minute). No enable flag — the
            service is additive; when the DB pool is absent the registrar skips mounting and every
            <code className="font-mono"> /pipelines</code> path would 404 intentionally.
          </li>
          <li>
            HTTP: <code className="font-mono">phase5_registrar.go:29</code> mounts{" "}
            <code className="font-mono">/ pipelines, /pipelines/:id, /pipelines/:id/runs, /pipeline-runs, /pipeline-runs/:id/logs, /pipeline-runs/:id/artifacts</code>{" "}
            plus <code className="font-mono">POST /pipelines/webhook/:id</code> (public,{" "}
            <code className="font-mono">PIPELINE_WEBHOOK_SECRET</code>).
          </li>
          <li>
            No dead route remains 404 when DB is present — this page proves liveness by listing
            definitions; empty list is valid (0 pipelines) not 404.
          </li>
        </ul>
      </Card>
    </AdminPageLayout>
  );
}
