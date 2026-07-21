"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import { useToast } from "@/components/ui/toast";
import { getSourceDeployment, deploySourceDeployment, cancelSourceDeployment, deleteSourceDeployment, getDeploymentBuildLogs, type SourceDeployment, type BuildLog } from "@/lib/api/source-deployments";
import { Play, XCircle, Trash2, GitBranch, Github, ArrowLeft, RefreshCw, Loader2, CheckCircle, Clock, AlertTriangle } from "lucide-react";
import { useEffect, useRef } from "react";

const statusColors: Record<string, string> = {
  pending: "text-yellow-400",
  queued: "text-blue-400",
  cloning: "text-blue-400",
  building: "text-purple-400",
  pushing: "text-purple-400",
  deploying: "text-indigo-400",
  healthy: "text-green-400",
  completed: "text-green-400",
  failed: "text-red-400",
  canceled: "text-gray-400",
  unhealthy: "text-orange-400",
};

function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`text-xs px-2 py-1 rounded-full capitalize ${statusColors[status] || ''} bg-current/10`}>
      {status}
    </span>
  );
}

export default function SourceDeploymentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const logsEndRef = useRef<HTMLDivElement>(null);
  const id = params.id as string;

  const { data: deployment, isLoading, refetch } = useQuery({
    queryKey: ["sourceDeployment", id],
    queryFn: () => getSourceDeployment(id),
    refetchInterval: (query) => {
      const d = query.state.data;
      if (d && !["completed", "failed", "canceled", "healthy", "unhealthy"].includes(d.status)) {
        return 3000;
      }
      return false;
    },
  });

  const { data: logs } = useQuery({
    queryKey: ["buildLogs", id],
    queryFn: () => getDeploymentBuildLogs(id),
    refetchInterval: (query) => {
      const d = queryClient.getQueryData<SourceDeployment>(["sourceDeployment", id]);
      if (d && !["completed", "failed", "canceled", "healthy", "unhealthy"].includes(d.status)) {
        return 3000;
      }
      return false;
    },
  });

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  const deployMutation = useMutation({
    mutationFn: () => deploySourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployment", id] });
      toast({ title: "Deployment triggered", tone: "success" });
    },
    onError: () => toast({ title: "Deploy failed", tone: "error" }),
  });

  const cancelMutation = useMutation({
    mutationFn: () => cancelSourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployment", id] });
      toast({ title: "Deployment canceled", tone: "success" });
    },
    onError: () => toast({ title: "Cancel failed", tone: "error" }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteSourceDeployment(id),
    onSuccess: () => {
      router.push("/admin/source-deployments");
      toast({ title: "Deployment deleted", tone: "success" });
    },
    onError: () => toast({ title: "Delete failed", tone: "error" }),
  });

  if (isLoading) {
    return <div className="p-6 text-center opacity-50">Loading deployment...</div>;
  }

  if (!deployment) {
    return (
      <div className="p-6 text-center">
        <p className="opacity-60">Deployment not found.</p>
        <button onClick={() => router.push("/admin/source-deployments")} className="btn btn-ghost mt-4">
          <ArrowLeft className="w-4 h-4 mr-2" /> Back
        </button>
      </div>
    );
  }

  const isActive = !["completed", "failed", "canceled", "healthy", "unhealthy"].includes(deployment.status);

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <button
        onClick={() => router.push("/admin/source-deployments")}
        className="btn btn-ghost mb-4"
      >
        <ArrowLeft className="w-4 h-4 mr-2" /> Back to Deployments
      </button>

      <div className="card p-6 mb-6">
        <div className="flex items-start justify-between mb-4">
          <div>
            <h1 className="text-2xl font-bold flex items-center gap-2">
              <GitBranch className="w-5 h-5" />
              {deployment.repository.split('/').pop()?.replace('.git', '')}
            </h1>
            <div className="flex items-center gap-2 mt-1 text-sm opacity-70">
              <span>{deployment.repository}</span>
              <span className="opacity-40">|</span>
              <span>{deployment.branch}</span>
              <StatusBadge status={deployment.status} />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={() => refetch()} className="btn btn-ghost btn-sm" title="Refresh">
              <RefreshCw className="w-4 h-4" />
            </button>
            {isActive && (
              <button
                onClick={() => cancelMutation.mutate()}
                className="btn btn-ghost btn-sm"
                disabled={cancelMutation.isPending}
              >
                <XCircle className="w-4 h-4 mr-1" /> Cancel
              </button>
            )}
            <button
              onClick={() => deployMutation.mutate()}
              className="btn btn-primary btn-sm"
              disabled={deployMutation.isPending}
            >
              <Play className="w-4 h-4 mr-1" /> Deploy
            </button>
            <button
              onClick={() => deleteMutation.mutate()}
              className="btn btn-ghost btn-sm text-red-400"
              disabled={deleteMutation.isPending}
            >
              <Trash2 className="w-4 h-4" />
            </button>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-4 text-sm">
          <div>
            <span className="opacity-50">Build Type:</span>{" "}
            <span className="capitalize">{deployment.buildType}</span>
          </div>
          <div>
            <span className="opacity-50">Build Context:</span>{" "}
            <span>{deployment.buildContext}</span>
          </div>
          {deployment.dockerfilePath && (
            <div>
              <span className="opacity-50">Dockerfile Path:</span>{" "}
              <span>{deployment.dockerfilePath}</span>
            </div>
          )}
          <div>
            <span className="opacity-50">Auto Deploy:</span>{" "}
            <span>{deployment.autoDeploy ? "Enabled" : "Disabled"}</span>
          </div>
          {deployment.commitHash && (
            <div className="col-span-2">
              <span className="opacity-50">Commit:</span>{" "}
              <code className="text-xs bg-black/20 px-1 py-0.5 rounded">{deployment.commitHash.substring(0, 8)}</code>
              {deployment.commitMessage && <span className="ml-2">{deployment.commitMessage}</span>}
            </div>
          )}
          {deployment.imageTag && (
            <div className="col-span-2">
              <span className="opacity-50">Image:</span>{" "}
              <code className="text-xs bg-black/20 px-1 py-0.5 rounded">{deployment.imageTag}</code>
            </div>
          )}
        </div>
      </div>

      <div className="card">
        <div className="p-4 border-b border-white/10 flex items-center justify-between">
          <h2 className="font-semibold">Build Logs</h2>
          {isActive && <Loader2 className="w-4 h-4 animate-spin text-blue-400" />}
        </div>
        <div className="p-4 max-h-96 overflow-y-auto font-mono text-xs">
          {logs && logs.length > 0 ? (
            <>
              {logs.map((log: BuildLog) => (
                <div key={log.id} className="py-1 flex gap-2">
                  <span className="text-blue-400 shrink-0">[{log.stage}]</span>
                  <span className="opacity-70 shrink-0">{new Date(log.createdAt).toLocaleTimeString()}</span>
                  <span>{log.message}</span>
                </div>
              ))}
              <div ref={logsEndRef} />
            </>
          ) : (
            <div className="text-center py-8 opacity-40">
              {isActive ? "Waiting for build logs..." : "No build logs available."}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
