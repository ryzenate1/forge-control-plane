"use client";

import { useEffect, useState } from "react";
import { AdminCard, AdminPageLayout } from "@/components/admin/admin-layout";
import * as billing from "@/lib/api/billing";
import { sanitizeError } from "@/lib/sanitize";

export function BillingManager() {
  const [plans, setPlans] = useState<billing.BillingPlan[]>([]);
  const [quotaOrgId, setQuotaOrgId] = useState("");
  const [quota, setQuota] = useState<billing.OrgQuota | null>(null);
  const [usage, setUsage] = useState<billing.UsageSummary | null>(null);
  const [events, setEvents] = useState<billing.UsageEvent[]>([]);
  const [settings, setSettings] = useState<billing.BillingSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [form, setForm] = useState({ code: "", name: "", centsPerMonth: 0, trialDays: 0, active: true, entitlements: '{"max_servers":10,"max_memory_gb":32}' });
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [planOrgId, setPlanOrgId] = useState("");
  const [planCode, setPlanCode] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [externalProcessor, setExternalProcessor] = useState("");

  async function load() {
    setLoading(true);
    setError(null);
    try {
      const [plansRes, settingsRes] = await Promise.all([
        billing.fetchBillingPlans().catch(() => [] as billing.BillingPlan[]),
        billing.fetchBillingSettings().catch(() => null),
      ]);
      setPlans(plansRes);
      setSettings(settingsRes);
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Failed to load"));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(null);
    try {
      const entitlements = JSON.parse(form.entitlements || "{}") as Record<string, unknown>;
      await billing.createBillingPlan({
        code: form.code,
        name: form.name,
        centsPerMonth: Number(form.centsPerMonth),
        trialDays: Number(form.trialDays),
        active: form.active,
        entitlements,
      });
      setSuccess("Plan created");
      setForm({ code: "", name: "", centsPerMonth: 0, trialDays: 0, active: true, entitlements: '{"max_servers":10,"max_memory_gb":32}' });
      await load();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Create failed"));
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this plan?")) return;
    try {
      const result = await billing.deleteBillingPlan(id);
      if (!result.ok) throw new Error("The server reported the billing plan was not deleted.");
      setSuccess("Plan deleted");
      await load();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Delete failed"));
    }
  }

  async function handleUpdate() {
    if (!editingId) return;
    try {
      await billing.updateBillingPlan(editingId, { name: editName });
      setEditingId(null);
      setEditName("");
      setSuccess("Plan updated");
      await load();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Update failed"));
    }
  }

  async function handleQuotaFetch() {
    if (!quotaOrgId.trim()) {
      setError("Org ID required");
      return;
    }
    setError(null);
    try {
      const [q, u, ev] = await Promise.all([
        billing.fetchOrgQuota(quotaOrgId.trim()),
        billing.fetchOrgUsage(quotaOrgId.trim()),
        billing.fetchOrgUsageEvents(quotaOrgId.trim(), undefined, 20),
      ]);
      setQuota(q);
      setUsage(u);
      setEvents(ev);
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Quota fetch failed"));
    }
  }

  async function handleSetPlan() {
    if (!planOrgId.trim() || !planCode.trim()) {
      setError("Org ID and plan code required");
      return;
    }
    try {
      await billing.setOrgPlan(planOrgId.trim(), planCode.trim(), false);
      setSuccess(`Plan ${planCode} assigned to ${planOrgId}`);
      if (quotaOrgId.trim() === planOrgId.trim()) await handleQuotaFetch();
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Set plan failed"));
    }
  }

  async function handleSaveSettings() {
    try {
      const res = await billing.updateBillingSettings({
        webhookSecret: webhookSecret || undefined,
        externalProcessor: externalProcessor || undefined,
      });
      setSettings(res);
      setSuccess("Billing settings saved");
      setWebhookSecret("");
    } catch (err) {
      setError(sanitizeError(err instanceof Error ? err.message : "Settings save failed"));
    }
  }

  return (
    <AdminPageLayout
      title="Billing & Plans"
      description="Manage purchasable tiers, per-org quotas, usage metering, and processor webhook settings. Public catalog at GET /billing/plans; admin CRUD under /billing/plans."
    >
      {error && (
        <div role="alert" className="rounded-xl border border-red-300 bg-red-wash p-4 text-sm text-red-dark">
          {error} <button onClick={() => setError(null)} className="ml-2 underline">Dismiss</button>
        </div>
      )}
      {success && (
        <div role="status" className="rounded-xl border border-green-300 bg-green-50 p-4 text-sm text-green-700">
          {success} <button onClick={() => setSuccess(null)} className="ml-2 underline">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title="Billing Plans" description="CRUD for billing_plans. Entitlements JSON: max_servers, max_memory_gb, max_environments, max_nodes, price_per_gb, features[]">
          {loading ? (
            <p className="text-sm text-muted">Loading…</p>
          ) : (
            <div className="space-y-3">
              {plans.length === 0 ? (
                <p className="text-sm text-muted">No plans. Create the free plan first.</p>
              ) : (
                <div className="space-y-2">
                  {plans.map((p) => (
                    <div key={p.id} className="rounded-lg border border-line bg-surface p-3">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="text-sm font-bold text-ink">{p.name} <span className="font-mono text-xs text-muted">({p.code})</span></p>
                          <p className="text-xs text-muted">${(p.centsPerMonth / 100).toFixed(2)}/mo · trial {p.trialDays}d · {p.active ? "active" : "inactive"}</p>
                          <p className="mt-1 text-xs font-mono text-muted truncate max-w-[320px]">{JSON.stringify(p.entitlements)}</p>
                        </div>
                        <div className="flex gap-2">
                          <button
                            onClick={() => {
                              setEditingId(p.id);
                              setEditName(p.name);
                            }}
                            className="rounded border border-line px-2 py-1 text-xs hover:bg-paper"
                          >
                            Edit
                          </button>
                          <button onClick={() => void handleDelete(p.id)} className="rounded border border-red-300 px-2 py-1 text-xs text-red-dark hover:bg-red-wash">
                            Delete
                          </button>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {editingId && (
                <div className="rounded-lg border border-amber-300 bg-amber-50 p-3 flex gap-2">
                  <input value={editName} onChange={(e) => setEditName(e.target.value)} className="flex-1 rounded border border-line bg-paper px-3 py-1.5 text-sm" placeholder="New name" />
                  <button onClick={() => void handleUpdate()} className="rounded bg-ink px-3 py-1.5 text-xs font-bold text-white">Save</button>
                  <button onClick={() => setEditingId(null)} className="rounded border px-3 py-1.5 text-xs">Cancel</button>
                </div>
              )}
            </div>
          )}
          <form onSubmit={handleCreate} className="mt-6 space-y-3 rounded-lg border border-line bg-surface p-4">
            <p className="text-xs font-bold uppercase text-muted">Create Plan</p>
            <div className="grid grid-cols-2 gap-2">
              <input value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} placeholder="code (free, pro)" className="rounded border border-line bg-paper px-3 py-2 text-sm" required />
              <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Name" className="rounded border border-line bg-paper px-3 py-2 text-sm" required />
              <input type="number" value={form.centsPerMonth} onChange={(e) => setForm({ ...form, centsPerMonth: Number(e.target.value) })} placeholder="centsPerMonth" className="rounded border border-line bg-paper px-3 py-2 text-sm" />
              <input type="number" value={form.trialDays} onChange={(e) => setForm({ ...form, trialDays: Number(e.target.value) })} placeholder="trialDays" className="rounded border border-line bg-paper px-3 py-2 text-sm" />
            </div>
            <textarea value={form.entitlements} onChange={(e) => setForm({ ...form, entitlements: e.target.value })} rows={2} className="w-full rounded border border-line bg-paper px-3 py-2 font-mono text-xs" placeholder='{"max_servers":10,"max_memory_gb":32}' />
            <label className="flex items-center gap-2 text-xs">
              <input type="checkbox" checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} /> Active
            </label>
            <button type="submit" className="rounded bg-red px-4 py-2 text-sm font-bold text-white hover:bg-red-dark">Create Plan</button>
          </form>
        </AdminCard>

        <div className="space-y-6">
          <AdminCard title="Org Quota & Usage" description="Inspect live quota counters (recomputed from authoritative tables) and 24h usage events.">
            <div className="space-y-3">
              <div className="flex gap-2">
                <input value={quotaOrgId} onChange={(e) => setQuotaOrgId(e.target.value)} placeholder="orgId" className="flex-1 rounded border border-line bg-paper px-3 py-2 text-sm" />
                <button onClick={() => void handleQuotaFetch()} className="rounded bg-ink px-4 py-2 text-xs font-bold text-white">Load</button>
              </div>
              {quota && (
                <div className="rounded-lg border border-line bg-surface p-3 text-xs">
                  <p className="font-bold">Plan: {quota.planCode} {quota.trialUntil ? `(trial until ${new Date(quota.trialUntil).toLocaleString()})` : ""}</p>
                  <div className="mt-2 grid grid-cols-2 gap-2 text-muted">
                    <span>Servers: {quota.serversCount}</span>
                    <span>Memory: {(quota.memoryUsageBytes / (1024 * 1024 * 1024)).toFixed(2)} GB</span>
                    <span>Storage: {(quota.storageBytes / (1024 * 1024 * 1024)).toFixed(2)} GB</span>
                    <span>Envs: {quota.environmentsCount}</span>
                    <span>Nodes: {quota.nodesCount}</span>
                    <span>Updated: {new Date(quota.quotaUpdatedAt).toLocaleString()}</span>
                  </div>
                </div>
              )}
              {usage && (
                <div className="rounded-lg border border-line bg-surface p-3 text-xs">
                  <p className="font-bold">Usage Summary (plan {usage.plan})</p>
                  <p className="text-muted">Servers {usage.servers} · Memory {usage.memoryBytes} · Storage {usage.storageBytes} · Envs {usage.environments} · Nodes {usage.nodes} · Events {usage.meterEvents}</p>
                </div>
              )}
              {events.length > 0 && (
                <div className="rounded-lg border border-line bg-paper p-3">
                  <p className="text-xs font-bold">Recent Usage Events (last {events.length})</p>
                  <ul className="mt-2 space-y-1 text-xs text-muted">
                    {events.map((ev) => (
                      <li key={ev.id} className="flex justify-between">
                        <span>{ev.resource} ×{ev.quantity} ({ev.kind})</span>
                        <span>{new Date(ev.occurredAt).toLocaleString()}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              <div className="rounded-lg border border-line bg-surface p-3 space-y-2">
                <p className="text-xs font-bold uppercase text-muted">Assign Plan to Org</p>
                <input value={planOrgId} onChange={(e) => setPlanOrgId(e.target.value)} placeholder="orgId" className="w-full rounded border border-line bg-paper px-3 py-2 text-sm" />
                <div className="flex gap-2">
                  <input value={planCode} onChange={(e) => setPlanCode(e.target.value)} placeholder="planCode (free, pro)" className="flex-1 rounded border border-line bg-paper px-3 py-2 text-sm" />
                  <button onClick={() => void handleSetPlan()} className="rounded bg-ink px-4 py-2 text-xs font-bold text-white">Assign</button>
                </div>
              </div>
              <div className="rounded-lg border border-line bg-surface p-3 space-y-2">
                <p className="text-xs font-bold uppercase text-muted">Record Meter Usage</p>
                <UsageRecorder orgId={quotaOrgId} />
              </div>
            </div>
          </AdminCard>

          <AdminCard title="Billing Settings" description="Webhook HMAC secret (write-only, 16+ chars) and external processor name. Without a secret the webhook receiver fails (audit F-HIGH-3).">
            {settings ? (
              <div className="space-y-2 text-xs">
                <p>Has secret: {settings.hasWebhookSecret ? "yes" : "no"} · Processor: {settings.externalProcessor || "none"}</p>
                <p className="text-muted">Updated: {settings.updatedAt ? new Date(settings.updatedAt).toLocaleString() : "—"}</p>
              </div>
            ) : (
              <p className="text-xs text-muted">No settings yet.</p>
            )}
            <div className="mt-3 space-y-2">
              <input value={webhookSecret} onChange={(e) => setWebhookSecret(e.target.value)} placeholder="new webhookSecret (≥16 chars)" className="w-full rounded border border-line bg-paper px-3 py-2 text-sm" />
              <input value={externalProcessor} onChange={(e) => setExternalProcessor(e.target.value)} placeholder="externalProcessor (e.g. stripe)" className="w-full rounded border border-line bg-paper px-3 py-2 text-sm" />
              <button onClick={() => void handleSaveSettings()} className="rounded bg-red px-4 py-2 text-xs font-bold text-white">Save Settings</button>
              <button
                onClick={() => void billing.updateBillingSettings({ clearWebhookSecret: true }).then(setSettings).catch((e: unknown) => setError(sanitizeError(e instanceof Error ? e.message : "Clear failed")))}
                className="ml-2 rounded border border-line px-4 py-2 text-xs"
              >
                Clear Secret
              </button>
            </div>
          </AdminCard>
        </div>
      </div>
    </AdminPageLayout>
  );
}

function UsageRecorder({ orgId }: { orgId: string }) {
  const [resource, setResource] = useState("servers");
  const [quantity, setQuantity] = useState(1);
  const [msg, setMsg] = useState<string | null>(null);
  return (
    <div className="flex gap-2 items-end">
      <div className="flex-1">
        <label className="text-[11px] uppercase text-muted">Resource</label>
        <select value={resource} onChange={(e) => setResource(e.target.value)} className="w-full rounded border border-line bg-paper px-2 py-1.5 text-sm">
          <option value="servers">servers</option>
          <option value="memory">memory</option>
          <option value="storage">storage</option>
          <option value="environments">environments</option>
          <option value="nodes">nodes</option>
        </select>
      </div>
      <div className="w-24">
        <label className="text-[11px] uppercase text-muted">Qty</label>
        <input type="number" value={quantity} onChange={(e) => setQuantity(Number(e.target.value))} className="w-full rounded border border-line bg-paper px-2 py-1.5 text-sm" />
      </div>
      <button
        onClick={() => {
          if (!orgId.trim()) {
            setMsg("orgId required");
            return;
          }
          void billing
            .recordBillingUsage(orgId.trim(), resource, quantity)
            .then(() => setMsg(`Recorded ${quantity} ${resource}`))
            .catch((e: unknown) => setMsg(sanitizeError(e instanceof Error ? e.message : "Failed")));
        }}
        className="rounded bg-ink px-3 py-1.5 text-xs font-bold text-white"
      >
        Record
      </button>
      {msg && <span className="text-xs text-muted">{msg}</span>}
    </div>
  );
}
