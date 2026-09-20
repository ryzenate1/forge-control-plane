"use client";

import { useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useParams, useRouter } from "next/navigation";
import { ArrowRightLeft, GitCommit, History, RotateCcw } from "lucide-react";
import {
  compareRevisions,
  fetchDeploymentRevisions,
  rollbackToPrevious,
  rollbackToRevision,
  type Revision,
} from "@/lib/api/deployments";
import { deploymentStatusTone } from "@/lib/api/status";
import {
  AdminErrorState,
  AdminLoadingRows,
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
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { errorMessage, formatDate } from "@/lib/utils";

type RevisionDiff = {
  fromRevisionId: number;
  toRevisionId: number;
  changes: { field: string; oldValue: string; newValue: string }[];
};

/**
 * Revision history for one deployment.
 *
 * The rollback controls used to derive their own ordering rules client-side:
 * "Rollback to Previous" computed `revs.findIndex(active) + 1`, and when the
 * active revision happened to be the last element of the array the click did
 * nothing at all — no toast, no error, button still enabled — while the
 * per-row gate (`idx > 0`) invented a *second*, opposite ordering assumption.
 * Both now defer to the server: `POST /:id/rollback-previous` picks the previous
 * revision (`revisions.go:224`) and `POST /:id/revisions/:revId/rollback`
 * accepts any revision belonging to the deployment (`revisions.go:147-164`).
 */

function humanToken(value: string): string {
  return value.replace(/_/g, " ");
}

export default function DeploymentRevisionsPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const id = params.id as string;
  const [showDiff, setShowDiff] = useState(false);
  const [diffFrom, setDiffFrom] = useState("");
  const [diffTo, setDiffTo] = useState("");
  const [diffData, setDiffData] = useState<RevisionDiff | null>(null);

  const revsQuery = useQuery({
    queryKey: ["admin", "deployments", id, "revisions"],
    queryFn: () => fetchDeploymentRevisions(id),
  });

  const rollbackMutation = useMutation({
    mutationFn: (revisionId: string | null) =>
      revisionId === null ? rollbackToPrevious(id) : rollbackToRevision(id, revisionId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id, "revisions"] });
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
      toast({ tone: "success", title: "Rollback started", message: "The deployment is moving to the selected revision." });
    },
    onError: (err) => toast({ tone: "error", title: "Rollback failed", message: errorMessage(err) }),
  });

  const revisions = useMemo<Revision[]>(() => (Array.isArray(revsQuery.data) ? revsQuery.data : []), [revsQuery.data]);
  const activeRevision = revisions.find((r) => r.status === "active");

  // A failed comparison must not look like "no changes": the mutation only
  // swaps in the diff once the panel has returned it, otherwise it reports the
  // failure next to the selector and through a toast.
  const diffMutation = useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) => compareRevisions(id, from, to),
    onSuccess: (data) => {
      setDiffData(data);
      setShowDiff(true);
    },
    onError: (err) => toast({ tone: "error", title: "Could not compare revisions", message: errorMessage(err) }),
  });

  const handleCompare = (from: string, to: string) => {
    setDiffData(null);
    diffMutation.mutate({ from, to });
  };

  async function rollbackTo(revision: Revision | null) {
    const target = revision ? `Rev #${revision.revisionNumber}` : "the previous revision";
    const ok = await confirm({
      title: `Roll back to ${target}?`,
      description: `The deployment for server ${revision?.deploymentId ?? id} starts serving ${
        revision?.imageRef ?? "the previous image"
      }. Revisions newer than the target are superseded. This redeploys workloads.`,
      danger: true,
      confirmLabel: "Roll back",
    });
    if (!ok) return;
    rollbackMutation.mutate(revision?.id ?? null);
  }

  return (
    <AdminPageLayout>
      <AdminPageHeader
        // Sub-route of a detail page: no registry row can name it.
        title="Revision History"
        backAction={() => router.push(`/admin/deployments/${encodeURIComponent(id)}`)}
        backLabel="Deployment"
      />

      {revsQuery.isPending ? (
        <AdminLoadingRows cols={4} rows={4} label="Loading revisions…" />
      ) : revsQuery.isError ? (
        <AdminErrorState
          message={`Revisions could not be loaded: ${errorMessage(revsQuery.error)}`}
          retry={() => void revsQuery.refetch()}
        />
      ) : revisions.length === 0 ? (
        <EmptyState
          icon={GitCommit}
          title="No revisions recorded"
          message="This deployment has no revisions yet. Revisions are recorded when a rollout or rollback runs."
        />
      ) : (
        <>
          {activeRevision ? (
            <AdminSection title="Active revision">
              <Card className="p-4">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div className="min-w-0">
                    <p className="text-meta uppercase tracking-wider text-text-muted">Currently serving</p>
                    <div className="mt-1 flex items-center gap-3">
                      <span className="text-sm font-semibold text-text">Rev #{activeRevision.revisionNumber}</span>
                      <Pill tone={deploymentStatusTone(activeRevision.status)}>
                        {humanToken(activeRevision.status)}
                      </Pill>
                    </div>
                    <p className="mt-1 break-all text-meta text-text-subtle">{activeRevision.imageRef || "—"}</p>
                  </div>
                  <Btn
                    tone="warning"
                    onClick={() => void rollbackTo(null)}
                    disabled={rollbackMutation.isPending}
                    title={
                      revisions.length < 2
                        ? "There is no earlier revision recorded to roll back to."
                        : undefined
                    }
                  >
                    <RotateCcw aria-hidden="true" size={14} /> Roll back to previous
                  </Btn>
                </div>
                {revisions.length < 2 ? (
                  <p className="mt-2 text-meta text-text-subtle">
                    Only one revision is recorded, so the control above is unavailable until a second rollout runs.
                  </p>
                ) : null}
              </Card>
            </AdminSection>
          ) : (
            <AdminSection title="Active revision">
              <Card className="p-4">
                <p className="text-meta text-text-subtle">
                  No revision is marked active. The revisions below are listed as the control plane reported them.
                </p>
              </Card>
            </AdminSection>
          )}

          <AdminSection
            title="Revisions"
            description="Newest recorded first, as the control plane returns them."
            action={
              revisions.length >= 2 ? (
                <Btn tone="ghost" size="sm" onClick={() => setShowDiff((v) => !v)}>
                  <ArrowRightLeft aria-hidden="true" size={14} /> {showDiff ? "Hide diff" : "Compare"}
                </Btn>
              ) : null
            }
          >
            <Card>
              {showDiff && revisions.length >= 2 ? (
                <div className="border-b border-line p-4">
                  <AdminToolbar>
                    <AdminSelect
                      label="From revision"
                      value={diffFrom}
                      onChange={setDiffFrom}
                      placeholder="Select…"
                      options={revisions.map((r) => ({
                        value: r.id,
                        label: `Rev #${r.revisionNumber} — ${r.description || "no description"}`,
                      }))}
                    />
                    <AdminSelect
                      label="To revision"
                      value={diffTo}
                      onChange={setDiffTo}
                      placeholder="Select…"
                      options={revisions.map((r) => ({
                        value: r.id,
                        label: `Rev #${r.revisionNumber} — ${r.description || "no description"}`,
                      }))}
                    />
                    <Btn
                      tone="primary"
                      size="sm"
                      disabled={!diffFrom || !diffTo || diffFrom === diffTo || diffMutation.isPending}
                      onClick={() => handleCompare(diffFrom, diffTo)}
                    >
                      <ArrowRightLeft aria-hidden="true" size={14} />
                      {diffMutation.isPending ? "Comparing…" : "Compare"}
                    </Btn>
                  </AdminToolbar>

                  {diffMutation.isError ? (
                    <div className="mt-3" role="alert">
                      <AdminErrorState
                        message={`The comparison could not be loaded: ${errorMessage(diffMutation.error)}`}
                      />
                    </div>
                  ) : diffData ? (
                    <div className="mt-3 space-y-2">
                      <p className="text-meta text-text-subtle">
                        Changes from Rev #{diffData.fromRevisionId} to Rev #{diffData.toRevisionId}
                      </p>
                      {diffData.changes.length === 0 ? (
                        <p className="text-meta text-text-subtle">
                          The comparison ran and found no differences between these revisions.
                        </p>
                      ) : (
                        <AdminTable label="Revision differences">
                          <AdminTHead>
                            <AdminTh>Field</AdminTh>
                            <AdminTh>From</AdminTh>
                            <AdminTh>To</AdminTh>
                          </AdminTHead>
                          <AdminTBody>
                            {diffData.changes.map((ch) => (
                              <AdminTr key={ch.field}>
                                <AdminTd className="font-mono text-meta">{ch.field}</AdminTd>
                                <AdminTd className="max-w-72 break-all font-mono text-meta">
                                  {ch.oldValue || "(empty)"}
                                </AdminTd>
                                <AdminTd className="max-w-72 break-all font-mono text-meta">
                                  {ch.newValue || "(empty)"}
                                </AdminTd>
                              </AdminTr>
                            ))}
                          </AdminTBody>
                        </AdminTable>
                      )}
                    </div>
                  ) : null}
                </div>
              ) : null}

              <AdminTable label="Revisions">
                <AdminTHead>
                  <AdminTh>Revision</AdminTh>
                  <AdminTh>Status</AdminTh>
                  <AdminTh>Image</AdminTh>
                  <AdminTh>Commit</AdminTh>
                  <AdminTh>Config hash</AdminTh>
                  <AdminTh>Created</AdminTh>
                  <AdminTh>Action</AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {revisions.map((rev) => {
                    const isActive = rev.status === "active";
                    const rowBusy = rollbackMutation.isPending && rollbackMutation.variables === rev.id;
                    return (
                      <AdminTr key={rev.id}>
                        <AdminTd className="text-sm">
                          <span className="font-semibold">Rev #{rev.revisionNumber}</span>
                          {rev.description ? (
                            <span className="mt-0.5 block max-w-72 break-words text-meta text-text-subtle">
                              {rev.description}
                            </span>
                          ) : null}
                        </AdminTd>
                        <AdminTd>
                          <Pill tone={deploymentStatusTone(rev.status)}>{humanToken(rev.status) || "unknown"}</Pill>
                        </AdminTd>
                        <AdminTd className="max-w-72 break-all font-mono text-meta">
                          {rev.imageRef || "Not reported"}
                        </AdminTd>
                        <AdminTd className="font-mono text-meta">
                          {rev.gitCommitSha ? (
                            <span className="inline-flex items-center gap-1.5">
                              <GitCommit aria-hidden="true" size={12} />
                              {rev.gitCommitSha.slice(0, 7)}
                            </span>
                          ) : (
                            "Not reported"
                          )}
                        </AdminTd>
                        <AdminTd className="max-w-40 break-all font-mono text-meta">
                          {rev.configHash || "—"}
                        </AdminTd>
                        <AdminTd className="whitespace-nowrap text-meta">
                          {rev.createdAt ? formatDate(rev.createdAt) : "—"}
                        </AdminTd>
                        <AdminTd>
                          <div className="flex flex-wrap items-center gap-2">
                            {/* The server accepts a rollback to any revision of this
                                deployment; only the revision already serving is excluded,
                                and the reason is said here rather than the control vanishing. */}
                            <Btn
                              size="sm"
                              tone="warning"
                              onClick={() => void rollbackTo(rev)}
                              disabled={isActive || rollbackMutation.isPending}
                              loading={rowBusy}
                            >
                              <RotateCcw aria-hidden="true" size={12} /> Roll back
                            </Btn>
                            {isActive ? <span className="text-meta text-text-muted">Already serving</span> : null}
                          </div>
                        </AdminTd>
                      </AdminTr>
                    );
                  })}
                </AdminTBody>
              </AdminTable>
            </Card>
          </AdminSection>

          {rollbackMutation.isPending ? (
            <p className="ui-alert ui-alert-warning" role="status">
              Rollback requested — the deployment is moving to the selected revision.
            </p>
          ) : null}
        </>
      )}

      {renderConfirm()}
    </AdminPageLayout>
  );
}
