"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Plus, Shield, ShieldAlert, Trash2, Zap } from "lucide-react";
import { deleteJSON, fetchJSON, postJSON, putJSON } from "@/lib/api";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  StatsRow,
} from "@/components/admin/admin-ui";
import type { AdminTone } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { NodeSelect } from "@/components/admin/node-select";
import { sourceState, worstSourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { formatDate } from "@/lib/utils";

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

/**
 * Mirrors `failover.Event` (forge/api/internal/services/failover/service.go:77).
 *
 * Both write endpoints answer `{"data": …}` and can answer **`null`**:
 * `RecordFailure` returns `nil, nil` when no enabled policy matches the node or the
 * failure threshold has not been reached (`:471`, `:491`), and the crash handler
 * does the same plus when the policy is inside its cooldown (`:505`, `:521`). A
 * 2xx with a null event therefore means "the control plane deliberately did
 * nothing" — which must never be drawn as a handled/success banner.
 */
type FailoverEvent = {
  id?: string;
  policyId?: string;
  nodeId: string;
  serverId?: string;
  eventType: string;
  action: string;
  status: string;
  message: string;
  timestamp?: string;
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

/** The action an operator is about to cause, phrased for a confirmation dialog. */
const ACTION_PHRASE: Record<FailoverAction, string> = {
  evacuate: "evacuate the node's workloads to other nodes",
  restart: "restart the affected workload",
  notify: "raise a notification only",
};

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function actionTone(action: string): AdminTone {
  if (action === "evacuate") return "red";
  if (action === "restart") return "yellow";
  if (action === "notify") return "blue";
  return "unknown";
}

/**
 * What one of the two failover writes actually did, read from the response body.
 * `null` is a real answer here ("no policy matched / threshold not reached /
 * cooling down"), so the three cases stay separate and only one of them is green.
 */
type WriteOutcome = { tone: AdminTone; title: string; detail: string; event?: FailoverEvent };

function describeOutcome(event: FailoverEvent | null | undefined, kind: "crash" | "record"): WriteOutcome {
  if (event === null || event === undefined) {
    return {
      tone: "unknown",
      title: kind === "crash" ? "No failover action was taken" : "Failure recorded, no action taken",
      detail:
        kind === "crash"
          ? "The request was accepted, but no enabled failover policy matched this node or the policy is inside its cooldown window. Nothing was evacuated, restarted or notified."
          : "The request was accepted. Either no enabled policy matched this node, or the failure count is still below the policy threshold, so no action ran.",
    };
  }
  if (event.status === "failed") {
    return { tone: "red", title: "Failover action failed", detail: event.message || "The action was attempted and reported an error.", event };
  }
  if (event.status === "completed") {
    return { tone: "green", title: `Failover action ${event.action} completed`, detail: event.message || "", event };
  }
  // "evacuating" / "restarting" / "notified" are in-flight or one-way statuses.
  return {
    tone: event.status === "notified" ? "blue" : "yellow",
    title: `Failover action ${event.action}: ${event.status}`,
    detail: event.message || "The action has been triggered; completion is reported by later events.",
    event,
  };
}

function OutcomePanel({ outcome }: { outcome: WriteOutcome }) {
  const cls =
    outcome.tone === "green" ? "ui-alert ui-alert-success" :
    outcome.tone === "red" ? "ui-alert ui-alert-danger" :
    outcome.tone === "yellow" ? "ui-alert ui-alert-warning" :
    "ui-alert";
  return (
    <div className={cls}>
      <div className="min-w-0 space-y-1">
        <p className="text-xs font-semibold">{outcome.title}</p>
        {outcome.detail ? <p className="break-words">{outcome.detail}</p> : null}
        {outcome.event ? (
          <dl className="mt-1 space-y-0.5">
            <div className="flex justify-between gap-2"><dt className="text-text-subtle">Node</dt><dd className="font-mono">{outcome.event.nodeId || "—"}</dd></div>
            {outcome.event.serverId ? <div className="flex justify-between gap-2"><dt className="text-text-subtle">Server</dt><dd className="font-mono">{outcome.event.serverId}</dd></div> : null}
            <div className="flex justify-between gap-2"><dt className="text-text-subtle">Event</dt><dd className="font-mono">{outcome.event.eventType || "—"}</dd></div>
            <div className="flex justify-between gap-2"><dt className="text-text-subtle">Status</dt><dd className="font-mono">{outcome.event.status || "—"}</dd></div>
            <div className="flex justify-between gap-2"><dt className="text-text-subtle">Recorded</dt><dd className="font-mono">{outcome.event.timestamp ? formatDate(outcome.event.timestamp) : "not reported"}</dd></div>
          </dl>
        ) : null}
      </div>
    </div>
  );
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
  const [crashOutcome, setCrashOutcome] = useState<WriteOutcome | null>(null);
  const [recordOutcome, setRecordOutcome] = useState<WriteOutcome | null>(null);

  const policiesQuery = useQuery({
    queryKey: ["admin", "failover", "policies"],
    queryFn: () => fetchJSON<ApiResponse<FailoverPolicy[]>>("/admin/failover/policies"),
    retry: false,
  });
  const metricsQuery = useQuery({
    queryKey: ["admin", "failover", "metrics"],
    queryFn: () => fetchJSON<ApiResponse<FailoverMetrics>>("/admin/failover/metrics"),
    retry: false,
  });

  const policies = useMemo(
    () => (Array.isArray(policiesQuery.data?.data) ? policiesQuery.data.data : []),
    [policiesQuery.data],
  );
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
  });
  const updateMutation = useMutation({
    mutationFn: () => putJSON<ApiResponse<FailoverPolicy>>(`/admin/failover/policies/${editingPolicy!.id}`, form),
    onSuccess: () => {
      invalidate();
      setEditingPolicy(null);
      setForm(defaultForm);
      toast({ tone: "success", title: "Failover policy updated" });
    },
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/failover/policies/${encodeURIComponent(id)}`),
    onSuccess: () => { invalidate(); toast({ tone: "success", title: "Failover policy deleted" }); },
  });

  // `POST /record-failure/:nodeId` appends to the node's failure window and may
  // trigger the policy action. It is a state write, so it is confirmed and its
  // answer is reported — a click used to invalidate the cache and say nothing.
  const recordFailureMutation = useMutation({
    mutationFn: (nodeId: string) => postJSON<ApiResponse<FailoverEvent | null>>(`/admin/failover/record-failure/${encodeURIComponent(nodeId)}`),
    onSuccess: (res) => {
      setRecordOutcome(describeOutcome(res?.data, "record"));
      invalidate();
    },
    onError: (e) => toast({ tone: "error", title: "Could not record the failure", message: errorMessage(e, "request failed") }),
  });

  /**
   * `POST /admin/failover/crash/:serverId/:nodeId` is **not** a simulation: the
   * handler calls the real `HandleServerCrash` recovery path with `failover.write`
   * scope (handlers_failover.go:82). The card is labelled accordingly and every
   * click goes through `useConfirm` with the matching policy's blast radius.
   */
  const crashMutation = useMutation({
    mutationFn: ({ serverId, nodeId }: { serverId: string; nodeId: string }) =>
      postJSON<ApiResponse<FailoverEvent | null>>(`/admin/failover/crash/${encodeURIComponent(serverId)}/${encodeURIComponent(nodeId)}`),
    onSuccess: (data) => { setCrashOutcome(describeOutcome(data?.data, "crash")); setRecordOutcome(null); invalidate(); },
    onError: (e) => {
      setCrashOutcome({ tone: "red", title: "Failover request rejected", detail: errorMessage(e, "The control plane did not accept the crash report.") });
      invalidate();
    },
  });

  const operationError = createMutation.error ?? updateMutation.error ?? deleteMutation.error;

  const lookupPolicy = async () => {
    setLookupPolicyError(null); setLookupPolicyResult(null);
    if (!lookupPolicyId.trim()) { setLookupPolicyError("Policy ID required"); return; }
    try {
      const res = await fetchJSON<ApiResponse<FailoverPolicy>>(`/admin/failover/policies/${encodeURIComponent(lookupPolicyId.trim())}`);
      setLookupPolicyResult(res.data);
    } catch (e) { setLookupPolicyError(errorMessage(e, "Failed to fetch policy")); }
  };
  const lookupByNode = async () => {
    setNodeLookupError(null); setNodePolicies(null);
    if (!nodeLookupId.trim()) { setNodeLookupError("Node ID required"); return; }
    try {
      const res = await fetchJSON<ApiResponse<FailoverPolicy[]>>(`/admin/failover/policies/node/${encodeURIComponent(nodeLookupId.trim())}`);
      setNodePolicies(res.data);
    } catch (e) { setNodeLookupError(errorMessage(e, "Failed to fetch policies")); }
  };

  // Which policy would actually run — read from the list we already have, so the
  // confirmation states a real consequence instead of a generic warning.
  const crashMatchedPolicies = useMemo(() => {
    const id = crashNodeId.trim();
    if (!id) return [];
    return policies.filter((p) => p.nodeId === id && p.enabled);
  }, [policies, crashNodeId]);

  const confirmCrash = () => {
    void (async () => {
      const serverId = crashServerId.trim();
      const nodeId = crashNodeId.trim();
      if (!serverId || !nodeId) {
        setCrashOutcome({ tone: "red", title: "Server ID and node ID are both required", detail: "Nothing was sent: an ambiguous target is rejected rather than guessed." });
        return;
      }
      const matched = crashMatchedPolicies;
      const radius = matched.length > 0
        ? `Enabled policies on this node: ${matched.map((p) => `${p.action} (after ${p.maxFailures} failures in ${p.failureWindowSec}s, cooldown ${p.cooldownSec}s)`).join("; ")}. At least one will ${ACTION_PHRASE[matched[0]!.action]}.`
        : "No enabled failover policy matches this node, so the crash will be recorded and no recovery action will run.";
      const ok = await confirm({
        title: "Report a real server crash?",
        description: `This is not a simulation. The control plane runs its failover recovery path for server ${serverId} on node ${nodeId}. ${radius} If the policy is in its cooldown window the action is skipped.`,
        confirmLabel: "Report crash and run failover",
        danger: true,
      });
      if (ok) { setCrashOutcome(null); crashMutation.mutate({ serverId, nodeId }); }
    })();
  };

  const confirmRecordFailure = (nodeId: string) => {
    void (async () => {
      const matched = policies.filter((p) => p.nodeId === nodeId && p.enabled);
      const ok = await confirm({
        title: `Record a failure for ${nodeId}?`,
        description: matched.length > 0
          ? `This appends a failure to the node's ${matched[0]!.failureWindowSec}s window. At ${matched[0]!.maxFailures} failures the policy runs "${matched[0]!.action}". This is a live state write, not a test.`
          : `This appends a failure for ${nodeId}. No enabled policy matches this node, so nothing will be triggered — the count is not tracked and the list here will not change.`,
        confirmLabel: "Record failure",
        danger: true,
      });
      if (ok) { setRecordOutcome(null); recordFailureMutation.mutate(nodeId); }
    })();
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
      <AdminPageHeader
        status={<FreshnessBadge state={worstSourceState([sourceState(policiesQuery, 30_000), sourceState(metricsQuery, 30_000)])} />}
        action={<Btn tone="primary" onClick={() => setShowCreate(true)}><Plus size={14} /> Create Policy</Btn>}
      />

      <StatsRow
        items={[
          { label: "Total policies", value: policiesQuery.isPending || policiesQuery.isError ? "—" : policies.length, icon: Shield },
          { label: "Failures detected", value: metrics?.failuresDetected ?? "—", icon: AlertTriangle, tone: "yellow" },
          { label: "Evacuations", value: metrics?.evacuationsTriggered ?? "—", icon: Zap, tone: "blue" },
          { label: "Restarts / notices", value: metrics ? `${metrics.restartsTriggered} / ${metrics.notificationsSent}` : "—", icon: ShieldAlert },
        ]}
      />

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader title="Failover Policies" icon={ShieldAlert} />
          <div className="mb-4">
            <Input label="Search by node ID" value={search} onChange={setSearch} placeholder="node UUID" />
          </div>
          {policiesQuery.isPending ? (
            <AdminLoadingState label="Loading policies…" />
          ) : policiesQuery.isError ? (
            <AdminErrorState message={errorMessage(policiesQuery.error, "Could not load failover policies.")} retry={() => void policiesQuery.refetch()} />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={ShieldAlert}
              title="No failover policies"
              message={search ? `No policy matches the node ID “${search.trim()}”.` : "No failover policies are configured, so no automatic recovery will run for any node."}
            />
          ) : (
            <AdminTable label="Failover policies">
              <AdminTHead>
                <AdminTh>Node ID</AdminTh>
                <AdminTh>Threshold</AdminTh>
                <AdminTh>Window</AdminTh>
                <AdminTh>Action</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {filtered.map((policy) => (
                  <AdminTr key={policy.id}>
                    <AdminTd className="font-mono text-xs text-text">{policy.nodeId}</AdminTd>
                    <AdminTd className="text-meta text-text-subtle">{policy.maxFailures} failures</AdminTd>
                    <AdminTd className="text-meta text-text-subtle">{policy.failureWindowSec}s</AdminTd>
                    <AdminTd><Pill tone={actionTone(policy.action)}>{policy.action}</Pill></AdminTd>
                    <AdminTd><Pill tone={policy.enabled ? "green" : "neutral"}>{policy.enabled ? "Enabled" : "Disabled"}</Pill></AdminTd>
                    <AdminTd>
                      <div className="flex gap-1">
                        <Btn size="sm" tone="ghost" onClick={() => openEdit(policy)}>Edit</Btn>
                        <Btn size="sm" tone="warning" onClick={() => confirmRecordFailure(policy.nodeId)} disabled={recordFailureMutation.isPending}>Record failure</Btn>
                        <Btn
                          size="sm"
                          tone="danger"
                          ariaLabel={`Delete the failover policy for ${policy.nodeId}`}
                          onClick={() => {
                            void (async () => {
                              const ok = await confirm({
                                title: `Delete failover policy for ${policy.nodeId}?`,
                                description: `Automatic ${policy.action} failover stops for this node; failures will still be reported but no recovery action will run. This cannot be undone.`,
                                danger: true,
                                confirmLabel: "Delete",
                              });
                              if (ok) deleteMutation.mutate(policy.id);
                            })();
                          }}
                          disabled={deleteMutation.isPending}
                        >
                          <Trash2 size={12} />
                        </Btn>
                      </div>
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>

        <Card>
          <CardHeader title="Failover Metrics" icon={ShieldAlert} />
          {metricsQuery.isPending ? (
            <AdminLoadingState label="Loading metrics…" />
          ) : metricsQuery.isError ? (
            <AdminErrorState message={errorMessage(metricsQuery.error, "Could not load failover metrics.")} retry={() => void metricsQuery.refetch()} />
          ) : (
            <div className="divide-y divide-line">
              <MetricRow label="Failures detected" value={metrics?.failuresDetected} />
              <MetricRow label="Evacuations triggered" value={metrics?.evacuationsTriggered} />
              <MetricRow label="Restarts triggered" value={metrics?.restartsTriggered} />
              <MetricRow label="Notifications sent" value={metrics?.notificationsSent} />
            </div>
          )}
          {recordOutcome ? <div className="border-t border-line p-4"><OutcomePanel outcome={recordOutcome} /></div> : null}
          {operationError ? (
            <div className="border-t border-line p-4">
              <AdminErrorState message={errorMessage(operationError, "The failover operation could not be completed.")} />
            </div>
          ) : null}
          <p className="border-t border-line p-4 text-meta leading-5 text-text-subtle">
            Counters are process-wide and reset when the API restarts. Recording a failure uses the configured node policy; an
            action runs only once its threshold is reached inside the window.
          </p>
        </Card>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader title="Policy lookup" icon={Shield} />
          <div className="space-y-3 p-4">
            <p className="text-meta text-text-subtle">Fetch a single policy by its ID.</p>
            <div className="flex gap-2">
              <Input label="Policy ID" value={lookupPolicyId} onChange={setLookupPolicyId} placeholder="policy UUID" />
              <Btn className="self-end" tone="primary" onClick={() => void lookupPolicy()}>Fetch</Btn>
            </div>
            {lookupPolicyError && <p className="text-meta text-danger">{lookupPolicyError}</p>}
            {lookupPolicyResult && <PolicySummary policy={lookupPolicyResult} />}
          </div>
        </Card>
        <Card>
          <CardHeader title="Policies by node" icon={ShieldAlert} />
          <div className="space-y-3 p-4">
            <p className="text-meta text-text-subtle">List every policy attached to one node.</p>
            <div className="flex gap-2">
              <Input label="Node ID" value={nodeLookupId} onChange={setNodeLookupId} placeholder="node UUID" />
              <Btn className="self-end" tone="primary" onClick={() => void lookupByNode()}>Fetch</Btn>
            </div>
            {nodeLookupError && <p className="text-meta text-danger">{nodeLookupError}</p>}
            {nodePolicies && (
              <div className="max-h-48 space-y-2 overflow-y-auto">
                {nodePolicies.length === 0 ? (
                  <EmptyState icon={ShieldAlert} title="No policies for this node" message="This node has no failover policies attached, so a crash here recovers nothing automatically." />
                ) : (
                  nodePolicies.map((p) => (
                    <div key={p.id} className="space-y-1 rounded-lg border border-line bg-overlay-subtle p-2 text-meta text-text">
                      <div className="flex items-center gap-2">
                        <Pill tone={actionTone(p.action)}>{p.action}</Pill>
                        <Pill tone={p.enabled ? "green" : "neutral"}>{p.enabled ? "Enabled" : "Disabled"}</Pill>
                      </div>
                      <div className="text-text-subtle">{p.maxFailures} failures / {p.failureWindowSec}s window · cooldown {p.cooldownSec}s</div>
                    </div>
                  ))
                )}
              </div>
            )}
          </div>
        </Card>
        <Card>
          <CardHeader title="Report a server crash" icon={AlertTriangle} action={<Pill tone="red">Live write</Pill>} />
          <div className="space-y-3 p-4">
            <p className="text-meta leading-5 text-text-subtle">
              Runs the real crash-recovery path for one server on one node — the same code the health watcher calls. There is no
              dry run: if an enabled policy matches, its action executes.
            </p>
            <Input label="Server ID" value={crashServerId} onChange={setCrashServerId} placeholder="server UUID" />
            <div>
              <span className="ui-label mb-1.5">Node</span>
              <NodeSelect label="Node" onChange={setCrashNodeId} value={crashNodeId} />
            </div>
            <p className="text-meta text-text-subtle">
              {crashNodeId.trim()
                ? `${crashMatchedPolicies.length} enabled ${crashMatchedPolicies.length === 1 ? "policy matches" : "policies match"} node ${crashNodeId.trim()}.`
                : "Choose a node to see which policy would act."}
            </p>
            {crashOutcome ? <OutcomePanel outcome={crashOutcome} /> : null}
            <Btn
              tone="danger"
              onClick={confirmCrash}
              disabled={crashMutation.isPending || !crashServerId.trim() || !crashNodeId.trim()}
            >
              {crashMutation.isPending ? "Reporting…" : "Report crash & run failover"}
            </Btn>
          </div>
        </Card>
      </div>

      {(showCreate || editingPolicy) && (
        <Modal title={showCreate ? "Create Failover Policy" : "Edit Failover Policy"} onClose={closeModal}>
          <div className="space-y-4">
            <div>
              <NodeSelect label="Target node" onChange={(nodeId) => setForm({ ...form, nodeId })} value={form.nodeId} />
              {editingPolicy && editingPolicy.nodeId !== form.nodeId ? (
                <p className="ui-alert ui-alert-warning mt-2">
                  Editing the node ID changes which host this policy guards; the existing policy row is updated in place rather
                  than a new one being created.
                </p>
              ) : null}
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="flex items-center gap-2 text-sm font-medium text-text">
                <input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} />
                Enabled
              </label>
              <Input label="Max failures" type="number" value={String(form.maxFailures)} onChange={(value) => setForm({ ...form, maxFailures: Number(value) })} />
              <Input label="Failure window (seconds)" type="number" value={String(form.failureWindowSec)} onChange={(value) => setForm({ ...form, failureWindowSec: Number(value) })} />
              <Input label="Cooldown (seconds)" type="number" value={String(form.cooldownSec)} onChange={(value) => setForm({ ...form, cooldownSec: Number(value) })} />
              <div className="sm:col-span-2">
                <AdminSelect
                  label="Action"
                  value={form.action}
                  onChange={(value) => setForm({ ...form, action: value as FailoverAction })}
                  options={[
                    { value: "evacuate", label: `Evacuate — ${ACTION_PHRASE.evacuate}` },
                    { value: "restart", label: `Restart — ${ACTION_PHRASE.restart}` },
                    { value: "notify", label: `Notify — ${ACTION_PHRASE.notify}` },
                  ]}
                />
              </div>
            </div>
          </div>
          <ModalFooter
            onCancel={closeModal}
            onConfirm={() => (showCreate ? createMutation.mutate() : updateMutation.mutate())}
            confirmLabel="Save"
            disabled={
              createMutation.isPending || updateMutation.isPending ||
              !form.nodeId.trim() || form.maxFailures < 1 || form.failureWindowSec < 1 || form.cooldownSec < 1
            }
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

function PolicySummary({ policy }: { policy: FailoverPolicy }) {
  return (
    <div className="space-y-2 rounded-lg border border-line bg-[var(--surface-raised)] p-3 text-xs text-text">
      <div className="flex items-center gap-2">
        <Pill tone={actionTone(policy.action)}>{policy.action}</Pill>
        <Pill tone={policy.enabled ? "green" : "neutral"}>{policy.enabled ? "Enabled" : "Disabled"}</Pill>
      </div>
      <dl className="divide-y divide-line space-y-1">
        <div className="flex justify-between gap-2"><dt className="text-text-subtle">Node</dt><dd className="font-mono">{policy.nodeId}</dd></div>
        <div className="flex justify-between gap-2"><dt className="text-text-subtle">Threshold</dt><dd>{policy.maxFailures} failures / {policy.failureWindowSec}s</dd></div>
        <div className="flex justify-between gap-2"><dt className="text-text-subtle">Cooldown</dt><dd>{policy.cooldownSec}s</dd></div>
        <div className="flex justify-between gap-2"><dt className="text-text-subtle">Updated</dt><dd>{formatDate(policy.updatedAt, "not reported")}</dd></div>
      </dl>
    </div>
  );
}

function MetricRow({ label, value }: { label: string; value: number | undefined }) {
  return (
    <div className="flex items-center justify-between px-4 py-3 text-sm">
      <span className="text-text-subtle">{label}</span>
      <span className="font-semibold text-text">{typeof value === "number" ? value : "Not reported"}</span>
    </div>
  );
}
