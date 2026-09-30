"use client";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { queryKeys } from "@/lib/api/query-keys";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useRouter } from "next/navigation";
import {
  FileText, Layers, Plus,
  Power, RefreshCw, RotateCcw, Square,
  Terminal, Trash2,
} from "lucide-react";
import { fetchApps, startApp, stopApp, restartApp, deleteApp, typeLabel, type ApiApp, type AppType } from "@/lib/api/apps";
import { AdminPageLayout, AdminErrorState, AdminLoadingState, Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { DeployStatusBadge } from "@/components/admin/AdminAppsShared";
import { APP_TYPE_ICONS } from "@/lib/app-type-icons";

export default function AdminAppsPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { toast } = useToast();
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<string>("");
  const [deleteTarget, setDeleteTarget] = useState<ApiApp | null>(null);

  const { data: apps = [], isLoading, isError, refetch } = useQuery({
    queryKey: queryKeys.apps.lists(),
    queryFn: fetchApps,
    refetchInterval: 15_000,
  });

  const startMut = useMutation({
    mutationFn: startApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() }),
    onError: (err) => toast({ tone: "error", title: "Failed to start app", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const stopMut = useMutation({
    mutationFn: stopApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() }),
    onError: (err) => toast({ tone: "error", title: "Failed to stop app", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const restartMut = useMutation({
    mutationFn: restartApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() }),
    onError: (err) => toast({ tone: "error", title: "Failed to restart app", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const deleteMut = useMutation({
    mutationFn: deleteApp,
    onSuccess: () => {
      setDeleteTarget(null);
      void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() });
      toast({ tone: "success", title: "Application deleted" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to delete app", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const types: AppType[] = ["image", "git", "compose", "game_server"];

  const filtered = useMemo(() => {
    if (!apps) return [];
    return apps.filter((app) => {
      if (search && !app.name.toLowerCase().includes(search.toLowerCase())) return false;
      if (typeFilter && app.type !== typeFilter) return false;
      return true;
    });
  }, [apps, search, typeFilter]);

  return (
    <AdminPageLayout>
      <SectionHeader
        info={adminPageGuides.applications}
        title="Applications"
        sub="Manage applications built from container images, Git repositories, Compose stacks, and game-server definitions."
        action={
          <Btn tone="primary" onClick={() => router.push("/admin/apps/new")}>
            <Plus size={14} /> Create App
          </Btn>
        }
      />
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-line bg-overlay-subtle px-4 py-3 text-xs text-text-subtle">
        <span>Ready to release an application?</span>
        <button type="button" onClick={() => router.push("/admin/deployments")} className="font-medium text-brand hover:underline">View deployments →</button>
        <button type="button" onClick={() => router.push("/admin/app-store")} className="font-medium text-text hover:underline">Browse the App Store →</button>
      </div>

      <Card>
        <CardHeader title={isLoading || isError ? "Applications" : `${filtered.length} application${filtered.length === 1 ? "" : "s"}`} icon={Layers} />
        <div className="flex flex-wrap items-end gap-3 pb-4">
          <div className="flex-1 min-w-[200px]">
            <Input label="Search applications" placeholder="Search by name..." value={search} onChange={setSearch} />
          </div>
          <select
            aria-label="Application type"
            className="h-9 rounded-lg border border-line bg-[var(--surface-input)] px-3 text-xs text-text-subtle outline-none"
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
          >
            <option value="">All Types</option>
            {types.map((t) => (
              <option key={t} value={t}>{typeLabel(t)}</option>
            ))}
          </select>
        </div>

        {isLoading ? (
          <AdminLoadingState label="Loading applications..." />
        ) : isError ? (
          <div className="p-4"><AdminErrorState message="Applications unavailable — could not load applications." retry={() => void refetch()} /></div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Layers} title={search || typeFilter ? "No matching applications" : "No applications yet"} message={search || typeFilter ? "Try a different name or application type." : "Create an application or start from the App Store."} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-line text-left text-[10px] uppercase tracking-widest text-text-muted">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Type</th>
                  <th className="px-4 py-3">Image/Version</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {filtered.map((app) => {
                  const Icon = APP_TYPE_ICONS[app.type] ?? Layers;
                  return (
                    <tr key={app.id} className="hover:bg-overlay-subtle">
                      <td className="px-4 py-3">
                        <button
                          type="button"
                          className="flex items-center gap-2 font-semibold text-left hover:text-text"
                          onClick={() => router.push(`/admin/apps/${app.id}`)}
                        >
                          <Icon size={14} className="text-text-muted" />
                          {app.name}
                        </button>
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone="neutral">{typeLabel(app.type)}</Pill>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-text-subtle">
                        {app.image ?? app.version ?? "—"}
                      </td>
                      <td className="px-4 py-3">
                        <DeployStatusBadge status={app.status} type="app" />
                      </td>
                      <td className="px-4 py-3 text-xs text-text-muted">
                        {new Date(app.createdAt).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          {app.status === "running" && (
                            <>
                              <Btn size="sm" tone="ghost" ariaLabel={`Stop ${app.name}`} disabled={startMut.isPending || stopMut.isPending || restartMut.isPending} onClick={() => stopMut.mutate(app.id)}>
                                <Square size={12} />
                              </Btn>
                              <Btn size="sm" tone="ghost" ariaLabel={`Restart ${app.name}`} disabled={startMut.isPending || stopMut.isPending || restartMut.isPending} onClick={() => restartMut.mutate(app.id)}>
                                <RotateCcw size={12} />
                              </Btn>
                            </>
                          )}
                          {app.status === "stopped" && (
                            <Btn size="sm" tone="success" ariaLabel={`Start ${app.name}`} disabled={startMut.isPending || stopMut.isPending || restartMut.isPending} onClick={() => startMut.mutate(app.id)}>
                              <Power size={12} />
                            </Btn>
                          )}
                          {app.status === "failed" && (
                            <Btn size="sm" tone="warning" ariaLabel={`Start ${app.name}`} disabled={startMut.isPending || stopMut.isPending || restartMut.isPending} onClick={() => startMut.mutate(app.id)}>
                              <RefreshCw size={12} />
                            </Btn>
                          )}
                          <Btn size="sm" tone="ghost" onClick={() => router.push(`/admin/apps/${app.id}?tab=logs`)}>
                            <FileText size={12} />
                          </Btn>
                          <Btn size="sm" tone="ghost" onClick={() => router.push(`/admin/apps/${app.id}?tab=console`)}>
                            <Terminal size={12} />
                          </Btn>
                          <Btn size="sm" tone="danger" ariaLabel={`Delete ${app.name}`} onClick={() => setDeleteTarget(app)}>
                            <Trash2 size={12} />
                          </Btn>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {deleteTarget && (
        <Modal title={`Delete ${deleteTarget.name}`} onClose={() => setDeleteTarget(null)}>
          <div className="space-y-4">
          <p className="text-sm text-text-subtle">
            Are you sure you want to delete <span className="font-semibold text-text">{deleteTarget.name}</span>?
            This action cannot be undone.
          </p>
          {deleteMut.error ? (
            <p className="text-sm text-danger">{deleteMut.error.message}</p>
          ) : null}
          </div>
          <ModalFooter
            onCancel={() => setDeleteTarget(null)}
            onConfirm={() => deleteMut.mutate(deleteTarget.id)}
            confirmLabel="Delete"
            disabled={deleteMut.isPending}
          />
        </Modal>
      )}
    </AdminPageLayout>
  );
}
