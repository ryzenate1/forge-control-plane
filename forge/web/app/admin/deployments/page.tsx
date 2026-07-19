"use client";

import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  CheckCircle, Clock, Layers, Plus, RefreshCw,
  RotateCcw, XCircle,
} from "lucide-react";
import { fetchJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";

type Deployment = {
  id: string;
  serverId: string;
  image: string;
  strategy: "blue_green" | "rolling" | "recreate";
  status: "pending" | "in_progress" | "completed" | "failed" | "rolled_back";
  targetGroup?: string;
  healthCheckPath?: string;
  healthCheckPort?: number;
  createdAt: string;
  completedAt?: string;
  error?: string;
};

const statusConfig: Record<string, { tone: "green" | "yellow" | "red" | "blue" | "neutral"; icon: typeof Clock }> = {
  pending: { tone: "yellow", icon: Clock },
  in_progress: { tone: "blue", icon: RefreshCw },
  completed: { tone: "green", icon: CheckCircle },
  failed: { tone: "red", icon: XCircle },
  rolled_back: { tone: "neutral", icon: RotateCcw },
};

export default function AdminDeploymentsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [strategyFilter, setStrategyFilter] = useState<string>("");

  const deploymentsQuery = useQuery({
    queryKey: ["admin", "deployments"],
    queryFn: () => fetchJSON<Deployment[]>("/admin/deployments"),
  });

  const deployments = useMemo(() => deploymentsQuery.data ?? [], [deploymentsQuery.data]);

  const filtered = useMemo(() => {
    return deployments.filter((d) => {
      if (search && !d.serverId.toLowerCase().includes(search.toLowerCase()) && !d.image.toLowerCase().includes(search.toLowerCase())) return false;
      if (statusFilter && d.status !== statusFilter) return false;
      if (strategyFilter && d.strategy !== strategyFilter) return false;
      return true;
    });
  }, [deployments, search, statusFilter, strategyFilter]);

  const statuses = ["pending", "in_progress", "completed", "failed", "rolled_back"];
  const strategies = ["blue_green", "rolling", "recreate"];

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Deployments"
        sub="Blue-green, rolling, and re-create deployments for game servers."
        action={
          <Btn tone="primary" onClick={() => router.push("/admin/deployments/new")}>
            <Plus size={14} /> New Blue-Green Deployment
          </Btn>
        }
      />

      <Card>
        <CardHeader title="All Deployments" icon={Layers} />
        <div className="flex flex-wrap items-center gap-3 p-4">
          <Input placeholder="Search by server or image" value={search} onChange={setSearch} />
          <select
            className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            <option value="">All Statuses</option>
            {statuses.map((s) => (
              <option key={s} value={s}>{s.replace("_", " ")}</option>
            ))}
          </select>
          <select
            className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
            value={strategyFilter}
            onChange={(e) => setStrategyFilter(e.target.value)}
          >
            <option value="">All Strategies</option>
            {strategies.map((s) => (
              <option key={s} value={s}>{s.replace("_", " ")}</option>
            ))}
          </select>
        </div>
        {deploymentsQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading deployments...</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Layers} message="No deployments found." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Server ID</th>
                  <th className="px-4 py-3">Image</th>
                  <th className="px-4 py-3">Strategy</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Target</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((dep) => {
                  const cfg = statusConfig[dep.status] ?? statusConfig.pending;
                  const StatusIcon = cfg.icon;
                  return (
                    <tr
                      key={dep.id}
                      className="hover:bg-white/[0.02] cursor-pointer"
                      onClick={() => router.push(`/admin/deployments/${dep.id}`)}
                    >
                      <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">{dep.serverId}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{dep.image}</td>
                      <td className="px-4 py-3">
                        <Pill tone={dep.strategy === "blue_green" ? "blue" : dep.strategy === "rolling" ? "yellow" : "neutral"}>
                          {dep.strategy.replace("_", "-")}
                        </Pill>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5">
                          <StatusIcon size={12} className={cn(
                            dep.status === "completed" && "text-emerald-400",
                            dep.status === "failed" && "text-red-400",
                            dep.status === "in_progress" && "text-blue-400",
                            dep.status === "pending" && "text-amber-400",
                            dep.status === "rolled_back" && "text-slate-400",
                          )} />
                          <Pill tone={cfg.tone}>{dep.status.replace("_", " ")}</Pill>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-400">{dep.targetGroup ?? "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-500">{new Date(dep.createdAt).toLocaleDateString()}</td>
                      <td className="px-4 py-3">
                        <Btn size="sm" tone="ghost" onClick={() => router.push(`/admin/deployments/${dep.id}`)}>
                          Details
                        </Btn>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
