"use client";

import { useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Droplets,
  Play,
  XCircle,
  CheckCircle2,
  CircleDashed,
  CircleSlash,
  LoaderCircle,
  ArrowRightLeft,
  Server,
} from "lucide-react";
import {
  AdminPageHeader,
  AdminPageLayout,
  AdminToolbar,
  AdminErrorState,
  AdminLoadingState,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  StatsRow,
  AdminSelect,
  AdminConfirmDialog,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { useToast } from "@/components/ui/toast";
import { formatDate } from "@/lib/utils";
import { cn } from "@/lib/utils";
import { fetchNodes } from "@/lib/api";
import {
  fetchDrainStates,
  beginDrain,
  cancelDrain,
  type DrainState,
} from "@/lib/api/drain";

const DRAIN_STEPS: { key: string; label: string }[] = [
  { key: "traffic-withdrawal", label: "Withdraw traffic" },
  { key: "evacuation-plan", label: "Plan evacuation" },
  { key: "migrate-servers", label: "Migrate servers" },
  { key: "complete", label: "Complete" },
];

function statusMeta(status: string): { tone: "green" | "red" | "yellow" | "blue" | "neutral"; label: string; icon: typeof CheckCircle2 } {
  switch (status) {
    case "draining":
      return { tone: "yellow", label: "Draining", icon: LoaderCircle };
    case "drained":
      return { tone: "green", label: "Drained", icon: CheckCircle2 };
    case "failed":
      return { tone: "red", label: "Failed", icon: CircleSlash };
    case "cancelled":
      return { tone: "neutral", label: "Cancelled", icon: XCircle };
    default:
      return { tone: "neutral", label: status || "Unknown", icon: CircleDashed };
  }
}

function stepState(stepName: string, current: string, remaining: number, overallStatus: string): "done" | "active" | "pending" {
  if (overallStatus === "drained" || overallStatus === "cancelled") return "done";
  const order = DRAIN_STEPS.map((s) => s.key);
  const curIdx = order.indexOf(current);
  const myIdx = order.indexOf(stepName);
  if (curIdx === -1 || myIdx === -1) {
    return overallStatus === "failed" ? "pending" : "pending";
  }
  if (myIdx < curIdx) return "done";
  if (myIdx === curIdx) return remaining > 0 || overallStatus === "draining" ? "active" : "done";
  return "pending";
}

function ProgressStepper({ state }: { state: DrainState }) {
  const current = state.progress?.current ?? "";
  const remaining = state.progress?.remaining ?? 0;
  return (
    <div className="flex items-center">
      {DRAIN_STEPS.map((step, i) => {
        const st = stepState(step.key, current, remaining, state.status);
        return (
          <div key={step.key} className="flex flex-1 items-center last:flex-none">
            <div className="flex flex-col items-center gap-1.5">
              <span
                className={cn(
                  "grid h-7 w-7 place-items-center rounded-full border text-[11px] font-semibold transition",
                  st === "done" && "border-emerald-400/40 bg-emerald-500/15 text-emerald-300",
                  st === "active" && "border-amber-400/50 bg-amber-500/15 text-amber-200 shadow-[0_0_0_4px_rgba(245,158,11,0.08)]",
                  st === "pending" && "border-white/10 bg-white/[0.02] text-slate-500",
                )}
                title={step.label}
              >
                {st === "done" ? <CheckCircle2 size={14} /> : st === "active" ? <LoaderCircle size={14} className="animate-spin" /> : i + 1}
              </span>
              <span className={cn("max-w-[84px] text-center text-[10px] leading-tight", st === "pending" ? "text-slate-600" : "text-slate-400")}>
                {step.label}
              </span>
            </div>
            {i < DRAIN_STEPS.length - 1 && (
              <span
                className={cn(
                  "mx-1 mb-4 h-0.5 flex-1 rounded-full",
                  st === "done" ? "bg-emerald-400/30" : "bg-white/[0.06]",
                )}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}

export function AdminDrain() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [selectedNode, setSelectedNode] = useState<string>("");
  const [confirm, setConfirm] = useState<{ open: boolean; nodeId: string; name: string }>({ open: false, nodeId: "", name: "" });

  const statesQ = useQuery({
    queryKey: ["drain-ledger"],
    queryFn: () => fetchDrainStates(),
    refetchInterval: (query) => (query.state.data?.some((s) => s.status === "draining") ? 5_000 : false),
  });

  const nodesQ = useQuery({
    queryKey: ["drain-nodes"],
    queryFn: () => fetchNodes(),
  });

  const beginMutation = useMutation({
    mutationFn: (nodeId: string) => beginDrain(nodeId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["drain-ledger"] });
      qc.invalidateQueries({ queryKey: ["drain-nodes"] });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to start drain", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const cancelMutation = useMutation({
    mutationFn: (nodeId: string) => cancelDrain(nodeId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["drain-ledger"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to cancel drain", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const states = useMemo(() => statesQ.data ?? [], [statesQ.data]);
  const nodes = useMemo(() => nodesQ.data ?? [], [nodesQ.data]);

  const drainingNow = states.filter((s) => s.status === "draining").length;
  const drainedTotal = states.filter((s) => s.status === "drained").length;
  const failedTotal = states.filter((s) => s.status === "failed" || s.status === "cancelled").length;

  // Only offer nodes not already draining and that are otherwise online-ish.
  const drainableNodes = nodes
    .filter((n) => !n.draining && n.status !== "offline")
    .map((n) => ({ value: n.id, label: `${n.name || n.id} · ${n.region || "no region"}` }));

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminPageHeader
        title="Drain Ledger"
        description="Durable, restart-safe record of every node drain. Drain orchestration (gateway withdrawal → evacuation → migration) is owned by cluster membership; this ledger mirrors those lifecycle events so you can watch progress and history at a glance."
      />

      <StatsRow
        items={[
          { label: "Recorded drains", value: states.length, icon: ArrowRightLeft },
          { label: "Active now", value: drainingNow, icon: LoaderCircle, tone: drainingNow ? "yellow" : "neutral" },
          { label: "Drained", value: drainedTotal, icon: CheckCircle2, tone: "green" },
          { label: "Failed / cancelled", value: failedTotal, icon: CircleSlash, tone: failedTotal ? "red" : "neutral" },
        ]}
      />

      <Card>
        <CardHeader title="Start a drain" icon={Droplets} />
        <AdminToolbar>
          <div className="min-w-[280px] flex-1">
            <AdminSelect
              label="Node"
              value={selectedNode}
              onChange={setSelectedNode}
              options={drainableNodes}
              placeholder="Select a node to drain…"
            />
          </div>
          <Btn
            tone="primary"
            disabled={!selectedNode || beginMutation.isPending}
            onClick={() => {
              const node = nodes.find((n) => n.id === selectedNode);
              setConfirm({ open: true, nodeId: selectedNode, name: node?.name || selectedNode });
            }}
          >
            <Play size={14} /> Drain node
          </Btn>
        </AdminToolbar>
        <p className="px-4 pb-4 text-xs leading-5 text-slate-500">
          Draining withdraws the node from traffic and migrates its workloads to other nodes.
          {drainableNodes.length === 0 && nodes.length > 0 ? " All nodes are already draining or offline." : ""}
        </p>
      </Card>

      {statesQ.isLoading ? (
        <AdminLoadingState label="Loading drain ledger…" />
      ) : statesQ.isError ? (
        <AdminErrorState
          message={statesQ.error instanceof Error ? statesQ.error.message : "Failed to load the drain ledger"}
          retry={() => void statesQ.refetch()}
        />
      ) : states.length === 0 ? (
        <EmptyState icon={Droplets} title="No drains recorded yet" message="When a node begins draining it will appear here with live step-by-step progress." />
      ) : (
        <div className="grid gap-3">
          {states.map((s) => {
            const meta = statusMeta(s.status);
            const Icon = meta.icon;
            const isActive = s.status === "draining";
            return (
              <Card key={s.nodeId} className="overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/[0.06] px-4 py-3">
                  <div className="flex items-center gap-3">
                    <span className={cn("grid h-9 w-9 place-items-center rounded-xl border", isActive ? "border-amber-400/30 bg-amber-500/10 text-amber-200" : "border-white/[0.06] bg-white/[0.02] text-slate-400")}>
                      <Server size={16} />
                    </span>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-xs text-slate-200">{s.nodeId}</span>
                        <Pill tone={meta.tone}>
                          <Icon size={12} className={isActive ? "animate-spin" : undefined} /> {meta.label}
                        </Pill>
                        {s.desiredFinal && <Pill tone="blue">terminal</Pill>}
                      </div>
                      <div className="mt-0.5 text-[11px] text-slate-500">
                        started {formatDate(s.startedAt)} · updated {formatDate(s.updatedAt)}
                        {s.planId ? ` · plan ${s.planId.slice(0, 8)}` : ""}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <div className="text-right text-[11px] text-slate-500">
                      <div className="text-slate-300">{s.progress?.remaining ?? 0} remaining</div>
                      <div>of {s.progress?.total ?? 0} workloads</div>
                    </div>
                    {isActive && (
                      <Btn
                        size="sm"
                        tone="danger"
                        disabled={cancelMutation.isPending}
                        onClick={() => cancelMutation.mutate(s.nodeId)}
                      >
                        <XCircle size={12} /> Cancel
                      </Btn>
                    )}
                  </div>
                </div>
                <div className="px-5 py-4">
                  <ProgressStepper state={s} />
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {confirm.open && (
        <AdminConfirmDialog
          title="Drain node?"
          description={`This withdraws ${confirm.name} from traffic and evacuates its workloads to other nodes. The drain will be tracked in the ledger below.`}
          confirmLabel="Drain node"
          destructive
          loading={beginMutation.isPending}
          onCancel={() => setConfirm({ open: false, nodeId: "", name: "" })}
          onConfirm={() => {
            beginMutation.mutate(confirm.nodeId, {
              onSettled: () => setConfirm({ open: false, nodeId: "", name: "" }),
            });
          }}
        />
      )}
    </AdminPageLayout>
  );
}
