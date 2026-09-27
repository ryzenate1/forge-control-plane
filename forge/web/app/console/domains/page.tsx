"use client";

import { useQuery } from "@tanstack/react-query";
import { Globe } from "lucide-react";
import { Card, SectionHeader } from "@/components/admin/admin-ui";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { fetchAdminProxyDomains } from "@/lib/api/proxy-domains";

/**
 * /console/domains — customer-facing domains overview.
 */
export default function ConsoleDomainsPage() {
  const { data: domains = [], isLoading } = useQuery({
    queryKey: ["proxy-domains"],
    queryFn: () => fetchAdminProxyDomains(),
    staleTime: 30_000,
    retry: 1,
  });

  if (isLoading) return <LoadingSpinner />;

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Domains"
        sub={`${domains.length} domain${domains.length !== 1 ? "s" : ""} configured`}
      />

      {domains.length === 0 ? (
        <Card>
          <div className="flex flex-col items-center gap-3 py-12 text-center">
            <Globe size={32} className="text-slate-600" />
            <p className="text-sm text-slate-400">No custom domains configured. Add domains to your applications for custom URLs.</p>
          </div>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {domains.map((domain) => (
            <Card key={domain.id}>
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-semibold text-white">{domain.hostname}</p>
                  <p className="text-xs text-slate-500">{domain.serviceType ?? "proxy"} · {domain.https ? "HTTPS" : "HTTP"}:{domain.port}</p>
                </div>
                <span className={`h-2 w-2 rounded-full ${domain.certType ? "bg-emerald-400" : "bg-amber-400"}`} title={domain.certType ? "SSL configured" : "No SSL"} />
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
