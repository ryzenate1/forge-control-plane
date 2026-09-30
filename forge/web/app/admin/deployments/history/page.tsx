"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { History, RotateCcw } from "lucide-react";
import { fetchJSON, unwrapList } from "@/lib/api";
import type { DeploymentRecord } from "@/lib/api/deployments";
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
  Card,
  EmptyState,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { errorMessage, formatDate } from "@/lib/utils";

/**
 * Deployment history is the `store.DeploymentRecord` projection reported by the
 * host agent (`GET /admin/deployment-history`), a different record set from the
 * deployment-service releases on `/admin/deployments`. Statuses here are a free
 * string in Go (`internal/store/store_deployment_history.go:12`), so the filter
 * options are read from the data and every status is coloured by the shared
 * table — `deploymentStatusTone` knows `pending`/`running`/`done`/`completed`/
 * `error`/`failed`/`cancelled` and falls through to `unknown` for anything else,
 * which is why the page-local colour map that used to sit here is gone.
 */

const POLL_MS = 10_000;

function humanToken(value: string): string {
  return value.replace(/_/g, " ");
}

function timeOrDash(value?: string | null): string {
  return value ? formatDate(value) : "—";
}

export default function AdminDeploymentHistoryPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [search, setSearch] = useState(searchParams.get("q") ?? "");

  const statusFilter = searchParams.get("status") ?? "";
  const setStatusFilter = (value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (value) params.set("status", value);
    else params.delete("status");
    const query = params.toString();
    router.replace(query ? `/admin/deployments/history?${query}` : "/admin/deployments/history", {
      scroll: false,
    });
  };

  const historyQuery = useQuery({
    queryKey: ["admin", "deployment-history"],
    queryFn: async () =>
      unwrapList<DeploymentRecord>(
        await fetchJSON<{ data: DeploymentRecord[] } | DeploymentRecord[]>("/admin/deployment-history"),
      ),
    refetchInterval: POLL_MS,
  });

  const records = useMemo(() => historyQuery.data ?? [], [historyQuery.data]);
  const hasData = historyQuery.data !== undefined;

  const statusOptions = useMemo(
    () => Array.from(new Set(records.map((r) => r.status).filter(Boolean))).sort(),
    [records],
  );

  const filtered = useMemo(() => {
    const needle = search.toLowerCase();
    return records.filter((d) => {
      if (needle) {
        const haystack = `${d.serverId ?? ""} ${d.commitHash ?? ""}`.toLowerCase();
        if (!haystack.includes(needle)) return false;
      }
      if (statusFilter && d.status !== statusFilter) return false;
      return true;
    });
  }, [records, search, statusFilter]);

  const filtersActive = Boolean(search || statusFilter);

  return (
    <AdminPageLayout>
      <AdminPageHeader
        // Not a registry row: this route renders a different record set from its
        // parent, so the title has to be said here. See "Needs central change".
        title="Deployment History"
        status={<FreshnessBadge state={sourceState(historyQuery, POLL_MS)} />}
        backAction={() => router.push("/admin/deployments")}
        backLabel="Deployments"
      />

      <AdminSection
        title="Records"
        description="Deployment records reported by the host agent, newest read first."
        action={
          hasData ? (
            <Pill tone="neutral">{`${records.length.toLocaleString()} records`}</Pill>
          ) : (
            <Pill tone="unknown">Count not loaded</Pill>
          )
        }
      >
        <Card>
          <AdminToolbar className="p-4">
            <Input
              label="Search"
              placeholder="Server ID or commit hash"
              value={search}
              onChange={setSearch}
            />
            <AdminSelect
              label="Status"
              value={statusFilter}
              onChange={setStatusFilter}
              placeholder="All statuses"
              options={statusOptions.map((s) => ({ value: s, label: humanToken(s) }))}
            />
          </AdminToolbar>

          {historyQuery.isPending ? (
            <AdminLoadingRows cols={5} rows={5} label="Loading deployment history…" />
          ) : historyQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Deployment history could not be loaded: ${errorMessage(historyQuery.error)}`}
                retry={() => void historyQuery.refetch()}
              />
            </div>
          ) : records.length === 0 ? (
            <EmptyState
              icon={History}
              title="No deployment records"
              message="The host agent has not reported a deployment for any server yet."
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={History}
              title="No records match"
              message="No deployment record matches the current search or status filter."
            />
          ) : (
            <AdminTable label="Deployment history">
              <AdminTHead>
                <AdminTh>Server</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Commit</AdminTh>
                <AdminTh>Message</AdminTh>
                <AdminTh>Started</AdminTh>
                <AdminTh>Finished</AdminTh>
                <AdminTh>Error</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {filtered.map((dep) => (
                  <AdminTr key={dep.id}>
                    <AdminTd className="font-mono text-meta">{dep.serverId || "—"}</AdminTd>
                    <AdminTd>
                      <Pill tone={deploymentStatusTone(dep.status)}>{humanToken(dep.status) || "unknown"}</Pill>
                    </AdminTd>
                    <AdminTd className="font-mono text-meta">
                      {dep.commitHash ? dep.commitHash.slice(0, 7) : "Not reported"}
                    </AdminTd>
                    <AdminTd className="max-w-72 break-words text-meta">
                      {dep.commitMessage || "—"}
                    </AdminTd>
                    <AdminTd className="text-meta">{timeOrDash(dep.startedAt)}</AdminTd>
                    <AdminTd className="text-meta">{timeOrDash(dep.finishedAt)}</AdminTd>
                    <AdminTd>
                      {/* The failure reason used to exist only as a hover `title` on an
                          icon — mouse-only, and invisible to a keyboard or screen-reader
                          operator. It is the cell's content now. */}
                      <div className="flex flex-col gap-1">
                        {dep.errorMessage ? (
                          <span className="max-w-72 break-words text-meta">{dep.errorMessage}</span>
                        ) : (
                          <span className="text-meta text-text-muted">None reported</span>
                        )}
                        {dep.rollbackId ? (
                          <span className="inline-flex items-center gap-1.5 text-meta text-text-subtle">
                            <RotateCcw aria-hidden="true" size={12} /> Rolled back
                          </span>
                        ) : null}
                      </div>
                    </AdminTd>
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
