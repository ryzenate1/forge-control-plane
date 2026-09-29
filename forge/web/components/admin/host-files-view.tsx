"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronRight, Copy, Download, File as FileIcon, Folder, FolderUp, Lock, PenLine, RefreshCw, Save,
  Search, Server, Trash2, Upload,
} from "lucide-react";
import Link from "next/link";
import { cn, errorMessage, formatDate, formatBytes } from "@/lib/utils";
import {
  listFiles, readFile, writeFile, createDir, deleteFile, renameFile, copyFile, chmodFile, downloadFile, uploadFile, pullRemoteFile,
  type FileEntry,
} from "@/lib/api/host-files";
import { sourceState, useNodesQuery } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "./telemetry-ui";
import {
  AdminSelect,
  AdminErrorState,
  AdminLoadingRows,
  AdminLoadingState,
  Btn,
  EmptyState,
  Input,
  Modal,
} from "./admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";

const NOT_REPORTED = "Not reported";

/** Grid template declared once so the header row and the body rows cannot drift. */
const ROW_GRID = "grid gap-2 sm:grid-cols-[28px_minmax(0,1fr)_88px_150px_130px_auto] sm:items-center";

/**
 * Paths where a single write can stop the machine booting or lock SSH out. A
 * host file manager runs with whatever privilege Beacon has, so these writes
 * get an explicit confirmation instead of one click.
 */
const SYSTEM_PATH_PREFIXES = ["/etc", "/boot", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/lib32", "/root", "/sys", "/proc", "/dev", "/var"];

function isSystemPath(path: string): boolean {
  return SYSTEM_PATH_PREFIXES.some((prefix) => path === prefix || path.startsWith(`${prefix}/`));
}

function finiteSize(size: number | null | undefined): number | null {
  return typeof size === "number" && Number.isFinite(size) ? size : null;
}

function sizeLabel(entry: FileEntry): string {
  if (entry.isDir) return "Folder";
  const bytes = finiteSize(entry.size);
  return bytes === null ? NOT_REPORTED : formatBytes(bytes);
}

function Breadcrumbs({ directory, onOpen }: { directory: string; onOpen: (path: string) => void }) {
  const parts = directory.split("/").filter(Boolean);
  return (
    <nav aria-label="File path" className="flex min-w-0 items-center gap-1 overflow-x-auto text-sm">
      <button
        className="shrink-0 font-semibold text-text hover:text-[var(--brand)] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
        onClick={() => onOpen("/")}
        type="button"
      >
        root
      </button>
      {parts.map((part, index) => {
        const path = "/" + parts.slice(0, index + 1).join("/");
        return (
          <span className="flex shrink-0 items-center gap-1" key={path}>
            <ChevronRight aria-hidden="true" className="shrink-0 text-text-muted" size={14} />
            <button
              className="max-w-[120px] truncate text-text-subtle hover:text-text transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] sm:max-w-[200px]"
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
  const [confirmAction, renderConfirm] = useConfirm();
  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const nodeOptions = useMemo(
    () => nodes.map((node) => ({ value: node.id, label: node.status === "active" ? node.name : `${node.name} · ${node.status}` })),
    [nodes],
  );

  // The node has to be named before anything is read or written: `/host/files/*`
  // rejects a request without `nodeId`, and browsing an arbitrary machine's
  // filesystem is not a safe default. A `?nodeId=` in the URL counts as an
  // explicit target (it is how the Terminal hand-off and shared links arrive).
  const [nodeId, setNodeId] = useState("");
  const [directory, setDirectory] = useState("/");
  const [editing, setEditing] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [fileLoaded, setFileLoaded] = useState(false);
  const [status, setStatus] = useState("No node selected");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState<"name" | "size" | "date">("name");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");
  const [selected, setSelected] = useState<string[]>([]);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const dragDepth = useRef(0);
  const [pullOpen, setPullOpen] = useState(false);
  const [pullUrl, setPullUrl] = useState("");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const fromUrl = new URLSearchParams(window.location.search).get("nodeId");
    if (fromUrl) {
      setNodeId(fromUrl);
      setStatus("Target loaded from link");
    }
  }, []);

  // Keep the target in the URL so a filtered directory view is shareable and a
  // browser back/forward does not drop back to "no node".
  useEffect(() => {
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (nodeId) url.searchParams.set("nodeId", nodeId);
    else url.searchParams.delete("nodeId");
    window.history.replaceState(null, "", url.toString());
  }, [nodeId]);

  const nodeName = nodes.find((node) => node.id === nodeId)?.name ?? (nodeId ? `${nodeId.slice(0, 12)}…` : "");

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ["host-files", nodeId, directory] });
  }, [queryClient, nodeId, directory]);

  const run = useCallback(async (label: string, action: () => Promise<void>) => {
    if (!nodeId) {
      setError("Select the node this action should run against.");
      return;
    }
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
  }, [nodeId]);

  /**
   * Batch edits report what actually happened. A mid-batch failure used to leave
   * the whole selection marked as if nothing had changed; now the paths that were
   * not applied stay selected and the message counts both halves.
   */
  const runBatch = useCallback(async (label: string, paths: string[], op: (path: string) => Promise<void>) => {
    if (!nodeId) {
      setError("Select the node this action should run against.");
      return;
    }
    setBusy(true);
    setError("");
    setStatus(`${label} (0/${paths.length})`);
    const failed: string[] = [];
    let firstFailure = "";
    for (let index = 0; index < paths.length; index += 1) {
      try {
        await op(paths[index]);
      } catch (batchError) {
        failed.push(paths[index]);
        if (!firstFailure) firstFailure = errorMessage(batchError, "no reason returned");
      }
      setStatus(`${label} (${index + 1}/${paths.length})`);
    }
    setSelected(failed);
    await refresh();
    setBusy(false);
    if (failed.length === 0) {
      setStatus(`${label} applied to all ${paths.length}`);
    } else {
      setError(`${label}: ${failed.length} of ${paths.length} failed (first failure: ${firstFailure}). The failed paths are still selected.`);
      setStatus(`${paths.length - failed.length} of ${paths.length} applied`);
    }
  }, [nodeId, refresh]);

  const files = useQuery({
    queryKey: ["host-files", nodeId, directory],
    queryFn: () => listFiles(directory, nodeId),
    enabled: Boolean(nodeId),
    retry: 1,
    staleTime: 10_000,
  });

  const entries = useMemo(() => {
    return (files.data ?? [])
      .filter((entry) => entry.name.toLowerCase().includes(search.trim().toLowerCase()))
      .sort((a, b) => {
        if (a.isDir !== b.isDir) return Number(b.isDir) - Number(a.isDir);
        if (sortBy === "size") {
          // An unmeasured size sorts last instead of being treated as 0 bytes.
          const left = finiteSize(a.size);
          const right = finiteSize(b.size);
          if (left === null && right === null) return a.name.localeCompare(b.name);
          if (left === null) return 1;
          if (right === null) return -1;
          return left - right;
        }
        const cmp = sortBy === "date"
          ? (a.modTime ?? "").localeCompare(b.modTime ?? "")
          : a.name.localeCompare(b.name);
        return sortDir === "desc" ? -cmp : cmp;
      });
  }, [files.data, search, sortBy, sortDir]);

  const handleDragEnter = useCallback((event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    if (!nodeId) return;
    dragDepth.current += 1;
    if (dragDepth.current === 1) setDragging(true);
  }, [nodeId]);

  const handleDragLeave = useCallback((event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    dragDepth.current = Math.max(0, dragDepth.current - 1);
    if (dragDepth.current === 0) setDragging(false);
  }, []);

  const handleDragOver = useCallback((event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
  }, []);

  const uploadMany = useCallback(async (dropped: File[], label: string) => {
    if (!dropped.length) return;
    await run(label, async () => {
      for (let index = 0; index < dropped.length; index += 1) {
        setUploadProgress(Math.round((index / dropped.length) * 100));
        await uploadFile(directory, dropped[index], nodeId);
      }
      setUploadProgress(null);
      await refresh();
    });
  }, [directory, nodeId, refresh, run]);

  const handleDrop = useCallback(async (event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    dragDepth.current = 0;
    setDragging(false);
    if (busy || !nodeId) return;
    await uploadMany(Array.from(event.dataTransfer.files), "Uploading dropped files");
  }, [busy, nodeId, uploadMany]);

  const openFile = useCallback(async (path: string) => {
    if (!nodeId) {
      setError("Select a node before opening a file.");
      return;
    }
    setEditing(path);
    setContent("");
    setFileLoaded(false);
    setError("");
    setStatus("Loading");
    try {
      const value = await readFile(path, nodeId);
      setContent(value);
      setFileLoaded(true);
      setStatus("Loaded");
    } catch (loadError) {
      setError(errorMessage(loadError, "File could not be loaded."));
      setStatus("Load failed");
    }
  }, [nodeId]);

  const save = useCallback(async () => {
    if (!editing || !fileLoaded) return;
    const path = editing;
    if (isSystemPath(path)) {
      const ok = await confirmAction({
        confirmLabel: "Write to host",
        danger: true,
        description: `${path} on ${nodeName} is under a system directory. Writing it can change how the machine boots, authenticates or runs.`,
        title: "Write this system path?",
      });
      if (!ok) return;
    }
    await run("Saving", async () => {
      await writeFile(path, content, nodeId);
      setStatus(`Saved ${path}`);
      setEditing(null);
      setFileLoaded(false);
      await refresh();
    });
  }, [confirmAction, content, editing, fileLoaded, nodeName, nodeId, refresh, run]);

  const leaveEditor = useCallback(async (nextDirectory?: string) => {
    if (status === "Edited") {
      const discard = await confirmAction({
        confirmLabel: "Discard changes",
        danger: true,
        description: `The edits to ${editing ?? "this file"} on ${nodeName} have not been written to the host. Discarding loses them.`,
        title: "Discard unsaved changes?",
      });
      if (!discard) return;
    }
    setEditing(null);
    setFileLoaded(false);
    setContent("");
    setStatus("Ready");
    if (nextDirectory !== undefined) setDirectory(nextDirectory);
  }, [confirmAction, editing, nodeName, status]);

  const [createKind, setCreateKind] = useState<"file" | "folder" | null>(null);
  const [createName, setCreateName] = useState("");
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null);
  const [renameName, setRenameName] = useState("");
  const [copyTarget, setCopyTarget] = useState<FileEntry | null>(null);
  const [copyName, setCopyName] = useState("");
  const [chmodTarget, setChmodTarget] = useState<FileEntry | null>(null);
  const [chmodMode, setChmodMode] = useState("");
  const [bulkChmodOpen, setBulkChmodOpen] = useState(false);
  const [bulkChmodMode, setBulkChmodMode] = useState("0644");

  const promptName = (kind: "file" | "folder") => {
    if (!nodeId) {
      setError("Select a node first.");
      return;
    }
    setCreateKind(kind);
    setCreateName("");
  };

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
      if (kind === "folder") await createDir(target, nodeId);
      else await writeFile(target, "", nodeId);
      await refresh();
    });
  };

  const handleUpload = async (event: FormEvent<HTMLInputElement>) => {
    const input = event.currentTarget;
    const picked = Array.from(input.files ?? []);
    input.value = "";
    if (!picked.length) return;
    await uploadMany(picked, "Uploading");
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
      await renameFile(target.path, parentPath + name, nodeId);
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
      await copyFile(target.path, parentPath + name, nodeId);
      await refresh();
    });
  };

  const handleDelete = async (entry: FileEntry) => {
    const confirmed = await confirmAction({
      title: `Permanently delete ${entry.isDir ? "directory" : "file"} "${entry.name}"?`,
      description: `${entry.path} on ${nodeName}${entry.isDir ? " and everything inside it" : ""} will be removed from the host. This cannot be undone.`,
      danger: true,
      confirmLabel: "Delete",
    });
    if (!confirmed) return;
    void run("Deleting", async () => {
      await deleteFile(entry.path, nodeId);
      setSelected((prev) => prev.filter((p) => p !== entry.path));
      await refresh();
    });
  };

  const handleBulkDelete = async () => {
    if (!selected.length) return;
    const confirmed = await confirmAction({
      title: `Delete ${selected.length} selected ${selected.length === 1 ? "item" : "items"} on ${nodeName}?`,
      description: `${selected.slice(0, 3).join(", ")}${selected.length > 3 ? `, +${selected.length - 3} more` : ""} will be permanently removed from the host, including the contents of any selected directory. This cannot be undone.`,
      danger: true,
      confirmLabel: `Delete ${selected.length}`,
    });
    if (!confirmed) return;
    void runBatch("Delete", [...selected], (path) => deleteFile(path, nodeId));
  };

  const handleBulkChmod = () => {
    if (!selected.length) return;
    setBulkChmodMode("0644");
    setBulkChmodOpen(true);
  };

  const submitBulkChmod = async () => {
    const mode = bulkChmodMode.trim();
    if (!/^[0-7]{3,4}$/.test(mode)) {
      setError("Permissions must be three or four octal digits.");
      return;
    }
    const paths = [...selected];
    const confirmed = await confirmAction({
      confirmLabel: `Apply to ${paths.length}`,
      danger: true,
      description: `${mode} will be applied to ${paths.length} path${paths.length === 1 ? "" : "s"} on ${nodeName}. Changing permissions can expose files or lock the host out of them.`,
      title: "Change permissions on this host?",
    });
    if (!confirmed) return;
    setBulkChmodOpen(false);
    void runBatch("Permissions", paths, (path) => chmodFile(path, mode, nodeId));
  };

  const handleChmod = (entry: FileEntry) => {
    setChmodTarget(entry);
    setChmodMode(entry.mode ?? "");
  };

  const submitChmod = async () => {
    if (!chmodTarget) return;
    const mode = chmodMode.trim();
    if (!mode || !/^[0-7]{3,4}$/.test(mode)) {
      if (mode) setError("Permissions must be three or four octal digits.");
      return;
    }
    const target = chmodTarget;
    const confirmed = await confirmAction({
      confirmLabel: "Apply permissions",
      danger: true,
      description: `${target.path} on ${nodeName} changes from ${target.mode || NOT_REPORTED} to ${mode}.`,
      title: `Change permissions on ${target.name}?`,
    });
    if (!confirmed) return;
    setChmodTarget(null);
    void run("Changing permissions", async () => {
      await chmodFile(target.path, mode, nodeId);
      await refresh();
    });
  };

  const handleDownload = (entry: FileEntry) => {
    void run("Downloading", async () => {
      const blob = await downloadFile(entry.path, nodeId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(url);
    });
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
    await run(`Pulling ${name}`, async () => {
      // No beacon host-pull endpoint: the browser fetches the URL, then the
      // result is uploaded to the current directory on the named node.
      const blob = await pullRemoteFile(raw);
      const file = new File([blob], name, { type: blob.type || "application/octet-stream" });
      await uploadFile(directory, file, nodeId);
      await refresh();
      setStatus(`Pulled to ${dest}`);
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
    setContent("");
    setError("");
    setSelected([]);
    setStatus(next ? "Ready" : "No node selected");
  }, []);

  const allVisibleSelected = entries.length > 0 && entries.every((entry) => selected.includes(entry.path));
  const hasNodes = nodes.length > 0;

  if (editing) {
    return (
      <div className="space-y-4">
        {renderConfirm()}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <button className="text-xs font-semibold text-text-muted hover:text-text transition-colors" onClick={() => void leaveEditor()} type="button">
            <ChevronRight aria-hidden="true" className="inline rotate-180" size={14} /> Back to {directory}
          </button>
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-text-subtle" role="status">{status}</span>
            <Btn onClick={() => void leaveEditor()} size="sm" tone="ghost">Close</Btn>
            <Btn disabled={!fileLoaded || busy} loading={busy} onClick={() => void save()} size="sm">
              <Save aria-hidden="true" size={14} /> Save
            </Btn>
          </div>
        </div>
        <p className="font-mono text-meta text-text-subtle">
          Editing {editing} on {nodeName || NOT_REPORTED}
          {isSystemPath(editing) ? " · system path" : ""}
        </p>
        {error ? <AdminErrorState message={error} /> : null}
        <div className="h-[65vh] min-h-96 overflow-hidden rounded-xl border border-line bg-[var(--surface-input)] p-4 font-mono text-sm text-text">
          <textarea
            aria-label={`Contents of ${editing} on ${nodeName}`}
            className="h-full w-full resize-none bg-transparent outline-none leading-relaxed"
            onChange={(event) => { if (fileLoaded) { setContent(event.target.value); setStatus("Edited"); } }}
            readOnly={!fileLoaded}
            spellCheck={false}
            value={content}
          />
        </div>
      </div>
    );
  }

  return (
    <div
      className="relative space-y-4"
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={(event) => void handleDrop(event)}
    >
      {renderConfirm()}
      {dragging ? (
        <div aria-live="assertive" className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-[color-mix(in_srgb,var(--canvas)_70%,transparent)] backdrop-blur-sm" role="status">
          <div className="flex flex-col items-center gap-4 rounded-2xl border-2 border-dashed border-[color-mix(in_srgb,var(--brand)_60%,transparent)] bg-[color-mix(in_srgb,var(--surface-raised)_90%,transparent)] px-16 py-12 text-center shadow-2xl">
            <Upload aria-hidden="true" className="h-12 w-12 animate-bounce text-[var(--brand)]" />
            <p className="text-lg font-bold text-text">Drop files to upload</p>
            <p className="text-sm text-text-subtle">Files land in {directory} on {nodeName}.</p>
          </div>
        </div>
      ) : null}

      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-56">
          <AdminSelect
            disabled={nodesQuery.isPending || !hasNodes}
            label="Target node"
            onChange={handleNodeChange}
            options={nodeOptions}
            placeholder={nodesQuery.isPending ? "Loading nodes…" : hasNodes ? "Select a node…" : "No nodes registered"}
            value={nodeId}
          />
        </div>
        {nodeId ? <Breadcrumbs directory={directory} onOpen={setDirectory} /> : null}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <label className="relative">
          <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-2.5 text-text-subtle" size={14} />
          <span className="sr-only">Filter files by name</span>
          <input
            className="h-9 w-40 rounded-lg border border-line bg-[var(--surface-input)] pl-9 pr-3 text-xs text-text outline-none placeholder:text-text-subtle focus:border-[var(--brand)] focus:ring-2 focus:ring-[var(--brand-subtle)] transition-all"
            disabled={!nodeId}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter files…"
            type="search"
            value={search}
          />
        </label>
        <Btn ariaLabel="New folder" disabled={!nodeId || busy} onClick={() => promptName("folder")} size="sm" tone="ghost" title="Create a folder in the current directory">
          <Folder aria-hidden="true" size={14} /> <span className="hidden sm:inline">New folder</span>
        </Btn>
        <Btn ariaLabel="New file" disabled={!nodeId || busy} onClick={() => promptName("file")} size="sm" tone="ghost" title="Create an empty file in the current directory">
          <FileIcon aria-hidden="true" size={14} /> <span className="hidden sm:inline">New file</span>
        </Btn>
        <Btn ariaLabel="Pull a file from a URL into the current directory" disabled={!nodeId || busy} onClick={() => setPullOpen(true)} size="sm" tone="ghost" title="Fetch a URL in the browser and upload it here">
          <Download aria-hidden="true" size={14} /> <span className="hidden sm:inline">Pull URL</span>
        </Btn>
        <Btn ariaLabel="Go up one directory" disabled={!nodeId || busy || directory === "/"} onClick={() => setDirectory(directory === "/" ? "/" : directory.substring(0, directory.lastIndexOf("/")) || "/")} size="sm" tone="ghost" title="Up one directory">
          <FolderUp aria-hidden="true" size={14} /> <span className="hidden sm:inline">Go up</span>
        </Btn>
        <Btn ariaLabel="Reload this directory" disabled={!nodeId || busy} onClick={() => void refresh()} size="sm" tone="ghost" title="Reload this directory">
          <RefreshCw aria-hidden="true" size={14} />
        </Btn>
        <label className="inline-flex">
          <span className="sr-only">Upload files from this computer</span>
          <Btn ariaLabel="Upload files" disabled={!nodeId || busy} size="sm" tone="ghost" title="Upload files to the current directory">
            <Upload aria-hidden="true" size={14} /> <span className="hidden sm:inline">Upload</span>
          </Btn>
          <input className="sr-only" disabled={!nodeId || busy} multiple onChange={handleUpload} type="file" />
        </label>
        <Link
          aria-label={nodeId ? `Open the host terminal on ${nodeName}` : "Open the host terminal"}
          className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-line px-3 text-xs font-semibold text-text-subtle transition-colors hover:border-line-strong hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
          href={nodeId ? `/admin/terminal?nodeId=${encodeURIComponent(nodeId)}` : "/admin/terminal"}
          title={nodeId ? `Open the host terminal on ${nodeName} (?nodeId= forwarded)` : "Select a node first, or open the terminal and pick one there"}
        >
          Terminal
        </Link>
      </div>

      {uploadProgress !== null ? (
        <div className="rounded-lg border border-line bg-overlay-subtle p-3" role="status">
          <div className="flex justify-between text-xs text-text-subtle">
            <span>Uploading files to {nodeName || NOT_REPORTED}</span>
            <span>{uploadProgress}%</span>
          </div>
          <div className="mt-2 h-2 overflow-hidden rounded bg-overlay-strong">
            <div className="h-full bg-[var(--brand)] transition-all" style={{ width: `${uploadProgress}%` }} />
          </div>
        </div>
      ) : null}

      {error ? <AdminErrorState message={error} /> : null}
      {files.isError ? (
        <AdminErrorState message={errorMessage(files.error, "The file list could not be loaded.")} retry={() => void files.refetch()} />
      ) : null}

      {nodesQuery.isPending ? (
        <AdminLoadingState label="Loading nodes…" />
      ) : nodesQuery.isError ? (
        <AdminErrorState message={errorMessage(nodesQuery.error, "The node list could not be loaded.")} retry={() => void nodesQuery.refetch()} />
      ) : !hasNodes ? (
        <EmptyState icon={Server} message="No Beacon node is registered, so there is no host filesystem to browse. Add one under Infrastructure → Nodes." title="No nodes available" />
      ) : !nodeId ? (
        <EmptyState
          icon={Server}
          message="Pick the node above. Listings, edits, uploads and permission changes all target that one machine, so nothing is read or written until you name it."
          title="No node selected"
        />
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-subtle">
            <label className="flex cursor-pointer items-center gap-2">
              <input
                checked={allVisibleSelected}
                onChange={(event) => setSelected(event.target.checked ? entries.map((entry) => entry.path) : [])}
                type="checkbox"
              />
              Select all visible
            </label>
            <span className="flex items-center gap-3">
              <span role="status">{files.isFetching ? "Loading…" : status}{selected.length ? ` · ${selected.length} selected` : ""}</span>
              <FreshnessBadge state={sourceState(files)} />
            </span>
          </div>

          {files.isPending ? (
            <AdminLoadingRows cols={5} rows={6} label="Loading file list…" />
          ) : entries.length === 0 && !files.isError ? (
            <EmptyState
              icon={FileIcon}
              message={search ? "No files in this directory match the filter." : `${directory} is empty on ${nodeName}. Drag files in or use Upload.`}
              title={search ? "No matching files" : "Empty directory"}
            />
          ) : entries.length > 0 ? (
            <div className="overflow-x-auto">
              <div className={cn(ROW_GRID, "px-3 py-2 text-meta font-bold uppercase tracking-widest text-text-subtle")}>
                <span />
                <button className="flex items-center gap-1 text-left hover:text-text transition-colors" onClick={() => toggleSort("name")} type="button">
                  Name {sortBy === "name" ? (sortDir === "asc" ? "↑" : "↓") : ""}
                </button>
                <button className="flex items-center gap-1 text-left hover:text-text transition-colors" onClick={() => toggleSort("size")} type="button">
                  Size {sortBy === "size" ? (sortDir === "asc" ? "↑" : "↓") : ""}
                </button>
                <button className="flex items-center gap-1 text-left hover:text-text transition-colors" onClick={() => toggleSort("date")} type="button">
                  Modified {sortBy === "date" ? (sortDir === "asc" ? "↑" : "↓") : ""}
                </button>
                <span>Permissions</span>
                <span className="text-right">Actions</span>
              </div>

              <div className="rounded-xl border border-line bg-[var(--surface-input)]">
                {entries.map((entry) => {
                  const checked = selected.includes(entry.path);
                  return (
                    <div className={cn(ROW_GRID, "border-b border-line px-3 py-2.5 transition-colors last:border-0 hover:bg-overlay-subtle")} key={entry.path}>
                      <input
                        aria-label={`Select ${entry.name}`}
                        checked={checked}
                        onChange={() => setSelected((items) => checked ? items.filter((item) => item !== entry.path) : [...items, entry.path])}
                        type="checkbox"
                      />
                      <button
                        className="flex min-w-0 items-center gap-3 text-left font-medium text-text hover:text-[var(--brand)] transition-colors"
                        onClick={() => (entry.isDir ? setDirectory(entry.path) : void openFile(entry.path))}
                        type="button"
                      >
                        {entry.isDir
                          ? <Folder aria-hidden="true" className="shrink-0 text-warn" size={18} />
                          : <FileIcon aria-hidden="true" className="shrink-0 text-text-subtle" size={18} />}
                        <span className="truncate">{entry.name}</span>
                      </button>
                      <span className="truncate font-mono text-xs text-text-subtle">{sizeLabel(entry)}</span>
                      <span className="font-mono text-xs text-text-muted">{entry.modTime ? formatDate(entry.modTime, NOT_REPORTED) : NOT_REPORTED}</span>
                      <span className="font-mono text-xs text-text-muted">{entry.mode || NOT_REPORTED}</span>
                      <div className="flex justify-end gap-0.5">
                        <Btn ariaLabel={`Download ${entry.name}`} disabled={busy || entry.isDir} onClick={() => handleDownload(entry)} size="sm" tone="ghost" title={`Download ${entry.path}`}>
                          <Download aria-hidden="true" size={12} />
                        </Btn>
                        <Btn ariaLabel={`Rename ${entry.name}`} disabled={busy} onClick={() => handleRename(entry)} size="sm" tone="ghost" title={`Rename ${entry.path}`}>
                          <PenLine aria-hidden="true" size={12} />
                        </Btn>
                        <Btn ariaLabel={`Copy ${entry.name}`} disabled={busy || entry.isDir} onClick={() => handleCopy(entry)} size="sm" tone="ghost" title={`Copy ${entry.path}`}>
                          <Copy aria-hidden="true" size={12} />
                        </Btn>
                        <Btn ariaLabel={`Permissions for ${entry.name}`} disabled={busy} onClick={() => handleChmod(entry)} size="sm" tone="ghost" title={`Permissions for ${entry.path}`}>
                          <Lock aria-hidden="true" size={12} />
                        </Btn>
                        <Btn ariaLabel={`Delete ${entry.name}`} disabled={busy} onClick={() => void handleDelete(entry)} size="sm" tone="danger" title={`Delete ${entry.path}`}>
                          <Trash2 aria-hidden="true" size={12} />
                        </Btn>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          ) : null}

          {selected.length ? (
            <div className="sticky bottom-4 z-20 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-[color-mix(in_srgb,var(--surface-raised)_95%,transparent)] p-3 shadow-2xl backdrop-blur">
              <span className="flex items-center gap-2 text-sm font-semibold text-text">
                {selected.length} selected on {nodeName}
              </span>
              <div className="flex flex-wrap gap-2">
                <Btn disabled={busy} onClick={handleBulkChmod} size="sm" tone="ghost">
                  <Lock aria-hidden="true" size={14} /> Permissions
                </Btn>
                <Btn disabled={busy} onClick={() => void handleBulkDelete()} size="sm" tone="danger">
                  <Trash2 aria-hidden="true" size={14} /> Delete
                </Btn>
                <Btn onClick={() => setSelected([])} size="sm" tone="ghost">Clear</Btn>
              </div>
            </div>
          ) : null}
        </>
      )}

      {createKind ? (
        <FormModal
          confirmLabel="Create"
          description={`Created in ${directory} on ${nodeName}. Names cannot contain slashes or traversal segments.`}
          disabled={!createName.trim()}
          label={createKind === "folder" ? "Folder name" : "File name"}
          onClose={() => setCreateKind(null)}
          onSubmit={submitCreate}
          placeholder={createKind === "folder" ? "logs" : "config.txt"}
          title={`Create ${createKind}`}
          value={createName}
          onChange={setCreateName}
        />
      ) : null}

      {renameTarget ? (
        <FormModal
          confirmLabel="Rename"
          description={`${renameTarget.path} on ${nodeName} keeps its directory and gets a new name.`}
          disabled={!renameName.trim() || renameName.trim() === renameTarget.name}
          label="New name"
          onClose={() => setRenameTarget(null)}
          onSubmit={submitRename}
          title={`Rename ${renameTarget.name}`}
          value={renameName}
          onChange={setRenameName}
        />
      ) : null}

      {copyTarget ? (
        <FormModal
          confirmLabel="Copy"
          description={`Copies ${copyTarget.path} into ${directory} on ${nodeName}.`}
          disabled={!copyName.trim()}
          label="Copy name"
          onClose={() => setCopyTarget(null)}
          onSubmit={submitCopy}
          title={`Copy ${copyTarget.name}`}
          value={copyName}
          onChange={setCopyName}
        />
      ) : null}

      {chmodTarget ? (
        <FormModal
          confirmLabel="Apply"
          destructive
          description={`${chmodTarget.path} on ${nodeName} is currently ${chmodTarget.mode || NOT_REPORTED}. Applying asks for confirmation.`}
          disabled={!/^[0-7]{3,4}$/.test(chmodMode.trim())}
          hint="Three or four octal digits (e.g. 0644, 0755)."
          label="Octal mode"
          mono
          onClose={() => setChmodTarget(null)}
          onSubmit={() => void submitChmod()}
          placeholder="0644"
          title={`Permissions — ${chmodTarget.name}`}
          value={chmodMode}
          onChange={setChmodMode}
        />
      ) : null}

      {bulkChmodOpen ? (
        <FormModal
          confirmLabel={`Apply to ${selected.length}`}
          destructive
          description={`Applies to every selected path on ${nodeName}, one request each — the host has no batch chmod.`}
          disabled={!/^[0-7]{3,4}$/.test(bulkChmodMode.trim())}
          hint="Three or four octal digits (e.g. 0644, 0755)."
          label="Octal mode"
          mono
          onClose={() => setBulkChmodOpen(false)}
          onSubmit={() => void submitBulkChmod()}
          placeholder="0644"
          title={`Permissions — ${selected.length} selected`}
          value={bulkChmodMode}
          onChange={setBulkChmodMode}
        />
      ) : null}

      {pullOpen ? (
        <FormModal
          confirmLabel="Download and upload"
          description={`The browser fetches the URL and uploads the result into ${directory} on ${nodeName}. Beacon has no host-side pull endpoint, so the download starts here.`}
          disabled={!pullUrl.trim()}
          label="Public file URL"
          onClose={() => { setPullOpen(false); setPullUrl(""); }}
          onSubmit={() => void handlePull()}
          placeholder="https://example.com/file.jar"
          title="Pull from URL"
          type="url"
          value={pullUrl}
          onChange={setPullUrl}
        />
      ) : null}
    </div>
  );
}

/**
 * One labelled field, Enter-to-submit, and the shared dialog footer geometry —
 * used instead of the hand-rolled fixed-overlay dialogs this view carried.
 */
function FormModal({
  title,
  description,
  label,
  value,
  onChange,
  placeholder,
  type = "text",
  mono,
  hint,
  confirmLabel,
  disabled,
  destructive,
  onClose,
  onSubmit,
}: {
  title: string;
  description?: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  mono?: boolean;
  hint?: string;
  confirmLabel: string;
  disabled?: boolean;
  destructive?: boolean;
  onClose: () => void;
  onSubmit: () => void;
}) {
  return (
    <Modal description={description} onClose={onClose} title={title}>
      <form onSubmit={(event) => { event.preventDefault(); if (!disabled) onSubmit(); }}>
        <Input label={label} mono={mono} onChange={onChange} placeholder={placeholder} type={type} value={value} />
        {hint ? <p className="mt-2 text-xs text-text-subtle">{hint}</p> : null}
        <div className="ui-dialog-footer sticky bottom-0 z-10 -mx-5 -mb-4 mt-4">
          <Btn onClick={onClose} tone="ghost">Cancel</Btn>
          <Btn disabled={disabled} tone={destructive ? "danger" : "primary"} type="submit">{confirmLabel}</Btn>
        </div>
      </form>
    </Modal>
  );
}

