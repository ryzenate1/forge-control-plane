"use client";

import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { Eye, Globe, ShieldAlert } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  Btn,
  Card,
  CardHeader,
  DataTable,
  EmptyState,
  Pill,
  SectionHeader,
} from "./admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { fetchAdminProxyDomains } from "@/lib/api/proxy-domains";
import { errorMessage } from "@/lib/utils";

/**
 * What this page can honestly show.
 *
 * It used to render a `GLOBAL_HEADERS` constant as though it were measured
 * configuration: six hardcoded header values — two of them containing editorial
 * fragments inside header syntax (`[unsafe-eval on monaco routes]`,
 * `preload (production)`) — each wearing an "active" `Pill`. Nothing on the
 * page ever read the deployed middleware, so the pill claimed a state that had
 * not been observed and the values could drift from reality silently.
 *
 * The control plane sets those headers in its HTTP middleware
 * (`forge/api/internal/http/middleware_security.go`,
 * `middleware_security_headers.go`) and exposes **no endpoint that reports the
 * values in effect**. Until one exists there is nothing to render, so the card
 * says so instead of guessing. Per-domain overrides *are* readable
 * (`/domains/:domainId/security-headers`) and are managed on each domain page,
 * which is what the lower card points at.
 */
export function AdminSecurity() {
  const router = useRouter();
  const domainsQuery = useQuery({
    queryKey: ["admin-proxy-domains"],
    queryFn: () => fetchAdminProxyDomains(),
    retry: 1,
  });
  // The API serialises `Hostname`, not `domain` — reading the wrong key made
  // every row of the previous table blank.
  const domains = domainsQuery.data ?? [];

  return (
    <AdminPageLayout>
      <SectionHeader status={<FreshnessBadge state={sourceState(domainsQuery)} />} />

      <Card>
        <CardHeader title="Global middleware headers" icon={ShieldAlert} />
        <div className="space-y-3 p-4">
          <div className="flex flex-wrap items-center gap-2">
            <Pill tone="unknown">Not measured</Pill>
            <span className="t-meta">Directory, not a console</span>
          </div>
          <p className="max-w-prose text-xs leading-5 text-text-subtle">
            The control plane applies its baseline security headers (CSP, HSTS, frame and referrer policy, content-type sniffing,
            permissions policy) from HTTP middleware at response time. No endpoint reports the values currently in effect, so this
            page cannot show what is deployed and shows nothing rather than a guess — a documented list is not a reading, and an
            &quot;active&quot; badge on a value nobody measured is a lie.
          </p>
          <p className="max-w-prose text-xs leading-5 text-text-subtle">
            To verify what a response actually carries, inspect a live response from your panel origin. The per-domain overrides
            below are readable and editable; those are the only header values this page can honestly list.
          </p>
        </div>
      </Card>

      <Card>
        <CardHeader title="Per-domain security header policies" icon={Globe} />
        {domainsQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading domains…" /></div>
        ) : domainsQuery.isError ? (
          <div className="p-4"><AdminErrorState message={errorMessage(domainsQuery.error, "Domains could not be loaded.")} retry={() => void domainsQuery.refetch()} /></div>
        ) : domains.length === 0 ? (
          <EmptyState
            icon={Globe}
            title="No proxy domains configured"
            sub="Per-domain header overrides become available once a domain is registered under Domains."
          />
        ) : (
          <div className="p-4">
            <p className="mb-4 max-w-prose text-xs leading-5 text-text-subtle">
              Each proxy domain can override the baseline through{" "}
              <code className="ui-code-inline">/domains/:domainId/security-headers</code>. Select a domain to read or edit its stored
              policy — this page lists domains only and does not fetch each domain&apos;s header record.
            </p>
            <DataTable
              label="Proxy domains"
              headers={["Domain", "Policy"]}
              rows={domains.map((d) => ({
                Domain: <span className="font-mono text-xs">{d.hostname}</span>,
                Policy: (
                  <Btn size="sm" tone="ghost" onClick={() => router.push(`/admin/domains/${d.id}`)}>
                    <Eye size={13} /> Open domain policy
                  </Btn>
                ),
              }))}
            />
          </div>
        )}
      </Card>
    </AdminPageLayout>
  );
}
