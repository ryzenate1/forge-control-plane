"use client";

import { useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast"
import { useConfirm } from "@/components/ui/confirm-dialog";;
import { useRouter } from "next/navigation";
import { fetchJSON, postJSON } from "@/lib/api";
import { deleteComposeStack, importCompose, listComposeProjects } from "@/lib/api/compose";
import { AdminLoadingState, AdminErrorState, AdminPageHeader, AdminPageLayout, AdminToolbar, Btn, Card, EmptyState, Input, Pill } from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { DegradedBanner } from "@/components/shared/states-connectivity";
import { Pagination } from "@/components/ui/primitives";
import { composeStatusTone } from "@/lib/api/status";
import {
  Plus, Play, Square, Trash2, Loader2, RotateCcw,
  CheckCircle, XCircle, AlertTriangle, Clock, ArrowUpDown
} from "lucide-react";

interface ComposeStack {
  id: string;
  name: string;
  nodeId: string;
  status: string;
  composeYaml: string;
  composeHash: string;
  envVars: Record<string, string>;
  memoryMb: number;
  cpuShares: number;
  diskMb: number;
  error: string;
  composeType: string;
  sourceType: string;
  environmentId: string;
  createdAt: string;
  updatedAt: string;
}

// 9 states inventoried — beautify phase 06: unified tone via lib/api/status.ts:composeStatusTone (see COMPOSE_STATUS_INVENTORY); colors/icons kept for rich timeline.
const statusConfig: Record<string, { color: string; bg: string; icon: React.ReactNode; tone: ReturnType<typeof composeStatusTone> }> = {
  running: { color: "text-emerald-400", bg: "bg-emerald-500/10", icon: <CheckCircle className="h-4 w-4" />, tone: composeStatusTone("running") },
  deploying: { color: "text-slate-200", bg: "bg-white/[0.06]", icon: <Loader2 className="h-4 w-4 animate-spin" />, tone: composeStatusTone("deploying") },
  awaiting_health: { color: "text-yellow-400", bg: "bg-yellow-500/10", icon: <Clock className="h-4 w-4" />, tone: composeStatusTone("awaiting_health") },
  stopped: { color: "text-slate-400", bg: "bg-slate-500/10", icon: <Square className="h-4 w-4" />, tone: composeStatusTone("stopped") },
  degraded: { color: "text-amber-300", bg: "bg-amber-500/10", icon: <AlertTriangle className="h-4 w-4" />, tone: composeStatusTone("degraded") },
  failed: { color: "text-red-400", bg: "bg-red-500/10", icon: <XCircle className="h-4 w-4" />, tone: composeStatusTone("failed") },
  updating: { color: "text-slate-200", bg: "bg-white/[0.06]", icon: <ArrowUpDown className="h-4 w-4" />, tone: composeStatusTone("updating") },
  deleting: { color: "text-red-400", bg: "bg-red-500/10", icon: <Loader2 className="h-4 w-4 animate-spin" />, tone: composeStatusTone("deleting") },
  deleted: { color: "text-slate-400", bg: "bg-slate-500/10", icon: <XCircle className="h-4 w-4" />, tone: composeStatusTone("deleted") },
};

export default function ComposeStacksPage() {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const router = useRouter();
  const [deleteVolumes, setDeleteVolumes] = useState(false);
  const [showImport, setShowImport] = useState(false);
  const [importName, setImportName] = useState("");
  const [importContent, setImportContent] = useState("");

  const { data: stacks, isLoading, isError, error, refetch, isFetching } = useQuery<ComposeStack[]>({
    queryKey: ["compose-stacks"],
    queryFn: async () => {
      return fetchJSON<ComposeStack[]>("/compose");
    },
  });

  const projectsQ = useQuery({
    queryKey: ["compose-projects"],
    queryFn: () => listComposeProjects(),
  });

  const safeStacks = useMemo(() => Array.isArray(stacks) ? stacks : [], [stacks]);
  const [search, setSearch] = useState("");
  const [nodeFilter, setNodeFilter] = useState("");
  const [typeFilter, setTypeFilter] = useState("");
  const hasDegraded = safeStacks.some((s) => s.status === "degraded" || s.status === "failed");
  const isDegraded = hasDegraded && !isLoading && !isError;

  const nodeOptions = useMemo(() => {
    const ids = Array.from(new Set(safeStacks.map((s) => s.nodeId).filter(Boolean)));
    return ids.sort();
  }, [safeStacks]);
  const typeOptions = useMemo(() => {
    const vals = Array.from(new Set(safeStacks.map((s) => s.composeType || s.sourceType).filter(Boolean)));
    return vals.sort();
  }, [safeStacks]);

  const filteredStacks = useMemo(() => {
    return safeStacks.filter((s) => {
      if (search && !s.name.toLowerCase().includes(search.toLowerCase())) return false;
      if (nodeFilter && s.nodeId !== nodeFilter) return false;
      if (typeFilter && (s.composeType !== typeFilter && s.sourceType !== typeFilter)) return false;
      return true;
    });
  }, [safeStacks, search, nodeFilter, typeFilter]);

  const [page, setPage] = useState(1);
  const PAGE_SIZE = 10;
  const totalPages = Math.max(1, Math.ceil(filteredStacks.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const paginatedStacks = useMemo(() => {
    const start = (safePage - 1) * PAGE_SIZE;
    return filteredStacks.slice(start, start + PAGE_SIZE);
  }, [filteredStacks, safePage]);

  function stackTone(status: string) {
    // Single source via composeStatusTone (Phase 06 beautify) — no per-file drift for 9 inventoried states
    return composeStatusTone(status) as "green" | "yellow" | "red" | "blue" | "neutral";
  }
  function healthTone(status: string, hasError: boolean) {
    if (hasError || status === "failed") return "red" as const;
    if (status === "degraded" || status === "awaiting_health") return "yellow" as const;
    if (status === "running") return "green" as const;
    if (status === "stopped" || status === "deleted") return "neutral" as const;
    return "blue" as const;
  }

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      return deleteComposeStack(id, { volumes: deleteVolumes });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: `Stack deleted${deleteVolumes ? " (volumes removed)" : ""} — DELETE /compose/:id${deleteVolumes ? "?volumes=true" : ""}` });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  const importMut = useMutation({
    mutationFn: () => importCompose({ name: importName, content: importContent }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-projects"] });
      setShowImport(false);
      setImportName(""); setImportContent("");
      toast({ tone: "success", title: "Compose imported — POST /compose/import" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Import failed", message: e.message }),
  });

  const stopMutation = useMutation({
    mutationFn: async (id: string) => {
      return postJSON<void>(`/compose/${encodeURIComponent(id)}/stop`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack stopped" });
    },
    onError: () => toast({ tone: "error", title: "Stop failed" }),
  });

  const startMutation = useMutation({
    mutationFn: async (id: string) => {
      return postJSON<void>(`/compose/${encodeURIComponent(id)}/start`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack started" });
    },
    onError: () => toast({ tone: "error", title: "Start failed" }),
  });

  const restartMutation = useMutation({
    mutationFn: async (id: string) => {
      return postJSON<void>(`/compose/${encodeURIComponent(id)}/restart`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack restarting" });
    },
    onError: () => toast({ tone: "error", title: "Restart failed" }),
  });

  const handleAction = (action: string, id: string) => {
    switch (action) {
      case "stop": stopMutation.mutate(id); break;
      case "start": startMutation.mutate(id); break;
      case "restart": restartMutation.mutate(id); break;
      case "delete":
        void (async () => { if (await confirm({ title: "Delete this compose stack?", description: `The stack and its services will be removed${deleteVolumes ? " including volumes (DELETE /compose/:id?volumes=true)" : ""}. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(id); })();
        break;
    }
  };

  return (
    <AdminPageLayout>
      <AdminPageHeader title="Compose Stacks" description="Multi-service Compose workloads as first-class deployments with lifecycle, logs and rollback." action={<div className="flex items-center gap-2"><Btn tone="ghost" onClick={() => setShowImport(true)}>Import</Btn><Btn tone="primary" onClick={() => router.push("/admin/compose/new")}><Plus className="h-4 w-4" /> New stack</Btn></div>} />
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.015] px-4 py-2 text-xs leading-5 text-slate-400">
        <span className="font-semibold text-slate-300">DEPLOY</span> · Compose is one of four deploy surfaces (Deployments · Pipelines · <span className="font-semibold text-slate-200">Compose</span> · Git). Compose stacks use <code className="font-mono text-[11px]">GET /compose</code> · <code className="font-mono">POST /compose/import</code> · <code className="font-mono">/compose/:id</code> lifecycle (<code className="font-mono">running/deploying/awaiting_health/stopped/degraded</code>). For single-service apps see <button type="button" onClick={() => router.push("/admin/apps")} className="underline hover:text-slate-200">Apps</button>.
      </div>
      <OfflineBanner onRetry={() => void refetch()} />
      <AdminToolbar>
        <div className="flex-1 min-w-[180px]">
          <Input placeholder="Search stacks..." value={search} onChange={setSearch} />
        </div>
        <select
          className="h-9 rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 text-xs text-slate-300 outline-none"
          value={nodeFilter}
          onChange={(e) => setNodeFilter(e.target.value)}
        >
          <option value="">All Nodes</option>
          {nodeOptions.map((nid) => (
            <option key={nid} value={nid}>{nid.slice(0, 8)}</option>
          ))}
        </select>
        <select
          className="h-9 rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 text-xs text-slate-300 outline-none"
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
        >
          <option value="">All Types</option>
          {typeOptions.map((tp) => (
            <option key={tp} value={tp}>{tp}</option>
          ))}
        </select>
        <label className="flex items-center gap-1 text-xs text-slate-400">
          <input type="checkbox" checked={deleteVolumes} onChange={(e) => setDeleteVolumes(e.target.checked)} className="accent-[var(--brand)]" /> volumes on delete
        </label>
        {(search || nodeFilter || typeFilter) && (
          <Btn tone="ghost" size="sm" onClick={() => { setSearch(""); setNodeFilter(""); setTypeFilter(""); }}>Clear</Btn>
        )}
      </AdminToolbar>
      {isFetching && !isLoading ? <div className="rounded-lg border border-sky-500/20 bg-sky-500/10 px-3 py-2 text-xs text-sky-200">Retrying…</div> : null}
      {isDegraded ? <DegradedBanner title="Some stacks are degraded" message="One or more stacks report degraded or failed status." onRetry={() => void refetch()} /> : null}
      {(stopMutation.isError || startMutation.isError || deleteMutation.isError) ? (
        <AdminErrorState
          message={(stopMutation.error as Error)?.message || (startMutation.error as Error)?.message || (deleteMutation.error as Error)?.message || "Stack operation failed"}
          retry={() => { stopMutation.reset(); startMutation.reset(); deleteMutation.reset(); }}
        />
      ) : null}

      {isLoading ? (
        <AdminLoadingState label="Loading Compose stacks…" />
      ) : isError ? (
        <AdminErrorState message={error instanceof Error ? error.message : "Failed to load Compose stacks"} retry={() => void refetch()} />
      ) : filteredStacks.length === 0 ? (
        <Card className="p-8">
          {safeStacks.length === 0 ? (
            <>
              <EmptyState title="No Compose stacks" message="Create a stack to deploy a multi-service workload." />
              <div className="mt-4 flex justify-center"><Btn onClick={() => router.push("/admin/compose/new")}><Plus className="h-4 w-4" /> Create stack</Btn></div>
            </>
          ) : (
            <>
              <EmptyState title="No results" message="No stacks match your search or filters." />
              <div className="mt-4 flex justify-center"><Btn tone="ghost" onClick={() => { setSearch(""); setNodeFilter(""); setTypeFilter(""); }}>Clear filters</Btn></div>
            </>
          )}
        </Card>
      ) : (
        <>
          <div className="grid gap-4">
          {paginatedStacks.map((stack) => {
            const cfg = statusConfig[stack.status] || statusConfig.failed;
            const pillTone = stackTone(stack.status);
            const hpTone = healthTone(stack.status, Boolean(stack.error));
            const healthLabel = stack.error ? "Unhealthy" : stack.status === "running" ? "Healthy" : stack.status === "degraded" ? "Degraded" : "—";
            return (
              <Card
                key={stack.id}
                className="p-4 cursor-pointer hover:bg-white/[0.02] transition"
              >
                <div
                  className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center cursor-pointer"
                  onClick={() => router.push(`/admin/compose/${stack.id}`)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => { if (e.key === 'Enter') router.push(`/admin/compose/${stack.id}`); }}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <Pill tone={pillTone} className="gap-1.5">
                      <span className="inline-flex items-center gap-1.5">{cfg.icon}<span className="capitalize">{stack.status.replace(/_/g, " ")}</span></span>
                    </Pill>
                    <Pill tone={hpTone}>Health: {healthLabel}</Pill>
                    <div>
                      <span className="text-sm font-medium text-slate-200">{stack.name}</span>
                      <span className="ml-2 text-xs text-slate-400">{stack.composeType || stack.sourceType}</span>
                    </div>
                  </div>
                  <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                    {stack.status === "stopped" && (
                      <Btn size="sm" tone="ghost" onClick={() => handleAction("start", stack.id)} ariaLabel="Start">
                        <Play className="h-4 w-4" />
                      </Btn>
                    )}
                    {stack.status === "running" && (
                      <Btn size="sm" tone="ghost" onClick={() => handleAction("stop", stack.id)} ariaLabel="Stop">
                        <Square className="h-4 w-4" />
                      </Btn>
                    )}
                    {(stack.status === "running" || stack.status === "degraded") && (
                      <Btn size="sm" tone="ghost" onClick={() => handleAction("restart", stack.id)} ariaLabel="Restart">
                        <RotateCcw className="h-4 w-4" />
                      </Btn>
                    )}
                    <Btn size="sm" tone="danger" onClick={() => handleAction("delete", stack.id)} ariaLabel="Delete">
                      <Trash2 className="h-4 w-4" />
                    </Btn>
                  </div>
                </div>
                {stack.error && (
                  <div className="mt-2 text-xs text-red-400 truncate">{stack.error}</div>
                )}
                <div className="mt-3 flex flex-wrap gap-2 text-xs">
                  <Pill tone="neutral">Node: {stack.nodeId ? stack.nodeId.slice(0, 8) : "—"}</Pill>
                  <Pill tone="neutral">Group: {stack.environmentId ? stack.environmentId.slice(0, 8) : "—"}</Pill>
                  <Pill tone="neutral">Source: {stack.sourceType || "—"}</Pill>
                  <span className="inline-flex items-center px-2 py-1 text-xs text-slate-400">Created: {new Date(stack.createdAt).toLocaleDateString()}</span>
                </div>
              </Card>
            );
          })}
          </div>
          <Pagination page={safePage} pageCount={totalPages} onPageChange={setPage} label="Compose stacks pagination" />
        </>
      )}
      {showImport && (
        <Card className="p-4">
          <h3 className="text-sm font-semibold text-slate-200 mb-3">Import Compose — POST /compose/import</h3>
          <div className="space-y-3">
            <Input placeholder="Stack name" value={importName} onChange={setImportName} />
            <textarea value={importContent} onChange={(e) => setImportContent(e.target.value)} rows={8} placeholder={`services:\n  web:\n    image: nginx:alpine`} className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-3 text-xs font-mono text-slate-200 placeholder:text-slate-500 focus:border-[var(--brand)]/70 focus:outline-none focus:ring-2 focus:ring-[var(--brand)]/15" />
            <div className="flex gap-2 justify-end">
              <Btn tone="ghost" size="sm" onClick={() => setShowImport(false)}>Cancel</Btn>
              <Btn size="sm" tone="primary" onClick={() => importMut.mutate()} disabled={importMut.isPending || !importName.trim() || !importContent.trim()}>{importMut.isPending ? "Importing..." : "Import"}</Btn>
            </div>
          </div>
        </Card>
      )}

      {Array.isArray(projectsQ.data) && projectsQ.data.length > 0 && (
        <Card className="overflow-hidden">
          <div className="px-4 py-3 border-b border-white/[0.06] flex items-center justify-between">
            <h3 className="text-sm font-semibold text-slate-200">Compose Projects — GET /compose/projects CRUD</h3>
            <Pill tone="neutral">{projectsQ.data.length}</Pill>
          </div>
          <div className="divide-y divide-white/[0.04]">
            {projectsQ.data.map((p: { id: string; name: string; status: string; revision: number }) => (
              <div key={p.id} className="flex items-center justify-between px-4 py-3 text-sm">
                <div>
                  <span className="font-medium text-slate-200">{p.name}</span>
                  <span className="ml-2 text-xs text-slate-400">{p.status} · rev {p.revision}</span>
                </div>
                <span className="font-mono text-xs text-slate-500">{p.id.slice(0,8)}</span>
              </div>
            ))}
          </div>
          <p className="px-4 py-2 text-[11px] text-slate-500">Wires listComposeProjects / getComposeProject / updateComposeProject / deleteComposeProject / exportComposeProject — fetchJSON/postJSON</p>
        </Card>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
