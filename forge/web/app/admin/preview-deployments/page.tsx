"use client";

import { useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useRouter, useSearchParams } from "next/navigation";
import { ExternalLink, GitPullRequest, Play, Trash2 } from "lucide-react";
import Link from "next/link";
import {
  cleanupPreview,
  deployPreview,
  fetchPreviewDeployments,
  type PreviewDeployment,
} from "@/lib/api/preview-deployments";
import { statusLabel } from "@/lib/api/apps";
import { previewStatusTone } from "@/lib/api/status";
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
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { errorMessage, formatDate } from "@/lib/utils";

const POLL_MS = 15_000;

/**
 * The five states this API persists (`PreviewDeployment.status` is a union of
 * exactly these in `lib/api/preview-deployments.ts`). The filter offers them
 * rather than a hand-written guess, and colour comes from the shared
 * `previewStatusTone` table — this page used to carry a private map that filed
 * `deploying` as inactive grey (the same chip as `cleaned_up`) and an
 * operator-intentional `stopped` as a warning, contradicting the shared table.
 */
const STATUSES: PreviewDeployment["status"][] = ["deploying", "running", "stopped", "failed", "cleaned_up"];

/** `POST /:id/deploy` only acts on a run in `deploying` (`previewenv/service.go:311-321`). */
function deployReason(preview: PreviewDeployment): string | undefined {
  if (preview.status === "running") return "Already deployed.";
  if (preview.status === "cleaned_up") return "Cleaned up — nothing left to deploy.";
  if (preview.status !== "deploying") return `The control plane only starts a preview from “deploying”; this one is “${statusLabel(preview.status)}”.`;
  return undefined;
}

function cleanupReason(preview: PreviewDeployment): string | undefined {
  if (preview.status === "cleaned_up") return "Already cleaned up.";
  return undefined;
}

function timeOrDash(value?: string | null): string {
  // `formatDate`'s own fallback is "Never", which is a claim about an outcome;
  // an absent timestamp here just has not been reported.
  return value ? formatDate(value) : "—";
}

export default function AdminPreviewDeploymentsPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [confirm, renderConfirm] = useConfirm();

  const statusFilter = searchParams.get("status") ?? "";
  const search = searchParams.get("q") ?? "";

  const setParam = (key: string, value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (value) params.set(key, value);
    else params.delete(key);
    const query = params.toString();
    router.replace(query ? `/admin/preview-deployments?${query}` : "/admin/preview-deployments", { scroll: false });
  };

  const clearFilters = () => {
    router.replace("/admin/preview-deployments", { scroll: false });
  };

  const previewsQuery = useQuery({
    queryKey: ["admin", "preview-deployments"],
    queryFn: () => fetchPreviewDeployments(),
    refetchInterval: POLL_MS,
  });

  const deployMutation = useMutation({
    mutationFn: (id: string) => deployPreview(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments"] });
      toast({ tone: "success", title: "Preview deploy queued", message: "The environment is being deployed." });
    },
    onError: (err) =>
      toast({ tone: "error", title: "Preview not deployed", message: errorMessage(err) }),
  });

  const cleanupMutation = useMutation({
    mutationFn: (id: string) => cleanupPreview(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments"] });
      toast({ tone: "success", title: "Preview cleaned up", message: "The environment has been torn down." });
    },
    onError: (err) =>
      toast({ tone: "error", title: "Cleanup failed", message: errorMessage(err) }),
  });

  const previews = useMemo(
    () => (Array.isArray(previewsQuery.data) ? previewsQuery.data : []),
    [previewsQuery.data],
  );

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return previews.filter((p) => {
      if (statusFilter && p.status !== statusFilter) return false;
      if (!needle) return true;
      return `${p.prNumber} ${p.prTitle ?? ""} ${p.branch ?? ""} ${p.repoOwner ?? ""}/${p.repoName ?? ""}`
        .toLowerCase()
        .includes(needle);
    });
  }, [previews, statusFilter, search]);

  const hasFilters = Boolean(statusFilter || search);
  const known = previewsQuery.data !== undefined;

  return (
    <AdminPageLayout>
      <AdminPageHeader
        status={<FreshnessBadge state={sourceState(previewsQuery, POLL_MS)} />}
        action={
          <Btn tone="ghost" size="sm" onClick={() => router.push("/admin/preview-environments")}>
            Preview Environments
          </Btn>
        }
      />

      <AdminToolbar>
        <Input
          label="Search previews"
          placeholder="PR number, title, branch or repository"
          value={search}
          onChange={(v) => setParam("q", v)}
        />
        <AdminSelect
          label="Status"
          value={statusFilter}
          onChange={(v) => setParam("status", v)}
          placeholder="All statuses"
          options={STATUSES.map((s) => ({ value: s, label: statusLabel(s) }))}
        />
        {hasFilters ? (
          <Btn tone="ghost" size="sm" onClick={clearFilters}>
            Clear filters
          </Btn>
        ) : null}
      </AdminToolbar>

      <AdminSection
        title="Preview deployments"
        description={
          known
            ? hasFilters
              ? `${filtered.length.toLocaleString()} of ${previews.length.toLocaleString()} preview deployments match these filters.`
              : `${previews.length.toLocaleString()} preview deployment${previews.length === 1 ? "" : "s"} recorded on this control plane.`
            : "Count not loaded."
        }
        action={previewsQuery.isFetching ? <span className="text-meta text-text-subtle">Refreshing…</span> : null}
      >
        <Card>
          {previewsQuery.isPending ? (
            <AdminLoadingRows cols={5} rows={4} label="Loading preview deployments…" />
          ) : previewsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Preview deployments could not be loaded: ${errorMessage(previewsQuery.error)}`}
                retry={() => void previewsQuery.refetch()}
              />
            </div>
          ) : previews.length === 0 ? (
            <EmptyState
              icon={GitPullRequest}
              title="No preview deployments"
              message="No pull-request preview has been recorded yet. They are created by the preview webhook or by the project-scoped Preview Environments surface."
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={GitPullRequest}
              title="No preview matches these filters"
              message="Every recorded preview was filtered out. Clear the filters above to see all of them."
            />
          ) : (
            <AdminTable label="Preview deployments">
              <AdminTHead>
                <AdminTh>PR</AdminTh>
                <AdminTh>Title</AdminTh>
                <AdminTh>Branch</AdminTh>
                <AdminTh>Source</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Preview URL</AdminTh>
                <AdminTh>Created</AdminTh>
                <AdminTh>Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {filtered.map((p) => {
                  const busy =
                    (deployMutation.isPending && deployMutation.variables === p.id) ||
                    (cleanupMutation.isPending && cleanupMutation.variables === p.id);
                  const blocked = deployReason(p);
                  const cleanupBlocked = cleanupReason(p);
                  return (
                    <AdminTr key={p.id}>
                      <AdminTd className="font-mono text-meta">
                        <Link
                          className="underline decoration-line-strong underline-offset-2 hover:text-text"
                          href={`/admin/preview-deployments/${encodeURIComponent(p.id)}`}
                        >
                          #{p.prNumber.toLocaleString()}
                          <span className="sr-only"> open this preview deployment</span>
                        </Link>
                      </AdminTd>
                      <AdminTd className="max-w-56 text-sm">
                        {p.prTitle ? (
                          <span className="block break-words">{p.prTitle}</span>
                        ) : (
                          <span className="text-text-muted">No title recorded</span>
                        )}
                        {p.repoOwner ? (
                          <span className="mt-0.5 block font-mono text-meta text-text-muted">
                            {p.repoOwner}/{p.repoName}
                          </span>
                        ) : null}
                      </AdminTd>
                      <AdminTd className="font-mono text-meta">
                        {p.branch || <span className="text-text-muted">Not reported</span>}
                      </AdminTd>
                      <AdminTd>
                        <Pill tone="neutral">{statusLabel(p.source)}</Pill>
                      </AdminTd>
                      <AdminTd>
                        <Pill tone={previewStatusTone(p.status)}>{statusLabel(p.status)}</Pill>
                      </AdminTd>
                      <AdminTd className="text-meta">
                        {p.previewUrl ? (
                          <a
                            className="inline-flex items-center gap-1 text-brand underline underline-offset-2"
                            href={p.previewUrl}
                            rel="noreferrer noopener"
                            target="_blank"
                          >
                            <ExternalLink aria-hidden="true" size={12} /> Open preview
                          </a>
                        ) : (
                          <span className="text-text-muted">Not available yet</span>
                        )}
                      </AdminTd>
                      <AdminTd className="whitespace-nowrap text-meta">{timeOrDash(p.createdAt)}</AdminTd>
                      <AdminTd>
                        <div className="flex flex-wrap items-center gap-1">
                          <Btn
                            ariaLabel={`Deploy preview for PR ${p.prNumber}`}
                            disabled={Boolean(blocked) || busy}
                            loading={busy && deployMutation.variables === p.id}
                            onClick={() => deployMutation.mutate(p.id)}
                            size="sm"
                            tone="primary"
                          >
                            <Play aria-hidden="true" size={12} /> Deploy
                          </Btn>
                          <Btn
                            ariaLabel={`Clean up preview for PR ${p.prNumber}`}
                            disabled={Boolean(cleanupBlocked) || busy}
                            loading={busy && cleanupMutation.variables === p.id}
                            onClick={() => void askCleanup(p)}
                            size="sm"
                            tone="danger"
                          >
                            <Trash2 aria-hidden="true" size={12} /> Cleanup
                          </Btn>
                          {p.prUrl ? (
                            <a
                              className="inline-flex items-center gap-1 rounded-lg px-2 py-1 text-meta text-text-subtle underline decoration-line-strong underline-offset-2 hover:text-text"
                              href={p.prUrl}
                              rel="noreferrer noopener"
                              target="_blank"
                            >
                              <ExternalLink aria-hidden="true" size={12} /> Pull request
                            </a>
                          ) : null}
                        </div>
                        {blocked || cleanupBlocked ? (
                          <p className="mt-1 text-meta text-text-muted">{blocked ?? cleanupBlocked}</p>
                        ) : null}
                      </AdminTd>
                    </AdminTr>
                  );
                })}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      </AdminSection>

      {renderConfirm()}
    </AdminPageLayout>
  );

  async function askCleanup(preview: PreviewDeployment) {
    const ok = await confirm({
      confirmLabel: "Clean up preview",
      danger: true,
      description: `The running environment for PR #${preview.prNumber}${
        preview.branch ? ` (${preview.branch})` : ""
      } is destroyed and its preview URL stops resolving.`,
      title: "Clean up this preview?",
    });
    if (ok) cleanupMutation.mutate(preview.id);
  }
}
