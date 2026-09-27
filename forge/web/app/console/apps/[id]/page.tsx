"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Database,
  ExternalLink,
  FileText,
  Globe,
  History,
  KeyRound,
  Power,
  RefreshCw,
  RotateCcw,
  Settings,
  Square,
  Wrench,
  XCircle,
} from "lucide-react";
import { useToast } from "@/components/ui/toast";
import {
  fetchApp,
  fetchAppBackups,
  fetchAppDeployments,
  fetchAppDomains,
  fetchAppLogs,
  restartApp,
  startApp,
  stopApp,
  triggerDeploy,
  typeLabel,
  type ApiAppDetail,
} from "@/lib/api/apps";
import { statusTone } from "@/lib/api/status";
import { Btn, Card, CardHeader, EmptyState, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";
import { DeployStatusBadge, LogViewer } from "@/components/admin/AdminAppsShared";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { formatBytes, formatDate } from "@/lib/utils";
import { useBreadcrumbLabel } from "@/lib/nav/breadcrumb-context";

/**
 * /console/apps/[id] — customer-facing app detail page.
 *
 * A lighter counterpart to /admin/apps/[id]: same data via @/lib/api/apps,
 * rendered inside the console shell (inherited from app/console/layout).
 * Advanced operations (compose editor, git source, console, config edits)
 * intentionally link to the admin pages instead of duplicating them here.
 */

type TabId = "overview" | "deployments" | "configuration" | "logs" | "domains" | "backups";

const TABS: { id: TabId; label: string; icon: typeof Settings }[] = [
  { id: "overview", label: "Overview", icon: Power },
  { id: "deployments", label: "Deployments", icon: History },
  { id: "configuration", label: "Configuration", icon: Settings },
  { id: "logs", label: "Logs", icon: FileText },
  { id: "domains", label: "Domains", icon: Globe },
  { id: "backups", label: "Backups", icon: Database },
];

export default function ConsoleAppDetailPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? "";
  const [tab, setTab] = useState<TabId>("overview");

  const { data: app, isLoading, error } = useQuery({
    queryKey: ["app", id],
    queryFn: () => fetchApp(id),
    enabled: !!id,
    refetchInterval: 10_000,
  });

  const primaryDomain = useMemo(() => {
    const domains = app?.domains ?? [];
    return domains[0]?.domain ?? "";
  }, [app]);

  // The console frame renders the one breadcrumb trail; this names the id
  // segment so it reads "Console / Applications / My API". This page used to
  // render a second trail of its own inside the page header.
  useBreadcrumbLabel(id, app?.name ?? null);

  if (isLoading) return <LoadingSpinner />;

  if (error || !app) {
    return (
      <div className="space-y-6">
        <SectionHeader title="Application" sub="Error loading application" backAction={() => undefined} backLabel="Apps" />
        <EmptyState icon={XCircle} message={error instanceof Error ? error.message : "Application not found."} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <SectionHeader
        title={app.name}
        sub={`${typeLabel(app.type)} · ${app.id.slice(0, 8)}...`}
        action={
          <div className="flex items-center gap-2">
            <Pill tone={statusTone(app.status)}>{app.status}</Pill>
            <Link href={`/admin/apps/${encodeURIComponent(app.id)}`}>
              <Btn tone="ghost" size="sm" title="Advanced operations live on the admin app page">
                <Wrench size={12} /> Manage in Admin
              </Btn>
            </Link>
          </div>
        }
      />

      <div className="flex flex-wrap items-center gap-3">
        <Link href="/console/apps" className="inline-flex items-center gap-1.5 text-xs font-semibold text-slate-400 transition hover:text-slate-200">
          ← Back to applications
        </Link>
        {primaryDomain && (
          <a href={`https://${primaryDomain}`} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 font-mono text-xs text-slate-300 hover:text-[var(--brand)]">
            <Globe size={12} /> {primaryDomain} <ExternalLink size={10} />
          </a>
        )}
      </div>

      <div className="flex gap-1 overflow-x-auto border-b border-white/[0.06]">
        {TABS.map(({ id: tId, label, icon: Icon }) => (
          <button
            key={tId}
            type="button"
            className={cn(
              "-mb-px flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-medium transition",
              tab === tId ? "border-[var(--brand)] text-[var(--brand)]" : "border-transparent text-slate-500 hover:text-slate-300",
            )}
            onClick={() => setTab(tId)}
          >
            <Icon size={12} />
            {label}
          </button>
        ))}
      </div>

      {tab === "overview" && <OverviewSection app={app} id={id} />}
      {tab === "deployments" && <DeploymentsSection appId={id} />}
      {tab === "configuration" && <ConfigurationSection app={app} id={id} />}
      {tab === "logs" && <LogsSection appId={id} />}
      {tab === "domains" && <DomainsSection app={app} appId={id} />}
      {tab === "backups" && <BackupsSection appId={id} />}
    </div>
  );
}

function OverviewSection({ app, id }: { app: ApiAppDetail; id: string }) {
  return (
    <div className="space-y-6">
      <PowerButtons app={app} id={id} />
      <Card>
        <CardHeader title="Information" icon={Power} />
        <div className="divide-y divide-white/[0.06] text-sm">
          <div className="flex justify-between px-4 py-3">
            <span className="text-slate-400">Status</span>
            <DeployStatusBadge status={app.status} type="app" />
          </div>
          <div className="flex justify-between px-4 py-3">
            <span className="text-slate-400">Type</span>
            <span className="text-slate-200">{typeLabel(app.type)}</span>
          </div>
          <div className="flex justify-between px-4 py-3">
            <span className="text-slate-400">Image</span>
            <span className="font-mono text-xs text-slate-300">{app.image ?? "—"}</span>
          </div>
          <div className="flex justify-between px-4 py-3">
            <span className="text-slate-400">Version</span>
            <span className="text-slate-200">{app.version ?? "—"}</span>
          </div>
          {app.uptime && (
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Uptime</span>
              <span className="text-slate-200">{app.uptime}</span>
            </div>
          )}
          <div className="flex justify-between px-4 py-3">
            <span className="text-slate-400">Created</span>
            <span className="text-slate-200">{formatDate(app.createdAt)}</span>
          </div>
          {app.deployedAt && (
            <div className="flex justify-between px-4 py-3">
              <span className="text-slate-400">Last Deployed</span>
              <span className="text-slate-200">{formatDate(app.deployedAt)}</span>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}

function PowerButtons({ app, id }: { app: ApiAppDetail; id: string }) {
  const qc = useQueryClient();
  const { toast } = useToast();

  const invalidate = () => void qc.invalidateQueries({ queryKey: ["app", id] });
  const onError = (label: string) => (error: unknown) =>
    toast({ tone: "error", title: label, message: error instanceof Error ? error.message : `Failed to ${label.toLowerCase()}` });

  const startMut = useMutation({
    mutationFn: async () => {
      const result = await startApp(id);
      if (!result.ok) throw new Error("The server reported the app did not start.");
      return result;
    },
    onSuccess: invalidate,
    onError: onError("Start failed"),
  });
  const stopMut = useMutation({
    mutationFn: async () => {
      const result = await stopApp(id);
      if (!result.ok) throw new Error("The server reported the app did not stop.");
      return result;
    },
    onSuccess: invalidate,
    onError: onError("Stop failed"),
  });
  const restartMut = useMutation({ mutationFn: () => restartApp(id), onSuccess: invalidate, onError: onError("Restart failed") });
  const deployMut = useMutation({ mutationFn: () => triggerDeploy(id), onSuccess: invalidate, onError: onError("Deploy failed") });

  return (
    <div className="flex flex-wrap gap-3">
      {app.status === "running" && (
        <>
          <Btn tone="warning" onClick={() => stopMut.mutate()} disabled={stopMut.isPending}>
            <Square size={14} /> Stop
          </Btn>
          <Btn tone="ghost" onClick={() => restartMut.mutate()} disabled={restartMut.isPending}>
            <RotateCcw size={14} /> Restart
          </Btn>
        </>
      )}
      {(app.status === "stopped" || app.status === "failed") && (
        <Btn tone="success" onClick={() => startMut.mutate()} disabled={startMut.isPending}>
          <Power size={14} /> Start
        </Btn>
      )}
      <Btn tone="primary" onClick={() => deployMut.mutate()} disabled={deployMut.isPending}>
        <RefreshCw size={14} className={deployMut.isPending ? "animate-spin" : ""} /> Redeploy
      </Btn>
    </div>
  );
}

function DeploymentsSection({ appId }: { appId: string }) {
  const { data: deployments, isLoading } = useQuery({
    queryKey: ["app-deployments", appId],
    queryFn: () => fetchAppDeployments(appId),
    refetchInterval: 10_000,
  });
  const rows = useMemo(() => (Array.isArray(deployments) ? deployments : []).slice(0, 15), [deployments]);

  return (
    <Card>
      <CardHeader
        title={`Recent deployments (${rows.length})`}
        icon={History}
        action={
          <Link href={`/admin/apps/${encodeURIComponent(appId)}?tab=deployments`}>
            <Btn tone="ghost" size="sm">Full history</Btn>
          </Link>
        }
      />
      {isLoading ? (
        <div className="p-8 text-center text-sm text-slate-500">Loading deployments…</div>
      ) : rows.length === 0 ? (
        <EmptyState icon={History} message="No deployments yet." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Image</th>
                <th className="px-4 py-3">Strategy</th>
                <th className="px-4 py-3">Started</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {rows.map((dep) => (
                <tr key={dep.id} className="hover:bg-white/[0.02]">
                  <td className="px-4 py-3">
                    <DeployStatusBadge status={dep.status} type="deployment" />
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-300">{dep.image ?? (dep.revision != null ? `rev #${dep.revision}` : "—")}</td>
                  <td className="px-4 py-3 text-xs text-slate-400">{dep.rolloutStrategy ?? dep.strategy ?? "—"}</td>
                  <td className="px-4 py-3 text-xs text-slate-500">{formatDate(dep.startedAt ?? dep.createdAt ?? "")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}

function ConfigurationSection({ app, id }: { app: ApiAppDetail; id: string }) {
  const entries = Object.entries(app.envVars ?? {});

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader
          title="Environment Variables"
          icon={KeyRound}
          action={
            <Link href={`/admin/apps/${encodeURIComponent(id)}?tab=configuration`}>
              <Btn tone="ghost" size="sm">Edit in Admin</Btn>
            </Link>
          }
        />
        {entries.length === 0 ? (
          <EmptyState icon={KeyRound} message="No environment variables configured." />
        ) : (
          <div className="divide-y divide-white/[0.06] text-sm">
            {entries.map(([key, value]) => (
              <div key={key} className="flex items-start justify-between gap-4 px-4 py-3">
                <span className="shrink-0 font-mono text-xs text-slate-300">{key}</span>
                <span className="break-all text-right font-mono text-xs text-slate-400">{value || "—"}</span>
              </div>
            ))}
          </div>
        )}
      </Card>

      {app.ports.length > 0 && (
        <Card>
          <CardHeader title="Ports" icon={Globe} />
          <div className="divide-y divide-white/[0.06] text-sm">
            {app.ports.map((port) => (
              <div key={`${port.protocol}-${port.containerPort}-${port.hostPort}`} className="flex justify-between px-4 py-3">
                <span className="text-slate-400">{port.name ?? `${port.protocol}/${port.containerPort}`}</span>
                <span className="font-mono text-xs text-slate-300">{port.hostPort}:{port.containerPort}/{port.protocol}</span>
              </div>
            ))}
          </div>
        </Card>
      )}

      {(app.type === "compose" || app.type === "git") && (
        <p className="text-xs text-slate-500">
          {app.type === "compose" ? "Compose stack editing" : "Git source and auto-deploy settings"} live on the admin page:{" "}
          <Link className="text-[var(--brand)] hover:underline" href={`/admin/apps/${encodeURIComponent(id)}`}>
            /admin/apps/{app.id.slice(0, 8)}
          </Link>
        </p>
      )}
    </div>
  );
}

function LogsSection({ appId }: { appId: string }) {
  const { data: logs = [], isLoading } = useQuery({
    queryKey: ["app-logs", appId],
    queryFn: () => fetchAppLogs(appId),
    refetchInterval: 5_000,
  });

  return (
    <Card>
      <CardHeader title="Application Logs" icon={FileText} />
      <div className="p-4">
        <LogViewer logs={logs ?? []} loading={isLoading} />
      </div>
    </Card>
  );
}

function DomainsSection({ app, appId }: { app: ApiAppDetail; appId: string }) {
  const { data: domainsRaw } = useQuery({
    queryKey: ["app-domains", appId],
    queryFn: () => fetchAppDomains(appId),
  });
  const domains = useMemo(() => domainsRaw ?? app.domains ?? [], [domainsRaw, app.domains]);

  return (
    <Card>
      <CardHeader
        title={`Domains (${domains.length})`}
        icon={Globe}
        action={
          <Link href={`/admin/apps/${encodeURIComponent(appId)}?tab=domains`}>
            <Btn tone="ghost" size="sm">Manage in Admin</Btn>
          </Link>
        }
      />
      {domains.length === 0 ? (
        <EmptyState icon={Globe} message="No domains configured." />
      ) : (
        <div className="divide-y divide-white/[0.06] text-sm">
          {domains.map((d) => (
            <div key={d.id} className="flex items-center justify-between gap-3 px-4 py-3">
              <a href={`https://${d.domain}`} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 font-mono text-xs text-slate-200 hover:text-[var(--brand)]">
                {d.domain} <ExternalLink size={10} />
              </a>
              <div className="flex items-center gap-3">
                {d.ssl ? (
                  <Pill tone={d.sslStatus === "active" ? "green" : d.sslStatus === "failed" ? "red" : "yellow"}>{d.sslStatus ?? "enabled"}</Pill>
                ) : (
                  <Pill tone="neutral">no ssl</Pill>
                )}
                <span className="text-xs text-slate-500">{formatDate(d.createdAt)}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function BackupsSection({ appId }: { appId: string }) {
  const { data: backupsRaw, isLoading } = useQuery({
    queryKey: ["app-backups", appId],
    queryFn: () => fetchAppBackups(appId),
    refetchInterval: 10_000,
  });
  const backups = useMemo(() => backupsRaw ?? [], [backupsRaw]);

  return (
    <Card>
      <CardHeader
        title={`Backups (${backups.length})`}
        icon={Database}
        action={
          <Link href={`/admin/apps/${encodeURIComponent(appId)}?tab=backups`}>
            <Btn tone="ghost" size="sm">Create / restore in Admin</Btn>
          </Link>
        }
      />
      {isLoading ? (
        <div className="p-8 text-center text-sm text-slate-500">Loading backups…</div>
      ) : backups.length === 0 ? (
        <EmptyState icon={Database} message="No backups yet." />
      ) : (
        <div className="divide-y divide-white/[0.06] text-sm">
          {backups.map((b) => (
            <div key={b.id} className="flex items-center justify-between gap-3 px-4 py-3">
              <span className="font-mono text-xs text-slate-200">{b.name}</span>
              <div className="flex items-center gap-3">
                <Pill tone={b.status === "completed" ? "green" : b.status === "failed" ? "red" : b.status === "creating" || b.status === "restoring" ? "blue" : "neutral"}>
                  {b.status}
                </Pill>
                <span className="text-xs text-slate-400">{b.size ? formatBytes(b.size) : "—"}</span>
                <span className="text-xs text-slate-500">{formatDate(b.createdAt)}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}
