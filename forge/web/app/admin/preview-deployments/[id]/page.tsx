"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useToast } from "@/components/ui/toast";
import { useParams, useRouter } from "next/navigation";
import { ExternalLink, GitPullRequest, Globe, Play, Trash2 } from "lucide-react";
import { cleanupPreview, deployPreview, fetchPreviewDeployment } from "@/lib/api/preview-deployments";
import { statusLabel } from "@/lib/api/apps";
import { previewStatusTone } from "@/lib/api/status";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  Btn,
  EmptyState,
  Pill,
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { errorMessage, formatDate } from "@/lib/utils";

const POLL_MS = 10_000;

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-meta uppercase tracking-wider text-text-muted">{label}</dt>
      <dd className="mt-1 break-words text-sm text-text">{children}</dd>
    </div>
  );
}

export default function AdminPreviewDeploymentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const id = params.id as string;

  const query = useQuery({
    queryKey: ["admin", "preview-deployments", id],
    queryFn: () => fetchPreviewDeployment(id),
    enabled: Boolean(id),
    refetchInterval: POLL_MS,
  });

  const deployMutation = useMutation({
    mutationFn: () => deployPreview(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments", id] });
      toast({ tone: "success", title: "Preview deploy queued", message: "The environment is being deployed." });
    },
    onError: (err) => toast({ tone: "error", title: "Preview not deployed", message: errorMessage(err) }),
  });

  const cleanupMutation = useMutation({
    mutationFn: () => cleanupPreview(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "preview-deployments", id] });
      toast({ tone: "success", title: "Preview cleaned up", message: "The environment has been torn down." });
    },
    onError: (err) => toast({ tone: "error", title: "Cleanup failed", message: errorMessage(err) }),
  });

  const p = query.data;

  // A header with an <h1> is rendered in every branch: the loading state used to
  // be a bare <p> with no heading at all.
  const heading = (
    <AdminPageHeader
      backAction={() => router.push("/admin/preview-deployments")}
      backLabel="Preview Deployments"
      description={p ? `PR #${p.prNumber}${p.branch ? ` · ${p.branch}` : ""}` : undefined}
      status={p ? <FreshnessBadge state={sourceState(query, POLL_MS)} /> : undefined}
      title={p ? `Preview: PR #${p.prNumber}` : "Preview deployment"}
      action={
        p
          ? (() => {
              // `POST /:id/deploy` only acts while the row is `deploying`
              // (`previewenv/service.go:311-321`), so the control stays visible and
              // disabled with that reason instead of vanishing.
              const blocked =
                p.status === "running"
                  ? "Already deployed."
                  : p.status === "cleaned_up"
                    ? "Cleaned up — nothing left to deploy."
                    : p.status !== "deploying"
                      ? `The control plane only starts a preview from “deploying”; this one is “${statusLabel(p.status)}”.`
                      : undefined;
              return (
                <div className="flex flex-wrap items-center gap-2">
                  {p.prUrl ? (
                    <a
                      className="inline-flex items-center gap-1.5 rounded-lg border border-line px-3 py-2 text-sm text-text transition-colors hover:border-line-strong hover:bg-overlay-strong"
                      href={p.prUrl}
                      rel="noreferrer noopener"
                      target="_blank"
                    >
                      <ExternalLink aria-hidden="true" size={14} /> Open pull request
                    </a>
                  ) : null}
                  <Btn
                    disabled={Boolean(blocked) || deployMutation.isPending}
                    loading={deployMutation.isPending}
                    onClick={() => deployMutation.mutate()}
                    tone="primary"
                  >
                    <Play aria-hidden="true" size={14} /> Deploy
                  </Btn>
                  <Btn
                    disabled={p.status === "cleaned_up" || cleanupMutation.isPending}
                    loading={cleanupMutation.isPending}
                    onClick={() => void askCleanup()}
                    tone="danger"
                  >
                    <Trash2 aria-hidden="true" size={14} /> Cleanup
                  </Btn>
                  {blocked || p.status === "cleaned_up" ? (
                    <span className="text-meta text-text-muted">
                      {blocked ?? "Already cleaned up."}
                    </span>
                  ) : null}
                </div>
              );
            })()
          : undefined
      }
    />
  );

  if (query.isPending) {
    return (
      <AdminPageLayout>
        {heading}
        <AdminLoadingState label="Loading this preview deployment…" />
      </AdminPageLayout>
    );
  }

  if (query.isError) {
    return (
      <AdminPageLayout>
        {heading}
        <AdminErrorState
          message={`This preview deployment could not be read: ${errorMessage(query.error)}`}
          retry={() => void query.refetch()}
        />
      </AdminPageLayout>
    );
  }

  if (!p) {
    return (
      <AdminPageLayout>
        {heading}
        <EmptyState
          icon={GitPullRequest}
          message="The control plane answered this request and returned no preview deployment for this id."
          title="No preview deployment with this id"
        />
      </AdminPageLayout>
    );
  }

  return (
    <AdminPageLayout className="max-w-5xl">
      {heading}
      {renderConfirm()}

      <AdminSection title="State" description="As the control plane recorded it, refreshed on every poll.">
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="rounded-xl border border-line bg-overlay-subtle p-4">
            <p className="flex items-center gap-2 text-meta uppercase tracking-wider text-text-muted">
              <Globe aria-hidden="true" size={12} /> Status
            </p>
            {/* One tone source: this page had a fourth private ternary that drew
                `deploying` blue here and grey in the list. */}
            <Pill className="mt-2" tone={previewStatusTone(p.status)}>
              {statusLabel(p.status)}
            </Pill>
          </div>
          <div className="rounded-xl border border-line bg-overlay-subtle p-4">
            <p className="text-meta uppercase tracking-wider text-text-muted">Pull request</p>
            <p className="mt-1 font-mono text-sm text-text">#{p.prNumber.toLocaleString()}</p>
            <p className="mt-1 break-words text-meta text-text-subtle">
              {p.prTitle || "No title recorded."}
            </p>
          </div>
          <div className="rounded-xl border border-line bg-overlay-subtle p-4">
            <p className="text-meta uppercase tracking-wider text-text-muted">Preview URL</p>
            {p.previewUrl ? (
              <a
                className="mt-1 block break-all text-sm text-brand underline underline-offset-2"
                href={p.previewUrl}
                rel="noreferrer noopener"
                target="_blank"
              >
                {p.previewUrl}
              </a>
            ) : (
              <p className="mt-1 text-sm text-text-muted">Not available yet</p>
            )}
          </div>
        </div>
      </AdminSection>

      <AdminSection title="Record">
        <dl className="grid grid-cols-2 gap-4 p-4 sm:grid-cols-4">
          <Fact label="Branch">{p.branch || "Not reported"}</Fact>
          <Fact label="Commit">
            {p.commitSha ? <span className="font-mono">{p.commitSha}</span> : "Not reported"}
          </Fact>
          <Fact label="Repository">
            {p.repoOwner ? `${p.repoOwner}/${p.repoName ?? "—"}` : "Not reported"}
          </Fact>
          <Fact label="Source">
            <Pill tone="neutral">{statusLabel(p.source)}</Pill>
          </Fact>
          <Fact label="URL suffix">{p.uniqueSuffix ? <span className="font-mono">{p.uniqueSuffix}</span> : "Not reported"}</Fact>
          <Fact label="Isolated environment">
            {/* A recorded boolean is a fact, so "No" is honest here; an absent one
                is not. */}
            {typeof p.isIsolated === "boolean" ? (p.isIsolated ? "Yes" : "No") : "Not reported"}
          </Fact>
          <Fact label="Server id">{p.serverId ? <span className="font-mono">{p.serverId}</span> : "Not reported"}</Fact>
          <Fact label="Created by">{p.createdBy || "Not reported"}</Fact>
          <Fact label="Created">{p.createdAt ? formatDate(p.createdAt) : "—"}</Fact>
          <Fact label="Updated">{p.updatedAt ? formatDate(p.updatedAt) : "—"}</Fact>
          <Fact label="Cleaned up">{p.cleanedAt ? formatDate(p.cleanedAt) : "Not cleaned up"}</Fact>
          <Fact label="Deployment">{p.deploymentUrl ? (
            <a className="break-all text-brand underline underline-offset-2" href={p.deploymentUrl} rel="noreferrer noopener" target="_blank">
              Open deployment record
            </a>
          ) : (
            "No deployment URL recorded"
          )}</Fact>
        </dl>
      </AdminSection>
    </AdminPageLayout>
  );

  async function askCleanup() {
    if (!p) return;
    const ok = await confirm({
      confirmLabel: "Clean up preview",
      danger: true,
      description: `The running environment for PR #${p.prNumber}${p.branch ? ` (${p.branch})` : ""} is destroyed and its preview URL stops resolving.`,
      title: "Clean up this preview?",
    });
    if (ok) cleanupMutation.mutate();
  }
}
