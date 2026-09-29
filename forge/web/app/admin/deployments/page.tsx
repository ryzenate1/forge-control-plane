"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { History, Plus } from "lucide-react";
import { fetchJSON, unwrapList } from "@/lib/api";
import type { Deployment } from "@/lib/api/deployments";
import { deploymentStatusTone } from "@/lib/api/status";
import { sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminToolbar,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { errorMessage, formatDate } from "@/lib/utils";

/**
 * One list, one endpoint.
 *
 * `GET /admin/deployments` is the deployment-service projection (see
 * `forge/api/internal/services/deployment/service.go`: `Deployment{}`). This
 * page used to fetch the same route twice, type it as two different shapes and
 * render two tables — "Server Deployments" and "App Deployments" — so an
 * operator reading "12 / 12" was reading the same 12 records twice, and the
 * App half rendered fields the route never sends (`revision`, `trigger`,
 * `commit`, `duration`) as `#undefined`, empty chips and a fake "—" duration.
 * Status colour now comes from `deploymentStatusTone` alone; the page-local
 * colour maps are gone.
 */

const POLL_MS = 15_000;

/** `_` is on the wire (`in_progress`); spaces are what an operator reads. */
function humanToken(value: string): string {
  return value.replace(/_/g, " ");
}

/** Strategies are hyphenated on the wire (`blue-green`); older rows used `_`. */
function strategyLabel(value: string): string {
  return value.replace(/_/g, "-");
}

/**
 * An absent timestamp is "not reported", not "Never": `completedAt` is omitted
 * for every deployment still in flight, so the shared formatter's default
 * fallback would read as a claim about a release that has not finished.
 */
function timeOrDash(value?: string | null): string {
  return value ? formatDate(value) : "—";
}

function readFilter(value: string | null): string {
  return value ?? "";
}

export default function AdminDeploymentsPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // Filters live in the URL so a filtered view survives refresh and can be shared.
  const search = readFilter(searchParams.get("q"));
  const statusFilter = readFilter(searchParams.get("status"));
  const strategyFilter = readFilter(searchParams.get("strategy"));

  const setFilter = (key: "q" | "status" | "strategy", value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (value) params.set(key, value);
    else params.delete(key);
    const query = params.toString();
    router.replace(query ? `/admin/deployments?${query}` : "/admin/deployments", { scroll: false });
  };

  const deploymentsQuery = useQuery({
    queryKey: ["admin", "deployments"],
    queryFn: async () =>
      unwrapList<Deployment>(await fetchJSON<{ data: Deployment[] } | Deployment[]>("/admin/deployments")),
    refetchInterval: POLL_MS,
  });

  const deployments = useMemo(() => deploymentsQuery.data ?? [], [deploymentsQuery.data]);
  const hasData = deploymentsQuery.data !== undefined;

  // Filter options are derived from what the platform actually reports rather
  // than a hard-coded vocabulary — the status set in the deployment service is
  // wider than any list typed into this page, and an invented option is a
  // filter that can never match.
  const statusOptions = useMemo(
    () => Array.from(new Set(deployments.map((d) => d.status).filter(Boolean))).sort(),
    [deployments],
  );
  const strategyOptions = useMemo(
    () => Array.from(new Set(deployments.map((d) => d.strategy).filter(Boolean))).sort(),
    [deployments],
  );

  const filtered = useMemo(() => {
    const needle = search.toLowerCase();
    return deployments.filter((d) => {
      if (needle) {
        const haystack = `${d.serverId ?? ""} ${d.image ?? ""}`.toLowerCase();
        if (!haystack.includes(needle)) return false;
      }
      if (statusFilter && d.status !== statusFilter) return false;
      if (strategyFilter && d.strategy !== strategyFilter) return false;
      return true;
    });
  }, [deployments, search, statusFilter, strategyFilter]);

  const filtersActive = Boolean(search || statusFilter || strategyFilter);
  const freshness = sourceState(deploymentsQuery, POLL_MS);

  const statusCounts = useMemo(() => {
    if (!hasData) return [];
    const counts = new Map<string, number>();
    for (const d of deployments) {
      if (!d.status) continue;
      counts.set(d.status, (counts.get(d.status) ?? 0) + 1);
    }
    return [...counts.entries()].sort((a, b) => b[1] - a[1]);
  }, [deployments, hasData]);

  return (
    <AdminPageLayout>
      <AdminPageHeader
        status={<FreshnessBadge state={freshness} />}
        action={
          <div className="flex items-center gap-2">
            <Btn tone="ghost" onClick={() => router.push("/admin/deployments/history")}>
              <History aria-hidden="true" size={14} /> History
            </Btn>
            <Btn tone="primary" onClick={() => router.push("/admin/deployments/new")}>
              <Plus aria-hidden="true" size={14} /> New Deployment
            </Btn>
          </div>
        }
      />

      <AdminSection
        title="Deployments"
        description="Releases the deployment service has run across your servers. Select a row to open its steps, revisions and rollback controls."
        action={
          hasData ? (
            <Pill tone="neutral">{`${deployments.length.toLocaleString()} recorded`}</Pill>
          ) : (
            <Pill tone="unknown">Count not loaded</Pill>
          )
        }
      >
        <Card>
          <AdminToolbar className="p-4">
            <Input
              label="Search"
              placeholder="Server ID or image"
              value={search}
              onChange={(v) => setFilter("q", v)}
            />
            <AdminSelect
              label="Status"
              value={statusFilter}
              onChange={(v) => setFilter("status", v)}
              placeholder="All statuses"
              options={statusOptions.map((s) => ({ value: s, label: humanToken(s) }))}
            />
            <AdminSelect
              label="Strategy"
              value={strategyFilter}
              onChange={(v) => setFilter("strategy", v)}
              placeholder="All strategies"
              options={strategyOptions.map((s) => ({ value: s, label: strategyLabel(s) }))}
            />
          </AdminToolbar>

          {statusCounts.length > 0 && !filtersActive ? (
            <div className="flex flex-wrap gap-1.5 border-y border-line bg-overlay-subtle px-4 py-2">
              {statusCounts.map(([status, count]) => (
                <Pill key={status} tone={deploymentStatusTone(status)}>
                  {`${humanToken(status)}: ${count.toLocaleString()}`}
                </Pill>
              ))}
            </div>
          ) : null}

          {deploymentsQuery.isPending ? (
            <AdminLoadingRows cols={6} rows={5} label="Loading deployments…" />
          ) : deploymentsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Deployments could not be loaded: ${errorMessage(deploymentsQuery.error)}`}
                retry={() => void deploymentsQuery.refetch()}
              />
            </div>
          ) : deployments.length === 0 ? (
            <EmptyState
              icon={History}
              title="No deployments recorded"
              message="No deployment has been started for any server yet. Create one with New Deployment."
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={History}
              title="No deployments match"
              message="No deployment matches the current search, status or strategy filter. Clear the filters above to see all records."
            />
          ) : (
            <AdminTable label="Deployments">
              <AdminTHead>
                <AdminTh>Server</AdminTh>
                <AdminTh>Image</AdminTh>
                <AdminTh>Strategy</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Active target</AdminTh>
                <AdminTh>Progress</AdminTh>
                <AdminTh>Created</AdminTh>
                <AdminTh>Completed</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {filtered.map((dep) => (
                  <AdminTr
                    key={dep.id}
                    onClick={() => router.push(`/admin/deployments/${encodeURIComponent(dep.id)}`)}
                  >
                    <AdminTd className="font-mono text-meta">{dep.serverId || "—"}</AdminTd>
                    <AdminTd className="max-w-64 truncate font-mono text-meta" title={dep.image}>
                      {dep.image || "—"}
                    </AdminTd>
                    <AdminTd>
                      <Pill tone="neutral">{strategyLabel(dep.strategy) || "—"}</Pill>
                    </AdminTd>
                    <AdminTd>
                      <Pill tone={deploymentStatusTone(dep.status)}>
                        {humanToken(dep.status) || "unknown"}
                      </Pill>
                    </AdminTd>
                    <AdminTd className="text-meta">
                      {/* `activeTarget` is omitted when the service never recorded one. */}
                      {dep.activeTarget ? strategyLabel(dep.activeTarget) : "Not reported"}
                    </AdminTd>
                    <AdminTd className="font-mono text-meta">
                      {typeof dep.progressPct === "number" && Number.isFinite(dep.progressPct)
                        ? `${dep.progressPct}%`
                        : "—"}
                    </AdminTd>
                    <AdminTd className="text-meta">{timeOrDash(dep.createdAt)}</AdminTd>
                    <AdminTd className="text-meta">{timeOrDash(dep.completedAt)}</AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      </AdminSection>
    </AdminPageLayout>
  );
}
