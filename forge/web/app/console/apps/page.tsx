"use client";
import { queryKeys } from "@/lib/api/query-keys";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Boxes, Plus } from "lucide-react";
import { fetchApps, typeLabel, type ApiApp } from "@/lib/api/apps";
import { Card, SectionHeader } from "@/components/admin/admin-ui";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { SearchInput } from "@/components/ui/primitives";
import { DeployStatusBadge } from "@/components/admin/AdminAppsShared";
import { APP_TYPE_ICONS } from "@/lib/app-type-icons";

/**
 * /console/apps — customer-facing application hosting list.
 * Uses the same API as admin apps but within the console shell.
 */
export default function ConsoleAppsPage() {
  const [search, setSearch] = useState("");

  const { data: apps = [], isLoading } = useQuery({
    queryKey: queryKeys.apps.lists(),
    queryFn: fetchApps,
    staleTime: 30_000,
    refetchInterval: 15_000,
    retry: 1,
  });

  const filtered = useMemo(() => {
    if (!search.trim()) return apps;
    const q = search.toLowerCase();
    return apps.filter((a) => a.name.toLowerCase().includes(q) || a.domain?.toLowerCase().includes(q));
  }, [apps, search]);

  if (isLoading) return <LoadingSpinner />;

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Applications"
        sub={`${apps.length} application${apps.length !== 1 ? "s" : ""} deployed`}
        action={
          <Link href="/admin/apps/new" className="inline-flex items-center gap-2 rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white transition-colors hover:bg-[var(--brand-dark)]">
            <Plus size={14} /> New App
          </Link>
        }
      />

      <SearchInput value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search applications…" />

      {filtered.length === 0 ? (
        <Card>
          <div className="flex flex-col items-center gap-3 py-12 text-center">
            <Boxes size={32} className="text-slate-600" />
            <p className="text-sm text-slate-400">{search ? "No applications match your search." : "No applications yet. Deploy one to get started."}</p>
          </div>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {filtered.map((app) => (
            <AppCard key={app.id} app={app} />
          ))}
        </div>
      )}
    </div>
  );
}

function AppCard({ app }: { app: ApiApp }) {
  const IconComponent = APP_TYPE_ICONS[app.type] ?? Boxes;

  return (
    <Link href={`/console/apps/${app.id}`} className="group">
      <Card className="h-full transition-all group-hover:border-red-400/20 group-hover:bg-white/[0.02]">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white/[0.04] text-slate-400">
              <IconComponent size={16} />
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold text-white group-hover:text-red-200">{app.name}</p>
              <p className="mt-0.5 truncate text-[11px] text-slate-500">{typeLabel(app.type)}</p>
            </div>
          </div>
          <DeployStatusBadge status={app.observedStatus} />
        </div>
        {app.domain && (
          <p className="mt-2 truncate font-mono text-[11px] text-slate-500">{app.domain}</p>
        )}
      </Card>
    </Link>
  );
}
