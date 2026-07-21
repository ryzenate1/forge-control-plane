"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useRouter } from "next/navigation";
import {
  Plus, Play, Square, RotateCcw, Trash2, Loader2,
  CheckCircle, XCircle, AlertTriangle, Clock, ArrowUpDown
} from "lucide-react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");

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
  deploying: { color: "text-blue-400", bg: "bg-blue-500/10", icon: <Loader2 className="h-4 w-4 animate-spin" /> },
  awaiting_health: { color: "text-yellow-400", bg: "bg-yellow-500/10", icon: <Clock className="h-4 w-4" /> },
  stopped: { color: "text-slate-400", bg: "bg-slate-500/10", icon: <Square className="h-4 w-4" /> },
  degraded: { color: "text-orange-400", bg: "bg-orange-500/10", icon: <AlertTriangle className="h-4 w-4" /> },
  failed: { color: "text-red-400", bg: "bg-red-500/10", icon: <XCircle className="h-4 w-4" /> },
  updating: { color: "text-blue-400", bg: "bg-blue-500/10", icon: <ArrowUpDown className="h-4 w-4" /> },
  deleting: { color: "text-red-400", bg: "bg-red-500/10", icon: <Loader2 className="h-4 w-4 animate-spin" /> },
  deleted: { color: "text-slate-500", bg: "bg-slate-500/10", icon: <XCircle className="h-4 w-4" /> },
};

export default function ComposeStacksPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const router = useRouter();

  const { data: stacks = [], isLoading } = useQuery<ComposeStack[]>({
    queryKey: ["compose-stacks"],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/compose`, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch stacks");
      return res.json();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/compose/${id}`, {
        method: "DELETE",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) throw new Error("Delete failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack deleted" });
    },
    onError: () => toast({ tone: "error", title: "Delete failed" }),
  });

  const stopMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/compose/${id}/stop`, {
        method: "POST", credentials: "include",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) throw new Error("Stop failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack stopped" });
    },
    onError: () => toast({ tone: "error", title: "Stop failed" }),
  });

  const startMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/compose/${id}/start`, {
        method: "POST", credentials: "include",
        headers: { "Content-Type": "application/json" },
      });
      if (!res.ok) throw new Error("Start failed");
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
        if (confirm("Delete this compose stack?")) deleteMutation.mutate(id);
        break;
    }
  };

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-100">Compose Stacks</h1>
        <button
          onClick={() => router.push("/admin/compose/new")}
          className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 transition-colors"
        >
          <Plus className="h-4 w-4" /> New Stack
        </button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-slate-400" />
        </div>
      ) : stacks.length === 0 ? (
        <div className="rounded-xl border border-slate-700/50 bg-[#1a2332] p-12 text-center">
          <p className="text-slate-400">No compose stacks yet.</p>
          <button
            onClick={() => router.push("/admin/compose/new")}
            className="mt-4 text-blue-400 hover:text-blue-300 text-sm"
          >
            Create your first stack
          </button>
        </div>
      ) : (
        <div className="grid gap-4">
          {stacks.map((stack) => {
            const cfg = statusConfig[stack.status] || statusConfig.failed;
            return (
              <div
                key={stack.id}
                className="rounded-xl border border-slate-700/50 bg-[#1a2332] p-4 hover:border-slate-600 transition-colors cursor-pointer"
                onClick={() => router.push(`/admin/compose/${stack.id}`)}
              >
                <div className="flex items-center justify-between">
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
                      <button
                        onClick={() => handleAction("start", stack.id)}
                        className="rounded-lg p-2 text-slate-400 hover:text-emerald-400 hover:bg-slate-700 transition-colors"
                        title="Start"
                      >
                        <Play className="h-4 w-4" />
                      </button>
                    )}
                    {stack.status === "running" && (
                      <button
                        onClick={() => handleAction("stop", stack.id)}
                        className="rounded-lg p-2 text-slate-400 hover:text-yellow-400 hover:bg-slate-700 transition-colors"
                        title="Stop"
                      >
                        <Square className="h-4 w-4" />
                      </button>
                    )}
                    <button
                      onClick={() => handleAction("delete", stack.id)}
                      className="rounded-lg p-2 text-slate-400 hover:text-red-400 hover:bg-slate-700 transition-colors"
                      title="Delete"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
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
    </div>
  );
}
