"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowUpCircle, Play, XCircle, RefreshCw, Trash2, Clock3, Database } from "lucide-react";
import {
  AdminPageLayout,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  SectionHeader,
  AdminLoadingState,
  AdminErrorState,
  AdminConfirmDialog,
} from "@/components/admin/admin-ui";
import { AggregateTile, FreshnessBadge, MetricTile, Reading } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { OfflineBanner } from "@/components/shared/states-offline";
import { formatDate } from "@/lib/utils";
import { deploymentStatusTone } from "@/lib/api/status";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { errorMessage } from "@/lib/utils";
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

/**
 * The four names the upgrade service accepts (`validComponent`,
 * `internal/services/upgrade/service.go:179-186`). Anything else is rejected by
 * the server, so the picker cannot offer more — and if the backend grows a
 * component this list has to grow with it.
 */
const COMPONENTS = ["api", "web", "beacon", "database"];

const TERMINAL = ["completed", "failed", "rolled_back"];

/**
 * planTone used to live here. Its default branch returned "blue", so an
 * upgrade-plan status this UI did not recognise was rendered as in progress —
 * claiming an upgrade was under way on no evidence. Its "pending" was neutral,
 * which read as idle rather than queued. deploymentStatusTone covers the plan
 * states and returns `unknown` for the rest.
 */
const planTone = deploymentStatusTone;

/**
 * `upgradable` from the server is `currentVersion != latestVersion`
 * (`service.go:137`) — a string inequality, and `latest` is read from an env var
 * or a `<component>.latest.version` file on this host (`service.go:167-177`).
 * Nothing queries a registry, so "differs" is the only claim available here, and
 * an older target is a downgrade dressed as an upgrade. This orders the two
 * dot-separated values when it can and says "not comparable" when it cannot.
 */
function compareVersions(current: string, latest: string): "newer" | "older" | "equal" | "unknown" {
  if (current.trim() === latest.trim()) return "equal";
  const parse = (value: string) => value.split(/[.\-+]/).map((part) => (/^\d+$/.test(part) ? Number(part) : Number.NaN));
  const a = parse(current);
  const b = parse(latest);
  const length = Math.max(a.length, b.length);
  for (let i = 0; i < length; i += 1) {
    const left = a[i] ?? 0;
    const right = b[i] ?? 0;
    if (Number.isNaN(left) || Number.isNaN(right)) return "unknown";
    if (right > left) return "newer";
    if (right < left) return "older";
  }
  return "unknown";
}

export function AdminUpgrade() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [confirmExec, setConfirmExec] = useState<UpgradePlan | null>(null);
  const [selectedType, setSelectedType] = useState<string>("full");
  const [selectedComponents, setSelectedComponents] = useState<string[]>([]);

  const activeCheck = (plans: UpgradePlan[]) => plans.some((p) => !TERMINAL.includes(p.status));

  const versionsQ = useQuery({ queryKey: ["upgrade-versions"], queryFn: checkForUpgrades, retry: false });
  const plansQ = useQuery({
    queryKey: ["upgrade-plans"],
    queryFn: () => listUpgradePlans(50),
    retry: false,
    // An executing upgrade reports progress through these rows, so a plan that
    // is not terminal keeps the list polling. Without this the bar sat at
    // "deploying · 40%" indefinitely after the one invalidation on execute.
    refetchInterval: (query) => {
      const data = query.state.data as UpgradePlan[] | undefined;
      return data && activeCheck(data) ? 5000 : false;
    },
  });

  const invalidatePlans = () => { void qc.invalidateQueries({ queryKey: ["upgrade-plans"] }); };

  const createMut = useMutation({
    mutationFn: () => createUpgradePlan(selectedType, selectedComponents),
    onSuccess: () => { invalidatePlans(); toast({ tone: "success", title: "Upgrade plan created", message: "Nothing has changed yet — a plan is a proposal until you execute it." }); },
    onError: (error) => toast({ tone: "error", title: "Plan could not be created", message: errorMessage(error, "The upgrade service rejected the request.") }),
  });

  const executeMut = useMutation({
    mutationFn: (id: string) => executeUpgradePlan(id),
    onSuccess: (result) => {
      invalidatePlans();
      if (result && result.success === false) {
        toast({ tone: "error", title: "Upgrade did not complete", message: result.error || result.message || "The upgrade service reported a failure." });
        return;
      }
      toast({ tone: "success", title: "Upgrade started", message: "Progress updates below while the plan is running." });
    },
    onError: (error) => toast({ tone: "error", title: "Upgrade could not be started", message: errorMessage(error, "The upgrade service rejected the request.") }),
  });

  const cancelMut = useMutation({
    mutationFn: (id: string) => cancelUpgradePlan(id),
    onSuccess: () => { invalidatePlans(); toast({ tone: "success", title: "Cancellation requested" }); },
    onError: (error) => toast({ tone: "error", title: "Cancel failed", message: `${errorMessage(error, "The upgrade service rejected the request.")} The plan may still be running.` }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteUpgradePlan(id),
    onSuccess: () => { invalidatePlans(); toast({ tone: "success", title: "Plan history deleted" }); },
    onError: (error) => toast({ tone: "error", title: "Delete failed", message: errorMessage(error, "The upgrade service rejected the request.") }),
  });

  const versions: UpgradeVersionInfo[] = versionsQ.data ?? [];
  const plans: UpgradePlan[] = plansQ.data ?? [];
  const upgradable = versions.filter((v) => v.upgradable).length;
  const activePlans = plans.filter((p) => !TERMINAL.includes(p.status)).length;

  const componentsLabel = (plan: UpgradePlan) =>
    Array.isArray(plan.components) && plan.components.length > 0 ? plan.components.join(", ") : "all components";
  const canCreate = selectedType === "full" || selectedComponents.length > 0;

  const reload = () => { void versionsQ.refetch(); void plansQ.refetch(); };

  return (
    <AdminPageLayout className="space-y-6">
      <OfflineBanner onRetry={reload} />
      <SectionHeader
        sub="Plan a control-plane self-upgrade, review it, then execute it explicitly. A plan changes nothing on its own; only Execute starts work."
        status={<FreshnessBadge state={sourceState(versionsQ)} />}
        info={{
          title: "Platform Upgrade",
          triggerLabel: "About platform upgrade",
          description: "What this page can prove about versions, backups and rollback — and what it cannot.",
          sections: [
            {
              title: "Where 'latest' comes from",
              content: "The upgrade service reads each component's current and target version from an environment variable or a version file on this host — <component>.version and <component>.latest.version. It never contacts a registry or the network. A target is therefore a recorded string, not a verified release, and 'differs from target' is the strongest statement available here.",
            },
            {
              title: "Backup and rollback",
              content: "The backup is taken at the start of execution, not before you confirm, so nothing here can promise a restore point in advance. It needs DATABASE_URL in the API process and a working pg_dump; if either is missing the plan fails with 'backup failed' before any component changes. If a step fails afterwards, rollback is attempted automatically and its outcome is written to the plan — a rollback that itself fails is logged, not surfaced as a second error.",
            },
            {
              title: "Health is not verified",
              content: "The control plane's verify step calls a health probe that returns success without measuring anything, and the API says so explicitly (GET/POST /upgrade/plans/:id/verify reports 'component health verification is not implemented'). An upgrade marked completed has had no post-upgrade health check from this page. Check component health separately after every upgrade.",
            },
          ],
        }}
      />

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <AggregateTile
          format={(value) => `${value}`}
          kind="configured"
          label="Components reporting a version"
          noun="components are read from local version files"
          state={sourceState(versionsQ)}
          total={{ value: versions.length, reported: versions.length, total: COMPONENTS.length }}
        />
        <MetricTile
          context="Targets differ from the current version. A target can be older than the current value — the comparison is shown per component."
          kind="configured"
          label="Targets differ from current"
          state={sourceState(versionsQ)}
          value={versionsQ.data === undefined ? undefined : upgradable}
        />
        <MetricTile
          context={activePlans > 0 ? "Polling every 5s while a plan is not terminal." : "No plan is running."}
          kind="live"
          label="Active plans"
          state={sourceState(plansQ)}
          tone={activePlans > 0 ? "warn" : undefined}
          value={plansQ.data === undefined ? undefined : activePlans}
        />
        <MetricTile
          kind="derived"
          label="Plans on record"
          missingReason="Plan history could not be read"
          state={sourceState(plansQ)}
          value={plansQ.data === undefined ? undefined : plans.length}
        />
      </div>

      {/* Version check */}
      <Card>
        <CardHeader
          action={
            <div className="flex items-center gap-2">
              <FreshnessBadge state={sourceState(versionsQ)} />
              <Btn onClick={() => void versionsQ.refetch()} size="sm" tone="ghost" title="Re-read the version files on this host">
                <RefreshCw size={12} className={versionsQ.isFetching ? "animate-spin" : ""} /> Re-read versions
              </Btn>
            </div>
          }
          icon={ArrowUpCircle}
          title="Component versions"
        />
        {versionsQ.isPending ? <AdminLoadingState label="Reading version files…" /> : null}
        {versionsQ.isError ? (
          <div className="p-4">
            <AdminErrorState message={`${errorMessage(versionsQ.error, "The upgrade service did not respond")} No component version is known, so nothing below can be called current or up to date.`} retry={() => void versionsQ.refetch()} />
          </div>
        ) : null}
        {!versionsQ.isPending && !versionsQ.isError && versions.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={ArrowUpCircle}
              message="The upgrade service reported no component versions. It skips a component when its version file cannot be read or holds an empty, oversized or CRLF value — that is a missing reading, not a zero."
              title="No version file could be read"
            />
          </div>
        ) : null}
        {versions.length > 0 ? (
          <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-4">
            {versions.map((v) => {
              const order = compareVersions(v.current, v.latest);
              return (
                <div className="rounded-xl border border-line bg-overlay-subtle p-4" key={v.component}>
                  <div className="text-meta uppercase tracking-wider text-text-subtle">{v.component}</div>
                  <div className="mt-1 font-mono text-sm text-text">
                    <Reading reason="Current version not reported" value={v.current} />
                  </div>
                  {order === "equal" ? (
                    <Pill tone="neutral">matches recorded target</Pill>
                  ) : order === "older" ? (
                    <Pill tone="warn"><ArrowUpCircle size={10} /> target is older: <Reading reason="Target not reported" value={v.latest} /></Pill>
                  ) : (
                    <Pill tone="info"><ArrowUpCircle size={10} /> differs from target: <Reading reason="Target not reported" value={v.latest} /></Pill>
                  )}
                  <p className="mt-1 text-[11px] leading-5 text-text-muted">
                    Target read from this host, not from a registry.{order === "older" ? " Executing this would move the component backwards." : null}
                  </p>
                </div>
              );
            })}
          </div>
        ) : null}
      </Card>

      {/* Create plan */}
      <Card>
        <CardHeader icon={Play} title="Create upgrade plan" />
        <div className="space-y-3 p-4">
          <fieldset>
            <legend className="ui-label mb-1.5 block">Plan scope</legend>
            <div className="flex flex-wrap gap-2" role="group">
              {["full", ...COMPONENTS].map((t) => (
                <Btn
                  key={t}
                  onClick={() => setSelectedType(t)}
                  size="sm"
                  tone={selectedType === t ? "primary" : "ghost"}
                >
                  {t}
                </Btn>
              ))}
            </div>
          </fieldset>
          {selectedType !== "full" ? (
            <fieldset>
              <legend className="ui-label mb-1.5 block">Components</legend>
              <div className="flex flex-wrap gap-2">
                {COMPONENTS.map((c) => (
                  <label className="flex cursor-pointer items-center gap-1.5 rounded-md border border-line bg-overlay-subtle px-2 py-1 text-xs text-text" key={c}>
                    <input
                      checked={selectedComponents.includes(c)}
                      className="accent-[var(--brand)]"
                      onChange={(e) => setSelectedComponents((prev) => e.target.checked ? [...prev, c] : prev.filter((x) => x !== c))}
                      type="checkbox"
                    />
                    <span className="font-mono">{c}</span>
                  </label>
                ))}
              </div>
              {!canCreate ? <p className="mt-1 text-[11px] text-warn" role="status">Select at least one component, or choose the full scope.</p> : null}
            </fieldset>
          ) : null}
          <p className="text-[11px] leading-5 text-text-muted">
            The upgrade service accepts only these four names; a component it does not recognise is rejected rather than ignored.
          </p>
          {createMut.isError ? <AdminErrorState message={errorMessage(createMut.error, "The plan could not be created.")} /> : null}
          <Btn disabled={createMut.isPending || !canCreate} loading={createMut.isPending} onClick={() => createMut.mutate()}>
            <Play size={14} /> Create plan
          </Btn>
        </div>
      </Card>

      {/* Plans list */}
      <Card>
        <CardHeader
          action={<FreshnessBadge state={sourceState(plansQ)} />}
          icon={Clock3}
          title={plansQ.data === undefined ? "Upgrade plans" : `Upgrade plans · ${plans.length}`}
        />
        {plansQ.isPending ? <AdminLoadingState label="Loading plans…" /> : null}
        {plansQ.isError ? (
          <div className="p-4"><AdminErrorState message={`${errorMessage(plansQ.error, "Plan history could not be read.")} An active upgrade would still be running even though it is not listed here.`} retry={() => void plansQ.refetch()} /></div>
        ) : null}
        {!plansQ.isPending && !plansQ.isError && plans.length === 0 ? (
          <EmptyState icon={Clock3} message="Create a plan above, then execute it to upgrade the control plane." title="No upgrade plans" />
        ) : null}
        {plans.length > 0 ? (
          <AdminTable label="Upgrade plans">
            <AdminTHead>
              <AdminTh>Status</AdminTh>
              <AdminTh>Scope</AdminTh>
              <AdminTh>Versions</AdminTh>
              <AdminTh>Progress</AdminTh>
              <AdminTh>Restore point</AdminTh>
              <AdminTh>Created / finished</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {plans.map((p) => {
                const tone = planTone(p.status);
                const isActive = !TERMINAL.includes(p.status);
                const knownTotal = Number.isFinite(p.totalSteps) && p.totalSteps > 0;
                const pct = knownTotal ? Math.max(0, Math.min(100, Math.round((p.progress / p.totalSteps) * 100))) : null;
                return (
                  <AdminTr key={p.id}>
                    <AdminTd><Pill tone={tone}>{p.status.replace(/_/g, " ")}</Pill></AdminTd>
                    <AdminTd>
                      <p className="text-xs font-medium text-text">{p.type}</p>
                      <p className="text-[11px] text-text-subtle">components: {componentsLabel(p)}</p>
                    </AdminTd>
                    <AdminTd className="font-mono text-xs text-text-subtle">
                      <Reading reason="Origin version not recorded" value={p.fromVersion} /> → <Reading reason="Target version not recorded" value={p.toVersion} />
                    </AdminTd>
                    <AdminTd>
                      {knownTotal ? (
                        <div className="min-w-32">
                          <div className="h-1.5 overflow-hidden rounded-full bg-overlay-strong">
                            <div className="h-full rounded-full bg-brand transition-[width]" style={{ width: `${pct}%` }} />
                          </div>
                          <p className="mt-0.5 text-[11px] text-text-subtle">
                            {p.currentStep ? p.currentStep.replace(/_/g, " ") : "step not reported"} · {p.progress} of {p.totalSteps}
                          </p>
                        </div>
                      ) : (
                        <span className="text-[11px] text-text-subtle">
                          {isActive ? "Progress not reported by this plan" : "No steps recorded"}
                        </span>
                      )}
                      {p.error ? <p className="mt-1 max-w-prose text-[11px] text-danger">{p.error}</p> : null}
                    </AdminTd>
                    <AdminTd className="max-w-56 break-all font-mono text-[11px] text-text-subtle">
                      {p.backupPath ? p.backupPath : <Reading reason={isActive ? "Backup is taken when execution starts" : "No backup recorded"} value={undefined} />}
                    </AdminTd>
                    <AdminTd className="text-[11px] whitespace-nowrap text-text-subtle">
                      {formatDate(p.createdAt, "Creation time not reported")}
                      {p.completedAt ? ` · finished ${formatDate(p.completedAt, "unknown")}` : ""}
                    </AdminTd>
                    <AdminTd className="text-right whitespace-nowrap">
                      <div className="flex justify-end gap-1">
                        {p.status === "pending" ? (
                          <Btn onClick={() => setConfirmExec(p)} size="sm"><Play size={12} /> Execute</Btn>
                        ) : null}
                        {isActive ? (
                          <Btn
                            ariaLabel={`Cancel the ${p.type} upgrade plan`}
                            disabled={cancelMut.isPending}
                            loading={cancelMut.isPending && cancelMut.variables === p.id}
                            onClick={() => cancelMut.mutate(p.id)}
                            size="sm"
                            tone="danger"
                          >
                            <XCircle size={12} /> Cancel
                          </Btn>
                        ) : null}
                        {!isActive ? (
                          <Btn
                            ariaLabel={`Delete the ${p.type} upgrade plan record`}
                            onClick={() => {
                              void (async () => {
                                const ok = await confirm({
                                  title: "Delete this upgrade plan?",
                                  description: "This removes the control plane's record of a change it has already made, including its restore path and error history. The components themselves are untouched. This cannot be undone.",
                                  danger: true,
                                  confirmLabel: "Delete plan",
                                });
                                if (ok) deleteMut.mutate(p.id);
                              })();
                            }}
                            size="sm"
                            tone="ghost"
                          >
                            <Trash2 size={12} />
                          </Btn>
                        ) : null}
                      </div>
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        ) : null}
      </Card>

      {confirmExec && (
        <AdminConfirmDialog
          confirmLabel="Execute upgrade"
          destructive
          description={
            `Scope: ${confirmExec.type === "full" ? "all components" : componentsLabel(confirmExec)} · ` +
            `${confirmExec.fromVersion || "current version not recorded"} → ${confirmExec.toVersion || "target not recorded"}. ` +
            `A database backup is attempted as the first step of execution — it has not been taken or verified yet, and it requires DATABASE_URL in the API process and a working pg_dump. ` +
            `If the backup fails the plan stops before any component changes. If a later step fails, rollback is attempted automatically and needs that backup. ` +
            `Component health is not verified by the control plane after an upgrade: its verify step returns success without probing, so check health yourself.`
          }
          loading={executeMut.isPending}
          onCancel={() => setConfirmExec(null)}
          onConfirm={() => { executeMut.mutate(confirmExec.id, { onSettled: () => setConfirmExec(null) }); }}
          title="Execute upgrade plan?"
        />
      )}
      {renderConfirm()}
      <p className="flex items-center gap-2 text-[11px] leading-5 text-text-muted">
        <Database size={12} aria-hidden="true" />
        Restore points appear in the plan table once execution has created them. The upgrade service writes the path into the plan; this page never invents one.
      </p>
    </AdminPageLayout>
  );
}
