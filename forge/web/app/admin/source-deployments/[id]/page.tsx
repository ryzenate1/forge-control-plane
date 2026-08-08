"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import { useToast } from "@/components/ui/toast";
import { getSourceDeployment, deploySourceDeployment, cancelSourceDeployment, deleteSourceDeployment, getDeploymentBuildLogs, type SourceDeployment, type BuildLog } from "@/lib/api/source-deployments";
import { Play, XCircle, Trash2, GitBranch, ArrowLeft, RefreshCw, Loader2 } from "lucide-react";
import { useEffect, useRef, useMemo } from "react";
import { AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, Pill, cn } from "@/components/admin/admin-ui";



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
    refetchInterval: () => {
      const d = queryClient.getQueryData<SourceDeployment>(["sourceDeployment", id]);
      if (d && !["completed", "failed", "canceled", "healthy", "unhealthy"].includes(d.status)) {
        return 3000;
      }
      return false;
    },
  });

  const safeLogs = useMemo(() => Array.isArray(logs) ? logs : [], [logs]);

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
    return <AdminPageLayout><div className="p-6 text-center text-slate-400">Loading deployment...</div></AdminPageLayout>;
  }

  if (!deployment) {
    return (
      <AdminPageLayout>
        <div className="p-6 text-center">
          <p className="opacity-60">Deployment not found.</p>
          <Btn tone="ghost" onClick={() => router.push("/admin/source-deployments")}>
            <ArrowLeft className="w-4 h-4 mr-2" /> Back
          </Btn>
        </div>
      </AdminPageLayout>
    );
  }

  const isActive = !["completed", "failed", "canceled", "healthy", "unhealthy"].includes(deployment.status);

  const toneMap: Record<string, "green" | "red" | "neutral" | "yellow"> = {
    healthy: "green", completed: "green", failed: "red", canceled: "neutral",
    pending: "yellow",
  };

  const logStageColor = (stage: string) => {
    if (stage === "error") return "text-red-400";
    if (stage === "warning") return "text-amber-400";
    if (stage === "success") return "text-emerald-400";
    return "text-slate-400";
  };

  return (
    <AdminPageLayout className="max-w-4xl">
      <AdminPageHeader
        title={deployment.repository.split('/').pop()?.replace('.git', '') ?? "Deployment"}
        description={`${deployment.repository} | ${deployment.branch}`}
        backAction={() => router.push("/admin/source-deployments")}
        backLabel="Source Deployments"
        action={
          <div className="flex items-center gap-2">
            <Btn size="sm" tone="ghost" onClick={() => refetch()} ariaLabel="Refresh"><RefreshCw className="w-4 h-4" /></Btn>
            <Pill tone={toneMap[deployment.status] ?? "neutral"}>{deployment.status}</Pill>
            {isActive && (
              <Btn size="sm" tone="ghost" onClick={() => cancelMutation.mutate()} disabled={cancelMutation.isPending}>
                <XCircle className="w-4 h-4" /> Cancel
              </Btn>
            )}
            <Btn size="sm" tone="primary" onClick={() => deployMutation.mutate()} disabled={deployMutation.isPending}>
              <Play className="w-4 h-4" /> Deploy
            </Btn>
            <Btn size="sm" tone="danger" onClick={() => deleteMutation.mutate()} disabled={deleteMutation.isPending}>
              <Trash2 className="w-4 h-4" />
            </Btn>
          </div>
        }
      />

      <Card>
        <CardHeader title="Details" icon={GitBranch} />
        <div className="grid grid-cols-2 gap-4 p-4 text-sm">
          <div>
            <span className="text-slate-400">Build Type:</span>{" "}
            <span className="capitalize text-slate-200">{deployment.buildType}</span>
          </div>
          <div>
            <span className="text-slate-400">Build Context:</span>{" "}
            <span className="text-slate-200">{deployment.buildContext}</span>
          </div>
          {deployment.dockerfilePath && (
            <div>
              <span className="text-slate-400">Dockerfile Path:</span>{" "}
              <span className="text-slate-200">{deployment.dockerfilePath}</span>
            </div>
          )}
          <div>
            <span className="text-slate-400">Auto Deploy:</span>{" "}
            <span className="text-slate-200">{deployment.autoDeploy ? "Enabled" : "Disabled"}</span>
          </div>
          {deployment.commitHash && (
            <div className="col-span-2">
              <span className="text-slate-400">Commit:</span>{" "}
              <code className="text-xs bg-black/20 px-1 py-0.5 rounded text-slate-200">{deployment.commitHash.substring(0, 8)}</code>
              {deployment.commitMessage && <span className="ml-2 text-slate-200">{deployment.commitMessage}</span>}
            </div>
          )}
          {deployment.imageTag && (
            <div className="col-span-2">
              <span className="text-slate-400">Image:</span>{" "}
              <code className="text-xs bg-black/20 px-1 py-0.5 rounded text-slate-200">{deployment.imageTag}</code>
            </div>
          )}
        </div>
      </Card>

      <Card>
        <CardHeader title="Build Logs" icon={Loader2} />
        <div className="p-4 max-h-96 overflow-y-auto font-mono text-xs">
          {safeLogs.length > 0 ? (
            <>
              {safeLogs.map((log: BuildLog) => (
                <div key={log.id} className="py-1 flex gap-2">
                  <span className={cn(logStageColor(log.stage), "shrink-0")}>[{log.stage}]</span>
                  <span className="opacity-70 shrink-0">{new Date(log.createdAt).toLocaleTimeString()}</span>
                  <span className="text-slate-300">{log.message}</span>
                </div>
              ))}
              <div ref={logsEndRef} />
            </>
          ) : (
            <div className="text-center py-8 text-slate-400">
              {isActive ? "Waiting for build logs..." : "No build logs available."}
            </div>
          )}
        </div>
      </Card>
    </AdminPageLayout>
  );
}
