"use client";

import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  CheckCircle, Clock, Layers, Plus, RefreshCw,
  RotateCcw, XCircle, History, Box,
} from "lucide-react";
import { fetchJSON } from "@/lib/api";
import { fetchAllDeployments, typeLabel, type AppDeployment } from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, Modal, cn } from "@/components/admin/admin-ui";
import { DeployStatusBadge } from "@/components/admin/AdminAppsShared";
import { formatDate } from "@/lib/utils";

type ServerDeployment = {
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
  log?: string;
};

const statusConfig: Record<string, { tone: "green" | "yellow" | "red" | "blue" | "neutral"; icon: typeof Clock }> = {
  pending: { tone: "yellow", icon: Clock },
  in_progress: { tone: "blue", icon: RefreshCw },
  completed: { tone: "green", icon: CheckCircle },
  failed: { tone: "red", icon: XCircle },
  rolled_back: { tone: "neutral", icon: RotateCcw },
};

type TabId = "servers" | "apps";

export default function AdminDeploymentsPage() {
  const router = useRouter();
  const [tab, setTab] = useState<TabId>("servers");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [strategyFilter, setStrategyFilter] = useState<string>("");
  const [selectedDeployment, setSelectedDeployment] = useState<ServerDeployment | AppDeployment | null>(null);
  const [isAppDeployment, setIsAppDeployment] = useState(false);

  const serverDeploymentsQuery = useQuery({
    queryKey: ["admin", "deployments"],
    queryFn: () => fetchJSON<ServerDeployment[]>("/admin/deployments"),
    refetchInterval: 15_000,
  });

  const appDeploymentsQuery = useQuery({
    queryKey: ["admin", "app-deployments"],
    queryFn: fetchAllDeployments,
    refetchInterval: 15_000,
  });

  const serverDeployments = useMemo(() => serverDeploymentsQuery.data ?? [], [serverDeploymentsQuery.data]);
  const appDeployments = useMemo(() => appDeploymentsQuery.data ?? [], [appDeploymentsQuery.data]);

  const filteredServers = useMemo(() => {
    return serverDeployments.filter((d) => {
      if (search && !d.serverId.toLowerCase().includes(search.toLowerCase()) && !d.image.toLowerCase().includes(search.toLowerCase())) return false;
      if (statusFilter && d.status !== statusFilter) return false;
      if (strategyFilter && d.strategy !== strategyFilter) return false;
      return true;
    });
  }, [serverDeployments, search, statusFilter, strategyFilter]);

  const filteredApps = useMemo(() => {
    return appDeployments.filter((d) => {
      const searchLower = search.toLowerCase();
      if (search && !d.appId.toLowerCase().includes(searchLower) &&
        !(d.image ?? "").toLowerCase().includes(searchLower) &&
        !(d.commit ?? "").toLowerCase().includes(searchLower)) return false;
      if (statusFilter && d.status !== statusFilter) return false;
      return true;
    });
  }, [appDeployments, search, statusFilter]);

  const serverStatuses = ["pending", "in_progress", "completed", "failed", "rolled_back"];
  const appStatuses = ["pending", "running", "completed", "failed", "canceled"];
  const strategies = ["blue_green", "rolling", "recreate"];

  const currentStatuses = tab === "servers" ? serverStatuses : appStatuses;

  const isLoading = tab === "servers" ? serverDeploymentsQuery.isLoading : appDeploymentsQuery.isLoading;
  const currentData = tab === "servers" ? filteredServers : filteredApps;
  const currentCount = tab === "servers" ? filteredServers.length : filteredApps.length;

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Deployments"
        sub="Server and application deployment history across the cluster."
        action={
          <Btn tone="primary" onClick={() => router.push("/admin/deployments/new")}>
            <Plus size={14} /> New Blue-Green Deployment
          </Btn>
        }
      />

      <div className="flex gap-1 border-b border-white/[0.06]">
        {([
          { id: "servers", label: "Server Deployments", icon: Layers },
          { id: "apps", label: "App Deployments", icon: Box },
        ] as { id: TabId; label: string; icon: typeof Layers }[]).map(
          ({ id: tId, label, icon: Icon }) => (
            <button
              key={tId}
              type="button"
              className={cn(
                "flex items-center gap-1.5 px-3 py-2 text-xs font-medium border-b-2 transition -mb-px",
                tab === tId
                  ? "border-[#dc2626] text-[#dc2626]"
                  : "border-transparent text-slate-500 hover:text-slate-300",
              )}
              onClick={() => { setTab(tId); setStatusFilter(""); setSearch(""); }}
            >
              <Icon size={12} />
              {label}
            </button>
          ),
        )}
      </div>

      <Card>
        <CardHeader title={`${currentCount.toLocaleString()} deployment${currentCount === 1 ? "" : "s"}`} icon={History} />
        <div className="flex flex-wrap items-center gap-3 p-4">
          <Input
            placeholder={tab === "servers" ? "Search by server or image" : "Search by app, image, or commit"}
            value={search}
            onChange={setSearch}
          />
          <select
            className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            <option value="">All Statuses</option>
            {currentStatuses.map((s) => (
              <option key={s} value={s}>{s.replace(/_/g, " ")}</option>
            ))}
          </select>
          {tab === "servers" && (
            <select
              className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
              value={strategyFilter}
              onChange={(e) => setStrategyFilter(e.target.value)}
            >
              <option value="">All Strategies</option>
              {strategies.map((s) => (
                <option key={s} value={s}>{s.replace(/_/g, " ")}</option>
              ))}
            </select>
          )}
        </div>

        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading deployments...</div>
        ) : currentData.length === 0 ? (
          <EmptyState icon={History} message="No deployments found." />
        ) : tab === "servers" ? (
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
                {(filteredServers).map((dep) => {
                  const cfg = statusConfig[dep.status] ?? statusConfig.pending;
                  const StatusIcon = cfg.icon;
                  return (
                    <tr key={dep.id} className="hover:bg-white/[0.02] cursor-pointer">
                      <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">{dep.serverId}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{dep.image}</td>
                      <td className="px-4 py-3">
                        <Pill tone={dep.strategy === "blue_green" ? "blue" : dep.strategy === "rolling" ? "yellow" : "neutral"}>
                          {dep.strategy.replace(/_/g, "-")}
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
                          <Pill tone={cfg.tone}>{dep.status.replace(/_/g, " ")}</Pill>
                        </div>
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-400">{dep.targetGroup ?? "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-500">{formatDate(dep.createdAt)}</td>
                      <td className="px-4 py-3">
                        <Btn size="sm" tone="ghost" onClick={() => { setSelectedDeployment(dep); setIsAppDeployment(false); }}>
                          Details
                        </Btn>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">App</th>
                  <th className="px-4 py-3">Revision</th>
                  <th className="px-4 py-3">Source</th>
                  <th className="px-4 py-3">Trigger</th>
                  <th className="px-4 py-3">Commit/Image</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Duration</th>
                  <th className="px-4 py-3">Started</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {(filteredApps).map((dep) => (
                  <tr key={dep.id} className="hover:bg-white/[0.02] cursor-pointer">
                    <td className="px-4 py-3 font-mono text-xs text-slate-200">
                      <button
                        type="button"
                        className="hover:text-white"
                        onClick={() => router.push(`/admin/apps/${dep.appId}`)}
                      >
                        {dep.appId.slice(0, 8)}...
                      </button>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">#{dep.revision}</td>
                    <td className="px-4 py-3">
                      <Pill tone="neutral">{typeLabel(dep.source)}</Pill>
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={dep.trigger === "webhook" ? "blue" : dep.trigger === "auto" ? "green" : "neutral"}>
                        {dep.trigger}
                      </Pill>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">
                      {dep.commit?.slice(0, 7) ?? dep.image?.slice(0, 30) ?? "—"}
                    </td>
                    <td className="px-4 py-3">
                      <DeployStatusBadge status={dep.status} type="deployment" />
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {dep.duration != null ? `${dep.duration}s` : "—"}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">{formatDate(dep.startedAt)}</td>
                    <td className="px-4 py-3">
                      <Btn size="sm" tone="ghost" onClick={() => { setSelectedDeployment(dep); setIsAppDeployment(true); }}>
                        Details
                      </Btn>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {selectedDeployment && (
        <DeploymentDetailModal
          deployment={selectedDeployment}
          isApp={isAppDeployment}
          onClose={() => setSelectedDeployment(null)}
        />
      )}
    </div>
  );
}

function DeploymentDetailModal({
  deployment,
  isApp,
  onClose,
}: {
  deployment: ServerDeployment | AppDeployment;
  isApp: boolean;
  onClose: () => void;
}) {
  if (isApp) {
    const dep = deployment as AppDeployment;
    return (
      <Modal title={`Deployment #${dep.revision}`} onClose={onClose} wide>
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <span className="text-slate-400">App:</span>
              <span className="font-mono text-xs text-slate-200 ml-1">{dep.appId}</span>
            </div>
            <div>
              <span className="text-slate-400">Revision:</span>
              <span className="text-slate-200 ml-1">#{dep.revision}</span>
            </div>
            <div>
              <span className="text-slate-400">Source:</span>
              <span className="text-slate-200 ml-1">{typeLabel(dep.source)}</span>
            </div>
            <div>
              <span className="text-slate-400">Trigger:</span>
              <span className="text-slate-200 ml-1">{dep.trigger}</span>
            </div>
            <div>
              <span className="text-slate-400">Status:</span>
              <DeployStatusBadge status={dep.status} type="deployment" />
            </div>
            <div>
              <span className="text-slate-400">Duration:</span>
              <span className="text-slate-200 ml-1">{dep.duration != null ? `${dep.duration}s` : "—"}</span>
            </div>
            <div>
              <span className="text-slate-400">Started:</span>
              <span className="text-slate-200 ml-1">{formatDate(dep.startedAt)}</span>
            </div>
            <div>
              <span className="text-slate-400">Completed:</span>
              <span className="text-slate-200 ml-1">{formatDate(dep.completedAt)}</span>
            </div>
            {dep.commit && (
              <div className="col-span-2">
                <span className="text-slate-400">Commit:</span>
                <span className="font-mono text-xs text-slate-200 ml-1">{dep.commit}</span>
              </div>
            )}
            {dep.commitMessage && (
              <div className="col-span-2">
                <span className="text-slate-400">Message:</span>
                <span className="text-slate-200 ml-1">{dep.commitMessage}</span>
              </div>
            )}
            {dep.image && (
              <div className="col-span-2">
                <span className="text-slate-400">Image:</span>
                <span className="font-mono text-xs text-slate-200 ml-1">{dep.image}</span>
              </div>
            )}
          </div>
          {dep.error && (
            <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
              {dep.error}
            </div>
          )}
          {dep.log && (
            <div>
              <p className="mb-2 text-xs font-semibold text-slate-400">Build/Deploy Log</p>
              <pre className="max-h-48 overflow-y-auto rounded-lg border border-white/[0.06] bg-[#0a0e14] p-3 font-mono text-xs text-slate-400 whitespace-pre-wrap">
                {dep.log}
              </pre>
            </div>
          )}
        </div>
      </Modal>
    );
  }

  const dep = deployment as ServerDeployment;
  const cfg = statusConfig[dep.status] ?? statusConfig.pending;
  const StatusIcon = cfg.icon;

  return (
    <Modal title="Deployment Details" onClose={onClose} wide>
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4 text-sm">
          <div>
            <span className="text-slate-400">Server:</span>
            <span className="font-mono text-xs text-slate-200 ml-1">{dep.serverId}</span>
          </div>
          <div>
            <span className="text-slate-400">Image:</span>
            <span className="text-slate-200 ml-1">{dep.image}</span>
          </div>
          <div>
            <span className="text-slate-400">Strategy:</span>
            <Pill tone={dep.strategy === "blue_green" ? "blue" : dep.strategy === "rolling" ? "yellow" : "neutral"}>
              {dep.strategy.replace(/_/g, "-")}
            </Pill>
          </div>
          <div>
            <span className="text-slate-400">Status:</span>
            <div className="inline-flex items-center gap-1.5 ml-1">
              <StatusIcon size={12} className={cn(
                dep.status === "completed" && "text-emerald-400",
                dep.status === "failed" && "text-red-400",
              )} />
              <Pill tone={cfg.tone}>{dep.status.replace(/_/g, " ")}</Pill>
            </div>
          </div>
          <div>
            <span className="text-slate-400">Target Group:</span>
            <span className="text-slate-200 ml-1">{dep.targetGroup ?? "—"}</span>
          </div>
          <div>
            <span className="text-slate-400">Created:</span>
            <span className="text-slate-200 ml-1">{formatDate(dep.createdAt)}</span>
          </div>
          {dep.healthCheckPath && (
            <div className="col-span-2">
              <span className="text-slate-400">Health Check:</span>
              <span className="text-slate-200 ml-1">{dep.healthCheckPath}:{dep.healthCheckPort}</span>
            </div>
          )}
        </div>
        {dep.error && (
          <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
            {dep.error}
          </div>
        )}
        {dep.log && (
          <div>
            <p className="mb-2 text-xs font-semibold text-slate-400">Deployment Log</p>
            <pre className="max-h-48 overflow-y-auto rounded-lg border border-white/[0.06] bg-[#0a0e14] p-3 font-mono text-xs text-slate-400 whitespace-pre-wrap">
              {dep.log}
            </pre>
          </div>
        )}
      </div>
    </Modal>
  );
}
