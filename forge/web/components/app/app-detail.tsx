"use client";

import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Server } from "lucide-react";
import Link from "next/link";
import { fetchApp, type ApiAppDetail } from "@/lib/api/apps";
import {
  SkeletonDetail,
  ErrorAlert,
  ErrorNotFound,
  ServerStatus,
} from "@/components/shared";
import type { ReactNode } from "react";

interface AppDetailViewProps {
  appId: string;
  children?: ReactNode | ((app: ApiAppDetail) => ReactNode);
}

export function AppDetailView({ appId, children }: AppDetailViewProps) {
  const query = useQuery({
    queryKey: ["app", appId],
    queryFn: () => fetchApp(appId),
    enabled: Boolean(appId),
  });

  if (query.isLoading) {
    return <SkeletonDetail />;
  }

  if (query.isError) {
    if (String(query.error).includes("404") || String(query.error).includes("not found")) {
      return <ErrorNotFound resource="App" />;
    }
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load app"
        onRetry={() => void query.refetch()}
        showDetails
      />
    );
  }

  const app = query.data;
  if (!app) {
    return <ErrorNotFound resource="App" />;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link
          className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-white/10 text-slate-400 hover:bg-white/[0.06]"
          href="/servers"
        >
          <ArrowLeft size={16} />
        </Link>
        <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
          <Server size={16} />
        </div>
        <div className="min-w-0 flex-1">
          <h1 className="text-xl font-bold text-slate-100">{app.name}</h1>
          <p className="text-xs text-slate-500">{app.id}</p>
        </div>
        <ServerStatus status={app.status} />
      </div>
      {typeof children === "function" ? children(app) : children}
    </div>
  );
}
