"use client";

import { useQuery } from "@tanstack/react-query";
import { Plus, Server } from "lucide-react";
import { fetchApps, type ApiApp } from "@/lib/api/apps";
import { Cpu, MemoryStick, HardDrive } from "lucide-react";
import { SkeletonList, EmptyList, ErrorAlert } from "@/components/shared";
import type { ReactNode } from "react";

interface AppListProps {
  onSelect?: (app: ApiApp) => void;
  emptyAction?: ReactNode;
}

export function AppList({ onSelect, emptyAction }: AppListProps) {
  const query = useQuery({
    queryKey: ["apps"],
    queryFn: fetchApps,
  });

  if (query.isLoading) {
    return <SkeletonList rows={6} columns={4} />;
  }

  if (query.isError) {
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load apps"
        onRetry={() => void query.refetch()}
        showDetails
      />
    );
  }

  const apps = query.data ?? [];

  if (!apps.length) {
    return (
      <EmptyList
        icon={Server}
        title="No apps yet"
        description="Deploy your first application to get started. Apps can be game servers, web apps, or any containerized service."
        action={emptyAction ?? (
          <button
            className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500"
            type="button"
          >
            <Plus size={14} />
            Create App
          </button>
        )}
      />
    );
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {apps.length} app{apps.length === 1 ? "" : "s"}
        </span>
      </div>
      <div className="divide-y divide-white/[0.07]">
        {apps.map((app) => (
          <button
            className="flex w-full items-center gap-4 px-5 py-4 text-left hover:bg-white/[0.02]"
            key={app.id}
            onClick={() => onSelect?.(app)}
            type="button"
          >
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <Server size={16} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold text-slate-200">{app.name}</p>
              <p className="text-xs text-slate-500">{app.id}</p>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
