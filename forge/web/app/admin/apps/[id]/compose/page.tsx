"use client";

import { useState, use } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  Container, Power, RefreshCw, RotateCcw,
  Square, FileText,
} from "lucide-react";
import {
  fetchApp, fetchAppComposeConfig, fetchAppServiceLogs,
  startApp, stopApp, restartApp, redeployComposeStack,
} from "@/lib/api/apps";
import type { ComposeService } from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Pill, SectionHeader, Modal, AdminErrorState, AdminLoadingState, AdminPageLayout } from "@/components/admin/admin-ui";
import { LogViewer } from "@/components/admin/AdminAppsShared";
import { toast } from "@/components/ui/sonner";
import { useBreadcrumbLabel } from "@/lib/nav/breadcrumb-context";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { queryKeys } from "@/lib/api/query-keys";

export default function ComposeStackPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const qc = useQueryClient();

  const { data: app, isLoading: appLoading, isError: appError, refetch: refetchApp } = useQuery({
    queryKey: queryKeys.apps.detail(id),
    queryFn: () => fetchApp(id),
    refetchInterval: 10_000,
  });

  const { data: composeData } = useQuery({
    queryKey: ["app-compose", id],
    queryFn: () => fetchAppComposeConfig(id),
  });

  const [selectedService, setSelectedService] = useState<string | null>(null);
  const [showConfig, setShowConfig] = useState(false);

  const startMut = useMutation({
    mutationFn: () => startApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.detail(id) }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to start stack"),
  });
  const stopMut = useMutation({
    mutationFn: () => stopApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.detail(id) }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to stop stack"),
  });
  const restartMut = useMutation({
    mutationFn: () => restartApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.detail(id) }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to restart stack"),
  });
  const redeployMut = useMutation({
    mutationFn: () => redeployComposeStack(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.apps.detail(id) });
      toast.success("Stack re-deploy queued");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to redeploy stack"),
  });

  const sourceConfig = composeData?.sourceConfig as Record<string, unknown> | undefined;
  const services: ComposeService[] = sourceConfig?.services
    ? (sourceConfig.services as ComposeService[])
    : [];
  // The editor screen stores the document under "content", the creation form
  // under "composeContent" — accept both before falling back to raw JSON.
  const composeContent = typeof sourceConfig?.content === "string" ? sourceConfig.content
    : typeof sourceConfig?.composeContent === "string" ? (sourceConfig.composeContent as string)
    : sourceConfig ? JSON.stringify(sourceConfig, null, 2) : "";

  // The shell renders the one breadcrumb trail; this names its id crumb so
  // it reads as the app rather than a bare uuid. Before the name loads the
  // crumb keeps the id — it does not flash a placeholder.
  useBreadcrumbLabel(id, app?.name ?? null);

  if (appLoading) {
    return (
      <AdminPageLayout>
        <SectionHeader title="Compose Stack" sub="Loading..." />
        <AdminLoadingState label="Loading stack details..." />
      </AdminPageLayout>
    );
  }

  if (appError || !app) {
    return (
      <AdminPageLayout>
        <SectionHeader title="Compose Stack" sub="Multi-service Docker Compose management" />
        <AdminErrorState message="Could not load this compose stack." retry={() => void refetchApp()} />
      </AdminPageLayout>
    );
  }

  return (
    <AdminPageLayout>
      <SectionHeader
        title={app?.name ? `${app.name} · Compose Stack` : "Compose Stack"}
        sub="Multi-service Docker Compose management"
        info={adminPageGuides.applications}
        backAction={() => router.push(`/admin/apps/${id}`)}
        backLabel={app?.name ?? "App"}
      />

      <div className="flex flex-wrap gap-3">
        {app?.status === "running" && (
          <>
            <Btn tone="warning" onClick={() => stopMut.mutate()} disabled={stopMut.isPending}>
              <Square size={14} /> Stop Stack
            </Btn>
            <Btn tone="ghost" onClick={() => restartMut.mutate()} disabled={restartMut.isPending}>
              <RotateCcw size={14} /> Restart Stack
            </Btn>
          </>
        )}
        {(app?.status === "stopped" || app?.status === "failed") && (
          <Btn tone="success" onClick={() => startMut.mutate()} disabled={startMut.isPending}>
            <Power size={14} /> Start Stack
          </Btn>
        )}
        <Btn tone="primary" onClick={() => redeployMut.mutate()} disabled={redeployMut.isPending}>
          <RefreshCw size={14} className={redeployMut.isPending ? "animate-spin" : ""} /> Re-deploy Stack
        </Btn>
        <Btn tone="ghost" onClick={() => setShowConfig(true)}>
          <FileText size={14} /> View Config
        </Btn>
      </div>

      <Card>
        <CardHeader title={`${services.length} service${services.length === 1 ? "" : "s"}`} icon={Container} />
        {services.length === 0 ? (
          <EmptyState icon={Container} message="No services found in this compose stack." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-line text-left text-[10px] uppercase tracking-widest text-text-muted">
                  <th className="px-4 py-3">Service</th>
                  <th className="px-4 py-3">Image</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Ports</th>
                  <th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {services.map((svc) => (
                  <tr key={svc.name} className="hover:bg-overlay-subtle">
                    <td className="px-4 py-3 font-semibold text-text">{svc.name}</td>
                    <td className="px-4 py-3 font-mono text-xs text-text-subtle">{svc.image}</td>
                    <td className="px-4 py-3">
                      <Pill
                        tone={
                          svc.status === "running" ? "green"
                            : svc.status === "failed" ? "red"
                            : svc.status === "pending" ? "yellow"
                            : "neutral"
                        }
                      >
                        {svc.status}
                      </Pill>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-text-subtle">
                      {svc.ports.length > 0 ? svc.ports.join(", ") : "—"}
                    </td>
                    <td className="px-4 py-3">
                      <Btn size="sm" tone="ghost" onClick={() => setSelectedService(svc.name)}>
                        <FileText size={12} /> Logs
                      </Btn>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {selectedService && (
        <ServiceLogsModal
          appId={id}
          serviceName={selectedService}
          onClose={() => setSelectedService(null)}
        />
      )}

      {showConfig && composeData && (
        <Modal title="Compose Configuration" onClose={() => setShowConfig(false)} wide>
          <div className="space-y-4">
          <pre className="max-h-96 overflow-y-auto rounded-lg border border-line bg-[var(--canvas)] p-4 font-mono text-xs text-text-subtle whitespace-pre-wrap">
            {composeContent || JSON.stringify(composeData.sourceConfig, null, 2)}
          </pre>
          </div>
        </Modal>
      )}

      {redeployMut.error && (
        <div className="p-4">
          <AdminErrorState message={redeployMut.error.message} retry={() => redeployMut.mutate()} />
        </div>
      )}
    </AdminPageLayout>
  );
}

function ServiceLogsModal({ appId, serviceName, onClose }: { appId: string; serviceName: string; onClose: () => void }) {
  const { data, isLoading } = useQuery({
    queryKey: ["app-service-logs", appId, serviceName],
    queryFn: () => fetchAppServiceLogs(appId, serviceName),
    refetchInterval: 5_000,
  });

  const displayLogs = data ?? [];

  return (
    <Modal title={`${serviceName} Logs`} onClose={onClose} wide>
      <div className="space-y-4">
      <LogViewer logs={displayLogs} loading={isLoading} />
      </div>
    </Modal>
  );
}
