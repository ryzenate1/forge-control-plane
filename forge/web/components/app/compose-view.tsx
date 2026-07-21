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

export function ComposeView({ appId, action }: ComposeViewProps) {
  const query = useQuery({
    queryKey: ["app-compose", appId],
    queryFn: () => fetchAppComposeConfig(appId),
    enabled: Boolean(appId),
    select: (data) => data.services,
  });

  if (query.isLoading) return <SpinnerInline label="Loading services…" />;
  if (query.isError) {
    return <ErrorAlert error={query.error} title="Failed to load services" onRetry={() => void query.refetch()} />;
  }

  const services: ComposeService[] = query.data ?? [];

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
          <div className="flex items-center gap-4 px-5 py-4" key={svc.name ?? idx}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <span className="text-xs font-bold">{svc.image.slice(0, 2).toUpperCase()}</span>
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
