"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { HardDrive } from "lucide-react";
import {
  EmptyBackups,
  ErrorAlert,
  SpinnerInline,
  useAppToast,
  SpinnerButton,
} from "@/components/shared";
import { fetchAppBackups, restoreAppBackup, deleteAppBackup } from "@/lib/api/apps";
import type { AppBackup } from "@/lib/api/apps";
import { errorMessage, formatDate, formatBytes } from "@/lib/utils";
import type { ReactNode } from "react";

interface BackupsViewProps {
  appId: string;
  action?: ReactNode;
}

export function BackupsView({ appId, action }: BackupsViewProps) {
  const qc = useQueryClient();
  const { success: showSuccess, error: showError } = useAppToast();

  const query = useQuery<AppBackup[]>({
    queryKey: ["app-backups", appId],
    queryFn: () => fetchAppBackups(appId),
    enabled: Boolean(appId),
  });

  const restoreMut = useMutation({
    mutationFn: async (backupId: string) => {
      await restoreAppBackup(appId, backupId);
    },
    onSuccess: () => {
      showSuccess("Backup", "restored");
      void qc.invalidateQueries({ queryKey: ["app-backups", appId] });
    },
    onError: (error) => showError("Backup", errorMessage(error, "Failed to restore backup")),
  });

  const deleteMut = useMutation({
    mutationFn: async (backupId: string) => {
      await deleteAppBackup(appId, backupId);
    },
    onSuccess: () => {
      showSuccess("Backup", "deleted");
      void qc.invalidateQueries({ queryKey: ["app-backups", appId] });
    },
    onError: (error) => showError("Backup", errorMessage(error, "Failed to delete backup")),
  });

  if (query.isLoading) return <SpinnerInline label="Loading backups…" />;

  if (query.isError) {
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load backups"
        onRetry={() => void query.refetch()}
      />
    );
  }

  const backups = query.data ?? [];

  if (!backups.length) {
    return <EmptyBackups action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {backups.length} backup{backups.length === 1 ? "" : "s"}
        </span>
        {action ? <div className="ml-auto">{action}</div> : null}
      </div>
      <div className="divide-y divide-white/[0.07]">
        {backups.map((backup) => (
          <div className="flex flex-wrap items-center gap-4 px-5 py-4" key={backup.id}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <HardDrive size={16} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <p className="truncate font-mono text-sm font-semibold text-slate-200">
                  {backup.name}
                </p>
                {backup.status === "creating" || backup.status === "restoring" ? (
                  <SpinnerButton label="In progress" className="text-amber-400" />
                ) : null}
              </div>
              <p className="text-xs text-slate-500">
                {backup.size != null ? formatBytes(backup.size) : "Unknown size"} ·{" "}
                {formatDate(backup.createdAt)}
              </p>
            </div>
            <div className="flex gap-2">
              <button
                className="rounded border border-white/10 px-3 py-1.5 text-xs font-semibold text-slate-300 hover:bg-white/[0.06] disabled:opacity-50"
                disabled={restoreMut.isPending || backup.status === "creating" || backup.status === "restoring"}
                onClick={() => {
                  if (window.confirm(`Restore backup ${backup.name}? This will overwrite current data.`)) {
                    restoreMut.mutate(backup.id);
                  }
                }}
                type="button"
              >
                {restoreMut.isPending ? "Restoring…" : "Restore"}
              </button>
              <button
                className="rounded border border-red-500/30 px-3 py-1.5 text-xs font-semibold text-red-300 hover:bg-red-500/10 disabled:opacity-50"
                disabled={deleteMut.isPending}
                onClick={() => {
                  if (window.confirm(`Delete backup ${backup.name}?`)) {
                    deleteMut.mutate(backup.id);
                  }
                }}
                type="button"
              >
                {deleteMut.isPending ? "Deleting…" : "Delete"}
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
