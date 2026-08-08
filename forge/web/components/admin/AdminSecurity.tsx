"use client";

import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { Globe, Shield, Eye } from "lucide-react";
import { AdminPageHeader, Card, CardHeader, Pill, DataTable, Btn } from "./admin-ui";

const GLOBAL_HEADERS = [
  {
    header: "Content-Security-Policy",
    value: `default-src 'self'; script-src 'self' [unsafe-eval on monaco routes]; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; report-uri /api/v1/csp-report`,
    purpose: "Prevents XSS and data injection attacks by controlling which resources can be loaded.",
  },
  {
    header: "Strict-Transport-Security",
    value: "max-age=31536000; includeSubDomains; preload (production)",
    purpose: "Enforces HTTPS connections and prevents protocol downgrade attacks.",
  },
  {
    header: "X-Frame-Options",
    value: "DENY",
    purpose: "Prevents clickjacking by disallowing the page from being rendered in a frame.",
  },
  {
    header: "X-Content-Type-Options",
    value: "nosniff",
    purpose: "Prevents MIME type sniffing by forcing the declared content-type.",
  },
  {
    header: "Referrer-Policy",
    value: "strict-origin-when-cross-origin",
    purpose: "Controls how much referrer information is sent with requests.",
  },
  {
    header: "Permissions-Policy",
    value: "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=()",
    purpose: "Restricts access to sensitive browser features and APIs.",
  },
];

export function AdminSecurity() {
  return (
    <div className="space-y-6">
      <AdminPageHeader
        title="Security"
        description="Global security headers applied by the middleware layer and per-domain overrides."
      />

      <Card>
        <CardHeader title="Global Middleware Headers" icon={Shield} />
        <div className="p-4">
          <p className="mb-4 text-sm leading-6 text-slate-400">
            These headers are applied globally to every response by the <code className="rounded bg-white/[0.06] px-1.5 py-0.5 font-mono text-xs text-slate-300">SecurityHeaders</code> middleware defined in <code className="rounded bg-white/[0.06] px-1.5 py-0.5 font-mono text-xs text-slate-300">middleware_security.go</code>. They provide baseline protection against XSS, clickjacking, MIME sniffing, protocol downgrades, and data exfiltration.
          </p>
          <div className="space-y-4">
            {GLOBAL_HEADERS.map((h) => (
              <div key={h.header} className="rounded-lg border border-white/[0.06] bg-white/[0.015] p-4">
                <div className="mb-2 flex items-center gap-2">
                  <span className="font-mono text-xs font-semibold text-slate-100">{h.header}</span>
                  <Pill tone="blue">active</Pill>
                </div>
                <p className="mb-2 text-xs leading-5 text-slate-400">{h.purpose}</p>
                <code className="block break-all rounded bg-black/30 px-2 py-1.5 font-mono text-[11px] leading-5 text-slate-300">
                  {h.value}
                </code>
              </div>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader title="Per-Domain Security Headers" icon={Globe} />
        <DomainSecurityHeadersSection />
      </Card>
    </div>
  );
}

function DomainSecurityHeadersSection() {
  const router = useRouter();
  const domainsQuery = useQuery({
    queryKey: ["admin-domains"],
    queryFn: () => fetchJSON<{ data: Array<{ id: string; domain: string }> }>("/domains"),
  });

  const domains = domainsQuery.data?.data ?? [];

  if (domainsQuery.isLoading) {
    return <div className="p-6 text-center text-sm text-slate-500">Loading domains…</div>;
  }

  if (domains.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 p-8 text-center">
        <Globe size={28} className="text-slate-500" strokeWidth={1.5} />
        <p className="text-sm text-slate-400">No proxy domains configured yet.</p>
        <p className="text-xs text-slate-500">
          Per-domain security headers can be managed once domains are added under Advanced &rarr; Domains.
        </p>
      </div>
    );
  }

  return (
    <div className="p-4">
      <p className="mb-4 text-sm leading-6 text-slate-400">
        Each proxy domain can have its own security header overrides via{" "}
        <code className="rounded bg-white/[0.06] px-1.5 py-0.5 font-mono text-xs text-slate-300">
          /domains/:domainId/security-headers
        </code>
        . Select a domain below to view or edit its configuration.
      </p>
      <DataTable
        label="Domains"
        headers={["Domain", "HSTS", "CSP", "X-Frame-Options", "Actions"]}
        rows={domains.map((d) => ({
          Domain: d.domain,
          HSTS: <Pill tone="green">Enabled</Pill>,
          CSP: <Pill tone="green">Active</Pill>,
          "X-Frame-Options": <Pill tone="blue">DENY</Pill>,
          Actions: (
            <Btn
              size="sm"
              tone="ghost"
              onClick={() => router.push(`/admin/domains/${d.id}`)}
            >
              <Eye size={13} className="mr-1" />
              View
            </Btn>
          ),
        }))}
      />
    </div>
  );
}

function fetchJSON<T>(path: string): Promise<T> {
  return import("@/lib/api/http").then((m) => m.fetchJSON<T>(path));
}
