"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Activity, BarChart3, Cpu, GanttChart, HardDrive, Network, Plus, Trash2, Zap,
} from "lucide-react";
import { fetchJSON, postJSON, deleteJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";

type NodeScore = {
  nodeId: string;
  score: number;
  cpuLoad: number;
  memoryUsage: number;
  diskUsage: number;
  networkLoad: number;
  activeServers: number;
};

type AffinityRule = {
  id: string;
  type: "affinity" | "anti_affinity";
  label: string;
  scope: "server" | "node" | "region";
  targetIds: string[];
  enabled: boolean;
  createdAt: string;
};

type Constraint = {
  id: string;
  name: string;
  type: "cpu" | "memory" | "disk" | "ports" | "custom";
  operator: "lt" | "gt" | "eq" | "neq" | "in";
  value: string;
  enabled: boolean;
};

const defaultAffinityForm = {
  type: "affinity" as "affinity" | "anti_affinity",
  label: "",
  scope: "node" as "server" | "node" | "region",
  targetIds: "",
  enabled: true,
};

const defaultConstraintForm = {
  name: "",
  type: "cpu" as Constraint["type"],
  operator: "lt" as Constraint["operator"],
  value: "",
  enabled: true,
};

export default function AdminSchedulerPage() {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<"scores" | "affinity" | "constraints">("scores");
  const [showCreateAffinity, setShowCreateAffinity] = useState(false);
  const [affinityForm, setAffinityForm] = useState(defaultAffinityForm);
  const [editingAffinity, setEditingAffinity] = useState<AffinityRule | null>(null);
  const [showCreateConstraint, setShowCreateConstraint] = useState(false);
  const [constraintForm, setConstraintForm] = useState(defaultConstraintForm);

  const scoresQuery = useQuery({
    queryKey: ["admin", "scheduler", "scores"],
    queryFn: () => fetchJSON<NodeScore[]>("/admin/scheduler/scores"),
  });

  const affinityQuery = useQuery({
    queryKey: ["admin", "scheduler", "affinity"],
    queryFn: () => fetchJSON<AffinityRule[]>("/admin/scheduler/affinity"),
  });

  const constraintsQuery = useQuery({
    queryKey: ["admin", "scheduler", "constraints"],
    queryFn: () => fetchJSON<Constraint[]>("/admin/scheduler/constraints"),
  });

  const scores = useMemo(() => scoresQuery.data ?? [], [scoresQuery.data]);
  const affinityRules = useMemo(() => affinityQuery.data ?? [], [affinityQuery.data]);
  const constraints = useMemo(() => constraintsQuery.data ?? [], [constraintsQuery.data]);

  const createAffinityMutation = useMutation({
    mutationFn: () => postJSON("/admin/scheduler/affinity", {
      ...affinityForm,
      targetIds: affinityForm.targetIds.split(",").map((s) => s.trim()),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "affinity"] });
      setShowCreateAffinity(false);
      setAffinityForm(defaultAffinityForm);
    },
  });

  const deleteAffinityMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/scheduler/affinity/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "affinity"] }),
  });

  const createConstraintMutation = useMutation({
    mutationFn: () => postJSON("/admin/scheduler/constraints", constraintForm),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "constraints"] });
      setShowCreateConstraint(false);
      setConstraintForm(defaultConstraintForm);
    },
  });

  const deleteConstraintMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/scheduler/constraints/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "scheduler", "constraints"] }),
  });

  const tabs: Array<{ id: typeof tab; label: string }> = [
    { id: "scores", label: "Scores" },
    { id: "affinity", label: "Affinity" },
    { id: "constraints", label: "Constraints" },
  ];

  const maxScore = Math.max(...scores.map((s) => s.score), 1);

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Scheduler Configuration"
        sub="Predictive scoring, affinity rules, and constraint-based placement configuration."
      />

      <div className="flex gap-1 rounded-lg border border-white/[0.06] bg-[#161b28] p-1 w-fit">
        {tabs.map((t) => (
          <button
            key={t.id}
            className={`px-4 py-2 text-xs font-semibold rounded-md transition ${tab === t.id ? "bg-[#dc2626] text-white" : "text-slate-400 hover:text-slate-200"}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "scores" && (
        <Card>
          <CardHeader title="Predictive Scoring Metrics" icon={BarChart3} />
          {scoresQuery.isLoading ? (
            <div className="p-8 text-center text-sm text-slate-500">Loading scores...</div>
          ) : scores.length === 0 ? (
            <EmptyState icon={BarChart3} message="No scoring data available." />
          ) : (
            <div className="space-y-3 p-4">
              {scores.map((node) => (
                <div key={node.nodeId} className="rounded-lg border border-white/[0.06] bg-[#151b27] p-4">
                  <div className="flex items-center justify-between mb-3">
                    <p className="font-mono text-sm font-medium text-slate-200">{node.nodeId}</p>
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-slate-500">Score</span>
                      <span className="text-lg font-bold text-slate-100">{node.score.toFixed(1)}</span>
                    </div>
                  </div>
                  <div className="mb-3 h-2 overflow-hidden rounded-full bg-slate-800">
                    <div
                      className={cn(
                        "h-full rounded-full",
                        node.score / maxScore > 0.8 ? "bg-emerald-500" :
                        node.score / maxScore > 0.5 ? "bg-blue-500" :
                        node.score / maxScore > 0.3 ? "bg-amber-500" : "bg-red-500"
                      )}
                      style={{ width: `${(node.score / maxScore) * 100}%` }}
                    />
                  </div>
                  <div className="grid grid-cols-4 gap-3 text-center">
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">CPU</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <Cpu size={12} className="text-slate-400" />
                        <span className="text-xs text-slate-300">{(node.cpuLoad * 100).toFixed(0)}%</span>
                      </div>
                    </div>
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Memory</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <HardDrive size={12} className="text-slate-400" />
                        <span className="text-xs text-slate-300">{(node.memoryUsage * 100).toFixed(0)}%</span>
                      </div>
                    </div>
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Disk</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <HardDrive size={12} className="text-slate-400" />
                        <span className="text-xs text-slate-300">{(node.diskUsage * 100).toFixed(0)}%</span>
                      </div>
                    </div>
                    <div>
                      <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Servers</p>
                      <div className="flex items-center justify-center gap-1 mt-1">
                        <Activity size={12} className="text-slate-400" />
                        <span className="text-xs text-slate-300">{node.activeServers}</span>
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}

      {tab === "affinity" && (
        <Card>
          <CardHeader
            title="Affinity & Anti-Affinity Rules"
            icon={GanttChart}
            action={
              <Btn size="sm" tone="primary" onClick={() => setShowCreateAffinity(true)}>
                <Plus size={12} /> Create Rule
              </Btn>
            }
          />
          {affinityQuery.isLoading ? (
            <div className="p-8 text-center text-sm text-slate-500">Loading rules...</div>
          ) : affinityRules.length === 0 ? (
            <EmptyState icon={GanttChart} message="No affinity rules configured." />
          ) : (
            <div className="divide-y divide-white/[0.04]">
              {affinityRules.map((rule) => (
                <div key={rule.id} className="flex items-center justify-between px-4 py-3">
                  <div className="flex items-center gap-3">
                    {rule.type === "affinity" ? (
                      <Zap size={16} className="text-emerald-400" />
                    ) : (
                      <Zap size={16} className="text-red-400" />
                    )}
                    <div>
                      <p className="text-sm font-medium text-slate-200">{rule.label}</p>
                      <p className="text-xs text-slate-500">
                        {rule.type.replace("_", " ")} — scope: {rule.scope} — targets: {rule.targetIds.join(", ")}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone={rule.type === "affinity" ? "green" : "red"}>{rule.type.replace("_", " ")}</Pill>
                    <Pill tone={rule.enabled ? "green" : "neutral"}>{rule.enabled ? "Enabled" : "Disabled"}</Pill>
                    <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete rule?")) deleteAffinityMutation.mutate(rule.id); }}>
                      <Trash2 size={12} />
                    </Btn>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
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
          {constraintsQuery.isLoading ? (
            <div className="p-8 text-center text-sm text-slate-500">Loading constraints...</div>
          ) : constraints.length === 0 ? (
            <EmptyState icon={Network} message="No scheduling constraints configured." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                    <th className="px-4 py-3">Name</th>
                    <th className="px-4 py-3">Type</th>
                    <th className="px-4 py-3">Operator</th>
                    <th className="px-4 py-3">Value</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {constraints.map((c) => (
                    <tr key={c.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-medium text-slate-200">{c.name}</td>
                      <td className="px-4 py-3">
                        <Pill tone={
                          c.type === "cpu" ? "blue" :
                          c.type === "memory" ? "yellow" :
                          c.type === "disk" ? "green" :
                          c.type === "ports" ? "neutral" : "red"
                        }>
                          {c.type}
                        </Pill>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{c.operator}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{c.value}</td>
                      <td className="px-4 py-3">
                        <Pill tone={c.enabled ? "green" : "neutral"}>{c.enabled ? "Active" : "Inactive"}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete constraint?")) deleteConstraintMutation.mutate(c.id); }}>
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
          <div className="grid gap-4">
            <Input label="Label" value={affinityForm.label} onChange={(v) => setAffinityForm({ ...affinityForm, label: v })} placeholder="Co-locate cache nodes" />
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Type</label>
              <select
                className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={affinityForm.type}
                onChange={(e) => setAffinityForm({ ...affinityForm, type: e.target.value as "affinity" | "anti_affinity" })}
              >
                <option value="affinity">Affinity (co-locate)</option>
                <option value="anti_affinity">Anti-Affinity (separate)</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Scope</label>
              <select
                className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={affinityForm.scope}
                onChange={(e) => setAffinityForm({ ...affinityForm, scope: e.target.value as "server" | "node" | "region" })}
              >
                <option value="server">Server</option>
                <option value="node">Node</option>
                <option value="region">Region</option>
              </select>
            </div>
            <Input label="Target IDs (comma separated)" value={affinityForm.targetIds} onChange={(v) => setAffinityForm({ ...affinityForm, targetIds: v })} placeholder="node_1, node_2" />
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
              <input type="checkbox" checked={affinityForm.enabled} onChange={(e) => setAffinityForm({ ...affinityForm, enabled: e.target.checked })} className="rounded border-white/10 bg-[#161b28]" />
              Enabled
            </label>
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
          <div className="grid gap-4 sm:grid-cols-2">
            <Input label="Name" value={constraintForm.name} onChange={(v) => setConstraintForm({ ...constraintForm, name: v })} placeholder="Max CPU per node" />
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300 pt-6">
              <input type="checkbox" checked={constraintForm.enabled} onChange={(e) => setConstraintForm({ ...constraintForm, enabled: e.target.checked })} className="rounded border-white/10 bg-[#161b28]" />
              Enabled
            </label>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Type</label>
              <select
                className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={constraintForm.type}
                onChange={(e) => setConstraintForm({ ...constraintForm, type: e.target.value as Constraint["type"] })}
              >
                <option value="cpu">CPU</option>
                <option value="memory">Memory</option>
                <option value="disk">Disk</option>
                <option value="ports">Ports</option>
                <option value="custom">Custom</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Operator</label>
              <select
                className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={constraintForm.operator}
                onChange={(e) => setConstraintForm({ ...constraintForm, operator: e.target.value as Constraint["operator"] })}
              >
                <option value="lt">Less Than (&lt;)</option>
                <option value="gt">Greater Than (&gt;)</option>
                <option value="eq">Equal (=)</option>
                <option value="neq">Not Equal (!=)</option>
                <option value="in">In</option>
              </select>
            </div>
            <Input label="Value" value={constraintForm.value} onChange={(v) => setConstraintForm({ ...constraintForm, value: v })} placeholder="80" />
          </div>
          <ModalFooter
            onCancel={() => setShowCreateConstraint(false)}
            onConfirm={() => createConstraintMutation.mutate()}
            confirmLabel={createConstraintMutation.isPending ? "Creating..." : "Create"}
            disabled={createConstraintMutation.isPending || !constraintForm.name}
          />
        </Modal>
      )}
    </div>
  );
}
