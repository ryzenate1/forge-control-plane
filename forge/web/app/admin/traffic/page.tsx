"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  GanttChart, Globe, Plus, RefreshCw, Shield, ShieldCheck,
  ShieldOff, SlidersHorizontal, Trash2, Zap,
} from "lucide-react";
import { fetchJSON, postJSON, patchJSON, deleteJSON } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";

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
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("routes");
  const [search, setSearch] = useState("");
  const [showCreateRoute, setShowCreateRoute] = useState(false);
  const [editingRoute, setEditingRoute] = useState<RouteRule | null>(null);
  const [routeForm, setRouteForm] = useState(defaultRouteForm);
  const [showCreatePolicy, setShowCreatePolicy] = useState(false);
  const [policyForm, setPolicyForm] = useState(defaultPolicyForm);
  const [policySearch, setPolicySearch] = useState("");

  const routesQuery = useQuery({
    queryKey: ["admin", "traffic", "routes"],
    queryFn: () => fetchJSON<RouteRule[]>("/admin/traffic/routes"),
  });

  const policiesQuery = useQuery({
    queryKey: ["admin", "traffic", "policies"],
    queryFn: () => fetchJSON<TrafficPolicy[]>("/admin/traffic/policies"),
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
    mutationFn: () => postJSON("/admin/traffic/routes", { ...routeForm, methods: routeForm.methods.split(",").map((m) => m.trim()) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "routes"] });
      setShowCreateRoute(false);
      setRouteForm(defaultRouteForm);
    },
  });

  const updateRouteMutation = useMutation({
    mutationFn: () =>
      patchJSON(`/admin/traffic/routes/${editingRoute!.id}`, { ...routeForm, methods: routeForm.methods.split(",").map((m) => m.trim()) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "routes"] });
      setEditingRoute(null);
    },
  });

  const deleteRouteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/traffic/routes/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "routes"] }),
  });

  const createPolicyMutation = useMutation({
    mutationFn: () => postJSON("/admin/traffic/policies", { ...policyForm, config: JSON.parse(policyForm.config) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "policies"] });
      setShowCreatePolicy(false);
      setPolicyForm(defaultPolicyForm);
    },
  });

  const deletePolicyMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/traffic/policies/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "policies"] }),
  });

  const syncRoutesMutation = useMutation({
    mutationFn: () => postJSON("/admin/traffic/routes/sync"),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "traffic", "routes"] }),
  });

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "routes", label: "Route Rules" },
    { id: "policies", label: "Traffic Policies" },
  ];

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Traffic Management"
        sub="Route rules and traffic policies for the API gateway."
        action={
          <Btn tone="ghost" onClick={() => syncRoutesMutation.mutate()} disabled={syncRoutesMutation.isPending}>
            <RefreshCw size={14} /> Sync Routes
          </Btn>
        }
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
            <Input placeholder="Search routes..." value={search} onChange={setSearch} />
          </div>
          {routesQuery.isLoading ? (
            <div className="p-8 text-center text-sm text-slate-500">Loading routes...</div>
          ) : filteredRoutes.length === 0 ? (
            <EmptyState icon={Globe} message="No route rules configured." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                    <th className="px-4 py-3">Path</th>
                    <th className="px-4 py-3">Target Group</th>
                    <th className="px-4 py-3">Priority</th>
                    <th className="px-4 py-3">Methods</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {filteredRoutes.map((rule) => (
                    <tr key={rule.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">{rule.path}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{rule.targetGroup}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{rule.priority}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{(rule.methods ?? ["ALL"]).join(", ")}</td>
                      <td className="px-4 py-3">
                        <Pill tone={rule.enabled ? "green" : "neutral"}>{rule.enabled ? "Active" : "Inactive"}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1">
                          <Btn size="sm" tone="ghost" onClick={() => { setEditingRoute(rule); setRouteForm({ ...rule, methods: (rule.methods ?? ["ALL"]).join(", ") }); }}>Edit</Btn>
                          <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete route?")) deleteRouteMutation.mutate(rule.id); }}>
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
            <Input placeholder="Search policies..." value={policySearch} onChange={setPolicySearch} />
          </div>
          {policiesQuery.isLoading ? (
            <div className="p-8 text-center text-sm text-slate-500">Loading policies...</div>
          ) : filteredPolicies.length === 0 ? (
            <EmptyState icon={Shield} message="No traffic policies configured." />
          ) : (
            <div className="divide-y divide-white/[0.04]">
              {filteredPolicies.map((p) => (
                <div key={p.id} className="flex items-center justify-between px-4 py-3">
                  <div className="flex items-center gap-3">
                    {p.type === "rate_limit" ? (
                      <SlidersHorizontal size={16} className="text-amber-400" />
                    ) : p.type === "ip_whitelist" ? (
                      <ShieldCheck size={16} className="text-emerald-400" />
                    ) : p.type === "ip_blacklist" ? (
                      <ShieldOff size={16} className="text-red-400" />
                    ) : (
                      <Zap size={16} className="text-blue-400" />
                    )}
                    <div>
                      <p className="text-sm font-medium text-slate-200">{p.name}</p>
                      <p className="text-xs text-slate-500">
                        {p.type.replace("_", " ")} — {Object.entries(p.config).map(([k, v]) => `${k}: ${v}`).join(", ")}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone={p.enabled ? "green" : "neutral"}>{p.enabled ? "Enabled" : "Disabled"}</Pill>
                    <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete policy?")) deletePolicyMutation.mutate(p.id); }}>
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
        />
      )}

      {showCreatePolicy && (
        <Modal title="Create Traffic Policy" onClose={() => setShowCreatePolicy(false)}>
          <div className="grid gap-4">
            <Input label="Name" value={policyForm.name} onChange={(v) => setPolicyForm({ ...policyForm, name: v })} placeholder="Rate limit API" />
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Type</label>
              <select
                className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={policyForm.type}
                onChange={(e) => setPolicyForm({ ...policyForm, type: e.target.value as TrafficPolicy["type"] })}
              >
                <option value="rate_limit">Rate Limit</option>
                <option value="ip_whitelist">IP Whitelist</option>
                <option value="ip_blacklist">IP Blacklist</option>
                <option value="circuit_breaker">Circuit Breaker</option>
              </select>
            </div>
            <Input label="Config (JSON)" value={policyForm.config} onChange={(v) => setPolicyForm({ ...policyForm, config: v })} placeholder='{"requests_per_second": 100}' />
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
              <input type="checkbox" checked={policyForm.enabled} onChange={(e) => setPolicyForm({ ...policyForm, enabled: e.target.checked })} className="rounded border-white/10 bg-[#161b28]" />
              Enabled
            </label>
          </div>
          <ModalFooter
            onCancel={() => setShowCreatePolicy(false)}
            onConfirm={() => createPolicyMutation.mutate()}
            confirmLabel={createPolicyMutation.isPending ? "Creating..." : "Create"}
            disabled={createPolicyMutation.isPending || !policyForm.name}
          />
        </Modal>
      )}
    </div>
  );
}

function RouteFormModal({
  title, form, onChange, onSave, onClose, saving,
}: {
  title: string;
  form: typeof defaultRouteForm;
  onChange: (f: typeof defaultRouteForm) => void;
  onSave: () => void;
  onClose: () => void;
  saving: boolean;
}) {
  return (
    <Modal title={title} onClose={onClose}>
      <div className="grid gap-4">
        <Input label="Path" value={form.path} onChange={(v) => onChange({ ...form, path: v })} placeholder="/api/v1/servers" />
        <Input label="Target Group" value={form.targetGroup} onChange={(v) => onChange({ ...form, targetGroup: v })} placeholder="prod-servers" />
        <Input label="Priority" type="number" value={String(form.priority)} onChange={(v) => onChange({ ...form, priority: Number(v) })} />
        <Input label="HTTP Methods (comma separated)" value={form.methods} onChange={(v) => onChange({ ...form, methods: v })} placeholder="GET,POST,PUT,DELETE" />
        <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
          <input type="checkbox" checked={form.enabled} onChange={(e) => onChange({ ...form, enabled: e.target.checked })} className="rounded border-white/10 bg-[#161b28]" />
          Enabled
        </label>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={onSave} confirmLabel={saving ? "Saving..." : "Save"} disabled={saving || !form.path} />
    </Modal>
  );
}
