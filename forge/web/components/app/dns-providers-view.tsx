"use client";

import { useQuery } from "@tanstack/react-query";
import { Cloud } from "lucide-react";
import {
  EmptyDNSProviders,
  ErrorAlert,
  VerificationStatus,
  SpinnerInline,
} from "@/components/shared";
import { fetchDnsProviders } from "@/lib/api/dns";
import type { DNSProvider } from "@/lib/api/dns";
import { formatDate } from "@/lib/utils";
import type { ReactNode } from "react";

interface DNSProvidersViewProps {
  action?: ReactNode;
}

export function DNSProvidersView({ action }: DNSProvidersViewProps) {
  const query = useQuery<DNSProvider[]>({
    queryKey: ["dns-providers"],
    queryFn: fetchDnsProviders,
  });

  if (query.isLoading) return <SpinnerInline label="Loading DNS providers…" />;

  if (query.isError) {
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load DNS providers"
        onRetry={() => void query.refetch()}
      />
    );
  }

  const providers = query.data ?? [];

  if (!providers.length) {
    return <EmptyDNSProviders action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {providers.length} DNS provider{providers.length === 1 ? "" : "s"}
        </span>
        {action ? <div className="ml-auto">{action}</div> : null}
      </div>
      <div className="divide-y divide-white/[0.07]">
        {providers.map((dns) => (
          <div className="flex items-center gap-4 px-5 py-4" key={dns.id}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <Cloud size={16} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-slate-200">{dns.name}</p>
              <p className="text-xs text-slate-500">
                {dns.provider} · {formatDate(dns.createdAt)}
              </p>
            </div>
            <VerificationStatus status={dns.verificationStatus} />
          </div>
        ))}
      </div>
    </div>
  );
}
