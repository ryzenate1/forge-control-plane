"use client";

import { useQuery } from "@tanstack/react-query";
import {
  SpinnerInline,
  EmptyGit,
  ErrorAlert,
  BuildStatus,
} from "@/components/shared";
import type { ReactNode } from "react";

type GitInfo = {
  provider: string;
  repository: string;
  branch: string;
  lastCommit?: string;
  lastCommitMessage?: string;
  lastBuildStatus?: string;
};

interface GitViewProps {
  appId: string;
  action?: ReactNode;
}

export function GitView({ appId, action }: GitViewProps) {
  const query = useQuery<GitInfo>({
    queryKey: ["app-git", appId],
    queryFn: async () => {
      const res = await fetch(`/api/v1/servers/${encodeURIComponent(appId)}/git`, {
        credentials: "include",
      });
      if (!res.ok) throw new Error(`Failed to load git info: ${res.status}`);
      return res.json() as Promise<GitInfo>;
    },
    enabled: Boolean(appId),
  });

  if (query.isLoading) return <SpinnerInline label="Loading source configuration…" />;
  if (query.isError) {
    return <ErrorAlert error={query.error} title="Failed to load source" onRetry={() => void query.refetch()} />;
  }

  const git = query.data;

  if (!git) {
    return <EmptyGit action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">Source Repository</span>
        {git.lastBuildStatus ? (
          <BuildStatus status={git.lastBuildStatus} />
        ) : null}
      </div>
      <div className="space-y-4 p-6">
        <div className="grid gap-4 md:grid-cols-3">
          <div>
            <p className="text-xs text-slate-500">Provider</p>
            <p className="text-sm font-semibold text-slate-200">{git.provider}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500">Repository</p>
            <p className="text-sm font-semibold text-slate-200">{git.repository}</p>
          </div>
          <div>
            <p className="text-xs text-slate-500">Branch</p>
            <p className="text-sm font-semibold text-slate-200">{git.branch}</p>
          </div>
        </div>
        {git.lastCommit ? (
          <div className="rounded-lg border border-white/[0.06] bg-[#161b28] p-4">
            <p className="text-xs text-slate-500">Last commit</p>
            <p className="mt-1 font-mono text-sm text-slate-300">{git.lastCommit.slice(0, 7)}</p>
            {git.lastCommitMessage ? (
              <p className="mt-1 text-sm text-slate-400">{git.lastCommitMessage}</p>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}
