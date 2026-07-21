"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useParams, useRouter } from "next/navigation";
import {
  Play, Square, RotateCcw, Trash2, Loader2, Terminal,
  CheckCircle, XCircle, AlertTriangle, Clock, ArrowLeft, RefreshCw, Eye, EyeOff
} from "lucide-react";
import Link from "next/link";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");

interface ComposeStack {
  id: string;
  userId: string;
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
  reservationId: string;
  composeType: string;
  sourceType: string;
  environmentId: string;
  createdAt: string;
  updatedAt: string;
}

interface ServiceState {
  name: string;
  image: string;
  status: string;
  state: string;
  ports: string;
}

interface StackStatus {
  stack: ComposeStack;
  services: ServiceState[];
}

const statusColors: Record<string, string> = {
  running: "text-emerald-400",
  deploying: "text-blue-400",
  awaiting_health: "text-yellow-400",
  stopped: "text-slate-400",
  degraded: "text-orange-400",
  failed: "text-red-400",
  updating: "text-blue-400",
};

function getServiceStatusColor(state: string, status: string): string {
  const s = (state || status || "").toLowerCase();
  if (s.includes("running") || s.includes("up")) return "text-emerald-400";
  if (s.includes("exited") || s.includes("stopped")) return "text-red-400";
  if (s.includes("paused")) return "text-yellow-400";
  if (s.includes("restarting")) return "text-blue-400";
  return "text-slate-400";
}

export default function ComposeStackDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const id = params.id as string;
  const [showYaml, setShowYaml] = useState(false);
  const [logService, setLogService] = useState("");
  const [logTail, setLogTail] = useState(100);

  const { data: status, isLoading } = useQuery<StackStatus>({
    queryKey: ["compose-stack-status", id],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/compose/${id}/status`, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch stack status");
      return res.json();
    },
    refetchInterval: 10000,
  });

  const { data: logs } = useQuery<string[]>({
    queryKey: ["compose-stack-logs", id, logService, logTail],
    queryFn: async () => {
      const params = new URLSearchParams({ tail: String(logTail) });
      if (logService) params.set("service", logService);
      const res = await fetch(`${API_BASE}/compose/${id}/logs?${params}`, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch logs");
      const data = await res.json();
      if (data.services) {
        const allLogs = data.services._all || "";
        return allLogs.split("\n").filter(Boolean);
      }
      return [];
    },
    refetchInterval: 5000,
  });

  const stopMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch(`${API_BASE}/compose/${id}/stop`, {
        method: "POST", credentials: "include",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) throw new Error("Stop failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Stack stopped" });
    },
    onError: () => toast({ tone: "error", title: "Stop failed" }),
  });

  const startMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch(`${API_BASE}/compose/${id}/start`, {
        method: "POST", credentials: "include",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) throw new Error("Start failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Stack started" });
    },
    onError: () => toast({ tone: "error", title: "Start failed" }),
  });

  const redeployMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch(`${API_BASE}/compose/${id}/deploy`, {
        method: "POST", credentials: "include",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) throw new Error("Redeploy failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Redeploying stack" });
    },
    onError: () => toast({ tone: "error", title: "Redeploy failed" }),
  });

  const deleteMutation = useMutation({
    mutationFn: async () => {
      const res = await fetch(`${API_BASE}/compose/${id}`, {
        method: "DELETE", credentials: "include",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) throw new Error("Delete failed");
    },
    onSuccess: () => {
      toast({ tone: "success", title: "Stack deleted" });
      router.push("/admin/compose");
    },
    onError: () => toast({ tone: "error", title: "Delete failed" }),
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 className="h-8 w-8 animate-spin text-slate-400" />
      </div>
    );
  }

  if (!status) {
    return (
      <div className="p-6">
        <p className="text-slate-400">Stack not found.</p>
        <Link href="/admin/compose" className="text-blue-400 hover:text-blue-300 text-sm">Back to stacks</Link>
      </div>
    );
  }

  const { stack, services } = status;
  const colorClass = statusColors[stack.status] || "text-slate-400";

  return (
    <div className="space-y-6 p-6 max-w-6xl">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/admin/compose" className="text-slate-400 hover:text-slate-300 transition-colors">
            <ArrowLeft className="h-5 w-5" />
          </Link>
          <h1 className="text-2xl font-bold text-slate-100">{stack.name}</h1>
          <span className={`text-sm font-medium ${colorClass} capitalize`}>
            {stack.status.replace(/_/g, " ")}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {stack.status === "stopped" && (
            <button onClick={() => startMutation.mutate()} className="flex items-center gap-2 rounded-lg bg-emerald-600 px-3 py-2 text-sm font-medium text-white hover:bg-emerald-700 transition-colors">
              <Play className="h-4 w-4" /> Start
            </button>
          )}
          {stack.status === "running" && (
            <button onClick={() => stopMutation.mutate()} className="flex items-center gap-2 rounded-lg bg-yellow-600 px-3 py-2 text-sm font-medium text-white hover:bg-yellow-700 transition-colors">
              <Square className="h-4 w-4" /> Stop
            </button>
          )}
          <button onClick={() => redeployMutation.mutate()} className="flex items-center gap-2 rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 transition-colors">
            <RotateCcw className="h-4 w-4" /> Redeploy
          </button>
          <button onClick={() => { if (confirm("Delete this stack?")) deleteMutation.mutate(); }} className="flex items-center gap-2 rounded-lg bg-red-600/20 px-3 py-2 text-sm font-medium text-red-400 hover:bg-red-600/30 transition-colors">
            <Trash2 className="h-4 w-4" /> Delete
          </button>
        </div>
      </div>

      {stack.error && (
        <div className="rounded-lg border border-red-500/50 bg-red-500/10 p-4">
          <div className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-red-400" />
            <span className="text-sm text-red-400">{stack.error}</span>
          </div>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-4 rounded-xl border border-slate-700/50 bg-[#1a2332] p-6">
          <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-wide">Info</h2>
          <div className="space-y-3 text-sm">
            <div className="flex justify-between">
              <span className="text-slate-500">ID</span>
              <span className="text-slate-300 font-mono text-xs">{stack.id}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-500">Type</span>
              <span className="text-slate-300">{stack.composeType}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-500">Source</span>
              <span className="text-slate-300">{stack.sourceType}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-500">Node</span>
              <span className="text-slate-300">{stack.nodeId || "—"}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-500">Created</span>
              <span className="text-slate-300">{new Date(stack.createdAt).toLocaleString()}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-500">Updated</span>
              <span className="text-slate-300">{new Date(stack.updatedAt).toLocaleString()}</span>
            </div>
          </div>
        </div>

        <div className="space-y-4 rounded-xl border border-slate-700/50 bg-[#1a2332] p-6">
          <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-wide">Resources</h2>
          <div className="space-y-3 text-sm">
            <div className="flex justify-between">
              <span className="text-slate-500">Memory</span>
              <span className="text-slate-300">{stack.memoryMb || "—"} MB</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-500">CPU</span>
              <span className="text-slate-300">{stack.cpuShares || "—"} shares</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-500">Disk</span>
              <span className="text-slate-300">{stack.diskMb || "—"} MB</span>
            </div>
          </div>
        </div>

        <div className="space-y-4 rounded-xl border border-slate-700/50 bg-[#1a2332] p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-wide">Compose YAML</h2>
            <button onClick={() => setShowYaml(!showYaml)} className="text-slate-500 hover:text-slate-300 transition-colors">
              {showYaml ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
          {showYaml && (
            <pre className="rounded bg-[#0f1419] p-3 text-xs font-mono text-slate-300 overflow-auto max-h-60 border border-slate-700/50">
              {stack.composeYaml}
            </pre>
          )}
          {!showYaml && (
            <p className="text-xs text-slate-500">Click the eye icon to view the compose file.</p>
          )}
        </div>
      </div>

      <div className="rounded-xl border border-slate-700/50 bg-[#1a2332]">
        <div className="border-b border-slate-700/50 p-4">
          <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-wide">Services</h2>
        </div>
        <div className="divide-y divide-slate-700/50">
          {(!services || services.length === 0) ? (
            <div className="p-4 text-sm text-slate-500">No services found.</div>
          ) : (
            services.map((svc) => (
              <div key={svc.name} className="flex items-center justify-between p-4 hover:bg-slate-700/20 transition-colors">
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
                  <span className={`${getServiceStatusColor(svc.state, svc.status)}`}>
                    {svc.status || svc.state || "unknown"}
                  </span>
                  {svc.ports && <span className="text-slate-500">{svc.ports}</span>}
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      <div className="rounded-xl border border-slate-700/50 bg-[#1a2332]">
        <div className="border-b border-slate-700/50 p-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-wide flex items-center gap-2">
              <Terminal className="h-4 w-4" /> Logs
            </h2>
            <div className="flex items-center gap-3">
              <select
                value={logService}
                onChange={(e) => setLogService(e.target.value)}
                className="rounded-lg border border-slate-600 bg-[#0f1419] px-2 py-1 text-xs text-slate-300 focus:border-blue-500 focus:outline-none"
              >
                <option value="">All services</option>
                {services?.map((s) => (
                  <option key={s.name} value={s.name}>{s.name}</option>
                ))}
              </select>
              <select
                value={logTail}
                onChange={(e) => setLogTail(Number(e.target.value))}
                className="rounded-lg border border-slate-600 bg-[#0f1419] px-2 py-1 text-xs text-slate-300 focus:border-blue-500 focus:outline-none"
              >
                <option value={50}>50 lines</option>
                <option value={100}>100 lines</option>
                <option value={500}>500 lines</option>
              </select>
            </div>
          </div>
        </div>
        <div className="p-4">
          <pre className="max-h-80 overflow-auto rounded-lg bg-[#0f1419] p-4 text-xs font-mono text-slate-300 border border-slate-700/50">
            {(!logs || logs.length === 0) ? (
              <span className="text-slate-500">No logs available.</span>
            ) : (
              logs.map((line, i) => (
                <div key={i} className="hover:bg-slate-800/50">
                  {line}
                </div>
              ))
            )}
          </pre>
        </div>
      </div>
    </div>
  );
}
