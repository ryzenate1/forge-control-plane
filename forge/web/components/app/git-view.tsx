"use client";

import { useQuery } from "@tanstack/react-query";
import {
  SpinnerInline,
  EmptyGit,
  ErrorAlert,
  BuildStatus,
} from "@/components/shared";
import { fetchAppGitSource } from "@/lib/api/apps";
import type { ReactNode } from "react";

interface GitViewProps {
  appId: string;
  action?: ReactNode;
}

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
  return sourceConfig as GitConfig;
}

export function GitView({ appId, action }: GitViewProps) {
  const query = useQuery({
    queryKey: ["app-git", appId],
    queryFn: () => fetchAppGitSource(appId),
    enabled: Boolean(appId),
  });

  if (query.isLoading) return <SpinnerInline label="Loading source configuration…" />;
  if (query.isError) {
    return <ErrorAlert error={query.error} title="Failed to load source" onRetry={() => void query.refetch()} />;
  }

  const git = query.data ? parseGitConfig(query.data.sourceConfig) : null;

  if (!git) {
    return <EmptyGit action={action} />;
  }

  const lastCommit = Array.isArray(git.commits) ? git.commits[0] : null;

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">Source Repository</span>
        {git.autoDeploy ? (
          <BuildStatus status="active" />
        ) : null}
      </div>
      <div className="space-y-4 p-6">
        <div className="grid gap-4 md:grid-cols-3">
          <div>
            <p className="text-xs text-slate-500">Provider</p>
            <p className="text-sm font-semibold text-slate-200">{git.provider ?? "—"}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500">Repository</p>
            <p className="text-sm font-semibold text-slate-200">{git.repoUrl ?? "—"}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500">Branch</p>
            <p className="text-sm font-semibold text-slate-200">{git.branch ?? "—"}</p>
          </div>
        </div>
        {lastCommit ? (
          <div className="rounded-lg border border-white/[0.06] bg-[var(--surface-input)] p-4">
            <p className="text-xs text-slate-500">Last commit</p>
            <p className="mt-1 font-mono text-sm text-slate-300">{lastCommit.sha.slice(0, 7)}</p>
            {lastCommit.message ? (
              <p className="mt-1 text-sm text-slate-400">{lastCommit.message}</p>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}
