"use client";

import { useMemo, type ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useParams, useRouter } from "next/navigation";
import { GitCommit, RotateCcw, Server, XOctagon, CheckCircle } from "lucide-react";
import { fetchJSON, unwrapList } from "@/lib/api";
import { cancelDeployment, completeDeployment, fetchDeployment, rollbackToPrevious } from "@/lib/api/deployments";
import { deploymentStatusTone } from "@/lib/api/status";
import { sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
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
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { errorMessage, formatDate } from "@/lib/utils";

type TimelineEvent = {
  id: string;
  resourceType?: string;
  resourceId?: string;
  eventType: string;
  source?: string;
  payload?: Record<string, unknown>;
  timestamp: string;
};

const POLL_MS = 10_000;

/** `_` appears on the wire (`in_progress`); operators read spaces. */
function humanToken(value: string): string {
  return value.replace(/_/g, " ");
}

function strategyLabel(value: string): string {
  return value.replace(/_/g, "-");
}

function timeOrDash(value?: string | null): string {
  return value ? formatDate(value) : "—";
}

export default function AdminDeploymentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const id = params.id as string;

  const depQuery = useQuery({
    queryKey: ["admin", "deployments", id],
    queryFn: () => fetchDeployment(id),
    refetchInterval: POLL_MS,
  });

  const timelineQuery = useQuery({
    queryKey: ["admin", "deployments", id, "timeline"],
    queryFn: () =>
      fetchJSON<TimelineEvent[] | { data?: TimelineEvent[] }>(
        `/timeline/deployments/${encodeURIComponent(id)}`,
      ).then((body) => unwrapList<TimelineEvent>(body)),
    refetchInterval: POLL_MS,
  });

  const rollbackMutation = useMutation({
    mutationFn: () => rollbackToPrevious(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
      toast({ tone: "success", title: "Rollback requested", message: "The deployment is rolling back to the previous revision." });
    },
    onError: (err) => toast({ tone: "error", title: "Rollback failed", message: errorMessage(err) }),
  });

  const completeMutation = useMutation({
    mutationFn: () => completeDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
      toast({ tone: "success", title: "Deployment marked complete", message: "The release was recorded as complete." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to complete deployment", message: errorMessage(err) }),
  });

  const cancelMutation = useMutation({
    mutationFn: () => cancelDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
      toast({ tone: "success", title: "Cancellation requested", message: "The deployment will stop at the next step boundary." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to cancel deployment", message: errorMessage(err) }),
  });

  const dep = depQuery.data;
  const timeline = useMemo(() => timelineQuery.data ?? [], [timelineQuery.data]);

  if (depQuery.isPending) {
    return (
      <AdminPageLayout>
        <AdminLoadingState label="Loading deployment…" />
      </AdminPageLayout>
    );
  }

  // A failed read is not a missing record: this used to fall through to
  // "Deployment not found." for every 500, timeout and expired session.
  if (depQuery.isError || !dep) {
    return (
      <AdminPageLayout>
        <AdminPageHeader
          title="Deployment"
          backAction={() => router.push("/admin/deployments")}
          backLabel="Deployments"
        />
        <AdminErrorState
          message={
            depQuery.isError
              ? `This deployment could not be read: ${errorMessage(depQuery.error)}`
              : "The control plane answered without a deployment record."
          }
          retry={() => void depQuery.refetch()}
        />
      </AdminPageLayout>
    );
  }

  // `POST /admin/deployments/:id/rollback-previous` needs a current revision to
  // stand down from (`revisions.go:224-242`), so the control is honest about
  // what it can do rather than being hidden.
  const canRollback = Boolean(dep.currentRevisionId);
  const rollbackReason = canRollback
    ? undefined
    : "Rollback needs a recorded current revision for this deployment; none is set.";
  const canComplete = dep.status === "in_progress";
  const completeReason = canComplete
    ? undefined
    : `Only an in-progress deployment can be marked complete; this one is “${humanToken(dep.status)}”.`;
  const canCancel = dep.status === "pending" || dep.status === "in_progress";
  const cancelReason = canCancel
    ? undefined
    : `Only a pending or in-progress deployment can be cancelled; this one is “${humanToken(dep.status)}”.`;

  const busy = rollbackMutation.isPending || completeMutation.isPending || cancelMutation.isPending;

  return (
    <AdminPageLayout>
      <AdminPageHeader
        // Detail route: the registry cannot name this resource, so the id is
        // said here — in full, mono, and wrapping, rather than truncated to
        // eight characters an operator cannot search for.
        title={
          <span className="flex min-w-0 flex-col gap-1">
            <span>Deployment</span>
            <code className="t-meta max-w-full break-all rounded border border-line bg-overlay-subtle px-1.5 py-0.5 font-mono text-text-subtle">
              {dep.id}
            </code>
          </span>
        }
        description={`Server ${dep.serverId} · ${strategyLabel(dep.strategy)} strategy`}
        status={<FreshnessBadge state={sourceState(depQuery, POLL_MS)} />}
        backAction={() => router.push("/admin/deployments")}
        backLabel="Deployments"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Btn tone="ghost" onClick={() => router.push(`/admin/deployments/${encodeURIComponent(id)}/revisions`)}>
              <GitCommit aria-hidden="true" size={14} /> Revisions
            </Btn>
            <Btn
              tone="warning"
              onClick={() => void handleRollback()}
              disabled={!canRollback || busy}
              title={rollbackReason}
            >
              <RotateCcw aria-hidden="true" size={14} /> Rollback
            </Btn>
            <Btn
              tone="success"
              onClick={() => void handleComplete()}
              disabled={!canComplete || busy}
              title={completeReason}
            >
              <CheckCircle aria-hidden="true" size={14} /> Complete
            </Btn>
            <Btn
              tone="danger"
              onClick={() => void handleCancel()}
              disabled={!canCancel || busy}
              title={cancelReason}
            >
              <XOctagon aria-hidden="true" size={14} /> Cancel
            </Btn>
          </div>
        }
      />

      {/* The reasons above are also in `title`, which is mouse-only; this line
          puts whichever control is unavailable and why in the readable text. */}
      {rollbackReason || completeReason || cancelReason ? (
        <p className="text-meta text-text-subtle">
          {[rollbackReason, completeReason, cancelReason].filter(Boolean).join(" ")}
        </p>
      ) : null}

      <AdminSection title="Release">
        <Card>
          <dl className="grid grid-cols-2 gap-4 p-4 sm:grid-cols-4">
            <Fact label="Server" icon={<Server aria-hidden="true" size={12} />} mono>
              {dep.serverId || "—"}
            </Fact>
            <Fact label="Image">{dep.image || "—"}</Fact>
            <Fact label="Status">
              <Pill tone={deploymentStatusTone(dep.status)}>{humanToken(dep.status) || "unknown"}</Pill>
            </Fact>
            <Fact label="Progress">
              {typeof dep.progressPct === "number" && Number.isFinite(dep.progressPct)
                ? `${dep.progressPct}%`
                : "Not reported"}
            </Fact>
            <Fact label="Strategy">{strategyLabel(dep.strategy) || "—"}</Fact>
            <Fact label="Active target">
              {dep.activeTarget ? strategyLabel(dep.activeTarget) : "Not reported"}
            </Fact>
            <Fact label="Current revision" mono>
              {dep.currentRevisionId ?? "Not set"}
            </Fact>
            <Fact label="Health check">
              {dep.healthCheckPath
                ? `${dep.healthCheckPath}${dep.healthCheckPort ? `:${dep.healthCheckPort}` : ""}`
                : "Not configured"}
            </Fact>
            <Fact label="Created">{timeOrDash(dep.createdAt)}</Fact>
            <Fact label="Updated">{timeOrDash(dep.updatedAt)}</Fact>
            <Fact label="Completed">{timeOrDash(dep.completedAt)}</Fact>
          </dl>
          {dep.error ? (
            <p className="ui-alert ui-alert-danger mx-4 mb-4" role="status">
              <span className="font-semibold">Recorded failure:</span> {dep.error}
            </p>
          ) : null}
        </Card>
      </AdminSection>

      <AdminSection title="Timeline" description="Observability events recorded against this deployment.">
        <Card>
          {timelineQuery.isPending ? (
            <AdminLoadingState label="Loading timeline…" />
          ) : timelineQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`The timeline could not be read: ${errorMessage(timelineQuery.error)}`}
                retry={() => void timelineQuery.refetch()}
              />
            </div>
          ) : timeline.length === 0 ? (
            <div className="p-4">
              <EmptyState
                icon={GitCommit}
                title="No timeline events"
                message="No events have been recorded for this deployment. This is not a failure to load — the timeline was read and is empty."
              />
            </div>
          ) : (
            <AdminTable label="Deployment timeline">
              <AdminTHead>
                <AdminTh>When</AdminTh>
                <AdminTh>Event</AdminTh>
                <AdminTh>Detail</AdminTh>
                <AdminTh>Source</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {timeline.map((event) => (
                  <AdminTr key={event.id}>
                    <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(event.timestamp)}</AdminTd>
                    <AdminTd className="font-mono text-meta">{humanToken(event.eventType)}</AdminTd>
                    <AdminTd className="max-w-96 break-words text-meta">
                      {typeof event.payload?.message === "string" ? event.payload.message : "—"}
                    </AdminTd>
                    <AdminTd className="text-meta">{event.source || "Not reported"}</AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      </AdminSection>

      {renderConfirm()}
    </AdminPageLayout>
  );

  async function handleRollback() {
    const ok = await confirm({
      title: "Roll this deployment back?",
      description:
        "The previous revision becomes live for this deployment and its server. Traffic moves back to the old target; the current revision is left in place for a later rollout.",
      danger: true,
      confirmLabel: "Roll back",
    });
    if (ok) rollbackMutation.mutate();
  }

  async function handleComplete() {
    const ok = await confirm({
      title: "Mark this deployment complete?",
      description:
        "Records the release as complete without running the remaining steps. Health gates and automatic rollback will not run for what is left.",
      confirmLabel: "Mark complete",
    });
    if (ok) completeMutation.mutate();
  }

  async function handleCancel() {
    const ok = await confirm({
      title: "Cancel this deployment?",
      description: `Steps not yet started for ${dep?.serverId ?? "this server"} will be skipped. Anything already deployed stays deployed.`,
      danger: true,
      confirmLabel: "Cancel deployment",
    });
    if (ok) cancelMutation.mutate();
  }
}

function Fact({
  label,
  children,
  mono,
  icon,
}: {
  label: string;
  children: ReactNode;
  mono?: boolean;
  icon?: ReactNode;
}) {
  return (
    <div className="min-w-0">
      <dt className="flex items-center gap-1.5 text-meta uppercase tracking-wider text-text-muted">
        {icon}
        {label}
      </dt>
      <dd className={`mt-1 break-words text-sm text-text ${mono ? "font-mono" : ""}`}>{children}</dd>
    </div>
  );
}
