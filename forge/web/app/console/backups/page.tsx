"use client";

import { useQuery } from "@tanstack/react-query";
import { Archive, Download, RefreshCw } from "lucide-react";
import { Card, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { fetchBackupJobs, type BackupJob } from "@/lib/api/admin-backups";

/**
 * /console/backups — customer-facing backups overview.
 */
export default function ConsoleBackupsPage() {
  const { data: backups = [], isLoading } = useQuery({
    queryKey: ["admin-backup-jobs"],
    queryFn: () => fetchBackupJobs(),
    staleTime: 30_000,
    retry: 1,
  });

  if (isLoading) return <LoadingSpinner />;

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Backups"
        sub={`${backups.length} backup${backups.length !== 1 ? "s" : ""} available`}
      />

      {backups.length === 0 ? (
        <Card>
          <div className="flex flex-col items-center gap-3 py-12 text-center">
            <Archive size={32} className="text-slate-600" />
            <p className="text-sm text-slate-400">No backups created yet. Backups are generated per-server or on schedule.</p>
          </div>
        </Card>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-white/[0.06]">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-white/[0.06] bg-white/[0.02]">
              <tr>
                <th className="px-4 py-3 font-semibold text-slate-300">Server</th>
                <th className="px-4 py-3 font-semibold text-slate-300">Created</th>
                <th className="px-4 py-3 font-semibold text-slate-300">Size</th>
                <th className="px-4 py-3 font-semibold text-slate-300">Status</th>
                <th className="px-4 py-3 font-semibold text-slate-300">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {backups.slice(0, 50).map((backup) => (
                <tr key={backup.id} className="transition-colors hover:bg-white/[0.02]">
                  <td className="px-4 py-3 font-medium text-white">{backup.name}</td>
                  <td className="px-4 py-3 text-slate-400">{backup.createdAt ? new Date(backup.createdAt).toLocaleDateString() : "—"}</td>
                  <td className="px-4 py-3 text-slate-400">{backup.totalBytes ? `${(backup.totalBytes / 1024 / 1024).toFixed(1)} MB` : "—"}</td>
                  <td className="px-4 py-3">
                    <Pill tone={backup.status === "completed" ? "success" : backup.status === "running" ? "warning" : backup.status === "failed" ? "danger" : "neutral"}>
                      {backup.status}
                    </Pill>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <button type="button" className="rounded p-1 text-slate-400 hover:text-white" title="Download"><Download size={14} /></button>
                      <button type="button" className="rounded p-1 text-slate-400 hover:text-white" title="Restore"><RefreshCw size={14} /></button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
