"use client";

// Container file browser for a *server's* running workload container.
//
// This is the UI for the /servers/:id/container/files API: browsing, reading,
// editing, uploading and deleting files INSIDE the container image layer (not
// the mounted data directory, which the server file manager already covers).
// Everything here addresses the server; the API + daemon resolve which
// container that means, so a browser can never target an arbitrary container.

import { useCallback, useMemo, useState, type DragEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronRight, Download, File, FileText, Folder, FolderUp, Loader2, RefreshCw, Save, Search, Trash2, Upload, X,
} from "lucide-react";
import { cn, errorMessage, formatBytes, formatDate } from "@/lib/utils";
import {
  createContainerDir, deleteContainerFile, downloadContainerFile, listContainerFiles,
  readContainerFile, uploadContainerFiles, writeContainerFile,
  type ContainerFileEntry,
} from "@/lib/api/container-files";
import { useConfirm } from "@/components/ui/confirm-dialog";

const btn = cn(
  "inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-[var(--line)] bg-white/[0.03]",
  "px-3 text-xs font-semibold text-[var(--text)] transition-all",
  "hover:bg-white/[0.06] hover:border-[var(--line-strong)] disabled:cursor-not-allowed disabled:opacity-40",
);

const iconBtn = cn(
  "rounded-lg p-2 text-[var(--text-subtle)] transition-all",
  "hover:bg-white/[0.06] hover:text-white disabled:cursor-not-allowed disabled:opacity-40",
);

// MAX_UPLOAD_BYTES mirrors the daemon's signed-streaming-upload ceiling: the
// node authenticates /files/upload with an empty-body signature and caps the
// streamed body at 8 MiB, so oversized picks are refused here with a clear
// message instead of a bare 413 from the node.
const MAX_UPLOAD_BYTES = 8 * 1024 * 1024;

function joinPath(directory: string, name: string): string {
  if (directory === "/" || directory === "") return `/${name}`;
  return `${directory.replace(/\/$/, "")}/${name}`;
}

function parentPath(directory: string): string {
  if (directory === "/" || directory === "") return "/";
  const trimmed = directory.replace(/\/$/, "");
  const index = trimmed.lastIndexOf("/");
  return index <= 0 ? "/" : trimmed.slice(0, index);
}

function Breadcrumbs({ directory, onOpen }: { directory: string; onOpen: (path: string) => void }) {
  const parts = directory.split("/").filter(Boolean);
  return (
    <nav aria-label="Container path" className="flex min-w-0 items-center gap-1 overflow-x-auto text-sm">
      <button className="shrink-0 font-semibold text-[var(--text)] transition-colors hover:text-white" onClick={() => onOpen("/")} type="button">
        /
      </button>
      {parts.map((part, index) => {
        const path = "/" + parts.slice(0, index + 1).join("/");
        return (
          <span className="flex shrink-0 items-center gap-1" key={path}>
            <ChevronRight className="shrink-0 text-[var(--text-subtle)]/60" size={14} />
            <button
              className="max-w-[140px] truncate text-[var(--text-subtle)] transition-colors hover:text-white sm:max-w-[240px]"
              onClick={() => onOpen(path)}
              type="button"
            >
              {part}
            </button>
          </span>
        );
      })}
    </nav>
  );
}

export function ContainerFilesView({
  serverId,
  serverName,
  onClose,
}: {
  serverId: string;
  serverName?: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const [directory, setDirectory] = useState("/home/container");
  const [search, setSearch] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [editing, setEditing] = useState<ContainerFileEntry | null>(null);
  const [content, setContent] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [loadingFile, setLoadingFile] = useState(false);

  const files = useQuery({
    queryKey: ["container-files", serverId, directory],
    queryFn: () => listContainerFiles(serverId, directory),
    retry: 1,
    staleTime: 10_000,
  });

  const entries = useMemo(() => {
    const list = files.data?.entries ?? [];
    const term = search.trim().toLowerCase();
    return list
      .filter((entry) => !term || entry.name.toLowerCase().includes(term))
      .sort((a, b) => (a.isDir === b.isDir ? a.name.localeCompare(b.name) : a.isDir ? -1 : 1));
  }, [files.data, search]);

  const openDirectory = useCallback((path: string) => {
    setDirectory(path);
    setEditing(null);
    setContent("");
    setLoaded(false);
    setDirty(false);
    setError("");
  }, []);

  const run = useCallback(async (label: string, action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (actionError) {
      setError(errorMessage(actionError, `${label} failed.`));
      throw actionError;
    } finally {
      setBusy(false);
    }
  }, []);

  const refresh = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["container-files", serverId, directory] });
  }, [queryClient, serverId, directory]);

  const openFile = useCallback(
    async (entry: ContainerFileEntry) => {
      if (entry.isDir) {
        openDirectory(entry.path);
        return;
      }
      setEditing(entry);
      setContent("");
      setLoaded(false);
      setDirty(false);
      setLoadingFile(true);
      setError("");
      try {
        const result = await readContainerFile(serverId, entry.path);
        if (result.binary) {
          setError("This file is not text — use Download to inspect it.");
          return;
        }
        setContent(result.text);
        setLoaded(true);
      } catch (loadError) {
        setError(errorMessage(loadError, "File could not be read."));
      } finally {
        setLoadingFile(false);
      }
    },
    [openDirectory, serverId],
  );

  const saveFile = useCallback(() => {
    if (!editing) return;
    void run("Saving", async () => {
      await writeContainerFile(serverId, editing.path, content);
      setDirty(false);
      refresh();
    }).catch(() => {});
  }, [content, editing, refresh, run, serverId]);

  const uploadFiles = useCallback(
    async (list: File[]) => {
      if (!list.length) return;
      const oversized = list.filter((file) => file.size > MAX_UPLOAD_BYTES);
      const accepted = list.filter((file) => file.size <= MAX_UPLOAD_BYTES);
      if (oversized.length) {
        setError(`Skipped ${oversized.map((file) => file.name).join(", ")}: larger than the ${formatBytes(MAX_UPLOAD_BYTES, 0)} per-file upload limit`);
      }
      if (!accepted.length) return;
      await run(`Uploading ${accepted.length} file${accepted.length === 1 ? "" : "s"}`, async () => {
        const result = await uploadContainerFiles(serverId, directory, accepted);
        if (result.failed?.length) {
          setError(`Some files were skipped: ${result.failed.map((f) => `${f.file} (${f.error})`).join(", ")}`);
        }
        refresh();
      }).catch(() => {});
    },
    [directory, refresh, run, serverId],
  );

  const removeEntry = useCallback(
    async (entry: ContainerFileEntry) => {
      const confirmed = await confirm({
        title: entry.isDir ? "Delete directory" : "Delete file",
        description: `${entry.path} will be removed from the running container. Files restored from the image on redeploy are unaffected, but this change is immediate and irreversible.`,
        confirmLabel: "Delete",
        danger: true,
      });
      if (!confirmed) return;
      await run("Deleting", async () => {
        await deleteContainerFile(serverId, entry.path);
        if (editing?.path === entry.path) {
          setEditing(null);
          setContent("");
          setDirty(false);
        }
        refresh();
      }).catch(() => {});
    },
    [confirm, editing, refresh, run, serverId],
  );

  const downloadEntry = useCallback(
    async (entry: ContainerFileEntry) => {
      await run("Downloading", async () => {
        const blob = await downloadContainerFile(serverId, entry.path);
        const url = URL.createObjectURL(blob);
        const anchor = document.createElement("a");
        anchor.href = url;
        anchor.download = entry.name;
        anchor.click();
        URL.revokeObjectURL(url);
      }).catch(() => {});
    },
    [run, serverId],
  );

  const [creating, setCreating] = useState(false);
  const [newFolder, setNewFolder] = useState("");
  const submitFolder = useCallback(() => {
    const name = newFolder.trim();
    if (!name || name.includes("/") || name === "." || name === "..") {
      setError("Folder names cannot contain slashes or traversal segments.");
      return;
    }
    setCreating(false);
    setNewFolder("");
    void run("Creating folder", async () => {
      await createContainerDir(serverId, joinPath(directory, name));
      refresh();
    }).catch(() => {});
  }, [directory, newFolder, refresh, run, serverId]);

  const handleDrop = useCallback(
    (event: DragEvent) => {
      event.preventDefault();
      event.stopPropagation();
      setDragging(false);
      const dropped = Array.from(event.dataTransfer.files);
      if (dropped.length) void uploadFiles(dropped);
    },
    [uploadFiles],
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      {renderConfirm()}
      {dragging ? (
        <div aria-live="assertive" className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" role="status">
          <div className="flex flex-col items-center gap-4 rounded-2xl border-2 border-dashed border-[var(--brand)]/60 bg-[var(--surface-raised)]/90 px-16 py-12 text-center shadow-2xl">
            <Upload className="h-10 w-10 text-[var(--brand)]" />
            <p className="text-lg font-bold text-[var(--text)]">Drop files to upload into {directory}</p>
          </div>
        </div>
      ) : null}

      <div
        className="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] shadow-2xl"
        onDragEnter={(event) => { event.preventDefault(); setDragging(true); }}
        onDragLeave={(event) => { event.preventDefault(); if (event.currentTarget.contains(event.relatedTarget as Node)) return; setDragging(false); }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={handleDrop}
      >
        <div className="flex items-center justify-between gap-3 border-b border-[var(--line)] bg-[var(--surface)] px-5 py-3">
          <div className="min-w-0">
            <h3 className="truncate font-semibold text-slate-100">{serverName || serverId} — container files</h3>
            <p className="truncate text-xs text-[var(--text-subtle)]">Operates inside the running workload container on its node</p>
          </div>
          <button aria-label="Close" className={iconBtn} onClick={onClose} type="button"><X size={16} /></button>
        </div>

        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--line)] px-4 py-2">
          <Breadcrumbs directory={directory} onOpen={openDirectory} />
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <label className="relative">
              <Search className="pointer-events-none absolute left-3 top-2.5 size-[14px] text-[var(--text-subtle)]" />
              <span className="sr-only">Filter entries</span>
              <input
                className="h-9 w-36 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-9 pr-3 text-xs text-[var(--text)] outline-none transition-all placeholder:text-[var(--text-subtle)] focus:border-[var(--brand)]"
                onChange={(event) => setSearch(event.target.value)}
                placeholder="Filter…"
                type="search"
                value={search}
              />
            </label>
            <button className={btn} disabled={busy || directory === "/"} onClick={() => openDirectory(parentPath(directory))} type="button">
              <FolderUp size={14} /><span className="hidden sm:inline">Up</span>
            </button>
            <button className={btn} disabled={busy} onClick={() => { setCreating(true); setNewFolder(""); }} type="button">
              <Folder size={14} /><span className="hidden sm:inline">New folder</span>
            </button>
            <label className={cn(btn, "cursor-pointer")} title={`Upload files (up to ${formatBytes(MAX_UPLOAD_BYTES, 0)} each)`}>
              <Upload size={14} /><span className="hidden sm:inline">Upload</span>
              <input
                className="sr-only"
                disabled={busy}
                multiple
                onChange={(event) => {
                  const picked = Array.from(event.target.files ?? []);
                  event.target.value = "";
                  void uploadFiles(picked);
                }}
                type="file"
              />
            </label>
            <button className={btn} disabled={busy || files.isFetching} onClick={refresh} type="button">
              <RefreshCw size={14} />
            </button>
          </div>
        </div>

        {creating ? (
          <div className="flex items-center gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-4 py-2">
            <input
              autoFocus
              className="h-8 flex-1 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-xs text-[var(--text)] outline-none focus:border-[var(--brand)]"
              onChange={(event) => setNewFolder(event.target.value)}
              onKeyDown={(event) => { if (event.key === "Enter") submitFolder(); if (event.key === "Escape") setCreating(false); }}
              placeholder="new-folder-name"
              value={newFolder}
            />
            <button className={btn} onClick={submitFolder} type="button">Create</button>
            <button className={btn} onClick={() => setCreating(false)} type="button">Cancel</button>
          </div>
        ) : null}

        {error ? (
          <div className="mx-4 mt-3 flex items-start gap-2 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">
            <span className="flex-1">{error}</span>
            <button className="shrink-0 font-semibold underline" onClick={() => setError("")} type="button">Dismiss</button>
          </div>
        ) : null}

        <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
          <div className="min-h-0 overflow-auto border-b border-[var(--line)] lg:border-b-0 lg:border-r">
            {files.isLoading ? (
              <div className="flex items-center gap-2 p-5 text-sm text-[var(--text-subtle)]"><Loader2 className="animate-spin" size={14} /> Listing {directory}…</div>
            ) : files.isError ? (
              <div className="p-5 text-sm text-amber-300">
                {errorMessage(files.error, "The container listing could not be loaded.")}
                <button className="ml-2 font-semibold underline" onClick={() => void files.refetch()} type="button">Retry</button>
              </div>
            ) : entries.length === 0 ? (
              <div className="p-10 text-center text-sm text-[var(--text-subtle)]">
                {search ? "No entries match this filter." : "This directory is empty."}
              </div>
            ) : (
              <ul className="divide-y divide-white/[0.04]">
                {entries.map((entry) => (
                  <li className="flex items-center gap-2 px-4 py-2 transition-colors hover:bg-white/[0.03]" key={entry.path}>
                    {entry.isDir ? <Folder size={14} className="shrink-0 text-sky-400" /> : <FileText size={14} className="shrink-0 text-[var(--text-subtle)]" />}
                    <button
                      className="min-w-0 flex-1 truncate text-left font-mono text-sm text-slate-200 transition-colors hover:text-[var(--brand)]"
                      onClick={() => void openFile(entry)}
                      title={entry.path}
                      type="button"
                    >
                      {entry.name}
                      {entry.type === "symlink" ? <span className="ml-2 text-[10px] uppercase text-[var(--text-subtle)]">link</span> : null}
                    </button>
                    {!entry.isDir ? <span className="shrink-0 text-[11px] text-[var(--text-subtle)]">{formatBytes(entry.size)}</span> : null}
                    <span className="hidden w-[130px] shrink-0 truncate text-[11px] text-[var(--text-subtle)] sm:block" title={entry.modified}>{formatDate(entry.modified, "")}</span>
                    {!entry.isDir ? (
                      <button aria-label={`Download ${entry.name}`} className={iconBtn} disabled={busy} onClick={() => void downloadEntry(entry)} title="Download" type="button"><Download size={13} /></button>
                    ) : null}
                    <button aria-label={`Delete ${entry.name}`} className={cn(iconBtn, "hover:text-red-400")} disabled={busy} onClick={() => void removeEntry(entry)} title="Delete" type="button"><Trash2 size={13} /></button>
                  </li>
                ))}
              </ul>
            )}
            {files.data?.truncated ? (
              <p className="px-4 py-3 text-[11px] text-amber-300/80">This directory has more entries than the listing cap — the list is truncated.</p>
            ) : null}
          </div>

          <div className="flex min-h-0 flex-col">
            <div className="flex items-center justify-between gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-4 py-2 text-xs font-semibold uppercase tracking-wider text-[var(--text-subtle)]">
              <span className="truncate">{editing ? editing.path : "Preview"}</span>
              {editing ? (
                <div className="flex shrink-0 items-center gap-2">
                  <button className={btn} onClick={() => { setEditing(null); setContent(""); setLoaded(false); setDirty(false); }} type="button">Close</button>
                  <button className={cn(btn, "border-[var(--brand)]/30 bg-[var(--brand-subtle)] text-white")} disabled={!loaded || !dirty || busy} onClick={saveFile} type="button">
                    <Save size={14} />Save
                  </button>
                </div>
              ) : null}
            </div>
            <div className="min-h-[240px] flex-1 overflow-hidden p-3">
              {!editing ? (
                <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-xs text-[var(--text-subtle)]">
                  <File size={26} strokeWidth={1} />
                  <p>Select a file to view or edit it. Text files open in an editor; binary files offer a download.</p>
                  <p>Changes apply to the live container and are lost when the container is recreated.</p>
                </div>
              ) : loadingFile ? (
                <div className="flex items-center gap-2 text-sm text-[var(--text-subtle)]"><Loader2 className="animate-spin" size={14} /> Reading…</div>
              ) : !loaded ? (
                <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
                  <p className="text-xs text-[var(--text-subtle)]">This file cannot be edited here.</p>
                  <button className={btn} onClick={() => void downloadEntry(editing)} type="button"><Download size={14} />Download</button>
                </div>
              ) : (
                <div className="h-full min-h-[220px] overflow-hidden rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-3">
                  <textarea
                    className="h-full w-full resize-none bg-transparent font-mono text-xs leading-relaxed text-[var(--text)] outline-none"
                    onChange={(event) => { setContent(event.target.value); setDirty(true); }}
                    spellCheck={false}
                    value={content}
                  />
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
