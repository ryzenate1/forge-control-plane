"use client";

import { use, useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  GitBranch, GitCommit,
  RefreshCw, Copy, Check, Globe, Link,
} from "lucide-react";
import {
  fetchApp, fetchAppGitSource, updateAppGitBranch,
  toggleAppAutoDeploy, triggerDeploy,
} from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, AdminErrorState, AdminLoadingState, AdminPageLayout, cn } from "@/components/admin/admin-ui";
import { formatDate } from "@/lib/utils";
import { toast } from "@/components/ui/sonner";
import { useBreadcrumbLabel } from "@/lib/nav/breadcrumb-context";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { queryKeys } from "@/lib/api/query-keys";

interface GitConfig {
  repoUrl?: string;
  branch?: string;
  provider?: string;
  autoDeploy?: boolean;
  webhookUrl?: string;
  commits?: Array<{ sha: string; message: string; author: string; timestamp: string; url?: string }>;
}

function parseGitConfig(sourceConfig: unknown): GitConfig | null {
  if (!sourceConfig || typeof sourceConfig !== "object") return null;
  const cfg = sourceConfig as Record<string, unknown>;
  const str = (v: unknown) => (typeof v === "string" ? v : undefined);
  // Accept both the legacy keys (repoUrl/branch/provider) and the canonical
  // keys the create/update API persists (gitUrl/gitBranch/gitProvider).
  return {
    ...(cfg as object),
    repoUrl: str(cfg.repoUrl) ?? str(cfg.gitUrl),
    branch: str(cfg.branch) ?? str(cfg.gitBranch),
    provider: str(cfg.provider) ?? str(cfg.gitProvider),
  } as GitConfig;
}

export default function GitSourcePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const qc = useQueryClient();

  const { data: app, isLoading: appLoading, isError: appError, refetch: refetchApp } = useQuery({
    queryKey: queryKeys.apps.detail(id),
    queryFn: () => fetchApp(id),
  });

  const { data: gitResp, isLoading: gitLoading } = useQuery({
    queryKey: ["app-git", id],
    queryFn: () => fetchAppGitSource(id),
  });

  const gitSource = useMemo(() => gitResp ? parseGitConfig(gitResp.sourceConfig) : null, [gitResp]);

  const [newBranch, setNewBranch] = useState("");
  const [webhookCopied, setWebhookCopied] = useState(false);

  const branchMut = useMutation({
    mutationFn: () => updateAppGitBranch(id, newBranch),
    onSuccess: () => {
      setNewBranch("");
      void qc.invalidateQueries({ queryKey: ["app-git", id] });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to switch branch"),
  });

  const autoDeployMut = useMutation({
    mutationFn: async (enabled: boolean) => { const result = await toggleAppAutoDeploy(id, enabled); if (!result.ok) throw new Error(`The server reported auto-deploy ${enabled ? "enable" : "disable"} did not complete.`); return result; },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app-git", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to toggle auto-deploy"),
  });

  const triggerMut = useMutation({
    mutationFn: () => triggerDeploy(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.detail(id) }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to trigger deploy"),
  });

  const copyWebhook = () => {
    if (gitSource?.webhookUrl) {
      navigator.clipboard.writeText(gitSource.webhookUrl).catch(() => {});
      setWebhookCopied(true);
      setTimeout(() => setWebhookCopied(false), 2000);
    }
  };

  // The shell renders the one breadcrumb trail; this names its id crumb so
  // it reads as the app rather than a bare uuid. Before the name loads the
  // crumb keeps the id — it does not flash a placeholder.
  useBreadcrumbLabel(id, app?.name ?? null);

  if (appLoading || gitLoading) {
    return (
      <AdminPageLayout>
        <SectionHeader title="Git Source" sub="Loading..." />
        <AdminLoadingState label="Loading Git source details..." />
      </AdminPageLayout>
    );
  }

  if (appError || !app) {
    return (
      <AdminPageLayout>
        <SectionHeader title="Git Source" sub="Repository configuration and deployment triggers" />
        <AdminErrorState message="Could not load this Git source." retry={() => void refetchApp()} />
      </AdminPageLayout>
    );
  }

  return (
    <AdminPageLayout>
      <SectionHeader
        title={app?.name ? `${app.name} · Git Source` : "Git Source"}
        sub="Repository configuration and deployment triggers"
        info={adminPageGuides.applications}
        backAction={() => router.push(`/admin/apps/${id}`)}
        backLabel={app?.name ?? "App"}
        action={
          <div className="flex items-center gap-3">
            {triggerMut.isPending && (
              <span className="flex items-center gap-2 text-xs text-text-subtle">
                <span className="h-2 w-2 animate-pulse rounded-full bg-info" />
                Build in progress...
              </span>
            )}
            <Btn tone="primary" onClick={() => triggerMut.mutate()} disabled={triggerMut.isPending}>
              <RefreshCw size={14} className={triggerMut.isPending ? "animate-spin" : ""} />
              {triggerMut.isPending ? "Building..." : "Trigger Build"}
            </Btn>
          </div>
        }
      />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Repository Info" icon={Globe} />
          <div className="divide-y divide-line text-sm">
            <div className="flex justify-between px-4 py-3">
              <span className="text-text-subtle">URL</span>
              <a
                href={gitSource?.repoUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="font-mono text-xs text-brand hover:text-brand"
              >
                {gitSource?.repoUrl ?? app?.gitRepo ?? "—"}
              </a>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-text-subtle">Branch</span>
              <span className="font-mono text-xs text-text">
                <GitBranch size={12} className="inline mr-1" />
                {gitSource?.branch ?? app?.gitBranch ?? "—"}
              </span>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-text-subtle">Provider</span>
              <Pill tone="blue">{gitSource?.provider ?? app?.gitProvider ?? "—"}</Pill>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-text-subtle">Auto-deploy</span>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={gitSource?.autoDeploy ?? false}
                  onChange={(e) => autoDeployMut.mutate(e.target.checked)}
                  className="h-3 w-3 rounded border-line bg-[var(--surface-input)] accent-[var(--brand)]"
                />
                <span className={cn("text-xs", gitSource?.autoDeploy ? "text-ok" : "text-text-muted")}>
                  {gitSource?.autoDeploy ? "Enabled" : "Disabled"}
                </span>
              </label>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Webhook URL" icon={Link} />
          <div className="space-y-3 p-4">
            <p className="text-xs text-text-muted">
              Configure this URL in your Git provider to trigger automatic deployments.
            </p>
            <div className="flex items-center gap-2">
              <code className="flex-1 break-all rounded-lg border border-line bg-[var(--canvas)] p-2 font-mono text-xs text-text-subtle">
                {gitSource?.webhookUrl ?? "Waiting for webhook URL..."}
              </code>
              <Btn tone="ghost" size="sm" onClick={copyWebhook} disabled={!gitSource?.webhookUrl}>
                {webhookCopied ? <Check size={14} className="text-ok" /> : <Copy size={14} />}
              </Btn>
            </div>
          </div>
        </Card>
      </div>

      <Card>
        <CardHeader
          title="Change Branch"
          icon={GitBranch}
        />
        <div className="flex items-end gap-3 p-4">
          <div className="flex-1">
            <Input
              label="New branch name"
              value={newBranch}
              onChange={setNewBranch}
              placeholder="develop"
            />
          </div>
          <Btn
            tone="primary"
            onClick={() => branchMut.mutate()}
            disabled={!newBranch.trim() || branchMut.isPending}
          >
            {branchMut.isPending ? "Switching..." : "Switch Branch"}
          </Btn>
        </div>
        {branchMut.error && (
          <div className="px-4 pb-3 text-sm text-danger">{branchMut.error.message}</div>
        )}
      </Card>

      <Card>
        <CardHeader title={`Recent Commits`} icon={GitCommit} />
        {!gitSource?.commits || gitSource.commits.length === 0 ? (
          <EmptyState icon={GitCommit} message="No commits found." />
        ) : (
          <div className="divide-y divide-line">
            {gitSource.commits.slice(0, 20).map((commit) => (
              <div key={commit.sha} className="flex items-start gap-3 px-4 py-3">
                <GitCommit size={14} className="mt-0.5 shrink-0 text-text-muted" />
                <div className="min-w-0 flex-1">
                  <p className="text-sm text-text truncate">{commit.message}</p>
                  <p className="mt-0.5 text-xs text-text-muted">
                    <span className="font-mono text-text-muted">{commit.sha.slice(0, 7)}</span>
                    <span className="mx-1">by</span>
                    {commit.author}
                    <span className="mx-1">—</span>
                    {formatDate(commit.timestamp)}
                  </p>
                </div>
                {commit.url && (
                  <a
                    href={commit.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="shrink-0 text-xs text-brand hover:text-brand"
                  >
                    View
                  </a>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>
    </AdminPageLayout>
  );
}
