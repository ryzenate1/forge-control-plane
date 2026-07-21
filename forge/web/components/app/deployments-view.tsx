"use client";

import { useQuery } from "@tanstack/react-query";
import { Rocket } from "lucide-react";
import {
  SkeletonList,
  EmptyDeployments,
  ErrorAlert,
  ErrorPermission,
  DeploymentStatus,
} from "@/components/shared";
import { fetchAppDeployments } from "@/lib/api/apps";
import type { AppDeployment } from "@/lib/api/apps";
import { formatDate } from "@/lib/utils";
import type { ReactNode } from "react";

interface DeploymentsViewProps {
  appId: string;
  action?: ReactNode;
  canDeploy?: boolean;
}

export function DeploymentsView({ appId, action, canDeploy = true }: DeploymentsViewProps) {
  const query = useQuery<AppDeployment[]>({
    queryKey: ["app-deployments", appId],
    queryFn: () => fetchAppDeployments(appId),
    enabled: Boolean(appId),
  });

  if (!canDeploy) {
    return <ErrorPermission permission="view deployments" />;
  }

  if (query.isLoading) {
    return <SkeletonList rows={4} columns={3} />;
  }

  if (query.isError) {
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load deployments"
        onRetry={() => void query.refetch()}
      />
    );
  }

  const deployments = query.data ?? [];

  if (!deployments.length) {
    return <EmptyDeployments action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {deployments.length} deployment{deployments.length === 1 ? "" : "s"}
        </span>
        {action ? <div className="ml-auto">{action}</div> : null}
      </div>
      <div className="divide-y divide-white/[0.07]">
        {deployments.map((deploy) => (
          <div className="flex items-center gap-4 px-5 py-4" key={deploy.id}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <Rocket size={16} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <p className="truncate text-sm font-semibold text-slate-200">
                  v{deploy.revision}
                </p>
                <DeploymentStatus status={deploy.status} />
              </div>
              {deploy.commit ? (
                <p className="text-xs text-slate-500">
                  {deploy.commit.slice(0, 7)}
                  {deploy.commitMessage ? ` — ${deploy.commitMessage}` : ""}
                </p>
              ) : null}
              <p className="text-xs text-slate-600">{formatDate(deploy.startedAt)}</p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
