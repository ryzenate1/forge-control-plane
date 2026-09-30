"use client";

import { useEffect, useMemo, useState } from "react";
import { Clock, Plus, RefreshCw, Trash2, TrendingUp } from "lucide-react";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  AdminSelect,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Pill,
  SectionHeader,
  AdminToolbar,
} from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { formatDate } from "@/lib/utils";
import * as api from "@/lib/api/nodeautoscale";
import { sanitizeError } from "@/lib/sanitize";

const PROVIDERS = [
  { value: "aws", label: "AWS" },
  { value: "gcp", label: "Google Cloud" },
  { value: "azure", label: "Azure" },
  { value: "digitalocean", label: "DigitalOcean" },
  { value: "hetzner", label: "Hetzner" },
];

const EVALUATORS = [
  { value: "cpu", label: "CPU utilisation" },
  { value: "memory", label: "Memory utilisation" },
  { value: "both", label: "CPU and memory" },
];

const emptyForm: Partial<api.NodeAutoscalePolicy> = {
  name: "",
  provider: "aws",
  region: "",
  instanceType: "",
  image: "",
  minNodes: 1,
  maxNodes: 5,
  targetCpuPercent: 70,
  targetMemPercent: 70,
  evaluator: "cpu",
  autoJoin: false,
  cooldownSeconds: 900,
  enabled: true,
};

/**
 * Structural checks the API accepts without. `minNodes: 5, maxNodes: 1` used to
 * persist happily, leaving a policy that can never satisfy its own bounds.
 */
function formDefects(form: Partial<api.NodeAutoscalePolicy>): string[] {
  const defects: string[] = [];
  if (!form.name?.trim()) defects.push("A policy name is required.");
  if (!form.provider?.trim()) defects.push("Choose a cloud provider.");
  if (!form.region?.trim()) defects.push("A region is required — instances would be requested against no location.");
  if (!form.instanceType?.trim()) defects.push("An instance type is required.");
  const min = Number(form.minNodes);
  const max = Number(form.maxNodes);
  if (!Number.isFinite(min) || !Number.isFinite(max)) defects.push("Minimum and maximum node counts must be numbers.");
  else if (min < 0 || max < 0) defects.push("Node counts cannot be negative.");
  else if (min > max) defects.push("Minimum nodes cannot exceed maximum nodes: the policy could never satisfy its own bounds.");
  const cpu = Number(form.targetCpuPercent);
  const mem = Number(form.targetMemPercent);
  if (!Number.isFinite(cpu) || cpu < 0 || cpu > 100) defects.push("Target CPU must be a percentage between 0 and 100.");
  if (!Number.isFinite(mem) || mem < 0 || mem > 100) defects.push("Target memory must be a percentage between 0 and 100.");
  if (!Number.isFinite(Number(form.cooldownSeconds)) || Number(form.cooldownSeconds) < 0) defects.push("Cooldown must be a non-negative number of seconds.");
  return defects;
}

export function NodeAutoscalerManager() {
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const [policies, setPolicies] = useState<api.NodeAutoscalePolicy[]>([]);
  const [events, setEvents] = useState<api.NodeAutoscaleEvent[]>([]);
  const [selected, setSelected] = useState<string>("");
  const [evalResult, setEvalResult] = useState<api.Evaluation | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [eventsState, setEventsState] = useState<"loading" | "ready" | "error">("loading");
  const [eventsError, setEventsError] = useState<string | null>(null);
  const [eventsRefreshedAt, setEventsRefreshedAt] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  /**
   * Evidence, not a guess. The control plane has no capability endpoint, so the
   * only honest moment at which Scale In can be declared unavailable is when the
   * service itself has just said so (`nodeautoscale/service.go:378`). Once seen,
   * the control stays disabled with that reason instead of re-offering a write
   * that is known to be rejected.
   */
  const [scaleInBlockedReason, setScaleInBlockedReason] = useState<string | null>(null);

  const [form, setForm] = useState<Partial<api.NodeAutoscalePolicy>>(emptyForm);
  const [scaleInNode, setScaleInNode] = useState("");

  const defects = useMemo(() => formDefects(form), [form]);

  async function loadPolicies() {
    setLoading(true);
    setLoadError(null);
    try {
      const list = await api.listPolicies();
      setPolicies(list);
      if (list.length > 0 && !selected) {
        const first = list[0];
        if (first) setSelected(first.id);
      }
    } catch (e) {
      setLoadError(sanitizeError(e instanceof Error ? e.message : "Failed to load policies"));
      setPolicies([]);
    } finally {
      setLoading(false);
    }
  }

  async function loadEvents(policyId?: string) {
    setEventsState("loading");
    setEventsError(null);
    try {
      const ev = await api.listEvents(policyId, 50);
      setEvents(ev);
      setEventsRefreshedAt(Date.now());
      setEventsState("ready");
    } catch (e) {
      setEvents([]);
      setEventsState("error");
      setEventsError(sanitizeError(e instanceof Error ? e.message : "Failed to load events"));
    }
  }

  useEffect(() => {
    void loadPolicies();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void loadEvents(selected || undefined);
     
  }, [selected]);

  /** True when a policy with the given id is the one currently selected. */
  const selectedPolicy = policies.find((p) => p.id === selected) ?? null;

  /**
   * Recognise the "this deployment cannot do it yet" answer. Matched on the
   * server's own wording plus the generic unwired/unconfigured phrasings so a
   * capability gap is never presented as a retryable glitch.
   */
  function capabilityReasonFrom(message: string): string | null {
    if (/not wired|not configured|unavailable|no cloud manager/i.test(message)) {
      return message;
    }
    return null;
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (defects.length > 0) {
      setError(`Policy not created: ${defects[0]}`);
      return;
    }
    const ok = await confirm({
      title: "Create this node autoscale policy?",
      description: `From now on the evaluator may provision ${form.provider ?? "cloud"} instances in ${form.region ?? "the chosen region"} between ${form.minNodes} and ${form.maxNodes} nodes when ${form.evaluator === "both" ? "CPU or memory" : (form.evaluator ?? "cpu")} load crosses the target.`,
      confirmLabel: "Create policy",
      danger: true,
    });
    if (!ok) return;
    try {
      await api.createPolicy(form);
      setSuccess("Policy created");
      setForm(emptyForm);
      await loadPolicies();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Create failed"));
    }
  }

  async function handleDelete(id: string, name: string) {
    const ok = await confirm({
      title: `Delete the “${name}” policy?`,
      description: "No further evaluations or provisioning happens for it. Nodes it already created stay up and are not retired.",
      danger: true,
      confirmLabel: "Delete policy",
    });
    if (!ok) return;
    try {
      await api.deletePolicy(id);
      setSuccess("Policy deleted");
      if (selected === id) setSelected("");
      await loadPolicies();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Delete failed"));
    }
  }

  /**
   * `PUT /autoscale/policies/:id` exists and `api.updatePolicy` already calls it,
   * so an existing policy's `enabled` flag is now changeable instead of forcing a
   * delete-and-retype. The whole record is sent because the handler replaces it.
   */
  async function handleToggleEnabled(policy: api.NodeAutoscalePolicy) {
    const ok = await confirm({
      title: `${policy.enabled ? "Disable" : "Enable"} “${policy.name}”?`,
      description: policy.enabled
        ? "The evaluator stops considering this policy; nodes it already created are untouched."
        : `The evaluator will again be allowed to provision ${policy.provider} capacity in ${policy.region} up to ${policy.maxNodes} nodes.`,
      danger: policy.enabled,
      confirmLabel: policy.enabled ? "Disable" : "Enable",
    });
    if (!ok) return;
    try {
      await api.updatePolicy(policy.id, { ...policy, enabled: !policy.enabled });
      setSuccess(`“${policy.name}” ${policy.enabled ? "disabled" : "enabled"}`);
      await loadPolicies();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Update failed"));
    }
  }

  async function handleEvaluate() {
    if (!selected) return;
    setEvalResult(null);
    try {
      const res = await api.evaluatePolicy(selected, true);
      setEvalResult(res);
    } catch (err) {
      const message = sanitizeError(err instanceof Error ? err.message : "Evaluate failed");
      setError(message);
      const reason = capabilityReasonFrom(message);
      if (reason) setScaleInBlockedReason(reason);
    }
  }

  async function handleScaleOut() {
    if (!selected || !selectedPolicy) return;
    const ok = await confirm({
      title: "Provision another node?",
      description: `This requests a real ${selectedPolicy.provider} instance in ${selectedPolicy.region} (${selectedPolicy.instanceType}) outside the dry run, up to the policy maximum of ${selectedPolicy.maxNodes} nodes. It bills and it joins the fleet.`,
      danger: true,
      confirmLabel: "Scale out",
    });
    if (!ok) return;
    try {
      const ev = await api.scaleOut(selected);
      setSuccess(`Scale-out ${ev.state}: ${ev.id.slice(0, 8)}`);
      await loadEvents(selected);
    } catch (err) {
      const message = sanitizeError(err instanceof Error ? err.message : "Scale-out failed");
      setError(message);
      const reason = capabilityReasonFrom(message);
      if (reason) setScaleInBlockedReason(reason);
    }
  }

  async function handleScaleIn() {
    const nodeId = scaleInNode.trim();
    if (!nodeId) {
      setError("A node ID is required — the autoscaler will not choose a node to retire.");
      return;
    }
    const ok = await confirm({
      title: `Drain and retire node ${nodeId}?`,
      description: "The node is withdrawn from traffic, its workloads are evacuated, and — when deprovision is requested — the backing cloud instance is destroyed. This is destructive and cannot be undone.",
      danger: true,
      confirmLabel: "Scale in",
    });
    if (!ok) return;
    try {
      const ev = await api.scaleIn(nodeId, false);
      setSuccess(`Scale-in ${ev.state} for ${nodeId}`);
      await loadEvents(selected || undefined);
    } catch (err) {
      const message = sanitizeError(err instanceof Error ? err.message : "Scale-in failed");
      setError(message);
      const reason = capabilityReasonFrom(message);
      if (reason) setScaleInBlockedReason(reason);
    }
  }

  return (
    <AdminPageLayout>
      <SectionHeader
        sub="Cluster-level node provisioning: policies evaluate fleet CPU and memory load against a target and request new cloud instances. This is a different subsystem from Workload Autoscaling, which adjusts resources for one running workload."
        status={
          loadError ? (
            <span className="font-mono text-eyebrow text-danger">Policies not loaded</span>
          ) : loading ? (
            <span className="font-mono text-eyebrow text-text-muted">Loading</span>
          ) : (
            <span className="font-mono text-eyebrow text-text-muted">{policies.length} {policies.length === 1 ? "policy" : "policies"}</span>
          )
        }
        action={
          <Btn tone="ghost" size="sm" onClick={() => { void loadPolicies(); void loadEvents(selected || undefined); }}>
            <RefreshCw size={14} /> Refresh
          </Btn>
        }
      />
      <OfflineBanner onRetry={() => { void loadPolicies(); void loadEvents(selected || undefined); }} />

      {error && (
        <div role="alert" className="ui-alert ui-alert-danger items-center justify-between gap-3">
          <span>{error}</span>
          <Btn size="sm" tone="ghost" onClick={() => setError(null)}>Dismiss</Btn>
        </div>
      )}
      {success && (
        <div role="status" className="ui-alert ui-alert-success items-center justify-between gap-3">
          <span>{success}</span>
          <Btn size="sm" tone="ghost" onClick={() => setSuccess(null)}>Dismiss</Btn>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Policies" icon={TrendingUp} />
          {loading ? (
            <AdminLoadingState label="Loading policies…" />
          ) : loadError ? (
            <AdminErrorState message={`Policies could not be loaded: ${loadError}. The empty list below is not evidence that none exist.`} retry={() => void loadPolicies()} />
          ) : policies.length === 0 ? (
            <EmptyState icon={TrendingUp} title="No node autoscale policies" message="The list loaded successfully and is empty, so no cloud capacity is being requested automatically. Create a policy to enable it." />
          ) : (
            <div className="space-y-2">
              {policies.map((p) => (
                <button
                  key={p.id}
                  onClick={() => setSelected(p.id)}
                  aria-label={`Select policy ${p.name}`}
                  aria-pressed={selected === p.id}
                  className={`w-full rounded-lg border p-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] ${
                    selected === p.id ? "border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]" : "border-line bg-overlay-subtle hover:bg-overlay-strong"
                  }`}
                >
                  <p className="flex flex-wrap items-center gap-2 text-sm font-bold text-text">
                    {p.name}
                    <Pill tone={p.enabled ? "green" : "neutral"}>{p.enabled ? "Enabled" : "Disabled"}</Pill>
                    <span className="text-xs font-normal text-text-subtle">· {p.provider}/{p.region} · {p.instanceType}</span>
                  </p>
                  <p className="mt-1 text-meta text-text-subtle">
                    {p.minNodes}–{p.maxNodes} nodes · target CPU {p.targetCpuPercent}% · target memory {p.targetMemPercent}% ·
                    {" "}evaluates {p.evaluator} · cooldown {p.cooldownSeconds}s · joins fleet automatically: {p.autoJoin ? "yes" : "no"}
                  </p>
                  {p.minNodes > p.maxNodes ? (
                    <p className="ui-alert ui-alert-warning mt-2">Minimum exceeds maximum — this policy can never satisfy its own bounds.</p>
                  ) : null}
                  <div className="mt-2 flex gap-1.5">
                    <Btn size="sm" tone={p.enabled ? "warning" : "primary"} onClick={() => void handleToggleEnabled(p)}>
                      {p.enabled ? "Disable" : "Enable"}
                    </Btn>
                    <Btn size="sm" tone="danger" ariaLabel={`Delete policy ${p.name}`} onClick={() => { void handleDelete(p.id, p.name); }}>
                      <Trash2 size={12} /> Delete
                    </Btn>
                  </div>
                </button>
              ))}
            </div>
          )}

          <AdminToolbar className="mt-4">
            <Btn tone="primary" onClick={() => void handleEvaluate()} disabled={!selected || loading}>
              Dry-run evaluation
            </Btn>
            <Btn tone="warning" onClick={() => void handleScaleOut()} disabled={!selected || loading}>
              Scale out
            </Btn>
            {!selected ? <span className="text-meta text-text-muted">Select a policy first — nothing is chosen for you.</span> : null}
          </AdminToolbar>

          {evalResult && (
            <div className="mt-4 rounded-lg border border-line bg-overlay-subtle p-3 text-xs">
              <p className="font-bold text-text">{evalResult.summary || "The evaluator returned no summary."}</p>
              <p className="mt-1 text-text-subtle">
                Missing capacity (deficit): {Number.isFinite(evalResult.deficit) ? evalResult.deficit : "not reported"} ·
                within cooldown: {typeof evalResult.cooldown === "boolean" ? (evalResult.cooldown ? "yes, scale-out is suppressed" : "no") : "not reported"} ·
                would scale out: {typeof evalResult.scaleOut === "boolean" ? (evalResult.scaleOut ? "yes" : "no") : "not reported"} ·
                dry run: {evalResult.dryRun ? "yes, nothing was provisioned" : "no"}
              </p>
              <p className="text-text-subtle">
                Active nodes {Number.isFinite(evalResult.load?.activeNodes) ? evalResult.load.activeNodes : "not reported"} ·
                CPU load {Number.isFinite(evalResult.load?.loadCpu) ? `${(evalResult.load.loadCpu * 100).toFixed(1)}%` : "not reported"} ·
                memory load {Number.isFinite(evalResult.load?.loadMemory) ? `${(evalResult.load.loadMemory * 100).toFixed(1)}%` : "not reported"}
              </p>
              {evalResult.scaleInNode ? (
                <p className="ui-alert ui-alert-warning mt-2">
                  Under-used fleet: the evaluator suggests node <span className="font-mono">{evalResult.scaleInNode}</span> as a
                  scale-in candidate. Use Scale In below to act on it — this panel does not remove anything.
                </p>
              ) : null}
            </div>
          )}

          <div className="mt-4 rounded-lg border border-line bg-overlay-subtle p-3">
            <p className="t-eyebrow">Scale in (drain and retire)</p>
            <div className="mt-2 flex flex-wrap items-end gap-2">
              <div className="min-w-[220px] flex-1">
                <Input label="Node to retire" value={scaleInNode} onChange={setScaleInNode} placeholder="node UUID" mono />
              </div>
              <Btn
                tone="danger"
                onClick={() => void handleScaleIn()}
                disabled={scaleInBlockedReason !== null || scaleInNode.trim() === ""}
              >
                Scale in
              </Btn>
            </div>
            {scaleInBlockedReason ? (
              <p className="ui-alert ui-alert-warning mt-2">
                Scale In is disabled: the control plane reports <span className="font-mono">{scaleInBlockedReason}</span>. The
                cluster membership service has to be wired into this deployment before a node can be drained and retired from
                here; nothing else on this page is affected. Refresh re-enables the control once the service is wired.
              </p>
            ) : (
              <p className="mt-2 text-meta leading-5 text-text-subtle">
                Retiring a node requires the cluster membership service to be wired on the control plane. If it is not, the
                request is rejected and no node is drained — this control becomes disabled with that reason the first time it
                happens, rather than looking available and failing repeatedly.
              </p>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title="Create policy" icon={Plus} />
          <form onSubmit={handleCreate} className="space-y-3">
            <Input label="Name" value={form.name ?? ""} onChange={(v) => setForm({ ...form, name: v })} placeholder="Nightly capacity buffer" />
            <div className="grid grid-cols-2 gap-2">
              <AdminSelect label="Provider" value={form.provider ?? ""} onChange={(v) => setForm({ ...form, provider: v })} options={PROVIDERS} />
              <Input label="Region" value={form.region ?? ""} onChange={(v) => setForm({ ...form, region: v })} placeholder="us-east-1" />
              <Input label="Instance type" value={form.instanceType ?? ""} onChange={(v) => setForm({ ...form, instanceType: v })} placeholder="t3.medium" />
              <Input label="Image" value={form.image ?? ""} onChange={(v) => setForm({ ...form, image: v })} placeholder="ubuntu-22.04" />
            </div>
            <div className="grid grid-cols-3 gap-2">
              <Input label="Min nodes" type="number" value={String(form.minNodes ?? 1)} onChange={(v) => setForm({ ...form, minNodes: Number(v) })} />
              <Input label="Max nodes" type="number" value={String(form.maxNodes ?? 5)} onChange={(v) => setForm({ ...form, maxNodes: Number(v) })} />
              <Input label="Cooldown (s)" type="number" value={String(form.cooldownSeconds ?? 900)} onChange={(v) => setForm({ ...form, cooldownSeconds: Number(v) })} />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <Input label="Target CPU %" type="number" value={String(form.targetCpuPercent ?? 70)} onChange={(v) => setForm({ ...form, targetCpuPercent: Number(v) })} />
              <Input label="Target memory %" type="number" value={String(form.targetMemPercent ?? 70)} onChange={(v) => setForm({ ...form, targetMemPercent: Number(v) })} />
            </div>
            <AdminSelect label="Evaluator" value={form.evaluator ?? "cpu"} onChange={(v) => setForm({ ...form, evaluator: v })} options={EVALUATORS} />
            <div className="flex flex-wrap gap-4 text-xs text-text">
              <label className="flex items-center gap-1.5">
                <input type="checkbox" checked={!!form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> Enabled on creation
              </label>
              <label className="flex items-center gap-1.5">
                <input type="checkbox" checked={!!form.autoJoin} onChange={(e) => setForm({ ...form, autoJoin: e.target.checked })} /> Join new nodes to the fleet automatically
              </label>
            </div>
            {defects.length > 0 ? (
              <ul className="ui-alert ui-alert-warning list-disc space-y-0.5 pl-5">
                {defects.map((d) => <li key={d}>{d}</li>)}
              </ul>
            ) : null}
            <Btn type="submit" tone="primary" disabled={defects.length > 0}>Create policy</Btn>
          </form>
        </Card>
      </div>

      <Card>
        <CardHeader
          title="Action history"
          icon={Clock}
          action={
            eventsRefreshedAt && eventsState === "ready" ? (
              <span className="font-mono text-eyebrow text-text-muted">Read {formatDate(eventsRefreshedAt)}</span>
            ) : eventsState === "error" ? (
              <Pill tone="danger">not loaded</Pill>
            ) : null
          }
        />
        <div className="space-y-2">
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-[220px]">
              <AdminSelect
                label="Filter by policy"
                value={selected}
                onChange={setSelected}
                options={policies.map((p) => ({ value: p.id, label: p.name }))}
                placeholder="All policies"
                disabled={loading || loadError !== null}
              />
            </div>
            <Btn tone="ghost" size="sm" onClick={() => void loadEvents(selected || undefined)}>
              <RefreshCw size={13} /> Refresh events
            </Btn>
          </div>
          {eventsState === "loading" ? (
            <AdminLoadingState label="Loading events…" />
          ) : eventsState === "error" ? (
            <AdminErrorState message={`The event history could not be read: ${eventsError ?? "request failed"}. “No events” would be a guess.`} retry={() => void loadEvents(selected || undefined)} />
          ) : events.length === 0 ? (
            <EmptyState
              icon={Clock}
              title="No autoscale actions recorded"
              message="The history loaded and is empty, so this policy has evaluated but never provisioned or retired anything yet."
            />
          ) : (
            <div className="max-h-[420px] space-y-2 overflow-auto">
              {events.map((ev) => (
                <div key={ev.id} className="rounded-lg border border-line bg-overlay-subtle p-3 text-xs">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="flex flex-wrap items-center gap-2 font-bold text-text">
                      {ev.action} → {ev.state}
                      <Pill tone={ev.state === "failed" ? "red" : ev.state === "applied" ? "green" : "yellow"}>{ev.state}</Pill>
                      <span className="font-normal text-text-subtle">
                        deficit {Number.isFinite(ev.deficit) ? ev.deficit : "not reported"}
                        {ev.policyId ? ` · policy ${ev.policyId.slice(0, 8)}` : " · no policy recorded"}
                      </span>
                    </span>
                    <span className="font-mono text-eyebrow text-text-muted">{formatDate(ev.createdAt, "no recorded time")}</span>
                  </div>
                  <p className="mt-1 break-all font-mono text-eyebrow text-text-subtle">
                    {ev.detail && Object.keys(ev.detail).length > 0 ? JSON.stringify(ev.detail) : "No detail reported for this event."}
                  </p>
                </div>
              ))}
            </div>
          )}
        </div>
      </Card>
      {renderConfirm()}
    </AdminPageLayout>
  );
}
