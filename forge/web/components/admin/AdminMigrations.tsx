"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, ArrowRightLeft, Ban, CheckCircle2, Loader2, Play, Plus } from "lucide-react";
import {
  cancelMigration,
  cancelRecoveryPlan,
  createMigration,
  createRecoveryPlan,
  executeMigration,
  fetchMigrationExecutorStatus,
  fetchMigrations,
  fetchNodes,
  fetchRecoveryPlans,
  fetchServers,
  startRecoveryPlan,
  type ApiMigration,
  type ApiRecoveryPlan,
} from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import {
  AdminConfirmDialog,
  AdminPageLayout,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
} from "./admin-ui";

const terminalStatuses = ["completed", "restored", "cancelled", "failed"];

function statusTone(status: string): "green" | "red" | "yellow" | "blue" {
  if (["completed", "restored"].includes(status)) return "green";
  if (["failed", "cancelled"].includes(status)) return "red";
  if (["planned", "pending", "planning"].includes(status)) return "yellow";
  return "blue";
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Unknown error";
}

function MigrationActions({ migration, executorAvailable, onAction }: {
  migration: ApiMigration;
  executorAvailable: boolean;
  onAction: () => void;
}) {
  const { toast } = useToast();
  const executeMut = useMutation({
    mutationFn: () => executeMigration(migration.id),
    onSuccess: () => { toast({ tone: "success", title: "Migration started" }); onAction(); },
    onError: (error) => toast({ tone: "error", title: "Execute failed", message: errorMessage(error) }),
  });
  const cancelMut = useMutation({
    mutationFn: () => cancelMigration(migration.id),
    onSuccess: () => { toast({ tone: "success", title: "Migration cancelled" }); onAction(); },
    onError: (error) => toast({ tone: "error", title: "Cancel failed", message: errorMessage(error) }),
  });

  return (
    <div className="flex gap-1.5">
      {migration.status === "planned" && (
        <Btn size="sm" tone="success" onClick={() => executeMut.mutate()} disabled={!executorAvailable || executeMut.isPending}>
          {executeMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <CheckCircle2 size={12} />} Execute
        </Btn>
      )}
      {!terminalStatuses.includes(migration.status) && (
        <Btn size="sm" tone="danger" onClick={() => cancelMut.mutate()} disabled={cancelMut.isPending}>
          {cancelMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Ban size={12} />} Cancel
        </Btn>
      )}
    </div>
  );
}

function CreateMigrationModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const servers = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
  const nodes = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });

  const [serverId, setServerId] = useState("");
  const [targetNodeId, setTargetNodeId] = useState("");

  const createMut = useMutation({
    mutationFn: () => createMigration({ serverId, targetNodeId: targetNodeId || undefined }),
    onSuccess: () => {
      toast({ tone: "success", title: "Migration plan created" });
      setServerId("");
      setTargetNodeId("");
      qc.invalidateQueries({ queryKey: ["migrations"] });
      onClose();
    },
    onError: (error) => toast({ tone: "error", title: "Migration creation failed", message: errorMessage(error) }),
  });

  return (
    <Modal title="Create Migration" onClose={onClose} open={open}>
      <div className="space-y-4">
        <label className="block text-sm text-slate-300">
          Server
          <select
            className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3"
            value={serverId}
            onChange={(e) => setServerId(e.target.value)}
          >
            <option value="">Select server…</option>
            {(servers.data ?? []).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </label>
        <label className="block text-sm text-slate-300">
          Target node (optional)
          <select
            className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3"
            value={targetNodeId}
            onChange={(e) => setTargetNodeId(e.target.value)}
          >
            <option value="">Automatic</option>
            {(nodes.data ?? []).map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
          </select>
        </label>
        {createMut.error && (
          <div className="rounded border border-red-700/30 bg-red-900/10 p-3 text-xs text-red-200">
            {errorMessage(createMut.error)}
          </div>
        )}
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={() => createMut.mutate()}
        disabled={!serverId || createMut.isPending}
        confirmLabel={createMut.isPending ? "Creating…" : "Create Plan"}
      />
    </Modal>
  );
}

function CreateRecoveryPlanModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const nodes = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });

  const [nodeId, setNodeId] = useState("");
  const [reason, setReason] = useState("");

  const createMut = useMutation({
    mutationFn: () => createRecoveryPlan({ nodeId, reason: reason.trim() }),
    onSuccess: () => {
      toast({ tone: "success", title: "Recovery plan created" });
      setNodeId("");
      setReason("");
      qc.invalidateQueries({ queryKey: ["recovery"] });
      onClose();
    },
    onError: (error) => toast({ tone: "error", title: "Recovery plan creation failed", message: errorMessage(error) }),
  });

  return (
    <Modal title="Create Recovery Plan" onClose={onClose} open={open}>
      <div className="space-y-4">
        <label className="block text-sm text-slate-300">
          Affected node
          <select
            className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3"
            value={nodeId}
            onChange={(e) => setNodeId(e.target.value)}
          >
            <option value="">Select node…</option>
            {(nodes.data ?? []).map((n) => <option key={n.id} value={n.id}>{n.name}</option>)}
          </select>
        </label>
        <Input label="Recovery reason" value={reason} onChange={setReason} placeholder="e.g. node is unavailable" />
        {createMut.error && (
          <div className="rounded border border-red-700/30 bg-red-900/10 p-3 text-xs text-red-200">
            {errorMessage(createMut.error)}
          </div>
        )}
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={() => createMut.mutate()}
        disabled={!nodeId || !reason.trim() || createMut.isPending}
        confirmLabel={createMut.isPending ? "Creating…" : "Create Plan"}
      />
    </Modal>
  );
}

function RecoveryPlanItem({ plan }: { plan: ApiRecoveryPlan }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="border-b border-white/[0.04] last:border-0">
      <div className="flex cursor-pointer items-center justify-between px-4 py-3 hover:bg-white/[0.02]" onClick={() => setExpanded(!expanded)}>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-slate-200">{plan.nodeId}</span>
            <Pill tone={statusTone(plan.status)}>{plan.status}</Pill>
          </div>
          <p className="mt-0.5 truncate text-xs text-slate-500">{plan.reason}</p>
          <p className="text-xs text-slate-500">{plan.items.length} workload(s)</p>
        </div>
      </div>
      {expanded && plan.items.length > 0 && (
        <div className="border-t border-white/[0.04] bg-black/10 px-4 py-3">
          <div className="space-y-2">
            {plan.items.map((item) => (
              <div key={item.id} className="rounded border border-white/[0.06] p-2 text-xs">
                <div className="flex items-center justify-between gap-2">
                  <code className="text-slate-200">{item.serverId}</code>
                  <Pill tone={statusTone(item.status)}>{item.status}</Pill>
                </div>
                <p className="mt-1 text-slate-500">{item.sourceNodeId} → {item.targetNodeId || "No target assigned"}</p>
                {item.reason && <p className="mt-1 text-amber-300">{item.reason}</p>}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export default function AdminMigrations() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [showCreateMigration, setShowCreateMigration] = useState(false);
  const [showCreateRecovery, setShowCreateRecovery] = useState(false);
  const [cancelConfirmId, setCancelConfirmId] = useState<string | null>(null);
  const [startConfirmId, setStartConfirmId] = useState<string | null>(null);

  const migrations = useQuery({ queryKey: ["migrations"], queryFn: fetchMigrations, refetchInterval: 10_000 });
  const recoveries = useQuery({ queryKey: ["recovery"], queryFn: fetchRecoveryPlans, refetchInterval: 10_000 });
  const executorStatus = useQuery({ queryKey: ["migrations-executor"], queryFn: fetchMigrationExecutorStatus, staleTime: 30_000 });
  const executorAvailable = executorStatus.data?.available === true;

  const refresh = () => Promise.all([
    qc.invalidateQueries({ queryKey: ["migrations"] }),
    qc.invalidateQueries({ queryKey: ["recovery"] }),
  ]);

  const startRecoveryMut = useMutation({
    mutationFn: (id: string) => startRecoveryPlan(id),
    onSuccess: () => { toast({ tone: "success", title: "Recovery started" }); void refresh(); },
    onError: (error) => toast({ tone: "error", title: "Start failed", message: errorMessage(error) }),
  });

  const cancelRecoveryMut = useMutation({
    mutationFn: (id: string) => cancelRecoveryPlan(id),
    onSuccess: () => { toast({ tone: "success", title: "Recovery plan cancelled" }); void refresh(); },
    onError: (error) => toast({ tone: "error", title: "Cancel failed", message: errorMessage(error) }),
  });

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Migrations & Recovery"
        sub="View, create, and manage migration jobs and recovery plans."
        action={
          <div className="flex flex-wrap gap-2">
            <Btn onClick={() => setShowCreateMigration(true)}><Plus size={14} /> New Migration</Btn>
            <Btn onClick={() => setShowCreateRecovery(true)} tone="warning"><Plus size={14} /> New Recovery Plan</Btn>
          </div>
        }
      />

      {!executorAvailable && executorStatus.isSuccess && (
        <div className="rounded-lg border border-amber-700/30 bg-amber-900/10 p-3 text-sm text-amber-200">
          <strong>Planning-only mode:</strong> Workload transfer runtime is not available. Plans can be created but execution is disabled.
        </div>
      )}

      <div className="grid gap-5 xl:grid-cols-2">
        <Card>
          <CardHeader title={`Migrations (${(migrations.data ?? []).length})`} icon={ArrowRightLeft} />
          {migrations.isLoading ? (
            <div className="py-10 text-center text-sm text-slate-500">Loading migrations…</div>
          ) : migrations.isError ? (
            <div className="p-4">
              <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                <span>Could not load migrations: {migrations.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void migrations.refetch()}>Retry</Btn>
              </div>
            </div>
          ) : (migrations.data ?? []).length === 0 ? (
            <EmptyState icon={ArrowRightLeft} message="No migrations." />
          ) : (
            <div className="divide-y divide-white/[0.04]">
              {(migrations.data ?? []).map((migration) => (
                <div key={migration.id} className="flex items-center justify-between gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-xs">{migration.serverId}</p>
                    <p className="truncate text-xs text-slate-500">
                      {migration.sourceNodeId} → {migration.targetNodeId}
                      {migration.error || migration.failureReason ? ` · ${migration.error ?? migration.failureReason}` : ""}
                    </p>
                    {migration.progress != null && (
                      <div className="mt-1.5 h-1.5 w-32 overflow-hidden rounded-full bg-white/[0.06]">
                        <div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${migration.progress}%` }} />
                      </div>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {migration.progress != null && <span className="text-xs text-slate-500">{migration.progress}%</span>}
                    <Pill tone={statusTone(migration.status)}>{migration.status}</Pill>
                    <MigrationActions migration={migration} executorAvailable={executorAvailable} onAction={() => void refresh()} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title={`Recovery Plans (${(recoveries.data ?? []).length})`} icon={AlertTriangle} />
          {recoveries.isLoading ? (
            <div className="py-10 text-center text-sm text-slate-500">Loading recovery plans…</div>
          ) : recoveries.isError ? (
            <div className="p-4">
              <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                <span>Could not load recovery plans: {recoveries.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void recoveries.refetch()}>Retry</Btn>
              </div>
            </div>
          ) : (recoveries.data ?? []).length === 0 ? (
            <EmptyState icon={AlertTriangle} message="No recovery plans." />
          ) : (
            <div className="divide-y divide-white/[0.04]">
              {(recoveries.data ?? []).map((plan) => (
                <div key={plan.id}>
                  <div className="px-4 py-3">
                    <RecoveryPlanItem plan={plan} />
                    <div className="mt-2 flex gap-2">
                      {plan.status === "planned" && (
                        <Btn size="sm" tone="warning" onClick={() => setStartConfirmId(plan.id)} disabled={startRecoveryMut.isPending}>
                          {startRecoveryMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />} Start
                        </Btn>
                      )}
                      {!terminalStatuses.includes(plan.status) && (
                        <Btn size="sm" tone="danger" onClick={() => setCancelConfirmId(plan.id)} disabled={cancelRecoveryMut.isPending}>
                          {cancelRecoveryMut.isPending ? <Loader2 size={12} className="animate-spin" /> : <Ban size={12} />} Cancel
                        </Btn>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>

      <CreateMigrationModal open={showCreateMigration} onClose={() => setShowCreateMigration(false)} />
      <CreateRecoveryPlanModal open={showCreateRecovery} onClose={() => setShowCreateRecovery(false)} />

      <AdminConfirmDialog
        open={startConfirmId !== null}
        title="Start recovery?"
        description="Start this recovery plan? Workload data will be restored to destination nodes."
        confirmLabel="Start Recovery"
        onCancel={() => setStartConfirmId(null)}
        onConfirm={() => { const id = startConfirmId; setStartConfirmId(null); if (id) startRecoveryMut.mutate(id); }}
        loading={startRecoveryMut.isPending}
      />

      <AdminConfirmDialog
        open={cancelConfirmId !== null}
        title="Cancel recovery plan?"
        description="Cancel this recovery plan? In-progress operations will be stopped."
        confirmLabel="Cancel Plan"
        destructive
        onCancel={() => setCancelConfirmId(null)}
        onConfirm={() => { const id = cancelConfirmId; setCancelConfirmId(null); if (id) cancelRecoveryMut.mutate(id); }}
        loading={cancelRecoveryMut.isPending}
      />
    </AdminPageLayout>
  );
}
