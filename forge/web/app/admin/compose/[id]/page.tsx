"use client";

import { useState, useMemo, Suspense } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import {
  Play, Square, RotateCcw, Trash2, Loader2, Terminal,
  XCircle, Eye, EyeOff, Save
} from "lucide-react";
import { AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, Pill, AdminLoadingState, cn, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { getComposeStackStatus, getComposeStackLogs, stopComposeStack, startComposeStack, deployComposeStack, deleteComposeStack, restartComposeStack, updateComposeStack, detectDrift, getComposeGitStatus, getLastWebhook, redeployFromGit, pullAndRedeploy, rollbackCompose, checkUpdate } from "@/lib/api/compose";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { composeStatusTone } from "@/lib/api/status";



type ComposeTab = "overview" | "services" | "logs" | "yaml" | "gitops";
const COMPOSE_TABS: ComposeTab[] = ["overview", "services", "logs", "yaml", "gitops"];

function ComposeStackDetailContent() {
  const [confirm, renderConfirm] = useConfirm();
  const params = useParams();
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const id = params.id as string;
  // Router-driven tab state (Phase 06 beautify): ?tab= survives refresh, matches admin/apps/[id] pattern
  const rawTab = searchParams.get("tab") as ComposeTab | null;
  const tab: ComposeTab = rawTab && COMPOSE_TABS.includes(rawTab) ? rawTab : "overview";
  const setTab = (next: ComposeTab) => {
    router.replace(`/admin/compose/${encodeURIComponent(id)}?tab=${encodeURIComponent(next)}`, { scroll: false });
  };
  const showYaml = tab === "yaml";
  const [logService, setLogService] = useState("");
  const [logTail, setLogTail] = useState(100);
  const [editYaml, setEditYaml] = useState("");
  const [editingYaml, setEditingYaml] = useState(false);
  const [deleteVolumes, setDeleteVolumes] = useState(false);

  const { data: status, isLoading } = useQuery({
    queryKey: ["compose-stack-status", id],
    queryFn: () => getComposeStackStatus(id),
    refetchInterval: 10000,
  });

  const { data: logs } = useQuery<string[]>({
    queryKey: ["compose-stack-logs", id, logService, logTail],
    queryFn: async () => {
      const data = await getComposeStackLogs(id, logService || undefined, logTail) as { services?: Record<string, string> };
      if (data.services) {
        const allLogs = data.services._all || "";
        return allLogs.split("\n").filter(Boolean);
      }
      return [];
    },
    refetchInterval: 5000,
  });

  const gitStatusQ = useQuery({
    queryKey: ["compose-git-status", id],
    queryFn: () => getComposeGitStatus(id),
    enabled: tab === "gitops",
  });

  const driftQ = useQuery({
    queryKey: ["compose-drift", id],
    queryFn: () => detectDrift(id),
    enabled: tab === "gitops",
  });

  const webhookQ = useQuery({
    queryKey: ["compose-webhook", id],
    queryFn: () => getLastWebhook(id),
    enabled: tab === "gitops",
  });

  const safeLogs = useMemo(() => Array.isArray(logs) ? logs : [], [logs]);
  const safeServices = useMemo(() => Array.isArray(status?.services) ? status.services : [], [status?.services]);

  const stopMutation = useMutation({
    mutationFn: () => stopComposeStack(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Stack stopped" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Stop failed", message: e.message }),
  });

  const startMutation = useMutation({
    mutationFn: () => startComposeStack(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Stack started" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Start failed", message: e.message }),
  });

  const redeployMutation = useMutation({
    mutationFn: () => deployComposeStack(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Redeploying stack" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Redeploy failed", message: e.message }),
  });

  const restartMutation = useMutation({
    mutationFn: () => restartComposeStack(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      toast({ tone: "success", title: "Stack restarting (docker compose restart)" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Restart failed", message: e.message }),
  });

  const updateMutation = useMutation({
    mutationFn: (yaml: string) => updateComposeStack(id, { composeYaml: yaml }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] });
      setEditingYaml(false);
      toast({ tone: "success", title: "Compose YAML updated — PATCH /compose/:id" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Update failed", message: e.message }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteComposeStack(id, { volumes: deleteVolumes }),
    onSuccess: () => {
      toast({ tone: "success", title: `Stack deleted${deleteVolumes ? " (volumes removed)" : ""}` });
      router.push("/admin/compose");
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  const gitRedeployMut = useMutation({
    mutationFn: () => redeployFromGit(id),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] }); toast({ tone: "success", title: "Git redeploy triggered — POST /compose/git/:id/redeploy" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Redeploy failed", message: e.message }),
  });
  const gitPullMut = useMutation({
    mutationFn: () => pullAndRedeploy(id),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] }); toast({ tone: "success", title: "Pull & redeploy — POST /compose/git/:id/pull-redeploy" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Pull failed", message: e.message }),
  });
  const gitRollbackMut = useMutation({
    mutationFn: () => rollbackCompose(id),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["compose-stack-status", id] }); toast({ tone: "success", title: "Rollback triggered — POST /compose/git/:id/rollback" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Rollback failed", message: e.message }),
  });
  const checkUpdateMut = useMutation({
    mutationFn: () => checkUpdate(id),
    onSuccess: (data) => toast({ tone: "success", title: "Check update — GET /compose/git/:id/check-update", message: JSON.stringify(data).slice(0,120) }),
    onError: (e: Error) => toast({ tone: "error", title: "Check failed", message: e.message }),
  });

  if (isLoading) {
    return (
      <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-11 w-11 animate-spin text-slate-400" />
        </div>
      </AdminPageLayout>
    );
  }

  if (!status) {
    return (
      <AdminPageLayout>
        <p className="text-slate-400">Stack not found.</p>
        <Btn tone="ghost" onClick={() => router.push("/admin/compose")}>Back to stacks</Btn>
      </AdminPageLayout>
    );
  }

  const { stack } = status;

  return (
    <AdminPageLayout className="max-w-6xl">
      <AdminPageHeader
        title={stack.name}
        description={stack.status.replace(/_/g, " ")}
        backAction={() => router.push("/admin/compose")}
        backLabel="Compose Stacks"
          action={
          <div className="flex items-center gap-2 flex-wrap">
            <Pill tone={composeStatusTone(stack.status) as "green" | "yellow" | "red" | "blue" | "neutral"}>{stack.status.replace(/_/g, " ")}</Pill>
            {stack.status === "stopped" && (
              <Btn size="sm" tone="success" onClick={() => startMutation.mutate()}>
                <Play className="h-4 w-4" /> Start
              </Btn>
            )}
            {stack.status === "running" && (
              <Btn size="sm" tone="warning" onClick={() => stopMutation.mutate()}>
                <Square className="h-4 w-4" /> Stop
              </Btn>
            )}
            {(stack.status === "running" || stack.status === "degraded") && (
              <Btn size="sm" tone="ghost" onClick={() => restartMutation.mutate()} disabled={restartMutation.isPending}>
                <RotateCcw className="h-4 w-4" /> Restart
              </Btn>
            )}
            <Btn size="sm" tone="primary" onClick={() => redeployMutation.mutate()}>
              <RotateCcw className="h-4 w-4" /> Redeploy
            </Btn>
            <label className="flex items-center gap-1 text-xs text-slate-400">
              <input type="checkbox" checked={deleteVolumes} onChange={(e) => setDeleteVolumes(e.target.checked)} className="accent-[var(--brand)]" /> volumes
            </label>
            <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: "Delete this compose stack?", description: `The stack and its resources will be removed${deleteVolumes ? " including volumes" : ""}. This cannot be undone. DELETE /compose/:id${deleteVolumes ? "?volumes=true" : ""}`, danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(); })(); }}>
              <Trash2 className="h-4 w-4" /> Delete
            </Btn>
          </div>
        }
      />

      <div className="flex gap-1 border-b border-white/[0.06] overflow-x-auto">
        {(COMPOSE_TABS as string[]).map((tId) => (
          <button
            key={tId}
            type="button"
            className={cn(
              "px-3 py-2 text-xs font-medium border-b-2 transition -mb-px capitalize whitespace-nowrap",
              tab === tId
                ? "border-[var(--brand)] text-[var(--brand)]"
                : "border-transparent text-slate-400 hover:text-slate-300",
            )}
            onClick={() => setTab(tId as ComposeTab)}
          >
            {tId.replace(/_/g, " ")}
          </button>
        ))}
      </div>

      {stack.error && (
        <div className="rounded-lg border border-red-500/50 bg-red-500/10 p-4">
          <div className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-red-400" />
            <span className="text-sm text-red-400">{stack.error}</span>
          </div>
        </div>
      )}

      {tab === "gitops" ? (
        <div className="space-y-4">
          <Card>
            <CardHeader title="GitOps — 9 endpoints wired" />
            <div className="p-4 space-y-3 text-sm">
              <div className="flex flex-wrap gap-2">
                <Btn size="sm" tone="ghost" onClick={() => gitRedeployMut.mutate()} disabled={gitRedeployMut.isPending}>Redeploy (git)</Btn>
                <Btn size="sm" tone="ghost" onClick={() => gitPullMut.mutate()} disabled={gitPullMut.isPending}>Pull & Redeploy</Btn>
                <Btn size="sm" tone="ghost" onClick={() => gitRollbackMut.mutate()} disabled={gitRollbackMut.isPending}>Rollback</Btn>
                <Btn size="sm" tone="ghost" onClick={() => checkUpdateMut.mutate()} disabled={checkUpdateMut.isPending}>Check Update</Btn>
                <Btn size="sm" tone="ghost" onClick={() => void gitStatusQ.refetch()}>Refresh Git Status</Btn>
                <Btn size="sm" tone="ghost" onClick={() => void driftQ.refetch()}>Detect Drift</Btn>
              </div>
              <p className="text-xs text-slate-500">Wires POST /compose/git/deploy, /:id/redeploy, /:id/check-update, /:id/preview, /:id/rollback, /:id/pull-redeploy, PUT /:id/branch, PUT /:id/auto-update, GET /:id/drift, GET /:id/status, GET /:id/last-webhook</p>
              {gitStatusQ.data ? (
                <pre className="rounded bg-[var(--surface-input)] p-3 text-xs font-mono text-slate-300 overflow-auto max-h-60">{JSON.stringify(gitStatusQ.data as unknown, null, 2)}</pre>
              ) : null}
              {driftQ.data ? (
                <div>
                  <p className="text-xs font-semibold text-slate-400">Drift — GET /compose/git/:id/drift</p>
                  <pre className="rounded bg-[var(--surface-input)] p-3 text-xs font-mono text-slate-300 overflow-auto max-h-60">{JSON.stringify(driftQ.data as unknown, null, 2)}</pre>
                </div>
              ) : null}
              {webhookQ.data && (
                <p className="text-xs text-slate-400">Last webhook: {webhookQ.data.lastWebhookAt ? new Date(webhookQ.data.lastWebhookAt).toLocaleString() : "—"} — GET /compose/git/:id/last-webhook</p>
              )}
              {gitStatusQ.isError && <p className="text-xs text-amber-400">Git status unavailable — stack may be raw (not git-backed)</p>}
            </div>
          </Card>
        </div>
      ) : (
      <>
      <div className="grid gap-6 lg:grid-cols-3">
        <Card>
          <CardHeader title="Info" />
          <div className="space-y-3 p-4 text-sm">
            <div className="flex justify-between">
              <span className="text-slate-400">ID</span>
              <span className="text-slate-300 font-mono text-xs">{stack.id}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Type</span>
              <span className="text-slate-300">{stack.composeType}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Source</span>
              <span className="text-slate-300">{stack.sourceType}</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Node</span>
              <Pill tone="neutral">{stack.nodeId ? stack.nodeId.slice(0, 12) : "—"}</Pill>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Group</span>
              <Pill tone="neutral">{(stack as unknown as { environmentId?: string }).environmentId ? (stack as unknown as { environmentId: string }).environmentId.slice(0, 12) : "—"}</Pill>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Health</span>
              <Pill tone={stack.error ? "red" : stack.status === "running" ? "green" : stack.status === "degraded" ? "yellow" : "neutral"}>{stack.error ? "Unhealthy" : stack.status === "running" ? "Healthy" : stack.status === "degraded" ? "Degraded" : stack.status.replace(/_/g, " ")}</Pill>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Created</span>
              <span className="text-slate-300">{new Date(stack.createdAt).toLocaleString()}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Updated</span>
              <span className="text-slate-300">{new Date(stack.updatedAt).toLocaleString()}</span>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Resources" />
          <div className="space-y-3 p-4 text-sm">
            <div className="flex justify-between">
              <span className="text-slate-400">Memory</span>
              <span className="text-slate-300">{stack.memoryMb || "—"} MB</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">CPU</span>
              <span className="text-slate-300">{stack.cpuShares || "—"} shares</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Disk</span>
              <span className="text-slate-300">{stack.diskMb || "—"} MB</span>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Compose YAML — PATCH /compose/:id" action={
            <div className="flex items-center gap-1">
              <button onClick={() => { setEditYaml(stack.composeYaml); setEditingYaml(true); }} className="text-slate-400 hover:text-[var(--brand)] transition-colors p-1" title="Edit YAML — PATCH /compose/:id">
                <Save className="h-4 w-4" />
              </button>
              <button onClick={() => setTab(showYaml ? "overview" : "yaml")} className="text-slate-400 hover:text-slate-200 transition-colors p-1">
                {showYaml ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
          } />
          <div className="p-4">
            {showYaml ? (
              <pre className="rounded bg-[var(--surface-input)] p-3 text-xs font-mono text-slate-300 overflow-auto max-h-60">{stack.composeYaml}</pre>
            ) : (
              <p className="text-xs text-slate-400">Click the eye icon to view or pencil to edit the compose file.</p>
            )}
          </div>
        </Card>
      </div>

      <Card>
        <CardHeader title="Services" />
        <div className="divide-y divide-white/[0.06]">
          {safeServices.length === 0 ? (
            <div className="p-8 text-center">
              <div className="text-sm text-slate-300">No services found.</div>
              <p className="mt-1 text-xs text-slate-500">Compose YAML parsed but no services detected.</p>
            </div>
          ) : (
            safeServices.map((svc) => {
              const svcStatus = svc.status || svc.state || "unknown";
              const tone = composeStatusTone(svcStatus) as "green" | "yellow" | "red" | "blue" | "neutral";
              return (
                <div key={svc.name} className="flex flex-wrap items-center justify-between gap-3 p-4 hover:bg-white/[0.02] transition-colors">
                  <div className="flex items-center gap-3 min-w-0">
                    <Pill tone={tone} className="gap-1.5">
                      <span className={`h-2 w-2 rounded-full bg-current opacity-80`} aria-hidden />
                      <span className="capitalize">{svcStatus.replace(/_/g, " ")}</span>
                    </Pill>
                    <div className="min-w-0">
                      <span className="text-sm font-medium text-slate-200">{svc.name}</span>
                      {svc.image && (
                        <span className="ml-2 text-xs text-slate-400 font-mono truncate">{svc.image}</span>
                      )}
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2 text-xs">
                    <Pill tone="neutral" className="font-mono text-[11px]">{svc.image ? svc.image.slice(0, 24) : "—"}</Pill>
                    {svc.ports && <Pill tone="neutral">{svc.ports}</Pill>}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </Card>

      <Card>
        <CardHeader title="Logs" icon={Terminal} action={
          <div className="flex items-center gap-3">
            <select
              value={logService}
              onChange={(e) => setLogService(e.target.value)}
              className="rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-2 py-1 text-xs text-slate-300 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
            >
              <option value="">All services</option>
              {safeServices.map((s) => (
                <option key={s.name} value={s.name}>{s.name}</option>
              ))}
            </select>
            <select
              value={logTail}
              onChange={(e) => setLogTail(Number(e.target.value))}
              className="rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-2 py-1 text-xs text-slate-300 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
            >
              <option value={50}>50 lines</option>
              <option value={100}>100 lines</option>
              <option value={500}>500 lines</option>
            </select>
          </div>
        } />
        <div className="p-4">
          <pre className="max-h-80 overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-4 text-xs font-mono text-slate-300">
            {safeLogs.length === 0 ? (
              <span className="text-slate-400">No logs available.</span>
            ) : (
              safeLogs.map((line, i) => (
                <div key={i} className="whitespace-pre-wrap break-all hover:bg-[var(--surface-hover)]">
                  {line}
                </div>
              ))
            )}
          </pre>
        </div>
      </Card>
      </>
      )}
      {editingYaml && (
        <Modal title="Edit Compose YAML" description="PATCH /compose/:id" onClose={() => setEditingYaml(false)} wide>
          <div className="space-y-4">
          <textarea
            value={editYaml}
            onChange={(e) => setEditYaml(e.target.value)}
            rows={20}
            className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-3 text-xs font-mono text-slate-200 placeholder:text-slate-500 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
            placeholder="services: ..."
          />
          <p className="text-xs text-slate-400">Wires <code className="font-mono">updateComposeStack</code> — PATCH /compose/:id with composeYaml</p>
          </div>
          <ModalFooter
            onCancel={() => setEditingYaml(false)}
            onConfirm={() => updateMutation.mutate(editYaml)}
            disabled={updateMutation.isPending || !editYaml.trim()}
            confirmLabel={updateMutation.isPending ? "Saving..." : "Save YAML"}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

export default function ComposeStackDetailPage() {
  return (
    <Suspense fallback={<AdminLoadingState label="Loading compose stack…" />}>
      <ComposeStackDetailContent />
    </Suspense>
  );
}
