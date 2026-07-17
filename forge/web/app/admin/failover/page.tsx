"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, BarChart3, Plus, Shield, ShieldAlert, Trash2, Zap } from "lucide-react";
import { deleteJSON, fetchJSON, postJSON, putJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";

type ApiResponse<T> = { data: T };
type FailoverAction = "evacuate" | "restart" | "notify";

type FailoverPolicy = {
  id: string;
  name: string;
  nodeId: string;
  maxFailures: number;
  failureWindowSec: number;
  cooldownSec: number;
  action: FailoverAction;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

type FailoverMetrics = {
  failuresDetected: number;
  evacuationsTriggered: number;
  restartsTriggered: number;
  notificationsSent: number;
};

type PolicyForm = Pick<FailoverPolicy, "nodeId" | "maxFailures" | "failureWindowSec" | "action" | "enabled">;

const defaultForm: PolicyForm = {
  nodeId: "",
  maxFailures: 3,
  failureWindowSec: 300,
  action: "evacuate",
  enabled: true,
};

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export default function AdminFailoverPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<FailoverPolicy | null>(null);
  const [form, setForm] = useState<PolicyForm>(defaultForm);

  const policiesQuery = useQuery({
    queryKey: ["admin", "failover", "policies"],
    queryFn: () => fetchJSON<ApiResponse<FailoverPolicy[]>>("/admin/failover/policies"),
  });
  const metricsQuery = useQuery({
    queryKey: ["admin", "failover", "metrics"],
    queryFn: () => fetchJSON<ApiResponse<FailoverMetrics>>("/admin/failover/metrics"),
  });

  const policies = useMemo(() => Array.isArray(policiesQuery.data?.data) ? policiesQuery.data.data : [], [policiesQuery.data]);
  const metrics = metricsQuery.data?.data;
  const filtered = policies.filter((policy) =>
    !search || policy.nodeId.toLowerCase().includes(search.trim().toLowerCase()),
  );

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["admin", "failover"] });
  };
  const createMutation = useMutation({
    mutationFn: () => postJSON<ApiResponse<FailoverPolicy>>("/admin/failover/policies", form),
    onSuccess: () => {
      invalidate();
      setShowCreate(false);
      setForm(defaultForm);
    },
  });
  const updateMutation = useMutation({
    mutationFn: () => putJSON<ApiResponse<FailoverPolicy>>(`/admin/failover/policies/${editingPolicy!.id}`, form),
    onSuccess: () => {
      invalidate();
      setEditingPolicy(null);
      setForm(defaultForm);
    },
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/failover/policies/${id}`),
    onSuccess: invalidate,
  });
  const recordFailureMutation = useMutation({
    mutationFn: (nodeId: string) => postJSON(`/admin/failover/record-failure/${encodeURIComponent(nodeId)}`),
    onSuccess: invalidate,
  });

  const operationError = createMutation.error ?? updateMutation.error ?? deleteMutation.error ?? recordFailureMutation.error;
  const openEdit = (policy: FailoverPolicy) => {
    setEditingPolicy(policy);
    setForm({
      nodeId: policy.nodeId,
      maxFailures: policy.maxFailures,
      failureWindowSec: policy.failureWindowSec,
      action: policy.action,
      enabled: policy.enabled,
    });
  };
  const closeModal = () => {
    setShowCreate(false);
    setEditingPolicy(null);
    setForm(defaultForm);
  };

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Failover Policies"
        sub="Configure threshold-based recovery actions for node failures."
        action={<Btn tone="primary" onClick={() => setShowCreate(true)}><Plus size={14} /> Create Policy</Btn>}
      />

      <div className="grid gap-4 md:grid-cols-4">
        <MetricCard icon={Shield} label="Total Policies" value={policies.length} tone="text-slate-100" />
        <MetricCard icon={AlertTriangle} label="Failures Detected" value={metrics?.failuresDetected ?? 0} tone="text-amber-400" />
        <MetricCard icon={Zap} label="Evacuations" value={metrics?.evacuationsTriggered ?? 0} tone="text-blue-400" />
        <MetricCard icon={BarChart3} label="Restarts / Notices" value={`${metrics?.restartsTriggered ?? 0} / ${metrics?.notificationsSent ?? 0}`} tone="text-emerald-400" />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader title="Failover Policies" icon={ShieldAlert} />
          <div className="flex items-center gap-3 p-4">
            <Input placeholder="Search by Node ID" value={search} onChange={setSearch} />
          </div>
          {policiesQuery.isLoading ? (
            <div className="p-8 text-center text-sm text-slate-500">Loading policies...</div>
          ) : policiesQuery.isError ? (
            <div className="p-8 text-center text-sm text-red-300">{errorMessage(policiesQuery.error, "Could not load failover policies.")}</div>
          ) : filtered.length === 0 ? (
            <EmptyState icon={ShieldAlert} message={search ? "No policies match this node ID." : "No failover policies configured."} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead><tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Node ID</th><th className="px-4 py-3">Threshold</th><th className="px-4 py-3">Window</th><th className="px-4 py-3">Action</th><th className="px-4 py-3">Status</th><th className="px-4 py-3" />
                </tr></thead>
                <tbody className="divide-y divide-white/[0.04]">{filtered.map((policy) => (
                  <tr key={policy.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">{policy.nodeId}</td>
                    <td className="px-4 py-3 text-xs text-slate-400">{policy.maxFailures} failures</td>
                    <td className="px-4 py-3 text-xs text-slate-400">{policy.failureWindowSec}s</td>
                    <td className="px-4 py-3"><Pill tone={policy.action === "evacuate" ? "red" : policy.action === "restart" ? "yellow" : "blue"}>{policy.action}</Pill></td>
                    <td className="px-4 py-3"><Pill tone={policy.enabled ? "green" : "neutral"}>{policy.enabled ? "Enabled" : "Disabled"}</Pill></td>
                    <td className="px-4 py-3"><div className="flex gap-1">
                      <Btn size="sm" tone="ghost" onClick={() => openEdit(policy)}>Edit</Btn>
                      <Btn size="sm" tone="warning" onClick={() => recordFailureMutation.mutate(policy.nodeId)} disabled={recordFailureMutation.isPending}>Record failure</Btn>
                      <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete this failover policy?")) deleteMutation.mutate(policy.id); }} disabled={deleteMutation.isPending}><Trash2 size={12} /></Btn>
                    </div></td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title="Failover Metrics" icon={BarChart3} />
          {metricsQuery.isLoading ? (
            <div className="p-8 text-center text-sm text-slate-500">Loading metrics...</div>
          ) : metricsQuery.isError ? (
            <div className="p-8 text-center text-sm text-red-300">{errorMessage(metricsQuery.error, "Could not load failover metrics.")}</div>
          ) : (
            <div className="divide-y divide-white/[0.04]">
              <MetricRow label="Failures detected" value={metrics?.failuresDetected ?? 0} />
              <MetricRow label="Evacuations triggered" value={metrics?.evacuationsTriggered ?? 0} />
              <MetricRow label="Restarts triggered" value={metrics?.restartsTriggered ?? 0} />
              <MetricRow label="Notifications sent" value={metrics?.notificationsSent ?? 0} />
            </div>
          )}
          {operationError ? <div className="border-t border-white/[0.06] p-4 text-sm text-red-300">{errorMessage(operationError, "The failover operation could not be completed.")}</div> : null}
          <p className="border-t border-white/[0.06] p-4 text-xs text-slate-500">Recording a failure uses the configured node policy. Actions run only after its threshold is reached.</p>
        </Card>
      </div>

      {(showCreate || editingPolicy) && (
        <Modal title={showCreate ? "Create Failover Policy" : "Edit Failover Policy"} onClose={closeModal}>
          <div className="grid gap-4 sm:grid-cols-2">
            <Input label="Node ID" value={form.nodeId} onChange={(nodeId) => setForm({ ...form, nodeId })} placeholder="node_abc" required />
            <label className="flex items-center gap-2 pt-6 text-sm font-medium text-slate-300"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} className="rounded border-white/10 bg-[#161b28]" /> Enabled</label>
            <Input label="Max Failures" type="number" value={String(form.maxFailures)} onChange={(value) => setForm({ ...form, maxFailures: Number(value) })} />
            <Input label="Failure Window (seconds)" type="number" value={String(form.failureWindowSec)} onChange={(value) => setForm({ ...form, failureWindowSec: Number(value) })} />
            <div className="sm:col-span-2"><label className="mb-1.5 block text-sm font-medium text-slate-300">Action</label><select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30" value={form.action} onChange={(event) => setForm({ ...form, action: event.target.value as FailoverAction })}><option value="evacuate">Evacuate</option><option value="restart">Restart</option><option value="notify">Notify</option></select></div>
          </div>
          <ModalFooter onCancel={closeModal} onConfirm={() => showCreate ? createMutation.mutate() : updateMutation.mutate()} confirmLabel="Save" disabled={createMutation.isPending || updateMutation.isPending || !form.nodeId.trim() || form.maxFailures < 1 || form.failureWindowSec < 1} />
        </Modal>
      )}
    </div>
  );
}

function MetricCard({ icon: Icon, label, value, tone }: { icon: typeof Shield; label: string; value: string | number; tone: string }) {
  return <Card className="p-4"><div className="mb-1 flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500"><Icon size={12} /> {label}</div><div className={`text-2xl font-bold ${tone}`}>{value}</div></Card>;
}

function MetricRow({ label, value }: { label: string; value: number }) {
  return <div className="flex items-center justify-between px-4 py-3 text-sm"><span className="text-slate-400">{label}</span><span className="font-semibold text-slate-100">{value}</span></div>;
}
