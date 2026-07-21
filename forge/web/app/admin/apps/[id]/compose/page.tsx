"use client";

import { useState, use } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, Container, Power, RefreshCw, RotateCcw,
  Square, FileText,
} from "lucide-react";
import {
  fetchApp, fetchAppComposeConfig, fetchAppServiceLogs,
  startApp, stopApp, restartApp, redeployComposeStack,
} from "@/lib/api/apps";
import { Btn, Card, CardHeader, EmptyState, Pill, SectionHeader, Modal } from "@/components/admin/admin-ui";
import { LogViewer } from "@/components/admin/AdminAppsShared";
import { toast, Toaster } from "@/components/ui/sonner";

export default function ComposeStackPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const qc = useQueryClient();

  const { data: app, isLoading: appLoading } = useQuery({
    queryKey: ["app", id],
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
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to start stack"),
  });
  const stopMut = useMutation({
    mutationFn: () => stopApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to stop stack"),
  });
  const restartMut = useMutation({
    mutationFn: () => restartApp(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["app", id] }),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to restart stack"),
  });
  const redeployMut = useMutation({
    mutationFn: () => redeployComposeStack(id),
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to redeploy stack"),
  });

  const services = composeData?.services ?? [];

  if (appLoading) {
    return (
      <div className="space-y-6">
        <SectionHeader title="Compose Stack" sub="Loading..." />
        <div className="p-8 text-center text-sm text-slate-500">Loading stack details...</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Btn tone="ghost" size="sm" onClick={() => router.push(`/admin/apps/${id}`)}>
          <ArrowLeft size={14} />
        </Btn>
        <SectionHeader
          title={app?.name ? `${app.name} - Compose Stack` : "Compose Stack"}
          sub="Multi-service Docker Compose management"
        />
      </div>

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
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Service</th>
                  <th className="px-4 py-3">Image</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Ports</th>
                  <th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {services.map((svc) => (
                  <tr key={svc.name} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-semibold text-slate-200">{svc.name}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">{svc.image}</td>
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
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">
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
          <pre className="max-h-96 overflow-y-auto rounded-lg border border-white/[0.06] bg-[#0a0e14] p-4 font-mono text-xs text-slate-400 whitespace-pre-wrap">
            {composeData.content}
          </pre>
        </Modal>
      )}

      {redeployMut.error && (
        <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
          {redeployMut.error.message}
        </div>
      )}
      <Toaster />
    </div>
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
      <LogViewer logs={displayLogs} loading={isLoading} />
    </Modal>
  );
}
