"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import {
  Activity, BarChart3, Cpu, GanttChart, HardDrive, Network, Plus, Trash2, Zap, Server, Settings2,
} from "lucide-react";
import { fetchJSON, postJSON, putJSON, deleteJSON } from "@/lib/api";
import {AdminPageLayout, AdminTabs, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, cn, AdminErrorState, AdminLoadingState} from "@/components/admin/admin-ui";
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

type ConstraintUI = {
  id: string;
  name: string;
  type: "cpu" | "memory" | "disk" | "ports" | "custom";
  operator: "lt" | "gt" | "eq" | "neq" | "in";
  value: string;
  enabled: boolean;
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

const defaultConstraintFormUI = {
  name: "",
  type: "cpu" as ConstraintUI["type"],
  operator: "eq" as ConstraintUI["operator"],
  value: "",
  enabled: true,
};

const defaultConstraintBackendForm = {
  type: "required" as ConstraintBackend["type"],
  key: "region" as string,
  operator: "eq" as string,
  value: "",
};

export default function AdminSchedulerPage() {
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [tab, setTab] = useState<"scores" | "affinity" | "constraints">("scores");
  const [showCreateAffinity, setShowCreateAffinity] = useState(false);
  const [affinityForm, setAffinityForm] = useState(defaultAffinityForm);
  const [showCreateConstraint, setShowCreateConstraint] = useState(false);
  const [constraintFormUI, setConstraintFormUI] = useState(defaultConstraintFormUI);
  const [constraintFormBackend, setConstraintFormBackend] = useState(defaultConstraintBackendForm);
  const [useBackendConstraintShape, setUseBackendConstraintShape] = useState(false);
  const [lookupNodeId, setLookupNodeId] = useState("");
  const [lookupResult, setLookupResult] = useState<PredictiveScore | null>(null);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [metricsNodeId, setMetricsNodeId] = useState("");
  const [metricsForm, setMetricsForm] = useState<ResourceMetric>({
    cpuPercent: 0,
    memoryUsedMb: 0,
    diskUsedMb: 0,
    networkRx: 0,
    networkTx: 0,
    serverCount: 0,
  });
  const [schedulerNodeId, setSchedulerNodeId] = useState("");
  const [schedulerType, setSchedulerType] = useState("docker");
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
    mutationFn: (id: string) => deleteJSON(`/admin/scheduler/predictive/affinity-rules/${encodeURIComponent(id)}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "affinity"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to delete affinity rule", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const deleteAntiAffinityMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/scheduler/predictive/anti-affinity-rules/${encodeURIComponent(id)}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "anti-affinity"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to delete anti-affinity rule", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const createConstraintMutation = useMutation({
    mutationFn: async () => {
      const existing = constraints ?? [];
      let newConstraint: ConstraintBackend;
      if (useBackendConstraintShape) {
        newConstraint = {
          type: constraintFormBackend.type,
          key: constraintFormBackend.key.trim(),
          operator: constraintFormBackend.operator,
          value: constraintFormBackend.value,
        };
        if (!newConstraint.key) throw new Error("Key is required");
      } else {
        const keyMap: Record<string, string> = {
          cpu: "cpu",
          memory: "memory",
          disk: "disk",
          ports: "ports",
          custom: constraintFormUI.name || "custom",
        };
        newConstraint = {
          type: "required",
          key: keyMap[constraintFormUI.type] || constraintFormUI.name || "custom",
          operator: constraintFormUI.operator,
          value: constraintFormUI.value,
        };
        if (!constraintFormUI.name) throw new Error("Name is required");
      }
      const next = [...existing, newConstraint];
      return putJSON("/admin/scheduler/constraints", next);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "constraints"] });
      setShowCreateConstraint(false);
      setConstraintFormUI(defaultConstraintFormUI);
      setConstraintFormBackend(defaultConstraintBackendForm);
      toast({ tone: "success", title: "Constraint saved" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to save constraint", message: err instanceof Error ? err.message : "An error occurred" }),
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

  const ingestMetricsMutation = useMutation({
    mutationFn: () => {
      if (!metricsNodeId.trim()) throw new Error("Node ID is required");
      const payload: ResourceMetric = {
        timestamp: new Date().toISOString(),
        cpuPercent: Number(metricsForm.cpuPercent) || 0,
        memoryUsedMb: Number(metricsForm.memoryUsedMb) || 0,
        diskUsedMb: Number(metricsForm.diskUsedMb) || 0,
        networkRx: Number(metricsForm.networkRx) || 0,
        networkTx: Number(metricsForm.networkTx) || 0,
        serverCount: Number(metricsForm.serverCount) || 0,
      };
      return postJSON(`/admin/scheduler/predictive/metrics/${encodeURIComponent(metricsNodeId.trim())}`, payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "scores"] });
      toast({ tone: "success", title: "Metrics ingested" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to ingest metrics", message: err instanceof Error ? err.message : "An error occurred" }),
  });

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
    onSuccess: () => toast({ tone: "success", title: "Scheduler config updated" }),
    onError: (err) => toast({ tone: "error", title: "Failed to update scheduler", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const maxScore = Math.max(...scores.map((s) => s.totalScore), 1);

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Scheduler"
        sub="Placement scoring, strategies and explanations"
      />
      <OfflineBanner onRetry={() => window.location.reload()} />

      <AdminTabs tabs={[{ id: "scores", label: "Scores" }, { id: "affinity", label: "Affinity" }, { id: "constraints", label: "Constraints" }]} active={tab} onChange={(id) => setTab(id as typeof tab)} />

      {tab === "scores" && (
        <div className="space-y-4">
          <Card>
            <CardHeader title="Predictive Scoring Metrics" icon={BarChart3} />
            {scoresQuery.isLoading ? (
              <AdminLoadingState label="Loading scores..." />
            ) : scoresQuery.isError ? (<div className="p-4"><AdminErrorState message={scoresQuery.error instanceof Error ? scoresQuery.error.message : "Failed to load data"} retry={() => void scoresQuery.refetch()} /></div>) : scores.length === 0 ? (
              <EmptyState icon={BarChart3} message="No scoring data available." />
            ) : (
            <div className="space-y-3 p-4">
              {scores.map((node) => {
                const scoreVal = node.totalScore;
                const cpuVal = node.predictedLoad;
                return (
                <div key={node.nodeId} className="rounded-lg border border-line bg-[var(--surface-raised)] p-4">
                  <div className="flex items-center justify-between mb-3">
                    <p className="font-mono text-sm font-medium text-text">{node.nodeId}</p>
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-text-subtle">Score</span>
                      <span className="text-lg font-bold text-text">{scoreVal.toFixed(1)}</span>
                      {typeof node.confidence === "number" && <span className="text-xs text-text-subtle">conf: {(node.confidence*100).toFixed(0)}%</span>}
                    </div>
                  </div>
                  <div className="mb-3 h-2 overflow-hidden rounded-full bg-overlay-strong">
                    <div
                      className={cn(
                        "h-full rounded-full",
                        scoreVal / maxScore > 0.8 ? "bg-ok" :
                        scoreVal / maxScore > 0.5 ? "bg-brand" :
                        scoreVal / maxScore > 0.3 ? "bg-warn" : "bg-danger"
                      )}
                      style={{ width: `${(scoreVal / maxScore) * 100}%` }}
                    />
                  </div>
                  <div className="grid grid-cols-4 gap-3 text-center">
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-text-subtle">Trend</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <Activity size={12} className="text-text-subtle" />
                        <span className="text-xs text-text-subtle">{node.trendScore.toFixed(2)}</span>
                      </div>
                    </div>
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-text-subtle">Affinity</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <Zap size={12} className="text-text-subtle" />
                        <span className="text-xs text-text-subtle">{node.affinityScore.toFixed(2)}</span>
                      </div>
                    </div>
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-text-subtle">Anti-Affinity</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <HardDrive size={12} className="text-text-subtle" />
                        <span className="text-xs text-text-subtle">{node.antiAffinityScore.toFixed(2)}</span>
                      </div>
                    </div>
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-text-subtle">Predicted</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <Cpu size={12} className="text-text-subtle" />
                        <span className="text-xs text-text-subtle">{(cpuVal * 100).toFixed(0)}%</span>
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
              <CardHeader title="Per-Node Score Lookup" icon={Server} />
              <div className="p-4 space-y-3">
                <p className="text-xs text-text-subtle">GET /admin/scheduler/predictive/nodes/:nodeId/score</p>
                <div className="flex gap-2">
                  <Input placeholder="nodeId" value={lookupNodeId} onChange={setLookupNodeId} />
                  <Btn tone="primary" onClick={lookupPerNodeScore}>Fetch Score</Btn>
                </div>
                {lookupError && <p className="text-xs text-danger">{lookupError}</p>}
                {lookupResult && (
                  <div className="rounded-lg border border-line bg-[var(--surface-raised)] p-3 text-xs font-mono text-text space-y-1">
                    <div>nodeId: {lookupResult.nodeId}</div>
                    <div>totalScore: {lookupResult.totalScore}</div>
                    <div>baseScore: {lookupResult.baseScore}</div>
                    <div>trendScore: {lookupResult.trendScore}</div>
                    <div>affinityScore: {lookupResult.affinityScore}</div>
                    <div>antiAffinityScore: {lookupResult.antiAffinityScore}</div>
                    <div>predictedLoad: {lookupResult.predictedLoad}</div>
                    <div>confidence: {lookupResult.confidence}</div>
                  </div>
                )}
              </div>
            </Card>
            <Card>
              <CardHeader title="Ingest Predictive Metrics" icon={Activity} />
              <div className="p-4 space-y-3">
                <p className="text-xs text-text-subtle">POST /admin/scheduler/predictive/metrics/:nodeId</p>
                <Input label="Node ID" value={metricsNodeId} onChange={setMetricsNodeId} placeholder="node_abc" />
                <div className="grid grid-cols-2 gap-3">
                  <Input label="CPU %" type="number" value={String(metricsForm.cpuPercent)} onChange={(v) => setMetricsForm({ ...metricsForm, cpuPercent: Number(v) })} />
                  <Input label="Memory MB" type="number" value={String(metricsForm.memoryUsedMb)} onChange={(v) => setMetricsForm({ ...metricsForm, memoryUsedMb: Number(v) })} />
                  <Input label="Disk MB" type="number" value={String(metricsForm.diskUsedMb)} onChange={(v) => setMetricsForm({ ...metricsForm, diskUsedMb: Number(v) })} />
                  <Input label="Server Count" type="number" value={String(metricsForm.serverCount)} onChange={(v) => setMetricsForm({ ...metricsForm, serverCount: Number(v) })} />
                  <Input label="Network Rx" type="number" value={String(metricsForm.networkRx)} onChange={(v) => setMetricsForm({ ...metricsForm, networkRx: Number(v) })} />
                  <Input label="Network Tx" type="number" value={String(metricsForm.networkTx)} onChange={(v) => setMetricsForm({ ...metricsForm, networkTx: Number(v) })} />
                </div>
                {ingestMetricsMutation.isError && <p className="text-xs text-danger">{ingestMetricsMutation.error instanceof Error ? ingestMetricsMutation.error.message : "Failed to ingest"}</p>}
                {ingestMetricsMutation.isSuccess && <p className="text-xs text-ok">Metrics ingested</p>}
                <Btn tone="primary" onClick={() => ingestMetricsMutation.mutate()} disabled={ingestMetricsMutation.isPending}>{ingestMetricsMutation.isPending ? "Ingesting..." : "Ingest Metrics"}</Btn>
              </div>
            </Card>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <CardHeader title="Scheduler Backends" icon={Settings2} />
              <div className="p-4">
                <p className="text-xs text-text-subtle mb-3">GET /admin/scheduler/backends (static list)</p>
                {backendsQuery.isLoading ? <div className="text-sm text-text-subtle">Loading backends...</div>
                : backendsQuery.isError ? <div className="text-sm text-danger">{backendsQuery.error instanceof Error ? backendsQuery.error.message : "Failed"}</div>
                : (
                  <div className="space-y-2">
                    {backends.map((b) => (
                      <div key={b.type} className="rounded-lg border border-line bg-overlay-subtle p-3">
                        <p className="text-sm font-medium text-text">{b.name} <span className="font-mono text-xs text-text-subtle">({b.type})</span></p>
                        <p className="text-xs text-text-subtle">{b.description}</p>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </Card>
            <Card>
              <CardHeader title="Node Scheduler Config" icon={HardDrive} />
              <div className="p-4 space-y-3">
                <p className="text-xs text-text-subtle">PUT /admin/scheduler/nodes/:id/scheduler</p>
                <Input label="Node ID" value={schedulerNodeId} onChange={setSchedulerNodeId} placeholder="node UUID" />
                <div>
                  <label className="block text-sm font-medium text-text-subtle mb-1.5">Scheduler Type</label>
                  <select className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text" value={schedulerType} onChange={(e) => setSchedulerType(e.target.value)}>
                    <option value="docker">Docker</option>
                    <option value="k3s">K3s</option>
                    <option value="nomad">Nomad</option>
                  </select>
                </div>
                <Input label="Scheduler Config (JSON)" value={schedulerConfigJson} onChange={setSchedulerConfigJson} placeholder='{}' />
                {updateNodeSchedulerMutation.isError && <p className="text-xs text-danger">{updateNodeSchedulerMutation.error instanceof Error ? updateNodeSchedulerMutation.error.message : "Update failed"}</p>}
                {updateNodeSchedulerMutation.isSuccess && <p className="text-xs text-ok">Scheduler config updated</p>}
                <Btn tone="primary" onClick={() => updateNodeSchedulerMutation.mutate()} disabled={updateNodeSchedulerMutation.isPending || !schedulerNodeId.trim()}>{updateNodeSchedulerMutation.isPending ? "Saving..." : "Update Scheduler"}</Btn>
              </div>
            </Card>
          </div>
        </div>
      )}

      {tab === "affinity" && (
        <div className="space-y-4">
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
              <AdminLoadingState label="Loading rules..." />
            ) : affinityQuery.isError ? (<div className="p-4"><AdminErrorState message={affinityQuery.error instanceof Error ? affinityQuery.error.message : "Failed to load data"} retry={() => void affinityQuery.refetch()} /></div>) : affinityRules.length === 0 ? (
              <EmptyState icon={GanttChart} message="No affinity rules configured." />
            ) : (
              <div className="divide-y divide-line">
                {affinityRules.map((rule) => (
                  <div key={rule.id} className="flex items-center justify-between px-4 py-3">
                    <div className="flex items-center gap-3">
                      <Zap size={16} className="text-ok" />
                      <div>
                        <p className="text-sm font-medium text-text">{rule.label || rule.name || rule.id}</p>
                        <p className="text-xs text-text-subtle">
                          name: {rule.name ?? "—"} — server: {rule.serverId ?? "—"} — node: {rule.nodeId ?? "—"} — weight: {rule.weight ?? 0}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Pill tone="green">affinity</Pill>
                      <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: "Delete this affinity rule?", description: "The scheduler will stop applying this rule. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteAffinityMutation.mutate(rule.id); })(); }}>
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
              <AdminLoadingState label="Loading anti-affinity..." />
            ) : antiAffinityQuery.isError ? (<div className="p-4"><AdminErrorState message={antiAffinityQuery.error instanceof Error ? antiAffinityQuery.error.message : "Failed to load anti-affinity"} retry={() => void antiAffinityQuery.refetch()} /></div>) : antiAffinityRules.length === 0 ? (
              <EmptyState icon={Network} message="No anti-affinity rules configured. GET /admin/scheduler/predictive/anti-affinity-rules" />
            ) : (
              <div className="divide-y divide-line">
                {antiAffinityRules.map((rule) => (
                  <div key={rule.id} className="flex items-center justify-between px-4 py-3">
                    <div className="flex items-center gap-3">
                      <Zap size={16} className="text-danger" />
                      <div>
                        <p className="text-sm font-medium text-text">{rule.label || rule.name || rule.id}</p>
                        <p className="text-xs text-text-subtle">
                          name: {rule.name ?? "—"} — server: {rule.serverId ?? "—"} — scope: {rule.scope ?? "—"} — weight: {rule.weight ?? 0}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Pill tone="red">anti-affinity</Pill>
                      <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: "Delete this anti-affinity rule?", description: "The scheduler will stop applying this rule. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteAntiAffinityMutation.mutate(rule.id); })(); }}>
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
          <div className="px-4 py-2 text-xs text-text-subtle">Backend: PUT /admin/scheduler/constraints expects []Constraint array — frontend now PUTs full list. DELETE /admin/scheduler/constraints/:id also wired (via PUT fallback).</div>
          {constraintsQuery.isLoading ? (
            <AdminLoadingState label="Loading constraints..." />
          ) : constraintsQuery.isError ? (<div className="p-4"><AdminErrorState message={constraintsQuery.error instanceof Error ? constraintsQuery.error.message : "Failed to load data"} retry={() => void constraintsQuery.refetch()} /></div>) : constraints.length === 0 ? (
            <EmptyState icon={Network} message="No scheduling constraints configured." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-line text-left text-[10px] uppercase tracking-widest text-text-subtle">
                    <th className="px-4 py-3">Type</th>
                    <th className="px-4 py-3">Key</th>
                    <th className="px-4 py-3">Operator</th>
                    <th className="px-4 py-3">Value</th>
                    <th className="px-4 py-3"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {constraints.map((c, idx) => (
                    <tr key={`${c.type}-${c.key}-${idx}`} className="hover:bg-overlay-subtle">
                      <td className="px-4 py-3"><Pill tone={c.type === "required" ? "red" : c.type === "preferred" ? "blue" : "yellow"}>{c.type}</Pill></td>
                      <td className="px-4 py-3 font-mono text-xs text-text-subtle">{c.key}</td>
                      <td className="px-4 py-3 font-mono text-xs text-text-subtle">{c.operator}</td>
                      <td className="px-4 py-3 text-xs text-text-subtle">{c.value}</td>
                      <td className="px-4 py-3">
                        <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: "Delete this constraint?", description: "The scheduler will stop applying this constraint. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteConstraintMutation.mutate({ index: idx, id: c.key }); })(); }}>
                          <Trash2 size={12} />
                        </Btn>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {showCreateAffinity && (
        <Modal title="Create Affinity Rule" onClose={() => setShowCreateAffinity(false)}>
          <div className="space-y-4">
            <Input label="Label" value={affinityForm.label} onChange={(v) => setAffinityForm({ ...affinityForm, label: v })} placeholder="Co-locate cache nodes" />
            <Input label="Name" value={affinityForm.name} onChange={(v) => setAffinityForm({ ...affinityForm, name: v })} placeholder="cache-affinity" />
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Type</label>
              <select
                className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none focus:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] focus:ring-1 focus:ring-[color-mix(in_srgb,var(--brand)_30%,transparent)]"
                value={affinityForm.type}
                onChange={(e) => setAffinityForm({ ...affinityForm, type: e.target.value as "affinity" | "anti_affinity" })}
              >
                <option value="affinity">Affinity (co-locate) → POST /predictive/affinity-rules</option>
                <option value="anti_affinity">Anti-Affinity (separate) → POST /predictive/anti-affinity-rules</option>
              </select>
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
          <div className="flex items-center gap-2 text-xs text-text-subtle">
            <label className="flex items-center gap-1"><input type="checkbox" checked={useBackendConstraintShape} onChange={(e) => setUseBackendConstraintShape(e.target.checked)} /> Use backend shape (required/preferred/forbidden + key)</label>
          </div>
          {useBackendConstraintShape ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Type</label>
                <select className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text" value={constraintFormBackend.type} onChange={(e) => setConstraintFormBackend({ ...constraintFormBackend, type: e.target.value as ConstraintBackend["type"] })}>
                  <option value="required">required</option>
                  <option value="preferred">preferred</option>
                  <option value="forbidden">forbidden</option>
                </select>
              </div>
              <Input label="Key" value={constraintFormBackend.key} onChange={(v) => setConstraintFormBackend({ ...constraintFormBackend, key: v })} placeholder="region | node_id | name" />
              <div>
                <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Operator</label>
                <select className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text" value={constraintFormBackend.operator} onChange={(e) => setConstraintFormBackend({ ...constraintFormBackend, operator: e.target.value })}>
                  <option value="eq">eq</option>
                  <option value="neq">neq</option>
                  <option value="in">in</option>
                  <option value="notin">notin</option>
                  <option value="exists">exists</option>
                </select>
              </div>
              <Input label="Value" value={constraintFormBackend.value} onChange={(v) => setConstraintFormBackend({ ...constraintFormBackend, value: v })} placeholder="us-east-1 or node-123" />
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              <Input label="Name" value={constraintFormUI.name} onChange={(v) => setConstraintFormUI({ ...constraintFormUI, name: v })} placeholder="Max CPU per node" />
              <label className="flex items-center gap-2 text-sm font-medium text-text-subtle pt-6">
                <input type="checkbox" checked={constraintFormUI.enabled} onChange={(e) => setConstraintFormUI({ ...constraintFormUI, enabled: e.target.checked })} className="rounded border-line bg-[var(--surface-input)]" />
                Enabled
              </label>
              <div>
                <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Type (maps to key)</label>
                <select
                  className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none focus:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] focus:ring-1 focus:ring-[color-mix(in_srgb,var(--brand)_30%,transparent)]"
                  value={constraintFormUI.type}
                  onChange={(e) => setConstraintFormUI({ ...constraintFormUI, type: e.target.value as ConstraintUI["type"] })}
                >
                  <option value="cpu">CPU</option>
                  <option value="memory">Memory</option>
                  <option value="disk">Disk</option>
                  <option value="ports">Ports</option>
                  <option value="custom">Custom</option>
                </select>
              </div>
              <div>
                <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Operator</label>
                <select
                  className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none focus:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] focus:ring-1 focus:ring-[color-mix(in_srgb,var(--brand)_30%,transparent)]"
                  value={constraintFormUI.operator}
                  onChange={(e) => setConstraintFormUI({ ...constraintFormUI, operator: e.target.value as ConstraintUI["operator"] })}
                >
                  <option value="lt">Less Than (&lt;)</option>
                  <option value="gt">Greater Than (&gt;)</option>
                  <option value="eq">Equal (=)</option>
                  <option value="neq">Not Equal (!=)</option>
                  <option value="in">In</option>
                </select>
              </div>
              <Input label="Value" value={constraintFormUI.value} onChange={(v) => setConstraintFormUI({ ...constraintFormUI, value: v })} placeholder="80" />
            </div>
          )}
          <div className="text-xs text-text-subtle">Will PUT full array to /admin/scheduler/constraints (backend expects []Constraint). Legacy single POST was 400.</div>
          {createConstraintMutation.isError && <p className="text-sm text-danger">{createConstraintMutation.error instanceof Error ? createConstraintMutation.error.message : "Create failed"}</p>}
          </div>
          <ModalFooter
            onCancel={() => setShowCreateConstraint(false)}
            onConfirm={() => createConstraintMutation.mutate()}
            confirmLabel={createConstraintMutation.isPending ? "Creating..." : "Create"}
            disabled={createConstraintMutation.isPending || (!useBackendConstraintShape ? !constraintFormUI.name : !constraintFormBackend.key)}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
