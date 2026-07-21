"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Play, Square, RotateCcw, Pause, Trash2, RefreshCw, Plus, Terminal, Download,
} from "lucide-react";
import {
  listContainers, operateContainer, deleteContainer, getContainerLogs, getContainerStats,
  type DockerContainerInfo,
} from "@/lib/api/docker";
import { Btn, Card, EmptyState, Input, cn } from "@/components/admin/admin-ui";
import { ConfirmDialog } from "@/components/ui/primitives";
import { ContainerCreateModal } from "@/components/docker/container-create-modal";

function formatDate(ts: string): string {
  if (!ts) return "";
  try {
    return new Date(ts).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric", hour: "2-digit", minute: "2-digit" });
  } catch {
    return ts;
  }
}

function stateTone(state: string): string {
  switch (state) {
    case "running": return "text-emerald-400 bg-emerald-900/30 border-emerald-500/30";
    case "exited":
    case "stopped": return "text-red-400 bg-red-900/30 border-red-500/30";
    case "paused": return "text-amber-400 bg-amber-900/30 border-amber-500/30";
    default: return "text-slate-400 bg-white/[0.03] border-white/10";
  }
}

export function ContainersView() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [logContainer, setLogContainer] = useState<DockerContainerInfo | null>(null);
  const [statsMap, setStatsMap] = useState<Record<string, { cpu: string; mem: string }>>({});
  const [deleteTarget, setDeleteTarget] = useState<DockerContainerInfo | null>(null);

  const containersQuery = useQuery({
    queryKey: ["docker", "containers"],
    queryFn: () => listContainers({ all: true }),
    refetchInterval: 15_000,
  });

  const containers = containersQuery.data ?? [];

  const operateMut = useMutation({
    mutationFn: ({ id, action, nodeId }: { id: string; action: "start" | "stop" | "restart" | "pause" | "unpause"; nodeId?: string }) =>
      operateContainer(id, action, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "containers"] }); },
  });

  const deleteMut = useMutation({
    mutationFn: ({ id, nodeId }: { id: string; nodeId?: string }) => deleteContainer(id, true, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "containers"] }); setDeleteTarget(null); },
  });

  const fetchStats = useCallback(async (id: string, nodeId?: string) => {
    try {
      const data = await getContainerStats(id, nodeId) as { stats?: { cpuPercent?: number; memoryBytes?: number; memoryLimit?: number } };
      const s = data?.stats;
      if (s) {
        setStatsMap((prev) => ({
          ...prev,
          [id]: {
            cpu: s.cpuPercent != null ? `${s.cpuPercent.toFixed(1)}%` : "",
            mem: s.memoryBytes != null ? `${(s.memoryBytes / 1024 / 1024).toFixed(0)}MB` : "",
          },
        }));
      }
    } catch {
      // stats unavailable
    }
  }, []);

  const filtered = containers.filter(
    (c) => !search || c.name.toLowerCase().includes(search.toLowerCase()) || c.image.toLowerCase().includes(search.toLowerCase()) || c.id.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <div className="flex-1">
          <Input placeholder="Search containers..." value={search} onChange={setSearch} />
        </div>
        <Btn tone="ghost" onClick={() => void containersQuery.refetch()}>
          <RefreshCw size={14} />
        </Btn>
        <Btn tone="primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> Create Container
        </Btn>
      </div>

      <Card>
        {containersQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading containers...</div>
        ) : containersQuery.isError ? (
          <div className="p-4 text-sm text-red-400">Failed to load containers. Ensure Beacon is running and nodes are configured.</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Terminal} message={search ? "No containers match your search." : "No containers found. Pull an image and create one."} title={search ? "No results" : "No containers"} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Image</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Ports</th>
                  <th className="px-4 py-3">Node</th>
                  <th className="px-4 py-3">CPU</th>
                  <th className="px-4 py-3">Memory</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((c) => (
                  <tr key={c.id} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                    <td className="max-w-[180px] truncate px-4 py-3 font-medium text-slate-200" title={c.name}>{c.name || c.id.slice(0, 12)}</td>
                    <td className="max-w-[200px] truncate px-4 py-3 text-slate-400" title={c.image}>{c.image}</td>
                    <td className="px-4 py-3">
                      <span className={cn("inline-block rounded border px-2 py-0.5 text-[10px] font-semibold uppercase", stateTone(c.state))}>{c.state}</span>
                    </td>
                    <td className="max-w-[150px] truncate px-4 py-3 font-mono text-[11px] text-slate-400" title={c.ports}>{c.ports || "-"}</td>
                    <td className="px-4 py-3 text-slate-400">{c.nodeName || c.nodeId?.slice(0, 8)}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">{statsMap[c.id]?.cpu ?? "-"}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">{statsMap[c.id]?.mem ?? "-"}</td>
                    <td className="px-4 py-3 text-slate-400">{formatDate(c.created)}</td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-1">
                        {c.state === "running" ? (
                          <>
                            <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-amber-400" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "pause", nodeId: c.nodeId })} title="Pause" type="button"><Pause size={13} /></button>
                            <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-red-400" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "stop", nodeId: c.nodeId })} title="Stop" type="button"><Square size={13} /></button>
                            <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-blue-400" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "restart", nodeId: c.nodeId })} title="Restart" type="button"><RotateCcw size={13} /></button>
                          </>
                        ) : c.state === "paused" ? (
                          <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-emerald-400" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "unpause", nodeId: c.nodeId })} title="Unpause" type="button"><Play size={13} /></button>
                        ) : (
                          <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-emerald-400" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "start", nodeId: c.nodeId })} title="Start" type="button"><Play size={13} /></button>
                        )}
                        <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-slate-200" onClick={() => { setLogContainer(c); void fetchStats(c.id, c.nodeId); }} title="Logs / Stats" type="button"><Terminal size={13} /></button>
                        <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-red-400" onClick={() => setDeleteTarget(c)} title="Delete" type="button"><Trash2 size={13} /></button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate && (
        <ContainerCreateModal onClose={() => setShowCreate(false)} onCreated={() => { queryClient.invalidateQueries({ queryKey: ["docker", "containers"] }); setShowCreate(false); }} />
      )}

      {logContainer && (
        <ContainerLogsModal container={logContainer} onClose={() => setLogContainer(null)} stats={statsMap[logContainer.id]} />
      )}

      <ConfirmDialog
        closeAction={() => setDeleteTarget(null)}
        confirmAction={() => deleteMut.mutate({ id: deleteTarget!.id, nodeId: deleteTarget!.nodeId })}
        confirmLabel="Delete"
        destructive
        loading={deleteMut.isPending}
        open={!!deleteTarget}
        title="Delete Container"
        description={`Are you sure you want to delete "${deleteTarget?.name || deleteTarget?.id}"? This cannot be undone.`}
      />
    </div>
  );
}

function ContainerLogsModal({ container, onClose, stats }: { container: DockerContainerInfo; onClose: () => void; stats?: { cpu: string; mem: string } }) {
  const [tail] = useState(200);
  const [autoScroll, setAutoScroll] = useState(true);
  const logRef = useRef<HTMLDivElement>(null);

  const logsQuery = useQuery({
    queryKey: ["docker", "logs", container.id, tail],
    queryFn: () => getContainerLogs(container.id, tail, container.nodeId),
    refetchInterval: 5_000,
  });

  const logs = logsQuery.data ?? "";

  useEffect(() => {
    if (autoScroll && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  const handleDownload = useCallback(() => {
    const blob = new Blob([logs], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${container.name || container.id}-logs.log`;
    a.click();
    URL.revokeObjectURL(url);
  }, [logs, container]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="max-h-[90vh] w-full max-w-4xl overflow-hidden rounded-xl border border-white/[0.06] bg-[#1a1f2e] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.06] px-5 py-3">
          <div>
            <h3 className="font-semibold text-slate-100">{container.name || container.id.slice(0, 12)}</h3>
            <p className="text-xs text-slate-400">{container.image} &middot; {container.state}</p>
          </div>
          <div className="flex items-center gap-3">
            {stats && (
              <span className="text-xs text-slate-400">
                CPU: {stats.cpu} &middot; Mem: {stats.mem}
              </span>
            )}
            <button className="rounded p-1 text-slate-400 hover:bg-white/[0.06] hover:text-slate-200" onClick={handleDownload} title="Download logs" type="button"><Download size={14} /></button>
            <button className="rounded p-1 text-slate-400 hover:bg-white/[0.06]" onClick={() => setAutoScroll(!autoScroll)} title="Toggle auto-scroll" type="button"><span className={cn("text-xs", autoScroll ? "text-emerald-400" : "text-slate-500")}>Auto</span></button>
            <button className="rounded p-1 text-slate-400 hover:bg-white/[0.06] hover:text-white" onClick={onClose} type="button">&times;</button>
          </div>
        </div>
        <div ref={logRef} className="h-96 overflow-y-auto p-4 font-mono text-xs leading-relaxed text-slate-300">
          {logsQuery.isLoading ? (
            <p className="text-slate-500">Loading logs...</p>
          ) : logs ? (
            logs.split("\n").map((line, i) => <div key={i} className="whitespace-pre-wrap break-all">{line || "\u00A0"}</div>)
          ) : (
            <p className="text-slate-500">No logs available.</p>
          )}
        </div>
      </div>
    </div>
  );
}
