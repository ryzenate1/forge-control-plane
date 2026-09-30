"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import {
  GanttChart, Globe, Plus, RefreshCw, Shield, ShieldCheck,
  ShieldOff, SlidersHorizontal, Trash2, Zap,
} from "lucide-react";
import { fetchJSON, postJSON, putJSON, deleteJSON, unwrapList } from "@/lib/api";
import { listTargetGroups } from "@/lib/api/loadbalancer";
import { AdminErrorState, AdminLoadingState, AdminPageLayout, AdminTabs, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";

const HTTP_METHODS = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE"];

/** The policy config is one free-text JSON box for four different policy types,
 *  so the shape each type expects is stated where it is typed rather than
 *  discovered from a 400. */
const CONFIG_HINT: Record<TrafficPolicy["type"], string> = {
  rate_limit: 'e.g. {"requests_per_second": 100, "burst": 50}',
  ip_whitelist: 'e.g. {"cidrs": ["203.0.113.0/24"]}',
  ip_blacklist: 'e.g. {"cidrs": ["198.51.100.7/32"]}',
  circuit_breaker: 'e.g. {"failure_threshold": 5, "recovery_seconds": 30}',
};

/** Parse before submit: an invalid box used to stay enabled and throw a sentinel
 *  string from inside the mutation, which the toast then swallowed by matching
 *  on the message. */
function parsePolicyConfig(raw: string): Record<string, unknown> | string {
  let parsed: unknown;
  try { parsed = JSON.parse(raw || "{}"); }
  catch { return "Config must be valid JSON."; }
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) return "Config must be a JSON object, like {\"requests_per_second\": 100}.";
  return parsed as Record<string, unknown>;
}

/** Keys that carry credentials must not be printed into a list row. */
const SECRET_KEY = /(token|secret|key|password|credential|authorization)/i;
function describeConfig(config: Record<string, unknown>): string {
  const parts = Object.entries(config ?? {}).map(([k, v]) => {
    if (SECRET_KEY.test(k)) return `${k}: ••••••`;
    if (v === null || v === undefined) return `${k}: not set`;
    if (typeof v === "object") return `${k}: ${JSON.stringify(v)}`;
    return `${k}: ${String(v)}`;
  });
  return parts.length ? parts.join(", ") : "no settings";
}

/** `get,get,GET,foo` used to submit unchanged: no upper-casing, no de-duplication
 *  and no check that the token is an HTTP method at all. */
function normalizedMethods(value: string): string[] {
  return [...new Set(value.split(",").map((m) => m.trim().toUpperCase()).filter(Boolean))];
}
function methodsError(value: string): string {
  const list = normalizedMethods(value);
  if (list.length === 0) return "List at least one HTTP method.";
  const unknown = list.filter((m) => !HTTP_METHODS.includes(m));
  return unknown.length ? `Not an HTTP method: ${unknown.join(", ")}.` : "";
}

type RouteRule = {
  id: string;
  path: string;
  targetGroup: string;
  priority: number;
  methods?: string[];
  enabled: boolean;
  createdAt: string;
};

type TrafficPolicy = {
  id: string;
  type: "rate_limit" | "ip_whitelist" | "ip_blacklist" | "circuit_breaker";
  name: string;
  config: Record<string, unknown>;
  enabled: boolean;
  createdAt: string;
};

type Tab = "routes" | "policies";

const defaultRouteForm = {
  path: "",
  targetGroup: "",
  priority: 100,
  methods: "GET,POST",
  enabled: true,
};

const defaultPolicyForm = {
  type: "rate_limit" as TrafficPolicy["type"],
  name: "",
  config: "{}",
  enabled: true,
};

export default function AdminTrafficPage() {
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>("routes");
  const [search, setSearch] = useState("");
  const [showCreateRoute, setShowCreateRoute] = useState(false);
  const [editingRoute, setEditingRoute] = useState<RouteRule | null>(null);
  const [routeForm, setRouteForm] = useState(defaultRouteForm);
  const [showCreatePolicy, setShowCreatePolicy] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<TrafficPolicy | null>(null);
  const [policyForm, setPolicyForm] = useState(defaultPolicyForm);
  const [policySearch, setPolicySearch] = useState("");
  const [policyConfigError, setPolicyConfigError] = useState<string | null>(null);

  const routesQuery = useQuery({
    queryKey: ["admin", "traffic", "rules"],
    queryFn: async () => unwrapList(await fetchJSON<RouteRule[]>("/admin/traffic/rules")),
  });

  const policiesQuery = useQuery({
    queryKey: ["admin", "traffic", "policies"],
    queryFn: async () => unwrapList(await fetchJSON<TrafficPolicy[]>("/admin/traffic/policies")),
  });

  const routes = useMemo(() => routesQuery.data ?? [], [routesQuery.data]);
  const policies = useMemo(() => policiesQuery.data ?? [], [policiesQuery.data]);

  const filteredRoutes = routes.filter((r) =>
    !search || r.path.toLowerCase().includes(search.toLowerCase()) || r.targetGroup.toLowerCase().includes(search.toLowerCase())
  );

  const filteredPolicies = policies.filter((p) =>
    !policySearch || p.name.toLowerCase().includes(policySearch.toLowerCase())
  );

  const createRouteMutation = useMutation({
    mutationFn: () => postJSON("/admin/traffic/rules", { ...routeForm, methods: normalizedMethods(routeForm.methods) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "rules"] });
      setShowCreateRoute(false);
      setRouteForm(defaultRouteForm);
      toast({ tone: "success", title: "Route rule created" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create route", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const updateRouteMutation = useMutation({
    mutationFn: () =>
      putJSON(`/admin/traffic/rules/${encodeURIComponent(editingRoute!.id)}`, { ...routeForm, methods: normalizedMethods(routeForm.methods) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "rules"] });
      setEditingRoute(null);
      toast({ tone: "success", title: "Route rule updated" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to update route", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteRouteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/traffic/rules/${encodeURIComponent(id)}`),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "rules"] }); toast({ tone: "success", title: "Route rule deleted" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to delete route", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const createPolicyMutation = useMutation({
    mutationFn: () => postJSON("/admin/traffic/policies", { ...policyForm, config: parsePolicyConfig(policyForm.config) as Record<string, unknown> }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "policies"] });
      setShowCreatePolicy(false);
      setPolicyForm(defaultPolicyForm);
      setPolicyConfigError(null);
      toast({ tone: "success", title: "Traffic policy created" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create policy", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deletePolicyMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/traffic/policies/${encodeURIComponent(id)}`),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "policies"] }); toast({ tone: "success", title: "Traffic policy deleted" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to delete policy", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const updatePolicyMutation = useMutation({
    mutationFn: () => putJSON(`/admin/traffic/policies/${encodeURIComponent(editingPolicy!.id)}`, { ...policyForm, config: parsePolicyConfig(policyForm.config) as Record<string, unknown> }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "policies"] });
      setEditingPolicy(null);
      setPolicyForm(defaultPolicyForm);
      setPolicyConfigError(null);
      toast({ tone: "success", title: "Traffic policy updated" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to update policy", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  // `POST /admin/traffic/sync` answers 204 with no body (handlers_trafficmanager.go:115),
  // so there is no count to report and no way for this page to confirm what the
  // gateway actually applied. It also never toasted on success, which made a
  // sync indistinguishable from nothing happening.
  const syncRoutesMutation = useMutation({
    mutationFn: () => postJSON("/admin/traffic/sync"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "rules"] });
      toast({ tone: "success", title: "Route sync requested", message: "The gateway was asked to reload its routes. It reports no result, so this page cannot confirm how many rules were applied." });
    },
    onError: (err) => toast({ tone: "error", title: "Route sync failed", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  // Target groups are a Load Balancer resource; a route naming one that does not
  // exist is accepted by the API and silently forwards nowhere, so the list is
  // fetched and offered as a choice instead of a free-text field.
  const groupsQuery = useQuery({ queryKey: ["admin", "load-balancer", "groups"], queryFn: () => listTargetGroups() });
  const groupNames = useMemo(() => (groupsQuery.data ?? []).map((g) => g.name), [groupsQuery.data]);

  // Everything the gateway would reject is caught before submit, not after.
  const routeError =
    !routeForm.path.trim() ? "A path is required, e.g. /api/v1/servers."
      : !routeForm.path.trim().startsWith("/") ? "The path must start with /."
        : !routeForm.targetGroup.trim() ? "Choose the target group this route forwards to."
          : !Number.isInteger(routeForm.priority) || routeForm.priority < 0 ? "Priority must be a whole number of 0 or more."
            : methodsError(routeForm.methods);

  const groupsNote = groupsQuery.isError
    ? "Target groups could not be loaded, so no list is available — type the group name."
    : groupsQuery.isPending
      ? "Loading target groups…"
      : groupNames.length === 0
        ? "No target group exists yet, so a route created here would forward nowhere. Create one under Load Balancer first."
        : "";

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "routes", label: "Route Rules" },
    { id: "policies", label: "Traffic Policies" },
  ];

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={sourceState(tab === "routes" ? routesQuery : policiesQuery)} />}
        action={
          <Btn tone="ghost" onClick={() => syncRoutesMutation.mutate()} disabled={syncRoutesMutation.isPending}>
            <RefreshCw size={14} /> {syncRoutesMutation.isPending ? "Syncing…" : "Sync Routes"}
          </Btn>
        }
      />

      <AdminTabs tabs={tabs} active={tab} onChange={(id) => setTab(id as Tab)} />

      {tab === "routes" && (
        <Card>
          <CardHeader
            title="Route Rules"
            icon={GanttChart}
            action={
              <Btn size="sm" tone="primary" onClick={() => setShowCreateRoute(true)}>
                <Plus size={12} /> Create Route
              </Btn>
            }
          />
          <div className="flex items-center gap-3 p-4">
            <Input aria-label="Search route rules by path or target group" placeholder="Search routes..." value={search} onChange={setSearch} />
          </div>
          {routesQuery.isPending ? (
            <div className="p-4"><AdminLoadingState label="Loading route rules…" /></div>
          ) : routesQuery.isError ? (
            <div className="p-4"><AdminErrorState message={`Route rules could not be loaded: ${routesQuery.error instanceof Error ? routesQuery.error.message : "request error"}. This is a failed read, not an empty gateway.`} retry={() => void routesQuery.refetch()} /></div>
          ) : filteredRoutes.length === 0 ? (
            <EmptyState
              icon={Globe}
              message={search
                ? `No route rule matches “${search}”.`
                : "No route rule has been created. The gateway forwards only what is listed here."}
              title={search ? "No routes match the search" : "No route rules"}
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-line text-left text-xs uppercase tracking-widest text-text-subtle">
                    <th className="px-4 py-3">Path</th>
                    <th className="px-4 py-3">Target Group</th>
                    <th className="px-4 py-3">Priority</th>
                    <th className="px-4 py-3">Methods</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3"><span className="sr-only">Actions</span></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {filteredRoutes.map((rule) => (
                    <tr key={rule.id} className="hover:bg-overlay-subtle">
                      <td className="px-4 py-3 font-mono text-xs font-medium text-text" title={rule.path}>{rule.path}</td>
                      <td className="px-4 py-3 text-xs text-text-subtle" title={rule.targetGroup}>{rule.targetGroup || <span className="text-warn">No target group named</span>}</td>
                      <td className="px-4 py-3 text-xs text-text-subtle">{Number.isFinite(rule.priority) ? rule.priority : "Not set"}</td>
                      <td className="px-4 py-3 text-xs text-text-subtle">{(rule.methods ?? []).length ? rule.methods!.join(", ") : "Not recorded"}</td>
                      <td className="px-4 py-3">
                        <Pill tone={rule.enabled ? "green" : "neutral"}>{rule.enabled ? "Enabled" : "Disabled"}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1">
                          <Btn ariaLabel={`Edit the route rule ${rule.path}`} size="sm" tone="ghost" onClick={() => { setEditingRoute(rule); setRouteForm({ ...rule, methods: (rule.methods ?? []).join(", ") }); }}>Edit</Btn>
                          <Btn ariaLabel={`Delete the route rule ${rule.path}`} size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete route "${rule.path || rule.id.slice(0, 8)}"?`, description: `Traffic matching ${rule.path || "this route"} will stop being forwarded to ${rule.targetGroup || "its target group"}. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteRouteMutation.mutate(rule.id); })(); }}>
                            <Trash2 size={12} />
                          </Btn>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {tab === "policies" && (
        <Card>
          <CardHeader
            title="Traffic Policies"
            icon={Shield}
            action={
              <Btn size="sm" tone="primary" onClick={() => setShowCreatePolicy(true)}>
                <Plus size={12} /> Create Policy
              </Btn>
            }
          />
          <div className="flex items-center gap-3 p-4">
            <Input aria-label="Search policies by name" placeholder="Search policies..." value={policySearch} onChange={setPolicySearch} />
          </div>
          {policiesQuery.isPending ? (
            <div className="p-4"><AdminLoadingState label="Loading traffic policies…" /></div>
          ) : policiesQuery.isError ? (
            <div className="p-4"><AdminErrorState message={`Traffic policies could not be loaded: ${policiesQuery.error instanceof Error ? policiesQuery.error.message : "request error"}. This is a failed read, not an unfiltered gateway.`} retry={() => void policiesQuery.refetch()} /></div>
          ) : filteredPolicies.length === 0 ? (
            <EmptyState
              icon={Shield}
              message={policySearch ? `No traffic policy is named “${policySearch}”.` : "No traffic policy has been created. Rate limits and IP rules live here."}
              title={policySearch ? "No policies match the search" : "No traffic policies"}
            />
          ) : (
            <div className="divide-y divide-line">
              {filteredPolicies.map((p) => (
                <div key={p.id} className="flex items-center justify-between px-4 py-3">
                  <div className="flex items-center gap-3">
                    {p.type === "rate_limit" ? (
                      <SlidersHorizontal size={16} className="text-warn" />
                    ) : p.type === "ip_whitelist" ? (
                      <ShieldCheck size={16} className="text-ok" />
                    ) : p.type === "ip_blacklist" ? (
                      <ShieldOff size={16} className="text-danger" />
                    ) : (
                      <Zap size={16} className="text-text-subtle" />
                    )}
                    <div>
                      <p className="text-sm font-medium text-text">{p.name}</p>
                      <p className="text-xs text-text-subtle">
                        {p.type.replace("_", " ")} — {describeConfig(p.config ?? {})}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone={p.enabled ? "green" : "neutral"}>{p.enabled ? "Enabled" : "Disabled"}</Pill>
                    <Btn size="sm" tone="ghost" onClick={() => { setEditingPolicy(p); setPolicyConfigError(null); setPolicyForm({ type: p.type, name: p.name, config: JSON.stringify(p.config ?? {}, null, 2), enabled: p.enabled ?? true }); }}>Edit</Btn>
                    <Btn ariaLabel={`Delete the traffic policy ${p.name}`} size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete the policy “${p.name}”?`, description: `Its ${p.type.replace("_", " ")} settings (${describeConfig(p.config ?? {})}) stop applying to matching traffic as soon as the gateway reloads. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deletePolicyMutation.mutate(p.id); })(); }}>
                      <Trash2 size={12} />
                    </Btn>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}

      {showCreateRoute && (
        <RouteFormModal
          title="Create Route Rule"
          form={routeForm}
          onChange={setRouteForm}
          onSave={() => createRouteMutation.mutate()}
          onClose={() => { setShowCreateRoute(false); setRouteForm(defaultRouteForm); }}
          saving={createRouteMutation.isPending}
          groupNames={groupNames}
          groupsNote={groupsNote}
          error={routeError}
        />
      )}

      {editingRoute && (
        <RouteFormModal
          title="Edit Route Rule"
          form={routeForm}
          onChange={setRouteForm}
          onSave={() => updateRouteMutation.mutate()}
          onClose={() => setEditingRoute(null)}
          saving={updateRouteMutation.isPending}
          groupNames={groupNames}
          groupsNote={groupsNote}
          error={routeError}
        />
      )}

      {showCreatePolicy && (
        <Modal title="Create Traffic Policy" onClose={() => { setShowCreatePolicy(false); setPolicyConfigError(null); }}>
          <PolicyFormFields form={policyForm} onChange={(f) => { setPolicyForm(f); setPolicyConfigError(null); }} configError={policyConfigError} />
          <ModalFooter
            onCancel={() => { setShowCreatePolicy(false); setPolicyConfigError(null); }}
            onConfirm={() => {
              const parsed = parsePolicyConfig(policyForm.config);
              if (typeof parsed === "string") { setPolicyConfigError(parsed); return; }
              createPolicyMutation.mutate();
            }}
            confirmLabel={createPolicyMutation.isPending ? "Creating…" : "Create"}
            disabled={createPolicyMutation.isPending || !policyForm.name.trim() || typeof parsePolicyConfig(policyForm.config) === "string"}
          />
        </Modal>
      )}
      {editingPolicy && (
        <Modal title="Edit Traffic Policy" onClose={() => { setEditingPolicy(null); setPolicyForm(defaultPolicyForm); setPolicyConfigError(null); }}>
          <PolicyFormFields form={policyForm} onChange={(f) => { setPolicyForm(f); setPolicyConfigError(null); }} configError={policyConfigError} />
          <ModalFooter
            onCancel={() => { setEditingPolicy(null); setPolicyForm(defaultPolicyForm); setPolicyConfigError(null); }}
            onConfirm={() => {
              const parsed = parsePolicyConfig(policyForm.config);
              if (typeof parsed === "string") { setPolicyConfigError(parsed); return; }
              updatePolicyMutation.mutate();
            }}
            confirmLabel={updatePolicyMutation.isPending ? "Saving…" : "Save"}
            disabled={updatePolicyMutation.isPending || !policyForm.name.trim() || typeof parsePolicyConfig(policyForm.config) === "string"}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}


function RouteFormModal({
  title, form, onChange, onSave, onClose, saving, groupNames, groupsNote, error,
}: {
  title: string;
  form: typeof defaultRouteForm;
  onChange: (f: typeof defaultRouteForm) => void;
  onSave: () => void;
  onClose: () => void;
  saving: boolean;
  groupNames: string[];
  groupsNote: string;
  error: string;
}) {
  const knownGroup = !form.targetGroup || groupNames.includes(form.targetGroup);
  return (
    <Modal title={title} onClose={onClose}>
      <div className="space-y-4">
        <Input label="Path" value={form.path} onChange={(v) => onChange({ ...form, path: v })} placeholder="/api/v1/servers" />
        <div>
          <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle" htmlFor="route-target-group">Target Group</label>
          {/* A route naming a group that does not exist is accepted by the API and
              forwards nowhere, so the Load Balancer's groups are offered as a list. */}
          {groupNames.length > 0 ? (
            <select aria-label="Target group this route forwards to" className="ui-input" id="route-target-group" value={form.targetGroup} onChange={(e) => onChange({ ...form, targetGroup: e.target.value })}>
              <option value="">Select a target group</option>
              {!knownGroup && form.targetGroup ? <option value={form.targetGroup}>{form.targetGroup} — not in the current list</option> : null}
              {groupNames.map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
          ) : (
            <Input mono onChange={(v) => onChange({ ...form, targetGroup: v })} placeholder="prod-servers" value={form.targetGroup} />
          )}
          {groupsNote ? <p className="mt-1.5 text-xs text-text-subtle">{groupsNote}</p> : null}
        </div>
        <Input label="Priority" mono type="number" value={String(form.priority)} onChange={(v) => onChange({ ...form, priority: Number(v) })} />
        <div>
          <Input label="HTTP Methods (comma separated)" mono value={form.methods} onChange={(v) => onChange({ ...form, methods: v })} placeholder="GET,POST,PUT,DELETE" />
          <p className="mt-1.5 text-xs text-text-subtle">Comma-separated HTTP methods, e.g. GET, POST. Matching is case-insensitive.</p>
        </div>
        <label className="flex items-center gap-2 text-sm font-medium text-text">
          <input type="checkbox" checked={form.enabled} onChange={(e) => onChange({ ...form, enabled: e.target.checked })} className="rounded border-line bg-[var(--surface-input)]" />
          {form.enabled ? "Enabled — this route forwards traffic" : "Disabled — this route is stored but does not forward traffic"}
        </label>
        {error ? <p className="text-xs text-danger" role="alert">{error}</p> : null}
      </div>
      <ModalFooter onCancel={onClose} onConfirm={onSave} confirmLabel={saving ? "Saving…" : "Save"} disabled={saving || !!error} />
    </Modal>
  );
}

function PolicyFormFields({ form, onChange, configError }: {
  form: typeof defaultPolicyForm;
  onChange: (f: typeof defaultPolicyForm) => void;
  configError: string | null;
}) {
  const liveError = (() => { const r = parsePolicyConfig(form.config); return typeof r === "string" ? r : ""; })();
  const selectClass = "h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none focus:border-brand focus:ring-1 focus:ring-brand/30";
  return (
    <div className="space-y-4">
      <Input label="Name" value={form.name} onChange={(v) => onChange({ ...form, name: v })} placeholder="Rate limit API" />
      <div>
        <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle" htmlFor="policy-type">Type</label>
        <select aria-label="Policy type" className={selectClass} id="policy-type" value={form.type} onChange={(e) => onChange({ ...form, type: e.target.value as TrafficPolicy["type"] })}>
          <option value="rate_limit">Rate Limit</option>
          <option value="ip_whitelist">IP Whitelist</option>
          <option value="ip_blacklist">IP Blacklist</option>
          <option value="circuit_breaker">Circuit Breaker</option>
        </select>
      </div>
      <div>
        <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle" htmlFor="policy-config">Config (JSON)</label>
        {/* A multi-line field: the stored config is re-serialised with newlines,
            and a one-line input showed a different value than the one submitted. */}
        <textarea
          aria-label="Policy configuration as JSON"
          autoComplete="off"
          className="ui-input h-28 w-full font-mono text-xs"
          id="policy-config"
          onChange={(e) => onChange({ ...form, config: e.target.value })}
          spellCheck={false}
          value={form.config}
        />
        <p className="mt-1.5 text-xs text-text-subtle">{CONFIG_HINT[form.type]}</p>
        {(liveError || configError) ? <p className="mt-1 text-xs text-danger" role="alert">{liveError || configError}</p> : null}
      </div>
      <label className="flex items-center gap-2 text-sm font-medium text-text">
        <input type="checkbox" checked={form.enabled} onChange={(e) => onChange({ ...form, enabled: e.target.checked })} className="rounded border-line bg-[var(--surface-input)]" />
        {form.enabled ? "Enabled — this policy applies to matching traffic" : "Disabled — this policy is stored but does not apply"}
      </label>
    </div>
  );
}
