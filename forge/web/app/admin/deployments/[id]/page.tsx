"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import {
  Activity, ArrowLeft, CheckCircle, Layers, RefreshCw,
  RotateCcw, Server, XOctagon,
} from "lucide-react";
import { fetchJSON, postJSON } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Pill, SectionHeader } from "@/components/admin/admin-ui";

type Deployment = {
  id: string;
  serverId: string;
  image: string;
  strategy: string;
  status: string;
  targetGroup?: string;
  healthCheckPath?: string;
  healthCheckPort?: number;
  createdAt: string;
  completedAt?: string;
  error?: string;
};

type TimelineEvent = {
  id: string;
  type: string;
  message: string;
  timestamp: string;
};

export default function AdminDeploymentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const id = params.id as string;

  const depQuery = useQuery({
    queryKey: ["admin", "deployments", id],
    queryFn: () => fetchJSON<Deployment>(`/admin/deployments/${id}`),
  });

  const timelineQuery = useQuery({
    queryKey: ["admin", "deployments", id, "timeline"],
    queryFn: () => fetchJSON<TimelineEvent[]>(`/admin/deployments/${id}/timeline`),
  });

  const rollbackMutation = useMutation({
    mutationFn: () => postJSON(`/admin/deployments/${id}/rollback`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
    },
  });

  const completeMutation = useMutation({
    mutationFn: () => postJSON(`/admin/deployments/${id}/complete`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
    },
  });

  const cancelMutation = useMutation({
    mutationFn: () => postJSON(`/admin/deployments/${id}/cancel`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
    },
  });

  const dep = depQuery.data;
  const timeline = timelineQuery.data ?? [];

  if (depQuery.isLoading) {
    return <div className="p-8 text-center text-sm text-slate-500">Loading deployment...</div>;
  }

  if (!dep) {
    return <div className="p-8 text-center text-sm text-red-300">Deployment not found.</div>;
  }

  const canRollback = dep.status === "completed";
  const canComplete = dep.status === "in_progress";
  const canCancel = dep.status === "pending" || dep.status === "in_progress";

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        <Btn tone="ghost" onClick={() => router.push("/admin/deployments")}>
          <ArrowLeft size={14} /> Back
        </Btn>
        <div className="flex-1">
          <SectionHeader
            title={`Deployment: ${dep.id.slice(0, 8)}...`}
            sub={`Server ${dep.serverId} — ${dep.strategy.replace("_", "-")} strategy`}
            action={
              <div className="flex gap-2">
                {canRollback && (
                  <Btn tone="warning" onClick={() => rollbackMutation.mutate()} disabled={rollbackMutation.isPending}>
                    <RotateCcw size={14} /> Rollback
                  </Btn>
                )}
                {canComplete && (
                  <Btn tone="success" onClick={() => completeMutation.mutate()} disabled={completeMutation.isPending}>
                    <CheckCircle size={14} /> Complete
                  </Btn>
                )}
                {canCancel && (
                  <Btn tone="danger" onClick={() => cancelMutation.mutate()} disabled={cancelMutation.isPending}>
                    <XOctagon size={14} /> Cancel
                  </Btn>
                )}
              </div>
            }
          />
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <Server size={12} /> Server
          </div>
          <p className="font-mono text-sm font-medium text-slate-200">{dep.serverId}</p>
        </Card>
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <Layers size={12} /> Image
          </div>
          <p className="text-sm font-mono text-slate-200">{dep.image}</p>
        </Card>
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <RefreshCw size={12} /> Status
          </div>
          <Pill tone={
            dep.status === "completed" ? "green" :
            dep.status === "failed" ? "red" :
            dep.status === "in_progress" ? "blue" :
            dep.status === "rolled_back" ? "neutral" : "yellow"
          }>
            {dep.status.replace("_", " ")}
          </Pill>
        </Card>
      </div>

      <Card>
        <CardHeader title="Deployment Details" icon={Layers} />
        <div className="grid grid-cols-2 gap-4 p-4 sm:grid-cols-4">
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Strategy</p>
            <p className="mt-1 text-sm text-slate-200">{dep.strategy.replace("_", "-")}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Target Group</p>
            <p className="mt-1 text-sm text-slate-200">{dep.targetGroup ?? "—"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Health Check Path</p>
            <p className="mt-1 text-sm text-slate-200">{dep.healthCheckPath ?? "—"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Health Check Port</p>
            <p className="mt-1 text-sm text-slate-200">{dep.healthCheckPort ?? "—"}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Created</p>
            <p className="mt-1 text-sm text-slate-200">{new Date(dep.createdAt).toLocaleString()}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Completed</p>
            <p className="mt-1 text-sm text-slate-200">{dep.completedAt ? new Date(dep.completedAt).toLocaleString() : "—"}</p>
          </div>
          {dep.error && (
            <div className="col-span-full rounded-lg border border-red-700/30 bg-red-900/10 p-3">
              <p className="text-[10px] font-semibold uppercase tracking-widest text-red-400">Error</p>
              <p className="mt-1 text-sm text-red-300">{dep.error}</p>
            </div>
          )}
        </div>
      </Card>

      <Card>
        <CardHeader title="Deployment Timeline" icon={Activity} />
        {timelineQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading timeline...</div>
        ) : timeline.length === 0 ? (
          <EmptyState icon={Activity} message="No timeline events recorded." />
        ) : (
          <div className="relative pl-8 pr-4 py-4">
            <div className="absolute left-4 top-0 bottom-0 w-px bg-white/[0.06]" />
            {timeline.map((event) => (
              <div key={event.id} className="relative pb-4 last:pb-0">
                <div className="absolute -left-[19px] mt-1.5 h-2.5 w-2.5 rounded-full border-2 border-[#dc2626] bg-[#1e2536]" />
                <div className="flex items-start justify-between">
                  <div>
                    <p className="text-sm font-medium text-slate-200">{event.message}</p>
                    <p className="text-xs text-slate-500">{event.type}</p>
                  </div>
                  <span className="shrink-0 text-xs text-slate-500">{new Date(event.timestamp).toLocaleString()}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
