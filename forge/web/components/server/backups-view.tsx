"use client";

import { useState } from "react";
import { Archive, Download, Lock, Pencil, RotateCcw, ShieldCheck, Sparkles, Trash2, Unlock } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createBackup, deleteBackup, fetchBackups, lockBackup as lockBackupApi, restoreBackup, unlockBackup as unlockBackupApi, getBackupDownloadURL } from "@/lib/api/servers";
import { cleanupServerBackups, renameServerBackup, verifyServerBackup, type BackupVerifyResult } from "@/lib/api/backup";
import { type ApiBackup, type ApiServer } from "@/lib/api/types";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { formatDate } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";

export function formatBackupBytes(value: number | null | undefined, opts?: { completed?: boolean }) {
  // The API omits size until the upload lands. Rendering that absence as
  // "0 Bytes" claims an empty backup the daemon never measured.
  if (value === null || value === undefined) return "Not reported";
  if (!Number.isFinite(value) || (value as number) < 0) return "Unknown size";
  if ((value as number) === 0) return opts?.completed ? "0 Bytes" : "Not reported";
  const n = value as number;
  if (n < 1024) return `${n} Bytes`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n > 100000 ? 0 : 2)} kB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

function isUsable(backup: ApiBackup) {
  return backup.status === "completed";
}

function errorText(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function BackupsView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canCreate = hasServerPermission(access, "backup.create");
  const canDownload = hasServerPermission(access, "backup.download");
  const canRestore = hasServerPermission(access, "backup.restore");
  const canDelete = hasServerPermission(access, "backup.delete");
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [ignoredFiles, setIgnoredFiles] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const [confirm, renderConfirm] = useConfirm();
  const backups = useQuery({
    queryKey: ["server-backups", server?.id, currentPage],
    queryFn: () => fetchBackups(server?.id ?? "", currentPage, 20),
    enabled: Boolean(server?.id),
    refetchInterval: (query) => (query.state.data as { data: ApiBackup[] } | undefined)?.data?.some((backup) => backup.status === "pending" || backup.status === "running") ? 3000 : false,
  });
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["server-backups", server?.id] });
  const createMutation = useMutation({ 
    mutationFn: () => createBackup(server?.id ?? "", {
      ignored: ignoredFiles ? ignoredFiles.split(",").map(f => f.trim()).filter(f => f) : undefined,
    }), 
    onSuccess: () => {
      invalidate();
      setIgnoredFiles("");
      setShowAdvanced(false);
      setCurrentPage(1);
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create backup", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const restoreMutation = useMutation({ mutationFn: async (backup: ApiBackup) => { const result = await restoreBackup(server?.id ?? "", backup.name, false); if (!result.ok) throw new Error("The server reported the restore did not complete."); return result; }, onSuccess: invalidate, onError: (err) => toast({ tone: "error", title: "Failed to restore backup", message: err instanceof Error ? err.message : "An error occurred" }) });
  const deleteMutation = useMutation({ mutationFn: (backup: ApiBackup) => deleteBackup(server?.id ?? "", backup.name), onSuccess: invalidate, onError: (err) => toast({ tone: "error", title: "Failed to delete backup", message: err instanceof Error ? err.message : "An error occurred" }) });
  const canVerify = hasServerPermission(access, "backup.read");
  const [verifyResults, setVerifyResults] = useState<Record<string, BackupVerifyResult>>({});
  const [verifying, setVerifying] = useState<string | null>(null);
  const verify = async (backup: ApiBackup) => {
    if (!server?.id || !isUsable(backup) || verifying) return;
    setVerifying(backup.name);
    try {
      const result = await verifyServerBackup(server.id, backup.name);
      setVerifyResults((current) => ({ ...current, [backup.name]: result }));
      toast({ tone: result.verified ? "success" : "warning", title: result.verified ? `Backup ${backup.name} verified` : `Backup ${backup.name} failed verification`, message: result.verified ? `Checksum match: ${result.checksumMatch ? "yes" : "panel copy only"}.` : "The node copy is missing, incomplete, or its checksum differs from the panel record." });
    } catch (error) {
      toast({ tone: "error", title: "Verification failed", message: error instanceof Error ? error.message : "The backup could not be verified." });
    } finally {
      setVerifying(null);
    }
  };
  const renameMutation = useMutation({
    mutationFn: ({ backup, name }: { backup: ApiBackup; name: string }) => renameServerBackup(server?.id ?? "", backup.name, name),
    onSuccess: (result) => { setVerifyResults((current) => { const next = { ...current }; delete next[result.previousName]; return next; }); invalidate(); },
    onError: (err) => toast({ tone: "error", title: "Failed to rename backup", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const rename = (backup: ApiBackup) => {
    if (!isUsable(backup) || backup.isLocked) return;
    const name = window.prompt("New backup name", backup.name)?.trim();
    if (!name || name === backup.name) return;
    renameMutation.mutate({ backup, name });
  };
  const cleanupMutation = useMutation({
    mutationFn: () => cleanupServerBackups(server?.id ?? ""),
    onSuccess: (result) => { invalidate(); toast({ tone: "success", title: "Backup cleanup complete", message: `${result.deleted} old backup${result.deleted === 1 ? "" : "s"} removed per the retention policy.` }); },
    onError: (err) => toast({ tone: "error", title: "Cleanup failed", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const lockBackupMutation = useMutation({ mutationFn: (backup: ApiBackup) => lockBackupApi(server?.id ?? "", backup.name), onSuccess: invalidate, onError: (err) => toast({ tone: "error", title: "Failed to lock backup", message: err instanceof Error ? err.message : "An error occurred" }) });
  const unlockBackupMutation = useMutation({ mutationFn: (backup: ApiBackup) => unlockBackupApi(server?.id ?? "", backup.name), onSuccess: invalidate, onError: (err) => toast({ tone: "error", title: "Failed to unlock backup", message: err instanceof Error ? err.message : "An error occurred" }) });
  const list = backups.data?.data ?? [];
  const pagination = backups.data?.pagination;
  const limit = server?.backupLimit;
  const backupsDisabled = limit === 0;
  const limitReached = typeof limit === "number" && limit > 0 && (pagination?.total ?? 0) >= limit;
  const actionError = createMutation.error ?? restoreMutation.error ?? deleteMutation.error ?? lockBackupMutation.error ?? unlockBackupMutation.error ?? renameMutation.error ?? cleanupMutation.error;

  const download = async (backup: ApiBackup) => {
    if (!server?.id || !isUsable(backup)) return;
    try {
      const { url } = await getBackupDownloadURL(server.id, backup.name);
      window.location.href = url;
    } catch (error) {
      toast({ tone: "error", title: "Download failed", message: error instanceof Error ? error.message : "Could not generate a download link for this backup." });
    }
  };

  return (
    <div className="space-y-6">
      {renderConfirm()}
      <div className="rounded-xl border border-line bg-[var(--surface-raised)] px-4 py-4 text-sm font-semibold text-[var(--text-subtle)]">
        {backupsDisabled ? "Backups are disabled for this server." : typeof limit === "number" && limit > 0 ? `${pagination?.total ?? 0} of ${limit} backup slots used.` : `${pagination?.total ?? 0} backups created; no quota was provided by the API.`}
        {limitReached ? <span className="ml-2 text-danger">Limit reached.</span> : null}
        {restoreMutation.isPending ? <span className="ml-2 text-warn">Restoring backup…</span> : null}
      </div>
      {backups.isError ? <div className="rounded-xl border border-danger-line bg-danger-subtle p-4 text-sm text-danger">{errorText(backups.error, "Backups could not be loaded.")}</div> : null}
      {actionError ? <div className="rounded-xl border border-danger-line bg-danger-subtle p-4 text-sm text-danger" role="alert">{errorText(actionError, "Backup action failed.")}</div> : null}
      <div className="space-y-3">
        {backups.isLoading ? <div className="rounded-xl bg-[var(--surface-raised)] px-4 py-5 text-sm font-semibold text-[var(--text-subtle)]">Loading backups…</div> : null}
        {!backups.isLoading && !backups.isError && list.length === 0 ? <div className="rounded-xl bg-[var(--surface-raised)] px-4 py-5 text-sm font-semibold text-[var(--text-subtle)]">No backups have been created for this server yet.</div> : null}
        {list.map((backup) => {
          const usable = isUsable(backup);
          const busy = restoreMutation.isPending || deleteMutation.isPending || lockBackupMutation.isPending || unlockBackupMutation.isPending || renameMutation.isPending || verifying === backup.name;
          const verification = verifyResults[backup.name];
          return (
            <div className="grid gap-4 rounded-xl bg-[var(--surface-raised)] px-4 py-5 text-[var(--text-subtle)] sm:grid-cols-[36px_1fr_220px_160px] sm:items-center" key={backup.uuid ?? backup.name}>
              <Archive size={20} />
              <div>
                <p className="text-base font-semibold text-text">{backup.name}</p>
                <p className="mt-1 text-xs"><span className="uppercase">{backup.status || "unknown"}</span> · {formatBackupBytes(backup.size, { completed: usable })}</p>
                <p className="mt-1 break-all font-mono text-xs text-[var(--text-muted)]">{backup.checksum ? `Checksum: ${backup.checksum}` : "Checksum not available"}</p>
                {verification ? <p className="mt-1 text-xs font-semibold text-[var(--text-muted)]">{verification.verified ? `Verified against node copy · checksum match: ${verification.checksumMatch ? "yes" : "panel copy only"}` : "Node copy missing or checksum mismatch"}</p> : null}
              </div>
              <div className="text-xs sm:text-right">
                <p className="font-semibold text-text">{formatDate(backup.createdAt, "Not completed")}</p><p className="uppercase text-[var(--text-muted)]">Created</p>
                <p className="mt-1 font-semibold text-text">{formatDate(backup.completedAt, "Not completed")}</p><p className="uppercase text-[var(--text-muted)]">Completed</p>
              </div>
              <div className="flex items-center gap-1 sm:justify-self-end">
                <button aria-label={`Download ${backup.name}`} className="grid h-9 w-9 place-items-center rounded hover:bg-[var(--surface-raised)] disabled:opacity-40" disabled={!usable || !canDownload} onClick={() => void download(backup)} title={usable ? "Download" : "Available after completion"} type="button"><Download size={18} /></button>
                {backup.isLocked ? (
                  <button
                    aria-label={`Unlock ${backup.name}`}
                    className="grid h-9 w-9 place-items-center rounded text-warn hover:bg-[var(--surface-raised)] disabled:opacity-40"
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
                    className="grid h-9 w-9 place-items-center rounded hover:bg-[var(--surface-raised)] disabled:opacity-40"
                    disabled={busy || !canDelete}
                    onClick={() => lockBackupMutation.mutate(backup)}
                    title="Lock backup (prevents deletion)"
                    type="button"
                  >
                    <Lock size={18} />
                  </button>
                )}
                <button aria-label={`Restore ${backup.name}`} className="grid h-9 w-9 place-items-center rounded text-warn hover:bg-[var(--surface-raised)] disabled:opacity-40" disabled={!usable || busy || !canRestore} onClick={async () => { if (await confirm({ title: `Restore ${backup.name}?`, description: "Current server files may be overwritten. This cannot be undone.", confirmLabel: "Restore" })) restoreMutation.mutate(backup); }} title={usable ? "Restore" : "Available after completion"} type="button"><RotateCcw size={18} /></button>
                <button aria-label={`Verify ${backup.name}`} className="grid h-9 w-9 place-items-center rounded hover:bg-[var(--surface-raised)] disabled:opacity-40" disabled={!usable || busy || !canVerify} onClick={() => void verify(backup)} title={usable ? "Verify against the node copy" : "Available after completion"} type="button"><ShieldCheck size={18} /></button>
                <button aria-label={`Rename ${backup.name}`} className="grid h-9 w-9 place-items-center rounded hover:bg-[var(--surface-raised)] disabled:opacity-40" disabled={!usable || busy || !canDelete || backup.isLocked} onClick={() => rename(backup)} title={usable ? (backup.isLocked ? "Unlock before renaming" : "Rename") : "Available after completion"} type="button"><Pencil size={18} /></button>
                <button aria-label={`Delete ${backup.name}`} className="grid h-9 w-9 place-items-center rounded text-danger hover:bg-[var(--surface-raised)] disabled:opacity-40" disabled={!usable || busy || !canDelete || backup.isLocked} onClick={async () => { if (await confirm({ title: `Delete backup ${backup.name}?`, description: "The backup will be permanently removed.", danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(backup); }} title={usable ? (backup.isLocked ? "Backup is locked" : "Delete") : "Available after completion"} type="button"><Trash2 size={18} /></button>
              </div>
            </div>
          );
        })}
      </div>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm font-semibold text-[var(--text-muted)]">Only completed backups can be downloaded, restored, or deleted. Locked backups cannot be deleted until unlocked.</p>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <button 
            className="text-sm font-semibold text-[var(--text-muted)] hover:text-text" 
            onClick={() => setShowAdvanced(!showAdvanced)}
            type="button"
          >
            {showAdvanced ? "Hide advanced options" : "Show advanced options"}
          </button>
          <button className="rounded-xl bg-[var(--brand)] px-5 py-4 text-sm font-bold uppercase text-white hover:bg-[var(--brand-dark)] disabled:opacity-60" disabled={!canCreate || backupsDisabled || createMutation.isPending || limitReached || !server?.id} onClick={() => createMutation.mutate()} type="button">{createMutation.isPending ? "Creating…" : backupsDisabled ? "Backups disabled" : "Create Backup"}</button>
          <button className="ui-button ui-button-secondary" disabled={!canDelete || cleanupMutation.isPending || !server?.id} onClick={async () => { if (await confirm({ title: "Clean up old backups?", description: "Backups beyond the retention policy and server limit will be permanently removed.", confirmLabel: "Clean up" })) cleanupMutation.mutate(); }} title="Remove backups beyond retention" type="button"><Sparkles size={15} />{cleanupMutation.isPending ? "Cleaning…" : "Clean up"}</button>
        </div>
      </div>
      {pagination && pagination.total_pages > 1 && (
        <div className="flex items-center justify-between rounded-xl bg-[var(--surface-raised)] px-4 py-3">
          <p className="text-sm font-semibold text-[var(--text-muted)]">Page {pagination.page} of {pagination.total_pages} ({pagination.total} total)</p>
          <div className="flex gap-2">
            <button 
              className="rounded-lg bg-[var(--surface)] px-3 py-2 text-sm font-semibold text-text hover:bg-[var(--surface-raised)] disabled:opacity-40"
              disabled={pagination.page <= 1}
              onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
              type="button"
            >
              Previous
            </button>
            <button 
              className="rounded-lg bg-[var(--surface)] px-3 py-2 text-sm font-semibold text-text hover:bg-[var(--surface-raised)] disabled:opacity-40"
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
        <div className="rounded-xl bg-[var(--surface-raised)] px-4 py-4 space-y-3">
          <div>
            <label className="block text-sm font-semibold text-text mb-1">Ignored Files (comma-separated patterns)</label>
            <input 
              className="w-full rounded-lg bg-[var(--surface)] border border-line px-3 py-2 text-sm text-text focus:outline-none focus:border-info"
              placeholder="e.g., node_modules, .git, *.log"
              value={ignoredFiles}
              onChange={(e) => setIgnoredFiles(e.target.value)}
              type="text"
            />
            <p className="text-xs text-[var(--text-muted)] mt-1">Use .gitignore-style patterns to exclude files from backup. The backup name is generated server-side.</p>
          </div>
          <div className="rounded-lg border border-line bg-[var(--surface)] px-3 py-3">
            <p className="text-sm font-semibold text-text mb-1">Storage Destination</p>
            <p className="text-xs text-[var(--text-muted)]">Custom storage destinations (S3, GCS, Azure) are not supported yet. Backups currently use the default node-local storage.</p>
          </div>
          <p className="text-xs text-[var(--text-muted)]">To prevent deletion, lock the backup after creation using the lock action.</p>
        </div>
      )}
    </div>
  );
}
