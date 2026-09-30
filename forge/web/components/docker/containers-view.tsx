"use client";

import { useState, useRef, useEffect, useCallback, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Play, Square, RotateCcw, Pause, Trash2, RefreshCw, Plus, Terminal, Download, Folder, FolderOpen, FileText, ChevronLeft,
} from "lucide-react";
import {
  listContainers, operateContainer, deleteContainer, getContainerLogs, getContainerStats,
  listContainerFiles, readContainerFile,
  type DockerContainerInfo,
} from "@/lib/api/docker";
import { Btn, Card, CardHeader, SectionHeader, EmptyState, Input, cn, AdminLoadingState, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd, Pill } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { ConfirmDialog, Pagination } from "@/components/ui/primitives";
import { useToast } from "@/components/ui/toast";
import { ContainerCreateModal } from "@/components/docker/container-create-modal";
import { ContainerFilesView } from "@/components/docker/container-files-view";

function formatDate(ts: string): string {
  if (!ts) return "—";
  try {
    return new Date(ts).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric", hour: "2-digit", minute: "2-digit" });
  } catch {
    return ts;
  }
}

/**
 * Container state as a canonical tone. Running is ok, paused needs attention
 * (warn), exited/stopped are observed-inactive (neutral); anything unreported
 * or unrecognised resolves to unknown rather than a plausible colour.
 */
function containerTone(state: string): "ok" | "warn" | "neutral" | "unknown" {
  const s = (state ?? "").toLowerCase();
  if (s === "running") return "ok";
  if (s === "paused") return "warn";
  if (s === "exited" || s === "stopped" || s === "created") return "neutral";
  if (!s) return "unknown";
  return "unknown";
}

export function ContainersView() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [logContainer, setLogContainer] = useState<DockerContainerInfo | null>(null);
  const [filesContainer, setFilesContainer] = useState<DockerContainerInfo | null>(null);
  // Workload containers open the server-scoped file manager: the API resolves
  // the container from the server binding, so only the server id travels.
  const [serverFiles, setServerFiles] = useState<{ serverId: string; name: string } | null>(null);
  const [statsMap, setStatsMap] = useState<Record<string, { cpu: string; mem: string }>>({});
  const [deleteTarget, setDeleteTarget] = useState<DockerContainerInfo | null>(null);
  const [page, setPage] = useState(1);

  const containersQuery = useQuery({
    queryKey: ["docker", "containers"],
    queryFn: () => listContainers({ all: true }),
    refetchInterval: 15_000,
    retry: false,
    staleTime: 10_000,
  });

  const containers = useMemo(() => Array.isArray(containersQuery.data) ? containersQuery.data : [], [containersQuery.data]);

  const operateMut = useMutation({
    mutationFn: ({ id, action, nodeId }: { id: string; action: "start" | "stop" | "restart" | "pause" | "unpause"; nodeId?: string }) =>
      operateContainer(id, action, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "containers"] }); },
    onError: (err, { action }) => toast({ tone: "error", title: `Failed to ${action} container`, message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteMut = useMutation({
    mutationFn: ({ id, nodeId }: { id: string; nodeId?: string }) => deleteContainer(id, true, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "containers"] }); setDeleteTarget(null); },
    onError: (err) => toast({ tone: "error", title: "Failed to delete container", message: err instanceof Error ? err.message : "An error occurred" }),
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

  const filtered = useMemo(
    () => containers.filter(
      (c) => !search || c.name.toLowerCase().includes(search.toLowerCase()) || c.image.toLowerCase().includes(search.toLowerCase()) || c.id.toLowerCase().includes(search.toLowerCase()),
    ),
    [containers, search],
  );

  const PAGE_SIZE = 10;
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const paginated = useMemo(() => {
    const start = (safePage - 1) * PAGE_SIZE;
    return filtered.slice(start, start + PAGE_SIZE);
  }, [filtered, safePage]);

  return (
    <div className="space-y-4">
      <SectionHeader
        title="Containers"
        sub="Docker containers across nodes, with lifecycle controls and per-container logs, stats and files."
        status={<FreshnessBadge state={sourceState(containersQuery, 15_000)} />}
      />
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
        <CardHeader title={containersQuery.isSuccess ? `${filtered.length} containers` : "Containers"} icon={Terminal} />
        {containersQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading containers…" /></div>
        ) : containersQuery.isError ? (
          <div className="space-y-1 p-4 text-sm">
            <p className="text-danger">Containers could not be loaded: {containersQuery.error instanceof Error ? containersQuery.error.message : "request failed"}. This is a failed read, not an empty host.</p>
            <p className="text-xs text-text-subtle">A node running with DAEMON_ALLOW_MOCK_RUNTIME=true answers every runtime call with “container runtime is unavailable” by design — no container operation can succeed there.</p>
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Terminal} message={search ? "No containers match your search." : "No containers found. Pull an image and create one."} title={search ? "No results" : "No containers"} />
        ) : (
          <AdminTable label="Docker containers">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Image</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh>Ports</AdminTh>
              <AdminTh>Node</AdminTh>
              <AdminTh>CPU</AdminTh>
              <AdminTh>Memory</AdminTh>
              <AdminTh>Created</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {paginated.map((c) => (
                <AdminTr key={c.id}>
                  <AdminTd className="max-w-[180px] truncate font-medium text-text" title={c.name}>{c.name || c.id.slice(0, 12)}</AdminTd>
                  <AdminTd className="max-w-[200px] truncate text-text-subtle" title={c.image}>{c.image}</AdminTd>
                  <AdminTd>
                    <Pill tone={containerTone(c.state)}>{c.state || "not reported"}</Pill>
                  </AdminTd>
                  <AdminTd className="max-w-[150px] truncate font-mono text-[11px] text-text-subtle" title={c.ports || "No published ports reported"}>{c.ports || "—"}</AdminTd>
                  <AdminTd className="text-text-subtle" title={c.nodeName || c.nodeId || "Node not reported"}>{c.nodeName || (c.nodeId ? c.nodeId.slice(0, 8) : "—")}</AdminTd>
                  <AdminTd className="font-mono text-xs text-text-subtle" title={statsMap[c.id]?.cpu ? undefined : "Not reported"}>{statsMap[c.id]?.cpu ?? "—"}</AdminTd>
                  <AdminTd className="font-mono text-xs text-text-subtle" title={statsMap[c.id]?.mem ? undefined : "Not reported"}>{statsMap[c.id]?.mem ?? "—"}</AdminTd>
                  <AdminTd className="text-text-subtle">{formatDate(c.created)}</AdminTd>
                  <AdminTd>
                      <div className="flex items-center gap-1">
                        {c.state === "running" ? (
                          <>
                            <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-warn" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "pause", nodeId: c.nodeId })} title="Pause" type="button"><Pause size={13} /></button>
                            <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-danger" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "stop", nodeId: c.nodeId })} title="Stop" type="button"><Square size={13} /></button>
                            <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-info" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "restart", nodeId: c.nodeId })} title="Restart" type="button"><RotateCcw size={13} /></button>
                          </>
                        ) : c.state === "paused" ? (
                          <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-ok" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "unpause", nodeId: c.nodeId })} title="Unpause" type="button"><Play size={13} /></button>
                        ) : (
                          <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-ok" disabled={operateMut.isPending} onClick={() => operateMut.mutate({ id: c.id, action: "start", nodeId: c.nodeId })} title="Start" type="button"><Play size={13} /></button>
                        )}
                        <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-text" onClick={() => { setLogContainer(c); void fetchStats(c.id, c.nodeId); }} title="Logs / Stats" type="button"><Terminal size={13} /></button>
                        {c.serverId ? (
                          <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-info" onClick={() => setServerFiles({ serverId: c.serverId, name: c.name || c.id.slice(0, 12) })} title="Container files" type="button"><FolderOpen size={13} /></button>
                        ) : (
                          <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-info" onClick={() => setFilesContainer(c)} title="Browse files" type="button"><Folder size={13} /></button>
                        )}
                        <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-danger" onClick={() => setDeleteTarget(c)} title="Delete" type="button"><Trash2 size={13} /></button>
                      </div>
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
        )}
        {filtered.length > PAGE_SIZE && (
          <div className="mt-4">
            <Pagination page={safePage} pageCount={totalPages} onPageChange={setPage} label="Containers pagination" />
          </div>
        )}
      </Card>

      {showCreate && (
        <ContainerCreateModal onClose={() => setShowCreate(false)} onCreated={() => { queryClient.invalidateQueries({ queryKey: ["docker", "containers"] }); setShowCreate(false); }} />
      )}

      {logContainer && (
        <ContainerLogsModal container={logContainer} onClose={() => setLogContainer(null)} stats={statsMap[logContainer.id]} />
      )}

      {filesContainer && (
        <ContainerFilesModal container={filesContainer} onClose={() => setFilesContainer(null)} />
      )}

      {serverFiles && (
        <ContainerFilesView serverId={serverFiles.serverId} serverName={serverFiles.name} onClose={() => setServerFiles(null)} />
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
      <div className="max-h-[90vh] w-full max-w-4xl overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] shadow-2xl">
        <div className="flex items-center justify-between border-b border-[var(--line)] px-5 py-3">
          <div>
            <h3 className="font-semibold text-text">{container.name || container.id.slice(0, 12)}</h3>
            <p className="text-xs text-text-subtle">{container.image} &middot; {container.state}</p>
          </div>
          <div className="flex items-center gap-3">
            {stats && (
              <span className="text-xs text-text-subtle">
                CPU: {stats.cpu} &middot; Mem: {stats.mem}
              </span>
            )}
            <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-text" onClick={handleDownload} title="Download logs" type="button"><Download size={14} /></button>
            <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong" onClick={() => setAutoScroll(!autoScroll)} title="Toggle auto-scroll" type="button"><span className={cn("text-xs", autoScroll ? "text-ok" : "text-text-subtle")}>Auto</span></button>
            <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-text" onClick={onClose} type="button">&times;</button>
          </div>
        </div>
        <div ref={logRef} className="h-96 overflow-y-auto p-4 font-mono text-xs leading-relaxed text-text">
          {logsQuery.isLoading ? (
            <p className="text-text-subtle">Loading logs...</p>
          ) : logs ? (
            logs.split("\n").map((line, i) => <div key={i} className="whitespace-pre-wrap break-all">{line || "\u00A0"}</div>)
          ) : (
            <p className="text-text-subtle">No logs available.</p>
          )}
        </div>
      </div>
    </div>
  );
}

function ContainerFilesModal({ container, onClose }: { container: DockerContainerInfo; onClose: () => void }) {
  const [path, setPath] = useState("/");
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const filesQuery = useQuery({
    queryKey: ["docker", "files", container.id, path],
    queryFn: () => listContainerFiles(container.id, path, container.nodeId),
  });
  const fileContentQuery = useQuery({
    queryKey: ["docker", "file-content", container.id, selectedFile],
    queryFn: () => selectedFile ? readContainerFile(container.id, selectedFile, container.nodeId) : Promise.resolve(null),
    enabled: !!selectedFile,
  });

  const raw = filesQuery.data as unknown;
  const entries: Array<{ name: string; isDir?: boolean; dir?: boolean; type?: string; size?: number }> = useMemo(() => {
    if (Array.isArray(raw)) return raw as Array<{ name: string }>;
    if (raw && typeof raw === "object") {
      const obj = raw as Record<string, unknown>;
      if (Array.isArray(obj.files)) return obj.files as Array<{ name: string }>;
      if (Array.isArray(obj.entries)) return obj.entries as Array<{ name: string }>;
      if (Array.isArray(obj.data)) return obj.data as Array<{ name: string }>;
    }
    return [];
  }, [raw]);

  const contentText = useMemo(() => {
    const data = fileContentQuery.data as unknown;
    if (!data) return "";
    if (typeof data === "string") return data;
    if (typeof data === "object" && data !== null) {
      const obj = data as Record<string, unknown>;
      if (typeof obj.content === "string") return obj.content as string;
      if (typeof obj.data === "string") return obj.data as string;
      return JSON.stringify(data, null, 2);
    }
    return String(data);
  }, [fileContentQuery.data]);

  const navigateUp = () => {
    if (path === "/" || path === "") { setPath("/"); return; }
    const trimmed = path.endsWith("/") && path !== "/" ? path.slice(0, -1) : path;
    const idx = trimmed.lastIndexOf("/");
    setPath(idx <= 0 ? "/" : trimmed.slice(0, idx) || "/");
    setSelectedFile(null);
  };

  const openEntry = (name: string, isDir?: boolean) => {
    const entryIsDir = isDir ?? name.endsWith("/");
    const cleanName = name.replace(/\/$/, "");
    const nextPath = path === "/" ? `/${cleanName}` : `${path.replace(/\/$/, "")}/${cleanName}`;
    if (entryIsDir) {
      setPath(nextPath);
      setSelectedFile(null);
    } else {
      setSelectedFile(nextPath);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] shadow-2xl">
        <div className="flex items-center justify-between border-b border-[var(--line)] bg-[var(--surface)] px-5 py-3">
          <div className="flex items-center gap-2 min-w-0">
            <Folder size={16} className="text-[var(--brand)] shrink-0" />
            <h3 className="font-semibold text-text truncate">{container.name || container.id.slice(0, 12)} — Files</h3>
            <span className="hidden sm:inline text-xs text-text-muted">{container.nodeName || container.nodeId?.slice(0, 8)}</span>
          </div>
          <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-text-subtle hover:bg-overlay-strong hover:text-text" onClick={onClose} type="button" aria-label="Close">&times;</button>
        </div>
        <div className="flex items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-5 py-2 text-xs">
          <button onClick={navigateUp} disabled={path === "/"} className="inline-flex items-center gap-1 rounded px-2 py-1 text-text hover:bg-overlay-strong disabled:opacity-40" type="button"><ChevronLeft size={12} /> Up</button>
          <span className="font-mono text-text truncate">{path}</span>
          <span className="ml-auto text-text-muted">{entries.length} entries</span>
        </div>
        <div className="grid flex-1 min-h-0 grid-cols-1 lg:grid-cols-2">
          <div className="overflow-auto border-r border-[var(--line)]">
            {filesQuery.isLoading ? (
              <div className="p-4"><AdminLoadingState label="Loading files…" /></div>
            ) : filesQuery.isError ? (
              <div className="p-4 text-xs text-warn">Cannot list files for this container — ensure the daemon file browser is enabled. {(filesQuery.error as Error)?.message}</div>
            ) : entries.length === 0 ? (
              <div className="p-8 text-center text-sm text-text-subtle">No entries in this directory. {path !== "/" ? "The path may be empty or not a directory." : ""}</div>
            ) : (
              <ul className="divide-y divide-line">
                {entries.map((entry, idx) => {
                  const name = entry.name ?? String(entry);
                  const isDir = (entry.isDir ?? entry.dir ?? (entry.type === "dir")) || name.endsWith("/");
                  return (
                    <li key={`${name}-${idx}`} className="flex items-center gap-2 px-4 py-2 hover:bg-[var(--surface-hover)]">
                      <span className="shrink-0 text-text-muted">{isDir ? <Folder size={14} /> : <FileText size={14} />}</span>
                      <button onClick={() => openEntry(name, isDir)} className="truncate text-left text-sm font-mono text-text hover:text-[var(--brand)]" title={name} type="button">{name}</button>
                      {entry.size != null && !isDir && <span className="ml-auto text-xs text-text-muted">{entry.size} B</span>}
                    </li>
                  );
                })}
              </ul>
            )}
            {selectedFile && (
              <div className="border-t border-[var(--line)] p-3">
                <p className="text-xs text-text-subtle">Selected: <span className="font-mono text-text">{selectedFile}</span></p>
                <button onClick={() => setSelectedFile(null)} className="mt-2 rounded bg-overlay-strong px-2.5 py-1 text-xs text-text hover:bg-overlay-strong" type="button">Clear selection</button>
              </div>
            )}
          </div>
          <div className="flex flex-col min-h-0">
            <div className="border-b border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-xs font-semibold uppercase tracking-wider text-text-subtle">Preview — {selectedFile ? selectedFile : "no file selected"}</div>
            <div className="flex-1 overflow-auto p-4">
              {!selectedFile ? (
                <p className="text-xs text-text-muted">Select a file on the left to preview. Directories open on click. Uses <code className="rounded bg-overlay-strong px-1 py-0.5 font-mono text-[11px]">GET /docker/containers/:id/files?path=</code> + <code className="rounded bg-overlay-strong px-1 py-0.5 font-mono text-[11px]">POST /files/read</code>.</p>
              ) : fileContentQuery.isLoading ? (
                <AdminLoadingState label="Reading file…" />
              ) : fileContentQuery.isError ? (
                <p className="text-xs text-danger">Could not read file: {(fileContentQuery.error as Error).message}</p>
              ) : (
                <pre className="max-h-[50vh] overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-3 text-xs font-mono text-text whitespace-pre-wrap break-all">{contentText || "(empty file)"}</pre>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
