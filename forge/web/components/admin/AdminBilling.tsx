"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CreditCard, Plus, RefreshCw, Save, Trash2, Shield, Building2, Gauge } from "lucide-react";
import {
  fetchBillingPlans,
  createBillingPlan,
  updateBillingPlan,
  deleteBillingPlan,
  fetchBillingSettings,
  updateBillingSettings,
  fetchOrgQuota,
  fetchOrgUsage,
  fetchOrgUsageEvents,
  setOrgPlan,
  type BillingPlan,
  type OrgQuota,
  type UsageSummary,
  type UsageEvent,
} from "@/lib/api/billing";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageLayout,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  SectionHeader,
  Card,
  CardHeader,
  Btn,
  Pill,
  AdminLoadingState,
  AdminErrorState,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  SubsystemHealthMeter,
  Textarea,
  AdminFormField,
} from "./admin-ui";
import { DataState, Reading, FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { errorMessage, formatDate } from "@/lib/utils";

const CURRENCY = "USD";

const priceFormatter = new Intl.NumberFormat(undefined, {
  style: "currency",
  currency: CURRENCY,
  minimumFractionDigits: 2,
});

export function AdminBilling() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const [showCreate, setShowCreate] = useState(false);
  const [editing, setEditing] = useState<BillingPlan | null>(null);

  const plansQ = useQuery({ queryKey: ["admin-billing-plans"], queryFn: fetchBillingPlans, retry: false });
  const plans = useMemo(() => plansQ.data ?? [], [plansQ.data]);

  const deleteMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await deleteBillingPlan(id);
      if (!result.ok) throw new Error("The server reported the billing plan was not deleted.");
      return result;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-billing-plans"] });
      toast({ tone: "success", title: "Plan deleted", message: "Organizations already assigned this plan keep their recorded quota until reassigned." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => void plansQ.refetch()} />
      <SectionHeader
        status={<FreshnessBadge state={sourceState(plansQ)} />}
        action={
          <div className="flex gap-2">
            <Btn size="sm" tone="ghost" onClick={() => void plansQ.refetch()}>
              <RefreshCw size={14} /> Refresh
            </Btn>
            <Btn size="sm" onClick={() => setShowCreate(true)}>
              <Plus size={14} /> New plan
            </Btn>
          </div>
        }
        info={{
          title: "Billing & Quotas",
          triggerLabel: "About billing and quotas",
          description: "Plans, per-organization quotas and usage, and the payment-processor webhook receiver.",
          sections: [
            {
              title: "Plans are quota definitions",
              content: "A plan carries an entitlements document (max_servers, max_memory_gb, max_environments, max_nodes, price_per_gb, features). An empty document means no limits have been configured for that plan; it is not the same as an unlimited plan, and a recorded 0 is shown as 0 rather than as a policy.",
            },
            {
              title: "Quota and usage are per organization",
              content: "Counters are recomputed by the control plane from authoritative tables when you load an organization. Nothing here is a live reading of workload consumption, and no organization is selected for you.",
            },
            {
              title: "The webhook secret is write-only",
              content: "The API stores the HMAC secret and only ever reports whether one is set. There is no way to read it back, so this page offers no reveal control — a value the system does not have cannot be shown.",
            },
          ],
        }}
      />

      {/* Plans */}
      <Card>
        <CardHeader title={plansQ.data === undefined ? "Plans" : `Plans · ${plans.length}`} icon={CreditCard} />
        <DataState
          emptyMessage="No plans exist yet. Create one to define a tier and its entitlements."
          emptyTitle="No billing plans"
          isEmpty={plans.length === 0}
          loadingLabel="Loading billing plans…"
          onRetry={() => void plansQ.refetch()}
          state={sourceState(plansQ)}
        >
          <div className="grid gap-4 p-4 sm:grid-cols-2 lg:grid-cols-3">
            {plans.map((plan) => (
              <div className="flex flex-col rounded-xl border border-line bg-overlay-subtle p-4" key={plan.id}>
                <div className="flex items-start justify-between gap-2">
                  <div>
                    <div className="font-mono text-sm font-semibold text-text">{plan.name}</div>
                    <div className="font-mono text-[11px] text-brand">{plan.code}</div>
                  </div>
                  <Pill tone={plan.active ? "ok" : "neutral"}>{plan.active ? "active" : "inactive"}</Pill>
                </div>
                <div className="mt-3">
                  <div className="font-mono text-lg font-bold text-text">
                    {Number.isFinite(plan.centsPerMonth)
                      ? priceFormatter.format(plan.centsPerMonth / 100)
                      : <Reading reason="Price not recorded" value={undefined} />}
                    <span className="text-xs font-normal text-text-subtle"> / month</span>
                  </div>
                  {plan.trialDays ? <div className="text-xs text-text-subtle">{plan.trialDays} trial days</div> : null}
                </div>
                <EntitlementsView raw={plan.entitlements} />
                <div className="mt-3 flex gap-2">
                  <Btn onClick={() => setEditing(plan)} size="sm" tone="ghost">
                    <Save size={12} /> Edit
                  </Btn>
                  <Btn
                    size="sm"
                    tone="danger"
                    loading={deleteMut.isPending && deleteMut.variables === plan.id}
                    onClick={() => {
                      void (async () => {
                        const ok = await confirm({
                          title: `Delete ${plan.code}?`,
                          description: `Plan ${plan.name} is permanently removed. Organizations assigned to it keep their recorded quota until they are reassigned. This cannot be undone.`,
                          danger: true,
                          confirmLabel: "Delete plan",
                        });
                        if (ok) deleteMut.mutate(plan.id);
                      })();
                    }}
                  >
                    <Trash2 size={12} /> Delete
                  </Btn>
                </div>
                <div className="mt-2 font-mono text-[11px] text-text-subtle">Updated {formatDate(plan.updatedAt, "not reported")}</div>
              </div>
            ))}
          </div>
        </DataState>
        <div className="border-t border-line p-3 text-xs leading-5 text-text-subtle">
          Plans define the limits an organization may consume. Assign one from the quota panel below and read the counters the control plane computes against it.
        </div>
      </Card>

      <QuotaCard plans={plans} />

      <BillingSettingsCard />

      {showCreate && <PlanModal onClose={() => setShowCreate(false)} onDone={() => { setShowCreate(false); void qc.invalidateQueries({ queryKey: ["admin-billing-plans"] }); }} />}
      {editing && <PlanModal plan={editing} onClose={() => setEditing(null)} onDone={() => { setEditing(null); void qc.invalidateQueries({ queryKey: ["admin-billing-plans"] }); }} />}

      {renderConfirm()}
    </AdminPageLayout>
  );
}

// Renders a plan's entitlements document as compact key/value chips.
// An empty document is reported as "no limits configured", never as
// "unlimited": `{}` is what an unconfigured plan looks like, and claiming a
// policy from a missing value is exactly what AGENTS.md forbids.
function EntitlementsView({ raw }: { raw: unknown }) {
  const obj = raw && typeof raw === "object" && !Array.isArray(raw) ? (raw as Record<string, unknown>) : null;
  const label = (k: string) => k.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
  const fmt = (v: unknown) => {
    if (v === null) return "not set";
    if (Array.isArray(v)) return `${v.length} entr${v.length === 1 ? "y" : "ies"}`;
    if (typeof v === "number") return v.toLocaleString();
    if (typeof v === "boolean") return v ? "yes" : "no";
    return String(v);
  };
  return (
    <div className="mt-3">
      <div className="text-meta uppercase tracking-wider text-text-subtle">Entitlements</div>
      {obj ? (
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {Object.keys(obj).length === 0 ? (
            <span className="text-[11px] text-text-subtle">No limits configured on this plan — not the same as unlimited.</span>
          ) : null}
          {Object.entries(obj).map(([k, v]) => (
            <span className="inline-flex items-center gap-1 rounded-md border border-line bg-overlay-subtle px-2 py-0.5 text-[11px]" key={k}>
              <span className="text-text-subtle">{label(k)}</span>
              <span className="font-semibold text-text">{fmt(v)}</span>
            </span>
          ))}
        </div>
      ) : (
        <p className="mt-1 text-[11px] leading-5 text-text-subtle">
          This plan&rsquo;s entitlements are not a key/value document, so they are shown as stored below.
          <pre className="mt-1 max-h-24 overflow-auto rounded-lg border border-line bg-overlay-subtle p-2 font-mono text-[11px] leading-5 text-text-subtle">{JSON.stringify(raw, null, 2)}</pre>
        </p>
      )}
    </div>
  );
}

type QuotaRead = { quota: OrgQuota; usage: UsageSummary; events: UsageEvent[] };

/**
 * Quota and usage for one organization, addressed explicitly.
 *
 * There is no org list on this page and nothing is auto-picked: the endpoints are
 * `/billing/org/:orgId/…`, so an identifier is required and a guessed one would
 * show another organization's numbers.
 */
function QuotaCard({ plans }: { plans: BillingPlan[] }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [orgId, setOrgId] = useState("");
  const [submitted, setSubmitted] = useState<string | null>(null);
  const [assignOrgId, setAssignOrgId] = useState("");
  const [assignPlan, setAssignPlan] = useState("");

  const quotaQ = useQuery({
    queryKey: ["admin-billing-quota", submitted],
    enabled: Boolean(submitted),
    queryFn: async () => {
      const [quota, usage, events] = await Promise.all([
        fetchOrgQuota(submitted!),
        fetchOrgUsage(submitted!),
        fetchOrgUsageEvents(submitted!, undefined, 20),
      ]);
      return { quota, usage, events };
    },
    retry: false,
  });

  const assignMut = useMutation({
    mutationFn: () => setOrgPlan(assignOrgId.trim(), assignPlan.trim(), false),
    onSuccess: () => {
      toast({ tone: "success", title: "Plan assigned" });
      if (submitted && assignOrgId.trim() === submitted) void quotaQ.refetch();
    },
    onError: (e: unknown) => toast({ tone: "error", title: "Assignment failed", message: errorMessage(e, "The control plane rejected the request.") }),
  });

  const data = quotaQ.data as QuotaRead | undefined;
  const planForQuota = data ? plans.find((p) => p.code === data.quota.planCode) : undefined;
  const entitlements = (planForQuota?.entitlements && typeof planForQuota.entitlements === "object" && !Array.isArray(planForQuota.entitlements))
    ? planForQuota.entitlements as Record<string, unknown>
    : null;

  const limitOf = (key: string): number | undefined => {
    const value = entitlements?.[key];
    return typeof value === "number" && Number.isFinite(value) ? value : undefined;
  };

  const rows: Array<{ label: string; used: number | undefined; limitKey: string; unit?: (v: number) => string }> = [
    { label: "Servers", limitKey: "max_servers", used: data?.quota.serversCount },
    { label: "Environments", limitKey: "max_environments", used: data?.quota.environmentsCount },
    { label: "Nodes", limitKey: "max_nodes", used: data?.quota.nodesCount },
    { label: "Memory", limitKey: "max_memory_gb", unit: (v) => `${v.toFixed(1)} GiB`, used: data ? data.quota.memoryUsageBytes / (1024 ** 3) : undefined },
    { label: "Storage", limitKey: "max_storage_gb", unit: (v) => `${v.toFixed(1)} GiB`, used: data ? data.quota.storageBytes / (1024 ** 3) : undefined },
  ];

  return (
    <Card>
      <CardHeader
        action={submitted ? <FreshnessBadge state={sourceState(quotaQ)} /> : null}
        icon={Gauge}
        title="Organization quota & usage"
      />
      <div className="space-y-4 p-4">
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-56 flex-1">
            <Input label="Organization id" mono onChange={setOrgId} placeholder="uuid of the organization" value={orgId} />
            <p className="mt-1 text-[11px] leading-5 text-text-muted">Enter an identifier explicitly — no organization is selected for you, and a wrong one shows someone else&rsquo;s numbers.</p>
          </div>
          <Btn disabled={!orgId.trim() || quotaQ.isFetching} loading={quotaQ.isFetching} onClick={() => setSubmitted(orgId.trim())}>
            Load quota
          </Btn>
        </div>

        {!submitted ? (
          <EmptyState icon={Building2} message="Enter an organization id to read its assigned plan, its recomputed counters and its recent usage events." title="No organization selected" />
        ) : null}

        {submitted ? (
          <DataState
            emptyMessage="The organization returned no quota record."
            emptyTitle="No quota record"
            isEmpty={!data?.quota}
            loadingLabel="Reading quota, usage and events…"
            onRetry={() => void quotaQ.refetch()}
            state={sourceState(quotaQ)}
          >
            {data ? (
              <div className="space-y-4">
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <Pill tone="info">plan: {data.quota.planCode || "none assigned"}</Pill>
                  <span className="text-[11px] text-text-subtle">
                    quota recomputed {formatDate(data.quota.quotaUpdatedAt, "not reported")}
                    {data.quota.trialUntil ? ` · trial until ${formatDate(data.quota.trialUntil, "not reported")}` : ""}
                    {data.quota.prevPlanCode ? ` · previously ${data.quota.prevPlanCode}` : ""}
                  </span>
                </div>
                {!planForQuota ? (
                  <p className="text-[11px] leading-5 text-warn" role="status">
                    Plan <code className="font-mono">{data.quota.planCode || "—"}</code> is not in the plan list above, so its limits are unknown. The usage figures below are real; there is nothing to compare them against.
                  </p>
                ) : null}
                <div className="space-y-2">
                  {rows.map((row) => {
                    const limit = row.limitKey ? limitOf(row.limitKey) : undefined;
                    const known = typeof row.used === "number" && Number.isFinite(row.used);
                    const format = row.unit ?? ((v: number) => String(Math.round(v)));
                    return (
                      <SubsystemHealthMeter
                        key={row.label}
                        label={row.label}
                        percentage={known && typeof limit === "number" && limit > 0 ? (row.used! / limit) * 100 : null}
                        valueText={known
                          ? `${format(row.used!)}${typeof limit === "number" ? ` / ${format(limit)}` : " / limit not configured"}`
                          : "not reported"}
                      />
                    );
                  })}
                </div>
                <p className="text-[11px] leading-5 text-text-muted">
                  A dashed bar means the plan records no numeric limit for that dimension — the control plane does not treat an absent limit as unlimited, and neither does this view.
                </p>

                <div>
                  <div className="ui-label mb-1.5">Recent usage events</div>
                  {data.events.length === 0 ? (
                    <p className="text-[11px] text-text-subtle">No usage events recorded for this organization.</p>
                  ) : (
                    <AdminTable label="Recent usage events for this organization">
                      <AdminTHead>
                        <AdminTh>Resource</AdminTh>
                        <AdminTh>Quantity</AdminTh>
                        <AdminTh>Kind</AdminTh>
                        <AdminTh>Occurred</AdminTh>
                      </AdminTHead>
                      <AdminTBody>
                        {data.events.map((event) => (
                          <AdminTr key={event.id}>
                            <AdminTd className="font-mono text-xs text-text">{event.resource}</AdminTd>
                            <AdminTd className="font-mono text-xs">{event.quantity}</AdminTd>
                            <AdminTd className="text-xs text-text-subtle">{event.kind}</AdminTd>
                            <AdminTd className="text-xs text-text-subtle">{formatDate(event.occurredAt, "not reported")}</AdminTd>
                          </AdminTr>
                        ))}
                      </AdminTBody>
                    </AdminTable>
                  )}
                </div>

                <div className="rounded-lg border border-line bg-overlay-subtle p-3">
                  <p className="text-[11px] leading-5 text-text-subtle">
                    Metered totals for the same window: servers {data.usage.servers}, environments {data.usage.environments},
                    nodes {data.usage.nodes}, memory {data.usage.memoryBytes.toLocaleString()} bytes,
                    storage {data.usage.storageBytes.toLocaleString()} bytes, {data.usage.meterEvents} metering events
                    (plan {data.usage.plan || "not reported"}).
                  </p>
                </div>
              </div>
            ) : null}
          </DataState>
        ) : null}

        <div className="border-t border-line pt-4">
          <AdminFormField label="Assign a plan" hint="Replaces the organization's plan and triggers a quota recomputation on the server.">
            <div className="flex flex-wrap gap-2">
              <Input label="" mono onChange={setAssignOrgId} placeholder="organization id" value={assignOrgId} />
              <AdminSelect
                label=""
                onChange={setAssignPlan}
                options={plans.length > 0 ? plans.map((p) => ({ label: `${p.name} (${p.code})`, value: p.code })) : [{ label: "No plans available", value: "" }]}
                placeholder="Select a plan"
                value={assignPlan}
              />
              <Btn
                disabled={assignMut.isPending || !assignOrgId.trim() || !assignPlan}
                loading={assignMut.isPending}
                onClick={() => {
                  void (async () => {
                    const ok = await confirm({
                      title: "Reassign this organization's plan?",
                      description: `The organization ${assignOrgId.trim()} moves to ${assignPlan}. Its quota limits change immediately; workloads already running are not stopped, and usage over the new limits is reported by the next recomputation.`,
                      confirmLabel: "Assign plan",
                    });
                    if (ok) assignMut.mutate();
                  })();
                }}
                tone="primary"
              >
                Assign
              </Btn>
            </div>
          </AdminFormField>
        </div>
        {renderConfirm()}
      </div>
    </Card>
  );
}

function BillingSettingsCard() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const q = useQuery({ queryKey: ["admin-billing-settings"], queryFn: fetchBillingSettings, retry: false });
  const [secret, setSecret] = useState("");
  const [processor, setProcessor] = useState("");

  // Hydrate the processor select from the read. This used to run inside a
  // useMemo that called setState during render, which fires twice under Strict
  // Mode and defeats the guard it was trying to implement.
  useEffect(() => {
    if (q.data) setProcessor(q.data.externalProcessor ?? "none");
  }, [q.data]);

  const saveMut = useMutation({
    mutationFn: () => {
      const trimmedSecret = secret.trim();
      if (trimmedSecret && trimmedSecret.length < 16) throw new Error("The webhook secret must be at least 16 characters.");
      return updateBillingSettings({
        webhookSecret: trimmedSecret || undefined,
        externalProcessor: processor.trim() || undefined,
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-billing-settings"] });
      setSecret("");
      toast({ tone: "success", title: "Billing settings saved" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  const clearMut = useMutation({
    mutationFn: () => updateBillingSettings({ clearWebhookSecret: true, externalProcessor: processor.trim() || "none" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-billing-settings"] });
      toast({ tone: "success", title: "Webhook secret cleared", message: "Provider deliveries are now rejected until a new secret is set." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Clear failed", message: e.message }),
  });

  const state = sourceState(q);

  return (
    <Card>
      <CardHeader
        action={q.data ? <Pill tone={q.data.hasWebhookSecret ? "ok" : "warn"}>{q.data.hasWebhookSecret ? "secret set" : "no secret set"}</Pill> : null}
        icon={Shield}
        title="Webhook receiver & security"
      />
      {q.isPending ? <AdminLoadingState label="Loading billing settings…" /> : null}
      {q.isError ? (
        <div className="p-4">
          <AdminErrorState message={`${errorMessage(q.error, "Billing settings could not be read.")} Saving is disabled rather than writing over a configuration that could not be read.`} retry={() => void q.refetch()} />
        </div>
      ) : null}
      {!q.isPending && !q.isError && !q.data ? (
        <div className="p-4">
          <AdminErrorState message="The billing settings request returned no document, so nothing below is known about the current configuration." retry={() => void q.refetch()} />
        </div>
      ) : null}
      {q.data ? (
        <div className="space-y-4 p-4">
          <div className="rounded-lg border border-warn-line bg-warn-subtle p-3 text-sm leading-6 text-warn">
            The payment-processor receiver is public and verified with an HMAC signature over the raw body.
            Without a secret every provider delivery is rejected. The secret is write-only over the API — a
            read returns only whether one is set, so it cannot be displayed here.
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <Input
                autoComplete="new-password"
                label="Webhook secret (16+ characters, write-only)"
                mono
                onChange={setSecret}
                placeholder={q.data.hasWebhookSecret ? "replace the stored secret" : "set a new secret"}
                type="password"
                value={secret}
              />
              <p className="mt-1 text-[11px] leading-5 text-text-muted">
                {q.data.hasWebhookSecret
                  ? "A stored secret cannot be shown or read back. Entering a value replaces it; leaving the field blank keeps it."
                  : "No secret is stored yet, so provider deliveries are currently rejected."}
              </p>
            </div>
            <AdminSelect
              label="External processor"
              onChange={setProcessor}
              options={[{ label: "none", value: "none" }, { label: "stripe", value: "stripe" }, { label: "generic", value: "generic" }]}
              value={processor}
            />
          </div>

          {saveMut.isError ? <AdminErrorState message={errorMessage(saveMut.error, "The settings could not be saved.")} /> : null}

          <div className="flex flex-wrap gap-2">
            <Btn loading={saveMut.isPending} onClick={() => saveMut.mutate()} size="sm">
              <Save size={12} /> Save settings
            </Btn>
            {q.data.hasWebhookSecret ? (
              <Btn
                loading={clearMut.isPending}
                onClick={() => {
                  void (async () => {
                    const ok = await confirm({
                      title: "Clear the webhook secret?",
                      description: "Every incoming provider delivery will fail signature verification until a new secret is configured on both sides. This affects live billing events, and the previous secret cannot be recovered.",
                      danger: true,
                      confirmLabel: "Clear secret",
                    });
                    if (ok) clearMut.mutate();
                  })();
                }}
                size="sm"
                tone="danger"
              >
                <Trash2 size={12} /> Clear secret
              </Btn>
            ) : null}
            <Btn onClick={() => void q.refetch()} size="sm" tone="ghost">
              <RefreshCw size={12} /> Reload
            </Btn>
            <FreshnessBadge state={state} />
          </div>

          <p className="text-[11px] leading-5 text-text-subtle">
            Provider deliveries are recorded idempotently by event id, so a replayed webhook does not
            apply the same change twice. Last updated {formatDate(q.data.updatedAt, "not reported")}.
          </p>
        </div>
      ) : null}
      {renderConfirm()}
    </Card>
  );
}

function PlanModal({ plan, onClose, onDone }: { plan?: BillingPlan; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const [code, setCode] = useState(plan?.code ?? "");
  const [name, setName] = useState(plan?.name ?? "");
  const [cents, setCents] = useState(plan ? String(plan.centsPerMonth) : "");
  const [trialDays, setTrialDays] = useState(plan ? String(plan.trialDays) : "");
  const [active, setActive] = useState(plan?.active ?? true);
  const [entitlements, setEntitlements] = useState(() => {
    if (!plan) return '{\n  "max_servers": 10,\n  "max_memory_gb": 32,\n  "features": []\n}';
    try { return JSON.stringify(plan.entitlements, null, 2); } catch { return "{}"; }
  });

  const centsInvalid = cents.trim() !== "" && (!Number.isFinite(Number(cents)) || Number(cents) < 0);
  const trialInvalid = trialDays.trim() !== "" && (!Number.isInteger(Number(trialDays)) || Number(trialDays) < 0);
  const jsonInvalid = (() => {
    if (!entitlements.trim()) return false;
    try { JSON.parse(entitlements); return false; } catch { return true; }
  })();
  const canSubmit = code.trim() !== "" && name.trim() !== "" && !centsInvalid && !trialInvalid && !jsonInvalid;

  const createMut = useMutation({
    mutationFn: () => {
      const ent = entitlements.trim() ? JSON.parse(entitlements) as unknown : {};
      if (plan) {
        return updateBillingPlan(plan.id, {
          name: name.trim(),
          centsPerMonth: cents.trim() ? Number(cents) : 0,
          trialDays: trialDays.trim() ? Number(trialDays) : 0,
          active,
          entitlements: ent,
        });
      }
      return createBillingPlan({
        code: code.trim(),
        name: name.trim(),
        centsPerMonth: cents.trim() ? Number(cents) : 0,
        trialDays: trialDays.trim() ? Number(trialDays) : 0,
        active,
        entitlements: ent,
      });
    },
    onSuccess: () => {
      toast({ tone: "success", title: plan ? "Plan updated" : "Plan created" });
      onDone();
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  return (
    <Modal
      description={plan ? `Stored under the code ${plan.code}, which cannot be changed.` : "A plan is a named tier with a price and an entitlements document."}
      onClose={onClose}
      title={plan ? "Edit billing plan" : "Create billing plan"}
    >
      <div className="space-y-4">
        {!plan && <Input label="Code (unique, e.g. pro, free)" onChange={setCode} placeholder="pro" required value={code} mono />}
        <Input label="Name" onChange={setName} placeholder="Pro plan" required value={name} />
        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <Input label="Price per month (cents)" onChange={setCents} placeholder="4900" type="number" value={cents} mono />
            <p className="mt-1 text-[11px] leading-5 text-text-muted">
              {cents.trim() && !centsInvalid ? `Displays as ${priceFormatter.format(Number(cents) / 100)} per month. ` : ""}
              Prices are stored in cents and shown in US dollars; this view does not read the panel currency format.
            </p>
            {centsInvalid ? <p className="mt-1 text-[11px] text-danger" role="alert">Enter a whole number of cents, or leave it empty for a free tier.</p> : null}
          </div>
          <div>
            <Input label="Trial days" onChange={setTrialDays} placeholder="14" type="number" value={trialDays} mono />
            {trialInvalid ? <p className="mt-1 text-[11px] text-danger" role="alert">Trial days must be a whole number of days.</p> : null}
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm text-text">
          <input checked={active} className="accent-[var(--brand)]" onChange={(e) => setActive(e.target.checked)} type="checkbox" /> Active
        </label>
        <div>
          <Textarea label="Entitlements (JSON)" onChange={setEntitlements} placeholder='{"max_servers": 10}' rows={6} value={entitlements} />
          {jsonInvalid ? <p className="mt-1 text-[11px] text-danger" role="alert">Entitlements must be valid JSON.</p> : null}
          {!jsonInvalid ? <p className="mt-1 text-[11px] leading-5 text-text-muted">An empty document means no limits are configured for this plan; it is not a claim of unlimited capacity.</p> : null}
        </div>
        {createMut.isError ? <AdminErrorState message={errorMessage(createMut.error, "The plan could not be saved.")} /> : null}
      </div>
      <ModalFooter
        confirmLabel={plan ? "Save" : "Create"}
        disabled={!canSubmit || createMut.isPending}
        onCancel={onClose}
        onConfirm={() => createMut.mutate()}
      />
    </Modal>
  );
}
