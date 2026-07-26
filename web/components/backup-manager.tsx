"use client";

import { useEffect, useRef, useState } from "react";
import type { Backup, BackupFormat, Server } from "@/lib/api";
import { formatBytes } from "@/lib/format";
import { sanitizeError } from "@/lib/sanitize";
import { getCSRFToken } from "@/lib/csrf";

const POLL_INTERVAL_MS = 30000;

export function BackupManager() {
  const [servers, setServers] = useState<Server[]>([]);
  const [selectedServer, setSelectedServer] = useState<string>("");
  const [backups, setBackups] = useState<Backup[]>([]);
  const [format, setFormat] = useState<BackupFormat>("zip");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [confirmRestore, setConfirmRestore] = useState<{ id: string; paths: string } | null>(null);
  const [restorePathsInput, setRestorePathsInput] = useState("");
  const [backupRefreshKey, setBackupRefreshKey] = useState(0);
  const [pollError, setPollError] = useState(false);
  const serverTokenRef = useRef(0);
  const backupTokenRef = useRef(0);
  const dismissError = () => setError(null);

  useEffect(() => {
    const controller = new AbortController();
    const token = ++serverTokenRef.current;
    setLoading(true);
    fetch("/api/proxy/servers", { signal: controller.signal, credentials: "include" })
      .then((r) => {
        if (!r.ok) throw new Error(`Failed to load: ${r.status}`);
        return r.json();
      })
      .then((data) => {
        if (token !== serverTokenRef.current) return;
        const list = Array.isArray(data) ? data : data.data ?? [];
        setServers(list);
        if (list.length > 0) setSelectedServer(list[0].id);
      })
      .catch((e) => {
        if (token === serverTokenRef.current) {
          if (e instanceof DOMException && e.name === "AbortError") return;
          setError(sanitizeError(e instanceof Error ? e.message : "An unexpected error occurred"));
        }
      })
      .finally(() => { if (token === serverTokenRef.current) setLoading(false); });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!selectedServer) return;
    const controller = new AbortController();
    const token = ++backupTokenRef.current;
    setLoading(true);
    fetch(`/api/proxy/servers/${encodeURIComponent(selectedServer)}/backups`, { signal: controller.signal, credentials: "include" })
      .then((r) => {
        if (!r.ok) throw new Error(`Failed to load: ${r.status}`);
        return r.json();
      })
      .then((data) => {
        if (token !== backupTokenRef.current) return;
        setBackups(Array.isArray(data) ? data : data.data ?? []);
      })
      .catch((e) => {
        if (token === backupTokenRef.current) {
          if (e instanceof DOMException && e.name === "AbortError") return;
          setError(sanitizeError(e instanceof Error ? e.message : "An unexpected error occurred"));
        }
      })
      .finally(() => { if (token === backupTokenRef.current) setLoading(false); });
    return () => controller.abort();
  }, [selectedServer, backupRefreshKey]);

  useEffect(() => {
    if (!selectedServer) return;
    let lastController = new AbortController();
    const interval = setInterval(() => {
      lastController.abort();
      lastController = new AbortController();
      const token = ++backupTokenRef.current;
      fetch(`/api/proxy/servers/${encodeURIComponent(selectedServer)}/backups`, {
        signal: lastController.signal,
        credentials: "include",
      })
        .then((r) => {
          if (!r.ok) throw new Error(`Failed to poll: ${r.status}`);
          return r.json();
        })
        .then((data) => {
          if (token === backupTokenRef.current) {
            setBackups(Array.isArray(data) ? data : data.data ?? []);
            setPollError(false);
          }
        })
        .catch((e) => {
          if (token === backupTokenRef.current) {
            if (e instanceof DOMException && e.name === "AbortError") return;
            setPollError(true);
          }
        });
    }, POLL_INTERVAL_MS);
    return () => { clearInterval(interval); lastController.abort(); };
  }, [selectedServer]);

  async function createBackup() {
    if (!selectedServer) return;
    setActionLoading(true);
    setError(null);
    const token = ++serverTokenRef.current;
    try {
      const res = await fetch(`/api/proxy/servers/${encodeURIComponent(selectedServer)}/backups`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": getCSRFToken() },
        credentials: "include",
        body: JSON.stringify({ format }),
      });
      if (!res.ok) throw new Error(`Failed to create backup: ${res.status}`);
      const newBackup = await res.json();
      if (token === serverTokenRef.current) setBackups((prev) => [newBackup, ...prev]);
    } catch (e) {
      if (token === serverTokenRef.current) setError(sanitizeError(e instanceof Error ? e.message : "Failed to create backup"));
    } finally {
      if (token === serverTokenRef.current) setActionLoading(false);
    }
  }

  function initiateRestore(backupId: string) {
    setRestorePathsInput("");
    setConfirmRestore({ id: backupId, paths: "" });
  }

  async function executeRestore() {
    if (!confirmRestore || !selectedServer) return;
    const rawPaths = restorePathsInput.split(",").map((p) => p.trim()).filter(Boolean);
    const isValidPath = (p: string) => /^[a-zA-Z0-9_\-\/\.]+$/.test(p) && !p.startsWith("/") && !p.includes("..");
    const invalidPaths = rawPaths.filter((p) => !isValidPath(p));
    if (invalidPaths.length > 0) {
      setError(sanitizeError("One or more paths contain invalid characters"));
      setConfirmRestore(null);
      return;
    }
    const paths = rawPaths;
    setActionLoading(true);
    setError(null);
    setConfirmRestore(null);
    const token = ++serverTokenRef.current;
    try {
      const res = await fetch(`/api/proxy/servers/${encodeURIComponent(selectedServer)}/backups/restore`, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": getCSRFToken() },
        credentials: "include",
        body: JSON.stringify({ name: confirmRestore.id, paths: paths.length > 0 ? paths : undefined }),
      });
      if (!res.ok) throw new Error(`Restore failed: ${res.status}`);
      if (token === serverTokenRef.current) setBackupRefreshKey((k) => k + 1);
    } catch (e) {
      if (token === serverTokenRef.current) setError(sanitizeError(e instanceof Error ? e.message : "Restore failed"));
    } finally {
      if (token === serverTokenRef.current) setActionLoading(false);
    }
  }

  async function deleteBackup(backupId: string) {
    if (!selectedServer) return;
    setActionLoading(true);
    setError(null);
    const token = ++serverTokenRef.current;
    try {
      const res = await fetch(`/api/proxy/servers/${encodeURIComponent(selectedServer)}/backups/${encodeURIComponent(backupId)}`, {
        method: "DELETE",
        headers: { "X-CSRF-Token": getCSRFToken() },
        credentials: "include",
      });
      if (!res.ok) throw new Error(`Delete failed: ${res.status}`);
      if (token === serverTokenRef.current) setBackups((prev) => prev.filter((b) => b.uuid !== backupId));
    } catch (e) {
      if (token === serverTokenRef.current) setError(sanitizeError(e instanceof Error ? e.message : "Delete failed"));
    } finally {
      if (token === serverTokenRef.current) setActionLoading(false);
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-4">
        <div className="flex-1">
          <label htmlFor="server-select" className="text-xs font-bold uppercase text-muted">Server</label>
          <select
            id="server-select"
            className="mt-1 block w-full rounded-lg border border-line bg-paper px-3 py-2 text-sm text-ink"
            value={selectedServer}
            onChange={(e) => setSelectedServer(e.target.value)}
          >
            {loading && servers.length === 0 ? (
              <option>Loading servers...</option>
            ) : (
              servers.map((s) => (
                <option key={s.id} value={s.id}>{s.name} ({s.node})</option>
              ))
            )}
          </select>
        </div>
        <div className="w-32">
          <label htmlFor="backup-format" className="text-xs font-bold uppercase text-muted">Format</label>
          <select
            id="backup-format"
            className="mt-1 block w-full rounded-lg border border-line bg-paper px-3 py-2 text-sm text-ink"
            value={format}
            onChange={(e) => setFormat(e.target.value as BackupFormat)}
          >
            <option value="zip">ZIP</option>
            <option value="tar.gz">TAR.GZ</option>
          </select>
        </div>
        <button
          className="mt-4 self-end rounded-lg bg-ink px-5 py-2 text-sm font-bold text-paper hover:bg-surface-hover transition-colors disabled:opacity-60"
          disabled={!selectedServer || actionLoading}
          onClick={createBackup}
        >
          {actionLoading ? "Creating…" : "Create Backup"}
        </button>
      </div>

      <div role="alert">
        {error && (
          <div className="flex items-start gap-2 rounded-xl border border-red-300 bg-red-wash p-6 text-sm text-red-dark">
            <span>{error}</span>
            <button onClick={dismissError} aria-label="Dismiss error" className="ml-auto flex h-10 w-10 items-center justify-center text-xs text-muted hover:text-ink">×</button>
          </div>
        )}
        {pollError && !error && (
          <p className="text-amber-600 text-sm">Unable to refresh. Showing last known data.</p>
        )}
      </div>

      <div className="flex items-center justify-between">
        <h2 className="text-xs font-bold uppercase text-muted">Backups</h2>
        <button
          onClick={() => setBackupRefreshKey((k) => k + 1)}
          className="text-xs text-muted hover:text-ink transition-colors"
          aria-label="Refresh backups"
        >
          ↻ Refresh
        </button>
      </div>

      {confirmRestore && (
        <div role="dialog" aria-label="Confirm restore" className="rounded-xl border border-line bg-paper p-5">
          <p className="text-sm font-bold text-ink mb-3">Restore backup {confirmRestore.id}</p>
          <label htmlFor="restore-paths" className="text-xs text-muted">Optional paths (comma-separated, blank = full restore)</label>
          <input
            id="restore-paths"
            type="text"
            className="mt-1 block w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink"
            placeholder="e.g. world/data, plugins/config.yml"
            value={restorePathsInput}
            onChange={(e) => setRestorePathsInput(e.target.value)}
          />
          <div className="mt-3 flex gap-2">
            <button onClick={executeRestore} className="rounded-lg bg-red-600 hover:bg-red-700 px-4 py-1.5 text-xs font-bold text-white transition-colors">Confirm Restore</button>
            <button onClick={() => setConfirmRestore(null)} aria-label="Cancel restore" className="rounded-lg border border-line px-4 py-1.5 text-xs font-bold text-ink hover:bg-surface transition-colors">Cancel</button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="space-y-3" role="status" aria-label="Loading backups">
          {[1, 2, 3].map(i => (
            <div key={i} className="animate-pulse bg-line/30 rounded-xl p-4 h-20" />
          ))}
        </div>
      ) : backups.length === 0 ? (
        <p className="rounded-xl border border-line bg-paper p-6 text-center text-sm text-muted">
          No backups for this server.
        </p>
      ) : (
        <div className="space-y-3">
          {backups.map((backup) => (
            <div key={backup.uuid} className="flex flex-wrap items-center gap-4 rounded-xl border border-line bg-paper px-5 py-4">
              <div className="flex-1">
                <p className="font-bold text-ink">{backup.name}</p>
                <p className="text-xs text-muted">
                  {backup.format.toUpperCase()} &middot; {formatBytes(backup.size)} &middot;{" "}
                  <span className={backup.status === "completed" ? "text-green-600" : "text-amber-600"}>
                    {backup.status}
                  </span>
                  {backup.isLocked && " \u00b7 Locked"}
                </p>
                <p className="text-xs text-muted">
                  Created: {new Date(backup.createdAt).toLocaleString()}
                  {backup.completedAt && ` \u00b7 Completed: ${new Date(backup.completedAt).toLocaleString()}`}
                </p>
              </div>
              <div className="flex gap-2">
                <button
                  className="rounded-lg border border-line px-3 py-1.5 text-xs font-bold text-ink hover:bg-surface transition-colors disabled:opacity-40"
                  disabled={backup.status !== "completed" || actionLoading}
                  onClick={() => initiateRestore(backup.uuid)}
                  aria-label={`Restore backup ${backup.name}`}
                >
                  Restore
                </button>
                {confirmDelete === backup.uuid ? (
                  <div className="flex items-center gap-1">
                    <span className="text-xs text-red-400">Confirm?</span>
                    <button
                      onClick={() => { deleteBackup(backup.uuid); setConfirmDelete(null); }}
                      className="text-xs px-2 py-1 bg-red-500/20 text-red-300 rounded-lg hover:bg-red-500/30"
                      aria-label="Confirm delete"
                    >
                      Yes
                    </button>
                    <button
                      onClick={() => setConfirmDelete(null)}
                      className="text-xs px-2 py-1 bg-surface text-muted rounded-lg hover:bg-line"
                      aria-label="Cancel delete"
                    >
                      No
                    </button>
                  </div>
                ) : (
                  <button
                    className="rounded-lg border border-red-300 px-3 py-1.5 text-xs font-bold text-red-dark hover:bg-red-wash transition-colors disabled:opacity-40"
                    disabled={backup.status !== "completed" || actionLoading || backup.isLocked}
                    onClick={() => setConfirmDelete(backup.uuid)}
                    aria-label={`Delete backup ${backup.name}`}
                  >
                    Delete
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
