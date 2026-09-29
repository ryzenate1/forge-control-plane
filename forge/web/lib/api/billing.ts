import { fetchJSON, postJSON, putJSON, deleteJSON, unwrapData, unwrapList } from "./http";

export type BillingPlan = {
  id: string;
  code: string;
  name: string;
  centsPerMonth: number;
  entitlements: unknown;
  trialDays: number;
  active: boolean;
  createdAt: string;
  updatedAt: string;
};

export type OrgQuota = {
  orgId: string;
  planCode: string;
  trialUntil?: string | null;
  memoryUsageBytes: number;
  serversCount: number;
  environmentsCount: number;
  nodesCount: number;
  storageBytes: number;
  prevPlanCode: string;
  quotaUpdatedAt: string;
};

export type UsageEvent = {
  id: string;
  orgId: string;
  resource: string;
  quantity: number;
  kind: string;
  occurredAt: string;
};

export type BillingSettings = {
  id: boolean;
  hasWebhookSecret: boolean;
  externalProcessor: string;
  updatedAt: string;
};

export type BillingWebhookReceipt = {
  id: string;
  provider: string;
  eventId: string;
  eventType: string;
  status: string;
  gotAt: string;
};

export async function fetchBillingPlans(): Promise<BillingPlan[]> {
  return unwrapList(await fetchJSON<{ data: BillingPlan[] } | BillingPlan[]>("/billing/plans"));
}

export async function fetchBillingPlanByCode(code: string): Promise<BillingPlan> {
  return unwrapData(await fetchJSON<{ data: BillingPlan } | BillingPlan>(`/billing/plans/${encodeURIComponent(code)}`));
}

export async function createBillingPlan(input: {
  code: string;
  name: string;
  centsPerMonth: number;
  entitlements?: unknown;
  trialDays?: number;
  active?: boolean;
}): Promise<BillingPlan> {
  return unwrapData(
    await postJSON<{ data: BillingPlan } | BillingPlan>("/billing/plans", {
      code: input.code,
      name: input.name,
      centsPerMonth: input.centsPerMonth,
      entitlements: input.entitlements ?? {},
      trialDays: input.trialDays ?? 0,
      active: input.active ?? true,
    }),
  );
}

export async function updateBillingPlan(
  id: string,
  patch: { name?: string; centsPerMonth?: number; entitlements?: unknown; trialDays?: number; active?: boolean },
): Promise<BillingPlan> {
  // Strip `undefined` fields so a partial patch never serializes explicit
  // `null`s/overwrites for keys the caller did not set. `JSON.stringify`
  // drops `undefined` anyway, but building the body explicitly keeps the
  // intent visible and avoids sending `{ name: undefined }` shapes to mocks.
  const body: Record<string, unknown> = {};
  if (patch.name !== undefined) body.name = patch.name;
  if (patch.centsPerMonth !== undefined) body.centsPerMonth = patch.centsPerMonth;
  if (patch.entitlements !== undefined) body.entitlements = patch.entitlements;
  if (patch.trialDays !== undefined) body.trialDays = patch.trialDays;
  if (patch.active !== undefined) body.active = patch.active;
  return unwrapData(
    await putJSON<{ data: BillingPlan } | BillingPlan>(`/billing/plans/${encodeURIComponent(id)}`, body),
  );
}

export async function deleteBillingPlan(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/billing/plans/${encodeURIComponent(id)}`);
}

export async function fetchOrgQuota(orgId: string): Promise<OrgQuota> {
  return unwrapData(
    await fetchJSON<{ data: OrgQuota } | OrgQuota>(`/billing/org/${encodeURIComponent(orgId)}/quota`),
  );
}

export async function setOrgPlan(orgId: string, planCode: string, trial = false): Promise<OrgQuota> {
  return unwrapData(
    await postJSON<{ data: OrgQuota } | OrgQuota>(`/billing/org/${encodeURIComponent(orgId)}/plan`, {
      planCode,
      trial,
    }),
  );
}

export type UsageSummary = {
  plan: string;
  servers: number;
  memoryBytes: number;
  storageBytes: number;
  environments: number;
  nodes: number;
  meterEvents: number;
  // allow extra fields from API without breaking
  [key: string]: unknown;
};

export async function fetchOrgUsage(orgId: string): Promise<UsageSummary> {
  return unwrapData(
    await fetchJSON<{ data: UsageSummary } | UsageSummary>(`/billing/org/${encodeURIComponent(orgId)}/usage`),
  );
}

export async function fetchOrgUsageEvents(orgId: string, since?: string, limit = 100): Promise<UsageEvent[]> {
  const q = new URLSearchParams();
  if (since) q.set("since", since);
  q.set("limit", String(limit));
  return unwrapList(
    await fetchJSON<{ data: UsageEvent[] } | UsageEvent[]>(
      `/billing/org/${encodeURIComponent(orgId)}/usage-events?${q.toString()}`,
    ),
  );
}

export async function recordBillingUsage(orgId: string, resource: string, quantity: number): Promise<unknown> {
  return postJSON<unknown>("/billing/usage", { orgId, resource, quantity });
}

export async function fetchBillingSettings(): Promise<BillingSettings> {
  return unwrapData(
    await fetchJSON<{ data: BillingSettings } | BillingSettings>("/billing/settings"),
  );
}

export async function updateBillingSettings(input: {
  webhookSecret?: string;
  externalProcessor?: string;
  clearWebhookSecret?: boolean;
}): Promise<BillingSettings> {
  return unwrapData(
    await putJSON<{ data: BillingSettings } | BillingSettings>("/billing/settings", input),
  );
}

export async function sendBillingWebhookTest(input: { provider?: string; eventId: string; eventType?: string; payload?: unknown }): Promise<BillingWebhookReceipt> {
  // Public webhook is HMAC-verified; this helper is for admin to test via the same path using an explicit header.
  // In production the HMAC secret must already be configured; otherwise the call will 401.
  const headers: Record<string, string> = {
    "X-Billing-Event-Id": input.eventId,
  };
  if (input.provider) headers["X-Billing-Provider"] = input.provider;
  if (input.eventType) headers["X-Billing-Event-Type"] = input.eventType;
  return unwrapData(
    await postJSON<{ data: BillingWebhookReceipt } | BillingWebhookReceipt>(
      "/billing/webhook",
      input.payload ?? { type: input.eventType ?? "test", test: true },
      { headers },
    ),
  );
}
