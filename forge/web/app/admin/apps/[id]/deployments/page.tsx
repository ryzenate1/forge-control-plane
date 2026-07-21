"use client";

import { useState, use } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, History, RotateCcw, RefreshCw, XCircle, CheckCircle,
  Clock, AlertTriangle, Eye,
} from "lucide-react";
import {
  fetchApp, fetchAppDeployments,
} from "@/lib/api/apps";
import {
  fetchDeploymentSteps, cancelDeployment, rollbackToPrevious,
} from "@/lib/api/deployments";
import type { AppDeployment } from "@/lib/api/apps";
import type { DeploymentStep } from "@/lib/api/deployments";
import { Btn, Card, CardHeader, EmptyState, Modal, SectionHeader } from "@/components/admin/admin-ui";
import { cn } from "@/lib/utils";
import { toast, Toaster } from "@/components/ui/sonner";
import { DeployStatusBadge } from "@/components/admin/AdminAppsShared";
import { DeploymentProgress } from "@/components/app/deployment-progress";
import { RevisionCompare } from "@/components/app/revision-compare";
import { RollbackConfirm } from "@/components/app/rollback-confirm";
import type { RollbackChange } from "@/components/app/rollback-confirm";
import { formatDate } from "@/lib/utils";

export default function AppDeploymentsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const qc = useQueryClient();
  const [selectedDeployment, setSelectedDeployment] = useState<AppDeployment | null>(null);
  const [showProgress, setShowProgress] = useState(false);
  const [showRollback, setShowRollback] = useState<AppDeployment | null>(null);
  const [showCompare, setShowCompare] = useState(false);

  const appQuery = useQuery({
    queryKey: ["app", id],
    queryFn: () => fetchApp(id),
    enabled: !!id,
  });

  const deploymentsQuery = useQuery({
    queryKey: ["app-deployments", id],
    queryFn: () => fetchAppDeployments(id),
    enabled: !!id,
    refetchInterval: 10_000,
  });

  const cancelMut = useMutation({
    mutationFn: (deploymentId: string) => cancelDeployment(deploymentId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["app-deployments", id] });
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to cancel deployment"),
  });

  const rollbackMut = useMutation({
    mutationFn: () => rollbackToPrevious(showRollback?.id ?? ""),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["app-deployments", id] });
      setShowRollback(null);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to rollback deployment"),
  });

  const handleRollbackConfirm = (createSnapshot: boolean) => {
    rollbackMut.mutate();
  };

  const deployments = deploymentsQuery.data ?? [];
  const app = appQuery.data;
  const inProgress = deployments.filter(
    (d) => d.status === "pending" || d.status === "running",
  );

  const buildRollbackChanges = (dep: AppDeployment): RollbackChange[] => {
    const changes: RollbackChange[] = [];
    if (dep.image) {
      changes.push({
        field: "image",
        oldValue: dep.image,
        newValue: "previous revision image",
      });
    }
    changes.push({
      field: "revision",
      oldValue: `#${dep.revision}`,
      newValue: `#${dep.revision - 1}`,
    });
    return changes;
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Btn tone="ghost" size="sm" onClick={() => router.push(`/admin/apps/${id}`)}>
          <ArrowLeft size={14} />
        </Btn>
        <SectionHeader
          title="Deployments"
          sub={app ? `${app.name} - ${deployments.length} total` : "Loading..."}
        />
        <div className="ml-auto flex gap-2">
          <Btn tone="ghost" size="sm" onClick={() => setShowCompare(!showCompare)}>
            <Eye size={12} />
            Compare Revisions
          </Btn>
          <Btn size="sm" onClick={() => deploymentsQuery.refetch()}>
            <RefreshCw size={12} className={deploymentsQuery.isRefetching ? "animate-spin" : ""} />
            Refresh
          </Btn>
        </div>
      </div>

      {inProgress.length > 0 && (
        <Card>
          <CardHeader
            title="Active Deployment"
            icon={RefreshCw}
            action={
              <Btn
                size="sm"
                tone="danger"
                onClick={() => cancelMut.mutate(inProgress[0].id)}
                disabled={cancelMut.isPending}
              >
                <XCircle size={12} />
                Cancel
              </Btn>
            }
          />
          <div className="p-4">
            <DeploymentProgress
              deploymentId={inProgress[0].id}
              onComplete={() => qc.invalidateQueries({ queryKey: ["app-deployments", id] })}
              onError={() => qc.invalidateQueries({ queryKey: ["app-deployments", id] })}
            />
          </div>
        </Card>
      )}

      {showCompare && (
        <RevisionCompare
          deploymentId={inProgress.length > 0 ? inProgress[0].id : deployments[0]?.id ?? ""}
          onRollback={() => {
            qc.invalidateQueries({ queryKey: ["app-deployments", id] });
          }}
        />
      )}

      <Card>
        <CardHeader title="Deployment History" icon={History} />
        {deploymentsQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading deployments...</div>
        ) : deployments.length === 0 ? (
          <EmptyState icon={History} message="No deployments yet." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Revision</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Source</th>
                  <th className="px-4 py-3">Trigger</th>
                  <th className="px-4 py-3">Image</th>
                  <th className="px-4 py-3">Started</th>
                  <th className="px-4 py-3">Duration</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {deployments.map((dep) => (
                  <tr
                    key={dep.id}
                    className="hover:bg-white/[0.02]"
                  >
                    <td className="px-4 py-3 font-mono text-xs text-slate-200">#{dep.revision}</td>
                    <td className="px-4 py-3">
                      <DeployStatusBadge status={dep.status} type="deployment" />
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {dep.source ?? "—"}
                      {dep.commit && (
                        <span className="ml-1 font-mono text-slate-500">({dep.commit.slice(0, 7)})</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span className="rounded bg-white/5 px-2 py-0.5 text-[10px] font-medium text-slate-400">
                        {dep.trigger}
                      </span>
                    </td>
                    <td className="max-w-[120px] truncate px-4 py-3 font-mono text-xs text-slate-500">
                      {dep.image ?? "—"}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">{formatDate(dep.startedAt)}</td>
                    <td className="px-4 py-3 text-xs text-slate-500">
                      {dep.duration != null ? `${dep.duration}s` : "—"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Btn
                          size="sm"
                          tone="ghost"
                          onClick={() => {
                            setSelectedDeployment(dep);
                            setShowProgress(true);
                          }}
                        >
                          <Eye size={12} />
                          Details
                        </Btn>
                        {dep.status === "completed" && (
                          <Btn
                            size="sm"
                            tone="warning"
                            onClick={() => setShowRollback(dep)}
                          >
                            <RotateCcw size={12} />
                            Rollback
                          </Btn>
                        )}
                        {(dep.status === "pending" || dep.status === "running") && (
                          <Btn
                            size="sm"
                            tone="danger"
                            onClick={() => cancelMut.mutate(dep.id)}
                            disabled={cancelMut.isPending}
                          >
                            <XCircle size={12} />
                            Cancel
                          </Btn>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showProgress && selectedDeployment && (
        <Modal
          title={`Deployment #${selectedDeployment.revision} Details`}
          onClose={() => { setShowProgress(false); setSelectedDeployment(null); }}
          wide
        >
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-slate-400">Status: </span>
                <DeployStatusBadge status={selectedDeployment.status} type="deployment" />
              </div>
              <div>
                <span className="text-slate-400">Trigger: </span>
                <span className="text-slate-200">{selectedDeployment.trigger}</span>
              </div>
              <div>
                <span className="text-slate-400">Started: </span>
                <span className="text-slate-200">{formatDate(selectedDeployment.startedAt)}</span>
              </div>
              <div>
                <span className="text-slate-400">Duration: </span>
                <span className="text-slate-200">
                  {selectedDeployment.duration != null ? `${selectedDeployment.duration}s` : "—"}
                </span>
              </div>
              {selectedDeployment.image && (
                <div className="col-span-2">
                  <span className="text-slate-400">Image: </span>
                  <span className="font-mono text-xs text-slate-200">{selectedDeployment.image}</span>
                </div>
              )}
              {selectedDeployment.commit && (
                <div className="col-span-2">
                  <span className="text-slate-400">Commit: </span>
                  <span className="font-mono text-xs text-slate-200">{selectedDeployment.commit}</span>
                </div>
              )}
            </div>
            {selectedDeployment.error && (
              <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
                {selectedDeployment.error}
              </div>
            )}
            {selectedDeployment.log && (
              <div>
                <p className="mb-2 text-xs font-semibold text-slate-400">Build/Deploy Log</p>
                <pre className="max-h-48 overflow-y-auto rounded-lg border border-white/[0.06] bg-[#0a0e14] p-3 font-mono text-xs text-slate-400 whitespace-pre-wrap">
                  {selectedDeployment.log}
                </pre>
              </div>
            )}
          </div>
        </Modal>
      )}

      {showRollback && (
        <RollbackConfirm
          revisionNumber={showRollback.revision}
          changes={buildRollbackChanges(showRollback)}
          estimatedDowntime="30-60 seconds"
          onConfirm={handleRollbackConfirm}
          onCancel={() => setShowRollback(null)}
          loading={rollbackMut.isPending}
        />
      )}
      <Toaster />
    </div>
  );
}
