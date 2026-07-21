"use client";

import { useQuery } from "@tanstack/react-query";
import { Cloud } from "lucide-react";
import {
  EmptyCertificates,
  ErrorAlert,
  CertStatus,
  SpinnerInline,
} from "@/components/shared";
import { fetchAppCertificates } from "@/lib/api/apps";
import type { AppCertificate } from "@/lib/api/apps";
import { formatDate } from "@/lib/utils";
import type { ReactNode } from "react";

interface CertificatesViewProps {
  appId: string;
  action?: ReactNode;
}

export function CertificatesView({ appId, action }: CertificatesViewProps) {
  const query = useQuery<AppCertificate[]>({
    queryKey: ["app-certs", appId],
    queryFn: () => fetchAppCertificates(appId),
    enabled: Boolean(appId),
  });

  if (query.isLoading) return <SpinnerInline label="Loading certificates…" />;

  if (query.isError) {
    return (
      <ErrorAlert
        error={query.error}
        title="Failed to load certificates"
        onRetry={() => void query.refetch()}
      />
    );
  }

  const certs = query.data ?? [];

  if (!certs.length) {
    return <EmptyCertificates action={action} />;
  }

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">
          {certs.length} certificate{certs.length === 1 ? "" : "s"}
        </span>
        {action ? <div className="ml-auto">{action}</div> : null}
      </div>
      <div className="divide-y divide-white/[0.07]">
        {certs.map((cert) => (
          <div className="flex items-center gap-4 px-5 py-4" key={cert.id}>
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-white/[0.06] text-slate-400">
              <Cloud size={16} />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-slate-200">{cert.domain}</p>
              <p className="text-xs text-slate-500">
                {cert.issuer}
                {cert.expiresAt ? ` · Expires ${formatDate(cert.expiresAt)}` : ""}
                {cert.autoRenew ? " · Auto-renew on" : " · Auto-renew off"}
              </p>
            </div>
            <CertStatus status={cert.status} expiresAt={cert.expiresAt} />
          </div>
        ))}
      </div>
    </div>
  );
}
