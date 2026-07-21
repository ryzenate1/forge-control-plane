"use client";
/* eslint-disable @next/next/no-img-element -- previews use authenticated, short-lived file URLs */

import { DragEvent, FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import dynamic from "next/dynamic";
import { Archive, ArrowUpDown, CheckSquare, ChevronRight, Download, File, Folder, FolderInput, Grid2X2, List, MoreHorizontal, Save, Search, ShieldX, Trash2, Upload, X } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiFileEntry, type ApiServer, chmodServerFile, downloadFileToServer, fetchServerFiles, writeServerFile } from "@/lib/api";
import { archiveServerFile, copyServerFile, createServerDirectory, deleteServerFiles, decompressServerFiles, getServerFileDownloadURL, readServerFile, renameServerFiles, uploadFileChunked } from "@/lib/api/files";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage, formatBytes } from "@/lib/utils";

const MonacoEditor = dynamic(() => import("@monaco-editor/react"), { ssr: false, loading: () => <div className="grid h-full place-items-center text-sm text-slate-400">Loading editor…</div> });
const button = "inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-white/10 px-3 text-xs font-bold text-slate-200 transition hover:bg-white/5 disabled:cursor-not-allowed disabled:opacity-40";

function languageFor(path: string) { const extension = path.split(".").pop()?.toLowerCase(); return ({ json: "json", yml: "yaml", yaml: "yaml", sh: "shell", js: "javascript", ts: "typescript", jsx: "javascript", tsx: "typescript", html: "html", css: "css", xml: "xml", py: "python", java: "java", properties: "ini", env: "ini" } as Record<string, string>)[extension ?? ""] ?? "plaintext"; }
function join(directory: string, name: string) { return [directory, name].filter(Boolean).join("/"); }

function Breadcrumbs({ directory, onOpen }: { directory: string; onOpen: (path: string) => void }) {
  const parts = directory.split("/").filter(Boolean);
  return <nav aria-label="File path" className="flex min-w-0 items-center gap-1 overflow-x-auto text-sm"><button className="shrink-0 font-semibold text-slate-200 hover:text-white" onClick={() => onOpen("")} type="button">container</button>{parts.map((part, index) => { const path = parts.slice(0, index + 1).join("/"); return <span className="flex shrink-0 items-center gap-1" key={path}><ChevronRight className="text-slate-600" size={14} /><button className="text-slate-400 hover:text-white" onClick={() => onOpen(path)} type="button">{part}</button></span>; })}</nav>;
}

export function FilesView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canRead = hasServerPermission(access, "file.read");
  const canCreate = hasServerPermission(access, "file.create");
  const canUpdate = hasServerPermission(access, "file.update");
  const canDelete = hasServerPermission(access, "file.delete");
  const canArchive = hasServerPermission(access, "file.archive");
  const canDownload = hasServerPermission(access, "file.read-content");
  const queryClient = useQueryClient();
  const [directory, setDirectory] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [editing, setEditing] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [fileLoaded, setFileLoaded] = useState(false);
  const [status, setStatus] = useState("Ready");
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"list" | "grid">("list");
  const [busy, setBusy] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [dragging, setDragging] = useState(false);
  const dragCounter = useState(0);
  const [sortBy, setSortBy] = useState<"name" | "size" | "date">("name");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");
  const [preview, setPreview] = useState<{ url: string; name: string } | null>(null);
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; entry: ApiFileEntry } | null>(null);

  const refresh = useCallback(async () => { await queryClient.invalidateQueries({ queryKey: ["server-files", server?.id] }); }, [queryClient, server?.id]);
  const run = useCallback(async (label: string, action: () => Promise<void>) => { setBusy(true); setError(""); setStatus(label); try { await action(); setStatus(`${label} complete`); } catch (actionError) { setError(errorMessage(actionError, `${label} failed.`)); setStatus("Action failed"); } finally { setBusy(false); } }, []);

  /* Drag-and-drop upload */
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
    if (!server?.id || !canCreate || busy) return;
    const droppedFiles = Array.from(event.dataTransfer.files);
    if (!droppedFiles.length) return;
    await run("Uploading dropped files", async () => {
      for (let index = 0; index < droppedFiles.length; index += 1) {
        const file = droppedFiles[index];
        await uploadFileChunked(server.id, join(directory, file.name), file, (progress) =>
          setUploadProgress(Math.round(((index + progress / 100) / droppedFiles.length) * 100)),
        );
      }
      setUploadProgress(null);
      await refresh();
    });
  }, [server?.id, canCreate, busy, directory, run, refresh, dragCounter]);

  const files = useQuery({ queryKey: ["server-files", server?.id, directory], queryFn: () => fetchServerFiles(server?.id ?? "", directory), enabled: Boolean(server?.id && canRead) });
  const entries = useMemo(() => [...(files.data ?? [])].filter((entry) => entry.name.toLowerCase().includes(search.trim().toLowerCase())).sort((a, b) => { if (a.directory !== b.directory) return Number(b.directory) - Number(a.directory); const cmp = sortBy === "size" ? (a.size ?? 0) - (b.size ?? 0) : sortBy === "date" ? (a.modifiedAt ?? "").localeCompare(b.modifiedAt ?? "") : a.name.localeCompare(b.name); return sortDir === "desc" ? -cmp : cmp; }), [files.data, search, sortBy, sortDir]);

  useEffect(() => { setSelected([]); setSearch(""); }, [directory]);
  useEffect(() => () => { const monaco = (window as unknown as { monaco?: { editor?: { getModels(): Array<{ dispose(): void }> } } }).monaco; monaco?.editor?.getModels().forEach((model) => model.dispose()); }, [editing]);

  const openFile = async (path: string) => { setEditing(path); setContent(""); setFileLoaded(false); setError(""); setStatus("Loading"); try { const value = await readServerFile(server?.id ?? "", path); setContent(value); setFileLoaded(true); setStatus("Loaded"); } catch (loadError) { setError(errorMessage(loadError, "File could not be loaded.")); setStatus("Load failed"); } };
  const save = () => run("Saving", async () => { if (!server?.id || !editing || !fileLoaded) return; await writeServerFile(server.id, editing, content); });
  const promptName = (kind: "file" | "folder") => { const value = window.prompt(`${kind === "file" ? "File" : "Folder"} name`)?.trim(); if (!value || value.includes("/") || value === "." || value === "..") { if (value) setError("Names cannot contain slashes or path traversal segments."); return null; } return value; };
  const createFolder = () => { const name = promptName("folder"); if (!name) return; void run("Creating folder", async () => { await createServerDirectory(server!.id, join(directory, name)); await refresh(); }); };
  const createFile = () => { const name = promptName("file"); if (!name) return; const path = join(directory, name); void run("Creating file", async () => { await writeServerFile(server!.id, path, ""); await refresh(); await openFile(path); }); };
  const upload = async (event: FormEvent<HTMLInputElement>) => { const input = event.currentTarget; const uploadFiles = Array.from(input.files ?? []); input.value = ""; if (!server?.id || !uploadFiles.length) return; await run("Uploading", async () => { for (let index = 0; index < uploadFiles.length; index += 1) { const file = uploadFiles[index]; await uploadFileChunked(server.id, join(directory, file.name), file, (progress) => setUploadProgress(Math.round(((index + progress / 100) / uploadFiles.length) * 100))); } setUploadProgress(null); await refresh(); }); };
  const fromUrl = () => { const raw = window.prompt("Public file URL to download")?.trim(); if (!raw) return; let parsed: URL; try { parsed = new URL(raw); if (!/^https?:$/.test(parsed.protocol)) throw new Error(); } catch { setError("Enter a valid HTTP or HTTPS URL."); return; } const name = decodeURIComponent(parsed.pathname.split("/").filter(Boolean).pop() || "downloaded-file"); void run("Downloading URL", async () => { await downloadFileToServer(server!.id, raw, join(directory, name)); await refresh(); }); };
  const rename = (entry: ApiFileEntry) => { const name = window.prompt("Rename to", entry.name)?.trim(); if (!name || name === entry.name) return; if (name.includes("/")) { setError("The new name cannot contain a slash."); return; } void run("Renaming", async () => { await renameServerFiles(server!.id, [{ from: entry.path, to: join(directory, name) }]); await refresh(); }); };
  const remove = useCallback((items: ApiFileEntry[]) => { if (!items.length || !window.confirm(`Permanently delete ${items.length} selected item${items.length === 1 ? "" : "s"}?`)) return; void run("Deleting", async () => { try { await deleteServerFiles(server!.id, items.map((item) => item.path)); setSelected([]); } finally { await refresh(); } }); }, [refresh, run, server]);
  const move = (items: ApiFileEntry[]) => { const destination = window.prompt("Move selected items to directory (relative to container root)", directory)?.replace(/^\/+|\/+$/g, ""); if (destination == null) return; void run("Moving", async () => { try { await renameServerFiles(server!.id, items.map((item) => ({ from: item.path, to: join(destination, item.name) }))); setSelected([]); } finally { await refresh(); } }); };
  const copy = (items: ApiFileEntry[]) => { if (items.some((item) => item.directory)) { setError("Directory copying is not supported; select regular files only."); return; } const destination = window.prompt("Copy selected files to directory (relative to container root)", directory)?.replace(/^\/+|\/+$/g, ""); if (destination == null) return; void run("Copying", async () => { for (const item of items) await copyServerFile(server!.id, item.path, join(destination, item.name)); setSelected([]); await refresh(); }); };
  const chmod = (items: ApiFileEntry[]) => { const mode = window.prompt("Apply octal permissions to selected items", "0644")?.trim(); if (mode == null) return; if (!/^[0-7]{3,4}$/.test(mode)) { setError("Permissions must contain three or four octal digits, for example 0644."); return; } void run("Updating permissions", async () => { for (const item of items) await chmodServerFile(server!.id, item.path, mode); setSelected([]); await refresh(); }); };
  const download = (entry: ApiFileEntry) => void run("Starting download", async () => { const { url } = await getServerFileDownloadURL(server!.id, entry.path); const anchor = document.createElement("a"); anchor.href = url; anchor.download = entry.name; anchor.rel = "noreferrer"; anchor.click(); });
  const archive = (entry: ApiFileEntry) => void run("Preparing archive download", async () => { const blob = await archiveServerFile(server!.id, entry.path); const href = URL.createObjectURL(blob); const anchor = document.createElement("a"); anchor.href = href; anchor.download = `${entry.name}.tar.gz`; anchor.click(); URL.revokeObjectURL(href); });
  const extract = (entry: ApiFileEntry) => void run("Extracting archive", async () => { await decompressServerFiles(server!.id, entry.path); await refresh(); });
  const selectedEntries = useMemo(() => (files.data ?? []).filter((entry: ApiFileEntry) => selected.includes(entry.path)), [files.data, selected]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (editing || preview) return;
      if (event.key === "Delete" || event.key === "Backspace") { if (selectedEntries.length) remove(selectedEntries); return; }
      if ((event.ctrlKey || event.metaKey) && event.key === "a") { event.preventDefault(); setSelected(entries.map((e) => e.path)); return; }
      if ((event.ctrlKey || event.metaKey) && event.key === "f") { (document.querySelector<HTMLInputElement>('[type="search"]') ?? document.querySelector<HTMLInputElement>('input[placeholder*="Filter"]'))?.focus(); event.preventDefault(); }
      if (event.key === "Escape") { setSelected([]); setContextMenu(null); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [editing, preview, entries, selectedEntries, remove]);
  if (!canRead) return <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-5 text-sm text-amber-100"><ShieldX className="mb-2" />You do not have permission to read this server&apos;s files.</div>;
  if (editing) return <div className="space-y-4"><div className="flex flex-wrap items-center justify-between gap-3"><Breadcrumbs directory={editing} onOpen={(path) => { setEditing(null); setDirectory(path); }} /><div className="flex items-center gap-2"><span className="text-xs text-slate-400" role="status">{status}</span><button className={button} onClick={() => { setEditing(null); setFileLoaded(false); }} type="button">Close</button><button className="inline-flex h-9 items-center gap-2 rounded-lg bg-red-600 px-4 text-xs font-bold text-white disabled:opacity-40" disabled={!canUpdate || !fileLoaded || busy} onClick={() => void save()} type="button"><Save size={15} />Save content</button></div></div>{error ? <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">{error}</div> : null}<div className="h-[65vh] min-h-96 overflow-hidden rounded-xl border border-white/10 bg-[#111722]"><MonacoEditor language={languageFor(editing)} onChange={(value) => { if (fileLoaded) { setContent(value ?? ""); setStatus("Edited"); } }} options={{ fontSize: 14, lineNumbers: "on", minimap: { enabled: false }, readOnly: !fileLoaded || !canUpdate, wordWrap: "on", automaticLayout: true }} theme="vs-dark" value={content} /></div></div>;

  return <div className="relative space-y-4" onDragEnter={handleDragEnter} onDragLeave={handleDragLeave} onDragOver={handleDragOver} onDrop={handleDrop}>
    {/* Drag-and-drop overlay */}
    {dragging ? <div aria-live="assertive" className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" role="status"><div className="flex flex-col items-center gap-4 rounded-2xl border-2 border-dashed border-red-500/60 bg-[#151b27]/90 px-16 py-12 text-center shadow-2xl"><Upload className="h-12 w-12 text-red-400 animate-bounce" /><p className="text-lg font-bold text-white">Drop files to upload</p><p className="text-sm text-slate-400">Files will be uploaded to the current directory</p></div></div> : null}

    <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between"><Breadcrumbs directory={directory} onOpen={setDirectory} /><div className="flex flex-wrap gap-2"><label className="relative"><Search className="absolute left-3 top-2.5 text-slate-500" size={14} /><span className="sr-only">Filter files</span><input className="h-9 w-44 rounded-lg border border-white/10 bg-[#151b27] pl-9 pr-3 text-xs text-white outline-none focus:border-red-500" onChange={(event) => setSearch(event.target.value)} placeholder="Filter this folder" type="search" value={search} /></label><button aria-label="List view" className={button} onClick={() => setView("list")} type="button"><List size={15} /></button><button aria-label="Grid view" className={button} onClick={() => setView("grid")} type="button"><Grid2X2 size={15} /></button><button aria-label={`Sort by ${sortBy} ${sortDir === "asc" ? "ascending" : "descending"}`} className={button} disabled={view === "grid"} onClick={() => { const next = { name: "size", size: "date", date: "name" } as const; if (sortBy === "name" && sortDir === "asc") { setSortBy("name"); setSortDir("desc"); } else if (sortBy === "name") { setSortBy(next[sortBy]); setSortDir("asc"); } else setSortBy(next[sortBy]); }} type="button"><ArrowUpDown size={14} /><span className="hidden sm:inline">{sortBy} {sortDir === "asc" ? "↑" : "↓"}</span></button><button className={button} disabled={!canCreate || busy} onClick={createFolder} type="button"><Folder size={14} />New folder</button><button className={button} disabled={!canCreate || busy} onClick={createFile} type="button"><File size={14} />New file</button><button className={button} disabled={!canCreate || busy} onClick={fromUrl} type="button"><Download size={14} />Pull URL</button><label className={`${button} cursor-pointer ${!canCreate || busy ? "pointer-events-none opacity-40" : ""}`}><Upload size={14} />Upload<input className="sr-only" disabled={!canCreate || busy} multiple onChange={upload} type="file" /></label></div></div>
    {(uploadProgress !== null) ? <div className="rounded-lg border border-white/10 bg-[#151b27] p-3" role="status"><div className="flex justify-between text-xs"><span>Uploading files</span><span>{uploadProgress}%</span></div><div className="mt-2 h-2 overflow-hidden rounded bg-slate-800"><div className="h-full bg-red-600 transition-all" style={{ width: `${uploadProgress}%` }} /></div></div> : null}
    {error || files.isError ? <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">{error || errorMessage(files.error, "Files could not be loaded.")}<button className="ml-3 underline" onClick={() => void files.refetch()} type="button">Retry</button></div> : null}
    <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-slate-400"><label className="flex items-center gap-2"><input checked={Boolean(entries.length) && entries.every((entry) => selected.includes(entry.path))} onChange={(event) => setSelected(event.target.checked ? entries.map((entry) => entry.path) : [])} type="checkbox" />Select all visible</label><span role="status">{files.isFetching ? "Loading…" : status}</span></div>
    {files.isLoading ? <div className="rounded-xl border border-white/10 bg-[#151b27] p-8 text-center text-sm text-slate-400">Loading files…</div> : entries.length === 0 ? <div className="rounded-xl border border-dashed border-white/10 bg-[#151b27] p-8 text-center text-sm text-slate-400">{search ? "No files match this filter." : "This directory is empty. Drag and drop files here to upload."}</div> : <div className={view === "grid" ? "grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4" : "overflow-hidden rounded-xl border border-white/[0.07] bg-[#151b27]"}>{entries.map((entry) => { const checked = selected.includes(entry.path); const archiveType = /\.(zip|tar|tar\.gz|tgz|gz)$/i.test(entry.name); return <article className={view === "grid" ? "rounded-xl border border-white/[0.07] bg-[#151b27] p-4" : "grid gap-3 border-b border-white/[0.06] p-3 last:border-0 sm:grid-cols-[28px_minmax(0,1fr)_100px_170px_auto] sm:items-center"} key={entry.path} onContextMenu={(event) => { event.preventDefault(); setContextMenu({ x: event.clientX, y: event.clientY, entry }); }}><input aria-label={`Select ${entry.name}`} checked={checked} onChange={() => setSelected((items) => checked ? items.filter((item) => item !== entry.path) : [...items, entry.path])} type="checkbox" /><button className="flex min-w-0 items-center gap-3 text-left font-semibold text-slate-100 hover:text-white" onClick={() => entry.directory ? setDirectory(entry.path) : void openFile(entry.path)} type="button">{entry.directory ? <Folder className="shrink-0 text-amber-300" size={20} /> : <File className="shrink-0 text-slate-400" size={20} />}<span className="truncate">{entry.name}</span></button><span className="text-xs text-slate-400">{entry.directory ? "Folder" : formatBytes(entry.size)}</span><time className="truncate text-xs text-slate-500">{entry.modifiedAt || "Modified time unavailable"}</time><div className="flex justify-end gap-1">{!entry.directory ? <button aria-label={`Download ${entry.name}`} className={button} disabled={!canDownload || busy} onClick={() => download(entry)} type="button"><Download size={14} /></button> : null}<button aria-label="Archive entry" className={button} disabled={!canArchive || busy} onClick={() => archive(entry)} title="Download as archive" type="button"><Archive size={14} /></button>{archiveType && !entry.directory ? <button aria-label="Extract archive" className={button} disabled={!canCreate || busy} onClick={() => extract(entry)} type="button"><Download size={14} /></button> : null}<button aria-label="Rename entry" className={button} disabled={!canUpdate || busy} onClick={() => rename(entry)} type="button"><MoreHorizontal size={14} /></button><button aria-label="Delete entry" className={button} disabled={!canDelete || busy} onClick={() => remove([entry])} type="button"><Trash2 className="text-red-300" size={14} /></button></div></article>; })}</div>}
    {selectedEntries.length ? <div className="sticky bottom-4 z-20 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-white/10 bg-[#20283a]/95 p-3 shadow-2xl backdrop-blur"><span className="flex items-center gap-2 text-sm font-semibold"><CheckSquare size={17} />{selectedEntries.length} selected</span><div className="flex gap-2"><button className={button} disabled={!canUpdate || busy} onClick={() => move(selectedEntries)} type="button"><FolderInput size={14} />Move</button><button className={button} disabled={!canCreate || busy} onClick={() => copy(selectedEntries)} type="button">Copy</button><button className={button} disabled={!canUpdate || busy} onClick={() => chmod(selectedEntries)} type="button">Permissions</button><button className={button} disabled={!canDelete || busy} onClick={() => remove(selectedEntries)} type="button"><Trash2 size={14} />Delete</button></div></div> : null}
    {/* Right-click context menu */}
    {contextMenu ? <div className="fixed inset-0 z-50" onClick={() => setContextMenu(null)} onContextMenu={(event) => { event.preventDefault(); setContextMenu(null); }}><div className="absolute w-44 rounded-xl border border-white/10 bg-[#20283a] p-1 shadow-2xl backdrop-blur" onClick={(event) => event.stopPropagation()} style={{ left: contextMenu.x, top: contextMenu.y }}><button className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-xs text-slate-200 hover:bg-white/5" disabled={busy || !canDownload} onClick={() => { download(contextMenu.entry); setContextMenu(null); }} type="button"><Download size={14} />Download</button><button className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-xs text-slate-200 hover:bg-white/5" disabled={busy || !canUpdate} onClick={() => { rename(contextMenu.entry); setContextMenu(null); }} type="button"><MoreHorizontal size={14} />Rename</button><button className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-xs text-slate-200 hover:bg-white/5" disabled={busy || !canArchive} onClick={() => { archive(contextMenu.entry); setContextMenu(null); }} type="button"><Archive size={14} />Archive</button>{contextMenu.entry.directory ? null : <button className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-xs text-slate-200 hover:bg-white/5" disabled={busy || !canDownload} onClick={() => { const imgExt = /\.(png|jpe?g|gif|svg|webp|ico|bmp)$/i; if (imgExt.test(contextMenu.entry.name)) { setPreview({ url: `${process.env.NEXT_PUBLIC_API_URL || ""}/api/v1/servers/${server!.id}/files/download?file=${encodeURIComponent(contextMenu.entry.path)}`, name: contextMenu.entry.name }); } setContextMenu(null); }} type="button"><File size={14} />Preview</button>}<hr className="my-1 border-white/10" /><button className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-xs text-red-300 hover:bg-red-500/10" disabled={busy || !canDelete} onClick={() => { remove([contextMenu.entry]); setContextMenu(null); }} type="button"><Trash2 size={14} />Delete</button></div></div> : null}
    {/* Image preview modal */}
    {preview ? <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm" onClick={() => setPreview(null)}><div className="relative max-h-[90vh] max-w-[90vw] rounded-xl border border-white/10 bg-[#151b27] p-4 shadow-2xl" onClick={(event) => event.stopPropagation()}><div className="mb-3 flex items-center justify-between"><p className="text-sm font-semibold text-slate-200">{preview.name}</p><button className="rounded-lg p-1 text-slate-400 hover:bg-white/5" onClick={() => setPreview(null)} type="button"><X size={18} /></button></div><img alt={preview.name} className="max-h-[75vh] max-w-full rounded-lg object-contain" onError={() => setPreview(null)} src={preview.url} /></div></div> : null}
    <p className="text-xs text-slate-500">Drag and drop files to upload, or use the Upload button. Right-click for context menu. Direct downloads use short-lived single-use streaming tickets.</p>
  </div>;
}
