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
  Ban,
  HelpCircle,
  Server,
  type LucideIcon,
} from "lucide-react";
import {
  AdminPageHeader,
  AdminPageLayout,
  AdminToolbar,
  AdminErrorState,
  AdminLoadingState,
  AdminConfirmDialog,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  StatsRow,
  AdminSelect,
} from "@/components/admin/admin-ui";
import type { AdminTone } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { OfflineBanner } from "@/components/shared/states-offline";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { formatDate } from "@/lib/utils";
import { cn } from "@/lib/utils";
import { fetchNodes } from "@/lib/api";
import {
  fetchDrainStates,
  beginDrain,
  cancelDrain,
  type DrainProgressStep,
  type DrainState,
} from "@/lib/api/drain";

const DRAIN_STEPS: { key: string; label: string }[] = [
  { key: "traffic-withdrawal", label: "Withdraw traffic" },
  { key: "evacuation-plan", label: "Plan evacuation" },
  { key: "migrate-servers", label: "Migrate servers" },
  { key: "complete", label: "Complete" },
];

const TERMINAL_STATUSES = ["drained", "cancelled", "failed"];

function statusMeta(status: string): { tone: "green" | "red" | "yellow" | "blue" | "neutral" | "unknown"; label: string; icon: typeof CheckCircle2 } {
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
      return { tone: "unknown", label: status || "Unknown", icon: CircleDashed };
  }
}

/**
 * How one step is drawn — straight from the backend's per-step state.
 *
 * `DrainProgressStep.state` (`lib/api/drain.ts:6-10`) is the authoritative
 * per-step verdict, so it is what gets rendered. Two things this replaces:
 *
 *  - A **cancelled** drain used to force every step to `done`, so an aborted
 *    evacuation showed four green checks on the graphic operators actually scan.
 *  - Deriving order from `progress.current` painted a *failed* drain's completed
 *    steps as pending.
 *
 * A step the ledger never reported resolves to `unknown` (dashed, "?"), which is
 * a different fact from `pending` ("has not run yet") and from `not-run` ("this
 * will never run — the drain ended before reaching it").
 */
type StepVisual = "done" | "active" | "pending" | "failed" | "not-run" | "unknown";

function stepVisualFor(step: DrainProgressStep | undefined, overallStatus: string): StepVisual {
  if (!step || !step.state) return "unknown";
  const reported = step.state.trim().toLowerCase();
  if (TERMINAL_STATUSES.includes(overallStatus) && reported === "pending") return "not-run";
  switch (reported) {
    case "done":
    case "completed":
    case "succeeded":
      return "done";
    case "active":
    case "running":
    case "in_progress":
    case "pending-approval":
      return "active";
    case "failed":
    case "error":
      return "failed";
    case "pending":
      return "pending";
    default:
      // A state this UI does not know. Guessing would be a lie.
      return "unknown";
  }
}

const VISUAL_STYLES: Record<StepVisual, { ring: string; label: string; connector: string }> = {
  done: { ring: "border-ok-line bg-ok-subtle text-ok", label: "text-text-subtle", connector: "bg-ok-line" },
  active: { ring: "border-warn-line bg-warn-subtle text-warn", label: "text-text", connector: "bg-warn-line" },
  pending: { ring: "border-line bg-overlay-subtle text-text-muted", label: "text-text-muted", connector: "bg-line" },
  failed: { ring: "border-danger-line bg-danger-subtle text-danger", label: "text-danger", connector: "bg-danger-line" },
  "not-run": { ring: "border-dashed border-line-strong text-text-muted", label: "text-text-muted", connector: "bg-line" },
  unknown: { ring: "border-dashed border-unknown-line bg-unknown-subtle text-unknown", label: "text-unknown", connector: "border-dashed border-unknown-line bg-transparent" },
};

const VISUAL_WORD: Record<StepVisual, string> = {
  done: "done",
  active: "in progress",
  pending: "not started",
  failed: "failed",
  "not-run": "not run",
  unknown: "state not reported",
};

function ProgressStepper({ state }: { state: DrainState }) {
  const reported = state.progress?.steps;
  // The ledger's own step list wins when it exists; the DRAIN_STEPS labels only
  // supply naming and order for a backend that reports nothing.
  const rows = DRAIN_STEPS.map((step, i) => {
    const match = reported?.find((s) => s.name === step.key) ?? reported?.[i];
    return { label: step.label, step: match };
  });

  return (
    <div>
      <div className="flex items-start">
        {rows.map((row, i) => {
          const st = stepVisualFor(row.step, state.status);
          const styles = VISUAL_STYLES[st];
          return (
            <div className="flex flex-1 items-start last:flex-none" key={row.label}>
              <div className="flex flex-col items-center gap-1.5">
                <span
                  aria-hidden="true"
                  className={cn("grid h-7 w-7 place-items-center rounded-full border text-eyebrow font-semibold transition", styles.ring)}
                >
                  {st === "done" ? <CheckCircle2 size={14} />
                    : st === "active" ? <LoaderCircle size={14} className="animate-spin" />
                    : st === "failed" ? <CircleSlash size={14} />
                    : st === "not-run" ? <Ban size={14} />
                    : <HelpCircle size={14} />}
                </span>
                <span className={cn("max-w-[104px] text-center text-eyebrow leading-tight", styles.label)}>
                  {row.label}
                </span>
                <span className="text-eyebrow text-text-muted">
                  {VISUAL_WORD[st]}
                  {row.step?.detail ? ` · ${row.step.detail}` : ""}
                </span>
              </div>
              {i < rows.length - 1 && (
                <span
                  aria-hidden="true"
                  className={cn("mt-3.5 mx-1 h-0.5 flex-1 rounded-full", VISUAL_STYLES[st].connector)}
                />
              )}
            </div>
          );
        })}
      </div>
      {!reported || reported.length === 0 ? (
        <p className="mt-2 text-eyebrow text-unknown">
          The ledger reported no per-step states for this drain, so the progress graphic is unknown — not empty.
        </p>
      ) : null}
    </div>
  );
}

export function AdminDrain() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [selectedNode, setSelectedNode] = useState<string>("");
  const [drainTarget, setDrainTarget] = useState<{ open: boolean; nodeId: string; name: string }>({ open: false, nodeId: "", name: "" });

  const statesQ = useQuery({
    queryKey: ["drain-ledger"],
    queryFn: () => fetchDrainStates(),
    retry: false,
    refetchInterval: (query) => (query.state.data?.some((s) => s.status === "draining") ? 5_000 : false),
  });

  const nodesQ = useQuery({
    queryKey: ["drain-nodes"],
    queryFn: () => fetchNodes(),
    retry: false,
  });

  const beginMutation = useMutation({
    mutationFn: (nodeId: string) => beginDrain(nodeId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["drain-ledger"] });
      qc.invalidateQueries({ queryKey: ["drain-nodes"] });
      toast({ tone: "success", title: "Drain started", message: "Progress appears in the ledger as lifecycle events arrive." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to start drain", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const cancelMutation = useMutation({
    mutationFn: (nodeId: string) => cancelDrain(nodeId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["drain-ledger"] });
      qc.invalidateQueries({ queryKey: ["drain-nodes"] });
      toast({ tone: "success", title: "Drain cancellation requested", message: "Workloads already migrated are not moved back automatically." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to cancel drain", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const states = useMemo(() => statesQ.data ?? [], [statesQ.data]);
  const nodes = useMemo(() => nodesQ.data ?? [], [nodesQ.data]);
  const ledgerReady = !statesQ.isPending && !statesQ.isError;

  // Counts over the ledger. Meaningless until the read has actually landed, which
  // is what `ledgerReady` below gates on.
  const drainingNow = states.filter((s) => s.status === "draining").length;
  const drainedTotal = states.filter((s) => s.status === "drained").length;
  const failedTotal = states.filter((s) => s.status === "failed" || s.status === "cancelled").length;

  // Only offer nodes not already draining and that are otherwise online-ish.
  const drainableNodes = nodes
    .filter((n) => !n.draining && n.status !== "offline")
    .map((n) => ({ value: n.id, label: `${n.name || n.id} · ${n.region || "no region"}` }));

  // A count of an unreadable ledger is not zero. Every stat below is gated on
  // the ledger read landing, and shows `—` until it does.
  const stats: Array<{ label: string; value: string | number; icon: LucideIcon; tone?: AdminTone }> = ledgerReady
    ? [
        { label: "Recorded drains", value: states.length, icon: ArrowRightLeft },
        { label: "Active now", value: drainingNow, icon: LoaderCircle, tone: drainingNow ? "yellow" : "neutral" },
        { label: "Drained", value: drainedTotal, icon: CheckCircle2, tone: "neutral" },
        { label: "Failed / cancelled", value: failedTotal, icon: CircleSlash, tone: failedTotal ? "red" : "neutral" },
      ]
    : [
        { label: "Recorded drains", value: "—", icon: ArrowRightLeft },
        { label: "Active now", value: "—", icon: LoaderCircle },
        { label: "Drained", value: "—", icon: CheckCircle2 },
        { label: "Failed / cancelled", value: "—", icon: CircleSlash },
      ];

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { void statesQ.refetch(); void nodesQ.refetch(); }} />
      <AdminPageHeader
        description="Durable, restart-safe record of every node drain. Drain orchestration (gateway withdrawal → evacuation → migration) is owned by cluster membership; this ledger mirrors those lifecycle events so you can watch progress and history at a glance."
        status={<FreshnessBadge state={sourceState(statesQ, 5_000)} />}
      />

      <StatsRow items={stats} />

      <Card>
        <CardHeader title="Start a drain" icon={Droplets} />
        {nodesQ.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading nodes…" /></div>
        ) : nodesQ.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`The node list could not be loaded, so no node can be selected: ${nodesQ.error instanceof Error ? nodesQ.error.message : "request failed"}. An empty picker here is not an empty fleet.`}
              retry={() => void nodesQ.refetch()}
            />
          </div>
        ) : (
          <>
            <AdminToolbar>
              <div className="min-w-[280px] flex-1">
                <AdminSelect
                  label="Node"
                  value={selectedNode}
                  onChange={setSelectedNode}
                  options={drainableNodes}
                  placeholder={nodes.length === 0 ? "No nodes reported" : "Select a node to drain…"}
                />
              </div>
              <Btn
                tone="primary"
                disabled={!selectedNode || beginMutation.isPending}
                onClick={() => {
                  const node = nodes.find((n) => n.id === selectedNode);
                  setDrainTarget({ open: true, nodeId: selectedNode, name: node?.name || selectedNode });
                }}
              >
                <Play size={14} /> Drain node
              </Btn>
            </AdminToolbar>
            <p className="px-4 pb-4 text-meta leading-5 text-text-subtle">
              Draining withdraws the node from traffic and migrates its workloads to other nodes.
              {drainableNodes.length === 0 && nodes.length > 0
                ? " No node is currently eligible: all reported nodes are already draining or offline."
                : ""}
            </p>
          </>
        )}
      </Card>

      {statesQ.isLoading ? (
        <AdminLoadingState label="Loading drain ledger…" />
      ) : statesQ.isError ? (
        <AdminErrorState
          message={statesQ.error instanceof Error ? statesQ.error.message : "Failed to load the drain ledger"}
          retry={() => void statesQ.refetch()}
        />
      ) : states.length === 0 ? (
        <EmptyState icon={Droplets} title="No drains recorded yet" message="This ledger is empty because no drain has been started. When a node begins draining it appears here with per-step progress." />
      ) : (
        <div className="grid gap-3">
          {states.map((s) => {
            const meta = statusMeta(s.status);
            const Icon = meta.icon;
            const isActive = s.status === "draining";
            const nodeName = nodes.find((n) => n.id === s.nodeId)?.name || s.nodeId;
            const progress = s.progress;
            const remaining = typeof progress?.remaining === "number" ? progress.remaining : undefined;
            const total = typeof progress?.total === "number" ? progress.total : undefined;
            return (
              <Card key={s.nodeId} className="overflow-hidden">
                <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
                  <div className="flex items-center gap-3">
                    <span className={cn("grid h-9 w-9 place-items-center rounded-xl border", isActive ? "border-warn-line bg-warn-subtle text-warn" : "border-line bg-overlay-subtle text-text-subtle")}>
                      <Server size={16} />
                    </span>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-xs text-text">{s.nodeId}</span>
                        <Pill tone={meta.tone}>
                          <Icon size={12} className={isActive ? "animate-spin" : undefined} /> {meta.label}
                        </Pill>
                        {s.desiredFinal && <Pill tone="blue">terminal</Pill>}
                      </div>
                      <div className="mt-0.5 text-eyebrow text-text-muted">
                        started {formatDate(s.startedAt, "not reported")} · updated {formatDate(s.updatedAt, "not reported")}
                        {s.planId ? ` · plan ${s.planId.slice(0, 8)}` : " · no evacuation plan recorded"}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <div className="text-right text-eyebrow text-text-muted">
                      {remaining === undefined || total === undefined ? (
                        <>
                          <div className="text-unknown">Progress not reported</div>
                          <div className="text-unknown">
                            {remaining === undefined ? "remaining unknown" : `${remaining} remaining`}
                            {" · "}
                            {total === undefined ? "total unknown" : `of ${total} workloads`}
                          </div>
                        </>
                      ) : (
                        <>
                          <div className="text-text">{remaining} remaining</div>
                          <div>of {total} workloads</div>
                        </>
                      )}
                    </div>
                    {isActive && (
                      <Btn
                        size="sm"
                        tone="danger"
                        disabled={cancelMutation.isPending}
                        onClick={() => {
                          void (async () => {
                            // Cancelling mid-evacuation leaves workloads on a node that
                            // was pulled from traffic, so it carries the same weight as
                            // starting the drain and is confirmed with the blast radius.
                            const ok = await confirm({
                              title: `Cancel the drain of ${nodeName}?`,
                              description:
                                `Withdrawal and migration stop where they are. Workloads already migrated to other nodes are not moved back, and this node may still be out of traffic. ` +
                                `${typeof remaining === "number" ? `${remaining} of ${typeof total === "number" ? total : "an unknown number of"} workloads were still pending when you asked.` : "The ledger has not reported how many workloads were pending."}`,
                              confirmLabel: "Cancel drain",
                              danger: true,
                            });
                            if (ok) cancelMutation.mutate(s.nodeId);
                          })();
                        }}
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

      {drainTarget.open && (
        <AdminConfirmDialog
          title="Drain node?"
          description={`This withdraws ${drainTarget.name} from traffic and evacuates its workloads to other nodes. The drain is tracked in the ledger below and must be cancelled explicitly to stop.`}
          confirmLabel="Drain node"
          destructive
          loading={beginMutation.isPending}
          onCancel={() => setDrainTarget({ open: false, nodeId: "", name: "" })}
          onConfirm={() => {
            beginMutation.mutate(drainTarget.nodeId, {
              onSettled: () => setDrainTarget({ open: false, nodeId: "", name: "" }),
            });
          }}
        />
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
