"use client";

import { BellRing } from "lucide-react";
import { Card, CardHeader, SectionHeader, AdminErrorState, AdminLoadingState, Pill } from "@/components/admin/admin-ui";
import { useMonitoringSummaryQuery, summaryIsTrustworthy, useNodesQuery, useServersQuery } from "@/lib/admin/telemetry";
import { SystemMetrics } from "@/components/monitoring/system-metrics";
import { NodeList } from "@/components/monitoring/node-list";
import { HealthStatusGauge } from "@/components/health/health-status-gauge";
import { HealthSummary } from "@/components/health/health-summary";
import { OfflineBanner } from "@/components/shared/states-offline";
import { DegradedBanner, ApiUnavailableState, BeaconUnavailableState, RetryingBanner } from "@/components/shared/states-connectivity";
import { ApiError } from "@/lib/api/http";

export default function ConsoleHealthPage() {
  const { data, isLoading, isError, error, refetch, isFetching } = useMonitoringSummaryQuery();
  const nodesQuery = useNodesQuery();
  const serversQuery = useServersQuery();
  const trustworthy = summaryIsTrustworthy(data, {
    nodeCount: nodesQuery.data?.length,
    serverCount: serversQuery.data?.length,
  });
  const displayData = trustworthy ? data : undefined;

  const isDegraded = !isLoading && !isError && displayData?.recentHealthChecks?.some((check) => check.reachable === false || check.status === "degraded" || check.status === "critical");

  if (isError) {
    const kind = error instanceof ApiError ? error.status : 0;
    if (kind === 0 || kind === 503) {
      return <ApiUnavailableState message={error instanceof Error ? error.message : "Health data is unavailable"} onRetry={() => void refetch()} />;
    }
    if (kind === 502 || kind === 504) {
      return <BeaconUnavailableState message={error instanceof Error ? error.message : "Beacon daemon is unreachable"} onRetry={() => void refetch()} />;
    }
    return (
      <AdminErrorState
        message={error instanceof Error ? `Health data is unavailable: ${error.message}` : "Health data is unavailable right now. Please try again."}
        retry={() => void refetch()}
      />
    );
  }

  return (
    <div className="space-y-6">
      <OfflineBanner onRetry={() => void refetch()} />
      {isFetching && !isLoading ? <RetryingBanner label="Refreshing health data…" /> : null}
      {isDegraded ? <DegradedBanner title="Degraded performance" message="One or more health checks are failing or degraded." onRetry={() => void refetch()} /> : null}
      <SectionHeader
        title="Health"
        sub="Node status, endpoint checks and system resource usage across the fleet"
        action={<Pill tone={displayData?.unacknowledgedAlerts == null ? "neutral" : displayData.unacknowledgedAlerts > 0 ? "red" : "green"}>{trustworthy ? `${displayData?.unacknowledgedAlerts ?? "Unreported"} unacknowledged alerts` : "Summary unreported"}</Pill>}
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <HealthStatusGauge checks={displayData?.recentHealthChecks ?? []} className="lg:col-span-2" />
        <Card>
          <CardHeader title="Alert Queue" icon={BellRing} />
          <div className="p-4 text-sm text-slate-300">
            {!trustworthy
              ? "Alert count is unavailable — observability not reporting."
              : displayData?.unacknowledgedAlerts == null
              ? "Alert count is unavailable."
              : displayData.unacknowledgedAlerts > 0
              ? `${displayData.unacknowledgedAlerts} unacknowledged alert${displayData.unacknowledgedAlerts === 1 ? "" : "s"} require attention.`
              : "All alerts are acknowledged. No action needed right now."}
          </div>
        </Card>
      </div>

      {isLoading ? (
        <AdminLoadingState label="Loading monitoring summary…" />
      ) : (
        <HealthSummary checks={displayData?.recentHealthChecks ?? []} />
      )}

      <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
        <SystemMetrics />
        <NodeList />
      </div>
    </div>
  );
}
