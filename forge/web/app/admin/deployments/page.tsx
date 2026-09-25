"use client";

import { useState, useMemo, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  CheckCircle, Clock, Layers, Plus, RefreshCw,
  RotateCcw, XCircle, History, Box,
} from "lucide-react";
import { fetchJSON } from "@/lib/api";
import { fetchAllDeployments, typeLabel, type AppDeployment } from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, Modal, cn } from "@/components/admin/admin-ui";
import { DashHeader, InfoCard } from "@/components/admin/dashboard-cards";
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
    if (!Array.isArray(serverDeployments)) return [];
    return serverDeployments.filter((d) => {
      if (search && !d.serverId.toLowerCase().includes(search.toLowerCase()) && !d.image.toLowerCase().includes(search.toLowerCase())) return false;
      if (statusFilter && d.status !== statusFilter) return false;
      if (strategyFilter && d.strategy !== strategyFilter) return false;
      return true;
    });
  }, [serverDeployments, search, statusFilter, strategyFilter]);

  const filteredApps = useMemo(() => {
    if (!Array.isArray(appDeployments)) return [];
    return appDeployments.filter((d) => {
      const searchLower = search.toLowerCase();
      if (search && !(d.appId ?? "").toLowerCase().includes(searchLower) &&
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
        sub="Deploy — workload releases to beacons. Blue-green, rolling and recreate strategies with health gates, revision history and rollback. (Build creates via Catalog/Apps; Deploy releases via Pipelines/Compose/Git.)"
        action={
          <div className="flex items-center gap-2">
            <Btn tone="ghost" onClick={() => router.push("/admin/deployments/history")}>
              <History size={14} /> History
            </Btn>
            <Btn tone="primary" onClick={() => router.push("/admin/deployments/new")}>
              <Plus size={14} /> New Deployment
            </Btn>
          </div>
        }
      />
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.015] px-4 py-2 text-xs leading-5 text-slate-400">
        <span className="font-semibold text-slate-300">DEPLOY</span> group per <code className="font-mono text-[11px]">target-ia.md §3</code>: Deployments · Pipelines · Compose · Git — releases (this page) vs Builds (Catalog/Apps) vs Operate (Ops). Server deployments use <code className="font-mono">blue_green / rolling / recreate</code> with target groups & health checks; App deployments track <code className="font-mono">revision</code> · <code className="font-mono">commit</code> · <code className="font-mono">trigger</code>. See also <button type="button" onClick={() => router.push("/admin/pipelines")} className="underline hover:text-slate-200">Pipelines</button> · <button type="button" onClick={() => router.push("/admin/compose")} className="underline hover:text-slate-200">Compose</button> · <button type="button" onClick={() => router.push("/admin/git")} className="underline hover:text-slate-200">Git</button>.
      </div>

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
                  ? "border-[var(--brand)] text-[var(--brand)]"
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
            className="h-9 rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 text-xs text-slate-300 outline-none"
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
              className="h-9 rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 text-xs text-slate-300 outline-none"
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

        {(tab === "servers" ? serverDeployments : appDeployments).length > 0 && (
          <div className="flex flex-wrap gap-1.5 border-y border-white/[0.04] bg-white/[0.015] px-4 py-2">
            {currentStatuses.map((s) => {
              const count = (tab === "servers" ? serverDeployments : appDeployments).filter((d) => (d as { status: string }).status === s).length;
              if (count === 0) return null;
              return <Pill key={s} tone={s === "completed" ? "green" : s === "failed" ? "red" : s === "in_progress" || s === "running" ? "blue" : "neutral"}>{s.replace(/_/g, " ")}: {count}</Pill>;
            })}
            <span className="ml-auto text-xs text-slate-600">Filter above to narrow · Strategy {tab === "servers" ? "blue-green tracks target groups" : "commit-triggered"}</span>
          </div>
        )}
        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading deployments…</div>
        ) : currentData.length === 0 ? (
          <EmptyState icon={History} title={search || statusFilter || strategyFilter ? "No matches" : tab === "servers" ? "No server deployments yet" : "No app deployments yet"} message={search || statusFilter || strategyFilter ? "No deployments match your filters — clear search/status above." : tab === "servers" ? "Create a blue-green/rolling deployment for a workload. See Apps → Deploy or use New Deployment." : "Trigger a pipeline or push to a Git-linked app to generate a deployment. See Pipelines or Git."} />
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
                        onClick={() => router.push(`/admin/apps/${dep.appId ?? ""}`)}
                      >
                        {(dep.appId ?? "").slice(0, 8)}...
                      </button>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">#{dep.revision}</td>
                    <td className="px-4 py-3">
                      <Pill tone="neutral">{dep.source ? typeLabel(dep.source) : "—"}</Pill>
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
    const appTone: "green" | "red" | "neutral" | "yellow" = dep.status === "completed" ? "green" : dep.status === "failed" ? "red" : dep.status === "pending" ? "neutral" : "yellow";
    return (
      <Modal title={`Deployment #${dep.revision}`} description={dep.commitMessage ?? dep.appId} onClose={onClose} wide className="max-w-6xl">
        <div className="space-y-4">
          <DashHeader
            icon={Layers}
            eyebrow="App deployment"
            title={`Deployment #${dep.revision}`}
            pill={{ tone: appTone, label: dep.status }}
            description={dep.commitMessage ?? undefined}
            tags={[dep.source ? typeLabel(dep.source) : "", dep.trigger].filter((t): t is string => Boolean(t))}
            meta={[
              { label: "App", value: <span key="app" className="font-mono">{dep.appId}</span> },
              { label: "Duration", value: dep.duration != null ? `${dep.duration}s` : "—" },
              { label: "Started", value: formatDate(dep.startedAt) },
            ]}
          />
          <InfoCard icon={Layers} title="Deployment Information" rows={[
            ["App", <span key="app" className="font-mono text-slate-200">{dep.appId}</span>],
            ["Revision", <span key="rev" className="text-slate-200">#{dep.revision}</span>],
            ["Source", <span key="src" className="text-slate-200">{dep.source ? typeLabel(dep.source) : "—"}</span>],
            ["Trigger", <span key="trig" className="text-slate-200">{dep.trigger}</span>],
            ["Status", <DeployStatusBadge key="st" status={dep.status} type="deployment" />],
            ["Duration", <span key="dur" className="text-slate-200">{dep.duration != null ? `${dep.duration}s` : "—"}</span>],
            ["Started", <span key="started" className="text-slate-200">{formatDate(dep.startedAt)}</span>],
            ["Completed", <span key="done" className="text-slate-200">{formatDate(dep.completedAt)}</span>],
            ...(dep.commit ? [["Commit", <span key="commit" className="font-mono text-slate-200">{dep.commit}</span>] as [string, ReactNode]] : []),
            ...(dep.commitMessage ? [["Message", <span key="msg" className="text-slate-200">{dep.commitMessage}</span>] as [string, ReactNode]] : []),
            ...(dep.image ? [["Image", <span key="img" className="font-mono text-slate-200">{dep.image}</span>] as [string, ReactNode]] : []),
          ]} />
          {dep.error && (
            <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
              {dep.error}
            </div>
          )}
          {dep.log && (
            <div>
              <p className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Build/Deploy Log</p>
              <pre className="max-h-48 overflow-y-auto rounded-lg border border-white/[0.06] bg-[var(--canvas)] p-3 font-mono text-xs text-slate-400 whitespace-pre-wrap">
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
    <Modal title="Deployment Details" description={`${dep.serverId} · ${dep.image}`} onClose={onClose} wide className="max-w-6xl">
      <div className="space-y-4">
        <DashHeader
          icon={Box}
          eyebrow="Server deployment"
          title="Deployment Details"
          pill={{ tone: cfg.tone, label: dep.status.replace(/_/g, " ") }}
          description={dep.image}
          tags={[dep.strategy.replace(/_/g, "-"), dep.targetGroup ?? ""].filter(Boolean)}
          meta={[
            { label: "Server", value: <span key="srv" className="font-mono">{dep.serverId}</span> },
            { label: "Created", value: formatDate(dep.createdAt) },
          ]}
        />
        <InfoCard icon={Box} title="Deployment Information" rows={[
          ["Server", <span key="srv" className="font-mono text-slate-200">{dep.serverId}</span>],
          ["Image", <span key="img" className="text-slate-200">{dep.image}</span>],
          ["Strategy", <Pill key="strat" tone={dep.strategy === "blue_green" ? "blue" : dep.strategy === "rolling" ? "yellow" : "neutral"}>{dep.strategy.replace(/_/g, "-")}</Pill>],
          ["Status", <span key="st" className="inline-flex items-center gap-1.5"><StatusIcon size={12} className={cn(dep.status === "completed" && "text-emerald-400", dep.status === "failed" && "text-red-400")} /><Pill tone={cfg.tone}>{dep.status.replace(/_/g, " ")}</Pill></span>],
          ["Target group", <span key="tg" className="text-slate-200">{dep.targetGroup ?? "—"}</span>],
          ["Created", <span key="created" className="text-slate-200">{formatDate(dep.createdAt)}</span>],
          ...(dep.healthCheckPath ? [["Health check", <span key="hc" className="text-slate-200">{dep.healthCheckPath}:{dep.healthCheckPort}</span>] as [string, ReactNode]] : []),
        ]} />
        {dep.error && (
          <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
            {dep.error}
          </div>
        )}
        {dep.log && (
          <div>
            <p className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Deployment Log</p>
            <pre className="max-h-48 overflow-y-auto rounded-lg border border-white/[0.06] bg-[var(--canvas)] p-3 font-mono text-xs text-slate-400 whitespace-pre-wrap">
              {dep.log}
            </pre>
          </div>
        )}
      </div>
    </Modal>
  );
}
