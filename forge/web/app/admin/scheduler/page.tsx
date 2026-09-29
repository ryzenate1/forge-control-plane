"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import {
  Activity, BarChart3, Cpu, GanttChart, HardDrive, Network, Plus, Trash2, Zap, Server, Settings2,
} from "lucide-react";
import { fetchJSON, postJSON, putJSON, deleteJSON } from "@/lib/api";
import {AdminPageHeader, AdminPageLayout, AdminTabs, AdminLoadingState, AdminSelect, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, cn, AdminErrorState} from "@/components/admin/admin-ui";
import type { AdminTone } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { OfflineBanner } from "@/components/shared/states-offline";
import { useConfirm } from "@/components/ui/confirm-dialog";

// Backend contract types
//
// Mirrors scheduler.PredictiveScore in
// forge/api/internal/services/scheduler/predictive.go, where every field is a
// plain float64 with no `omitempty` — the encoder always emits all of them.
// There is therefore no "field absent" state to defend against on this
// endpoint, and a `?? 0` fallback could only invent a score the API never
// withholds. Uncertainty is carried by `confidence`, not by missing numbers.
//
// The previously declared `score`, `cpuLoad`, `memoryUsage`, `diskUsage`,
// `networkLoad` and `activeServers` fields were never part of that response;
// they only made dead fallbacks look load-bearing.
type PredictiveScore = {
  nodeId: string;
  baseScore: number;
  trendScore: number;
  affinityScore: number;
  antiAffinityScore: number;
  totalScore: number;
  predictedLoad: number;
  confidence: number;
};

type AffinityRule = {
  id: string;
  name?: string;
  serverId?: string;
  nodeId?: string;
  label: string;
  weight?: number;
  type?: string;
  scope?: string;
  targetIds?: string[];
  enabled?: boolean;
  createdAt?: string;
};

type AntiAffinityRule = {
  id: string;
  name?: string;
  serverId?: string;
  label: string;
  scope?: string;
  weight?: number;
};

type ConstraintBackend = {
  type: "required" | "preferred" | "forbidden";
  key: string;
  operator: string;
  value: string;
};

type BackendInfo = {
  type: string;
  name: string;
  description: string;
};

type ResourceMetric = {
  timestamp?: string;
  cpuPercent: number;
  memoryUsedMb: number;
  diskUsedMb: number;
  networkRx?: number;
  networkTx?: number;
  serverCount?: number;
};

const defaultAffinityForm = {
  type: "affinity" as "affinity" | "anti_affinity",
  label: "",
  name: "",
  serverId: "",
  nodeId: "",
  scope: "node" as string,
  weight: 1,
  enabled: true,
};

const defaultConstraintBackendForm = {
  type: "required" as ConstraintBackend["type"],
  key: "region" as string,
  operator: "eq" as string,
  value: "",
};

/**
 * The ingest form holds **strings**, empty until the operator types them.
 *
 * `scheduler.ResourceMetric` (`predictive.go:23-31`) is a struct of bare numbers
 * with no `omitempty` and no pointers, so a field the UI omits lands as `0` in the
 * prediction store — the backend cannot tell "not reported" from "idle". Keeping
 * these empty-by-default means the page can refuse to submit instead of silently
 * forging a zero-load sample that skews placement scoring fleet-wide.
 */
const METRIC_FIELDS: { key: keyof ResourceMetric; label: string; hint: string }[] = [
  { key: "cpuPercent", label: "CPU %", hint: "0–100" },
  { key: "memoryUsedMb", label: "Memory used (MB)", hint: "mebibytes in use" },
  { key: "diskUsedMb", label: "Disk used (MB)", hint: "mebibytes in use" },
  { key: "serverCount", label: "Active servers", hint: "count on this node" },
  { key: "networkRx", label: "Network RX", hint: "bytes received" },
  { key: "networkTx", label: "Network TX", hint: "bytes sent" },
];

const emptyMetricForm: Record<string, string> = {
  cpuPercent: "",
  memoryUsedMb: "",
  diskUsedMb: "",
  serverCount: "",
  networkRx: "",
  networkTx: "",
};

export default function AdminSchedulerPage() {
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [tab, setTab] = useState<"scores" | "affinity" | "constraints">("scores");
  const [showCreateAffinity, setShowCreateAffinity] = useState(false);
  const [affinityForm, setAffinityForm] = useState(defaultAffinityForm);
  const [showCreateConstraint, setShowCreateConstraint] = useState(false);
  const [constraintFormBackend, setConstraintFormBackend] = useState(defaultConstraintBackendForm);
  const [lookupNodeId, setLookupNodeId] = useState("");
  const [lookupResult, setLookupResult] = useState<PredictiveScore | null>(null);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [metricsNodeId, setMetricsNodeId] = useState("");
  const [metricsForm, setMetricsForm] = useState<Record<string, string>>(emptyMetricForm);
  const [schedulerNodeId, setSchedulerNodeId] = useState("");
  const [schedulerType, setSchedulerType] = useState("");
  const [schedulerConfigJson, setSchedulerConfigJson] = useState("{}");

  const scoresQuery = useQuery({
    queryKey: ["admin", "scheduler", "scores"],
    queryFn: () => fetchJSON<{ data: PredictiveScore[] }>("/admin/scheduler/predictive/scores").then(r => r.data),
  });

  const affinityQuery = useQuery({
    queryKey: ["admin", "scheduler", "affinity"],
    queryFn: () => fetchJSON<{ data: AffinityRule[] }>("/admin/scheduler/predictive/affinity-rules").then(r => r.data),
  });

  const antiAffinityQuery = useQuery({
    queryKey: ["admin", "scheduler", "anti-affinity"],
    queryFn: () => fetchJSON<{ data: AntiAffinityRule[] }>("/admin/scheduler/predictive/anti-affinity-rules").then(r => r.data),
  });

  const constraintsQuery = useQuery({
    queryKey: ["admin", "scheduler", "constraints"],
    queryFn: () => fetchJSON<{ data: ConstraintBackend[] }>("/admin/scheduler/constraints").then(r => r.data),
  });

  const backendsQuery = useQuery({
    queryKey: ["admin", "scheduler", "backends"],
    queryFn: () => fetchJSON<{ data: BackendInfo[] }>("/admin/scheduler/backends").then(r => r.data),
  });

  const scores = useMemo(() => scoresQuery.data ?? [], [scoresQuery.data]);
  const affinityRules = useMemo(() => affinityQuery.data ?? [], [affinityQuery.data]);
  const antiAffinityRules = useMemo(() => antiAffinityQuery.data ?? [], [antiAffinityQuery.data]);
  const constraints = useMemo(() => constraintsQuery.data ?? [], [constraintsQuery.data]);
  const backends = useMemo(() => backendsQuery.data ?? [], [backendsQuery.data]);

  const createAffinityMutation = useMutation({
    mutationFn: () => {
      if (affinityForm.type === "anti_affinity") {
        const payload: Record<string, unknown> = {
          name: affinityForm.name || affinityForm.label,
          label: affinityForm.label,
          serverId: affinityForm.serverId || undefined,
          scope: affinityForm.scope || "node",
          weight: Number(affinityForm.weight) || 1,
        };
        return postJSON("/admin/scheduler/predictive/anti-affinity-rules", payload);
      }
      const payload: Record<string, unknown> = {
        name: affinityForm.name || affinityForm.label,
        label: affinityForm.label,
        serverId: affinityForm.serverId || undefined,
        nodeId: affinityForm.nodeId || undefined,
        weight: Number(affinityForm.weight) || 1,
      };
      return postJSON("/admin/scheduler/predictive/affinity-rules", payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "affinity"] });
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "anti-affinity"] });
      setShowCreateAffinity(false);
      setAffinityForm(defaultAffinityForm);
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create affinity rule", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteAffinityMutation = useMutation({
    mutationFn: ({ kind, id }: { kind: "affinity" | "anti-affinity"; id: string }) =>
      deleteJSON(`/admin/scheduler/predictive/${kind === "affinity" ? "affinity-rules" : "anti-affinity-rules"}/${encodeURIComponent(id)}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "affinity"] });
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "anti-affinity"] });
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "scores"] });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to delete rule", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteRule = async (kind: "affinity" | "anti-affinity", id: string, label: string) => {
    const confirmed = await confirm({
      title: `Delete this ${kind} rule?`,
      description: `The scheduler will stop applying “${label}”. This cannot be undone.`,
      danger: true,
      confirmLabel: "Delete",
    });
    if (confirmed) deleteAffinityMutation.mutate({ kind, id });
  };

  const createConstraintMutation = useMutation({
    mutationFn: async () => {
      const existing = constraints ?? [];
      const key = constraintFormBackend.key.trim();
      if (!key) throw new Error("Key is required");
      const newConstraint: ConstraintBackend = {
        type: constraintFormBackend.type,
        key,
        operator: constraintFormBackend.operator,
        value: constraintFormBackend.value,
      };
      return putJSON("/admin/scheduler/constraints", [...existing, newConstraint]);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "constraints"] });
      setShowCreateConstraint(false);
      setConstraintFormBackend(defaultConstraintBackendForm);
    },
  });

  const deleteConstraintMutation = useMutation({
    mutationFn: async (target: { index: number; id?: string }) => {
      const existing = constraints ?? [];
      if (target.index >= 0 && target.index < existing.length) {
        const next = existing.filter((_, i) => i !== target.index);
        try {
          return await putJSON("/admin/scheduler/constraints", next);
        } catch {
          return deleteJSON(`/admin/scheduler/constraints/${encodeURIComponent(String(target.index))}`);
        }
      }
      if (target.id) {
        return deleteJSON(`/admin/scheduler/constraints/${encodeURIComponent(target.id)}`);
      }
      throw new Error("No constraint target");
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "constraints"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to delete constraint", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  /**
   * Every field must be typed and numeric. `Number("") || 0` used to turn an
   * untouched field into a reported zero, which is a fabricated reading rather
   * than a missing one — so an incomplete form is rejected here.
   */
  const metricReadiness = useMemo(() => {
    const missing = METRIC_FIELDS.filter((f) => metricsForm[f.key]?.trim() === "").map((f) => f.label);
    const nonNumeric = METRIC_FIELDS.filter((f) => {
      const raw = metricsForm[f.key]?.trim();
      return raw !== "" && !Number.isFinite(Number(raw));
    }).map((f) => f.label);
    return { missing, nonNumeric, ready: missing.length === 0 && nonNumeric.length === 0 && metricsNodeId.trim() !== "" };
  }, [metricsForm, metricsNodeId]);

  const ingestMetricsMutation = useMutation({
    mutationFn: () => {
      if (!metricsNodeId.trim()) throw new Error("Node ID is required");
      if (metricReadiness.missing.length > 0) {
        throw new Error(`Not sent: ${metricReadiness.missing.join(", ")} were left blank, and a blank field would be stored as a measured 0.`);
      }
      if (metricReadiness.nonNumeric.length > 0) {
        throw new Error(`Not sent: ${metricReadiness.nonNumeric.join(", ")} must be a number.`);
      }
      const payload: ResourceMetric = {
        timestamp: new Date().toISOString(),
        cpuPercent: Number(metricsForm.cpuPercent),
        memoryUsedMb: Number(metricsForm.memoryUsedMb),
        diskUsedMb: Number(metricsForm.diskUsedMb),
        networkRx: Number(metricsForm.networkRx),
        networkTx: Number(metricsForm.networkTx),
        serverCount: Number(metricsForm.serverCount),
      };
      return postJSON(`/admin/scheduler/predictive/metrics/${encodeURIComponent(metricsNodeId.trim())}`, payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "scores"] });
      toast({ tone: "success", title: "Sample recorded", message: `Node ${metricsNodeId.trim()}'s predicted load now reflects the values you entered.` });
    },
    onError: (err) => toast({ tone: "error", title: "Metric not ingested", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const confirmIngestMetrics = () => {
    void (async () => {
      const ok = await confirm({
        title: "Write a manual metric sample?",
        description:
          `This is a test hook, not a telemetry read. The six values you typed are stored as node ${metricsNodeId.trim()}'s latest resource sample and feed its predictive score, which changes where the scheduler places work across the fleet. ` +
          `Beacon reports these values itself under Nodes — use that instead unless you are deliberately probing the scorer.`,
        confirmLabel: "Record sample",
        danger: true,
      });
      if (ok) ingestMetricsMutation.mutate();
    })();
  };

  const lookupPerNodeScore = async () => {
    setLookupError(null);
    setLookupResult(null);
    if (!lookupNodeId.trim()) { setLookupError("Node ID required"); return; }
    try {
      const res = await fetchJSON<{ data: PredictiveScore }>(`/admin/scheduler/predictive/nodes/${encodeURIComponent(lookupNodeId.trim())}/score`);
      setLookupResult(res.data);
    } catch (e) {
      setLookupError(e instanceof Error ? e.message : "Failed to fetch score");
    }
  };

  const updateNodeSchedulerMutation = useMutation({
    mutationFn: () => {
      if (!schedulerNodeId.trim()) throw new Error("Node ID required");
      let config: unknown = undefined;
      if (schedulerConfigJson.trim()) {
        try { config = JSON.parse(schedulerConfigJson); } catch { throw new Error("schedulerConfig must be valid JSON"); }
      }
      return putJSON(`/admin/scheduler/nodes/${encodeURIComponent(schedulerNodeId.trim())}/scheduler`, {
        schedulerType: schedulerType,
        schedulerConfig: config,
      });
    },
  });

  const maxScore = Math.max(...scores.map((s) => s.totalScore), 1);

  /**
   * Pinning is only meaningful against a runtime the control plane reports.
   * Unknown (loading or failed) and empty both disable the control with a reason,
   * instead of offering docker/k3s/nomad regardless of what this deployment has.
   */
  const backendsReady = !backendsQuery.isPending && !backendsQuery.isError && backends.length > 0;

  const confirmPinScheduler = () => {
    void (async () => {
      const ok = await confirm({
        title: `Pin node ${schedulerNodeId.trim()} to ${schedulerType}?`,
        description: "New workloads on this node are scheduled through the chosen runtime. Workloads already running are not moved or restarted by this change.",
        confirmLabel: "Pin runtime",
        danger: true,
      });
      if (ok) updateNodeSchedulerMutation.mutate();
    })();
  };

  return (
    <AdminPageLayout>
      <AdminPageHeader
        description="Predictive placement scores, affinity and anti-affinity rules, and placement constraints. Why a specific node was chosen is explained on Placement Affinity; per-node runtime pinning is set under Nodes."
        status={<FreshnessBadge state={sourceState(scoresQuery, 30_000)} />}
      />
      <OfflineBanner onRetry={() => window.location.reload()} />

      <AdminTabs tabs={[{ id: "scores", label: "Scores" }, { id: "affinity", label: "Affinity" }, { id: "constraints", label: "Constraints" }]} active={tab} onChange={(id) => setTab(id as typeof tab)} />

      {tab === "scores" && (
        <div className="space-y-4">
          <Card>
            <CardHeader title="Predictive Scoring Metrics" icon={BarChart3} />
            {scoresQuery.isLoading ? (
              <AdminLoadingState label="Loading predictive scores…" />
            ) : scoresQuery.isError ? (<div className="p-4"><AdminErrorState message={scoresQuery.error instanceof Error ? scoresQuery.error.message : "Failed to load data"} retry={() => void scoresQuery.refetch()} /></div>) : scores.length === 0 ? (
              <EmptyState icon={BarChart3} title="No scoring data" message="The scores endpoint answered successfully with no rows: the predictor has no node samples yet, so no placement ranking exists." />
            ) : (
            <div className="space-y-3 p-4">
              {scores.map((node) => {
                const scoreVal = node.totalScore;
                const cpuVal = node.predictedLoad;
                return (
                <div key={node.nodeId} className="rounded-lg border border-line bg-[var(--surface-raised)] p-4">
                  <div className="mb-3 flex items-center justify-between">
                    <p className="font-mono text-sm font-medium text-text">{node.nodeId}</p>
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-text-subtle">Score</span>
                      <span className="text-lg font-bold text-text">{scoreVal.toFixed(1)}</span>
                      <span className="text-xs text-text-subtle">
                        confidence: {typeof node.confidence === "number" ? `${(node.confidence * 100).toFixed(0)}%` : "not reported"}
                      </span>
                    </div>
                  </div>
                  <div className="mb-3 h-2 overflow-hidden rounded-full bg-overlay-strong">
                    <div
                      className={cn(
                        "h-full rounded-full",
                        scoreVal / maxScore > 0.8 ? "bg-ok" :
                        scoreVal / maxScore > 0.5 ? "bg-info" :
                        scoreVal / maxScore > 0.3 ? "bg-warn" : "bg-danger"
                      )}
                      style={{ width: `${(scoreVal / maxScore) * 100}%` }}
                    />
                  </div>
                  <div className="grid grid-cols-4 gap-3 text-center">
                    <div>
                      <p className="t-eyebrow">Trend</p>
                      <div className="mt-1 flex items-center justify-center gap-1">
                        <Activity aria-hidden="true" size={12} className="text-text-muted" />
                        <span className="text-xs text-text">{node.trendScore.toFixed(2)}</span>
                      </div>
                    </div>
                    <div>
                      <p className="t-eyebrow">Affinity</p>
                      <div className="mt-1 flex items-center justify-center gap-1">
                        <Zap aria-hidden="true" size={12} className="text-text-muted" />
                        <span className="text-xs text-text">{node.affinityScore.toFixed(2)}</span>
                      </div>
                    </div>
                    <div>
                      <p className="t-eyebrow">Anti-Affinity</p>
                      <div className="mt-1 flex items-center justify-center gap-1">
                        <HardDrive aria-hidden="true" size={12} className="text-text-muted" />
                        <span className="text-xs text-text">{node.antiAffinityScore.toFixed(2)}</span>
                      </div>
                    </div>
                    <div>
                      <p className="t-eyebrow">Predicted</p>
                      <div className="mt-1 flex items-center justify-center gap-1">
                        <Cpu aria-hidden="true" size={12} className="text-text-muted" />
                        <span className="text-xs text-text">{(cpuVal * 100).toFixed(0)}%</span>
                      </div>
                    </div>
                  </div>
                </div>
              )})}
            </div>
          )}
          </Card>

          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader title="Per-node score lookup" icon={Server} />
              <div className="space-y-3 p-4">
                <p className="text-meta text-text-subtle">Score one node in isolation, with the same breakdown the fleet list shows.</p>
                <div className="flex gap-2">
                  <Input label="Node ID" value={lookupNodeId} onChange={setLookupNodeId} placeholder="nodeId" />
                  <Btn className="self-end" tone="primary" onClick={() => void lookupPerNodeScore()}>Fetch Score</Btn>
                </div>
                {lookupError && <p className="ui-alert ui-alert-danger">{lookupError}</p>}
                {lookupResult && <ScoreVerdict score={lookupResult} />}
                <p className="text-meta text-text-muted">
                  Scores cover predicted load and affinity weights only. For environment pinning,{" "}
                  <a className="font-medium text-[var(--brand)] hover:underline" href="/admin/env-affinity">explain placement in Placement Affinity</a>.
                </p>
              </div>
            </Card>
            <Card>
              <CardHeader title="Ingest predictive metrics" icon={Activity} action={<Pill tone="yellow">Test hook · writes</Pill>} />
              <div className="space-y-3 p-4">
                <p className="text-meta leading-5 text-text-subtle">
                  Writes a hand-typed resource sample into the predictor for one node. It is not a telemetry read: the values
                  become that node&rsquo;s latest measurement and shift where the scheduler places work across the fleet. Leave a
                  field blank and nothing is sent — a blank is never stored as zero.
                </p>
                <Input label="Node ID" value={metricsNodeId} onChange={setMetricsNodeId} placeholder="node_abc" />
                <div className="grid grid-cols-2 gap-3">
                  {METRIC_FIELDS.map((field) => (
                    <Input
                      key={field.key}
                      label={`${field.label} (${field.hint})`}
                      type="number"
                      value={metricsForm[field.key] ?? ""}
                      onChange={(v) => setMetricsForm({ ...metricsForm, [field.key]: v })}
                    />
                  ))}
                </div>
                {!metricReadiness.missing.length && !metricReadiness.nonNumeric.length ? null : (
                  <p className="ui-alert ui-alert-warning">
                    {metricReadiness.missing.length
                      ? `Still blank, and would be written as a measured 0: ${metricReadiness.missing.join(", ")}.`
                      : `Not numbers: ${metricReadiness.nonNumeric.join(", ")}.`}
                  </p>
                )}
                {ingestMetricsMutation.isSuccess && <p className="ui-alert ui-alert-success">Sample recorded for node {metricsNodeId.trim()}.</p>}
                <Btn tone="primary" onClick={confirmIngestMetrics} disabled={!metricReadiness.ready || ingestMetricsMutation.isPending}>
                  {ingestMetricsMutation.isPending ? "Recording…" : "Record metric sample"}
                </Btn>
                {!metricsNodeId.trim() ? <p className="text-meta text-text-muted">A node must be named — the page will not pick one for you.</p> : null}
              </div>
            </Card>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader title="Scheduler Backends" icon={Settings2} />
              <div className="p-4">
                <p className="mb-3 text-meta text-text-subtle">Runtimes this control plane can actually place work onto.</p>
                {backendsQuery.isLoading ? (
                  <AdminLoadingState label="Loading backends…" />
                ) : backendsQuery.isError ? (
                  <AdminErrorState
                    message={`The backend list could not be read: ${backendsQuery.error instanceof Error ? backendsQuery.error.message : "request failed"}. Pinning is disabled until it can, because the available runtimes are unknown.`}
                    retry={() => void backendsQuery.refetch()}
                  />
                ) : backends.length === 0 ? (
                  <EmptyState icon={Settings2} title="No schedulable backends" message="The control plane reported no placement backends, so no runtime can be pinned to a node." />
                ) : (
                  <div className="space-y-2">
                    {backends.map((b) => (
                      <div key={b.type} className="rounded-lg border border-line bg-overlay-subtle p-3">
                        <p className="text-sm font-medium text-text">{b.name} <span className="font-mono text-xs text-text-subtle">({b.type})</span></p>
                        <p className="text-meta text-text-subtle">{b.description || "No description reported."}</p>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </Card>
            <Card>
              <CardHeader title="Node scheduler config" icon={HardDrive} />
              <div className="space-y-3 p-4">
                <p className="text-meta text-text-subtle">Pin one node to a runtime scheduler.</p>
                <Input label="Node ID" value={schedulerNodeId} onChange={setSchedulerNodeId} placeholder="node UUID" />
                {/*
                  Options come from `/admin/scheduler/backends` — the list this tab
                  already fetches — instead of a hardcoded docker/k3s/nomad triple.
                  A pin the backend does not report would be accepted by the form and
                  silently unschedulable, so while the list is unknown or empty the
                  control renders disabled with the reason rather than enabled-and-lying.
                */}
                <AdminSelect
                  label="Scheduler type"
                  value={backends.some((b) => b.type === schedulerType) ? schedulerType : (backends[0]?.type ?? "")}
                  onChange={setSchedulerType}
                  disabled={!backendsReady}
                  placeholder={backendsReady ? "No schedulable backend reported" : "Unavailable — backend list not loaded"}
                  options={backends.map((b) => ({ value: b.type, label: `${b.name} (${b.type})` }))}
                />
                {!backendsReady ? (
                  <p className="ui-alert ui-alert-warning">
                    Runtime pinning is disabled because {backendsQuery.isError ? "the backend list failed to load" : "the backend list has not loaded yet"} — the available runtimes are unknown, and an invented pin would look saved but never schedule.
                  </p>
                ) : null}
                <Input label="Scheduler config (JSON)" value={schedulerConfigJson} onChange={setSchedulerConfigJson} placeholder="{}" mono />
                {updateNodeSchedulerMutation.isError && <p className="ui-alert ui-alert-danger">{updateNodeSchedulerMutation.error instanceof Error ? updateNodeSchedulerMutation.error.message : "Update failed"}</p>}
                {updateNodeSchedulerMutation.isSuccess && <p className="ui-alert ui-alert-success">Scheduler config updated for node {schedulerNodeId.trim()}.</p>}
                <Btn
                  tone="primary"
                  onClick={confirmPinScheduler}
                  disabled={updateNodeSchedulerMutation.isPending || !schedulerNodeId.trim() || !backendsReady || !backends.some((b) => b.type === schedulerType)}
                >
                  {updateNodeSchedulerMutation.isPending ? "Saving…" : "Pin runtime"}
                </Btn>
              </div>
            </Card>
          </div>
        </div>
      )}

      {tab === "affinity" && (
        <div className="space-y-4">
          <p className="text-meta text-text-muted">
            Raw affinity weights live here. To see why a specific server lands (or does not land) on a node,{" "}
            <a className="font-medium text-[var(--brand)] hover:underline" href="/admin/env-affinity">explain placement in Placement Affinity</a>.
          </p>
          <Card>
            <CardHeader
              title="Affinity Rules"
              icon={GanttChart}
              action={
                <Btn size="sm" tone="primary" onClick={() => setShowCreateAffinity(true)}>
                  <Plus size={12} /> Create Rule
                </Btn>
              }
            />
            {affinityQuery.isLoading ? (
              <AdminLoadingState label="Loading affinity rules…" />
            ) : affinityQuery.isError ? (<div className="p-4"><AdminErrorState message={affinityQuery.error instanceof Error ? affinityQuery.error.message : "Failed to load affinity rules"} retry={() => void affinityQuery.refetch()} /></div>) : affinityRules.length === 0 ? (
              <EmptyState icon={GanttChart} title="No affinity rules" message="The rules endpoint answered successfully with nothing configured." />
            ) : (
              <div className="divide-y divide-line">
                {affinityRules.map((rule) => (
                  <div key={rule.id} className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="flex min-w-0 items-center gap-3">
                      <Zap aria-hidden="true" size={16} className="shrink-0 text-text-subtle" />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-text">{rule.label || rule.name || rule.id}</p>
                        {/* `weight` is optional on the wire and creation defaults it to 1,
                            so an absent weight is unknown — not a weight of zero. */}
                        <p className="text-meta text-text-subtle">
                          name: {rule.name || "not set"} · server: {rule.serverId || "not set"} · node: {rule.nodeId || "not set"} · weight: {typeof rule.weight === "number" ? rule.weight : "not reported"}
                        </p>
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <Pill tone="blue">affinity</Pill>
                      <Btn size="sm" tone="danger" ariaLabel={`Delete affinity rule ${rule.label || rule.name || rule.id}`} onClick={() => { void deleteRule("affinity", rule.id, rule.label || rule.name || rule.id); }}>
                        <Trash2 size={12} />
                      </Btn>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Card>

          <Card>
            <CardHeader title="Anti-Affinity Rules" icon={Network} />
            {antiAffinityQuery.isLoading ? (
              <AdminLoadingState label="Loading anti-affinity rules…" />
            ) : antiAffinityQuery.isError ? (<div className="p-4"><AdminErrorState message={antiAffinityQuery.error instanceof Error ? antiAffinityQuery.error.message : "Failed to load anti-affinity"} retry={() => void antiAffinityQuery.refetch()} /></div>) : antiAffinityRules.length === 0 ? (
              <EmptyState icon={Network} title="No anti-affinity rules" message="The rules endpoint answered successfully with nothing configured. Create one to keep workloads apart." />
            ) : (
              <div className="divide-y divide-line">
                {antiAffinityRules.map((rule) => (
                  <div key={rule.id} className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="flex min-w-0 items-center gap-3">
                      <Zap aria-hidden="true" size={16} className="shrink-0 text-text-subtle" />
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-text">{rule.label || rule.name || rule.id}</p>
                        <p className="text-meta text-text-subtle">
                          name: {rule.name || "not set"} · server: {rule.serverId || "not set"} · scope: {rule.scope || "not set"} · weight: {typeof rule.weight === "number" ? rule.weight : "not reported"}
                        </p>
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <Pill tone="red">anti-affinity</Pill>
                      <Btn size="sm" tone="danger" ariaLabel={`Delete anti-affinity rule ${rule.label || rule.name || rule.id}`} onClick={() => { void deleteRule("anti-affinity", rule.id, rule.label || rule.name || rule.id); }}>
                        <Trash2 size={12} />
                      </Btn>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Card>
        </div>
      )}

      {tab === "constraints" && (
        <Card>
          <CardHeader
            title="Constraint-Based Scheduling"
            icon={Network}
            action={
              <Btn size="sm" tone="primary" onClick={() => setShowCreateConstraint(true)}>
                <Plus size={12} /> Add Constraint
              </Btn>
            }
          />
          <div className="px-4 py-2 text-meta text-text-subtle">Constraints are saved as a full set: adding or removing one rewrites the whole list, so two admins editing at once will overwrite each other. Environment-pinned rules are managed in <a className="font-medium text-[var(--brand)] hover:underline" href="/admin/env-affinity">Placement Affinity</a>.</div>
          {constraintsQuery.isLoading ? (
            <AdminLoadingState label="Loading constraints…" />
          ) : constraintsQuery.isError ? (<div className="p-4"><AdminErrorState message={constraintsQuery.error instanceof Error ? constraintsQuery.error.message : "Failed to load constraints"} retry={() => void constraintsQuery.refetch()} /></div>) : constraints.length === 0 ? (
            <EmptyState icon={Network} title="No scheduling constraints" message="The constraints endpoint answered successfully with nothing configured — no placement rule is currently enforced." />
          ) : (
            <AdminTable label="Placement constraints">
              <AdminTHead>
                <AdminTh>Type</AdminTh>
                <AdminTh>Key</AdminTh>
                <AdminTh>Operator</AdminTh>
                <AdminTh>Value</AdminTh>
                <AdminTh>Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {constraints.map((c, idx) => (
                  <AdminTr key={`${c.type}-${c.key}-${idx}`}>
                    <AdminTd><Pill tone={c.type === "required" ? "red" : c.type === "preferred" ? "blue" : "yellow"}>{c.type}</Pill></AdminTd>
                    <AdminTd className="font-mono text-xs text-text">{c.key}</AdminTd>
                    <AdminTd className="font-mono text-xs text-text-subtle">{c.operator}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{c.value || "not set"}</AdminTd>
                    <AdminTd>
                      <Btn size="sm" tone="danger" ariaLabel={`Delete the ${c.type} constraint on ${c.key}`} onClick={() => { void (async () => { if (await confirm({ title: "Delete this constraint?", description: `Placements will stop being filtered by ${c.type} ${c.key} ${c.operator} ${c.value || "—"} — workloads may land on nodes this rule kept them away from. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteConstraintMutation.mutate({ index: idx, id: c.key }); })(); }}>
                        <Trash2 size={12} />
                      </Btn>
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      )}

      {showCreateAffinity && (
        <Modal title="Create Affinity Rule" onClose={() => setShowCreateAffinity(false)}>
          <div className="space-y-4">
            <Input label="Label" value={affinityForm.label} onChange={(v) => setAffinityForm({ ...affinityForm, label: v })} placeholder="Co-locate cache nodes" />
            <Input label="Name" value={affinityForm.name} onChange={(v) => setAffinityForm({ ...affinityForm, name: v })} placeholder="cache-affinity" />
            <div>
              <AdminSelect
                label="Rule type"
                value={affinityForm.type}
                onChange={(value) => setAffinityForm({ ...affinityForm, type: value as "affinity" | "anti_affinity" })}
                options={[
                  { value: "affinity", label: "Affinity (place together)" },
                  { value: "anti_affinity", label: "Anti-affinity (keep apart)" },
                ]}
              />
            </div>
            <Input label="Server ID (optional)" value={affinityForm.serverId} onChange={(v) => setAffinityForm({ ...affinityForm, serverId: v })} placeholder="server UUID" />
            {affinityForm.type === "affinity" ? (
              <Input label="Node ID (optional)" value={affinityForm.nodeId} onChange={(v) => setAffinityForm({ ...affinityForm, nodeId: v })} placeholder="node UUID for affinity targeting" />
            ) : (
              <Input label="Scope" value={affinityForm.scope} onChange={(v) => setAffinityForm({ ...affinityForm, scope: v })} placeholder="node / region" />
            )}
            <Input label="Weight" type="number" value={String(affinityForm.weight)} onChange={(v) => setAffinityForm({ ...affinityForm, weight: Number(v) })} placeholder="1" />
          </div>
          <ModalFooter
            onCancel={() => setShowCreateAffinity(false)}
            onConfirm={() => createAffinityMutation.mutate()}
            confirmLabel={createAffinityMutation.isPending ? "Creating..." : "Create"}
            disabled={createAffinityMutation.isPending || !affinityForm.label}
          />
        </Modal>
      )}

      {showCreateConstraint && (
        <Modal title="Add Constraint" onClose={() => setShowCreateConstraint(false)}>
          <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <AdminSelect
              label="Constraint type"
              value={constraintFormBackend.type}
              onChange={(value) => setConstraintFormBackend({ ...constraintFormBackend, type: value as ConstraintBackend["type"] })}
              options={[
                { value: "required", label: "required — must match" },
                { value: "preferred", label: "preferred — ranks higher" },
                { value: "forbidden", label: "forbidden — excludes" },
              ]}
            />
            <Input label="Key" value={constraintFormBackend.key} onChange={(v) => setConstraintFormBackend({ ...constraintFormBackend, key: v })} placeholder="region | node_id | name" />
            <AdminSelect
              label="Operator"
              value={constraintFormBackend.operator}
              onChange={(value) => setConstraintFormBackend({ ...constraintFormBackend, operator: value })}
              options={[
                { value: "eq", label: "eq — equals" },
                { value: "neq", label: "neq — not equals" },
                { value: "in", label: "in — one of a list" },
                { value: "notin", label: "notin — not in a list" },
                { value: "exists", label: "exists — label present" },
              ]}
            />
            <Input label="Value" value={constraintFormBackend.value} onChange={(v) => setConstraintFormBackend({ ...constraintFormBackend, value: v })} placeholder="us-east-1 or node-123" />
          </div>
          <div className="text-meta text-text-subtle">Required constraints must match; preferred ones rank higher; forbidden ones exclude. Saving rewrites the whole constraint set.</div>
          {createConstraintMutation.isError && <p className="ui-alert ui-alert-danger">{createConstraintMutation.error instanceof Error ? createConstraintMutation.error.message : "Create failed"}</p>}
          </div>
          <ModalFooter
            onCancel={() => setShowCreateConstraint(false)}
            onConfirm={() => createConstraintMutation.mutate()}
            confirmLabel={createConstraintMutation.isPending ? "Creating..." : "Create"}
            disabled={createConstraintMutation.isPending || !constraintFormBackend.key.trim()}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

/**
 * The lookup verdict.
 *
 * This used to print a green "scored node" pill for any response, including a
 * node with total score 0 and confidence 0 — which is the scorer saying it has no
 * evidence, not that the node is a good target. A score is a ranking, not a health
 * claim, so a ordinary reading is `neutral` and a no-signal reading is `unknown`.
 */
function ScoreVerdict({ score }: { score: PredictiveScore }) {
  const hasConfidence = typeof score.confidence === "number" && Number.isFinite(score.confidence);
  const confidencePct = hasConfidence ? Math.round(score.confidence * 100) : null;
  const noSignal = !hasConfidence || score.confidence <= 0 || !Number.isFinite(score.totalScore) || score.totalScore <= 0;
  const tone: AdminTone = noSignal ? "unknown" : "neutral";
  const label = noSignal
    ? score.confidence <= 0 || !hasConfidence
      ? "scored with no confidence"
      : "score is not above zero"
    : "scored";
  return (
    <div className="space-y-3 rounded-xl border border-line bg-[var(--surface-raised)] p-4">
      <div className="flex flex-wrap items-center gap-2">
        <Pill tone={tone}>{label}</Pill>
        <span className="font-mono text-xs text-text-subtle">{score.nodeId}</span>
        <Pill tone={confidencePct === null ? "unknown" : confidencePct < 30 ? "yellow" : "blue"}>
          confidence {confidencePct === null ? "not reported" : `${confidencePct}%`}
        </Pill>
      </div>
      {noSignal ? (
        <p className="ui-alert ui-alert-warning">
          The predictor returned a score but no confidence behind it. Treat this node as unranked — the number is not evidence
          that it is a good or bad target.
        </p>
      ) : null}
      <dl className="divide-y divide-line text-xs">
        <div className="flex items-center justify-between py-1.5">
          <dt className="text-text-muted">Total score</dt>
          <dd className="font-mono text-sm font-bold text-text">{Number.isFinite(score.totalScore) ? score.totalScore.toFixed(1) : "not reported"}</dd>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <dt className="text-text-muted">Base</dt>
          <dd className="font-mono text-text">{score.baseScore.toFixed(2)}</dd>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <dt className="text-text-muted">Trend</dt>
          <dd className="font-mono text-text">{score.trendScore.toFixed(2)}</dd>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <dt className="text-text-muted">Affinity</dt>
          <dd className="font-mono text-text">+{score.affinityScore.toFixed(2)}</dd>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <dt className="text-text-muted">Anti-affinity</dt>
          <dd className="font-mono text-text">{score.antiAffinityScore.toFixed(2)}</dd>
        </div>
        <div className="flex items-center justify-between py-1.5">
          <dt className="text-text-muted">Predicted load</dt>
          <dd className="font-mono text-text">{(score.predictedLoad * 100).toFixed(0)}%</dd>
        </div>
      </dl>
    </div>
  );
}
