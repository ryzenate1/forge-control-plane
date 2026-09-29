"use client";
import { queryKeys } from "@/lib/api/query-keys";

import { useEffect, useMemo, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useRouter } from "next/navigation";
import {
  FileText, Layers, Plus,
  Power, RefreshCw, RotateCcw, Square,
  Terminal, Trash2,
} from "lucide-react";
import { fetchApps, startApp, stopApp, restartApp, deleteApp, typeLabel, type ApiApp, type AppStatus, type AppType } from "@/lib/api/apps";
import { AdminErrorState, AdminLoadingRows, AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { DeployStatusBadge } from "@/components/admin/AdminAppsShared";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { APP_TYPE_ICONS } from "@/lib/app-type-icons";
import { countLabel, errorMessage, isAvailable, sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";
import { Pagination } from "@/components/ui/primitives";

const POLL_INTERVAL_MS = 15_000;

/**
 * Which power controls a row may offer, and why the others are locked.
 *
 * `AppStatus` has eleven members and `mapApplication` deliberately yields `unknown`
 * when the control plane reported nothing. Previously this row only rendered a
 * button for `running`, `stopped` or `failed`, so an unmeasured app silently lost
 * every lifecycle affordance — an operator could not tell "cannot act" from "the
 * panel forgot to show me the button". All three controls now always render and
 * carry the reason they are disabled.
 */
type RowActions = { canStart: boolean; canStop: boolean; canRestart: boolean; reason: string };

function actionsFor(status: AppStatus): RowActions {
  switch (status) {
    case "running":
      return { canStart: false, canStop: true, canRestart: true, reason: "Already running" };
    case "idle":
      return { canStart: true, canStop: false, canRestart: false, reason: "Idle — nothing running to stop" };
    case "stopped":
      return { canStart: true, canStop: false, canRestart: false, reason: "Stopped — start it before restarting" };
    case "failed":
      return { canStart: true, canStop: false, canRestart: false, reason: "Failed — start attempts a new run" };
    case "starting":
    case "installing":
    case "deploying":
    case "restarting":
      return { canStart: false, canStop: false, canRestart: false, reason: `${status} — wait for the current operation to finish` };
    case "stopping":
      return { canStart: false, canStop: false, canRestart: false, reason: "stopping — wait for the current operation to finish" };
    case "pending":
      return { canStart: false, canStop: false, canRestart: false, reason: "Not yet scheduled — deploy it first from the application page" };
    default:
      return { canStart: false, canStop: false, canRestart: false, reason: "Lifecycle state not reported — open the application to resolve it" };
  }
}

export default function AdminAppsPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<string>("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const appsQuery = useQuery({
    queryKey: queryKeys.apps.lists(),
    queryFn: fetchApps,
    refetchInterval: POLL_INTERVAL_MS,
  });
  const apps = appsQuery.data ?? [];
  const appsReady = isAvailable(appsQuery);

  const startMut = useMutation({
    mutationFn: startApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() }),
    onError: (err) => toast({ tone: "error", title: "Failed to start app", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const stopMut = useMutation({
    mutationFn: stopApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() }),
    onError: (err) => toast({ tone: "error", title: "Failed to stop app", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const restartMut = useMutation({
    mutationFn: restartApp,
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() }),
    onError: (err) => toast({ tone: "error", title: "Failed to restart app", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const deleteMut = useMutation({
    mutationFn: deleteApp,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() });
      toast({ tone: "success", title: "Application deleted" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to delete app", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const types: AppType[] = ["image", "git", "compose", "game_server"];
  const filtering = Boolean(search.trim() || typeFilter);

  // Any filter change invalidates the current page slice, so go back to page 1
  // rather than landing on an empty page of a shorter result set.
  useEffect(() => {
    setPage(1);
  }, [search, typeFilter, pageSize]);

  const filtered = useMemo(() => {
    return apps.filter((app) => {
      if (search && !app.name.toLowerCase().includes(search.toLowerCase())) return false;
      if (typeFilter && app.type !== typeFilter) return false;
      return true;
    });
  }, [apps, search, typeFilter]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const current = Math.min(page, pageCount);
  const pageRows = filtered.slice((current - 1) * pageSize, current * pageSize);

  async function handleDelete(app: ApiApp) {
    const ok = await confirm({
      title: `Delete ${app.name}?`,
      description: "This removes the application definition and its stored configuration from the control plane. Deployments already running on a node are not stopped by this action. This cannot be undone.",
      confirmLabel: "Delete application",
      danger: true,
    });
    if (ok) deleteMut.mutate(app.id);
  }

  const totalLabel = filtering
    ? `${countLabel(filtered.length)} of ${countLabel(apps.length)} applications (filtered)`
    : `${countLabel(apps.length)} applications`;

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={sourceState(appsQuery, POLL_INTERVAL_MS)} />}
        info={adminPageGuides.applications}
        action={
          <>
            <Btn
              ariaLabel="Refresh applications"
              loading={appsQuery.isFetching}
              onClick={() => void appsQuery.refetch()}
              tone="ghost"
            >
              <RefreshCw size={14} /> Refresh
            </Btn>
            <Btn onClick={() => router.push("/admin/apps/new")} tone="primary">
              <Plus size={14} /> Create App
            </Btn>
          </>
        }
      />

      <Card>
        {/* The count is only asserted once the query has actually delivered a list;
            while it is in flight the header reads "Applications", not "0 applications". */}
        <CardHeader action={appsReady ? <span className="t-meta text-text-subtle">{totalLabel}</span> : null} icon={Layers} title="Applications" />
        <div className="flex flex-wrap items-end gap-3 p-4">
          <div className="min-w-[200px] flex-1">
            <Input label="Search applications" onChange={setSearch} placeholder="Search by name…" type="search" value={search} />
          </div>
          <label className="block">
            <span className="ui-label mb-1.5">Type</span>
            <select
              aria-label="Filter by application type"
              className="ui-input w-full cursor-pointer sm:w-48"
              onChange={(e) => setTypeFilter(e.target.value)}
              value={typeFilter}
            >
              <option value="">All types</option>
              {types.map((t) => (
                <option key={t} value={t}>{typeLabel(t)}</option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="ui-label mb-1.5">Rows</span>
            <select
              aria-label="Applications per page"
              className="ui-input w-full cursor-pointer sm:w-24"
              onChange={(e) => setPageSize(Number(e.target.value))}
              value={String(pageSize)}
            >
              {[10, 20, 50].map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
          </label>
          {filtering ? (
            <Btn ariaLabel="Clear filters" onClick={() => { setSearch(""); setTypeFilter(""); }} tone="subtle">
              Clear filters
            </Btn>
          ) : null}
        </div>

        {appsQuery.isPending ? (
          <AdminLoadingRows cols={6} label="Loading applications…" rows={5} />
        ) : appsQuery.isError ? (
          // Precedence matters here: a failed read must never fall through to the
          // empty state, which would tell the operator there are no applications
          // when the control plane was unreachable.
          <div className="p-4">
            <AdminErrorState
              message={`Could not load applications: ${errorMessage(appsQuery.error)}`}
              retry={() => void appsQuery.refetch()}
            />
          </div>
        ) : filtered.length === 0 ? (
          filtering ? (
            <EmptyState
              icon={Layers}
              message={`No applications match your search across ${apps.length} application${apps.length === 1 ? "" : "s"}.`}
              title="No matches"
            />
          ) : (
            <EmptyState
              icon={Layers}
              message="No applications have been created in this panel yet."
              title="No applications"
            />
          )
        ) : (
          <div className="overflow-x-auto">
            <table className="ui-table w-full">
              <caption className="sr-only">Applications with lifecycle controls</caption>
              <thead>
                <tr className="border-b border-line text-left">
                  <th className="ui-th px-4 py-3">Name</th>
                  <th className="ui-th px-4 py-3">Type</th>
                  <th className="ui-th px-4 py-3">Image/Version</th>
                  <th className="ui-th px-4 py-3">Status</th>
                  <th className="ui-th px-4 py-3">Created</th>
                  <th className="ui-th px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {pageRows.map((app) => {
                  const Icon = APP_TYPE_ICONS[app.type] ?? Layers;
                  const actions = actionsFor(app.status);
                  const rowBusy = (startMut.isPending || stopMut.isPending || restartMut.isPending) &&
                    (startMut.variables === app.id || stopMut.variables === app.id || restartMut.variables === app.id);
                  const unresolved = app.status === "unknown";

                  return (
                    <tr key={app.id} className="hover:bg-overlay-subtle">
                      <td className="px-4 py-3">
                        <button
                          className="flex items-center gap-2 text-left font-semibold text-text underline-offset-2 transition-colors hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                          onClick={() => router.push(`/admin/apps/${app.id}`)}
                          type="button"
                        >
                          <Icon aria-hidden="true" className="shrink-0 text-text-muted" size={14} />
                          {app.name}
                        </button>
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone="neutral">{typeLabel(app.type)}</Pill>
                      </td>
                      <td className="px-4 py-3 font-mono text-meta text-text-subtle">
                        {app.image ?? app.version ?? <span className="text-text-muted" title="Neither an image nor a version was reported">—</span>}
                      </td>
                      <td className="px-4 py-3">
                        <div className="space-y-1">
                          <DeployStatusBadge status={app.status} type="app" />
                          {unresolved ? (
                            <p className="max-w-prose text-meta text-text-muted">
                              Lifecycle not reported — power controls are disabled until the node reports state.
                            </p>
                          ) : null}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-meta text-text-subtle">
                        {app.createdAt ? formatDate(app.createdAt, "Unknown") : "Not reported"}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-1">
                          <Btn
                            ariaLabel={`Start ${app.name}`}
                            disabled={!actions.canStart || rowBusy}
                            loading={startMut.isPending && startMut.variables === app.id}
                            onClick={() => startMut.mutate(app.id)}
                            title={actions.canStart ? `Start ${app.name}` : `Start unavailable — ${actions.reason}`}
                            tone="success"
                          >
                            <Power size={12} />
                          </Btn>
                          <Btn
                            ariaLabel={`Stop ${app.name}`}
                            disabled={!actions.canStop || rowBusy}
                            loading={stopMut.isPending && stopMut.variables === app.id}
                            onClick={() => stopMut.mutate(app.id)}
                            title={actions.canStop ? `Stop ${app.name}` : `Stop unavailable — ${actions.reason}`}
                            tone="ghost"
                          >
                            <Square size={12} />
                          </Btn>
                          <Btn
                            ariaLabel={`Restart ${app.name}`}
                            disabled={!actions.canRestart || rowBusy}
                            loading={restartMut.isPending && restartMut.variables === app.id}
                            onClick={() => restartMut.mutate(app.id)}
                            title={actions.canRestart ? `Restart ${app.name}` : `Restart unavailable — ${actions.reason}`}
                            tone="ghost"
                          >
                            <RotateCcw size={12} />
                          </Btn>
                          <Btn
                            ariaLabel={`View logs for ${app.name}`}
                            onClick={() => router.push(`/admin/apps/${app.id}?tab=logs`)}
                            title={`Logs for ${app.name}`}
                            tone="ghost"
                          >
                            <FileText size={12} />
                          </Btn>
                          <Btn
                            ariaLabel={`Open console for ${app.name}`}
                            onClick={() => router.push(`/admin/apps/${app.id}?tab=console`)}
                            title={`Console for ${app.name}`}
                            tone="ghost"
                          >
                            <Terminal size={12} />
                          </Btn>
                          <Btn
                            ariaLabel={`Delete ${app.name}`}
                            disabled={rowBusy}
                            onClick={() => void handleDelete(app)}
                            title={`Delete ${app.name}`}
                            tone="danger"
                          >
                            <Trash2 size={12} />
                          </Btn>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <div className="px-4 pb-4">
              <Pagination label="Application pagination" onPageChange={setPage} page={current} pageCount={pageCount} />
            </div>
          </div>
        )}
      </Card>

      {deleteMut.isError ? (
        <p className="ui-alert ui-alert-danger" role="alert">
          Could not delete the application: {errorMessage(deleteMut.error)}
        </p>
      ) : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
