"use client";

import { useEffect, useState } from "react";
import { OfflineBanner } from "@/components/shared/states-offline";
import { AdminCard, AdminPageLayout } from "@/components/admin/admin-layout";
import * as api from "@/lib/api/nodeautoscale";
import { sanitizeError } from "@/lib/sanitize";

export function NodeAutoscalerManager() {
  const [policies, setPolicies] = useState<api.NodeAutoscalePolicy[]>([]);
  const [events, setEvents] = useState<api.NodeAutoscaleEvent[]>([]);
  const [selected, setSelected] = useState<string>("");
  const [evalResult, setEvalResult] = useState<api.Evaluation | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [form, setForm] = useState<Partial<api.NodeAutoscalePolicy>>({
    name: "",
    provider: "aws",
    region: "us-east-1",
    instanceType: "t3.medium",
    image: "ubuntu-22.04",
    minNodes: 1,
    maxNodes: 5,
    targetCpuPercent: 70,
    targetMemPercent: 70,
    evaluator: "cpu",
    autoJoin: false,
    cooldownSeconds: 900,
    enabled: true,
  });

  const [scaleInNode, setScaleInNode] = useState("");

  async function loadPolicies() {
    setLoading(true);
    setError(null);
    try {
      const list = await api.listPolicies();
      setPolicies(list);
      if (list.length > 0 && !selected) {
        const first = list[0];
        if (first) setSelected(first.id);
      }
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Failed to load policies"));
    } finally {
      setLoading(false);
    }
  }

  async function loadEvents(policyId?: string) {
    try {
      const ev = await api.listEvents(policyId, 50);
      setEvents(ev);
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Failed to load events"));
    }
  }

  useEffect(() => {
    void loadPolicies();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    void loadEvents(selected || undefined);
  }, [selected]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await api.createPolicy(form);
      setSuccess("Policy created");
      await loadPolicies();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Create failed"));
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete policy?")) return;
    try {
      await api.deletePolicy(id);
      setSuccess("Policy deleted");
      await loadPolicies();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Delete failed"));
    }
  }

  async function handleEvaluate() {
    if (!selected) return;
    try {
      const res = await api.evaluatePolicy(selected, true);
      setEvalResult(res);
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Evaluate failed"));
    }
  }

  async function handleScaleOut() {
    if (!selected) return;
    if (!confirm("Confirm scale-out provisioning? This will call cloud.ProvisionNode.")) return;
    try {
      const ev = await api.scaleOut(selected);
      setSuccess(`Scale-out ${ev.state}: ${ev.id.slice(0, 8)}`);
      await loadEvents(selected);
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Scale-out failed"));
    }
  }

  async function handleScaleIn() {
    if (!scaleInNode.trim()) {
      setError("nodeId required");
      return;
    }
    if (!confirm(`Drain and retire node ${scaleInNode}?`)) return;
    try {
      const ev = await api.scaleIn(scaleInNode.trim(), false);
      setSuccess(`Scale-in ${ev.state}`);
      await loadEvents(selected);
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Scale-in failed"));
    }
  }

  return (
    <AdminPageLayout
      title="Node Autoscaler"
      description="Cluster-level node provisioning — distinct from the service autoscaler (which scales replicas). Policies evaluate fleet CPU/memory and provision cloud instances via the cloud manager (suggest + explicit confirm)."
    >
      <OfflineBanner onRetry={() => void loadPolicies()} />
      {error && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-500/[0.09] p-4 text-sm text-red-200">
          <span>{error}</span> <button onClick={() => setError(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}
      {success && (
        <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.09] p-4 text-sm text-emerald-200">
          <span>{success}</span> <button onClick={() => setSuccess(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title="Policies" description="node_autoscale_policies · suggest + apply with explicit confirm. AutoJoin=false by default.">
          {loading ? (
            <p className="text-sm text-[var(--text-subtle)]">Loading…</p>
          ) : policies.length === 0 ? (
            <p className="text-sm text-[var(--text-subtle)]">No policies. Create one to enable node autoscaling.</p>
          ) : (
            <div className="space-y-2">
              {policies.map((p) => (
                <button
                  key={p.id}
                  onClick={() => setSelected(p.id)}
                  className={`w-full text-left rounded-lg border p-3 ${selected === p.id ? "border-red-400 bg-[var(--brand)]-wash" : "border-[var(--line)] bg-surface hover:bg-[var(--surface-input)]"}`}
                >
                  <p className="text-sm font-bold text-[var(--text)]">{p.name} <span className="text-xs font-normal text-[var(--text-subtle)]">· {p.provider}/{p.region} · {p.instanceType}</span></p>
                  <p className="text-xs text-[var(--text-subtle)]">min {p.minNodes} · max {p.maxNodes} · target CPU {p.targetCpuPercent}% · eval {p.evaluator} · {p.enabled ? "enabled" : "disabled"} · cd {p.cooldownSeconds}s</p>
                </button>
              ))}
            </div>
          )}

          <div className="mt-4 flex flex-wrap gap-2">
            <button onClick={() => void handleEvaluate()} disabled={!selected} className="rounded bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white disabled:opacity-50">Evaluate (dryRun)</button>
            <button onClick={() => void handleScaleOut()} disabled={!selected} className="rounded bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white disabled:opacity-50">Scale Out (confirm)</button>
            <button onClick={() => void loadPolicies()} className="rounded border border-[var(--line)] px-4 py-2 text-xs">Refresh</button>
          </div>

          {evalResult && (
            <div className="mt-4 rounded-lg border border-[var(--line)] bg-surface p-3 text-xs">
              <p className="font-bold">{evalResult.summary}</p>
              <p className="text-[var(--text-subtle)]">Deficit {evalResult.deficit} · cooldown {String(evalResult.cooldown)} · scaleOut {String(evalResult.scaleOut)} · active {evalResult.load.activeNodes} · CPU {(evalResult.load.loadCpu * 100).toFixed(1)}% · Mem {(evalResult.load.loadMemory * 100).toFixed(1)}%</p>
              {evalResult.scaleInNode && <p className="text-amber-600">Scale-in candidate: {evalResult.scaleInNode}</p>}
            </div>
          )}

          <div className="mt-4 rounded-lg border border-[var(--line)] bg-surface p-3">
            <p className="text-xs font-bold uppercase text-[var(--text-subtle)]">Scale-In (drain + retire)</p>
            <div className="mt-2 flex gap-2">
              <input value={scaleInNode} onChange={(e) => setScaleInNode(e.target.value)} placeholder="nodeId" className="flex-1 rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
              <button onClick={() => void handleScaleIn()} className="rounded border border-amber-300 bg-amber-50 px-4 py-2 text-xs font-bold text-amber-700">Scale In</button>
            </div>
            <p className="mt-2 text-xs text-[var(--text-subtle)]">Requires membership service wiring; otherwise returns “membership service not wired for scale-in”.</p>
          </div>
        </AdminCard>

        <AdminCard title="Create Policy" description="Provider, region, instance type, evaluator (cpu|memory|both), cooldown. Mirrors the server autoscaler but for nodes.">
          <form onSubmit={handleCreate} className="space-y-3">
            <input value={form.name ?? ""} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Name" className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" required />
            <div className="grid grid-cols-2 gap-2">
              <input value={form.provider ?? ""} onChange={(e) => setForm({ ...form, provider: e.target.value })} placeholder="provider (aws)" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
              <input value={form.region ?? ""} onChange={(e) => setForm({ ...form, region: e.target.value })} placeholder="region (us-east-1)" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
              <input value={form.instanceType ?? ""} onChange={(e) => setForm({ ...form, instanceType: e.target.value })} placeholder="instanceType (t3.medium)" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
              <input value={form.image ?? ""} onChange={(e) => setForm({ ...form, image: e.target.value })} placeholder="image (ubuntu-22.04)" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
            </div>
            <div className="grid grid-cols-3 gap-2">
              <input type="number" value={form.minNodes ?? 1} onChange={(e) => setForm({ ...form, minNodes: Number(e.target.value) })} placeholder="minNodes" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
              <input type="number" value={form.maxNodes ?? 5} onChange={(e) => setForm({ ...form, maxNodes: Number(e.target.value) })} placeholder="maxNodes" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
              <input type="number" value={form.cooldownSeconds ?? 900} onChange={(e) => setForm({ ...form, cooldownSeconds: Number(e.target.value) })} placeholder="cooldownSeconds" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <input type="number" step="0.1" value={form.targetCpuPercent ?? 70} onChange={(e) => setForm({ ...form, targetCpuPercent: Number(e.target.value) })} placeholder="targetCpuPercent" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
              <input type="number" step="0.1" value={form.targetMemPercent ?? 70} onChange={(e) => setForm({ ...form, targetMemPercent: Number(e.target.value) })} placeholder="targetMemPercent" className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
            </div>
            <div className="flex gap-4 text-xs">
              <label className="flex gap-1 items-center"><input type="checkbox" checked={!!form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> Enabled</label>
              <label className="flex gap-1 items-center"><input type="checkbox" checked={!!form.autoJoin} onChange={(e) => setForm({ ...form, autoJoin: e.target.checked })} /> AutoJoin</label>
              <select value={form.evaluator ?? "cpu"} onChange={(e) => setForm({ ...form, evaluator: e.target.value })} className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-2 py-1">
                <option value="cpu">cpu</option>
                <option value="memory">memory</option>
                <option value="both">both</option>
              </select>
            </div>
            <button type="submit" className="rounded bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white">Create Policy</button>
          </form>

          {policies.length > 0 && (
            <div className="mt-6">
              <p className="text-xs font-bold uppercase text-[var(--text-subtle)]">Delete Policy</p>
              <div className="mt-2 space-y-1">
                {policies.map((p) => (
                  <div key={p.id} className="flex items-center justify-between rounded border border-[var(--line)] bg-surface px-3 py-2 text-xs">
                    <span>{p.name}</span>
                    <button onClick={() => void handleDelete(p.id)} className="rounded border border-red-300 px-2 py-1 text-[var(--brand)]">Delete</button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </AdminCard>
      </div>

      <AdminCard title="Audit Ledger" description="node_autoscale_events · ?policyId=&limit= . Action: evaluate|scale-out|scale-in|error · State: suggested|applied|failed">
        <div className="space-y-2">
          <div className="flex gap-2">
            <select value={selected} onChange={(e) => setSelected(e.target.value)} className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm">
              <option value="">All policies</option>
              {policies.map((p) => (
                <option key={p.id} value={p.id}>{p.name}</option>
              ))}
            </select>
            <button onClick={() => void loadEvents(selected || undefined)} className="rounded border border-[var(--line)] px-4 py-2 text-xs">Refresh Events</button>
          </div>
          {events.length === 0 ? (
            <p className="text-sm text-[var(--text-subtle)]">No events.</p>
          ) : (
            <div className="max-h-[420px] overflow-auto space-y-2">
              {events.map((ev) => (
                <div key={ev.id} className="rounded-lg border border-[var(--line)] bg-surface p-3 text-xs">
                  <div className="flex justify-between">
                    <span className="font-bold">{ev.action} → {ev.state} <span className="font-normal text-[var(--text-subtle)]">deficit {ev.deficit}</span></span>
                    <span className="text-[var(--text-subtle)]">{new Date(ev.createdAt).toLocaleString()}</span>
                  </div>
                  <p className="mt-1 font-mono text-[11px] text-[var(--text-subtle)] break-all">{JSON.stringify(ev.detail)}</p>
                </div>
              ))}
            </div>
          )}
        </div>
      </AdminCard>
    </AdminPageLayout>
  );
}
