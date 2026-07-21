"use client";

import { useState } from "react";
import { Archive, Download, Lock, RotateCcw, Trash2, Unlock } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createBackup, deleteBackup, fetchBackups, lockBackup as lockBackupApi, restoreBackup, unlockBackup as unlockBackupApi, getBackupDownloadURL } from "@/lib/api/servers";
import { type ApiBackup, type ApiServer } from "@/lib/api/types";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { formatDate } from "@/lib/utils";
import { fetchBackupProviders } from "@/lib/api/database-containers";

export function formatBackupBytes(value: number) {
  if (!Number.isFinite(value) || value < 0) return "Unknown size";
  if (value < 1024) return `${value} Bytes`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(value > 100000 ? 0 : 2)} kB`;
  return `${(value / 1024 / 1024).toFixed(2)} MB`;
}

function isUsable(backup: ApiBackup) {
  return backup.status === "completed";
}

function errorText(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function BackupsView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canCreate = hasServerPermission(access, "backup.create");
  const canDownload = hasServerPermission(access, "backup.download");
  const canRestore = hasServerPermission(access, "backup.restore");
  const canDelete = hasServerPermission(access, "backup.delete");
  const queryClient = useQueryClient();
  const [backupName, setBackupName] = useState("");
  const [ignoredFiles, setIgnoredFiles] = useState("");
  const [lockOnCreate, setLockOnCreate] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [storageProvider, setStorageProvider] = useState("");
  const [storageConfig, setStorageConfig] = useState<Record<string, string>>({});
  const providers = useQuery({
    queryKey: ["backup-providers"],
    queryFn: fetchBackupProviders,
		enabled: showAdvanced,
    staleTime: 60000,
  });
  const [currentPage, setCurrentPage] = useState(1);
  const backups = useQuery({
    queryKey: ["server-backups", server?.id, currentPage],
    queryFn: () => fetchBackups(server?.id ?? "", currentPage, 20),
    enabled: Boolean(server?.id),
    refetchInterval: (query) => (query.state.data as { data: ApiBackup[] } | undefined)?.data?.some((backup) => backup.status === "pending" || backup.status === "running") ? 3000 : false,
  });
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["server-backups", server?.id] });
  const createMutation = useMutation({ 
    mutationFn: () => createBackup(server?.id ?? "", {
      name: backupName || undefined,
      ignored: ignoredFiles ? ignoredFiles.split(",").map(f => f.trim()).filter(f => f) : undefined,
      is_locked: lockOnCreate || undefined
    }), 
    onSuccess: () => {
      invalidate();
      setBackupName("");
      setIgnoredFiles("");
      setLockOnCreate(false);
      setShowAdvanced(false);
      setCurrentPage(1);
    }
  });
  const restoreMutation = useMutation({ mutationFn: (backup: ApiBackup) => restoreBackup(server?.id ?? "", backup.name, false), onSuccess: invalidate });
  const deleteMutation = useMutation({ mutationFn: (backup: ApiBackup) => deleteBackup(server?.id ?? "", backup.name), onSuccess: invalidate });
  const lockBackupMutation = useMutation({ mutationFn: (backup: ApiBackup) => lockBackupApi(server?.id ?? "", backup.name), onSuccess: invalidate });
  const unlockBackupMutation = useMutation({ mutationFn: (backup: ApiBackup) => unlockBackupApi(server?.id ?? "", backup.name), onSuccess: invalidate });
  const list = backups.data?.data ?? [];
  const pagination = backups.data?.pagination;
  const limit = server?.backupLimit;
  const backupsDisabled = limit === 0;
  const limitReached = typeof limit === "number" && limit > 0 && (pagination?.total ?? 0) >= limit;
  const actionError = createMutation.error ?? restoreMutation.error ?? deleteMutation.error ?? lockBackupMutation.error ?? unlockBackupMutation.error;

  const download = async (backup: ApiBackup) => {
    if (!server?.id || !isUsable(backup)) return;
    const { url } = await getBackupDownloadURL(server.id, backup.name);
    window.location.href = url;
  };

  return (
    <div className="space-y-6">
      <div className="rounded-xl border border-white/[0.08] bg-[#1e2536] px-4 py-4 text-sm font-semibold text-[#94a3b8]">
        {backupsDisabled ? "Backups are disabled for this server." : typeof limit === "number" && limit > 0 ? `${pagination?.total ?? 0} of ${limit} backup slots used.` : `${pagination?.total ?? 0} backups created; no quota was provided by the API.`}
        {limitReached ? <span className="ml-2 text-red-300">Limit reached.</span> : null}
        {restoreMutation.isPending ? <span className="ml-2 text-amber-300">Restoring backup…</span> : null}
      </div>
      {backups.isError ? <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">{errorText(backups.error, "Backups could not be loaded.")}</div> : null}
      {actionError ? <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200" role="alert">{errorText(actionError, "Backup action failed.")}</div> : null}
      <div className="space-y-3">
        {backups.isLoading ? <div className="rounded-xl bg-[#1e2536] px-4 py-5 text-sm font-semibold text-[#94a3b8]">Loading backups…</div> : null}
        {!backups.isLoading && !backups.isError && list.length === 0 ? <div className="rounded-xl bg-[#1e2536] px-4 py-5 text-sm font-semibold text-[#94a3b8]">No backups have been created for this server yet.</div> : null}
        {list.map((backup) => {
          const usable = isUsable(backup);
          const busy = restoreMutation.isPending || deleteMutation.isPending || lockBackupMutation.isPending || unlockBackupMutation.isPending;
          return (
            <div className="grid gap-4 rounded-xl bg-[#1e2536] px-4 py-5 text-[#94a3b8] sm:grid-cols-[36px_1fr_220px_160px] sm:items-center" key={backup.uuid ?? backup.name}>
              <Archive size={20} />
              <div>
                <p className="text-base font-semibold text-slate-100">{backup.name}</p>
                <p className="mt-1 text-xs"><span className="uppercase">{backup.status || "unknown"}</span> · {formatBackupBytes(backup.size ?? 0)}</p>
                <p className="mt-1 break-all font-mono text-xs text-[#64748b]">{backup.checksum ? `Checksum: ${backup.checksum}` : "Checksum not available"}</p>
              </div>
              <div className="text-xs sm:text-right">
                <p className="font-semibold text-slate-100">{formatDate(backup.createdAt, "Not completed")}</p><p className="uppercase text-[#64748b]">Created</p>
                <p className="mt-1 font-semibold text-slate-300">{formatDate(backup.completedAt, "Not completed")}</p><p className="uppercase text-[#64748b]">Completed</p>
              </div>
              <div className="flex items-center gap-1 sm:justify-self-end">
                <button aria-label={`Download ${backup.name}`} className="grid h-9 w-9 place-items-center rounded hover:bg-[#4b5563] disabled:opacity-40" disabled={!usable || !canDownload} onClick={() => void download(backup).catch(() => undefined)} title={usable ? "Download" : "Available after completion"} type="button"><Download size={18} /></button>
                {backup.isLocked ? (
                  <button
                    aria-label={`Unlock ${backup.name}`}
                    className="grid h-9 w-9 place-items-center rounded text-amber-200 hover:bg-[#4b5563] disabled:opacity-40"
                    disabled={busy || !canDelete}
                    onClick={() => unlockBackupMutation.mutate(backup)}
                    title="Unlock backup"
                    type="button"
                  >
                    <Unlock size={18} />
                  </button>
                ) : (
                  <button
                    aria-label={`Lock ${backup.name}`}
                    className="grid h-9 w-9 place-items-center rounded hover:bg-[#4b5563] disabled:opacity-40"
                    disabled={busy || !canDelete}
                    onClick={() => lockBackupMutation.mutate(backup)}
                    title="Lock backup (prevents deletion)"
                    type="button"
                  >
                    <Lock size={18} />
                  </button>
                )}
                <button aria-label={`Restore ${backup.name}`} className="grid h-9 w-9 place-items-center rounded text-amber-200 hover:bg-[#4b5563] disabled:opacity-40" disabled={!usable || busy || !canRestore} onClick={() => { if (window.confirm(`Restore ${backup.name}? Current files may be overwritten.`)) restoreMutation.mutate(backup); }} title={usable ? "Restore" : "Available after completion"} type="button"><RotateCcw size={18} /></button>
                <button aria-label={`Delete ${backup.name}`} className="grid h-9 w-9 place-items-center rounded text-red-200 hover:bg-[#4b5563] disabled:opacity-40" disabled={!usable || busy || !canDelete || backup.isLocked} onClick={() => { if (window.confirm(`Delete backup ${backup.name}?`)) deleteMutation.mutate(backup); }} title={usable ? (backup.isLocked ? "Backup is locked" : "Delete") : "Available after completion"} type="button"><Trash2 size={18} /></button>
              </div>
            </div>
          );
        })}
      </div>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm font-semibold text-[#64748b]">Only completed backups can be downloaded, restored, or deleted. Locked backups cannot be deleted until unlocked.</p>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <button 
            className="text-sm font-semibold text-[#64748b] hover:text-slate-100" 
            onClick={() => setShowAdvanced(!showAdvanced)}
            type="button"
          >
            {showAdvanced ? "Hide advanced options" : "Show advanced options"}
          </button>
          <button className="rounded-xl bg-[#dc2626] px-5 py-4 text-sm font-bold uppercase text-white hover:bg-[#b91c1c] disabled:opacity-60" disabled={!canCreate || backupsDisabled || createMutation.isPending || limitReached || !server?.id} onClick={() => createMutation.mutate()} type="button">{createMutation.isPending ? "Creating…" : backupsDisabled ? "Backups disabled" : "Create Backup"}</button>
        </div>
      </div>
      {pagination && pagination.total_pages > 1 && (
        <div className="flex items-center justify-between rounded-xl bg-[#1e2536] px-4 py-3">
          <p className="text-sm font-semibold text-[#64748b]">Page {pagination.page} of {pagination.total_pages} ({pagination.total} total)</p>
          <div className="flex gap-2">
            <button 
              className="rounded-lg bg-[#0f141f] px-3 py-2 text-sm font-semibold text-slate-100 hover:bg-[#1e2536] disabled:opacity-40"
              disabled={pagination.page <= 1}
              onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
              type="button"
            >
              Previous
            </button>
            <button 
              className="rounded-lg bg-[#0f141f] px-3 py-2 text-sm font-semibold text-slate-100 hover:bg-[#1e2536] disabled:opacity-40"
              disabled={pagination.page >= pagination.total_pages}
              onClick={() => setCurrentPage(p => Math.min(pagination.total_pages, p + 1))}
              type="button"
            >
              Next
            </button>
          </div>
        </div>
      )}
      {showAdvanced && (
        <div className="rounded-xl bg-[#1e2536] px-4 py-4 space-y-3">
          <div>
            <label className="block text-sm font-semibold text-slate-100 mb-1">Backup Name (optional)</label>
            <input 
              className="w-full rounded-lg bg-[#0f141f] border border-white/[0.1] px-3 py-2 text-sm text-slate-100 focus:outline-none focus:border-blue-500"
              placeholder="Leave empty for auto-generated name"
              value={backupName}
              onChange={(e) => setBackupName(e.target.value)}
              type="text"
            />
          </div>
          <div>
            <label className="block text-sm font-semibold text-slate-100 mb-1">Ignored Files (comma-separated patterns)</label>
            <input 
              className="w-full rounded-lg bg-[#0f141f] border border-white/[0.1] px-3 py-2 text-sm text-slate-100 focus:outline-none focus:border-blue-500"
              placeholder="e.g., node_modules, .git, *.log"
              value={ignoredFiles}
              onChange={(e) => setIgnoredFiles(e.target.value)}
              type="text"
            />
            <p className="text-xs text-[#64748b] mt-1">Use .gitignore-style patterns to exclude files from backup</p>
          </div>
          <div>
            <label className="block text-sm font-semibold text-slate-100 mb-1">Storage Destination</label>
            <select
              className="w-full rounded-lg bg-[#0f141f] border border-white/[0.1] px-3 py-2 text-sm text-slate-100 focus:outline-none focus:border-blue-500"
              value={storageProvider}
              onChange={(e) => { setStorageProvider(e.target.value); setStorageConfig({}); }}
            >
              <option value="">Default</option>
              {(providers.data?.providers ?? []).map((p) => (
                <option key={p} value={p}>{p.toUpperCase()}</option>
              ))}
            </select>
          </div>
          {storageProvider === "s3" && (
            <div className="space-y-3 rounded-lg bg-[#0f141f] p-3">
              <p className="text-xs font-semibold text-slate-400 uppercase">S3 Configuration</p>
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Bucket" value={storageConfig.bucket ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, bucket: e.target.value }))} />
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Region (e.g. us-east-1)" value={storageConfig.region ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, region: e.target.value }))} />
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Endpoint (optional)" value={storageConfig.endpoint ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, endpoint: e.target.value }))} />
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Access Key ID" value={storageConfig.accessKeyId ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, accessKeyId: e.target.value }))} />
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Secret Access Key" type="password" value={storageConfig.secretAccessKey ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, secretAccessKey: e.target.value }))} />
            </div>
          )}
          {storageProvider === "gcs" && (
            <div className="space-y-3 rounded-lg bg-[#0f141f] p-3">
              <p className="text-xs font-semibold text-slate-400 uppercase">GCS Configuration</p>
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Bucket Name" value={storageConfig.bucketName ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, bucketName: e.target.value }))} />
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Key File Path (optional)" value={storageConfig.keyFile ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, keyFile: e.target.value }))} />
            </div>
          )}
          {storageProvider === "azure" && (
            <div className="space-y-3 rounded-lg bg-[#0f141f] p-3">
              <p className="text-xs font-semibold text-slate-400 uppercase">Azure Configuration</p>
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Container Name" value={storageConfig.containerName ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, containerName: e.target.value }))} />
              <input className="w-full rounded-lg bg-[#161b28] border border-white/[0.1] px-3 py-2 text-sm text-slate-100" placeholder="Connection String" value={storageConfig.connectionString ?? ""} onChange={(e) => setStorageConfig(c => ({ ...c, connectionString: e.target.value }))} />
            </div>
          )}
          {providers.isLoading && <p className="text-xs text-slate-500">Loading providers...</p>}
          {providers.isError && <p className="text-xs text-red-400">Failed to load providers</p>}
          <div className="flex items-center gap-2">
            <input 
              checked={lockOnCreate}
              onChange={(e) => setLockOnCreate(e.target.checked)}
              type="checkbox"
              id="lock-backup"
            />
            <label htmlFor="lock-backup" className="text-sm font-semibold text-slate-100">Lock backup on creation</label>
          </div>
        </div>
      )}
    </div>
  );
}
