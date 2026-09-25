"use client";

import { useCallback, useEffect, useMemo, useState, type DragEvent, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Archive, CheckSquare, ChevronRight, Download, File, Folder, FolderUp, PackageOpen, RefreshCw, Save, Trash2, Upload, Search, Lock, Copy, Terminal,
} from "lucide-react";
import Link from "next/link";
import { cn, errorMessage, formatBytes } from "@/lib/utils";
import {
  listFiles, readFile, writeFile, createDir, deleteFile, renameFile, copyFile, chmodFile, downloadFile, uploadFile, pullRemoteFile,
  type FileEntry,
} from "@/lib/api/host-files";
import { NodeSelect } from "./node-select";
import { AdminToolbar } from "./admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { toast } from "@/components/ui/sonner";
import { Dialog } from "@/components/ui/primitives";

const btn = cn(
  "inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-[var(--line)] bg-white/[0.03]",
  "px-3 text-xs font-semibold text-[var(--text)] transition-all",
  "hover:bg-white/[0.06] hover:border-[var(--line-strong)] disabled:cursor-not-allowed disabled:opacity-40",
);

const iconBtn = cn(
  "rounded-lg p-2 text-[var(--text-subtle)] transition-all",
  "hover:bg-white/[0.06] hover:text-white disabled:opacity-40 disabled:cursor-not-allowed",
);

function Breadcrumbs({ directory, onOpen }: { directory: string; onOpen: (path: string) => void }) {
  const parts = directory.split("/").filter(Boolean);
  return (
    <nav aria-label="File path" className="flex min-w-0 items-center gap-1 overflow-x-auto text-sm">
      <button
        className="shrink-0 font-semibold text-[var(--text)] hover:text-white transition-colors"
        onClick={() => onOpen("/")}
        type="button"
      >
        root
      </button>
      {parts.map((part, index) => {
        const path = "/" + parts.slice(0, index + 1).join("/");
        return (
          <span className="flex shrink-0 items-center gap-1" key={path}>
            <ChevronRight className="text-[var(--text-subtle)]/60 shrink-0" size={14} />
            <button
              className="text-[var(--text-subtle)] hover:text-white transition-colors truncate max-w-[120px] sm:max-w-[200px]"
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

export function HostFilesView() {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const [directory, setDirectory] = useState("/");
  const [editing, setEditing] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [fileLoaded, setFileLoaded] = useState(false);
  const [status, setStatus] = useState("Ready");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<"name" | "size" | "date">("name");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");
  const [nodeId, setNodeId] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const dragCounter = useState(0);
  const [pullOpen, setPullOpen] = useState(false);
  const [pullUrl, setPullUrl] = useState("");

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ["host-files", nodeId, directory] });
  }, [queryClient, nodeId, directory]);

  const run = useCallback(async (label: string, action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    setStatus(label);
    try {
      await action();
      setStatus(`${label} complete`);
    } catch (actionError) {
      setError(errorMessage(actionError, `${label} failed.`));
      setStatus("Action failed");
    } finally {
      setBusy(false);
    }
  }, []);

  const files = useQuery({
    queryKey: ["host-files", nodeId, directory],
    queryFn: () => listFiles(directory, nodeId || undefined),
    retry: 1,
    staleTime: 10_000,
  });

  const entries = useMemo(() => {
    return (files.data ?? [])
      .filter((entry) => entry.name.toLowerCase().includes(search.trim().toLowerCase()))
      .sort((a, b) => {
        if (a.isDir !== b.isDir) return Number(b.isDir) - Number(a.isDir);
        const cmp =
          sortBy === "size" ? (a.size ?? 0) - (b.size ?? 0) :
          sortBy === "date" ? (a.modTime ?? "").localeCompare(b.modTime ?? "") :
          a.name.localeCompare(b.name);
        return sortDir === "desc" ? -cmp : cmp;
      });
  }, [files.data, search, sortBy, sortDir]);

  // Drag-and-drop for host files – mirrors server files-view behavior, wired to uploadFile
  const handleDragEnter = useCallback((event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    dragCounter[1]((c) => { const next = c + 1; if (next === 1) setDragging(true); return next; });
  }, [dragCounter]);
  const handleDragLeave = useCallback((event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    dragCounter[1]((c) => { const next = Math.max(0, c - 1); if (next === 0) setDragging(false); return next; });
  }, [dragCounter]);
  const handleDragOver = useCallback((event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
  }, []);
  const handleDrop = useCallback(async (event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    setDragging(false);
    dragCounter[1](0);
    if (busy) return;
    const droppedFiles = Array.from(event.dataTransfer.files);
    if (!droppedFiles.length) return;
    await run("Uploading dropped files", async () => {
      for (let index = 0; index < droppedFiles.length; index += 1) {
        const f = droppedFiles[index];
        setUploadProgress(Math.round(((index) / droppedFiles.length) * 100));
        await uploadFile(directory, f, nodeId || undefined);
        setUploadProgress(Math.round(((index + 1) / droppedFiles.length) * 100));
      }
      setUploadProgress(null);
      await refresh();
    });
  }, [busy, directory, nodeId, run, refresh, dragCounter]);

  const openFile = async (path: string) => {
    setEditing(path);
    setContent("");
    setFileLoaded(false);
    setError("");
    setStatus("Loading");
    try {
      const value = await readFile(path, nodeId || undefined);
      setContent(value);
      setFileLoaded(true);
      setStatus("Loaded");
    } catch (loadError) {
      setError(errorMessage(loadError, "File could not be loaded."));
      setStatus("Load failed");
    }
  };

  const save = () =>
    run("Saving", async () => {
      if (!editing || !fileLoaded) return;
      await writeFile(editing, content, nodeId || undefined);
    });

  const [createKind, setCreateKind] = useState<"file" | "folder" | null>(null);
  const [createName, setCreateName] = useState("");
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null);
  const [renameName, setRenameName] = useState("");
  const [copyTarget, setCopyTarget] = useState<FileEntry | null>(null);
  const [copyName, setCopyName] = useState("");
  const [chmodTarget, setChmodTarget] = useState<FileEntry | null>(null);
  const [chmodMode, setChmodMode] = useState("");
  // bulk chmod dialog reuses chmodTarget but for multiple paths
  const [bulkChmodOpen, setBulkChmodOpen] = useState(false);
  const [bulkChmodMode, setBulkChmodMode] = useState("0644");

  const promptName = (kind: "file" | "folder") => {
    setCreateKind(kind);
    setCreateName("");
  };

  const createFolder = () => promptName("folder");

  const submitCreate = () => {
    const value = createName.trim();
    if (!value || value.includes("/") || value === "." || value === "..") {
      if (value) setError("Names cannot contain slashes or path traversal segments.");
      return;
    }
    const kind = createKind;
    setCreateKind(null);
    setCreateName("");
    void run(kind === "folder" ? "Creating folder" : "Creating file", async () => {
      const target = directory === "/" ? "/" + value : directory + "/" + value;
      if (kind === "folder") await createDir(target, nodeId || undefined);
      else await writeFile(target, "", nodeId || undefined);
      await refresh();
    });
  };

  const handleUpload = async (event: FormEvent<HTMLInputElement>) => {
    const input = event.currentTarget;
    const uploadFiles = Array.from(input.files ?? []);
    input.value = "";
    if (!uploadFiles.length) return;
    await run("Uploading", async () => {
      for (let index = 0; index < uploadFiles.length; index += 1) {
        const f = uploadFiles[index];
        setUploadProgress(Math.round(((index) / uploadFiles.length) * 100));
        await uploadFile(directory, f, nodeId || undefined);
      }
      setUploadProgress(null);
      await refresh();
    });
  };

  const handleRename = (entry: FileEntry) => {
    setRenameTarget(entry);
    setRenameName(entry.name);
  };

  const submitRename = () => {
    if (!renameTarget) return;
    const name = renameName.trim();
    if (!name || name === renameTarget.name || name.includes("/")) return;
    const target = renameTarget;
    setRenameTarget(null);
    void run("Renaming", async () => {
      const parentPath = target.path.includes("/") ? target.path.substring(0, target.path.lastIndexOf("/") + 1) : "";
      await renameFile(target.path, parentPath + name, nodeId || undefined);
      await refresh();
    });
  };

  const handleCopy = (entry: FileEntry) => {
    setCopyTarget(entry);
    setCopyName("copy_of_" + entry.name);
  };

  const submitCopy = () => {
    if (!copyTarget) return;
    const name = copyName.trim();
    if (!name || name.includes("/")) return;
    const target = copyTarget;
    setCopyTarget(null);
    void run("Copying", async () => {
      const parentPath = target.path.includes("/") ? target.path.substring(0, target.path.lastIndexOf("/") + 1) : "";
      await copyFile(target.path, parentPath + name, nodeId || undefined);
      await refresh();
    });
  };

  const handleDelete = async (entry: FileEntry) => {
    const confirmed = await confirm({
      title: `Permanently delete ${entry.isDir ? "directory" : "file"} "${entry.name}"?`,
      description: `This ${entry.isDir ? "directory and everything inside it" : "file"} will be removed from the host node. This cannot be undone.`,
      danger: true,
      confirmLabel: "Delete",
    });
    if (!confirmed) return;
    void run("Deleting", async () => {
      await deleteFile(entry.path, nodeId || undefined);
      setSelected((prev) => prev.filter((p) => p !== entry.path));
      await refresh();
    });
  };

  const handleBulkDelete = async () => {
    if (!selected.length) return;
    const confirmed = await confirm({
      title: `Delete ${selected.length} selected ${selected.length === 1 ? "file" : "files"}?`,
      description: `${selected.slice(0, 3).join(", ")}${selected.length > 3 ? `, +${selected.length - 3} more` : ""} will be permanently removed. This cannot be undone.`,
      danger: true,
      confirmLabel: "Delete",
    });
    if (!confirmed) return;
    void run("Deleting selected", async () => {
      for (const p of selected) {
        await deleteFile(p, nodeId || undefined);
      }
      setSelected([]);
      await refresh();
    });
  };

  const handleBulkChmod = () => {
    if (!selected.length) return;
    setBulkChmodMode("0644");
    setBulkChmodOpen(true);
  };
  const submitBulkChmod = () => {
    const mode = bulkChmodMode.trim();
    if (!/^[0-7]{3,4}$/.test(mode)) {
      setError("Permissions must be three or four octal digits.");
      return;
    }
    const paths = [...selected];
    setBulkChmodOpen(false);
    void run("Updating permissions", async () => {
      for (const p of paths) {
        await chmodFile(p, mode, nodeId || undefined);
      }
      setSelected([]);
      await refresh();
    });
  };

  const handleChmod = (entry: FileEntry) => {
    setChmodTarget(entry);
    setChmodMode(entry.mode ?? "");
  };

  const submitChmod = () => {
    if (!chmodTarget) return;
    const mode = chmodMode.trim();
    if (!mode || !/^[0-7]{3,4}$/.test(mode)) {
      if (mode) setError("Permissions must be three or four octal digits.");
      return;
    }
    const target = chmodTarget;
    setChmodTarget(null);
    void run("Changing permissions", async () => {
      await chmodFile(target.path, mode, nodeId || undefined);
      await refresh();
    });
  };

  const handleDownload = (entry: FileEntry) => {
    void run("Downloading", async () => {
      const blob = await downloadFile(entry.path, nodeId || undefined);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(url);
    });
  };

  // Archive / decompress on host: backend has no dedicated archive endpoint.
  // Wire to download for files and show honest disabled state for directories;
  // decompress shows informational toast – keeps UI parity with server files-view
  // without hiding the action (beacon gap: host archive/decompress not yet exposed).
  const handleArchive = (entry: FileEntry) => {
    if (entry.isDir) {
      setError("Host archive for directories is not yet exposed by the beacon – use server file manager for tar.gz archives or download files individually.");
      toast.error("Host archive not available for directories");
      return;
    }
    handleDownload(entry);
  };
  const handleDecompress = (entry: FileEntry) => {
    const isArchive = /\.(zip|tar|tar\.gz|tgz|gz)$/i.test(entry.name);
    if (!isArchive) {
      setError("Only .zip / .tar / .tar.gz archives can be decompressed.");
      return;
    }
    setError("Host decompress is not yet exposed by the beacon host file API – upload the archive to a server container to decompress.");
    toast.error("Host decompress not yet available");
  };

  const handlePull = async () => {
    const raw = pullUrl.trim();
    if (!raw) return;
    let parsed: URL;
    try {
      parsed = new URL(raw);
      if (!/^https?:$/.test(parsed.protocol)) throw new Error();
    } catch {
      setError("Enter a valid HTTP or HTTPS URL.");
      return;
    }
    const name = decodeURIComponent(parsed.pathname.split("/").filter(Boolean).pop() || "downloaded-file");
    const dest = directory === "/" ? "/" + name : directory + "/" + name;
    setPullOpen(false);
    setPullUrl("");
    void run("Pulling URL", async () => {
      // Client-side pull: fetch then upload – keeps host pull wired without a dedicated beacon endpoint.
      const blob = await pullRemoteFile(raw);
      const file = new (File as unknown as new (parts: BlobPart[], name: string, opts?: FilePropertyBag) => File)([blob as BlobPart], name, { type: blob.type || "application/octet-stream" });
      await uploadFile(directory, file, nodeId || undefined);
      await refresh();
      void dest;
    });
  };

  const toggleSort = useCallback((column: "name" | "size" | "date") => {
    setSortBy((prev) => {
      if (prev === column) {
        setSortDir((d) => (d === "asc" ? "desc" : "asc"));
        return prev;
      }
      setSortDir("asc");
      return column;
    });
  }, []);

  useEffect(() => {
    setSearch("");
    setSelected([]);
  }, [directory]);

  // Switching nodes returns to root and closes any open editor, since paths
  // from one node are not meaningful on another.
  const handleNodeChange = useCallback((next: string) => {
    setNodeId(next);
    setDirectory("/");
    setEditing(null);
    setFileLoaded(false);
    setError("");
    setStatus("Ready");
    setSelected([]);
  }, []);

  const allVisibleSelected = entries.length > 0 && entries.every((e) => selected.includes(e.path));

  if (editing) {
    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Breadcrumbs directory={editing} onOpen={(path) => { setEditing(null); setDirectory(path); }} />
          <div className="flex items-center gap-2">
            <span className="text-xs text-[var(--text-subtle)] font-mono" role="status">{status}</span>
            <button className={btn} onClick={() => { setEditing(null); setFileLoaded(false); }} type="button">
              Close
            </button>
            <button
              className="inline-flex h-9 items-center gap-2 rounded-lg bg-[var(--brand)] px-4 text-xs font-bold text-white hover:bg-[var(--brand-hover)] disabled:opacity-40 transition-colors"
              disabled={!fileLoaded || busy}
              onClick={() => void save()}
              type="button"
            >
              <Save size={15} />Save
            </button>
          </div>
        </div>
        {error ? (
          <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">
            {error}
          </div>
        ) : null}
        <div className="h-[65vh] min-h-96 overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface-input)] p-4 font-mono text-sm text-[var(--text)]">
          <textarea
            className="h-full w-full resize-none bg-transparent outline-none leading-relaxed"
            value={content}
            onChange={(e) => { if (fileLoaded) { setContent(e.target.value); setStatus("Edited"); } }}
            readOnly={!fileLoaded}
            spellCheck={false}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="relative space-y-4" onDragEnter={handleDragEnter} onDragLeave={handleDragLeave} onDragOver={handleDragOver} onDrop={handleDrop}>
      {renderConfirm()}
      {/* Drag-and-drop overlay – mirrors server files-view */}
      {dragging ? <div aria-live="assertive" className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" role="status"><div className="flex flex-col items-center gap-4 rounded-2xl border-2 border-dashed border-[var(--brand)]/60 bg-[var(--surface-raised)]/90 px-16 py-12 text-center shadow-2xl"><Upload className="h-12 w-12 text-[var(--brand)] animate-bounce" /><p className="text-lg font-bold text-[var(--text)]">Drop files to upload</p><p className="text-sm text-[var(--text-subtle)]">Files will be uploaded to the current directory</p></div></div> : null}

      <AdminToolbar className="items-center">
        <Breadcrumbs directory={directory} onOpen={setDirectory} />
        <div className="flex flex-wrap items-center gap-2">
          <NodeSelect value={nodeId} onChange={handleNodeChange} />
          <Link
            href={nodeId ? `/admin/terminal?nodeId=${encodeURIComponent(nodeId)}` : "/admin/terminal"}
            className={cn(btn, "gap-1.5 border-[var(--brand)]/20 hover:border-[var(--brand)]/40 hover:bg-[var(--brand-subtle)]")}
            title={nodeId ? `Open host terminal for selected node (${nodeId.slice(0, 8)}) — forwards ?nodeId=` : "Open host terminal (select a node first)"}
          >
            <Terminal size={14} />
            <span className="hidden sm:inline">Terminal</span>
          </Link>
          <label className="relative">
            <Search className="absolute left-3 top-2.5 text-[var(--text-subtle)] pointer-events-none" size={14} />
            <span className="sr-only">Filter files</span>
            <input
              className="h-9 w-40 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-9 pr-3 text-xs text-[var(--text)] outline-none placeholder:text-[var(--text-subtle)] focus:border-[var(--brand)] focus:ring-2 focus:ring-[var(--brand-subtle)] transition-all"
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Filter files..."
              type="search"
              value={search}
            />
          </label>
          <button className={btn} onClick={createFolder} disabled={busy} type="button">
            <Folder size={14} /><span className="hidden sm:inline">New folder</span>
          </button>
          <button className={btn} onClick={() => promptName("file")} disabled={busy} type="button">
            <File size={14} /><span className="hidden sm:inline">New file</span>
          </button>
          <button className={btn} onClick={() => setPullOpen(true)} disabled={busy} type="button">
            <Download size={14} /><span className="hidden sm:inline">Pull URL</span>
          </button>
          <button
            className={btn}
            disabled={directory === "/"}
            onClick={() => setDirectory(directory === "/" ? "/" : directory.substring(0, directory.lastIndexOf("/")) || "/")}
            type="button"
          >
            <FolderUp size={14} /><span className="hidden sm:inline">Go up</span>
          </button>
          <button className={btn} disabled={busy} onClick={() => void refresh()} type="button">
            <RefreshCw size={14} />
          </button>
          <label className={cn(btn, "cursor-pointer")}>
            <Upload size={14} /><span className="hidden sm:inline">Upload</span>
            <input className="sr-only" disabled={busy} multiple onChange={handleUpload} type="file" />
          </label>
        </div>
      </AdminToolbar>

      {/* Upload progress – tokenized */}
      {uploadProgress !== null ? <div className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3" role="status"><div className="flex justify-between text-xs text-[var(--text-subtle)]"><span>Uploading files</span><span>{uploadProgress}%</span></div><div className="mt-2 h-2 overflow-hidden rounded bg-[var(--surface)]"><div className="h-full bg-[var(--brand)] transition-all" style={{ width: `${uploadProgress}%` }} /></div></div> : null}

      {/* Error */}
      {(error || files.isError) ? (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200 flex items-center gap-2" role="alert">
          <span className="flex-1">{error || errorMessage(files.error, "Files could not be loaded.")}</span>
          <button className="shrink-0 underline font-semibold hover:text-red-100 transition-colors" onClick={() => void files.refetch()} type="button">Retry</button>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-[var(--text-subtle)]">
        <label className="flex items-center gap-2 cursor-pointer"><input checked={allVisibleSelected} onChange={(e) => setSelected(e.target.checked ? entries.map((en) => en.path) : [])} type="checkbox" />Select all visible</label>
        <span role="status">{files.isFetching ? "Loading…" : status}{selected.length ? ` · ${selected.length} selected` : ""}</span>
      </div>

      {/* Loading skeleton */}
      {files.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-12 animate-pulse rounded-lg bg-white/[0.04] border border-[var(--line)]" />
          ))}
        </div>
      ) : entries.length === 0 && !files.isError ? (
        <div className="rounded-xl border border-dashed border-[var(--line)] bg-[var(--surface-input)] p-10 text-center">
          <File className="mx-auto mb-3 text-[var(--text-subtle)]/50" size={32} strokeWidth={1} />
          <p className="text-sm text-[var(--text-subtle)]">
            {search ? "No files match this filter." : "This directory is empty."}
          </p>
          <p className="mt-2 text-xs text-[var(--text-subtle)]">Drag and drop files here to upload, or use the Upload button.</p>
        </div>
      ) : entries.length > 0 ? (
        <>
          {/* Column headers - desktop only */}
          <div className="hidden sm:grid sm:grid-cols-[28px_1fr_100px_170px_auto] gap-3 px-3 py-2 text-[11px] font-bold uppercase tracking-widest text-[var(--text-subtle)]">
            <span />
            <button className="text-left flex items-center gap-1 hover:text-[var(--text)] transition-colors" onClick={() => toggleSort("name")} type="button">
              Name {sortBy === "name" && (sortDir === "asc" ? "↑" : "↓")}
            </button>
            <button className="text-left flex items-center gap-1 hover:text-[var(--text)] transition-colors" onClick={() => toggleSort("size")} type="button">
              Size {sortBy === "size" && (sortDir === "asc" ? "↑" : "↓")}
            </button>
            <button className="text-left flex items-center gap-1 hover:text-[var(--text)] transition-colors" onClick={() => toggleSort("date")} type="button">
              Modified {sortBy === "date" && (sortDir === "asc" ? "↑" : "↓")}
            </button>
            <div />
          </div>

          {/* File entries – tokenized, selectable, archive/decompress wired */}
          <div className="overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface-input)]">
            {entries.map((entry) => {
              const checked = selected.includes(entry.path);
              const isArchive = /\.(zip|tar|tar\.gz|tgz|gz)$/i.test(entry.name);
              return (
              <div
                className="grid gap-2 border-b border-[var(--line)] px-3 py-2.5 last:border-0 hover:bg-white/[0.02] transition-colors sm:grid-cols-[28px_1fr_100px_170px_auto] sm:items-center"
                key={entry.path}
              >
                <input aria-label={`Select ${entry.name}`} checked={checked} onChange={() => setSelected((items) => checked ? items.filter((it) => it !== entry.path) : [...items, entry.path])} type="checkbox" />
                <button
                  className="flex min-w-0 items-center gap-3 text-left font-medium text-[var(--text)] hover:text-white transition-colors"
                  onClick={() => entry.isDir ? setDirectory(entry.path) : void openFile(entry.path)}
                  type="button"
                >
                  {entry.isDir
                    ? <Folder className="shrink-0 text-amber-400/80" size={18} />
                    : <File className="shrink-0 text-[var(--text-subtle)]" size={18} />
                  }
                  <span className="truncate">{entry.name}</span>
                </button>
                <span className="text-xs text-[var(--text-subtle)] truncate">{entry.isDir ? "Folder" : formatBytes(entry.size)}</span>
                <time className="text-xs text-[var(--text-subtle)]/70 truncate hidden sm:block">{entry.modTime || "—"}</time>
                <div className="flex justify-end gap-0.5">
                  <button
                    aria-label="Download"
                    className={iconBtn}
                    disabled={busy || entry.isDir}
                    onClick={() => handleDownload(entry)}
                    title="Download"
                    type="button"
                  >
                    <Download size={14} />
                  </button>
                  <button
                    aria-label="Archive"
                    className={iconBtn}
                    disabled={busy}
                    onClick={() => handleArchive(entry)}
                    title={entry.isDir ? "Archive (host directories: not yet exposed – see note)" : "Archive – download as file (host has no tar endpoint, wired to download)"}
                    type="button"
                  >
                    <Archive size={14} />
                  </button>
                  {isArchive ? (
                    <button
                      aria-label="Decompress"
                      className={iconBtn}
                      disabled={busy}
                      onClick={() => handleDecompress(entry)}
                      title="Decompress – host decompress not yet exposed (server files support tar.gz)"
                      type="button"
                    >
                      <PackageOpen size={14} />
                    </button>
                  ) : null}
                  <button
                    aria-label="Rename"
                    className={iconBtn}
                    disabled={busy}
                    onClick={() => handleRename(entry)}
                    title="Rename"
                    type="button"
                  >
                    <Save size={14} />
                  </button>
                  <button
                    aria-label="Copy"
                    className={iconBtn}
                    disabled={busy || entry.isDir}
                    onClick={() => handleCopy(entry)}
                    title="Copy"
                    type="button"
                  >
                    <Copy size={14} />
                  </button>
                  {!entry.isDir ? (
                    <button
                      aria-label="Permissions"
                      className={iconBtn}
                      disabled={busy}
                      onClick={() => handleChmod(entry)}
                      title="Permissions"
                      type="button"
                    >
                      <Lock size={14} />
                    </button>
                  ) : null}
                  <button
                    aria-label="Delete"
                    className={cn(iconBtn, "text-red-400 hover:bg-red-500/10 hover:text-red-300")}
                    disabled={busy}
                    onClick={() => handleDelete(entry)}
                    title="Delete"
                    type="button"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
              );
            })}
          </div>
        </>
      ) : null}

      {/* Bulk action bar – wired to delete-batch (looped) + bulk chmod */}
      {selected.length ? (
        <div className="sticky bottom-4 z-20 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-[var(--line)] bg-[var(--surface-raised)]/95 p-3 shadow-2xl backdrop-blur">
          <span className="flex items-center gap-2 text-sm font-semibold text-[var(--text)]"><CheckSquare size={17} />{selected.length} selected</span>
          <div className="flex flex-wrap gap-2">
            <button className={btn} disabled={busy} onClick={handleBulkChmod} type="button"><Lock size={14} />Permissions</button>
            <button className={cn(btn, "text-red-300 border-red-500/30 hover:bg-red-500/10")} disabled={busy} onClick={() => void handleBulkDelete()} type="button"><Trash2 size={14} />Delete</button>
            <button className={btn} onClick={() => setSelected([])} type="button">Clear</button>
          </div>
        </div>
      ) : null}

      <p className="text-xs text-[var(--text-subtle)]">Drag and drop files to upload. Archive for host files is wired to download (beacon host archive endpoint not yet exposed – parity kept with server files archive/decompress/pull/bulk). Use server file manager for full tar.gz archive lifecycle.</p>

      {createKind && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => setCreateKind(null)}>
          <div role="dialog" aria-modal="true" aria-label={`Create ${createKind}`} className="w-full max-w-md rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold text-white">Create {createKind}</h3>
            <input className="ui-input mt-3 w-full" autoFocus value={createName} onChange={(e) => setCreateName(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") submitCreate(); if (e.key === "Escape") setCreateKind(null); }} placeholder={`${createKind} name`} />
            <div className="mt-4 flex justify-end gap-2">
              <button className="ui-button ui-button-ghost" onClick={() => setCreateKind(null)} type="button">Cancel</button>
              <button className="ui-button ui-button-primary" disabled={!createName.trim()} onClick={submitCreate} type="button">Create</button>
            </div>
          </div>
        </div>
      )}
      {renameTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => setRenameTarget(null)}>
          <div role="dialog" aria-modal="true" aria-label={`Rename ${renameTarget.name}`} className="w-full max-w-md rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold text-white">Rename &quot;{renameTarget.name}&quot;</h3>
            <input className="ui-input mt-3 w-full" autoFocus value={renameName} onChange={(e) => setRenameName(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") submitRename(); if (e.key === "Escape") setRenameTarget(null); }} placeholder="New name" />
            <div className="mt-4 flex justify-end gap-2">
              <button className="ui-button ui-button-ghost" onClick={() => setRenameTarget(null)} type="button">Cancel</button>
              <button className="ui-button ui-button-primary" disabled={!renameName.trim() || renameName.trim() === renameTarget.name} onClick={submitRename} type="button">Rename</button>
            </div>
          </div>
        </div>
      )}
      {copyTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => setCopyTarget(null)}>
          <div role="dialog" aria-modal="true" aria-label={`Copy ${copyTarget.name}`} className="w-full max-w-md rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold text-white">Copy &quot;{copyTarget.name}&quot;</h3>
            <input className="ui-input mt-3 w-full" autoFocus value={copyName} onChange={(e) => setCopyName(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") submitCopy(); if (e.key === "Escape") setCopyTarget(null); }} placeholder="Copy name" />
            <div className="mt-4 flex justify-end gap-2">
              <button className="ui-button ui-button-ghost" onClick={() => setCopyTarget(null)} type="button">Cancel</button>
              <button className="ui-button ui-button-primary" disabled={!copyName.trim()} onClick={submitCopy} type="button">Copy</button>
            </div>
          </div>
        </div>
      )}
      {chmodTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => setChmodTarget(null)}>
          <div role="dialog" aria-modal="true" aria-label={`Permissions for ${chmodTarget.name}`} className="w-full max-w-md rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold text-white">Permissions — {chmodTarget.name}</h3>
            <input className="ui-input mt-3 w-full font-mono" autoFocus value={chmodMode} onChange={(e) => setChmodMode(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") submitChmod(); if (e.key === "Escape") setChmodTarget(null); }} placeholder="0644" inputMode="numeric" pattern="[0-7]{3,4}" />
            <p className="mt-2 text-xs text-[var(--text-subtle)]">Three or four octal digits (e.g. 0644, 0755).</p>
            <div className="mt-4 flex justify-end gap-2">
              <button className="ui-button ui-button-ghost" onClick={() => setChmodTarget(null)} type="button">Cancel</button>
              <button className="ui-button ui-button-primary" disabled={!/^[0-7]{3,4}$/.test(chmodMode.trim())} onClick={submitChmod} type="button">Apply</button>
            </div>
          </div>
        </div>
      )}
      {bulkChmodOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={() => setBulkChmodOpen(false)}>
          <div role="dialog" aria-modal="true" aria-label="Bulk permissions" className="w-full max-w-md rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-sm font-semibold text-white">Permissions — {selected.length} selected</h3>
            <input className="ui-input mt-3 w-full font-mono" autoFocus value={bulkChmodMode} onChange={(e) => setBulkChmodMode(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") submitBulkChmod(); if (e.key === "Escape") setBulkChmodOpen(false); }} placeholder="0644" inputMode="numeric" pattern="[0-7]{3,4}" />
            <p className="mt-2 text-xs text-[var(--text-subtle)]">Applied to every selected item (batch chmod loop – host has no batch endpoint yet).</p>
            <div className="mt-4 flex justify-end gap-2">
              <button className="ui-button ui-button-ghost" onClick={() => setBulkChmodOpen(false)} type="button">Cancel</button>
              <button className="ui-button ui-button-primary" disabled={!/^[0-7]{3,4}$/.test(bulkChmodMode.trim())} onClick={submitBulkChmod} type="button">Apply to {selected.length}</button>
            </div>
          </div>
        </div>
      )}
      {pullOpen && (
        <Dialog closeAction={() => { if (!busy) { setPullOpen(false); setPullUrl(""); } }} description="The file is downloaded to the current directory over HTTP(S) via client-side fetch + host upload (no dedicated beacon host pull endpoint yet)." open title="Pull from URL — host">
          <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); void handlePull(); }}>
            <label className="block space-y-1.5">
              <span className="text-sm font-medium text-[var(--text)]">Public file URL</span>
              <input autoFocus className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2.5 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus:border-[var(--brand)]/70 focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--brand-subtle)]" onChange={(ev) => setPullUrl(ev.target.value)} placeholder="https://example.com/file.jar" type="url" value={pullUrl} />
            </label>
            {error ? <p className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-200" role="alert">{error}</p> : null}
            <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <button className="inline-flex h-10 items-center justify-center rounded-lg border border-[var(--line)] px-4 text-sm font-semibold text-[var(--text-subtle)] transition hover:bg-white/[0.06] disabled:cursor-not-allowed disabled:opacity-40" disabled={busy} onClick={() => { setPullOpen(false); setPullUrl(""); }} type="button">Cancel</button>
              <button className="inline-flex h-10 items-center justify-center rounded-lg bg-[var(--brand)] px-4 text-sm font-bold text-white transition hover:bg-[var(--brand-hover)] disabled:cursor-not-allowed disabled:opacity-40" disabled={busy || !pullUrl.trim()} type="submit">Download</button>
            </div>
          </form>
        </Dialog>
      )}
    </div>
  );
}
