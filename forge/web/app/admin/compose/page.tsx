"use client";

import { useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast"
import { useConfirm } from "@/components/ui/confirm-dialog";;
import { useRouter } from "next/navigation";
import { deleteJSON, fetchJSON, postJSON } from "@/lib/api";
import { AdminLoadingState, AdminPageHeader, Btn, Card, EmptyState } from "@/components/admin/admin-ui";
import {
  Plus, Play, Square, Trash2, Loader2,
  CheckCircle, XCircle, AlertTriangle, Clock, ArrowUpDown
} from "lucide-react";

interface ComposeStack {
  id: string;
  name: string;
  nodeId: string;
  status: string;
  composeYaml: string;
  composeHash: string;
  envVars: Record<string, string>;
  memoryMb: number;
  cpuShares: number;
  diskMb: number;
  error: string;
  composeType: string;
  sourceType: string;
  environmentId: string;
  createdAt: string;
  updatedAt: string;
}

const statusConfig: Record<string, { color: string; bg: string; icon: React.ReactNode }> = {
  running: { color: "text-emerald-400", bg: "bg-emerald-500/10", icon: <CheckCircle className="h-4 w-4" /> },
  deploying: { color: "text-slate-200", bg: "bg-white/[0.06]", icon: <Loader2 className="h-4 w-4 animate-spin" /> },
  awaiting_health: { color: "text-yellow-400", bg: "bg-yellow-500/10", icon: <Clock className="h-4 w-4" /> },
  stopped: { color: "text-slate-400", bg: "bg-slate-500/10", icon: <Square className="h-4 w-4" /> },
  degraded: { color: "text-amber-300", bg: "bg-amber-500/10", icon: <AlertTriangle className="h-4 w-4" /> },
  failed: { color: "text-red-400", bg: "bg-red-500/10", icon: <XCircle className="h-4 w-4" /> },
  updating: { color: "text-slate-200", bg: "bg-white/[0.06]", icon: <ArrowUpDown className="h-4 w-4" /> },
  deleting: { color: "text-red-400", bg: "bg-red-500/10", icon: <Loader2 className="h-4 w-4 animate-spin" /> },
  deleted: { color: "text-slate-500", bg: "bg-slate-500/10", icon: <XCircle className="h-4 w-4" /> },
};

export default function ComposeStacksPage() {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const router = useRouter();

  const { data: stacks, isLoading } = useQuery<ComposeStack[]>({
    queryKey: ["compose-stacks"],
    queryFn: async () => {
      return fetchJSON<ComposeStack[]>("/compose");
    },
  });

  const safeStacks = useMemo(() => Array.isArray(stacks) ? stacks : [], [stacks]);

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      return deleteJSON<void>(`/compose/${encodeURIComponent(id)}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack deleted" });
    },
    onError: () => toast({ tone: "error", title: "Delete failed" }),
  });

  const stopMutation = useMutation({
    mutationFn: async (id: string) => {
      return postJSON<void>(`/compose/${encodeURIComponent(id)}/stop`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack stopped" });
    },
    onError: () => toast({ tone: "error", title: "Stop failed" }),
  });

  const startMutation = useMutation({
    mutationFn: async (id: string) => {
      return postJSON<void>(`/compose/${encodeURIComponent(id)}/start`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack started" });
    },
    onError: () => toast({ tone: "error", title: "Start failed" }),
  });

  const handleAction = (action: string, id: string) => {
    switch (action) {
      case "stop": stopMutation.mutate(id); break;
      case "start": startMutation.mutate(id); break;
      case "delete":
        void (async () => { if (await confirm({ title: "Delete this compose stack?", description: "The stack and its services will be removed. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(id); })();
        break;
    }
  };

  return (
    <div className="space-y-6">
      <AdminPageHeader title="Compose Stacks" description="Deploy and operate multi-service Compose workloads." action={<Btn onClick={() => router.push("/admin/compose/new")}><Plus className="h-4 w-4" /> New stack</Btn>} />

      {isLoading ? (
        <AdminLoadingState label="Loading Compose stacks…" />
      ) : safeStacks.length === 0 ? (
        <Card className="p-8"><EmptyState title="No Compose stacks" message="Create a stack to deploy a multi-service workload." /><div className="mt-4 flex justify-center"><Btn onClick={() => router.push("/admin/compose/new")}><Plus className="h-4 w-4" /> Create stack</Btn></div></Card>
      ) : (
        <div className="grid gap-4">
          {safeStacks.map((stack) => {
            const cfg = statusConfig[stack.status] || statusConfig.failed;
            return (
              <div
                key={stack.id}
                className="ui-card p-4 cursor-pointer"
                onClick={() => router.push(`/admin/compose/${stack.id}`)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => { if (e.key === 'Enter') router.push(`/admin/compose/${stack.id}`); }}
              >
                <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
                  <div className="flex items-center gap-3">
                    <div className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs ${cfg.bg} ${cfg.color}`}>
                      {cfg.icon}
                      <span className="capitalize">{stack.status.replace(/_/g, " ")}</span>
                    </div>
                    <div>
                      <span className="text-sm font-medium text-slate-200">{stack.name}</span>
                      <span className="ml-2 text-xs text-slate-500">{stack.composeType}</span>
                    </div>
                  </div>
                  <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                    {stack.status === "stopped" && (
                      <Btn size="sm" tone="ghost" onClick={() => handleAction("start", stack.id)} ariaLabel="Start">
                        <Play className="h-4 w-4" />
                      </Btn>
                    )}
                    {stack.status === "running" && (
                      <Btn size="sm" tone="ghost" onClick={() => handleAction("stop", stack.id)} ariaLabel="Stop">
                        <Square className="h-4 w-4" />
                      </Btn>
                    )}
                    <Btn size="sm" tone="danger" onClick={() => handleAction("delete", stack.id)} ariaLabel="Delete">
                      <Trash2 className="h-4 w-4" />
                    </Btn>
                  </div>
                </div>
                {stack.error && (
                  <div className="mt-2 text-xs text-red-400 truncate">{stack.error}</div>
                )}
                <div className="mt-2 flex gap-4 text-xs text-slate-500">
                  <span>Node: {stack.nodeId || "—"}</span>
                  <span>Source: {stack.sourceType}</span>
                  <span>Created: {new Date(stack.createdAt).toLocaleDateString()}</span>
                </div>
              </div>
            );
          })}
        </div>
      )}
      {renderConfirm()}
    </div>
  );
}
