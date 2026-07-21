"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronRight, Download, File, Folder, FolderUp, RefreshCw, Save, Trash2, Upload, Search, Lock, Copy,
} from "lucide-react";
import { cn, errorMessage, formatBytes } from "@/lib/utils";
import {
  listFiles, readFile, writeFile, createDir, deleteFile, renameFile, copyFile, chmodFile, downloadFile, uploadFile,
  type FileEntry,
} from "@/lib/api/host-files";

const btn = cn(
  "inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-white/10",
  "px-3 text-xs font-semibold text-slate-200 transition-all",
  "hover:bg-white/5 hover:border-white/20 disabled:cursor-not-allowed disabled:opacity-40",
);

const iconBtn = cn(
  "rounded-lg p-2 text-slate-400 transition-all",
  "hover:bg-white/5 hover:text-white disabled:opacity-40 disabled:cursor-not-allowed",
);

function Breadcrumbs({ directory, onOpen }: { directory: string; onOpen: (path: string) => void }) {
  const parts = directory.split("/").filter(Boolean);
  return (
    <nav aria-label="File path" className="flex min-w-0 items-center gap-1 overflow-x-auto text-sm">
      <button
        className="shrink-0 font-semibold text-slate-200 hover:text-white transition-colors"
        onClick={() => onOpen("/")}
        type="button"
      >
        root
      </button>
      {parts.map((part, index) => {
        const path = "/" + parts.slice(0, index + 1).join("/");
        return (
          <span className="flex shrink-0 items-center gap-1" key={path}>
            <ChevronRight className="text-slate-600 shrink-0" size={14} />
            <button
              className="text-slate-400 hover:text-white transition-colors truncate max-w-[120px] sm:max-w-[200px]"
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

  const refresh = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ["host-files", directory] });
  }, [queryClient, directory]);

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
    queryKey: ["host-files", directory],
    queryFn: () => listFiles(directory),
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

  const openFile = async (path: string) => {
    setEditing(path);
    setContent("");
    setFileLoaded(false);
    setError("");
    setStatus("Loading");
    try {
      const value = await readFile(path);
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
      await writeFile(editing, content);
    });

  const promptName = (kind: "file" | "folder") => {
    const value = window.prompt(`${kind === "file" ? "File" : "Folder"} name`)?.trim();
    if (!value || value.includes("/") || value === "." || value === "..") {
      if (value) setError("Names cannot contain slashes or path traversal segments.");
      return null;
    }
    return value;
  };

  const createFolder = () => {
    const name = promptName("folder");
    if (!name) return;
    void run("Creating folder", async () => {
      await createDir(directory === "/" ? "/" + name : directory + "/" + name);
      await refresh();
    });
  };

  const handleUpload = async (event: FormEvent<HTMLInputElement>) => {
    const input = event.currentTarget;
    const uploadFiles = Array.from(input.files ?? []);
    input.value = "";
    if (!uploadFiles.length) return;
    await run("Uploading", async () => {
      for (const file of uploadFiles) {
        await uploadFile(directory, file);
      }
      await refresh();
    });
  };

  const handleRename = (entry: FileEntry) => {
    const name = window.prompt("Rename to", entry.name)?.trim();
    if (!name || name === entry.name || name.includes("/")) return;
    void run("Renaming", async () => {
      const parentPath = entry.path.includes("/") ? entry.path.substring(0, entry.path.lastIndexOf("/") + 1) : "";
      await renameFile(entry.path, parentPath + name);
      await refresh();
    });
  };

  const handleCopy = (entry: FileEntry) => {
    const name = window.prompt("Copy to name", "copy_of_" + entry.name)?.trim();
    if (!name || name.includes("/")) return;
    void run("Copying", async () => {
      const parentPath = entry.path.includes("/") ? entry.path.substring(0, entry.path.lastIndexOf("/") + 1) : "";
      await copyFile(entry.path, parentPath + name);
      await refresh();
    });
  };

  const handleDelete = (entry: FileEntry) => {
    if (!window.confirm(`Permanently delete ${entry.isDir ? "directory" : "file"} "${entry.name}"?`)) return;
    void run("Deleting", async () => {
      await deleteFile(entry.path);
      await refresh();
    });
  };

  const handleChmod = (entry: FileEntry) => {
    const mode = window.prompt("Enter octal permissions (e.g. 0644)", entry.mode)?.trim();
    if (!mode || !/^[0-7]{3,4}$/.test(mode)) {
      if (mode) setError("Permissions must be three or four octal digits.");
      return;
    }
    void run("Changing permissions", async () => {
      await chmodFile(entry.path, mode);
      await refresh();
    });
  };

  const handleDownload = (entry: FileEntry) => {
    void run("Downloading", async () => {
      const blob = await downloadFile(entry.path);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(url);
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
  }, [directory]);

  if (editing) {
    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Breadcrumbs directory={editing} onOpen={(path) => { setEditing(null); setDirectory(path); }} />
          <div className="flex items-center gap-2">
            <span className="text-xs text-slate-500 font-mono" role="status">{status}</span>
            <button className={btn} onClick={() => { setEditing(null); setFileLoaded(false); }} type="button">
              Close
            </button>
            <button
              className="inline-flex h-9 items-center gap-2 rounded-lg bg-red-600 px-4 text-xs font-bold text-white hover:bg-red-500 disabled:opacity-40 transition-colors"
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
        <div className="h-[65vh] min-h-96 overflow-hidden rounded-xl border border-white/10 bg-[#0d131d] p-4 font-mono text-sm text-slate-200">
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
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <Breadcrumbs directory={directory} onOpen={setDirectory} />
        <div className="flex flex-wrap gap-2">
          <label className="relative">
            <Search className="absolute left-3 top-2.5 text-slate-500 pointer-events-none" size={14} />
            <span className="sr-only">Filter files</span>
            <input
              className="h-9 w-40 rounded-lg border border-white/10 bg-[#0d131d] pl-9 pr-3 text-xs text-white outline-none placeholder:text-slate-600 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15 transition-all"
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Filter files..."
              type="search"
              value={search}
            />
          </label>
          <button className={btn} onClick={createFolder} disabled={busy} type="button">
            <Folder size={14} /><span className="hidden sm:inline">New folder</span>
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
      </div>

      {/* Error */}
      {(error || files.isError) ? (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200 flex items-center gap-2" role="alert">
          <span className="flex-1">{error || errorMessage(files.error, "Files could not be loaded.")}</span>
          <button className="shrink-0 underline font-semibold hover:text-red-100 transition-colors" onClick={() => void files.refetch()} type="button">Retry</button>
        </div>
      ) : null}

      {/* Loading skeleton */}
      {files.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-12 animate-pulse rounded-lg bg-white/[0.04]" />
          ))}
        </div>
      ) : entries.length === 0 && !files.isError ? (
        <div className="rounded-xl border border-dashed border-white/10 bg-[#0d131d] p-10 text-center">
          <File className="mx-auto mb-3 text-slate-600" size={32} strokeWidth={1} />
          <p className="text-sm text-slate-400">
            {search ? "No files match this filter." : "This directory is empty."}
          </p>
        </div>
      ) : entries.length > 0 ? (
        <>
          {/* Column headers - desktop only */}
          <div className="hidden sm:grid sm:grid-cols-[1fr_100px_170px_auto] gap-3 px-3 py-2 text-[11px] font-bold uppercase tracking-widest text-slate-600">
            <button className="text-left flex items-center gap-1 hover:text-slate-400 transition-colors" onClick={() => toggleSort("name")} type="button">
              Name {sortBy === "name" && (sortDir === "asc" ? "↑" : "↓")}
            </button>
            <button className="text-left flex items-center gap-1 hover:text-slate-400 transition-colors" onClick={() => toggleSort("size")} type="button">
              Size {sortBy === "size" && (sortDir === "asc" ? "↑" : "↓")}
            </button>
            <button className="text-left flex items-center gap-1 hover:text-slate-400 transition-colors" onClick={() => toggleSort("date")} type="button">
              Modified {sortBy === "date" && (sortDir === "asc" ? "↑" : "↓")}
            </button>
            <div />
          </div>

          {/* File entries */}
          <div className="overflow-hidden rounded-xl border border-white/[0.07] bg-[#0d131d]">
            {entries.map((entry) => (
              <div
                className="grid gap-2 border-b border-white/[0.06] px-3 py-2.5 last:border-0 hover:bg-white/[0.02] transition-colors sm:grid-cols-[1fr_100px_170px_auto] sm:items-center"
                key={entry.path}
              >
                <button
                  className="flex min-w-0 items-center gap-3 text-left font-medium text-slate-100 hover:text-white transition-colors"
                  onClick={() => entry.isDir ? setDirectory(entry.path) : void openFile(entry.path)}
                  type="button"
                >
                  {entry.isDir
                    ? <Folder className="shrink-0 text-amber-400/80" size={18} />
                    : <File className="shrink-0 text-slate-500" size={18} />
                  }
                  <span className="truncate">{entry.name}</span>
                </button>
                <span className="text-xs text-slate-500 truncate">{entry.isDir ? "Folder" : formatBytes(entry.size)}</span>
                <time className="text-xs text-slate-600 truncate hidden sm:block">{entry.modTime || "—"}</time>
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
            ))}
          </div>
        </>
      ) : null}
    </div>
  );
}
