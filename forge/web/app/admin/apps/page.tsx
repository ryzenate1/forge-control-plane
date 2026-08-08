"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  Box, Container, FileText, GitBranch, Layers, Plus,
  Power, RefreshCw, RotateCcw, Square,
  Terminal, Trash2,
} from "lucide-react";
import { fetchApps, startApp, stopApp, restartApp, deleteApp, typeLabel, type ApiApp, type AppType } from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { DeployStatusBadge } from "@/components/admin/AdminAppsShared";

const typeIcons: Record<AppType, typeof Container> = {
  image: Box,
  git: GitBranch,
  compose: Container,
  game_server: Layers,
};

export default function AdminAppsPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<string>("");
  const [deleteTarget, setDeleteTarget] = useState<ApiApp | null>(null);

  const { data: apps = [], isLoading } = useQuery({
    queryKey: ["apps"],
    queryFn: fetchApps,
    refetchInterval: 15_000,
  });

  const startMut = useMutation({
    mutationFn: startApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["apps"] }),
  });
  const stopMut = useMutation({
    mutationFn: stopApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["apps"] }),
  });
  const restartMut = useMutation({
    mutationFn: restartApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["apps"] }),
  });
  const deleteMut = useMutation({
    mutationFn: deleteApp,
    onSuccess: () => {
      setDeleteTarget(null);
      void qc.invalidateQueries({ queryKey: ["apps"] });
    },
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
    <div className="space-y-6">
      <SectionHeader
        title="Applications"
        sub="Manage Docker images, Git repos, Compose stacks, and game servers."
        action={
          <Btn tone="primary" onClick={() => router.push("/admin/apps/new")}>
            <Plus size={14} /> Create App
          </Btn>
        }
      />

      <Card>
        <CardHeader title={`${filtered.length} application${filtered.length === 1 ? "" : "s"}`} icon={Layers} />
        <div className="flex flex-wrap items-center gap-3 p-4">
          <div className="flex-1 min-w-[200px]">
            <Input placeholder="Search by name..." value={search} onChange={setSearch} />
          </div>
          <select
            className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
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
          <div className="p-8 text-center text-sm text-slate-500">Loading applications...</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Layers} message="No applications found." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Type</th>
                  <th className="px-4 py-3">Image/Version</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((app) => {
                  const Icon = typeIcons[app.type] ?? Layers;
                  return (
                    <tr key={app.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3">
                        <button
                          type="button"
                          className="flex items-center gap-2 font-semibold text-left hover:text-white"
                          onClick={() => router.push(`/admin/apps/${app.id}`)}
                        >
                          <Icon size={14} className="text-slate-500" />
                          {app.name}
                        </button>
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone="neutral">{typeLabel(app.type)}</Pill>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">
                        {app.image ?? app.version ?? "—"}
                      </td>
                      <td className="px-4 py-3">
                        <DeployStatusBadge status={app.status} type="app" />
                      </td>
                      <td className="px-4 py-3 text-xs text-slate-500">
                        {new Date(app.createdAt).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          {app.status === "running" && (
                            <>
                              <Btn size="sm" tone="ghost" onClick={() => stopMut.mutate(app.id)}>
                                <Square size={12} />
                              </Btn>
                              <Btn size="sm" tone="ghost" onClick={() => restartMut.mutate(app.id)}>
                                <RotateCcw size={12} />
                              </Btn>
                            </>
                          )}
                          {app.status === "stopped" && (
                            <Btn size="sm" tone="success" onClick={() => startMut.mutate(app.id)}>
                              <Power size={12} />
                            </Btn>
                          )}
                          {app.status === "failed" && (
                            <Btn size="sm" tone="warning" onClick={() => startMut.mutate(app.id)}>
                              <RefreshCw size={12} />
                            </Btn>
                          )}
                          <Btn size="sm" tone="ghost" onClick={() => router.push(`/admin/apps/${app.id}?tab=logs`)}>
                            <FileText size={12} />
                          </Btn>
                          <Btn size="sm" tone="ghost" onClick={() => router.push(`/admin/apps/${app.id}?tab=console`)}>
                            <Terminal size={12} />
                          </Btn>
                          <Btn size="sm" tone="danger" onClick={() => setDeleteTarget(app)}>
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
          <p className="text-sm text-slate-300">
            Are you sure you want to delete <span className="font-semibold text-white">{deleteTarget.name}</span>?
            This action cannot be undone.
          </p>
          {deleteMut.error ? (
            <p className="mt-3 text-sm text-red-400">{deleteMut.error.message}</p>
          ) : null}
          <ModalFooter
            onCancel={() => setDeleteTarget(null)}
            onConfirm={() => deleteMut.mutate(deleteTarget.id)}
            confirmLabel="Delete"
            disabled={deleteMut.isPending}
          />
        </Modal>
      )}
    </div>
  );
}
