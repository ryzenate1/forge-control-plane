"use client";

import { use, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, GitBranch, GitCommit,
  RefreshCw, Copy, Check, Globe, Link,
} from "lucide-react";
import {
  fetchApp, fetchAppGitSource, updateAppGitBranch,
  toggleAppAutoDeploy, triggerDeploy,
} from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";
import { formatDate } from "@/lib/utils";

export default function GitSourcePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const qc = useQueryClient();

  const { data: app, isLoading: appLoading } = useQuery({
    queryKey: ["app", id],
    queryFn: () => fetchApp(id),
  });

  const { data: gitSource, isLoading: gitLoading } = useQuery({
    queryKey: ["app-git", id],
    queryFn: () => fetchAppGitSource(id),
  });

  const [newBranch, setNewBranch] = useState("");
  const [webhookCopied, setWebhookCopied] = useState(false);

  const branchMut = useMutation({
    mutationFn: () => updateAppGitBranch(id, newBranch),
    onSuccess: () => {
      setNewBranch("");
      void qc.invalidateQueries({ queryKey: ["app-git", id] });
    },
  });

  const autoDeployMut = useMutation({
    mutationFn: (enabled: boolean) => toggleAppAutoDeploy(id, enabled),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app-git", id] }),
  });

  const triggerMut = useMutation({
    mutationFn: () => triggerDeploy(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
  });

  const copyWebhook = () => {
    if (gitSource?.webhookUrl) {
      navigator.clipboard.writeText(gitSource.webhookUrl).catch(() => {});
      setWebhookCopied(true);
      setTimeout(() => setWebhookCopied(false), 2000);
    }
  };

  if (appLoading || gitLoading) {
    return (
      <div className="space-y-6">
        <SectionHeader title="Git Source" sub="Loading..." />
        <div className="p-8 text-center text-sm text-slate-500">Loading Git source details...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Btn tone="ghost" size="sm" onClick={() => router.push(`/admin/apps/${id}`)}>
          <ArrowLeft size={14} />
        </Btn>
        <SectionHeader
          title={app?.name ? `${app.name} - Git Source` : "Git Source"}
          sub="Repository configuration and deployment triggers"
        />
      </div>

      <div className="flex flex-wrap gap-3">
        <Btn tone="primary" onClick={() => triggerMut.mutate()} disabled={triggerMut.isPending}>
          <RefreshCw size={14} className={triggerMut.isPending ? "animate-spin" : ""} />
          {triggerMut.isPending ? "Building..." : "Trigger Build"}
        </Btn>
        {triggerMut.isPending && (
          <span className="flex items-center gap-2 text-xs text-slate-400">
            <span className="h-2 w-2 animate-pulse rounded-full bg-blue-500" />
            Build in progress...
          </span>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Repository Info" icon={Globe} />
          <div className="divide-y divide-white/[0.06] text-sm">
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">URL</span>
              <a
                href={gitSource?.repoUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="font-mono text-xs text-blue-400 hover:text-blue-300"
              >
                {gitSource?.repoUrl ?? "—"}
              </a>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Branch</span>
              <span className="font-mono text-xs text-slate-200">
                <GitBranch size={12} className="inline mr-1" />
                {gitSource?.branch ?? app?.gitBranch ?? "—"}
              </span>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Provider</span>
              <Pill tone="blue">{gitSource?.provider ?? app?.gitProvider ?? "—"}</Pill>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Auto-deploy</span>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={gitSource?.autoDeploy ?? false}
                  onChange={(e) => autoDeployMut.mutate(e.target.checked)}
                  className="h-3 w-3 rounded border-white/20 bg-[#161b28] accent-[#dc2626]"
                />
                <span className={cn("text-xs", gitSource?.autoDeploy ? "text-emerald-400" : "text-slate-500")}>
                  {gitSource?.autoDeploy ? "Enabled" : "Disabled"}
                </span>
              </label>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Webhook URL" icon={Link} />
          <div className="space-y-3 p-4">
            <p className="text-xs text-slate-500">
              Configure this URL in your Git provider to trigger automatic deployments.
            </p>
            <div className="flex items-center gap-2">
              <code className="flex-1 break-all rounded-lg border border-white/[0.06] bg-[#0a0e14] p-2 font-mono text-xs text-slate-400">
                {gitSource?.webhookUrl ?? "Waiting for webhook URL..."}
              </code>
              <Btn tone="ghost" size="sm" onClick={copyWebhook} disabled={!gitSource?.webhookUrl}>
                {webhookCopied ? <Check size={14} className="text-emerald-400" /> : <Copy size={14} />}
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
          <div className="px-4 pb-3 text-sm text-red-400">{branchMut.error.message}</div>
        )}
      </Card>

      <Card>
        <CardHeader title={`Recent Commits`} icon={GitCommit} />
        {!gitSource?.commits || gitSource.commits.length === 0 ? (
          <EmptyState icon={GitCommit} message="No commits found." />
        ) : (
          <div className="divide-y divide-white/[0.04]">
            {gitSource.commits.slice(0, 20).map((commit) => (
              <div key={commit.sha} className="flex items-start gap-3 px-4 py-3">
                <GitCommit size={14} className="mt-0.5 shrink-0 text-slate-500" />
                <div className="min-w-0 flex-1">
                  <p className="text-sm text-slate-200 truncate">{commit.message}</p>
                  <p className="mt-0.5 text-xs text-slate-500">
                    <span className="font-mono text-slate-600">{commit.sha.slice(0, 7)}</span>
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
                    className="shrink-0 text-xs text-blue-400 hover:text-blue-300"
                  >
                    View
                  </a>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
