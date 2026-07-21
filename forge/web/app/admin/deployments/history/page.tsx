"use client";

import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, CheckCircle, Clock, History, RotateCcw, XCircle, AlertCircle,
} from "lucide-react";
import { fetchJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";
import { formatDate } from "@/lib/utils";

type DeploymentRecord = {
  id: string;
  serverId: string;
  serviceId?: string;
  status: "pending" | "running" | "done" | "error" | "cancelled";
  logPath?: string;
  commitHash?: string;
  commitMessage?: string;
  errorMessage?: string;
  rollbackId?: string;
  startedAt?: string;
  finishedAt?: string;
  createdAt: string;
  updatedAt: string;
};

const statusConfig: Record<string, { tone: "green" | "yellow" | "red" | "blue" | "neutral"; icon: typeof Clock }> = {
  pending: { tone: "yellow", icon: Clock },
  running: { tone: "blue", icon: Clock },
  done: { tone: "green", icon: CheckCircle },
  error: { tone: "red", icon: XCircle },
  cancelled: { tone: "neutral", icon: AlertCircle },
};

export default function AdminDeploymentHistoryPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("");

  const historyQuery = useQuery({
    queryKey: ["admin", "deployment-history"],
    queryFn: () => fetchJSON<DeploymentRecord[]>("/admin/deployment-history"),
    refetchInterval: 10_000,
  });

  const records = useMemo(() => historyQuery.data ?? [], [historyQuery.data]);

  const filtered = useMemo(() => {
    return records.filter((d) => {
      if (search && !d.serverId.toLowerCase().includes(search.toLowerCase()) && !(d.commitHash ?? "").toLowerCase().includes(search.toLowerCase())) return false;
      if (statusFilter && d.status !== statusFilter) return false;
      return true;
    });
  }, [records, search, statusFilter]);

  const statuses = ["pending", "running", "done", "error", "cancelled"];

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Deployment History"
        sub="Historical record of all deployments across the cluster."
        action={
          <Btn tone="ghost" onClick={() => router.push("/admin/deployments")}>
            <ArrowLeft size={14} /> Back to Deployments
          </Btn>
        }
      />

      <Card>
        <CardHeader title={`${filtered.length.toLocaleString()} record${filtered.length === 1 ? "" : "s"}`} icon={History} />
        <div className="flex flex-wrap items-center gap-3 p-4">
          <Input
            placeholder="Search by server ID or commit hash"
            value={search}
            onChange={setSearch}
          />
          <select
            className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            <option value="">All Statuses</option>
            {statuses.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
        </div>

        {historyQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading history...</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={History} message="No deployment records found." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Server</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Commit</th>
                  <th className="px-4 py-3">Message</th>
                  <th className="px-4 py-3">Started</th>
                  <th className="px-4 py-3">Finished</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((dep) => {
                  const cfg = statusConfig[dep.status] ?? statusConfig.pending;
                  const StatusIcon = cfg.icon;
                  return (
                    <tr key={dep.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">{dep.serverId.slice(0, 8)}...</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5">
                          <StatusIcon size={12} className={cn(
                            dep.status === "done" && "text-emerald-400",
                            dep.status === "error" && "text-red-400",
                            dep.status === "running" && "text-blue-400",
                            dep.status === "pending" && "text-amber-400",
                            dep.status === "cancelled" && "text-slate-400",
                          )} />
                          <Pill tone={cfg.tone}>{dep.status}</Pill>
                        </div>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{dep.commitHash ? dep.commitHash.slice(0, 7) : "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-400 max-w-[200px] truncate">{dep.commitMessage || "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-500">{dep.startedAt ? formatDate(dep.startedAt) : "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-500">{dep.finishedAt ? formatDate(dep.finishedAt) : "—"}</td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1">
                          {dep.errorMessage && (
                            <span className="inline-flex items-center gap-1 rounded-lg px-2 py-1 text-xs text-red-400" title={dep.errorMessage}>
                              <AlertCircle size={12} />
                            </span>
                          )}
                          {dep.rollbackId && (
                            <span className="inline-flex items-center gap-1 rounded-lg px-2 py-1 text-xs text-slate-400" title="Rolled back">
                              <RotateCcw size={12} />
                            </span>
                          )}
                        </div>
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
