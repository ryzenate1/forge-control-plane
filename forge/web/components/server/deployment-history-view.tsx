"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertCircle,
  CheckCircle,
  Clock,
  GitCommit,
  History,
  Loader2,
  RotateCcw,
  XCircle,
} from "lucide-react";
import { DeploymentsView } from "@/components/server/deployments-view";
import { EmptyState, Pagination, ProgressBar, StatusPill } from "@/components/ui/primitives";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { hasServerPermission, useOptionalServerContext } from "@/components/server/server-context";
import { cn, errorMessage } from "@/lib/utils";
import type { ApiServer } from "@/lib/api/types";
import {
  diffDeploymentVersions,
  getDeploymentVersion,
  listDeploymentRollbacks,
  listDeploymentVersions,
  rollbackDeploymentVersion,
  rollbackInFlight,
  shortImage,
  type DeploymentHistoryScope,
  type DeploymentRollbackRecord,
  type DeploymentVersion,
  type DeploymentVersionDiff,
  type RollbackAccepted,
} from "@/lib/api/deployment-rollbacks";

/**
 * Deployment history + one-click rollback to any past version.
 *
 * This extends DeploymentsView rather than replacing it: the release panel
 * below (health checks, force-promote, the zero-downtime release feed) answers
 * "what is happening now", while the version history underneath answers "what
 * has this workload ever run, and how do I put one of those back". They read
 * different API surfaces, so both stay on the page.
 *
 * Everything shown here is what the API actually recorded. Where a version has
 * no commit SHA — rollouts created from an image never have one — the row says
 * so instead of leaving a blank that looks like a bug.
 */

const statusTone: Record<string, "neutral" | "success" | "warning" | "danger" | "info"> = {
  pending: "neutral",
  provisioning: "info",
  in_progress: "info",
  awaiting_health: "info",
  promoting: "info",
  rollback_pending: "warning",
  rolling_back: "warning",
  completed: "success",
  failed: "danger",
  rolled_back: "warning",
  cancelled: "neutral",
};

const statusLabels: Record<string, string> = {
  pending: "Queued",
  provisioning: "Provisioning",
  in_progress: "Running",
  awaiting_health: "Awaiting health",
  promoting: "Promoting",
  rollback_pending: "Rollback queued",
  rolling_back: "Rolling back",
  completed: "Completed",
  failed: "Failed",
  rolled_back: "Rolled back",
  cancelled: "Cancelled",
};

const rollbackTone: Record<string, "neutral" | "success" | "warning" | "danger" | "info"> = {
  pending: "neutral",
  in_progress: "info",
  completed: "success",
  failed: "danger",
  cancelled: "neutral",
};

const STATUS_FILTERS = [
  { value: "", label: "Any status" },
  { value: "completed", label: "Completed" },
  { value: "failed", label: "Failed" },
  { value: "rolled_back", label: "Rolled back" },
  { value: "in_progress", label: "In progress" },
  { value: "cancelled", label: "Cancelled" },
];

function StatusIcon({ status }: { status: string }) {
  if (status === "completed") return <CheckCircle className="h-4 w-4 shrink-0 text-emerald-500" />;
  if (status === "failed") return <XCircle className="h-4 w-4 shrink-0 text-red-400" />;
  if (status === "rolled_back" || status === "rolling_back" || status === "rollback_pending") {
    return <RotateCcw className="h-4 w-4 shrink-0 text-amber-400" />;
  }
  if (rollbackInFlight(status) || status === "running" || status === "started") {
    return <Loader2 className="h-4 w-4 shrink-0 animate-spin text-sky-400" />;
  }
  return <Clock className="h-4 w-4 shrink-0 text-[var(--text-muted)]" />;
}

function formatTime(value: string | null | undefined): string {
  if (!value) return "—";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

/** Compact, honest summary for the confirmation dialog (it takes plain text). */
function describeDiff(diff: DeploymentVersionDiff): string {
  const bits: string[] = [];
  bits.push(`Now running ${shortImage(diff.from.image) || "(no image)"}. After rollback: ${shortImage(diff.to.image)}.`);
  const configChanges = diff.changes ?? [];
  const envChanges = (diff.envDiff ?? []).filter((change) => change.field.startsWith("env."));
  if (configChanges.length === 0 && envChanges.length === 0) {
    bits.push("No other recorded differences between the two versions.");
  } else {
    if (configChanges.length > 0) {
      bits.push(`${configChanges.length} config change(s): ${configChanges.map((c) => c.field).join(", ")}.`);
    }
    if (envChanges.length > 0) {
      bits.push(`${envChanges.length} environment change(s): ${envChanges.map((c) => c.field.slice(4)).join(", ")}.`);
    }
  }
  if (diff.notes?.length) bits.push(diff.notes.join(" "));
  return bits.join(" ");
}

interface DeploymentHistoryViewProps {
  server: ApiServer;
  /**
   * Read history through the /apps scope instead of /servers. Both are served
   * by the same handler; the app scope additionally records the application on
   * each rollback audit row.
   */
  appId?: string;
}

export function DeploymentHistoryView({ server, appId }: DeploymentHistoryViewProps) {
  const scope: DeploymentHistoryScope = appId
    ? { kind: "app", id: appId }
    : { kind: "server", id: server.id };

  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canRollback = hasServerPermission(access, "settings.reinstall");
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();

  const [tab, setTab] = useState<"history" | "rollbacks">("history");
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(25);
  const [statusFilter, setStatusFilter] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [activeRollback, setActiveRollback] = useState<RollbackAccepted | null>(null);
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState<{
    target: DeploymentVersion;
    diff: DeploymentVersionDiff | null;
    error: string | null;
  } | null>(null);

  const versionsQuery = useQuery({
    queryKey: ["deployment-versions", scope.kind, scope.id, page, perPage, statusFilter],
    queryFn: () =>
      listDeploymentVersions(scope, {
        page,
        perPage,
        status: statusFilter || undefined,
      }),
    // Stop polling once nothing is in flight — same rule the release panel uses.
    refetchInterval: (query) => {
      const rows = query.state.data?.data ?? [];
      if (activeRollback) return 4000;
      return rows.some((row) => rollbackInFlight(row.status)) ? 5000 : false;
    },
  });

  const rollbacksQuery = useQuery({
    queryKey: ["deployment-rollbacks", scope.kind, scope.id],
    queryFn: () => listDeploymentRollbacks(scope, { limit: 100 }),
    enabled: tab === "rollbacks",
  });

  const detailQuery = useQuery({
    queryKey: ["deployment-version-detail", scope.kind, scope.id, selectedId],
    queryFn: () => getDeploymentVersion(scope, selectedId as string),
    enabled: !!selectedId,
  });

  // Polled independently of the list: the rollback's deployment row may not be
  // on the page the operator is looking at, and its progress is the point.
  const progressQuery = useQuery({
    queryKey: ["deployment-version-progress", scope.kind, scope.id, activeRollback?.deploymentId],
    queryFn: () => getDeploymentVersion(scope, activeRollback?.deploymentId as string),
    enabled: !!activeRollback?.deploymentId,
    refetchInterval: (query) => (rollbackInFlight(query.state.data?.status) ? 3000 : false),
  });

  const rollbackMut = useMutation({
    mutationFn: (input: { deploymentId: string; reason: string }) =>
      rollbackDeploymentVersion(scope, input.deploymentId, input.reason),
    onSuccess: (accepted) => {
      setActiveRollback(accepted);
      setPending(null);
      setReason("");
      void qc.invalidateQueries({ queryKey: ["deployment-versions", scope.kind, scope.id] });
      void qc.invalidateQueries({ queryKey: ["deployment-rollbacks", scope.kind, scope.id] });
    },
    onError: (err) => {
      setPending((current) =>
        current ? { ...current, error: errorMessage(err, "Rollback could not be started") } : current,
      );
    },
  });

  const versions = versionsQuery.data?.data ?? [];
  const pageCount = versionsQuery.data?.pageCount ?? 1;
  const total = versionsQuery.data?.total ?? 0;

  const progressRow =
    progressQuery.data ??
    (activeRollback ? versions.find((row) => row.id === activeRollback.deploymentId) ?? null : null);

  async function beginRollback(target: DeploymentVersion) {
    setSelectedId(target.id);
    setPending({ target, diff: null, error: null });
    try {
      // No `with` argument: the API compares against what is live right now,
      // which is exactly what the operator is about to change.
      const diff = await diffDeploymentVersions(scope, target.id);
      setPending((current) => (current && current.target.id === target.id ? { ...current, diff } : current));
    } catch (err) {
      setPending((current) =>
        current && current.target.id === target.id
          ? { ...current, error: errorMessage(err, "Could not compare the two versions") }
          : current,
      );
    }
  }

  async function confirmRollback() {
    if (!pending) return;
    const description = pending.diff
      ? describeDiff(pending.diff)
      : `Restores image ${shortImage(pending.target.image)}. The comparison could not be loaded, so the change list is unknown.`;
    const ok = await confirm({
      title: `Roll back to the ${formatTime(pending.target.createdAt)} version?`,
      description,
      danger: true,
      confirmLabel: "Roll back now",
    });
    if (!ok) return;
    rollbackMut.mutate({ deploymentId: pending.target.id, reason: reason.trim() });
  }

  const queryError = versionsQuery.isError
    ? errorMessage(versionsQuery.error, "Failed to load deployment history")
    : null;

  return (
    <div className="space-y-8">
      <DeploymentsView server={server} />

      <section className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="flex items-center gap-2 text-lg font-bold text-[var(--text)]">
              <History className="h-4 w-4 text-[var(--brand)]" />
              Version history &amp; rollbacks
            </h2>
            <p className="mt-1 text-xs text-[var(--text-subtle)]">
              Every version this workload was asked to run, newest first. {total} recorded.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <div className="inline-flex rounded-lg border border-[var(--line)] bg-[var(--surface)] p-0.5">
              {(["history", "rollbacks"] as const).map((value) => (
                <button
                  key={value}
                  type="button"
                  onClick={() => setTab(value)}
                  className={cn(
                    "rounded-md px-3 py-1 text-xs font-semibold capitalize transition-colors",
                    tab === value
                      ? "bg-[var(--brand)]/15 text-[var(--brand)]"
                      : "text-[var(--text-subtle)] hover:text-[var(--text)]",
                  )}
                >
                  {value === "history" ? "Versions" : "Rollback audit"}
                </button>
              ))}
            </div>
            <select
              aria-label="Filter by status"
              value={statusFilter}
              onChange={(event) => {
                setStatusFilter(event.target.value);
                setPage(1);
              }}
              className="ui-input h-9 min-w-[8rem]"
            >
              {STATUS_FILTERS.map((filter) => (
                <option key={filter.value} value={filter.value}>
                  {filter.label}
                </option>
              ))}
            </select>
            <select
              aria-label="Versions per page"
              value={perPage}
              onChange={(event) => {
                setPerPage(Number(event.target.value) || 25);
                setPage(1);
              }}
              className="ui-input h-9 min-w-[6rem]"
            >
              {[10, 25, 50, 100].map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </div>
        </div>

        {(queryError || rollbackMut.isError) && (
          <div className="ui-alert ui-alert-error" role="alert">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
            {queryError || errorMessage(rollbackMut.error, "Rollback failed")}
          </div>
        )}

        {activeRollback && progressRow && (
          <div className="ui-card space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-semibold text-[var(--text)]">Rollback in progress</h3>
              <button
                type="button"
                onClick={() => {
                  setActiveRollback(null);
                  void progressQuery.refetch();
                }}
                className="ui-button ui-button-ghost"
              >
                Dismiss
              </button>
            </div>
            <p className="text-xs text-[var(--text-subtle)]">
              Restoring <span className="font-mono text-[var(--text)]">{shortImage(activeRollback.image)}</span>{" "}
              as deployment <span className="font-mono">{activeRollback.deploymentId.slice(0, 8)}</span>.
              {activeRollback.reason ? ` Reason: ${activeRollback.reason}` : ""}
            </p>
            {typeof progressRow.progressPct === "number" && progressRow.progressPct > 0 ? (
              <ProgressBar value={progressRow.progressPct} label="Rollback progress" />
            ) : (
              <p className="text-xs text-[var(--text-muted)]">
                The executor has not reported a percentage for this step yet.
              </p>
            )}
            <div className="flex items-center gap-2">
              <StatusPill tone={statusTone[progressRow.status] ?? "neutral"} pulse={rollbackInFlight(progressRow.status)}>
                {statusLabels[progressRow.status] ?? progressRow.status}
              </StatusPill>
              {progressRow.error && <span className="text-xs text-red-300">{progressRow.error}</span>}
            </div>
            {!rollbackInFlight(progressRow.status) && (
              <p className="text-xs text-[var(--text-subtle)]">
                {progressRow.status === "completed"
                  ? "Rollback finished. The history below now marks this version as live."
                  : `Rollback stopped with status ${progressRow.status}. Nothing was restored automatically — review the steps in the detail panel.`}
              </p>
            )}
          </div>
        )}

        {tab === "history" ? (
          versionsQuery.isLoading ? (
            <CardSkeleton />
          ) : versions.length === 0 ? (
            <EmptyState
              icon={<History size={20} />}
              title="No deployment history"
              description="Nothing has been deployed through the deployment service for this workload yet, so there is no version to roll back to."
            />
          ) : (
            <div className="grid grid-cols-1 gap-4 xl:grid-cols-3">
              <div className="space-y-2 xl:col-span-2">
                {versions.map((version) => (
                  <div
                    key={version.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => setSelectedId(version.id === selectedId ? null : version.id)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        setSelectedId(version.id === selectedId ? null : version.id);
                      }
                    }}
                    className={cn(
                      "flex cursor-pointer flex-col gap-2 rounded-lg border p-3 transition-colors",
                      selectedId === version.id
                        ? "border-[var(--brand)]/40 bg-[var(--brand)]/10"
                        : "border-[var(--line)] bg-[var(--surface-raised)] hover:border-[var(--line-strong)]",
                    )}
                  >
                    <div className="flex min-w-0 items-center justify-between gap-3">
                      <div className="flex min-w-0 items-center gap-3">
                        <StatusIcon status={version.status} />
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-sm font-semibold text-[var(--text)]">
                              {version.revisionNumber ? `r${version.revisionNumber}` : version.id.slice(0, 8)}
                            </span>
                            <StatusPill
                              tone={statusTone[version.status] ?? "neutral"}
                              pulse={rollbackInFlight(version.status)}
                            >
                              {statusLabels[version.status] ?? version.status}
                            </StatusPill>
                            {version.isLive && <span className="ui-badge ui-badge-success">Live</span>}
                            {version.strategy === "rollback" && (
                              <span className="ui-badge ui-badge-warning">Rollback</span>
                            )}
                            {version.strategy !== "rollback" && (
                              <span className="ui-badge ui-badge-neutral">{version.strategy}</span>
                            )}
                          </div>
                          <div className="mt-1 truncate font-mono text-xs text-[var(--text-subtle)]">
                            {shortImage(version.image) || "no image recorded"}
                          </div>
                        </div>
                      </div>
                      <div className="flex shrink-0 flex-col items-end gap-2">
                        <span className="font-mono text-xs text-[var(--text-muted)]">
                          {formatTime(version.createdAt)}
                        </span>
                        {canRollback && version.canRollbackTo && (
                          <button
                            type="button"
                            onClick={(event) => {
                              event.stopPropagation();
                              void beginRollback(version);
                            }}
                            disabled={rollbackMut.isPending}
                            className="inline-flex items-center gap-1 rounded-lg border border-[var(--warning)]/40 bg-[var(--warning-subtle)] px-2 py-1 text-xs font-bold text-amber-200 transition-colors hover:bg-[var(--warning)]/25 disabled:opacity-40"
                          >
                            <RotateCcw className="h-3 w-3" />
                            Rollback to this
                          </button>
                        )}
                      </div>
                    </div>

                    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--text-muted)]">
                      <span className="inline-flex items-center gap-1">
                        <GitCommit className="h-3 w-3" />
                        {version.commitSha ? (
                          <span className="font-mono text-[var(--text-subtle)]">
                            {version.commitSha.slice(0, 10)}
                          </span>
                        ) : (
                          "no commit recorded"
                        )}
                      </span>
                      {version.completedAt && <span>finished {formatTime(version.completedAt)}</span>}
                      {version.description && <span className="truncate">{version.description}</span>}
                      {version.configHash && (
                        <span className="font-mono">config {version.configHash.slice(0, 8)}</span>
                      )}
                      {version.rolledBackTo && version.rolledBackTo.length > 0 && (
                        <span className="text-amber-200/80">
                          restored {version.rolledBackTo.length}× ({version.rolledBackTo.map((entry) => entry.status).join(", ")})
                        </span>
                      )}
                      {version.rollback && (
                        <span>this row carried a rollback ({version.rollback.status})</span>
                      )}
                    </div>
                  </div>
                ))}

                {pageCount > 1 && (
                  <Pagination page={page} pageCount={pageCount} onPageChange={setPage} label="Version history pages" />
                )}
              </div>

              <div className="space-y-4">
                {selectedId ? (
                  detailQuery.isLoading ? (
                    <CardSkeleton />
                  ) : detailQuery.isError ? (
                    <div className="ui-alert ui-alert-error" role="alert">
                      <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                      {errorMessage(detailQuery.error, "Failed to load version detail")}
                    </div>
                  ) : detailQuery.data ? (
                    <div className="ui-card space-y-3">
                      <div className="flex items-center justify-between">
                        <h3 className="text-xs font-medium uppercase tracking-wider text-[var(--text-subtle)]">
                          Version detail
                        </h3>
                        <button
                          type="button"
                          onClick={() => setSelectedId(null)}
                          className="ui-button ui-button-ghost"
                        >
                          Close
                        </button>
                      </div>
                      <dl className="space-y-1 text-xs">
                        <div className="flex justify-between gap-2">
                          <dt className="text-[var(--text-muted)]">Deployment id</dt>
                          <dd className="font-mono text-[var(--text-subtle)]">{detailQuery.data.id}</dd>
                        </div>
                        <div className="flex justify-between gap-2">
                          <dt className="text-[var(--text-muted)]">Image</dt>
                          <dd className="max-w-[60%] break-all text-right font-mono text-[var(--text-subtle)]">
                            {detailQuery.data.image || "—"}
                          </dd>
                        </div>
                        <div className="flex justify-between gap-2">
                          <dt className="text-[var(--text-muted)]">Compose ref</dt>
                          <dd className="max-w-[60%] break-all text-right font-mono text-[var(--text-subtle)]">
                            {detailQuery.data.composeManifestRef || "none recorded"}
                          </dd>
                        </div>
                      </dl>

                      <div>
                        <h4 className="mb-1 text-xs font-medium uppercase tracking-wider text-[var(--text-subtle)]">
                          Steps
                        </h4>
                        {detailQuery.data.steps.length === 0 ? (
                          <p className="text-xs text-[var(--text-muted)]">No steps were recorded.</p>
                        ) : (
                          <ol className="space-y-1">
                            {detailQuery.data.steps.map((step) => (
                              <li key={step.id} className="flex items-center justify-between gap-2 text-xs">
                                <span className="flex items-center gap-1.5">
                                  <StatusIcon status={step.status} />
                                  <span className="text-[var(--text-subtle)]">{step.stepName}</span>
                                </span>
                                <span className="font-mono text-[var(--text-muted)]">{step.status}</span>
                              </li>
                            ))}
                          </ol>
                        )}
                      </div>

                      {detailQuery.data.environment && Object.keys(detailQuery.data.environment).length > 0 ? (
                        <div>
                          <h4 className="mb-1 text-xs font-medium uppercase tracking-wider text-[var(--text-subtle)]">
                            Environment snapshot
                          </h4>
                          <ul className="space-y-0.5">
                            {Object.entries(detailQuery.data.environment).map(([key, value]) => (
                              <li key={key} className="flex justify-between gap-2 text-xs">
                                <span className="font-mono text-[var(--text-subtle)]">{key}</span>
                                <span className="max-w-[55%] truncate text-right font-mono text-[var(--text-muted)]">
                                  {String(value)}
                                </span>
                              </li>
                            ))}
                          </ul>
                        </div>
                      ) : (
                        <p className="text-xs text-[var(--text-muted)]">
                          No environment snapshot was stored for this version — only image-based rollouts are
                          recorded, so environment differences cannot be shown.
                        </p>
                      )}

                      {detailQuery.data.rollbacks.length > 0 && (
                        <div>
                          <h4 className="mb-1 text-xs font-medium uppercase tracking-wider text-[var(--text-subtle)]">
                            Rollback records
                          </h4>
                          <ul className="space-y-1">
                            {detailQuery.data.rollbacks.map((record) => (
                              <li key={record.id} className="text-xs text-[var(--text-subtle)]">
                                <span className="font-mono">{record.id.slice(0, 8)}</span> · {record.status} ·{" "}
                                {formatTime(record.createdAt)}
                                {record.reason ? ` — ${record.reason}` : ""}
                              </li>
                            ))}
                          </ul>
                        </div>
                      )}
                    </div>
                  ) : null
                ) : (
                  <div className="ui-card text-xs text-[var(--text-subtle)]">
                    Select a version to see its steps, compose ref and environment snapshot.
                  </div>
                )}
              </div>
            </div>
          )
        ) : (
          <RollbackAuditPanel
            loading={rollbacksQuery.isLoading}
            error={
              rollbacksQuery.isError
                ? errorMessage(rollbacksQuery.error, "Failed to load rollback audit")
                : null
            }
            records={rollbacksQuery.data ?? []}
            onInspect={(deploymentId) => {
              setTab("history");
              setSelectedId(deploymentId);
            }}
          />
        )}
      </section>

      {pending && (
        <div className="ui-card space-y-3 border-[var(--warning)]/40">
          <div className="flex items-center gap-2">
            <RotateCcw className="h-4 w-4 text-amber-300" />
            <h3 className="text-sm font-semibold text-[var(--text)]">
              Roll back to the version from {formatTime(pending.target.createdAt)}?
            </h3>
          </div>

          {pending.diff ? (
            <div className="space-y-2 text-xs">
              <div className="grid gap-2 sm:grid-cols-2">
                <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-2">
                  <p className="mb-1 font-semibold uppercase tracking-wider text-[var(--text-muted)]">Running now</p>
                  <p className="break-all font-mono text-[var(--text-subtle)]">
                    {pending.diff.from.image || "(no image)"}
                  </p>
                  <p className="mt-1 text-[var(--text-muted)]">
                    {statusLabels[pending.diff.from.status] ?? pending.diff.from.status} ·{" "}
                    {pending.diff.from.commitSha ? pending.diff.from.commitSha.slice(0, 10) : "no commit"}
                  </p>
                </div>
                <div className="rounded-lg border border-[var(--warning)]/40 bg-[var(--warning-subtle)] p-2">
                  <p className="mb-1 font-semibold uppercase tracking-wider text-amber-200/80">After rollback</p>
                  <p className="break-all font-mono text-[var(--text-subtle)]">{pending.diff.to.image}</p>
                  <p className="mt-1 text-[var(--text-muted)]">
                    {statusLabels[pending.diff.to.status] ?? pending.diff.to.status} ·{" "}
                    {pending.diff.to.commitSha ? pending.diff.to.commitSha.slice(0, 10) : "no commit"}
                  </p>
                </div>
              </div>

              {(pending.diff.changes ?? []).length === 0 && (pending.diff.envDiff ?? []).length === 0 ? (
                <p className="text-[var(--text-muted)]">
                  No recorded configuration or environment differences — only the image changes.
                </p>
              ) : (
                <ul className="space-y-1">
                  {[...(pending.diff.changes ?? []), ...(pending.diff.envDiff ?? [])].map((change, index) => (
                    <li key={`${change.field}-${index}`} className="flex flex-wrap gap-2">
                      <span className="font-mono text-[var(--text-subtle)]">{change.field}</span>
                      <span className="font-mono text-red-300/80 line-through">{change.oldValue || "∅"}</span>
                      <span className="text-[var(--text-muted)]">→</span>
                      <span className="font-mono text-emerald-300/90">{change.newValue || "∅"}</span>
                    </li>
                  ))}
                </ul>
              )}

              {pending.diff.notes?.map((note) => (
                <p key={note} className="text-[var(--text-muted)] italic">
                  {note}
                </p>
              ))}
            </div>
          ) : pending.error ? (
            <p className="text-xs text-red-300">{pending.error}</p>
          ) : (
            <p className="flex items-center gap-2 text-xs text-[var(--text-muted)]">
              <Loader2 className="h-3 w-3 animate-spin" /> Comparing the two versions…
            </p>
          )}

          <label className="ui-label block" htmlFor="rollback-reason">
            Reason (recorded in the audit trail)
          </label>
          <textarea
            id="rollback-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            rows={2}
            maxLength={2000}
            placeholder="e.g. checkout latency doubled after the last deploy"
            className="ui-input w-full"
          />

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => void confirmRollback()}
              disabled={rollbackMut.isPending || !canRollback}
              className="ui-button ui-button-danger"
            >
              {rollbackMut.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RotateCcw className="h-4 w-4" />
              )}
              Roll back to this version
            </button>
            <button
              type="button"
              onClick={() => setPending(null)}
              className="ui-button ui-button-secondary"
            >
              Cancel
            </button>
            {!canRollback && (
              <span className="text-xs text-[var(--text-muted)]">
                Your role cannot change what is deployed on this server.
              </span>
            )}
          </div>
        </div>
      )}

      {renderConfirm()}
    </div>
  );
}

function RollbackAuditPanel({
  loading,
  error,
  records,
  onInspect,
}: {
  loading: boolean;
  error: string | null;
  records: DeploymentRollbackRecord[];
  onInspect: (deploymentId: string) => void;
}) {
  if (error) {
    return (
      <div className="ui-alert ui-alert-error" role="alert">
        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" /> {error}
      </div>
    );
  }
  if (loading) return <CardSkeleton />;
  if (records.length === 0) {
    return (
      <EmptyState
        icon={<RotateCcw size={20} />}
        title="No rollbacks recorded"
        description="Explicit rollbacks are written to the audit trail with who asked, why, and which deployment carried them out."
      />
    );
  }
  return (
    <div className="ui-card space-y-2">
      {records.map((record) => (
        <div
          key={record.id}
          className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] pb-2 text-xs last:border-0 last:pb-0"
        >
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-[var(--text-subtle)]">{record.id.slice(0, 8)}</span>
              <StatusPill tone={rollbackTone[record.status] ?? "neutral"}>{record.status}</StatusPill>
              {record.initiatedBy && (
                <span className="text-[var(--text-muted)]">by {record.initiatedBy.slice(0, 8)}</span>
              )}
            </div>
            <p className="text-[var(--text-subtle)]">
              Target <span className="font-mono">{record.deploymentId.slice(0, 8)}</span>
              {record.triggeredByDeploymentId
                ? ` · triggered by ${record.triggeredByDeploymentId.slice(0, 8)}`
                : ""}
              {record.rollbackDeploymentId
                ? ` · carried out by ${record.rollbackDeploymentId.slice(0, 8)}`
                : " · no deployment row was created"}
            </p>
            {record.reason && <p className="text-[var(--text-muted)]">{record.reason}</p>}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <span className="font-mono text-[var(--text-muted)]">{formatTime(record.createdAt)}</span>
            {record.rollbackDeploymentId && (
              <button
                type="button"
                onClick={() => onInspect(record.rollbackDeploymentId as string)}
                className="ui-button ui-button-secondary"
              >
                Inspect
              </button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
