"use client";

import { useEffect, useMemo, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import { useToast } from "@/components/ui/toast";
import {
  cancelSourceDeployment,
  deleteSourceDeployment,
  deploySourceDeployment,
  getDeploymentBuildLogs,
  getSourceDeployment,
  type BuildLog,
} from "@/lib/api/source-deployments";
import { statusLabel } from "@/lib/api/apps";
import { sourceStatusTone } from "@/lib/api/status";
import { Play, XCircle, Trash2, RefreshCw } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  AdminSection,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Pill,
  SectionHeader,
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { errorMessage, formatDate } from "@/lib/utils";

/** Terminal states per `handlers_source_deployments.go:220-223`. */
const TERMINAL = new Set(["completed", "failed", "canceled"]);

function repoName(repository: string): string {
  const last = repository.split("/").pop() ?? repository;
  return last.replace(/\.git$/i, "") || repository;
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-meta uppercase tracking-wider text-text-muted">{label}</dt>
      <dd className="mt-1 break-words text-sm text-text">{children}</dd>
    </div>
  );
}

/**
 * Log stage colour came from a private ladder of `text-danger` /
 * `text-warn` / `text-ok` / `text-text-subtle`; it now uses the same
 * tone vocabulary the rest of the admin uses.
 */
function stageClass(stage: string): string {
  if (stage === "error" || stage === "failed") return "text-danger";
  if (stage === "warning") return "text-warn";
  if (stage === "success") return "text-ok";
  return "text-text-subtle";
}

export default function SourceDeploymentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const logsEndRef = useRef<HTMLDivElement>(null);
  const id = params.id as string;

  const deploymentQuery = useQuery({
    queryKey: ["sourceDeployment", id],
    queryFn: () => getSourceDeployment(id),
    enabled: Boolean(id),
    refetchInterval: (query) => {
      const d = query.state.data;
      return d && !TERMINAL.has(d.status) && d.status !== "healthy" && d.status !== "unhealthy" ? 3_000 : false;
    },
  });

  const deployment = deploymentQuery.data;
  const isActive = Boolean(deployment && !TERMINAL.has(deployment.status));

  const logsQuery = useQuery({
    queryKey: ["buildLogs", id],
    queryFn: () => getDeploymentBuildLogs(id),
    enabled: Boolean(id),
    refetchInterval: isActive ? 3_000 : false,
  });

  const logs = useMemo(() => (Array.isArray(logsQuery.data) ? logsQuery.data : []), [logsQuery.data]);

  // Following the tail of a build is right; being dragged back down while reading
  // an earlier line is not. Follow only when already at the bottom.
  useEffect(() => {
    const node = logsEndRef.current;
    if (!node) return;
    const container = node.parentElement;
    if (!container) return;
    const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 80;
    if (nearBottom) node.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [logs]);

  const deployMutation = useMutation({
    mutationFn: () => deploySourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployment", id] });
      queryClient.invalidateQueries({ queryKey: ["buildLogs", id] });
      toast({ tone: "success", title: "Build queued", message: "The deployment was queued for a rebuild." });
    },
    onError: (err) => toast({ tone: "error", title: "Build not queued", message: errorMessage(err) }),
  });

  const cancelMutation = useMutation({
    mutationFn: () => cancelSourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployment", id] });
      queryClient.invalidateQueries({ queryKey: ["buildLogs", id] });
      toast({ tone: "success", title: "Deployment canceled" });
    },
    onError: (err) => toast({ tone: "error", title: "Cancel failed", message: errorMessage(err) }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteSourceDeployment(id),
    onSuccess: () => {
      router.push("/admin/source-deployments");
      toast({ tone: "success", title: "Source deployment removed" });
    },
    onError: (err) => toast({ tone: "error", title: "Delete failed", message: errorMessage(err) }),
  });

  const heading = (
    <SectionHeader
      backAction={() => router.push("/admin/source-deployments")}
      backLabel="Source Deployments"
      sub={
        deployment
          ? `${deployment.repository} · branch ${deployment.branch || "not reported"}`
          : undefined
      }
      status={
        deployment ? <FreshnessBadge state={sourceState(deploymentQuery, 3_000)} /> : undefined
      }
      title={deployment ? repoName(deployment.repository) : "Source deployment"}
      action={
        deployment
          ? (() => {
              const cancelBlocked = TERMINAL.has(deployment.status)
                ? `Already ${statusLabel(deployment.status)} — the control plane refuses to cancel a deployment in a terminal state.`
                : undefined;
              return (
                <div className="flex flex-wrap items-center gap-2">
                  <Btn
                    ariaLabel="Reload this deployment"
                    disabled={deploymentQuery.isFetching}
                    onClick={() => void deploymentQuery.refetch()}
                    size="sm"
                    tone="ghost"
                  >
                    <RefreshCw aria-hidden="true" size={14} />
                  </Btn>
                  <Pill tone={sourceStatusTone(deployment.status)}>
                    {statusLabel(deployment.status) || "unknown"}
                  </Pill>
                  <Btn
                    disabled={Boolean(cancelBlocked) || cancelMutation.isPending}
                    loading={cancelMutation.isPending}
                    onClick={() => void askCancel()}
                    size="sm"
                    tone="ghost"
                  >
                    <XCircle aria-hidden="true" size={14} /> Cancel
                  </Btn>
                  <Btn
                    disabled={deployMutation.isPending}
                    loading={deployMutation.isPending}
                    onClick={() => deployMutation.mutate()}
                    size="sm"
                    tone="primary"
                  >
                    <Play aria-hidden="true" size={14} /> Deploy
                  </Btn>
                  <Btn
                    ariaLabel="Delete this source deployment"
                    disabled={deleteMutation.isPending}
                    loading={deleteMutation.isPending}
                    onClick={() => void askDelete()}
                    size="sm"
                    tone="danger"
                  >
                    <Trash2 aria-hidden="true" size={14} />
                  </Btn>
                  {cancelBlocked ? (
                    <span className="w-full text-right text-meta text-text-muted">{cancelBlocked}</span>
                  ) : null}
                </div>
              );
            })()
          : undefined
      }
    />
  );

  if (deploymentQuery.isPending) {
    return (
      <AdminPageLayout className="max-w-4xl">
        {heading}
        <AdminLoadingState label="Loading this source deployment…" />
      </AdminPageLayout>
    );
  }

  if (deploymentQuery.isError) {
    return (
      <AdminPageLayout className="max-w-4xl">
        {heading}
        <AdminErrorState
          message={`This source deployment could not be read: ${errorMessage(deploymentQuery.error)}`}
          retry={() => void deploymentQuery.refetch()}
        />
      </AdminPageLayout>
    );
  }

  if (!deployment) {
    return (
      <AdminPageLayout className="max-w-4xl">
        {heading}
        <EmptyState
          message="The control plane answered this request and returned no source deployment for this id."
          title="No source deployment with this id"
        />
      </AdminPageLayout>
    );
  }

  return (
    <AdminPageLayout className="max-w-4xl">
      {heading}
      {renderConfirm()}

      <AdminSection description="Stored on the source deployment record; not a live measurement." title="Build and target">
        <Card>
          <dl className="grid grid-cols-2 gap-4 p-4 sm:grid-cols-3">
            <Fact label="Build type">{statusLabel(deployment.buildType)}</Fact>
            <Fact label="Build context">
              {deployment.buildContext ? <span className="font-mono">{deployment.buildContext}</span> : "Not reported"}
            </Fact>
            <Fact label="Dockerfile path">
              {deployment.dockerfilePath ? (
                <span className="font-mono">{deployment.dockerfilePath}</span>
              ) : (
                "Not applicable for this build type"
              )}
            </Fact>
            <Fact label="Auto-deploy on push">
              {typeof deployment.autoDeploy === "boolean" ? (deployment.autoDeploy ? "Enabled" : "Disabled") : "Not reported"}
            </Fact>
            <Fact label="Registry">
              {deployment.registry ? <span className="font-mono">{deployment.registry}</span> : "No registry recorded"}
            </Fact>
            <Fact label="Image tag">
              {deployment.imageTag ? <span className="font-mono">{deployment.imageTag}</span> : "Not built yet"}
            </Fact>
            <Fact label="Server">
              {deployment.serverId ? <span className="font-mono">{deployment.serverId}</span> : "No target recorded"}
            </Fact>
            <Fact label="Git provider">
              {deployment.gitProviderId ? (
                <span className="font-mono">{deployment.gitProviderId}</span>
              ) : (
                "None — cloned anonymously"
              )}
            </Fact>
            <Fact label="Created by">{deployment.createdBy || "Not reported"}</Fact>
            <Fact label="Health gate">
              {deployment.healthCheckPath
                ? `${deployment.healthCheckPath}${
                    typeof deployment.healthCheckPort === "number" ? ` on port ${deployment.healthCheckPort}` : " (port not reported)"
                  }${deployment.rollbackOnHealthFailure ? " · roll back on failure" : " · no rollback on failure"}`
                : "No health check configured"}
            </Fact>
            <Fact label="Webhook">
              {deployment.webhookUrl ? (
                <a
                  className="break-all text-brand underline underline-offset-2"
                  href={deployment.webhookUrl}
                  rel="noreferrer noopener"
                  target="_blank"
                >
                  {deployment.webhookUrl}
                </a>
              ) : (
                "No webhook recorded"
              )}
            </Fact>
            <Fact label="Commit">
              {deployment.commitHash ? (
                <>
                  <span className="font-mono">{deployment.commitHash}</span>
                  {deployment.commitMessage ? (
                    <span className="mt-1 block text-meta text-text-subtle">
                      {deployment.commitMessage}
                      {deployment.commitAuthor ? ` — ${deployment.commitAuthor}` : ""}
                    </span>
                  ) : null}
                </>
              ) : (
                "No commit recorded"
              )}
            </Fact>
          </dl>
        </Card>
      </AdminSection>

      <AdminSection
        action={
          logsQuery.isFetching ? <span className="text-meta text-text-subtle">Refreshing…</span> : undefined
        }
        description={isActive ? "Polling every 3 s while the build is in flight." : "The build is not running, so the log is no longer polled."}
        title="Build log"
      >
        <Card>
          {logsQuery.isPending ? (
            <AdminLoadingState label="Loading build log…" />
          ) : logsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`The build log could not be read: ${errorMessage(logsQuery.error)}`}
                retry={() => void logsQuery.refetch()}
              />
            </div>
          ) : logs.length === 0 ? (
            <div className="p-4">
              <EmptyState
                message={
                  isActive
                    ? "The build is running and the log request has answered with no lines yet."
                    : "The log request completed and returned no lines for this deployment."
                }
                title="No log lines"
              />
            </div>
          ) : (
            <AdminTable label="Build log lines">
              <AdminTHead>
                <AdminTh>Stage</AdminTh>
                <AdminTh>Time</AdminTh>
                <AdminTh>Message</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {logs.map((log: BuildLog) => (
                  <AdminTr key={log.id}>
                    <AdminTd className={`whitespace-nowrap font-mono text-meta ${stageClass(log.stage)}`}>
                      {statusLabel(log.stage) || "log"}
                    </AdminTd>
                    <AdminTd className="whitespace-nowrap font-mono text-meta">
                      {log.createdAt ? formatDate(log.createdAt) : "—"}
                    </AdminTd>
                    <AdminTd className="break-words text-meta">{log.message}</AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
          <div ref={logsEndRef} />
        </Card>
      </AdminSection>
    </AdminPageLayout>
  );

  async function askCancel() {
    const ok = await confirm({
      confirmLabel: "Cancel deployment",
      danger: true,
      description: `The in-flight build for ${deployment!.repository} (${deployment!.branch}) is marked canceled. Anything already deployed to the server stays as it is.`,
      title: "Cancel this build?",
    });
    if (ok) cancelMutation.mutate();
  }

  async function askDelete() {
    const ok = await confirm({
      confirmLabel: "Delete source deployment",
      danger: true,
      description: `${deployment!.repository} (${deployment!.branch}) is removed together with its whole build history. The deployed server and its running container are not undone by this.`,
      title: "Delete this source deployment?",
    });
    if (ok) deleteMutation.mutate();
  }
}
