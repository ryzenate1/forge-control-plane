"use client";

import { useCallback, useMemo, useState } from "react";
import dynamic from "next/dynamic";
import { ChevronRight, Download, File, Folder, Save, ShieldX, Trash2 } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiServer } from "@/lib/api";
import {
  createContainerDir,
  deleteContainerFile,
  downloadContainerFile,
  listContainerFiles,
  readContainerFile,
  uploadContainerFiles,
  writeContainerFile,
  type ContainerFileEntry,
} from "@/lib/api/container-files";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { toast } from "@/components/ui/sonner";
import { errorMessage, formatBytes } from "@/lib/utils";

const MonacoEditor = dynamic(() => import("@monaco-editor/react"), { ssr: false, loading: () => <div className="grid h-full place-items-center text-sm text-[var(--text-subtle)]">Loading editor…</div> });
const button = "inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-[var(--line)] bg-white/[0.03] px-3 text-xs font-bold text-[var(--text)] transition hover:bg-white/[0.06] hover:border-[var(--line-strong)] disabled:cursor-not-allowed disabled:opacity-40";

function languageFor(path: string) { const extension = path.split(".").pop()?.toLowerCase(); return ({ json: "json", yml: "yaml", yaml: "yaml", sh: "shell", js: "javascript", ts: "typescript", jsx: "javascript", tsx: "typescript", html: "html", css: "css", xml: "xml", py: "python", java: "java", properties: "ini", env: "ini" } as Record<string, string>)[extension ?? ""] ?? "plaintext"; }
function joinPath(directory: string, name: string) { if (directory === "/") return `/${name}`; return `${directory}/${name}`; }

/**
 * ContainerFilesView — browses the workload container's own filesystem
 * (image layout, /etc, /usr/local/bin…) rather than the server volume.
 *
 * The daemon resolves the container from the server binding, so the browser
 * never handles container ids. The container API is deliberately narrower
 * than the volume API: no rename/move/copy/chmod/archive/pull, and deletes
 * of guarded filesystem roots are refused server-side.
 */
export function ContainerFilesView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canRead = hasServerPermission(access, "file.read");
  const canCreate = hasServerPermission(access, "file.create");
  const canUpdate = hasServerPermission(access, "file.update");
  const canDelete = hasServerPermission(access, "file.delete");
  const canDownload = hasServerPermission(access, "file.read-content");
  const queryClient = useQueryClient();
  const [directory, setDirectory] = useState("/");
  const [editing, setEditing] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [dirty, setDirty] = useState(false);
  const [initialContent, setInitialContent] = useState("");
  const [status, setStatus] = useState("Ready");
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [busy, setBusy] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ContainerFileEntry | null>(null);

  const refresh = useCallback(async () => { await queryClient.invalidateQueries({ queryKey: ["container-files", server?.id] }); }, [queryClient, server?.id]);
  const run = useCallback(async (label: string, action: () => Promise<void>) => { setBusy(true); setError(""); setStatus(label); try { await action(); setStatus(`${label} complete`); } catch (actionError) { setError(errorMessage(actionError, `${label} failed.`)); setStatus("Action failed"); } finally { setBusy(false); } }, []);

  const listing = useQuery({
    queryKey: ["container-files", server?.id, directory],
    queryFn: () => listContainerFiles(server?.id ?? "", directory || "/"),
    enabled: Boolean(server?.id && canRead),
  });
  const entries = useMemo(() => {
    const rows = [...(listing.data?.entries ?? [])].filter((entry) => entry.name.toLowerCase().includes(search.trim().toLowerCase()));
    return rows.sort((a, b) => {
      if ((a.type === "dir") !== (b.type === "dir")) return a.type === "dir" ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
  }, [listing.data, search]);

  const openFile = async (path: string) => {
    if (!canDownload) {
      setError("You do not have permission to view file contents on this server.");
      return;
    }
    setEditing(path); setContent(""); setDirty(false); setError(""); setStatus("Loading");
    try {
      const file = await readContainerFile(server?.id ?? "", path);
      if (file.binary) {
        setEditing(null);
        setError("This file looks binary. Use Download to fetch it instead.");
        setStatus("Binary file");
        return;
      }
      setContent(file.text); setInitialContent(file.text); setDirty(false); setStatus("Loaded");
    } catch (loadError) {
      setEditing(null);
      setError(errorMessage(loadError, "File could not be loaded.")); setStatus("Load failed");
    }
  };
  const save = () => void run("Saving", async () => { if (!server?.id || !editing) return; await writeContainerFile(server.id, editing, content); setInitialContent(content); setDirty(false); await refresh(); });
  const createFolder = () => { const name = window.prompt("Folder name")?.trim(); if (!name || name.includes("/")) { if (name) setError("Folder names cannot contain slashes."); return; } void run("Creating folder", async () => { await createContainerDir(server!.id, joinPath(directory, name)); await refresh(); }); };
  const createFile = () => { const name = window.prompt("File name")?.trim(); if (!name || name.includes("/")) { if (name) setError("File names cannot contain slashes."); return; } const path = joinPath(directory, name); void run("Creating file", async () => { await writeContainerFile(server!.id, path, ""); await refresh(); await openFile(path); }); };
  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const input = event.currentTarget;
    const uploadFiles = Array.from(input.files ?? []);
    input.value = "";
    if (!server?.id || !uploadFiles.length) return;
    await run("Uploading", async () => { await uploadContainerFiles(server.id, directory, uploadFiles); await refresh(); });
  };
  const download = (entry: ContainerFileEntry) => void run("Starting download", async () => {
    const blob = await downloadContainerFile(server!.id, entry.path);
    const href = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = href; anchor.download = entry.name; anchor.rel = "noreferrer"; anchor.click();
    URL.revokeObjectURL(href);
  });
  const performDelete = useCallback(async () => {
    if (!server?.id || !deleteTarget) return;
    setBusy(true); setError(""); setStatus("Deleting");
    try {
      await deleteContainerFile(server.id, deleteTarget.path);
      setDeleteTarget(null);
      setStatus("Delete complete");
      toast.success("Path deleted.");
      await refresh();
    } catch (deleteError) {
      setError(errorMessage(deleteError, "Delete failed."));
      setStatus("Delete failed");
      setDeleteTarget(null);
      toast.error(errorMessage(deleteError, "Delete failed."));
    } finally {
      setBusy(false);
    }
  }, [server?.id, deleteTarget, refresh]);

  if (!canRead) return <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-5 text-sm text-amber-100"><ShieldX className="mb-2" />You do not have permission to read this server&apos;s container files.</div>;

  if (editing) return <div className="space-y-4">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="truncate font-mono text-sm text-[var(--text)]">{editing}</p>
      <div className="flex items-center gap-2">
        <span className="text-xs text-[var(--text-subtle)]" role="status">{status}{dirty ? " · unsaved changes" : ""}</span>
        <button className={button} onClick={() => { setEditing(null); setDirty(false); setStatus("Ready"); }} type="button">Close</button>
        <button className="inline-flex h-9 items-center gap-2 rounded-lg bg-[var(--brand)] px-4 text-xs font-bold text-white hover:bg-[var(--brand-hover)] disabled:opacity-40 transition-colors" disabled={!canUpdate || busy} onClick={() => void save()} type="button"><Save size={15} />Save content</button>
      </div>
    </div>
    {error ? <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">{error}</div> : null}
    <div className="h-[65vh] min-h-96 overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)]"><MonacoEditor language={languageFor(editing)} onChange={(value) => { setContent(value ?? ""); setDirty(value !== initialContent); setStatus(value === initialContent ? "Loaded" : "Edited"); }} options={{ fontSize: 14, lineNumbers: "on", minimap: { enabled: false }, readOnly: !canUpdate, wordWrap: "on", automaticLayout: true }} theme="vs-dark" value={content} /></div>
  </div>;

  const parts = directory.split("/").filter(Boolean);

  return <div className="relative space-y-4">
    <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <nav aria-label="Container path" className="flex min-w-0 items-center gap-1 overflow-x-auto text-sm">
        <button className="shrink-0 font-semibold text-[var(--text)] hover:text-white transition-colors" onClick={() => { setDirectory("/"); setSearch(""); }} type="button">/</button>
        {parts.map((part, index) => { const path = `/${parts.slice(0, index + 1).join("/")}`; return <span className="flex shrink-0 items-center gap-1" key={path}><ChevronRight className="text-[color-mix(in_srgb,var(--text-subtle)_60%,transparent)] shrink-0" size={14} /><button className="text-[var(--text-subtle)] hover:text-white transition-colors truncate max-w-[120px] sm:max-w-[200px]" onClick={() => { setDirectory(path); setSearch(""); }} type="button">{part}</button></span>; })}
      </nav>
      <div className="flex flex-wrap gap-2">
        <button className={button} disabled={!canCreate || busy} onClick={createFolder} type="button"><Folder size={14} />New folder</button>
        <button className={button} disabled={!canCreate || busy} onClick={createFile} type="button"><File size={14} />New file</button>
        <label className={`${button} cursor-pointer ${!canCreate || busy ? "pointer-events-none opacity-40" : ""}`}>Upload<input accept="*/*" className="hidden" disabled={!canCreate || busy} multiple onChange={(event) => void upload(event)} type="file" /></label>
      </div>
    </div>
    <p className="text-xs text-[var(--text-subtle)]">Container filesystem — the image layout outside the server volume. No rename, move, or archive operations here; guarded system roots cannot be deleted.</p>
    {error || listing.isError ? <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200 flex items-center gap-2" role="alert"><span className="flex-1">{error || errorMessage(listing.error, "Container files could not be loaded.")}</span><button className="shrink-0 underline font-semibold hover:text-red-100 transition-colors" onClick={() => void listing.refetch()} type="button">Retry</button></div> : null}
    <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-[var(--text-subtle)]">
      <input className="h-9 w-44 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 text-xs text-[var(--text)] outline-none placeholder:text-[var(--text-subtle)] focus:border-[var(--brand)]" onChange={(event) => setSearch(event.target.value)} placeholder="Filter this folder" type="search" value={search} />
      <span role="status">{listing.isFetching ? "Loading…" : status}</span>
    </div>
    {listing.isLoading ? <div className="rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-8 text-center text-sm text-[var(--text-subtle)]">Loading container files…</div>
      : entries.length === 0 ? <div className="rounded-xl border border-dashed border-[var(--line)] bg-[var(--surface-input)] p-8 text-center text-sm text-[var(--text-subtle)]">{search ? "No files match this filter." : "This directory is empty."}</div>
      : <div className="overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface-input)]">{entries.map((entry) => (
        <article className="grid gap-3 border-b border-[var(--line)] p-3 last:border-0 sm:grid-cols-[28px_minmax(0,1fr)_100px_170px_auto] sm:items-center" key={entry.path}>
          <button className="flex min-w-0 items-center gap-3 text-left font-semibold text-[var(--text)] hover:text-white transition-colors col-span-2 sm:col-span-1" onClick={() => entry.type === "dir" ? (setDirectory(entry.path), setSearch("")) : void openFile(entry.path)} type="button">
            {entry.type === "dir" ? <Folder className="shrink-0 text-amber-400/80" size={20} /> : <File className="shrink-0 text-[var(--text-subtle)]" size={20} />}
            <span className="truncate">{entry.name}</span>
          </button>
          <span className="text-xs text-[var(--text-subtle)]">{entry.type === "dir" ? "Folder" : entry.type === "symlink" ? "Symlink" : formatBytes(entry.size ?? 0)}</span>
          <span className="truncate font-mono text-xs text-[var(--text-subtle)]">{entry.modified || entry.mode || ""}</span>
          <span className="flex items-center gap-1 sm:justify-self-end">
            {entry.type === "dir" ? null : <button aria-label={`Download ${entry.name}`} className="grid h-9 w-9 place-items-center rounded hover:bg-[var(--surface-raised)] disabled:opacity-40" disabled={!canDownload || busy} onClick={() => download(entry)} title="Download" type="button"><Download size={18} /></button>}
            <button aria-label={`Delete ${entry.name}`} className="grid h-9 w-9 place-items-center rounded text-danger hover:bg-[var(--surface-raised)] disabled:opacity-40" disabled={!canDelete || busy} onClick={() => setDeleteTarget(entry)} title="Delete" type="button"><Trash2 size={18} /></button>
          </span>
        </article>
      ))}</div>}
    <ConfirmDialog
      danger
      open={Boolean(deleteTarget)}
      title={deleteTarget ? `Delete ${deleteTarget.path}?` : ""}
      description={deleteTarget?.type === "dir" ? "The directory and everything inside it will be permanently removed. Guarded system roots are refused by the server." : "This cannot be undone."}
      confirmLabel={busy ? "Deleting…" : "Delete"}
      onClose={() => { if (!busy) setDeleteTarget(null); }}
      onConfirm={() => void performDelete()}
    />
  </div>;
}
