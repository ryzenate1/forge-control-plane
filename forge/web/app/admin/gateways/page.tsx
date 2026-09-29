"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Globe,
  Shield,
  Network,
  Router,
  Route,
  Server,
  ArrowRight,
  Layers,
  Box,
  Activity,
  Lock,
  Zap,
  ShieldCheck,
  ShieldOff,
  SlidersHorizontal,
  GitBranch,
  Container,
} from "lucide-react";
import { fetchJSON } from "@/lib/api";
import {
  AdminPageLayout,
  AdminTabs,
  type AdminTab,
  AdminTable,
  AdminTHead,
  AdminTh,
  AdminTBody,
  AdminTr,
  AdminTd,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  SectionHeader,
  AdminLoadingState,
  AdminErrorState,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";

type GatewayTab = "routers" | "services" | "middlewares" | "certs";

type RoutingRule = {
  id: string;
  domain: string;
  path: string;
  targetHost?: string;
  targetPort: number;
  protocol?: string;
  strategy?: string;
  enabled: boolean;
  createdAt: string;
};

type TargetGroup = {
  id: string;
  name: string;
  algorithm: string;
  port: number;
  protocol: string;
  targets: Array<{ id: string; ip: string; port: number; status: string; weight: number }>;
};

type Certificate = {
  id: string;
  domains: string[];
  issuer: string;
  provider: string;
  expiresAt: string;
  autoRenew: boolean;
};

type ProxyDomain = {
  id: string;
  hostname: string;
  serviceId?: string;
  https?: boolean;
};

// Industrial Terminal graph — routers → services arrows
function matchServiceName(domain: string | undefined, services: Array<{ name: string }>): string {
  const needle = (domain || "").split(".")[0]?.trim().toLowerCase();
  if (!needle) return "—";
  return services.find((s) => s.name.toLowerCase() === needle)?.name
    ?? services.find((s) => s.name.toLowerCase().includes(needle))?.name
    ?? "—";
}

function GatewayTopology({ routers, services }: { routers: RoutingRule[]; services: TargetGroup[] }) {
  const routerCount = routers.length;
  const serviceCount = services.length;
  const targetCount = services.reduce((a, s) => a + (s.targets?.length ?? 0), 0);

  return (
    <Card className="overflow-hidden">
      <CardHeader title="Gateway Topology" icon={GitBranch} />
      <div className="p-5">
        {/* ASCII / terminal header — Industrial Terminal */}
        <div className="mb-4 flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2 font-mono text-[11px] text-[var(--text-subtle)]">
          <span className="h-2 w-2 rounded-full bg-[var(--success)] shadow-[0_0_8px_rgba(5,150,105,0.4)]" aria-hidden />
          <span className="text-[var(--text)]">forge</span>
          <span className="text-[color-mix(in_srgb,var(--text-subtle)_60%,transparent)]">::</span>
          <span className="text-[var(--brand)]">gateway</span>
          <span className="text-[var(--text-subtle)]">— routers → services → targets</span>
          <span className="ml-auto hidden sm:inline text-[10px] uppercase tracking-widest text-[var(--text-subtle)]">
            Industrial Terminal · var(--brand) · var(--canvas)
          </span>
        </div>

        {/* Visual lanes */}
        <div className="grid gap-4 md:grid-cols-[1fr_auto_1fr_auto_1fr]">
          {/* Routers lane */}
          <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
            <div className="mb-3 flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">
              <Route size={12} className="text-[var(--brand)]" />
              Routers
              <Pill tone="neutral" className="ml-auto font-mono text-[10px]">
                {routerCount}
              </Pill>
            </div>
            {routerCount === 0 ? (
              <p className="rounded-lg border border-dashed border-[var(--line)] bg-[var(--surface-raised)] px-3 py-6 text-center text-xs text-[var(--text-subtle)]">
                No routers
              </p>
            ) : (
              <div className="space-y-2">
                {routers.slice(0, 5).map((r) => (
                  <div
                    key={r.id}
                    className="flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2"
                  >
                    <Globe size={12} className="shrink-0 text-[var(--text-subtle)]" aria-hidden />
                    <span className="truncate font-mono text-xs font-medium text-[var(--text)]">
                      {r.domain}
                    </span>
                    <span className="shrink-0 font-mono text-[11px] text-[var(--text-subtle)]">{r.path}</span>
                    <span
                      className={`ml-auto h-1.5 w-1.5 shrink-0 rounded-full ${r.enabled ? "bg-[var(--success)]" : "bg-[color-mix(in_srgb,var(--text-subtle)_40%,transparent)]"}`}
                      aria-hidden
                    />
                  </div>
                ))}
                {routerCount > 5 && (
                  <p className="text-center font-mono text-[11px] text-[var(--text-subtle)]">+{routerCount - 5} more</p>
                )}
              </div>
            )}
          </div>

          {/* arrow routers → services */}
          <div className="hidden place-items-center md:grid">
            <div className="flex flex-col items-center gap-1 text-[var(--text-subtle)]">
              <div className="h-px w-12 bg-[var(--line)]" />
              <ArrowRight size={14} className="text-[var(--brand)]" aria-hidden />
              <div className="h-px w-12 bg-[var(--line)]" />
              <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--text-subtle)]">match</span>
            </div>
          </div>

          {/* Services lane */}
          <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
            <div className="mb-3 flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">
              <Layers size={12} className="text-blue-400" aria-hidden />
              Services
              <Pill tone="blue" className="ml-auto font-mono text-[10px] tracking-wide">
                {serviceCount}
              </Pill>
            </div>
            {serviceCount === 0 ? (
              <p className="rounded-lg border border-dashed border-[var(--line)] bg-[var(--surface-raised)] px-3 py-6 text-center text-xs text-[var(--text-subtle)]">
                No services
              </p>
            ) : (
              <div className="space-y-2">
                {services.slice(0, 5).map((s) => (
                  <div
                    key={s.id}
                    className="flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2"
                  >
                    <Server size={12} className="shrink-0 text-blue-300" aria-hidden />
                    <span className="truncate text-xs font-medium text-[var(--text)]">{s.name}</span>
                    <Pill tone="neutral" className="ml-auto font-mono text-[10px] capitalize tracking-wide">
                      {s.algorithm.replaceAll("_", " ")}
                    </Pill>
                  </div>
                ))}
                {serviceCount > 5 && (
                  <p className="text-center font-mono text-[11px] text-[var(--text-subtle)]">+{serviceCount - 5} more</p>
                )}
              </div>
            )}
          </div>

          {/* arrow services → targets */}
          <div className="hidden place-items-center md:grid">
            <div className="flex flex-col items-center gap-1 text-[var(--text-subtle)]">
              <div className="h-px w-12 bg-[var(--line)]" />
              <ArrowRight size={14} className="text-emerald-400" aria-hidden />
              <div className="h-px w-12 bg-[var(--line)]" />
              <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--text-subtle)]">LB</span>
            </div>
          </div>

          {/* Targets lane */}
          <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
            <div className="mb-3 flex items-center gap-2 text-[10px] font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">
              <Container size={12} className="text-emerald-400" aria-hidden />
              Targets
              <Pill tone="green" className="ml-auto font-mono text-[10px] tracking-wide">
                {targetCount}
              </Pill>
            </div>
            <div className="grid grid-cols-3 gap-2 text-center">
              <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-2 py-3">
                <div className="font-mono text-lg font-bold text-emerald-400">
                  {services.flatMap((s) => s.targets ?? []).filter((t) => t.status === "healthy").length}
                </div>
                <div className="text-[10px] uppercase tracking-widest text-emerald-300/80">healthy</div>
              </div>
              <div className="rounded-lg border border-amber-500/20 bg-amber-500/10 px-2 py-3">
                <div className="font-mono text-lg font-bold text-amber-400">
                  {services.flatMap((s) => s.targets ?? []).filter((t) => t.status === "draining").length}
                </div>
                <div className="text-[10px] uppercase tracking-widest text-amber-300/80">draining</div>
              </div>
              <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-2 py-3">
                <div className="font-mono text-lg font-bold text-red-400">
                  {services.flatMap((s) => s.targets ?? []).filter((t) => t.status === "unhealthy").length}
                </div>
                <div className="text-[10px] uppercase tracking-widest text-red-300/80">unhealthy</div>
              </div>
            </div>
            <p className="mt-3 text-center font-mono text-[11px] leading-5 text-[var(--text-subtle)]">
              via <span className="text-[var(--text)]">/admin/load-balancer</span> · weight &amp; least-conn handled at service
            </p>
          </div>
        </div>

        {/* bottom rule */}
        <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-[var(--line)] pt-3 font-mono text-[11px] text-[var(--text-subtle)]">
          <span className="inline-flex items-center gap-1.5">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--brand)]" aria-hidden /> Caddy · Traefik
          </span>
          <span className="text-[color-mix(in_srgb,var(--text-subtle)_60%,transparent)]">·</span>
          <span>5 writers collapsed → 1 reconciler (desired-state)</span>
          <span className="ml-auto hidden sm:inline text-[color-mix(in_srgb,var(--text-subtle)_70%,transparent)]">tip: Hover router to see its service binding</span>
        </div>
      </div>
    </Card>
  );
}

export default function AdminGatewaysPage() {
  const router = useRouter();
  const [tab, setTab] = useState<GatewayTab>("routers");

  const routersQuery = useQuery({
    queryKey: ["gateways", "routers"],
    queryFn: async () => {
      const res = await fetchJSON<{ data: RoutingRule[] } | RoutingRule[]>("/admin/traffic/rules");
      if (Array.isArray(res)) return res;
      return (res as { data: RoutingRule[] }).data ?? [];
    },
  });

  const servicesQuery = useQuery({
    queryKey: ["gateways", "services"],
    queryFn: async () => {
      const res = await fetchJSON<{ data: TargetGroup[] } | TargetGroup[]>("/admin/load-balancer/groups");
      if (Array.isArray(res)) return res;
      return (res as { data: TargetGroup[] }).data ?? [];
    },
  });

  const domainsQuery = useQuery({
    queryKey: ["gateways", "domains"],
    queryFn: async () => {
      const res = await fetchJSON<{ data: ProxyDomain[] } | ProxyDomain[]>("/domains");
      if (Array.isArray(res)) return res;
      return (res as { data: ProxyDomain[] }).data ?? [];
    },
  });

  const certsQuery = useQuery({
    queryKey: ["gateways", "certs"],
    queryFn: async () => {
      const res = await fetchJSON<{ data: Certificate[] } | Certificate[]>("/certificates");
      if (Array.isArray(res)) return res;
      return (res as { data: Certificate[] }).data ?? [];
    },
  });

  const routers = useMemo(() => routersQuery.data ?? [], [routersQuery.data]);
  const services = useMemo(() => servicesQuery.data ?? [], [servicesQuery.data]);
  const domains = useMemo(() => domainsQuery.data ?? [], [domainsQuery.data]);
  const certs = useMemo(() => certsQuery.data ?? [], [certsQuery.data]);

  const tabs: Array<AdminTab & { id: GatewayTab }> = [
    { id: "routers", label: `Routers · ${routers.length}` },
    { id: "services", label: `Services · ${services.length}` },
    { id: "middlewares", label: "Middlewares" },
    { id: "certs", label: `Certs · ${certs.length}` },
  ];

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <SectionHeader
        title="Gateways"
        sub="Edge gateway routers, services and middlewares."
        action={
          <div className="flex gap-2">
            <Btn tone="ghost" onClick={() => router.push("/admin/traffic")}>
              <Route size={14} /> Traffic
            </Btn>
            <Btn tone="ghost" onClick={() => router.push("/admin/load-balancer")}>
              <Network size={14} /> Load Balancer
            </Btn>
            <Btn tone="ghost" onClick={() => router.push("/admin/domains")}>
              <Globe size={14} /> Domains
            </Btn>
            <Btn tone="primary" onClick={() => router.push("/admin/certificates")}>
              <Lock size={14} /> Certificates
            </Btn>
          </div>
        }
      />

      <GatewayTopology routers={routers} services={services} />

      <div className="flex flex-wrap gap-2">
        <Btn
          size="sm"
          tone="ghost"
          onClick={() => router.push("/admin/firewall")}
          className="gap-1.5"
        >
          <Shield size={12} /> Firewall
        </Btn>
        <Btn
          size="sm"
          tone="ghost"
          onClick={() => router.push("/admin/endpoints")}
          className="gap-1.5"
        >
          <Box size={12} /> Endpoints
        </Btn>
      </div>

      <AdminTabs tabs={tabs} active={tab} onChange={(id) => setTab(id as GatewayTab)} />

      {tab === "routers" && (
        <Card>
          <CardHeader title="Routers" icon={Router} />
          {routersQuery.isLoading ? (
            <AdminLoadingState label="Loading routers…" />
          ) : routersQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={routersQuery.error instanceof Error ? routersQuery.error.message : "Failed to load routers"}
                retry={() => void routersQuery.refetch()}
              />
            </div>
          ) : routers.length === 0 ? (
            <EmptyState
              icon={Route}
              title="No routers"
              message="No routing rules yet. Create a route from Traffic Policies to get started."
            />
          ) : (
            <AdminTable label="Routers">
              <AdminTHead><AdminTh>Host</AdminTh><AdminTh>Path</AdminTh><AdminTh>Target</AdminTh><AdminTh>Strategy</AdminTh><AdminTh>Service →</AdminTh><AdminTh>Status</AdminTh></AdminTHead>
              <AdminTBody>
                  {routers.map((r) => (
                    <AdminTr key={r.id}>
                      <AdminTd className="font-mono text-xs font-medium">{r.domain || "—"}</AdminTd>
                      <AdminTd className="font-mono text-xs">{r.path}</AdminTd>
                      <AdminTd className="font-mono text-xs">
                        {r.targetHost ? `${r.targetHost}:${r.targetPort}` : `:${r.targetPort}`}
                      </AdminTd>
                      <AdminTd>
                        <Pill tone="neutral" className="font-mono text-[11px] capitalize tracking-wide">
                          {r.strategy || "round_robin"}
                        </Pill>
                      </AdminTd>
                      <AdminTd>
                        <span className="inline-flex items-center gap-1 text-xs text-[var(--text-subtle)]">
                          <ArrowRight size={12} className="text-[var(--brand)]" aria-hidden />
                          {matchServiceName(r.domain, services)}
                        </span>
                      </AdminTd>
                      <AdminTd>
                        <Pill tone={r.enabled ? "green" : "neutral"}>{r.enabled ? "enabled" : "disabled"}</Pill>
                      </AdminTd>
                    </AdminTr>
                  ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      )}

      {tab === "services" && (
        <div className="grid gap-4">
          {servicesQuery.isLoading ? (
            <Card>
              <div className="p-8">
                <AdminLoadingState label="Loading services…" />
              </div>
            </Card>
          ) : servicesQuery.isError ? (
            <Card>
              <div className="p-4">
                <AdminErrorState
                  message={servicesQuery.error instanceof Error ? servicesQuery.error.message : "Failed to load services"}
                  retry={() => void servicesQuery.refetch()}
                />
              </div>
            </Card>
          ) : services.length === 0 ? (
            <Card>
              <EmptyState icon={Layers} title="No services" message="No target groups yet. Create one in Load Balancer, then bind routers to it." />
            </Card>
          ) : (
            services.map((s) => (
              <Card key={s.id}>
                <CardHeader
                  title={s.name}
                  icon={Server}
                  action={
                    <div className="flex items-center gap-2">
                      <Pill tone={s.algorithm === "round_robin" ? "blue" : s.algorithm === "least_connections" ? "green" : s.algorithm === "ip_hash" ? "yellow" : "neutral"}>
                        {s.algorithm.replaceAll("_", " ")}
                      </Pill>
                      <Pill tone="neutral" className="font-mono text-[11px]">
                        :{s.port} · {s.protocol.toUpperCase()}
                      </Pill>
                    </div>
                  }
                />
                {!s.targets || s.targets.length === 0 ? (
                  <EmptyState icon={Container} title="No targets" message="No healthy targets in this service." />
                ) : (
                  <div className="divide-y divide-[var(--line)]">
                    {s.targets.map((t) => (
                      <div key={t.id} className="flex items-center justify-between px-4 py-3 hover:bg-[color-mix(in_srgb,var(--surface-hover)_50%,transparent)] motion-safe:transition-colors motion-reduce:transition-none">
                        <div className="flex items-center gap-3">
                          <span className="font-mono text-sm text-[var(--text)]">
                            {t.ip}:{t.port}
                          </span>
                          <span className="text-xs text-[var(--text-subtle)]">weight {t.weight}</span>
                        </div>
                        <Pill tone={t.status === "healthy" ? "green" : t.status === "draining" ? "yellow" : "red"}>{t.status}</Pill>
                      </div>
                    ))}
                  </div>
                )}
              </Card>
            ))
          )}
        </div>
      )}

      {tab === "middlewares" && (
        <Card>
          <CardHeader title="Middlewares" icon={Shield} />
          <div className="p-4">
            <div className="rounded-lg border border-[color-mix(in_srgb,var(--warning)_20%,transparent)] bg-[var(--warning-subtle)] px-4 py-3 text-sm leading-6 text-[var(--text)]">
              Middlewares (rate-limit, IP allow/deny, circuit-breaker, headers, redirect) are currently applied to all routes. Manage them from Traffic Policies.
            </div>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              {[
                { icon: SlidersHorizontal, label: "Rate Limit", desc: "requests/sec + burst", tone: "yellow" as const },
                { icon: ShieldCheck, label: "IP Whitelist", desc: "allow CIDR", tone: "green" as const },
                { icon: ShieldOff, label: "IP Blacklist", desc: "deny CIDR", tone: "red" as const },
                { icon: Zap, label: "Circuit Breaker", desc: "threshold + timeout", tone: "blue" as const },
                { icon: Globe, label: "Headers / HSTS", desc: "per-domain security_headers", tone: "neutral" as const },
                { icon: Activity, label: "Forward Auth / Redirect", desc: "auth URL + stripPath", tone: "neutral" as const },
              ].map((m) => (
                <div key={m.label} className="flex gap-3 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
                  <m.icon size={18} className="mt-0.5 shrink-0 text-[var(--text-subtle)]" aria-hidden />
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-[var(--text)]">{m.label}</span>
                      <Pill tone={m.tone} className="tracking-wide">{m.tone}</Pill>
                    </div>
                    <p className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">{m.desc}</p>
                  </div>
                </div>
              ))}
            </div>
            <div className="mt-4 rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4">
              <p className="text-xs font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Traffic → Policies tab</p>
              <p className="mt-1 text-sm leading-6 text-[var(--text-subtle)]">
                Middlewares are applied to all routes. Create policies from Traffic Policies and they will be rendered here.
              </p>
            </div>
          </div>
        </Card>
      )}

      {tab === "certs" && (
        <Card>
          <CardHeader title="Certificates" icon={Lock} />
          {certsQuery.isLoading ? (
            <AdminLoadingState label="Loading certificates…" />
          ) : certsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={certsQuery.error instanceof Error ? certsQuery.error.message : "Failed to load certificates"}
                retry={() => void certsQuery.refetch()}
              />
            </div>
          ) : certs.length === 0 ? (
            <EmptyState
              icon={Lock}
              title="No certificates"
              message="No certificates yet. Upload one from Certificates or issue one via ACME."
            />
          ) : (
            <AdminTable label="Certificates">
              <AdminTHead><AdminTh>Domains</AdminTh><AdminTh>Provider</AdminTh><AdminTh>Expiry</AdminTh><AdminTh>Auto-Renew</AdminTh></AdminTHead>
              <AdminTBody>
                  {certs.map((c) => (
                    <AdminTr key={c.id}>
                      <AdminTd className="font-mono text-xs">{c.domains?.join(", ") ?? "—"}</AdminTd>
                      <AdminTd>
                        <Pill tone={c.provider === "letsencrypt" ? "blue" : "neutral"}>{c.provider}</Pill>
                      </AdminTd>
                      <AdminTd className="text-xs">
                        {c.expiresAt ? new Date(c.expiresAt).toLocaleDateString() : "—"}
                      </AdminTd>
                      <AdminTd>
                        <Pill tone={c.autoRenew ? "green" : "neutral"}>{c.autoRenew ? "Enabled" : "Disabled"}</Pill>
                      </AdminTd>
                    </AdminTr>
                  ))}
              </AdminTBody>
            </AdminTable>
          )}
          <div className="border-t border-[var(--line)] p-4">
            <p className="text-xs leading-5 text-[var(--text-subtle)]">
              <span className="font-semibold tracking-[0.04em] text-[var(--text-subtle)]">Proxy domains</span> ({domains.length}) —{" "}
              {domains.slice(0, 3).map((d) => d.hostname).join(", ")}
              {domains.length > 3 ? ` +${domains.length - 3} more` : ""}
              {domains.length === 0 ? "none yet — add under Domains / Gateways" : ""}
            </p>
          </div>
        </Card>
      )}
    </AdminPageLayout>
  );
}
