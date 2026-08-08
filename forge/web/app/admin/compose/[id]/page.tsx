"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useParams, useRouter } from "next/navigation";
import {
  Play, Square, RotateCcw, Trash2, Loader2, Terminal,
  XCircle, Eye, EyeOff
} from "lucide-react";
import { AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, Pill } from "@/components/admin/admin-ui";
import { getComposeStackStatus, getComposeStackLogs, stopComposeStack, startComposeStack, deployComposeStack, deleteComposeStack } from "@/lib/api/compose";
import { useConfirm } from "@/components/ui/confirm-dialog";

function getServiceStatusColor(state: string, status: string): string {
  const s = (state || status || "").toLowerCase();
  if (s.includes("running") || s.includes("up")) return "text-emerald-400";
  if (s.includes("exited") || s.includes("stopped")) return "text-red-400";
  if (s.includes("paused")) return "text-yellow-400";
  if (s.includes("restarting")) return "text-slate-300";
  return "text-slate-400";
}

export default function ComposeStackDetailPage() {
  const [confirm, renderConfirm] = useConfirm();
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const id = params.id as string;
  const [showYaml, setShowYaml] = useState(false);
  const [logService, setLogService] = useState("");
  const [logTail, setLogTail] = useState(100);

  const { data: status, isLoading } = useQuery({
    queryKey: ["compose-stack-status", id],
    queryFn: () => getComposeStackStatus(id),
    refetchInterval: 10000,
  });

  const { data: logs } = useQuery<string[]>({
    queryKey: ["compose-stack-logs", id, logService, logTail],
    queryFn: async () => {
      const data = await getComposeStackLogs(id, logService || undefined, logTail) as { services?: Record<string, string> };
      if (data.services) {
        const allLogs = data.services._all || "";
        return allLogs.split("\n").filter(Boolean);
      }
      return [];
    },
    refetchInterval: 5000,
  });

  const safeLogs = useMemo(() => Array.isArray(logs) ? logs : [], [logs]);
  const safeServices = useMemo(() => Array.isArray(status?.services) ? status.services : [], [status?.services]);

  const stopMutation = useMutation({
    mutationFn: () => stopComposeStack(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Stack stopped" });
    },
    onError: () => toast({ tone: "error", title: "Stop failed" }),
  });

  const startMutation = useMutation({
    mutationFn: () => startComposeStack(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Stack started" });
    },
    onError: () => toast({ tone: "error", title: "Start failed" }),
  });

  const redeployMutation = useMutation({
    mutationFn: () => deployComposeStack(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Redeploying stack" });
    },
    onError: () => toast({ tone: "error", title: "Redeploy failed" }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteComposeStack(id),
    onSuccess: () => {
      toast({ tone: "success", title: "Stack deleted" });
      router.push("/admin/compose");
    },
    onError: () => toast({ tone: "error", title: "Delete failed" }),
  });

  if (isLoading) {
    return (
      <AdminPageLayout>
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-slate-400" />
        </div>
      </AdminPageLayout>
    );
  }

  if (!status) {
    return (
      <AdminPageLayout>
        <p className="text-slate-400">Stack not found.</p>
        <Btn tone="ghost" onClick={() => router.push("/admin/compose")}>Back to stacks</Btn>
      </AdminPageLayout>
    );
  }

  const { stack } = status;

  return (
    <AdminPageLayout className="max-w-6xl">
      <AdminPageHeader
        title={stack.name}
        description={stack.status.replace(/_/g, " ")}
        backAction={() => router.push("/admin/compose")}
        backLabel="Compose Stacks"
        action={
          <div className="flex items-center gap-2">
            <Pill tone={
              stack.status === "running" ? "green" :
              stack.status === "failed" ? "red" :
              stack.status === "stopped" ? "neutral" :
              stack.status === "degraded" ? "yellow" : "neutral"
            }>{stack.status.replace(/_/g, " ")}</Pill>
            {stack.status === "stopped" && (
              <Btn size="sm" tone="success" onClick={() => startMutation.mutate()}>
                <Play className="h-4 w-4" /> Start
              </Btn>
            )}
            {stack.status === "running" && (
              <Btn size="sm" tone="warning" onClick={() => stopMutation.mutate()}>
                <Square className="h-4 w-4" /> Stop
              </Btn>
            )}
            <Btn size="sm" tone="primary" onClick={() => redeployMutation.mutate()}>
              <RotateCcw className="h-4 w-4" /> Redeploy
            </Btn>
            <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: "Delete this compose stack?", description: "The stack and its resources will be removed. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(); })(); }}>
              <Trash2 className="h-4 w-4" /> Delete
            </Btn>
          </div>
        }
      />

      {stack.error && (
        <div className="rounded-lg border border-red-500/50 bg-red-500/10 p-4">
          <div className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-red-400" />
            <span className="text-sm text-red-400">{stack.error}</span>
          </div>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        <Card>
          <CardHeader title="Info" />
          <div className="space-y-3 p-4 text-sm">
            <div className="flex justify-between">
              <span className="text-slate-400">ID</span>
              <span className="text-slate-300 font-mono text-xs">{stack.id}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Type</span>
              <span className="text-slate-300">{stack.composeType}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Source</span>
              <span className="text-slate-300">{stack.sourceType}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Node</span>
              <span className="text-slate-300">{stack.nodeId || "—"}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Created</span>
              <span className="text-slate-300">{new Date(stack.createdAt).toLocaleString()}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Updated</span>
              <span className="text-slate-300">{new Date(stack.updatedAt).toLocaleString()}</span>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Resources" />
          <div className="space-y-3 p-4 text-sm">
            <div className="flex justify-between">
              <span className="text-slate-400">Memory</span>
              <span className="text-slate-300">{stack.memoryMb || "—"} MB</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">CPU</span>
              <span className="text-slate-300">{stack.cpuShares || "—"} shares</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Disk</span>
              <span className="text-slate-300">{stack.diskMb || "—"} MB</span>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Compose YAML" action={
            <button onClick={() => setShowYaml(!showYaml)} className="text-slate-400 hover:text-slate-200 transition-colors">
              {showYaml ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          } />
          <div className="p-4">
            {showYaml ? (
              <pre className="rounded bg-[#0d131d] p-3 text-xs font-mono text-slate-300 overflow-auto max-h-60">{stack.composeYaml}</pre>
            ) : (
              <p className="text-xs text-slate-500">Click the eye icon to view the compose file.</p>
            )}
          </div>
        </Card>
      </div>

      <Card>
        <CardHeader title="Services" />
        <div className="divide-y divide-white/[0.06]">
          {safeServices.length === 0 ? (
            <div className="p-4 text-sm text-slate-500">No services found.</div>
          ) : (
            safeServices.map((svc) => (
              <div key={svc.name} className="flex items-center justify-between p-4 hover:bg-white/[0.02] transition-colors">
                <div className="flex items-center gap-3">
                  <div className={`h-2 w-2 rounded-full ${getServiceStatusColor(svc.state, svc.status)}`} />
                  <div>
                    <span className="text-sm font-medium text-slate-200">{svc.name}</span>
                    {svc.image && (
                      <span className="ml-2 text-xs text-slate-500">{svc.image}</span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-4 text-xs">
                  <span className={getServiceStatusColor(svc.state, svc.status)}>
                    {svc.status || svc.state || "unknown"}
                  </span>
                  {svc.ports && <span className="text-slate-500">{svc.ports}</span>}
                </div>
              </div>
            ))
          )}
        </div>
      </Card>

      <Card>
        <CardHeader title="Logs" icon={Terminal} action={
          <div className="flex items-center gap-3">
            <select
              value={logService}
              onChange={(e) => setLogService(e.target.value)}
              className="rounded-lg border border-white/10 bg-[#0d131d] px-2 py-1 text-xs text-slate-300 focus:border-red-400/70 focus:outline-none focus:ring-2 focus:ring-red-500/15"
            >
              <option value="">All services</option>
              {safeServices.map((s) => (
                <option key={s.name} value={s.name}>{s.name}</option>
              ))}
            </select>
            <select
              value={logTail}
              onChange={(e) => setLogTail(Number(e.target.value))}
              className="rounded-lg border border-white/10 bg-[#0d131d] px-2 py-1 text-xs text-slate-300 focus:border-red-400/70 focus:outline-none focus:ring-2 focus:ring-red-500/15"
            >
              <option value={50}>50 lines</option>
              <option value={100}>100 lines</option>
              <option value={500}>500 lines</option>
            </select>
          </div>
        } />
        <div className="p-4">
          <pre className="max-h-80 overflow-auto rounded-lg bg-[#0d131d] p-4 text-xs font-mono text-slate-300">
            {safeLogs.length === 0 ? (
              <span className="text-slate-500">No logs available.</span>
            ) : (
              safeLogs.map((line, i) => (
                <div key={i} className="hover:bg-slate-800/50">
                  {line}
                </div>
              ))
            )}
          </pre>
        </div>
      </Card>
      {renderConfirm()}
    </AdminPageLayout>
  );
}

