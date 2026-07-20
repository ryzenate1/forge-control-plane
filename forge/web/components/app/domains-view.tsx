"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Globe } from "lucide-react";
import {
  EmptyDomains,
  ErrorAlert,
  VerificationStatus,
  SpinnerInline,
  useAppToast,
} from "@/components/shared";
import { errorMessage, formatDate } from "@/lib/utils";
import type { ReactNode } from "react";

type DomainInfo = {
  id: string;
  domain: string;
  verified: boolean;
  verificationStatus?: string;
  createdAt: string;
};

interface DomainsViewProps {
  appId: string;
  action?: ReactNode;
}

export function DomainsView({ appId, action }: DomainsViewProps) {
  const qc = useQueryClient();
  const { success: showSuccess, error: showError } = useAppToast();

  const query = useQuery<DomainInfo[]>({
    queryKey: ["app-domains", appId],
    queryFn: async () => {
      const res = await fetch(`/api/v1/servers/${encodeURIComponent(appId)}/domains`, {
        credentials: "include",
      });
      if (!res.ok) throw new Error(`Failed to load domains: ${res.status}`);
      return res.json() as Promise<DomainInfo[]>;
    },
    enabled: Boolean(appId),
  });

  const deleteMut = useMutation({
    mutationFn: async (domainId: string) => {
      const res = await fetch(
        `/api/v1/servers/${encodeURIComponent(appId)}/domains/${encodeURIComponent(domainId)}`,
        { method: "DELETE", credentials: "include" },
      );
      if (!res.ok) throw new Error(`Failed to delete domain: ${res.status}`);
    },
    onSuccess: () => {
      showSuccess("Domain", "deleted");
      void qc.invalidateQueries({ queryKey: ["app-domains", appId] });
    },
    onError: (error) => showError("Domain", errorMessage(error, "Failed to delete domain")),
  });

  if (query.isLoading) return <SpinnerInline label="Loading domains…" />;

  if (query.isError) {
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load domains"
        onRetry={() => void query.refetch()}
      />
    );
  }

  const domains = query.data ?? [];

  if (!domains.length) {
    return <EmptyDomains action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {domains.length} domain{domains.length === 1 ? "" : "s"}
        </span>
        {action ? <div className="ml-auto">{action}</div> : null}
      </div>
      <div className="divide-y divide-white/[0.07]">
        {domains.map((domain) => (
          <div className="flex items-center gap-4 px-5 py-4" key={domain.id}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <Globe size={16} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate font-mono text-sm font-semibold text-slate-200">
                {domain.domain}
              </p>
              <p className="text-xs text-slate-500">
                {formatDate(domain.createdAt)}
              </p>
            </div>
            <VerificationStatus status={domain.verificationStatus ?? (domain.verified ? "verified" : "pending")} />
            <button
              className="rounded border border-red-500/30 px-3 py-1.5 text-xs font-semibold text-red-300 hover:bg-red-500/10"
              disabled={deleteMut.isPending}
              onClick={() => {
                if (window.confirm(`Remove ${domain.domain}?`)) deleteMut.mutate(domain.id);
              }}
              type="button"
            >
              {deleteMut.isPending ? "Removing…" : "Remove"}
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}
