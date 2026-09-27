"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowUpCircle, Play, XCircle, CheckCircle2, Clock3, RefreshCw, Trash2, ShieldCheck } from "lucide-react";
import {
  AdminPageHeader,
  AdminPageLayout,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  StatsRow,
  AdminLoadingState,
  AdminErrorState,
  AdminConfirmDialog,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { formatDate } from "@/lib/utils";
import { deploymentStatusTone } from "@/lib/api/status";
import {
  checkForUpgrades,
  listUpgradePlans,
  createUpgradePlan,
  executeUpgradePlan,
  cancelUpgradePlan,
  deleteUpgradePlan,
  type UpgradeVersionInfo,
  type UpgradePlan,
} from "@/lib/api/upgrade";

const COMPONENTS = ["api", "web", "beacon", "database"];

// planTone used to live here. Its default branch returned "blue", so an
// upgrade-plan status this UI did not recognise was rendered as in progress —
// claiming an upgrade was under way on no evidence. Its "pending" was neutral,
// which read as idle rather than queued. deploymentStatusTone covers the plan
// states and returns `unknown` for the rest.
const planTone = deploymentStatusTone;

export function AdminUpgrade() {
  const qc = useQueryClient();
  const [confirmExec, setConfirmExec] = useState<UpgradePlan | null>(null);
  const [selectedType, setSelectedType] = useState<string>("full");
  const [selectedComponents, setSelectedComponents] = useState<string[]>([]);

  const versionsQ = useQuery({ queryKey: ["upgrade-versions"], queryFn: checkForUpgrades, retry: false });
  const plansQ = useQuery({ queryKey: ["upgrade-plans"], queryFn: () => listUpgradePlans(50), retry: false });

  const createMut = useMutation({
    mutationFn: () => createUpgradePlan(selectedType, selectedComponents),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["upgrade-plans"] }); },
  });
  const executeMut = useMutation({
    mutationFn: (id: string) => executeUpgradePlan(id),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["upgrade-plans"] }); },
  });
  const cancelMut = useMutation({
    mutationFn: (id: string) => cancelUpgradePlan(id),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["upgrade-plans"] }); },
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteUpgradePlan(id),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["upgrade-plans"] }); },
  });

  const versions: UpgradeVersionInfo[] = versionsQ.data ?? [];
  const plans: UpgradePlan[] = plansQ.data ?? [];
  const upgradable = versions.filter((v) => v.upgradable).length;
  const activePlans = plans.filter((p) => !["completed", "failed", "rolled_back"].includes(p.status)).length;

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminPageHeader
        title="Platform Upgrade"
        description="Self-upgrade the control plane with automatic backup and rollback. Create a plan for one or more components, review it, then execute — health is verified after each step and a failed upgrade rolls back automatically."
      />

      <StatsRow
        items={[
          { label: "Components", value: versions.length || 4, icon: ArrowUpCircle },
          { label: "Upgradable", value: upgradable, icon: RefreshCw, tone: upgradable ? "yellow" : "neutral" },
          { label: "Active plans", value: activePlans, icon: Clock3, tone: activePlans ? "blue" : "neutral" },
          { label: "Total plans", value: plans.length, icon: ShieldCheck },
        ]}
      />

      {/* Version check */}
      <Card>
        <CardHeader title="Component versions" icon={ArrowUpCircle} action={<Btn size="sm" tone="ghost" onClick={() => void versionsQ.refetch()}><RefreshCw size={12} className={versionsQ.isFetching ? "animate-spin" : ""} /> Re-check</Btn>} />
        {versionsQ.isLoading ? (
          <AdminLoadingState label="Checking versions…" />
        ) : versionsQ.isError ? (
          <div className="p-4"><AdminErrorState message={(versionsQ.error as Error).message} retry={() => void versionsQ.refetch()} /></div>
        ) : versions.length === 0 ? (
          <EmptyState icon={ArrowUpCircle} title="No version data" message="The upgrade service could not determine current versions. Ensure VERSION files exist at the configured paths." />
        ) : (
          <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-4">
            {versions.map((v) => (
              <div key={v.component} className="rounded-xl border border-white/[0.07] bg-white/[0.015] p-4">
                <div className="text-[11px] uppercase tracking-widest text-slate-500">{v.component}</div>
                <div className="mt-1 font-mono text-sm text-slate-200">{v.current}</div>
                {v.upgradable ? (
                  <Pill tone="yellow"><ArrowUpCircle size={10} /> → {v.latest}</Pill>
                ) : (
                  <Pill tone="green"><CheckCircle2 size={10} /> up to date</Pill>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* Create plan */}
      <Card>
        <CardHeader title="Create upgrade plan" icon={Play} />
        <div className="space-y-3 p-4">
          <div className="flex flex-wrap gap-2">
            {["full", ...COMPONENTS].map((t) => (
              <button key={t} type="button" onClick={() => setSelectedType(t)} className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${selectedType === t ? "bg-blue-500/20 text-blue-300 ring-1 ring-blue-500/30" : "border border-white/[0.08] text-slate-400 hover:bg-white/[0.04]"}`}>
                {t}
              </button>
            ))}
          </div>
          {selectedType !== "full" && (
            <div>
              <div className="mb-1 text-[11px] uppercase tracking-widest text-slate-500">Components (select specific ones)</div>
              <div className="flex flex-wrap gap-2">
                {COMPONENTS.map((c) => (
                  <label key={c} className="flex items-center gap-1.5 rounded-md border border-white/[0.08] bg-white/[0.02] px-2 py-1 text-xs cursor-pointer">
                    <input type="checkbox" checked={selectedComponents.includes(c)} onChange={(e) => setSelectedComponents((prev) => e.target.checked ? [...prev, c] : prev.filter((x) => x !== c))} className="accent-blue-500" />
                    <span className="font-mono">{c}</span>
                  </label>
                ))}
              </div>
            </div>
          )}
          {createMut.isError && <div className="rounded-lg border border-red-500/25 bg-red-500/10 p-3 text-sm text-red-200">{(createMut.error as Error).message}</div>}
          <Btn tone="primary" loading={createMut.isPending} onClick={() => createMut.mutate()}>
            <Play size={14} /> Create plan
          </Btn>
        </div>
      </Card>

      {/* Plans list */}
      <Card>
        <CardHeader title={`Upgrade plans · ${plans.length}`} icon={Clock3} />
        {plansQ.isLoading ? (
          <AdminLoadingState label="Loading plans…" />
        ) : plansQ.isError ? (
          <div className="p-4"><AdminErrorState message={(plansQ.error as Error).message} retry={() => void plansQ.refetch()} /></div>
        ) : plans.length === 0 ? (
          <EmptyState icon={Clock3} title="No upgrade plans" message="Create a plan above, then execute it to upgrade the control plane." />
        ) : (
          <div className="divide-y divide-white/[0.04]">
            {plans.map((p) => {
              const tone = planTone(p.status);
              const isActive = !["completed", "failed", "rolled_back"].includes(p.status);
              const pct = p.totalSteps > 0 ? Math.round((p.progress / p.totalSteps) * 100) : 0;
              return (
                <div key={p.id} className="flex items-start justify-between gap-4 p-4">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <Pill tone={tone}>{p.status.replace(/_/g, " ")}</Pill>
                      <span className="text-xs font-semibold text-slate-200">{p.type}</span>
                      {p.fromVersion && p.toVersion && <span className="font-mono text-[11px] text-slate-500">{p.fromVersion} → {p.toVersion}</span>}
                    </div>
                    <div className="mt-1 text-xs text-slate-500">
                      {p.components?.length ? `components: ${Array.isArray(p.components) ? p.components.join(", ") : String(p.components)}` : "all components"}
                      {" · "}created {formatDate(p.createdAt)}
                      {p.completedAt ? ` · finished ${formatDate(p.completedAt)}` : ""}
                    </div>
                    {isActive && p.totalSteps > 0 && (
                      <div className="mt-2 max-w-xs">
                        <div className="h-1 overflow-hidden rounded-full bg-white/[0.06]"><div className="h-full bg-blue-400/60 transition-all" style={{ width: `${pct}%` }} /></div>
                        <div className="mt-0.5 text-[10px] text-slate-500">{p.currentStep ? p.currentStep.replace(/_/g, " ") : "waiting…"} · {pct}%</div>
                      </div>
                    )}
                    {p.error && <div className="mt-1 text-[11px] text-red-300">{p.error}</div>}
                  </div>
                  <div className="flex shrink-0 gap-1">
                    {p.status === "pending" && (
                      <Btn size="sm" tone="primary" onClick={() => setConfirmExec(p)}>
                        <Play size={12} /> Execute
                      </Btn>
                    )}
                    {isActive && (
                      <Btn size="sm" tone="danger" disabled={cancelMut.isPending} onClick={() => cancelMut.mutate(p.id)}>
                        <XCircle size={12} /> Cancel
                      </Btn>
                    )}
                    {!isActive && (
                      <Btn size="sm" tone="ghost" disabled={deleteMut.isPending} onClick={() => deleteMut.mutate(p.id)}>
                        <Trash2 size={12} />
                      </Btn>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>

      {confirmExec && (
        <AdminConfirmDialog
          title="Execute upgrade plan?"
          description={`This will upgrade ${confirmExec.type === "full" ? "all components" : confirmExec.components.join(", ")} (${confirmExec.fromVersion || "?"} → ${confirmExec.toVersion || "?"}). A database backup is taken first; if any step fails the upgrade rolls back automatically.`}
          confirmLabel="Execute upgrade"
          destructive
          loading={executeMut.isPending}
          onCancel={() => setConfirmExec(null)}
          onConfirm={() => { executeMut.mutate(confirmExec.id, { onSettled: () => setConfirmExec(null) }); }}
        />
      )}
    </AdminPageLayout>
  );
}
