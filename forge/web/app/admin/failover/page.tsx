"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, BarChart3, Plus, Shield, ShieldAlert, Trash2, Zap } from "lucide-react";
import { deleteJSON, fetchJSON, postJSON, putJSON } from "@/lib/api";
import { AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminErrorState, AdminLoadingState } from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";

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

type PolicyForm = Pick<FailoverPolicy, "nodeId" | "maxFailures" | "failureWindowSec" | "cooldownSec" | "action" | "enabled">;

const defaultForm: PolicyForm = {
  nodeId: "",
  maxFailures: 3,
  failureWindowSec: 300,
  cooldownSec: 600,
  action: "evacuate",
  enabled: true,
};

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export default function AdminFailoverPage() {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const [search, setSearch] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<FailoverPolicy | null>(null);
  const [form, setForm] = useState<PolicyForm>(defaultForm);
  const [lookupPolicyId, setLookupPolicyId] = useState("");
  const [lookupPolicyResult, setLookupPolicyResult] = useState<FailoverPolicy | null>(null);
  const [lookupPolicyError, setLookupPolicyError] = useState<string | null>(null);
  const [nodeLookupId, setNodeLookupId] = useState("");
  const [nodePolicies, setNodePolicies] = useState<FailoverPolicy[] | null>(null);
  const [nodeLookupError, setNodeLookupError] = useState<string | null>(null);
  const [crashServerId, setCrashServerId] = useState("");
  const [crashNodeId, setCrashNodeId] = useState("");
  const [crashResult, setCrashResult] = useState<unknown>(null);
  const [crashError, setCrashError] = useState<string | null>(null);

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
      toast({ tone: "success", title: "Failover policy created" });
    },
    onError: (e) => toast({ tone: "error", title: "Failed to create policy", message: errorMessage(e, "The policy could not be created.") }),
  });
  const updateMutation = useMutation({
    mutationFn: () => putJSON<ApiResponse<FailoverPolicy>>(`/admin/failover/policies/${editingPolicy!.id}`, form),
    onSuccess: () => {
      invalidate();
      setEditingPolicy(null);
      setForm(defaultForm);
      toast({ tone: "success", title: "Failover policy updated" });
    },
    onError: (e) => toast({ tone: "error", title: "Failed to update policy", message: errorMessage(e, "The policy could not be updated.") }),
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/failover/policies/${encodeURIComponent(id)}`),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Failover policy deleted" }); },
    onError: (e) => toast({ tone: "error", title: "Failed to delete policy", message: errorMessage(e, "The policy could not be deleted.") }),
  });
  const recordFailureMutation = useMutation({
    mutationFn: (nodeId: string) => postJSON(`/admin/failover/record-failure/${encodeURIComponent(nodeId)}`),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Failure recorded" }); },
    onError: (e) => toast({ tone: "error", title: "Failed to record failure", message: errorMessage(e, "The failure could not be recorded.") }),
  });
  const crashMutation = useMutation({
    mutationFn: ({ serverId, nodeId }: { serverId: string; nodeId: string }) =>
      postJSON(`/admin/failover/crash/${encodeURIComponent(serverId)}/${encodeURIComponent(nodeId)}`),
    onSuccess: (data) => { setCrashResult(data); setCrashError(null); invalidate(); toast({ tone: "success", title: "Crash simulation processed" }); },
    onError: (e) => { setCrashError(e instanceof Error ? e.message : "Crash simulation failed"); toast({ tone: "error", title: "Crash simulation failed", message: errorMessage(e, "Crash simulation failed") }); },
  });

  const operationError = createMutation.error ?? updateMutation.error ?? deleteMutation.error ?? recordFailureMutation.error ?? crashMutation.error;

  const lookupPolicy = async () => {
    setLookupPolicyError(null); setLookupPolicyResult(null);
    if (!lookupPolicyId.trim()) { setLookupPolicyError("Policy ID required"); return; }
    try {
      const res = await fetchJSON<ApiResponse<FailoverPolicy>>(`/admin/failover/policies/${encodeURIComponent(lookupPolicyId.trim())}`);
      setLookupPolicyResult(res.data);
    } catch (e) { setLookupPolicyError(e instanceof Error ? e.message : "Failed to fetch policy"); }
  };
  const lookupByNode = async () => {
    setNodeLookupError(null); setNodePolicies(null);
    if (!nodeLookupId.trim()) { setNodeLookupError("Node ID required"); return; }
    try {
      const res = await fetchJSON<ApiResponse<FailoverPolicy[]>>(`/admin/failover/policies/node/${encodeURIComponent(nodeLookupId.trim())}`);
      setNodePolicies(res.data);
    } catch (e) { setNodeLookupError(e instanceof Error ? e.message : "Failed to fetch policies"); }
  };
  const triggerCrash = () => {
    setCrashError(null); setCrashResult(null);
    if (!crashServerId.trim() || !crashNodeId.trim()) { setCrashError("Server ID and Node ID required"); return; }
    crashMutation.mutate({ serverId: crashServerId.trim(), nodeId: crashNodeId.trim() });
  };
  const openEdit = (policy: FailoverPolicy) => {
    setEditingPolicy(policy);
    setForm({
      nodeId: policy.nodeId,
      maxFailures: policy.maxFailures,
      failureWindowSec: policy.failureWindowSec,
      cooldownSec: policy.cooldownSec,
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
    <AdminPageLayout>
      <SectionHeader
        title="Failover"
        sub="Automatic failover and recovery behaviour"
        action={<Btn tone="primary" onClick={() => setShowCreate(true)}><Plus size={14} /> Create Policy</Btn>}
      />

      <div className="grid gap-4 md:grid-cols-4">
        <MetricCard icon={Shield} label="Total Policies" value={policies.length} tone="text-text" />
        <MetricCard icon={AlertTriangle} label="Failures Detected" value={metrics?.failuresDetected ?? 0} tone="text-warn" />
        <MetricCard icon={Zap} label="Evacuations" value={metrics?.evacuationsTriggered ?? 0} tone="text-brand" />
        <MetricCard icon={BarChart3} label="Restarts / Notices" value={`${metrics?.restartsTriggered ?? 0} / ${metrics?.notificationsSent ?? 0}`} tone="text-ok" />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader title="Failover Policies" icon={ShieldAlert} />
          <div className="flex items-center gap-3 p-4">
            <Input placeholder="Search by Node ID" value={search} onChange={setSearch} />
          </div>
          {policiesQuery.isLoading ? (
            <AdminLoadingState label="Loading policies..." />
          ) : policiesQuery.isError ? (
            <div className="p-4"><AdminErrorState message={errorMessage(policiesQuery.error, "Could not load failover policies.")} retry={() => void policiesQuery.refetch()} /></div>
          ) : filtered.length === 0 ? (
            <EmptyState icon={ShieldAlert} message={search ? "No policies match this node ID." : "No failover policies configured."} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead><tr className="border-b border-line text-left text-[10px] uppercase tracking-widest text-text-muted">
                  <th className="px-4 py-3">Node ID</th><th className="px-4 py-3">Threshold</th><th className="px-4 py-3">Window</th><th className="px-4 py-3">Action</th><th className="px-4 py-3">Status</th><th className="px-4 py-3" />
                </tr></thead>
                <tbody className="divide-y divide-line">{filtered.map((policy) => (
                  <tr key={policy.id} className="hover:bg-overlay-subtle">
                    <td className="px-4 py-3 font-mono text-xs font-medium text-text">{policy.nodeId}</td>
                    <td className="px-4 py-3 text-xs text-text-subtle">{policy.maxFailures} failures</td>
                    <td className="px-4 py-3 text-xs text-text-subtle">{policy.failureWindowSec}s</td>
                    <td className="px-4 py-3"><Pill tone={policy.action === "evacuate" ? "red" : policy.action === "restart" ? "yellow" : "blue"}>{policy.action}</Pill></td>
                    <td className="px-4 py-3"><Pill tone={policy.enabled ? "green" : "neutral"}>{policy.enabled ? "Enabled" : "Disabled"}</Pill></td>
                    <td className="px-4 py-3"><div className="flex gap-1">
                      <Btn size="sm" tone="ghost" onClick={() => openEdit(policy)}>Edit</Btn>
                      <Btn size="sm" tone="warning" onClick={() => recordFailureMutation.mutate(policy.nodeId)} disabled={recordFailureMutation.isPending}>Record failure</Btn>
                      <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete failover policy for ${policy.nodeId}?`, description: "Automatic failover for this node will stop. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(policy.id); })(); }} disabled={deleteMutation.isPending}><Trash2 size={12} /></Btn>
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
            <AdminLoadingState label="Loading metrics..." />
          ) : metricsQuery.isError ? (
            <div className="p-4"><AdminErrorState message={errorMessage(metricsQuery.error, "Could not load failover metrics.")} retry={() => void metricsQuery.refetch()} /></div>
          ) : (
            <div className="divide-y divide-line">
              <MetricRow label="Failures detected" value={metrics?.failuresDetected ?? 0} />
              <MetricRow label="Evacuations triggered" value={metrics?.evacuationsTriggered ?? 0} />
              <MetricRow label="Restarts triggered" value={metrics?.restartsTriggered ?? 0} />
              <MetricRow label="Notifications sent" value={metrics?.notificationsSent ?? 0} />
            </div>
          )}
          {operationError ? <div className="border-t border-line p-4 text-sm text-danger">{errorMessage(operationError, "The failover operation could not be completed.")}</div> : null}
          <p className="border-t border-line p-4 text-xs text-text-muted">Recording a failure uses the configured node policy. Actions run only after its threshold is reached.</p>
        </Card>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader title="Lookup Policy" icon={Shield} />
          <div className="p-4 space-y-3">
            <p className="text-xs text-text-subtle">GET /admin/failover/policies/:id</p>
            <div className="flex gap-2">
              <Input placeholder="policy ID" value={lookupPolicyId} onChange={setLookupPolicyId} />
              <Btn tone="primary" onClick={lookupPolicy}>Fetch</Btn>
            </div>
            {lookupPolicyError && <p className="text-xs text-danger">{lookupPolicyError}</p>}
            {lookupPolicyResult && (
              <div className="rounded-lg border border-line bg-[var(--surface-raised)] p-3 text-xs font-mono text-text space-y-1">
                <div>id: {lookupPolicyResult.id}</div>
                <div>nodeId: {lookupPolicyResult.nodeId}</div>
                <div>action: {lookupPolicyResult.action}</div>
                <div>maxFailures: {lookupPolicyResult.maxFailures}</div>
                <div>enabled: {String(lookupPolicyResult.enabled)}</div>
              </div>
            )}
          </div>
        </Card>
        <Card>
          <CardHeader title="Policies by Node" icon={ShieldAlert} />
          <div className="p-4 space-y-3">
            <p className="text-xs text-text-subtle">GET /admin/failover/policies/node/:nodeId</p>
            <div className="flex gap-2">
              <Input placeholder="node ID" value={nodeLookupId} onChange={setNodeLookupId} />
              <Btn tone="primary" onClick={lookupByNode}>Fetch</Btn>
            </div>
            {nodeLookupError && <p className="text-xs text-danger">{nodeLookupError}</p>}
            {nodePolicies && (
              <div className="space-y-2 max-h-48 overflow-y-auto">
                {nodePolicies.length === 0 ? <p className="text-xs text-text-subtle">No policies for this node.</p> : nodePolicies.map((p) => (
                  <div key={p.id} className="rounded border border-line bg-overlay-subtle p-2 text-xs text-text">
                    <div className="font-mono">{p.id}</div>
                    <div>{p.action} — {p.maxFailures} failures / {p.failureWindowSec}s</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>
        <Card>
          <CardHeader title="Simulate Crash" icon={AlertTriangle} />
          <div className="p-4 space-y-3">
            <p className="text-xs text-text-subtle">POST /admin/failover/crash/:serverId/:nodeId</p>
            <Input label="Server ID" value={crashServerId} onChange={setCrashServerId} placeholder="server UUID" />
            <Input label="Node ID" value={crashNodeId} onChange={setCrashNodeId} placeholder="node UUID" />
            {crashError && <p className="text-xs text-danger">{crashError}</p>}
            {crashResult ? <p className="text-xs text-ok">Crash handled — {String(JSON.stringify(crashResult)).slice(0, 200)}</p> : null}
            <Btn tone="primary" onClick={triggerCrash} disabled={crashMutation.isPending}>{crashMutation.isPending ? "Processing..." : "Trigger Crash"}</Btn>
          </div>
        </Card>
      </div>

      {(showCreate || editingPolicy) && (
        <Modal title={showCreate ? "Create Failover Policy" : "Edit Failover Policy"} onClose={closeModal}>
          <div className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Input label="Node ID" value={form.nodeId} onChange={(nodeId) => setForm({ ...form, nodeId })} placeholder="node_abc" required />
            <label className="flex items-center gap-2 pt-6 text-sm font-medium text-text-subtle"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} className="rounded border-line bg-[var(--surface-input)]" /> Enabled</label>
            <Input label="Max Failures" type="number" value={String(form.maxFailures)} onChange={(value) => setForm({ ...form, maxFailures: Number(value) })} />
            <Input label="Failure Window (seconds)" type="number" value={String(form.failureWindowSec)} onChange={(value) => setForm({ ...form, failureWindowSec: Number(value) })} />
            <Input label="Cooldown (seconds)" type="number" value={String(form.cooldownSec)} onChange={(value) => setForm({ ...form, cooldownSec: Number(value) })} />
            <div className="sm:col-span-2"><label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Action</label><select className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none focus:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] focus:ring-1 focus:ring-[color-mix(in_srgb,var(--brand)_30%,transparent)]" value={form.action} onChange={(event) => setForm({ ...form, action: event.target.value as FailoverAction })}><option value="evacuate">Evacuate</option><option value="restart">Restart</option><option value="notify">Notify</option></select></div>
          </div>
          <p className="text-xs text-text-subtle">Backend: POST /admin/failover/policies creates, PUT /policies/:id updates, GET/:id and GET /policies/node/:nodeId wired via lookup cards. Crash endpoint POST /crash/:serverId/:nodeId wired.</p>
          </div>
          <ModalFooter onCancel={closeModal} onConfirm={() => showCreate ? createMutation.mutate() : updateMutation.mutate()} confirmLabel="Save" disabled={createMutation.isPending || updateMutation.isPending || !form.nodeId.trim() || form.maxFailures < 1 || form.failureWindowSec < 1 || form.cooldownSec < 1} />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

function MetricCard({ icon: Icon, label, value, tone }: { icon: typeof Shield; label: string; value: string | number; tone: string }) {
  return <Card className="p-4"><div className="mb-1 flex items-center gap-2 text-xs uppercase tracking-wider text-text-muted"><Icon size={12} /> {label}</div><div className={`text-2xl font-bold ${tone}`}>{value}</div></Card>;
}

function MetricRow({ label, value }: { label: string; value: number }) {
  return <div className="flex items-center justify-between px-4 py-3 text-sm"><span className="text-text-subtle">{label}</span><span className="font-semibold text-text">{value}</span></div>;
}
