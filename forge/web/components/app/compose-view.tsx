"use client";

import { useQuery } from "@tanstack/react-query";
import { SpinnerInline, EmptyServices, ErrorAlert, DBStatus } from "@/components/shared";
import { fetchAppComposeConfig } from "@/lib/api/apps";
import type { ComposeService } from "@/lib/api/apps";
import type { ReactNode } from "react";

interface ComposeViewProps {
  appId: string;
  action?: ReactNode;
}

function parseServices(sourceConfig: unknown): ComposeService[] {
  if (!sourceConfig || typeof sourceConfig !== "object") return [];
  const cfg = sourceConfig as Record<string, unknown>;
  const raw = cfg.services as unknown[];
  if (!Array.isArray(raw)) return [];
  return raw.map((s: unknown) => {
    const entry = s as Record<string, unknown>;
    return {
      name: (entry.name ?? "") as string,
      status: (entry.status ?? "unknown") as ComposeService["status"],
      image: (entry.image ?? "") as string,
      ports: Array.isArray(entry.ports) ? (entry.ports as string[]) : [],
      createdAt: (entry.createdAt ?? "") as string,
    };
  });
}

export function ComposeView({ appId, action }: ComposeViewProps) {
  const query = useQuery({
    queryKey: ["app-compose", appId],
    queryFn: () => fetchAppComposeConfig(appId),
    enabled: Boolean(appId),
  });

  if (query.isLoading) return <SpinnerInline label="Loading services…" />;
  if (query.isError) {
    return <ErrorAlert error={query.error} title="Failed to load services" onRetry={() => void query.refetch()} />;
  }

  const services = query.data ? parseServices(query.data.sourceConfig) : [];

  if (!services.length) {
    return <EmptyServices action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {services.length} service{services.length === 1 ? "" : "s"}
        </span>
      </div>
      <div className="divide-y divide-white/[0.07]">
        {services.map((svc, idx) => (
          <div className="flex items-center gap-4 px-5 py-4" key={svc.name || String(idx)}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <span className="text-xs font-bold">{(svc.image || "?").slice(0, 2).toUpperCase()}</span>
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-slate-200">{svc.name}</p>
              <p className="text-xs text-slate-500">{svc.image}</p>
            </div>
            <DBStatus status={svc.status} />
            {svc.ports?.length ? (
              <span className="text-xs text-slate-500">{svc.ports.join(", ")}</span>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  );
}
