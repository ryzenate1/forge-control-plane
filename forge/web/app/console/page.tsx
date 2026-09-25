"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Boxes, Database, HardDrive, Server, Shield } from "lucide-react";
import { fetchServers } from "@/lib/api/servers";
import { fetchApps } from "@/lib/api/apps";
import { Card, CardHeader, Pill } from "@/components/admin/admin-ui";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";

/**
 * Console Dashboard — customer landing at /console.
 * Shows workload counts, recent activity, and quick-action cards.
 */
export default function ConsoleDashboardPage() {
  const serversQuery = useQuery({
    queryKey: ["servers"],
    queryFn: fetchServers,
    staleTime: 30_000,
    retry: 1,
  });

  const appsQuery = useQuery({
    queryKey: ["apps"],
    queryFn: fetchApps,
    staleTime: 30_000,
    retry: 1,
  });

  const servers = serversQuery.data ?? [];
  const apps = appsQuery.data ?? [];
  const runningServers = servers.filter((s) => s.status === "running").length;
  const runningApps = apps.filter((a) => a.desiredState === "running").length;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-white">Console</h1>
        <p className="mt-1 text-sm text-slate-400">Manage your workloads, applications, and infrastructure.</p>
      </div>

      {/* Stats row */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Game Servers"
          value={servers.length}
          subtitle={`${runningServers} running`}
          icon={Server}
          href="/console/servers"
          loading={serversQuery.isPending}
        />
        <StatCard
          title="Applications"
          value={apps.length}
          subtitle={`${runningApps} active`}
          icon={Boxes}
          href="/console/apps"
          loading={appsQuery.isPending}
        />
        <StatCard
          title="Databases"
          value="—"
          subtitle="Managed instances"
          icon={Database}
          href="/console/databases"
        />
        <StatCard
          title="Backups"
          value="—"
          subtitle="Last 24 hours"
          icon={HardDrive}
          href="/console/backups"
        />
      </div>

      {/* Quick actions */}
      <div>
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-slate-400">Quick Actions</h2>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <ActionCard
            title="Deploy Application"
            description="Launch a web app, API, or microservice from Git or Docker"
            icon={Boxes}
            href="/console/apps/new"
          />
          <ActionCard
            title="Create Game Server"
            description="Provision a Minecraft, Rust, or other game server"
            icon={Server}
            href="/console/servers"
          />
          <ActionCard
            title="Platform Health"
            description="Check node status, uptime, and service connectivity"
            icon={Shield}
            href="/console/health"
          />
        </div>
      </div>

      {/* Recent servers list */}
      {!serversQuery.isPending && servers.length > 0 && (
        <Card>
          <CardHeader
            title="Your Servers"
            action={<Link className="flex items-center gap-1 text-xs font-medium text-red-400 hover:text-red-300" href="/console/servers">View all <ArrowRight size={12} /></Link>}
          />
          <div className="divide-y divide-white/[0.04]">
            {servers.slice(0, 5).map((server) => (
              <Link
                key={server.id}
                className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-white/[0.02]"
                href={`/console/servers/${server.id}`}
              >
                <span className={`h-2 w-2 shrink-0 rounded-full ${server.suspended ? "bg-rose-400" : server.status === "running" ? "bg-emerald-400" : "bg-slate-500"}`} />
                <span className="min-w-0 flex-1 truncate text-sm font-medium text-slate-200">{server.name}</span>
                <Pill tone={server.status === "running" ? "success" : server.suspended ? "danger" : "neutral"}>
                  {server.suspended ? "Suspended" : server.status || "Offline"}
                </Pill>
              </Link>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}

/* ─── Sub-components ──────────────────────────────────────────────────────── */

function StatCard({ title, value, subtitle, icon: Icon, href, loading }: {
  title: string; value: number | string; subtitle: string;
  icon: React.ComponentType<{ size?: number; className?: string }>; href: string; loading?: boolean;
}) {
  return (
    <Link href={href} className="group">
      <Card className="transition-colors group-hover:border-white/[0.12]">
        <div className="flex items-start justify-between">
          <div>
            <p className="text-xs font-medium text-slate-400">{title}</p>
            {loading ? (
              <LoadingSpinner size="sm" className="mt-2" />
            ) : (
              <p className="mt-1 text-2xl font-bold text-white">{value}</p>
            )}
            <p className="mt-0.5 text-[11px] text-slate-500">{subtitle}</p>
          </div>
          <Icon size={20} className="text-slate-500" />
        </div>
      </Card>
    </Link>
  );
}

function ActionCard({ title, description, icon: Icon, href }: {
  title: string; description: string;
  icon: React.ComponentType<{ size?: number; className?: string }>; href: string;
}) {
  return (
    <Link href={href} className="group rounded-xl border border-white/[0.06] bg-[var(--surface)] p-4 transition-all hover:border-red-400/30 hover:bg-white/[0.02]">
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-red-500/10 text-red-400">
          <Icon size={18} />
        </div>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-white group-hover:text-red-200">{title}</p>
          <p className="truncate text-xs text-slate-400">{description}</p>
        </div>
      </div>
    </Link>
  );
}
