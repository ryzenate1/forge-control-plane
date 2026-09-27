"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Globe, RefreshCw, Server, Trash2, Waypoints, Zap, Shield, Network } from "lucide-react";
import {
  fetchCrossNodeHealth,
  resolveCrossNodeTarget,
  clearCrossNodeCache,
  setCrossNodeCacheTTL,
  describeCrossNodeHost,
  fetchCrossNodeIngressRules,
  fetchCrossNodeIngressPolicies,
  triggerCrossNodeIngressSync,
  fetchCrossNodeIngressHealthStats,
  cleanupCrossNodeIngressStale,
  fetchCrossNodeIngressStats,
} from "@/lib/api/crossnode";
import { useToast } from "@/components/ui/toast";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageLayout,
  SectionHeader,
  Card,
  CardHeader,
  Btn,
  Pill,
  AdminLoadingState,
  AdminErrorState,
  AdminTabs,
  Input,
} from "./admin-ui";

type Tab = "health" | "resolver" | "ingress";

export function AdminCrossnode() {
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>("health");

  const healthQ = useQuery({
    queryKey: ["admin-crossnode-health"],
    queryFn: fetchCrossNodeHealth,
    refetchInterval: 30_000,
    retry: false,
  });

  const tabs: Array<{ id: string; label: string }> = [
    { id: "health", label: "Health" },
    { id: "resolver", label: "Resolver & Cache" },
    { id: "ingress", label: "Ingress Sync" },
  ];

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { void healthQ.refetch(); qc.invalidateQueries({ queryKey: ["admin-crossnode"] }); }} />
      <SectionHeader
        title="Cross-Node"
        sub="Cross-node resolver cache and ingress sync controls — 11 routes from handlers_crossnode.go:12. Health, resolve, cache, ingress sync and cleanup."
        action={
          <Btn size="sm" tone="ghost" onClick={() => { void healthQ.refetch(); qc.invalidateQueries({ queryKey: ["admin-crossnode"] }); }}>
            <RefreshCw size={14} /> Refresh
          </Btn>
        }
      />

      <AdminTabs tabs={tabs} active={tab} onChange={(id) => setTab(id as Tab)} />

      {tab === "health" && <HealthPanel healthQ={healthQ} />}
      {tab === "resolver" && <ResolverPanel />}
      {tab === "ingress" && <IngressPanel />}
    </AdminPageLayout>
  );
}

function HealthPanel({ healthQ }: { healthQ: ReturnType<typeof useQuery> }) {
  const h = healthQ.data as import("@/lib/api/crossnode").CrossNodeHealth | undefined;
  return (
    <div className="grid gap-6">
      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title="Health — GET /admin/crossnode/health" icon={Activity} />
        <div className="p-4">
          {healthQ.isLoading ? (
            <AdminLoadingState label="Loading cross-node health…" />
          ) : healthQ.isError ? (
            <AdminErrorState message={(healthQ.error as Error).message} retry={() => void healthQ.refetch()} />
          ) : (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <div className="rounded-xl border border-[var(--line)] bg-[var(--canvas)] p-4">
                <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Resolver</div>
                <div className="mt-2 flex items-center gap-2">
                  <span className={`h-2 w-2 rounded-full ${h?.resolver_available ? "bg-emerald-500" : "bg-red-500"}`} />
                  <span className="text-sm font-semibold text-[var(--text)]">{h?.resolver_available ? "Available" : "Unavailable"}</span>
                </div>
                <div className="mt-1 text-xs text-[var(--text-subtle)]">{h?.resolver_status ?? "—"}</div>
                <Pill tone={h?.resolver_available ? "green" : "red"} className="mt-2">{h?.resolver_available ? "active" : "inactive"}</Pill>
              </div>
              <div className="rounded-xl border border-[var(--line)] bg-[var(--canvas)] p-4">
                <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Ingress Sync</div>
                <div className="mt-2 flex items-center gap-2">
                  <span className={`h-2 w-2 rounded-full ${h?.ingress_sync_available ? "bg-emerald-500" : "bg-red-500"}`} />
                  <span className="text-sm font-semibold text-[var(--text)]">{h?.ingress_sync_available ? "Available" : "Unavailable"}</span>
                </div>
                <div className="mt-1 text-xs text-[var(--text-subtle)]">{h?.ingress_sync_status ?? "—"}</div>
                <Pill tone={h?.ingress_sync_available ? "green" : "yellow"} className="mt-2">{h?.ingress_sync_available ? "active" : "inactive"}</Pill>
              </div>
              <div className="rounded-xl border border-[var(--line)] bg-[var(--canvas)] p-4 sm:col-span-2">
                <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Capabilities (handlers_crossnode.go)</div>
                <p className="mt-2 text-xs leading-5 text-[var(--text-subtle)]">
                  Resolver: <code className="rounded bg-white/[0.06] px-1 font-mono text-[11px]">GET /resolve</code> ·{" "}
                  <code className="font-mono text-[11px]">POST /cache/clear</code> ·{" "}
                  <code className="font-mono text-[11px]">POST /cache/ttl</code> ·{" "}
                  <code className="font-mono text-[11px]">GET /describe/:host/:port</code>
                </p>
                <p className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">
                  Ingress: <code className="font-mono text-[11px]">GET /ingress/rules</code> ·{" "}
                  <code className="font-mono text-[11px]">GET /ingress/policies</code> ·{" "}
                  <code className="font-mono text-[11px]">POST /ingress/sync</code> ·{" "}
                  <code className="font-mono text-[11px]">POST /ingress/cleanup</code>
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Btn size="sm" tone="ghost" onClick={() => void healthQ.refetch()}>
                    <RefreshCw size={12} /> Refresh health
                  </Btn>
                </div>
              </div>
            </div>
          )}
        </div>
      </Card>

      <Card className="border border-[var(--line)] bg-[var(--surface)] p-4">
        <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]">
          <Waypoints size={14} className="text-[var(--brand)]" /> How cross-node works
        </div>
        <p className="mt-2 text-sm leading-6 text-[var(--text-subtle)]">
          The <span className="font-medium text-[var(--text)]">Resolver</span> maps <code className="font-mono text-xs">server_id / node_id</code> → target host (used by the load-balancer and traffic manager to forward to the correct beacon).{" "}
          <span className="font-medium text-[var(--text)]">Ingress Sync</span> reconciles desired routing rules/policies to the edge (Traefik/Caddy) and cleans stale routes. Health reflects whether the panel was booted with those services (<code className="font-mono text-xs">resolver != nil</code>, <code className="font-mono text-xs">ingressSync != nil</code>).
        </p>
      </Card>
    </div>
  );
}

function ResolverPanel() {
  const { toast } = useToast();
  const [serverId, setServerId] = useState("");
  const [nodeId, setNodeId] = useState("");
  const [ttl, setTtl] = useState("30s");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");

  const resolveMut = useMutation({
    mutationFn: () => resolveCrossNodeTarget({ serverId: serverId.trim() || undefined, nodeId: nodeId.trim() || undefined }),
    onSuccess: (data) => toast({ tone: "success", title: `Resolved → ${data.host}` }),
    onError: (e: Error) => toast({ tone: "error", title: "Resolve failed", message: e.message }),
  });

  const clearMut = useMutation({
    mutationFn: clearCrossNodeCache,
    onSuccess: (data) => toast({ tone: "success", title: data.message ?? "Cache cleared" }),
    onError: (e: Error) => toast({ tone: "error", title: "Clear failed", message: e.message }),
  });

  const ttlMut = useMutation({
    mutationFn: () => setCrossNodeCacheTTL(ttl.trim()),
    onSuccess: (data) => toast({ tone: "success", title: data.message ?? "TTL set" }),
    onError: (e: Error) => toast({ tone: "error", title: "TTL failed", message: e.message }),
  });

  const describeMut = useMutation({
    mutationFn: () => describeCrossNodeHost(host.trim(), Number(port)),
    onSuccess: (data) => toast({ tone: "success", title: data.description.slice(0, 80) }),
    onError: (e: Error) => toast({ tone: "error", title: "Describe failed", message: e.message }),
  });

  return (
    <div className="grid gap-6">
      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title="Resolve — GET /admin/crossnode/resolve?server_id=&node_id=" icon={Globe} />
        <div className="space-y-4 p-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <Input label="server_id" value={serverId} onChange={setServerId} placeholder="e.g. 550e8400-..." mono />
            <Input label="node_id" value={nodeId} onChange={setNodeId} placeholder="e.g. 123e4567-..." mono />
          </div>
          <div className="flex flex-wrap gap-2">
            <Btn tone="primary" size="sm" disabled={!serverId.trim() && !nodeId.trim()} loading={resolveMut.isPending} onClick={() => resolveMut.mutate()}>
              <Zap size={12} /> Resolve
            </Btn>
            <Btn tone="ghost" size="sm" onClick={() => { setServerId(""); setNodeId(""); }}>Clear</Btn>
          </div>
          {resolveMut.isSuccess && (
            <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-3 font-mono text-xs text-emerald-200">
              host: <span className="font-semibold text-emerald-100">{(resolveMut.data as { host: string }).host || "—"}</span>
            </div>
          )}
          {resolveMut.isError && <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200">{(resolveMut.error as Error).message}</div>}
          <p className="text-xs leading-5 text-[var(--text-subtle)]">
            Requires <code className="font-mono text-[11px]">server_id</code> or <code className="font-mono text-[11px]">node_id</code> query — both missing returns <code className="font-mono">400</code>.
          </p>
        </div>
      </Card>

      <div className="grid gap-6 md:grid-cols-2">
        <Card className="border border-[var(--line)] bg-[var(--surface)]">
          <CardHeader title="Cache — POST /admin/crossnode/cache/clear" icon={Trash2} />
          <div className="space-y-3 p-4">
            <p className="text-sm leading-6 text-[var(--text-subtle)]">Clears the in-memory resolver cache. Requires <code className="font-mono text-xs">routing.write</code>.</p>
            <Btn tone="danger" size="sm" loading={clearMut.isPending} onClick={() => clearMut.mutate()}>
              <Trash2 size={12} /> Clear cache
            </Btn>
            {clearMut.isSuccess && <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-2 text-xs text-emerald-200">{clearMut.data.message}</div>}
            {clearMut.isError && <div className="rounded-lg border border-red-500/20 bg-red-500/10 p-2 text-xs text-red-200">{(clearMut.error as Error).message}</div>}
          </div>
        </Card>
        <Card className="border border-[var(--line)] bg-[var(--surface)]">
          <CardHeader title="Cache TTL — POST /admin/crossnode/cache/ttl" icon={Server} />
          <div className="space-y-3 p-4">
            <Input label="TTL (e.g. 30s, 5m, 1h)" value={ttl} onChange={setTtl} placeholder="30s" mono />
            <Btn tone="primary" size="sm" loading={ttlMut.isPending} onClick={() => ttlMut.mutate()}>
              Set TTL
            </Btn>
            {ttlMut.isSuccess && <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-2 text-xs text-emerald-200">{ttlMut.data.message}</div>}
            {ttlMut.isError && <div className="rounded-lg border border-red-500/20 bg-red-500/10 p-2 text-xs text-red-200">{(ttlMut.error as Error).message}</div>}
            <p className="text-xs text-[var(--text-subtle)]">Note: current backend is a stub — it parses but does not persist TTL. Wired for future impl.</p>
          </div>
        </Card>
      </div>

      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title="Describe unreachable — GET /admin/crossnode/describe/:host/:port" icon={Shield} />
        <div className="space-y-3 p-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <Input label="host" value={host} onChange={setHost} placeholder="e.g. 10.0.0.5" mono />
            <Input label="port" value={port} onChange={setPort} placeholder="9090" mono />
          </div>
          <Btn tone="ghost" size="sm" disabled={!host.trim() || !port.trim()} loading={describeMut.isPending} onClick={() => describeMut.mutate()}>
            Describe
          </Btn>
          {describeMut.isSuccess && (
            <pre className="overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
              {JSON.stringify(describeMut.data, null, 2)}
            </pre>
          )}
          {describeMut.isError && <div className="rounded-lg border border-red-500/20 bg-red-500/10 p-3 text-sm text-red-200">{(describeMut.error as Error).message}</div>}
        </div>
      </Card>
    </div>
  );
}

function IngressPanel() {
  const { toast } = useToast();
  const qc = useQueryClient();

  const rulesQ = useQuery({ queryKey: ["admin-crossnode-ingress-rules"], queryFn: fetchCrossNodeIngressRules, retry: false });
  const policiesQ = useQuery({ queryKey: ["admin-crossnode-ingress-policies"], queryFn: fetchCrossNodeIngressPolicies, retry: false });
  const healthStatsQ = useQuery({ queryKey: ["admin-crossnode-ingress-health-stats"], queryFn: fetchCrossNodeIngressHealthStats, retry: false });
  const statsQ = useQuery({ queryKey: ["admin-crossnode-ingress-stats"], queryFn: fetchCrossNodeIngressStats, retry: false });

  const syncMut = useMutation({
    mutationFn: triggerCrossNodeIngressSync,
    onSuccess: (data) => {
      toast({ tone: "success", title: data.message ?? "Sync triggered" });
      void qc.invalidateQueries({ queryKey: ["admin-crossnode-ingress"] });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Sync failed", message: e.message }),
  });

  const cleanupMut = useMutation({
    mutationFn: cleanupCrossNodeIngressStale,
    onSuccess: (data) => {
      toast({ tone: "success", title: data.message ?? "Stale routes cleaned" });
      void qc.invalidateQueries({ queryKey: ["admin-crossnode-ingress"] });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Cleanup failed", message: e.message }),
  });

  return (
    <div className="grid gap-6">
      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader
          title="Ingress Sync Controls"
          icon={Network}
          action={
            <div className="flex gap-2">
              <Btn size="sm" tone="primary" loading={syncMut.isPending} onClick={() => syncMut.mutate()}>
                <Zap size={12} /> POST /ingress/sync
              </Btn>
              <Btn size="sm" tone="ghost" loading={cleanupMut.isPending} onClick={() => cleanupMut.mutate()}>
                <Trash2 size={12} /> POST /ingress/cleanup
              </Btn>
            </div>
          }
        />
        <div className="grid gap-4 p-4 md:grid-cols-2">
          <div className="space-y-3">
            <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Rules — GET /ingress/rules</div>
            {rulesQ.isLoading ? <AdminLoadingState label="Loading rules…" /> : rulesQ.isError ? <AdminErrorState message={(rulesQ.error as Error).message} retry={() => void rulesQ.refetch()} /> : (
              <pre className="max-h-48 overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs leading-5 text-[var(--text-subtle)]">{JSON.stringify(rulesQ.data, null, 2)}</pre>
            )}
          </div>
          <div className="space-y-3">
            <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Policies — GET /ingress/policies</div>
            {policiesQ.isLoading ? <AdminLoadingState label="Loading policies…" /> : policiesQ.isError ? <AdminErrorState message={(policiesQ.error as Error).message} retry={() => void policiesQ.refetch()} /> : (
              <pre className="max-h-48 overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs leading-5 text-[var(--text-subtle)]">{JSON.stringify(policiesQ.data, null, 2)}</pre>
            )}
          </div>
        </div>
        <div className="border-t border-[var(--line)] p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Health filter stats — GET /ingress/health/stats</div>
              {healthStatsQ.isLoading ? <div className="mt-2 text-xs text-[var(--text-subtle)]">Loading…</div> : healthStatsQ.isError ? <div className="mt-2 text-xs text-red-300">{(healthStatsQ.error as Error).message}</div> : (
                <pre className="mt-2 max-h-32 overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-2 text-xs leading-5 text-[var(--text-subtle)]">{JSON.stringify(healthStatsQ.data, null, 2)}</pre>
              )}
            </div>
            <div>
              <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Sync stats — GET /ingress/stats</div>
              {statsQ.isLoading ? <div className="mt-2 text-xs text-[var(--text-subtle)]">Loading…</div> : statsQ.isError ? <div className="mt-2 text-xs text-red-300">{(statsQ.error as Error).message}</div> : (
                <pre className="mt-2 max-h-32 overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-2 text-xs leading-5 text-[var(--text-subtle)]">{JSON.stringify(statsQ.data, null, 2)}</pre>
              )}
            </div>
          </div>
        </div>
      </Card>

      <Card className="border border-[var(--line)] bg-[var(--surface)] p-4">
        <p className="text-sm leading-6 text-[var(--text-subtle)]">
          Ingress sync is <span className="font-medium text-[var(--text)]">leader-only</span> and reconciles desired proxy state. <code className="rounded bg-white/[0.06] px-1 font-mono text-[11px]">POST /ingress/sync</code> triggers an immediate pass (normally on a ticker);{" "}
          <code className="font-mono text-[11px]">POST /ingress/cleanup</code> removes stale routes. All routes require <code className="font-mono text-[11px]">admin : routing.write</code> and are rate-limited by the mutation limiter.
        </p>
      </Card>
    </div>
  );
}
