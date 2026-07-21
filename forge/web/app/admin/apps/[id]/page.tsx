"use client";

import { useState, useEffect, useRef, useCallback, use, Suspense } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { toast, Toaster } from "@/components/ui/sonner";
import {
  ArrowLeft, Cloud, Cpu, Database, FileText,
  Globe, HardDrive, History, KeyRound, Power,
  RefreshCw, RotateCcw, Settings, Square, Terminal,
  Wrench, XCircle,
} from "lucide-react";
import {
  fetchApp, fetchAppDeployments, fetchAppLogs, fetchAppDomains, fetchAppBackups,
  addAppDomain, deleteAppDomain, createAppBackup, restoreAppBackup, deleteAppBackup,
  startApp, stopApp, restartApp, triggerDeploy, updateApp,
  typeLabel,
  type ApiAppDetail, type AppDeployment,
} from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";
import { DeployStatusBadge, LogViewer, ResourceGauge, EnvVarEditor, PortMapper, VolumeEditor } from "@/components/admin/AdminAppsShared";
import { formatDate, formatBytes } from "@/lib/utils";

type TabId = "overview" | "deployments" | "configuration" | "logs" | "console" | "domains" | "backups";

const TABS: { id: TabId; label: string; icon: typeof Settings }[] = [
  { id: "overview", label: "Overview", icon: Cpu },
  { id: "deployments", label: "Deployments", icon: History },
  { id: "configuration", label: "Configuration", icon: Settings },
  { id: "logs", label: "Logs", icon: FileText },
  { id: "console", label: "Console", icon: Terminal },
  { id: "domains", label: "Domains", icon: Globe },
  { id: "backups", label: "Backups", icon: Database },
];

function AdminAppDetailContent({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  const [tab, setTab] = useState<TabId>((searchParams.get("tab") as TabId) || "overview");

  const { data: app, isLoading, error } = useQuery({
    queryKey: ["app", id],
    queryFn: () => fetchApp(id),
    enabled: !!id,
    refetchInterval: 10_000,
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <SectionHeader title="Application" sub="Loading..." />
        <div className="p-8 text-center text-sm text-slate-500">Loading application details...</div>
      </div>
    );
  }

  if (error || !app) {
    return (
      <div className="space-y-6">
        <SectionHeader title="Application" sub="Error loading application" />
        <EmptyState icon={XCircle} message={error instanceof Error ? error.message : "Application not found."} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Btn tone="ghost" size="sm" onClick={() => router.push("/admin/apps")}>
          <ArrowLeft size={14} />
        </Btn>
        <SectionHeader
          title={app.name}
          sub={`${typeLabel(app.type)} - ${app.id.slice(0, 8)}...`}
        />
      </div>

      <div className="flex flex-wrap gap-4">
        {app.type === "compose" && (
          <Btn tone="ghost" size="sm" onClick={() => router.push(`/admin/apps/${id}/compose`)}>
            <Wrench size={12} /> Compose View
          </Btn>
        )}
        {app.type === "git" && (
          <Btn tone="ghost" size="sm" onClick={() => router.push(`/admin/apps/${id}/git`)}>
            <Cloud size={12} /> Git Source
          </Btn>
        )}
      </div>

      <div className="flex gap-1 border-b border-white/[0.06]">
        {TABS.map(({ id: tId, label, icon: Icon }) => (
          <button
            key={tId}
            type="button"
            className={cn(
              "flex items-center gap-1.5 px-3 py-2 text-xs font-medium border-b-2 transition -mb-px",
              tab === tId
                ? "border-[#dc2626] text-[#dc2626]"
                : "border-transparent text-slate-500 hover:text-slate-300",
            )}
            onClick={() => setTab(tId)}
          >
            <Icon size={12} />
            {label}
          </button>
        ))}
      </div>

      {tab === "overview" && <OverviewTab app={app} id={id} />}
      {tab === "deployments" && <DeploymentsTab appId={id} />}
      {tab === "configuration" && <ConfigurationTab app={app} id={id} />}
      {tab === "logs" && <LogsTab appId={id} />}
      {tab === "console" && <ConsoleTab appId={id} />}
      {tab === "domains" && <DomainsTab appId={id} />}
      {tab === "backups" && <BackupsTab appId={id} />}
      <Toaster />
    </div>
  );
}

export default function AdminAppDetailPage({ params }: { params: Promise<{ id: string }> }) {
  return (
    <Suspense fallback={
      <div className="space-y-6">
        <SectionHeader title="Application" sub="Loading..." />
        <div className="p-8 text-center text-sm text-slate-500">Loading application details...</div>
      </div>
    }>
      <AdminAppDetailContent params={params} />
    </Suspense>
  );
}

function OverviewTab({ app, id }: { app: ApiAppDetail; id: string }) {
  const qc = useQueryClient();
  const startMut = useMutation({
    mutationFn: () => startApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to start app"),
  });
  const stopMut = useMutation({
    mutationFn: () => stopApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to stop app"),
  });
  const restartMut = useMutation({
    mutationFn: () => restartApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to restart app"),
  });
  const triggerMut = useMutation({
    mutationFn: () => triggerDeploy(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to trigger deploy"),
  });

  const cpuUsage = app.cpuUsage ?? 0;
  const cpuLimit = typeof app.resourceLimits?.cpu === "string" ? parseFloat(app.resourceLimits.cpu) : (app.cpuLimit ?? 1);
  const memUsage = app.memoryUsage ?? 0;
  const memLimit = typeof app.resourceLimits?.memory === "string" ? parseInt(app.resourceLimits.memory) : (app.memoryLimit ?? 1024);
  const diskUsage = app.diskUsage ?? 0;
  const diskLimit = typeof app.resourceLimits?.disk === "string" ? parseInt(app.resourceLimits.disk) : (app.diskLimit ?? 10240);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap gap-3">
        {app.status === "running" && (
          <>
            <Btn tone="warning" onClick={() => stopMut.mutate()} disabled={stopMut.isPending}>
              <Square size={14} /> Stop
            </Btn>
            <Btn tone="ghost" onClick={() => restartMut.mutate()} disabled={restartMut.isPending}>
              <RotateCcw size={14} /> Restart
            </Btn>
          </>
        )}
        {(app.status === "stopped" || app.status === "failed") && (
          <Btn tone="success" onClick={() => startMut.mutate()} disabled={startMut.isPending}>
            <Power size={14} /> Start
          </Btn>
        )}
        <Btn tone="primary" onClick={() => triggerMut.mutate()} disabled={triggerMut.isPending}>
          <RefreshCw size={14} className={triggerMut.isPending ? "animate-spin" : ""} /> Deploy
        </Btn>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Resource Usage" icon={Cpu} />
          <div className="space-y-4 p-4">
            <ResourceGauge label="CPU" value={cpuUsage} limit={cpuLimit} unit="cores" />
            <ResourceGauge label="Memory" value={memUsage} limit={memLimit} unit="MiB" />
            <ResourceGauge label="Disk" value={diskUsage} limit={diskLimit} unit="MiB" />
          </div>
        </Card>

        <Card>
          <CardHeader title="Information" icon={Cloud} />
          <div className="divide-y divide-white/[0.06] text-sm">
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Status</span>
              <DeployStatusBadge status={app.status} type="app" />
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Type</span>
              <span className="text-slate-200">{typeLabel(app.type)}</span>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Image</span>
              <span className="font-mono text-xs text-slate-300">{app.image ?? "—"}</span>
            </div>
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Version</span>
              <span className="text-slate-200">{app.version ?? "—"}</span>
            </div>
            {app.uptime && (
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Uptime</span>
                <span className="text-slate-200">{app.uptime}</span>
              </div>
            )}
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Created</span>
              <span className="text-slate-200">{formatDate(app.createdAt)}</span>
            </div>
            {app.deployedAt && (
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Last Deployed</span>
                <span className="text-slate-200">{formatDate(app.deployedAt)}</span>
              </div>
            )}
            {app.node && (
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Node</span>
                <span className="font-mono text-xs text-slate-300">{app.node}</span>
              </div>
            )}
          </div>
        </Card>
      </div>

      {app.ports.length > 0 && (
        <Card>
          <CardHeader title="Ports" icon={Globe} />
          <div className="divide-y divide-white/[0.06] text-sm">
            {app.ports.map((port) => (
              <div key={`${port.protocol}-${port.containerPort}-${port.hostPort}`} className="flex justify-between px-4 py-3">
                <span className="text-slate-400">{port.name ?? `${port.protocol}/${port.containerPort}`}</span>
                <span className="font-mono text-xs text-slate-300">{port.hostPort}:{port.containerPort}/{port.protocol}</span>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}

function DeploymentsTab({ appId }: { appId: string }) {
  const router = useRouter();
  const { data: deployments = [], isLoading } = useQuery({
    queryKey: ["app-deployments", appId],
    queryFn: () => fetchAppDeployments(appId),
    refetchInterval: 10_000,
  });
  const [selected, setSelected] = useState<AppDeployment | null>(null);

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title={`${deployments?.length ?? 0} deployment${deployments?.length === 1 ? "" : "s"}`}
          icon={History}
          action={
            <Btn tone="ghost" size="sm" onClick={() => router.push(`/admin/apps/${appId}/deployments`)}>
              <History size={12} />
              View Full History
            </Btn>
          }
        />
        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading deployments...</div>
        ) : !deployments || deployments.length === 0 ? (
          <EmptyState icon={History} message="No deployments yet." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Revision</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Source</th>
                  <th className="px-4 py-3">Trigger</th>
                  <th className="px-4 py-3">Started</th>
                  <th className="px-4 py-3">Duration</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {deployments.map((dep) => (
                  <tr
                    key={dep.id}
                    className="hover:bg-white/[0.02] cursor-pointer"
                    onClick={() => setSelected(dep)}
                  >
                    <td className="px-4 py-3 font-mono text-xs text-slate-200">#{dep.revision}</td>
                    <td className="px-4 py-3">
                      <DeployStatusBadge status={dep.status} type="deployment" />
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {typeLabel(dep.source)}
                      {dep.commit && (
                        <span className="ml-1 font-mono text-slate-500">({dep.commit.slice(0, 7)})</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone="neutral">{dep.trigger}</Pill>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">{formatDate(dep.startedAt)}</td>
                    <td className="px-4 py-3 text-xs text-slate-500">
                      {dep.duration != null ? `${dep.duration}s` : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {selected && (
        <Modal title={`Deployment #${selected.revision}`} onClose={() => setSelected(null)} wide>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-slate-400">Status:</span>
                <DeployStatusBadge status={selected.status} type="deployment" />
              </div>
              <div><span className="text-slate-400">Trigger:</span> <span className="text-slate-200">{selected.trigger}</span></div>
              <div><span className="text-slate-400">Started:</span> <span className="text-slate-200">{formatDate(selected.startedAt)}</span></div>
              <div><span className="text-slate-400">Completed:</span> <span className="text-slate-200">{formatDate(selected.completedAt)}</span></div>
              {selected.commit && (
                <div className="col-span-2">
                  <span className="text-slate-400">Commit:</span>
                  <span className="font-mono text-xs text-slate-200 ml-1">{selected.commit}</span>
                </div>
              )}
              {selected.commitMessage && (
                <div className="col-span-2">
                  <span className="text-slate-400">Message:</span>
                  <span className="text-slate-200 ml-1">{selected.commitMessage}</span>
                </div>
              )}
            </div>
            {selected.error && (
              <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
                {selected.error}
              </div>
            )}
            {selected.log && (
              <div>
                <p className="mb-2 text-xs font-semibold text-slate-400">Build/Deploy Log</p>
                <pre className="max-h-48 overflow-y-auto rounded-lg border border-white/[0.06] bg-[#0a0e14] p-3 font-mono text-xs text-slate-400 whitespace-pre-wrap">
                  {selected.log}
                </pre>
              </div>
            )}
          </div>
        </Modal>
      )}
    </div>
  );
}

function ConfigurationTab({ app, id }: { app: ApiAppDetail; id: string }) {
  const qc = useQueryClient();
  const [envVars, setEnvVars] = useState<Record<string, string>>(app.envVars ?? {});
  const [ports, setPorts] = useState(app.ports ?? []);
  const [volumes, setVolumes] = useState(app.volumes ?? []);
  const [cpu, setCpu] = useState(app.resourceLimits?.cpu ?? "");
  const [memory, setMemory] = useState(app.resourceLimits?.memory ?? "");
  const [disk, setDisk] = useState(app.resourceLimits?.disk ?? "");

  const updateMut = useMutation({
    mutationFn: () => updateApp(id, {
      envVars,
      ports,
      volumes,
      cpuLimit: cpu,
      memoryLimit: memory,
      diskLimit: disk,
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["app", id] });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to update app configuration"),
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader title="Resource Limits" icon={Cpu} />
        <div className="grid gap-4 p-4 sm:grid-cols-3">
          <Input label="CPU (cores)" value={cpu} onChange={setCpu} placeholder="1.0" />
          <Input label="Memory (MiB)" value={memory} onChange={setMemory} placeholder="1024" />
          <Input label="Disk (MiB)" value={disk} onChange={setDisk} placeholder="10240" />
        </div>
      </Card>

      <Card>
        <CardHeader title="Environment Variables" icon={KeyRound} />
        <div className="p-4">
          <EnvVarEditor envVars={envVars} onChange={setEnvVars} />
        </div>
      </Card>

      <Card>
        <CardHeader title="Port Mapping" icon={Globe} />
        <div className="p-4">
          <PortMapper ports={ports} onChange={setPorts} />
        </div>
      </Card>

      <Card>
        <CardHeader title="Volume Mounts" icon={HardDrive} />
        <div className="p-4">
          <VolumeEditor volumes={volumes} onChange={setVolumes} />
        </div>
      </Card>

      <div className="flex justify-end gap-3">
        <Btn tone="ghost" onClick={() => {
          setEnvVars(app.envVars ?? {});
          setPorts(app.ports ?? []);
          setVolumes(app.volumes ?? []);
          setCpu(app.resourceLimits?.cpu ?? "");
          setMemory(app.resourceLimits?.memory ?? "");
          setDisk(app.resourceLimits?.disk ?? "");
        }}>
          Reset
        </Btn>
        <Btn tone="primary" onClick={() => updateMut.mutate()} disabled={updateMut.isPending}>
          {updateMut.isPending ? "Saving..." : "Save Configuration"}
        </Btn>
      </div>
      {updateMut.error && (
        <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
          {updateMut.error.message}
        </div>
      )}
    </div>
  );
}

function LogsTab({ appId }: { appId: string }) {
  const { data: logs = [], isLoading } = useQuery({
    queryKey: ["app-logs", appId],
    queryFn: () => fetchAppLogs(appId),
    refetchInterval: 5_000,
  });

  return (
    <Card>
      <CardHeader title="Application Logs" icon={FileText} />
      <div className="p-4">
        <LogViewer logs={logs ?? []} loading={isLoading} />
      </div>
    </Card>
  );
}

function ConsoleTab({ appId }: { appId: string }) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [connected, setConnected] = useState(false);
  const [input, setInput] = useState("");
  const [output, setOutput] = useState<string[]>([]);

  const connect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
    }

    try {
      const url = `${process.env.NEXT_PUBLIC_API_URL ?? (typeof window !== "undefined" ? `${window.location.protocol}//${window.location.host}/api/v1` : "http://localhost:8080/api/v1")}`;
      const wsBase = url.replace("http:", "ws:").replace("https:", "wss:");
      const socket = new WebSocket(`${wsBase}/admin/apps/${appId}/ws/console`);

      socket.onopen = () => setConnected(true);
      socket.onclose = () => setConnected(false);
      socket.onmessage = (event) => {
        setOutput((prev) => [...prev.slice(-500), event.data]);
      };
      socket.onerror = () => setConnected(false);

      wsRef.current = socket;
    } catch {
      setConnected(false);
    }
  }, [appId]);

  useEffect(() => {
    return () => {
      if (wsRef.current) wsRef.current.close();
    };
  }, []);

  const send = (e: React.FormEvent) => {
    e.preventDefault();
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN && input.trim()) {
      wsRef.current.send(input + "\n");
      setOutput((prev) => [...prev.slice(-500), `> ${input}`]);
      setInput("");
    }
  };

  return (
    <Card>
      <CardHeader
        title="Console"
        icon={Terminal}
        action={
          <div className="flex items-center gap-2">
            <span className={cn("h-2 w-2 rounded-full", connected ? "bg-emerald-500" : "bg-red-500")} />
            <Btn size="sm" tone={connected ? "ghost" : "primary"} onClick={connect}>
              {connected ? "Reconnect" : "Connect"}
            </Btn>
          </div>
        }
      />
      <div className="p-4 space-y-3">
        <div
          ref={terminalRef}
          className="h-96 overflow-y-auto rounded-lg border border-white/[0.06] bg-[#0a0e14] p-3 font-mono text-xs text-slate-300"
        >
          {output.length === 0 ? (
            <div className="py-8 text-center text-slate-500">
              {connected ? "Waiting for output..." : "Click Connect to start the console session."}
            </div>
          ) : (
            output.map((line, i) => (
              <div key={i} className="whitespace-pre-wrap break-all py-0.5">
                {line}
              </div>
            ))
          )}
        </div>
        <form onSubmit={send} className="flex gap-2">
          <input
            className="flex-1 h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 font-mono text-xs text-slate-100 outline-none"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Type a command..."
            disabled={!connected}
          />
          <Btn type="submit" disabled={!connected || !input.trim()}>
            Send
          </Btn>
        </form>
      </div>
    </Card>
  );
}

function DomainsTab({ appId }: { appId: string }) {
  const qc = useQueryClient();
  const { data: domains = [], isLoading } = useQuery({
    queryKey: ["app-domains", appId],
    queryFn: () => fetchAppDomains(appId),
  });
  const [newDomain, setNewDomain] = useState("");
  const [enableTls, setEnableTls] = useState(false);

  const addMut = useMutation({
    mutationFn: () => addAppDomain(appId, newDomain.trim(), enableTls),
    onSuccess: () => {
      setNewDomain("");
      setEnableTls(false);
      void qc.invalidateQueries({ queryKey: ["app-domains", appId] });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to add domain"),
  });

  const deleteMut = useMutation({
    mutationFn: (domainId: string) => deleteAppDomain(appId, domainId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app-domains", appId] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to delete domain"),
  });

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader title={`${domains?.length ?? 0} domain${domains?.length === 1 ? "" : "s"}`} icon={Globe} />
        <div className="flex flex-wrap items-end gap-3 p-4">
          <div className="flex-1 min-w-[200px]">
            <Input label="New Domain" value={newDomain} onChange={setNewDomain} placeholder="example.com" />
          </div>
          <label className="flex items-center gap-2 text-xs text-slate-400 pb-2">
            <input
              type="checkbox"
              checked={enableTls}
              onChange={(e) => setEnableTls(e.target.checked)}
              className="h-3 w-3 rounded border-white/20 bg-[#161b28] accent-[#dc2626]"
            />
            Enable TLS
          </label>
          <Btn onClick={() => addMut.mutate()} disabled={!newDomain.trim() || addMut.isPending}>
            Add Domain
          </Btn>
        </div>
        {addMut.error && (
          <div className="px-4 pb-3 text-sm text-red-400">{addMut.error.message}</div>
        )}
        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading domains...</div>
        ) : !domains || domains.length === 0 ? (
          <EmptyState icon={Globe} message="No domains configured." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Domain</th>
                  <th className="px-4 py-3">SSL</th>
                  <th className="px-4 py-3">Added</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {domains.map((d) => (
                  <tr key={d.id}>
                    <td className="px-4 py-3 font-mono text-xs text-slate-200">{d.domain}</td>
                    <td className="px-4 py-3">
                      {d.ssl ? (
                        <Pill tone={d.sslStatus === "active" ? "green" : d.sslStatus === "failed" ? "red" : "yellow"}>
                          {d.sslStatus ?? "enabled"}
                        </Pill>
                      ) : (
                        <Pill tone="neutral">none</Pill>
                      )}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">{formatDate(d.createdAt)}</td>
                    <td className="px-4 py-3 text-right">
                      <Btn tone="danger" size="sm" onClick={() => { if (confirm(`Remove ${d.domain}?`)) deleteMut.mutate(d.id); }}>
                        Remove
                      </Btn>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}

function BackupsTab({ appId }: { appId: string }) {
  const qc = useQueryClient();
  const { data: backups = [], isLoading } = useQuery({
    queryKey: ["app-backups", appId],
    queryFn: () => fetchAppBackups(appId),
    refetchInterval: 10_000,
  });

  const createMut = useMutation({
    mutationFn: () => createAppBackup(appId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app-backups", appId] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to create backup"),
  });

  const restoreMut = useMutation({
    mutationFn: (backupId: string) => restoreAppBackup(appId, backupId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app-backups", appId] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to restore backup"),
  });

  const deleteMut = useMutation({
    mutationFn: (backupId: string) => deleteAppBackup(appId, backupId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app-backups", appId] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to delete backup"),
  });

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Btn tone="primary" onClick={() => createMut.mutate()} disabled={createMut.isPending}>
          {createMut.isPending ? "Creating..." : "Create Backup"}
        </Btn>
      </div>

      <Card>
        <CardHeader title={`${backups?.length ?? 0} backup${backups?.length === 1 ? "" : "s"}`} icon={Database} />
        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading backups...</div>
        ) : !backups || backups.length === 0 ? (
          <EmptyState icon={Database} message="No backups yet." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Size</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {backups.map((b) => (
                  <tr key={b.id}>
                    <td className="px-4 py-3 font-mono text-xs text-slate-200">{b.name}</td>
                    <td className="px-4 py-3">
                      <Pill
                        tone={b.status === "completed" ? "green" : b.status === "failed" ? "red" : b.status === "creating" || b.status === "restoring" ? "blue" : "neutral"}
                      >
                        {b.status}
                      </Pill>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {b.size ? formatBytes(b.size) : "—"}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">{formatDate(b.createdAt)}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Btn
                          size="sm"
                          tone="ghost"
                          onClick={() => { if (confirm("Restore this backup?")) restoreMut.mutate(b.id); }}
                          disabled={restoreMut.isPending}
                        >
                          Restore
                        </Btn>
                        <Btn
                          size="sm"
                          tone="danger"
                          onClick={() => { if (confirm("Delete this backup?")) deleteMut.mutate(b.id); }}
                          disabled={deleteMut.isPending}
                        >
                          Delete
                        </Btn>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
