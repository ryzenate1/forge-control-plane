"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Globe, Heart, Network, Shield, Trash2, RefreshCw, Server } from "lucide-react";
import {
  fetchDiscoveryEndpoints,
  fetchDiscoveryServices,
  fetchNetworkVisibility,
  fetchNodeNetworkView,
  fetchReaperStats,
  fetchDiscoveryPolicy,
  fetchDiscoveryEndpoint,
  registerDiscoveryEndpoint,
  resolveDiscoveryService,
  deleteDiscoveryEndpoint,
  heartbeatDiscoveryEndpoint,
  updateDiscoveryEndpointStatus,
  sweepReachability,
  verifyReachability,
  allowPolicyPort,
  revokePolicyPort,
  addPrivateCIDR,
  removePrivateCIDR,
  type DiscoveryEndpoint,
  type DiscoveryEndpointStatus,
  type PolicyView,
} from "@/lib/api/discovery";
import { fetchNodes } from "@/lib/api";
import { discoveryStatusTone } from "@/lib/api/status";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AdminSelect, AdminTabs, Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader,
} from "./admin-ui";

type Tab = "endpoints" | "services" | "visibility" | "policy" | "reachability" | "manage";

// STATUS_COLORS used to live here. It was keyed on raw Tailwind colours and read
// `STATUS_COLORS[status] ?? ""` at four call sites — and Pill defaults to the
// neutral chip, so an endpoint health value this table did not know rendered as
// a confident "inactive" rather than as unread. It also drew `unknown` in plain
// slate, indistinguishable from a deliberately idle endpoint. discoveryStatusTone
// keeps unknown in the dashed unknown treatment.

const NETWORK_COLORS: Record<string, string> = {
  public: "bg-cyan-500/10 text-cyan-400",
  private: "bg-purple-500/10 text-purple-400",
  isolated: "bg-slate-500/10 text-slate-400",
};

export function AdminDiscovery() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>("endpoints");
  const [filterService, setFilterService] = useState("");
  const [filterNodeId, setFilterNodeId] = useState("");
  const [healthyOnly, setHealthyOnly] = useState(false);

  const endpointsQuery = useQuery({
    queryKey: ["discovery-endpoints", filterService, filterNodeId, healthyOnly],
    queryFn: () => fetchDiscoveryEndpoints({ service: filterService || undefined, nodeId: filterNodeId || undefined, healthyOnly: healthyOnly || undefined }),
  });
  const servicesQuery = useQuery({ queryKey: ["discovery-services"], queryFn: fetchDiscoveryServices });
  const visibilityQuery = useQuery({ queryKey: ["discovery-visibility"], queryFn: fetchNetworkVisibility });
  const reaperQuery = useQuery({ queryKey: ["discovery-reaper"], queryFn: fetchReaperStats });
  const policyQuery = useQuery({ queryKey: ["discovery-policy"], queryFn: fetchDiscoveryPolicy });

  const endpoints = useMemo(() => endpointsQuery.data ?? [], [endpointsQuery.data]);
  const services = useMemo(() => servicesQuery.data ?? [], [servicesQuery.data]);
  const visibility = visibilityQuery.data;
  const reaper = reaperQuery.data;
  const policy = policyQuery.data;

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Service Discovery"
        sub="Service endpoints, network visibility, beacon liveness and private-network policy. Endpoints self-register via heartbeats and are reaped after 3m TTL."
        action={
          <Btn size="sm" tone="ghost" onClick={() => { qc.invalidateQueries({ queryKey: ["discovery-endpoints"] }); qc.invalidateQueries({ queryKey: ["discovery-visibility"] }); qc.invalidateQueries({ queryKey: ["discovery-reaper"] }); }}>
            <RefreshCw size={14} /> Refresh
          </Btn>
        }
      />
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.015] px-4 py-2 text-xs leading-5 text-slate-400">
        <span className="font-semibold text-slate-300">INFRA</span> · <span className="font-semibold text-slate-200">Networking Advanced</span> — Discovery is the dynamic resolver behind Endpoints/Domains. For primary networking see <code className="font-mono text-[11px]">/admin/endpoints</code> · <code className="font-mono">/admin/domains</code> · <code className="font-mono">/admin/traffic /load-balancer</code>; advanced also includes <code className="font-mono">Gateways/Cross-Node/DNS/ACME/Certs/Firewall/mTLS</code>.
      </div>

      <div className="rounded-lg border border-amber-500/20 bg-amber-950/10 p-3 text-xs text-amber-200">
        <span className="font-semibold">Beacon liveness:</span> each beacon registers a <code className="rounded bg-white/10 px-1">beacon</code> endpoint on first heartbeat and touches <code>LastHeartbeat</code> every 30s. The stale reaper marks endpoints <span className="font-mono">unhealthy</span> after <code className="rounded bg-white/10 px-1">3m</code> without a touch (draining endpoints skipped). Ensure beacons send <code className="rounded bg-white/10 px-1">endpoints[]</code> liveness in <code className="rounded bg-white/10 px-1">POST /nodes/:id/heartbeat</code>.
        {/* A failed reaper query used to render "loading…" indefinitely, which
            tells the operator to wait for something that is never coming. */}
        <span className="ml-2">Reaper: {reaper
          ? `${reaper.count} reaped, interval ${reaper.interval / 1e9}s, lastRun ${reaper.lastRun ?? "never"}`
          : reaperQuery.isError
            ? `unavailable — ${(reaperQuery.error as Error)?.message ?? "request failed"}`
            : "loading…"}</span>
      </div>

      <AdminTabs tabs={[
        { id: "endpoints", label: "Endpoints" },
        { id: "services", label: "Services" },
        { id: "visibility", label: "Network Visibility" },
        { id: "policy", label: "Private Network Policy" },
        { id: "reachability", label: "Reachability" },
        { id: "manage", label: "Manage" },
      ]} active={tab} onChange={(id) => setTab(id as Tab)} />

      {tab === "endpoints" && (
        <Card>
          <CardHeader title="Endpoints" icon={Server} action={<span className="text-xs text-slate-400">{endpoints.length} total</span>} />
          <div className="flex flex-wrap gap-3 p-4 border-b border-white/[0.06]">
            <Input label="Service filter" value={filterService} onChange={setFilterService} placeholder="e.g. beacon, nginx" />
            <Input label="Node filter" value={filterNodeId} onChange={setFilterNodeId} placeholder="node ID" />
            <label className="flex items-center gap-2 text-xs text-slate-300">
              <input type="checkbox" checked={healthyOnly} onChange={e => setHealthyOnly(e.target.checked)} /> healthy only
            </label>
          </div>
          {endpointsQuery.isLoading ? <div className="p-8 text-center text-sm text-slate-300">Loading endpoints…</div>
            : endpointsQuery.isError ? <div className="p-4 text-sm text-red-300">Failed: {endpointsQuery.error instanceof Error ? endpointsQuery.error.message : "error"}</div>
            : endpoints.length === 0 ? <EmptyState icon={Server} title="No endpoints" message="No service endpoints registered. Beacons self-register via heartbeats." />
            : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-white/[0.06] bg-[var(--surface-input)] text-left text-[10px] uppercase tracking-widest text-slate-400">
                      <th className="px-3 py-2">Service</th>
                      <th className="px-3 py-2">Address</th>
                      <th className="px-3 py-2">Node</th>
                      <th className="px-3 py-2">Status</th>
                      <th className="px-3 py-2">LastHeartbeat</th>
                      <th className="px-3 py-2 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {endpoints.map((ep) => (
                      <EndpointRow key={ep.id} ep={ep} />
                    ))}
                  </tbody>
                </table>
              </div>
            )}
        </Card>
      )}

      {tab === "services" && (
        <Card>
          <CardHeader title="Services" icon={Globe} />
          {servicesQuery.isLoading ? <div className="p-8 text-center text-sm text-slate-300">Loading…</div>
            : services.length === 0 ? <EmptyState icon={Globe} title="No services" message="No service sets discovered." />
            : (
              <div className="space-y-3 p-4">
                {services.map((svc) => (
                  <div key={svc.serviceName + "/" + (svc.tenantId ?? "")} className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
                    <div className="flex items-center justify-between">
                      <span className="font-mono text-sm text-white">{svc.serviceName}</span>
                      <span className="text-xs text-slate-400">{svc.endpoints.length} endpoints</span>
                    </div>
                    {svc.tenantId && <div className="text-xs text-slate-500">tenant: {svc.tenantId}</div>}
                    <div className="mt-2 overflow-x-auto">
                      <table className="w-full text-xs">
                        <thead><tr className="text-left text-slate-500"><th className="pr-2">Node</th><th className="pr-2">Address</th><th>Status</th><th>Replica</th></tr></thead>
                        <tbody>
                          {svc.endpoints.map(e => (
                            <tr key={e.id} className="border-t border-white/[0.04]">
                              <td className="pr-2 py-1">{e.nodeId}</td>
                              <td className="pr-2 font-mono">{e.address}:{e.port}</td>
                              <td><Pill tone={discoveryStatusTone(e.status)}>{e.status}</Pill></td>
                              <td>{e.replicaIndex}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                ))}
              </div>
            )}
        </Card>
      )}

      {tab === "visibility" && (
        <div className="space-y-4">
          <Card>
            <CardHeader title="Network Visibility" icon={Network} action={<Btn size="sm" tone="ghost" onClick={() => visibilityQuery.refetch()}>Reload</Btn>} />
            {visibilityQuery.isLoading ? <div className="p-8 text-center text-sm">Loading…</div>
              : !visibility ? <div className="p-4 text-sm text-amber-300">No data</div>
              : (
                <div className="p-4 space-y-3">
                  <div className="flex flex-wrap gap-2 text-xs">
                    <Pill>Total: {visibility.totalEndpoints}</Pill>
                    <Pill tone="green">Healthy: {visibility.healthyCount}</Pill>
                    <Pill tone="red">Unhealthy: {visibility.unhealthyCount}</Pill>
                    <Pill>Nodes: {visibility.nodesCount}</Pill>
                    <span className="text-slate-500">lastUpdated: {visibility.lastUpdated}</span>
                  </div>
                  {visibility.services.length === 0 ? <EmptyState icon={Network} title="No services visible" message="Register an endpoint to see visibility." />
                    : visibility.services.map(svc => (
                      <div key={svc.serviceName} className="rounded-lg border border-white/[0.06] p-3">
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-sm text-white">{svc.serviceName}</span>
                          <Pill className={NETWORK_COLORS[svc.access] ?? ""}>{svc.access}</Pill>
                          <span className="text-xs text-slate-400">health {svc.healthyCount}/{svc.endpointCount} — nodes {svc.nodes.join(", ") || "—"}</span>
                        </div>
                        <div className="mt-2 overflow-x-auto">
                          <table className="w-full text-xs">
                            <thead><tr className="text-left text-slate-500"><th>ID</th><th>Node</th><th>Address</th><th>Network</th><th>Status</th><th>LastSeen</th></tr></thead>
                            <tbody>
                              {svc.endpoints.map(ev => (
                                <tr key={ev.id} className="border-t border-white/[0.04]">
                                  <td className="font-mono">{ev.id.slice(0, 8)}…</td>
                                  <td>{ev.nodeId}</td>
                                  <td className="font-mono">{ev.address}:{ev.port} {ev.protocol}</td>
                                  <td><Pill className={NETWORK_COLORS[ev.network] ?? ""}>{ev.network}</Pill></td>
                                  <td><Pill tone={discoveryStatusTone(ev.status)}>{ev.status}</Pill></td>
                                  <td className="text-slate-400">{ev.lastSeen ? new Date(ev.lastSeen).toLocaleString() : "—"}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      </div>
                    ))}
                </div>
              )}
          </Card>
          <NodeViewCard />
        </div>
      )}

      {tab === "policy" && (
        <PolicyCard policy={policy} onRefresh={() => policyQuery.refetch()} />
      )}

      {tab === "reachability" && (
        <ReachabilityCard />
      )}

      {tab === "manage" && (
        <DiscoveryManageCard />
      )}

    </div>
  );
}

function EndpointRow({ ep }: { ep: DiscoveryEndpoint }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const deleteMut = useMutation({
    mutationFn: () => deleteDiscoveryEndpoint(ep.id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["discovery-endpoints"] }); toast({ tone: "success", title: "Endpoint removed" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });
  const hbMut = useMutation({
    mutationFn: () => heartbeatDiscoveryEndpoint(ep.id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["discovery-endpoints"] }); toast({ tone: "success", title: "Heartbeat touched" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Heartbeat failed", message: e.message }),
  });
  const statusMut = useMutation({
    mutationFn: (status: DiscoveryEndpointStatus) => updateDiscoveryEndpointStatus(ep.id, status),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["discovery-endpoints"] }),
    onError: (e: Error) => toast({ tone: "error", title: "Status update failed", message: e.message }),
  });
  const ageMs = Date.now() - new Date(ep.lastHeartbeat).getTime();
  const ageSec = Math.floor(ageMs / 1000);
  const stale = ageSec > 180;
  return (
    <tr className="border-b border-white/[0.04] hover:bg-white/[0.02]">
      <td className="px-3 py-2 font-mono text-xs text-white">{ep.serviceName}{ep.tenantId ? `/${ep.tenantId}` : ""}</td>
      <td className="px-3 py-2 font-mono text-xs">{ep.address}:{ep.port}/{ep.protocol}</td>
      <td className="px-3 py-2 text-xs">{ep.nodeId.slice(0, 8)}<span className="text-slate-500"> {ep.nodeName}</span></td>
      <td className="px-3 py-2"><Pill tone={discoveryStatusTone(ep.status)}>{ep.status}</Pill>{stale && <span className="ml-1 text-[10px] text-amber-300">stale {ageSec}s</span>}</td>
      <td className="px-3 py-2 text-xs text-slate-400">{ep.lastHeartbeat ? new Date(ep.lastHeartbeat).toLocaleString() : "—"}</td>
      <td className="px-3 py-2 text-right space-x-1">
        <Btn size="sm" tone="ghost" disabled={hbMut.isPending} onClick={() => hbMut.mutate()}>Touch</Btn>
        <Btn size="sm" tone="ghost" disabled={statusMut.isPending} onClick={() => statusMut.mutate(ep.status === "healthy" ? "unhealthy" : "healthy")}>Toggle</Btn>
        <Btn size="sm" tone="danger" disabled={deleteMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Delete endpoint ${ep.id.slice(0, 8)}?`, description: `${ep.serviceName} @ ${ep.address}:${ep.port}`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(); })(); }}><Trash2 size={12} /></Btn>
        {renderConfirm()}
      </td>
    </tr>
  );
}

function NodeViewCard() {
  const [nodeId, setNodeId] = useState("");
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const q = useQuery({ queryKey: ["node-view", nodeId], queryFn: () => fetchNodeNetworkView(nodeId), enabled: !!nodeId });
  return (
    <Card>
      <CardHeader title="Node Network View" icon={Activity} />
      <div className="p-4 space-y-3">
        <AdminSelect label="Node" value={nodeId} onChange={setNodeId} placeholder="select node" options={(nodesQuery.data ?? []).map(n => ({ value: n.id, label: n.name }))} />
        {nodeId && (
          q.isLoading ? <div className="text-sm text-slate-300">Loading…</div>
            : q.isError ? <div className="text-sm text-red-300">{q.error instanceof Error ? q.error.message : "error"}</div>
            : q.data ? (
              <div className="text-xs space-y-2">
                <div>Services: {q.data.services.join(", ") || "—"}</div>
                <div>Endpoints: {q.data.endpoints.length}</div>
                {q.data.endpoints.length > 0 && (
                  <table className="w-full text-xs">
                    <thead><tr className="text-left text-slate-500"><th>ID</th><th>Address</th><th>Status</th></tr></thead>
                    <tbody>{q.data.endpoints.map(e => <tr key={e.id} className="border-t border-white/[0.04]"><td className="font-mono">{e.id.slice(0, 8)}…</td><td className="font-mono">{e.address}:{e.port}</td><td><Pill tone={discoveryStatusTone(e.status)}>{e.status}</Pill></td></tr>)}</tbody>
                  </table>
                )}
                {q.data.reachability && q.data.reachability.length > 0 && (
                  <div>Reachability: {q.data.reachability.map(r => `${r.serviceName}:${r.reachable ? "ok" : "fail"}`).join(", ")}</div>
                )}
              </div>
            ) : null
        )}
      </div>
    </Card>
  );
}

function PolicyCard({ policy, onRefresh }: { policy: PolicyView | undefined; onRefresh: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [cidr, setCidr] = useState("");
  const [svc, setSvc] = useState("");
  const [port, setPort] = useState("");
  const addCidrMut = useMutation({
    mutationFn: () => addPrivateCIDR(cidr),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["discovery-policy"] }); setCidr(""); toast({ tone: "success", title: "CIDR added" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Add CIDR failed", message: e.message }),
  });
  const rmCidrMut = useMutation({
    mutationFn: (c: string) => removePrivateCIDR(c),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["discovery-policy"] }); toast({ tone: "success", title: "CIDR removed" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Remove CIDR failed", message: e.message }),
  });
  const allowMut = useMutation({
    mutationFn: () => allowPolicyPort(svc, Number(port)),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["discovery-policy"] }); setPort(""); toast({ tone: "success", title: "Port allowed" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Allow failed", message: e.message }),
  });
  const revokeMut = useMutation({
    mutationFn: (args: { svc: string; port: number }) => revokePolicyPort(args.svc, args.port),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["discovery-policy"] }); toast({ tone: "success", title: "Port revoked" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Revoke failed", message: e.message }),
  });
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader title="Private Network Policy" icon={Shield} action={<Btn size="sm" tone="ghost" onClick={onRefresh}>Reload</Btn>} />
        {!policy ? <div className="p-8 text-center text-sm">Loading policy…</div> : (
          <div className="p-4 space-y-4">
            <div className="rounded-lg border border-purple-500/20 bg-purple-950/10 p-3 text-xs text-purple-200">
              Enforcement: endpoint registration checks <code className="bg-white/10 px-1 rounded">private CIDRs</code> to classify <code>public/private/isolated</code> (see <code>visibility</code>). When a service has an allowlist (via <code>allowedPorts</code>), registration is rejected unless <code>port</code> ∈ allowlist. Firewall sync: allowed ports are reconciled to beacon <code>/host/firewall</code> allow-rules (source = private CIDRs, protocol tcp). Remove the allowlist (revoke all) to return to default-allow.
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-widest text-slate-400">Private CIDRs</div>
              <div className="mt-2 flex flex-wrap gap-2">
                {(policy.privateCIDRs ?? []).map(c => (
                  <span key={c} className="inline-flex items-center gap-1 rounded-full bg-purple-500/10 px-2 py-1 text-xs text-purple-300">{c}<button className="ml-1 text-purple-400 hover:text-red-300" onClick={() => rmCidrMut.mutate(c)}>×</button></span>
                ))}
              </div>
              <div className="mt-3 flex gap-2">
                <Input label="Add CIDR" value={cidr} onChange={setCidr} placeholder="10.1.0.0/16" />
                <div className="pt-6"><Btn size="sm" tone="primary" disabled={!cidr || addCidrMut.isPending} onClick={() => addCidrMut.mutate()}>{addCidrMut.isPending ? "…" : "Add"}</Btn></div>
              </div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-widest text-slate-400">Allowed Ports (per-service ACL)</div>
              {Object.keys(policy.allowedPorts ?? {}).length === 0 ? <div className="mt-2 text-xs text-slate-400">No port ACLs — all ports default-allow (no enforcement). Add an entry to enforce allowlist for that service.</div> : (
                <div className="mt-2 space-y-2">
                  {Object.entries(policy.allowedPorts).map(([s, ports]) => (
                    <div key={s} className="flex items-center justify-between rounded border border-white/[0.06] p-2 text-xs">
                      <span className="font-mono text-white">{s}</span>
                      <span className="flex gap-1">{(ports ?? []).map(p => <span key={p} className="rounded bg-white/10 px-1.5 py-0.5">{p} <button className="text-red-300" onClick={() => revokeMut.mutate({ svc: s, port: p })}>×</button></span>)}</span>
                    </div>
                  ))}
                </div>
              )}
              <div className="mt-3 flex gap-2">
                <Input label="Service" value={svc} onChange={setSvc} placeholder="e.g. mysql" />
                <Input label="Port" value={port} onChange={setPort} placeholder="3306" type="number" />
                <div className="pt-6"><Btn size="sm" tone="primary" disabled={!svc || !port || allowMut.isPending} onClick={() => allowMut.mutate()}>{allowMut.isPending ? "…" : "Allow"}</Btn></div>
              </div>
            </div>
            <div className="rounded border border-cyan-500/20 bg-cyan-950/10 p-3 text-xs text-cyan-200">
              Gateway / firewall linkage: when a port is allowed, the panel also ensures a firewall allow-rule exists on each node for that port (TCP, source = private CIDRs ∪ firewall rules). Removing the last allowed port for a service restores firewall default-deny for that port (rule deleted).
            </div>
          </div>
        )}
      </Card>
    </div>
  );
}

function ReachabilityCard() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [source, setSource] = useState("");
  const [target, setTarget] = useState("");
  const [svc, setSvc] = useState("");
  const verifyMut = useMutation({
    mutationFn: () => verifyReachability({ sourceNodeId: source, targetNodeId: target, serviceName: svc }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["reachability"] }),
  });
  const sweepMut = useMutation({
    mutationFn: () => sweepReachability(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["reachability"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to sweep reachability", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  return (
    <Card>
      <CardHeader title="Reachability" icon={Heart} action={<Btn size="sm" tone="primary" onClick={() => sweepMut.mutate()} disabled={sweepMut.isPending}>{sweepMut.isPending ? "Sweeping…" : "Sweep All"}</Btn>} />
      <div className="p-4 space-y-4">
        <p className="text-xs text-slate-400">Wires <code className="font-mono">verifyReachability</code> (<code>POST /admin/service-discovery/reachability/verify</code>) and <code>sweepReachability</code> (<code>POST /admin/service-discovery/reachability/sweep</code>) + reaper stats below.</p>
        <div className="grid grid-cols-3 gap-3">
          <Input label="Source Node ID" value={source} onChange={setSource} placeholder="node-1" />
          <Input label="Target Node ID" value={target} onChange={setTarget} placeholder="node-2" />
          <Input label="Service" value={svc} onChange={setSvc} placeholder="beacon" />
        </div>
        <Btn size="sm" tone="primary" disabled={!source || !target || !svc || verifyMut.isPending} onClick={() => verifyMut.mutate()}>{verifyMut.isPending ? "Verifying…" : "Verify Cross-Node"}</Btn>
        {verifyMut.isSuccess && verifyMut.data && (
          <div className="rounded border border-white/[0.06] p-2 text-xs">Result: {verifyMut.data.reachable ? <span className="text-green-400">reachable</span> : <span className="text-red-400">unreachable {verifyMut.data.error ?? ""}</span>} latency {verifyMut.data.latency ?? "—"}</div>
        )}
        {verifyMut.isError && <div className="text-xs text-red-300">{verifyMut.error instanceof Error ? verifyMut.error.message : "verify failed"}</div>}
        {sweepMut.isSuccess && Array.isArray(sweepMut.data) && sweepMut.data.length > 0 && (
          <div className="rounded border border-white/[0.06] p-2 text-xs space-y-1">
            <div className="font-semibold">Sweep results ({sweepMut.data.length})</div>
            {sweepMut.data.slice(0, 20).map((r, i) => <div key={i} className={r.reachable ? "text-green-300" : "text-red-300"}>{r.serviceName} {r.sourceNodeId}→{r.targetNodeId}: {r.reachable ? "ok" : r.error}</div>)}
          </div>
        )}
      </div>
    </Card>
  );
}

function DiscoveryManageCard() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const [regService, setRegService] = useState("my-service");
  const [regNode, setRegNode] = useState("");
  const [regAddr, setRegAddr] = useState("10.0.0.1");
  const [regPort, setRegPort] = useState("8080");
  const [fetchId, setFetchId] = useState("");
  const [resolveService, setResolveService] = useState("beacon");
  const [resolveTenant, setResolveTenant] = useState("");
  const reaperQuery = useQuery({ queryKey: ["discovery-reaper"], queryFn: fetchReaperStats });

  const registerMut = useMutation({
    mutationFn: () => registerDiscoveryEndpoint({ serviceName: regService, nodeId: regNode, address: regAddr, port: Number(regPort) }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["discovery-endpoints"] }); toast({ tone: "success", title: "Endpoint registered" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Register failed", message: e.message }),
  });
  const fetchOneMut = useMutation({
    mutationFn: () => fetchDiscoveryEndpoint(fetchId),
    onSuccess: (data) => toast({ tone: "success", title: `Fetched ${data.serviceName}@${data.address}:${data.port}` }),
    onError: (e: Error) => toast({ tone: "error", title: "Fetch failed", message: e.message }),
  });
  const resolveMut = useMutation({
    mutationFn: () => resolveDiscoveryService(resolveService, resolveTenant || undefined),
    onSuccess: (data) => toast({ tone: "success", title: `Resolved ${data.length} endpoints` }),
    onError: (e: Error) => toast({ tone: "error", title: "Resolve failed", message: e.message }),
  });

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader title="Discovery Operations — wires orphaned manage funcs" icon={Activity} />
        <div className="p-4 space-y-6">
          <div className="rounded border border-amber-500/20 bg-amber-950/10 p-3 text-xs text-amber-200">
            Wires <code className="font-mono">registerDiscoveryEndpoint</code> (<code>POST /admin/service-discovery/endpoints</code>), <code>fetchDiscoveryEndpoint</code> (<code>GET .../endpoints/:id</code>), <code>resolveDiscoveryService</code> (<code>GET .../resolve?service=</code>), plus reaper/heartbeat/sweep already in other tabs.
          </div>
          <div>
            <h4 className="text-xs font-semibold uppercase tracking-widest text-slate-400 mb-2">Register Endpoint</h4>
            <div className="grid grid-cols-2 gap-3">
              <Input label="Service Name" value={regService} onChange={setRegService} placeholder="my-service" />
              <AdminSelect label="Node" value={regNode} onChange={setRegNode} placeholder="select node" options={(nodesQuery.data ?? []).map(n => ({ value: n.id, label: n.name }))} />
              <Input label="Address" value={regAddr} onChange={setRegAddr} placeholder="10.0.0.1" />
              <Input label="Port" value={regPort} onChange={setRegPort} placeholder="8080" type="number" />
            </div>
            <Btn size="sm" tone="primary" disabled={!regService || !regNode || !regAddr || !regPort || registerMut.isPending} onClick={() => registerMut.mutate()} className="mt-2">{registerMut.isPending ? "Registering…" : "Register"}</Btn>
          </div>
          <div>
            <h4 className="text-xs font-semibold uppercase tracking-widest text-slate-400 mb-2">Fetch Endpoint by ID — wires fetchDiscoveryEndpoint</h4>
            <div className="flex gap-2">
              <Input label="Endpoint ID" value={fetchId} onChange={setFetchId} placeholder="uuid" />
              <div className="pt-6"><Btn size="sm" tone="primary" disabled={!fetchId || fetchOneMut.isPending} onClick={() => fetchOneMut.mutate()}>{fetchOneMut.isPending ? "…" : "Fetch"}</Btn></div>
            </div>
            {fetchOneMut.isSuccess && fetchOneMut.data && (
              <pre className="mt-2 rounded bg-black/30 p-3 text-xs text-slate-300 overflow-auto">{JSON.stringify(fetchOneMut.data, null, 2)}</pre>
            )}
          </div>
          <div>
            <h4 className="text-xs font-semibold uppercase tracking-widest text-slate-400 mb-2">Resolve Service — wires resolveDiscoveryService</h4>
            <div className="flex gap-2">
              <Input label="Service" value={resolveService} onChange={setResolveService} placeholder="beacon" />
              <Input label="Tenant (optional)" value={resolveTenant} onChange={setResolveTenant} placeholder="tenant-id" />
              <div className="pt-6"><Btn size="sm" tone="primary" disabled={!resolveService || resolveMut.isPending} onClick={() => resolveMut.mutate()}>{resolveMut.isPending ? "…" : "Resolve"}</Btn></div>
            </div>
            {resolveMut.isSuccess && Array.isArray(resolveMut.data) && (
              <div className="mt-2 text-xs">Found {resolveMut.data.length} endpoint(s): <span className="font-mono">{resolveMut.data.map(r => `${r.address}:${r.port}`).join(", ") || "none"}</span></div>
            )}
          </div>
          <div>
            <h4 className="text-xs font-semibold uppercase tracking-widest text-slate-400 mb-2">Reaper Stats — wires fetchReaperStats</h4>
            {reaperQuery.isLoading ? <div className="text-xs text-slate-400">Loading…</div>
              : reaperQuery.isError ? <div className="text-xs text-red-300">{reaperQuery.error instanceof Error ? reaperQuery.error.message : "error"}</div>
              : reaperQuery.data ? <div className="text-xs font-mono text-slate-300">count={reaperQuery.data.count} interval={reaperQuery.data.interval/1e9}s lastRun={reaperQuery.data.lastRun ?? "never"}</div>
              : <div className="text-xs text-slate-400">No data</div>}
            <Btn size="sm" tone="ghost" onClick={() => reaperQuery.refetch()} className="mt-2">Refresh Reaper</Btn>
          </div>
        </div>
      </Card>
    </div>
  );
}
