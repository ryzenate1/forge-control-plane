"use client";

import { useQuery } from "@tanstack/react-query";
import { Archive } from "lucide-react";
import {
  EmptyOrganizations,
  ErrorAlert,
  SpinnerInline,
} from "@/components/shared";
import type { ReactNode } from "react";

type OrgInfo = {
  id: string;
  name: string;
  role: string;
  memberCount: number;
  createdAt: string;
};

interface TeamTenancyViewProps {
  access?: {
    isAdmin?: boolean;
    isOwner?: boolean;
    permissions?: string[] | null;
  } | null;
  action?: ReactNode;
}

export function TeamTenancyView({ action }: TeamTenancyViewProps) {
  const query = useQuery<OrgInfo[]>({
    queryKey: ["organizations"],
    queryFn: async () => {
      const res = await fetch("/api/v1/organizations", { credentials: "include" });
      if (!res.ok) throw new Error(`Failed to load organizations: ${res.status}`);
      return res.json() as Promise<OrgInfo[]>;
    },
  });

  if (query.isLoading) return <SpinnerInline label="Loading organizations…" />;

  if (query.isError) {
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load organizations"
        onRetry={() => void query.refetch()}
      />
    );
  }

  const orgs = query.data ?? [];

  if (!orgs.length) {
    return <EmptyOrganizations action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {orgs.length} organization{orgs.length === 1 ? "" : "s"}
        </span>
      </div>
      <div className="divide-y divide-white/[0.07]">
        {orgs.map((org) => (
          <div className="flex items-center gap-4 px-5 py-4" key={org.id}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <Archive size={16} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-slate-200">{org.name}</p>
              <p className="text-xs text-slate-500">
                {org.role} · {org.memberCount} member{org.memberCount === 1 ? "" : "s"}
              </p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
