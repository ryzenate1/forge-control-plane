/**
 * Canonical data layer for the admin operational experiences
 * (Overview / Monitoring / Health).
 *
 * Why this exists: those three pages read the same sources. When each page
 * owned its own `useQuery` call they disagreed on cache keys (`["nodes"]` vs
 * `["nodes","all"]`, `["servers"]` vs `["servers","all"]`, two shapes of
 * `admin-activity`), which means one page can render a reading the others have
 * already refreshed — stale data presented as live. Every operational read in
 * this feature area goes through a hook here, so the keys are standardized by
 * construction.
 *
 * The second job of this module is truthfulness. The rules, applied uniformly:
 *
 *   - A value that was not reported is `undefined`, never `0`. `0` is a real
 *     reading and must only appear when the source actually reported zero.
 *   - A query that failed is an error, not an empty dataset.
 *   - A 403 is "restricted", not "unavailable" and not "zero".
 *   - A reading older than twice its refresh interval is `stale`, and stale is
 *     not healthy.
 *
 * No JSX lives here; the presentational half is `components/admin/telemetry-ui.tsx`.
 */

import { useCallback, useEffect } from "react";
import { useQuery, useQueryClient, type QueryClient, type UseQueryResult } from "@tanstack/react-query";

import {
  fetchAdminActivity,
  fetchAdminAudit,
  fetchAllNodes,
  fetchAllServers,
  fetchHealthStatus,
  fetchRecoveryPlans,
  fetchReservations,
} from "@/lib/api";
import { queryKeys } from "@/lib/api/query-keys";
import { ApiError } from "@/lib/api/http";
import {
  getNodeMetrics,
  getSystemInfo,
  metricWindow,
  type MetricPeriod,
  type NodeMetrics,
  type SystemInfo,
} from "@/lib/api/monitoring";
import {
  fetchHealthCheckMetrics,
  listUnhealthyTargets,
  type HealthCheckMetrics,
} from "@/lib/api/health-checks";
import type { ApiHealthCheck, ApiHealthReport, ApiNode, ApiServer } from "@/lib/api/types";
import type { ForgeTone } from "@/components/ui/forge/status";

/* ------------------------------------------------------------------ *
 * Refresh cadence
 * ------------------------------------------------------------------ */

/**
 * One cadence table for the whole feature area. `staleAfter` is derived from
 * the interval rather than configured separately so a reading can never be
 * declared fresh longer than the page's own refresh promise.
 */
export const REFRESH = {
  /** Live host telemetry — the fastest-moving source we have. */
  telemetry: 15_000,
  /** Control-plane diagnostics. */
  health: 30_000,
  /** Inventory (nodes, servers). Changes on operator action, not continuously. */
  inventory: 30_000,
  /** Historical metric series. */
  history: 60_000,
  /** Event feeds. */
  activity: 60_000,
} as const;

/* ------------------------------------------------------------------ *
 * Source state
 * ------------------------------------------------------------------ */

export type SourceStatus = "loading" | "error" | "restricted" | "ready";

export type SourceState = {
  status: SourceStatus;
  /** `Date.now()`-comparable timestamp of the last successful fetch, if any. */
  updatedAt: number | null;
  /**
   * True when the last successful reading is older than twice its refresh
   * interval. A stale reading is still shown — hiding it would lose
   * information — but it must never be presented as current.
   */
  stale: boolean;
  /** Human-readable failure reason. Only set when `status` is error/restricted. */
  message?: string;
  /** True while a background refetch is in flight over existing data. */
  refreshing: boolean;
};

/** A 403 from the control plane means the caller lacks the scope, not that the subsystem is down. */
export function isPermissionError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 403;
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return "Request failed";
}

/**
 * Collapse a react-query result into the four states the UI distinguishes.
 * `intervalMs` should be the same value passed as `refetchInterval` — that is
 * what makes the staleness verdict meaningful rather than arbitrary.
 */
export function sourceState(
  query: Pick<
    UseQueryResult<unknown>,
    "isPending" | "isError" | "error" | "data" | "dataUpdatedAt" | "isFetching"
  >,
  intervalMs?: number,
): SourceState {
  const updatedAt = query.dataUpdatedAt > 0 ? query.dataUpdatedAt : null;
  const refreshing = query.isFetching && query.data !== undefined;

  if (query.isError) {
    const restricted = isPermissionError(query.error);
    return {
      status: restricted ? "restricted" : "error",
      updatedAt,
      stale: false,
      message: restricted ? "Permission restricted" : errorMessage(query.error),
      refreshing,
    };
  }

  if (query.isPending || query.data === undefined) {
    return { status: "loading", updatedAt, stale: false, refreshing };
  }

  const stale = intervalMs != null && updatedAt != null && Date.now() - updatedAt > intervalMs * 2;
  return { status: "ready", updatedAt, stale, refreshing };
}

/** True only when a query has actually delivered data we may read. */
export function isAvailable(query: Pick<UseQueryResult<unknown>, "isPending" | "isError" | "data">): boolean {
  return !query.isPending && !query.isError && query.data !== undefined;
}

/** Worst state across several sources, for a section header that summarises them. */
export function worstSourceState(states: SourceState[]): SourceState {
  const rank: Record<SourceStatus, number> = { error: 3, restricted: 2, loading: 1, ready: 0 };
  let worst = states[0] ?? { status: "ready" as SourceStatus, updatedAt: null, stale: false, refreshing: false };
  for (const state of states) {
    if (rank[state.status] > rank[worst.status]) worst = state;
  }
  const updatedAt = states.reduce<number | null>(
    (oldest, state) =>
      state.updatedAt == null ? oldest : oldest == null ? state.updatedAt : Math.min(oldest, state.updatedAt),
    null,
  );
  return {
    ...worst,
    updatedAt,
    stale: states.some((state) => state.stale),
    refreshing: states.some((state) => state.refreshing),
  };
}

/* ------------------------------------------------------------------ *
 * Partial-result reporting
 * ------------------------------------------------------------------ */

/**
 * A fleet-wide reading assembled from N per-node requests, some of which may
 * have failed.
 *
 * The previous implementation did `getNodeMetrics(...).catch(() => [])`, which
 * renders a partial fleet as if it were the whole fleet. Callers get the
 * failures here so they can say so.
 */
export type PartialFleet<T> = {
  rows: T[];
  /** Node ids whose request failed. Empty when the fan-out was complete. */
  failedNodeIds: string[];
  /** Node ids we deliberately did not request (fan-out cap). */
  skippedNodeIds: string[];
};

/**
 * Derived totals that distinguish "N of M reported" from "N".
 *
 * `reported` is how many records carried the field at all; `total` is how many
 * records exist. When `reported < total` the sum is a floor, not a total, and
 * the UI must label it as such.
 */
export type ReportedTotal = {
  value: number | undefined;
  reported: number;
  total: number;
};

export function reportedTotal<T>(records: T[], read: (record: T) => number | null | undefined): ReportedTotal {
  let value = 0;
  let reported = 0;
  for (const record of records) {
    const raw = read(record);
    if (typeof raw === "number" && Number.isFinite(raw)) {
      value += raw;
      reported += 1;
    }
  }
  return { value: reported > 0 ? value : undefined, reported, total: records.length };
}

/** True when the sum covers only part of the population and must be read as a floor. */
export function isPartial(total: ReportedTotal): boolean {
  return total.reported < total.total;
}

/* ------------------------------------------------------------------ *
 * Formatters — `undefined` in, `undefined` out. Never 0.
 * ------------------------------------------------------------------ */

function finite(value: number | null | undefined): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

/** Mebibytes → human string. `undefined` when not reported, so callers print "Not reported". */
export function mbLabel(value: number | null | undefined): string | undefined {
  const mb = finite(value);
  if (mb === undefined) return undefined;
  if (mb >= 1024 * 1024) return `${(mb / (1024 * 1024)).toFixed(2)} TiB`;
  if (mb >= 1024) return `${(mb / 1024).toFixed(mb >= 10240 ? 0 : 1)} GiB`;
  return `${Math.round(mb)} MiB`;
}

export function bytesLabel(value: number | null | undefined): string | undefined {
  const bytes = finite(value);
  if (bytes === undefined) return undefined;
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let scaled = bytes;
  let unit = 0;
  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024;
    unit += 1;
  }
  return `${scaled >= 100 || unit === 0 ? Math.round(scaled) : scaled.toFixed(1)} ${units[unit]}`;
}

export function percentLabel(value: number | null | undefined, digits = 0): string | undefined {
  const percent = finite(value);
  if (percent === undefined) return undefined;
  return `${percent.toFixed(digits)}%`;
}

export function secondsLabel(value: number | null | undefined): string | undefined {
  const seconds = finite(value);
  if (seconds === undefined) return undefined;
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  return `${Math.floor(hours / 24)}d ${hours % 24}h`;
}

export function countLabel(value: number | null | undefined): string | undefined {
  const count = finite(value);
  if (count === undefined) return undefined;
  return count.toLocaleString();
}

/** Latency in ms. Reported-zero is legitimate (sub-millisecond), so 0 formats as "0 ms". */
export function latencyLabel(value: number | null | undefined): string | undefined {
  const ms = finite(value);
  if (ms === undefined) return undefined;
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)} s`;
  return `${ms < 10 ? ms.toFixed(1) : Math.round(ms)} ms`;
}

/** Relative age of a timestamp. `undefined` for absent or unparseable input. */
export function relativeTime(value: string | number | null | undefined, now = Date.now()): string | undefined {
  if (value == null) return undefined;
  const at = typeof value === "number" ? value : Date.parse(value);
  if (!Number.isFinite(at)) return undefined;
  const deltaSeconds = Math.round((now - at) / 1000);
  if (deltaSeconds < 0) {
    const ahead = secondsLabel(Math.abs(deltaSeconds));
    return ahead ? `in ${ahead}` : undefined;
  }
  if (deltaSeconds < 10) return "just now";
  const label = secondsLabel(deltaSeconds);
  return label ? `${label} ago` : undefined;
}

/** Absolute clock time for tooltips and table cells. */
export function absoluteTime(value: string | number | null | undefined): string | undefined {
  if (value == null) return undefined;
  const at = typeof value === "number" ? value : Date.parse(value);
  if (!Number.isFinite(at)) return undefined;
  return new Date(at).toLocaleString();
}

/* ------------------------------------------------------------------ *
 * Node status semantics
 * ------------------------------------------------------------------ */

/**
 * Telemetry verdicts speak the one Forge status vocabulary
 * (`components/ui/forge/status.ts`) rather than a private set of colour words,
 * so a verdict produced here and a chip rendered elsewhere cannot disagree.
 *
 * Note what this buys: the states below that carry `known: false` /
 * `expected: false` now resolve to `unknown`, which renders grey *with a dashed
 * edge*. Previously they were `neutral` — visually identical to a node an
 * operator had deliberately stopped. "We have no reading" and "it is off on
 * purpose" are different facts and now look different.
 */
export type StatusTone = ForgeTone;

export type NodeStatusVerdict = {
  label: string;
  tone: StatusTone;
  /** True when the node is offline because an operator asked it to be. */
  expected: boolean;
};

/**
 * One reading of node status for every surface in this feature area.
 *
 * Deliberate rules:
 *   - Desired state wins the label when it is maintenance/draining, because an
 *     offline node under maintenance is not an incident.
 *   - `"unknown"` and a missing status stay Unknown. They are not "offline"
 *     (we have not observed it down) and not "online" — and they are not
 *     `neutral` either, which is reserved for a node that is genuinely idle.
 */
export function nodeStatus(node: ApiNode): NodeStatusVerdict {
  if (node.maintenanceMode || node.desiredState === "maintenance") {
    return { label: "Maintenance", tone: "info", expected: true };
  }
  if (node.draining || node.desiredState === "draining") {
    return { label: "Draining", tone: "warn", expected: true };
  }

  const observed = (node.actualState ?? node.status ?? "").toLowerCase();
  switch (observed) {
    case "online":
      return { label: "Online", tone: "ok", expected: false };
    case "degraded":
      return { label: "Degraded", tone: "warn", expected: false };
    case "reconciling":
      return { label: "Reconciling", tone: "info", expected: false };
    case "offline":
      return { label: "Offline", tone: "danger", expected: false };
    case "":
    case "unknown":
      return { label: "Unknown", tone: "unknown", expected: false };
    default:
      // A state the control plane reports but this UI does not know how to
      // interpret. Surfacing the raw word is useful; claiming to know what it
      // means is not.
      return { label: observed.charAt(0).toUpperCase() + observed.slice(1), tone: "unknown", expected: false };
  }
}

/** True when observed state diverges from what the control plane asked for. */
export function hasDrift(node: ApiNode): boolean {
  if (!node.desiredState || !node.actualState) return false;
  const reconciled: Record<string, string[]> = {
    active: ["online"],
    maintenance: ["offline", "online"],
    draining: ["online", "offline"],
  };
  const acceptable = reconciled[node.desiredState];
  return acceptable ? !acceptable.includes(node.actualState) : false;
}

/** The state we have actually observed on the node, lowercased. */
function observedState(node: ApiNode): string {
  return (node.actualState ?? node.status ?? "").toLowerCase();
}

/**
 * Split **offline** nodes into the two populations an operator cares about.
 * Nodes in maintenance/draining are expected to be down; unknown-state nodes
 * belong in neither bucket because we have not observed them down at all.
 *
 * Both buckets require an observed-offline reading. `nodeStatus().expected` is
 * not sufficient on its own: it is true for any maintenance or draining node
 * regardless of observed state, so keying off it alone counted nodes that are
 * up and serving as "down by operator intent" — the exact inverse of the
 * truth, on the panel an operator uses to decide whether to intervene.
 */
export function partitionOffline(nodes: ApiNode[]): { expected: ApiNode[]; unexpected: ApiNode[] } {
  const expected: ApiNode[] = [];
  const unexpected: ApiNode[] = [];
  for (const node of nodes) {
    if (observedState(node) !== "offline") continue;
    if (nodeStatus(node).expected) expected.push(node);
    else unexpected.push(node);
  }
  return { expected, unexpected };
}

/* ------------------------------------------------------------------ *
 * Health-check semantics
 * ------------------------------------------------------------------ */

export type CheckVerdict = {
  label: string;
  tone: StatusTone;
  /** False when we have no reading at all — the subsystem state is unknown. */
  known: boolean;
};

/**
 * Render a single control-plane check.
 *
 * `available` must be the result of {@link isAvailable} on the /health query.
 * The three outcomes are kept distinct on purpose: an unreachable /health is
 * "Unavailable", a reachable /health that omits the check is "Not reported",
 * and only a present check yields a real status. None of them may collapse
 * into "Healthy".
 */
export function checkVerdict(available: boolean, check: ApiHealthCheck | undefined): CheckVerdict {
  if (!available) return { label: "Unavailable", tone: "unknown", known: false };
  if (!check) return { label: "Not reported", tone: "unknown", known: false };
  switch (check.status) {
    case "ok":
      return { label: "Healthy", tone: "ok", known: true };
    case "warning":
      return { label: "Warning", tone: "warn", known: true };
    case "failed":
      return { label: "Failed", tone: "danger", known: true };
    default:
      return { label: "Unknown", tone: "unknown", known: false };
  }
}

export function findCheck(report: ApiHealthReport | undefined, name: string): ApiHealthCheck | undefined {
  return report?.checks?.find((check) => check.name === name);
}

/** Numeric detail from a check's `details` bag. `undefined` when absent or non-numeric. */
export function checkDetailNumber(check: ApiHealthCheck | undefined, key: string): number | undefined {
  const raw = check?.details?.[key];
  return typeof raw === "number" && Number.isFinite(raw) ? raw : undefined;
}

export function checkDetailString(check: ApiHealthCheck | undefined, key: string): string | undefined {
  const raw = check?.details?.[key];
  if (typeof raw === "string" && raw.length > 0) return raw;
  if (typeof raw === "number" && Number.isFinite(raw)) return String(raw);
  return undefined;
}

/**
 * Operator-facing next step for a failing check, keyed on the check names the
 * Go health service actually emits (`internal/services/health`). Returns
 * `undefined` when we have no specific guidance — better than inventing some.
 */
export function remediationFor(check: ApiHealthCheck): string | undefined {
  if (check.status === "ok") return undefined;
  const guidance: Record<string, string> = {
    database: "Verify Postgres is reachable and the control-plane credentials are current.",
    cache: "Check the Redis service and the cache connection settings.",
    queue: "Inspect the job queue backlog and confirm workers are consuming.",
    daemon: "Check Beacon connectivity on the affected hosts; heartbeats are not arriving.",
    disk: "Free space on the control-plane volume or raise the disk threshold.",
    memory: "Investigate control-plane memory growth; the API process is above its heap threshold.",
    storage: "Verify the configured storage backend is mounted and writable.",
    sftp: "Confirm the SFTP listener is running and reachable on its configured port.",
    backup: "Review the backup engine configuration and the most recent backup job.",
    api: "Inspect API runtime logs; the process reported a degraded runtime.",
    system: "Review host-level resources on the control-plane machine.",
  };
  return guidance[check.name];
}

/* ------------------------------------------------------------------ *
 * Query hooks — the only sanctioned readers of these sources
 * ------------------------------------------------------------------ */

export function useNodesQuery(): UseQueryResult<ApiNode[]> {
  return useQuery({
    queryKey: queryKeys.nodes.allLists(),
    queryFn: fetchAllNodes,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
}

export function useServersQuery(): UseQueryResult<ApiServer[]> {
  return useQuery({
    queryKey: queryKeys.servers.allLists(),
    queryFn: fetchAllServers,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
}

export function useHealthQuery(): UseQueryResult<ApiHealthReport> {
  return useQuery({
    queryKey: queryKeys.health.report(),
    queryFn: fetchHealthStatus,
    refetchInterval: REFRESH.health,
    retry: false,
  });
}

/** Latest reported metric row per node. `GET /monitoring/nodes/metrics` with no nodeId. */
export function useLatestNodeMetricsQuery(): UseQueryResult<NodeMetrics[]> {
  return useQuery({
    queryKey: queryKeys.monitoring.latest(),
    queryFn: () => getNodeMetrics(),
    refetchInterval: REFRESH.telemetry,
    retry: false,
  });
}

/**
 * Observability summary.
 *
 * Caution baked into the type: when the API's observability service is not
 * wired, `GET /monitoring/summary` returns every field as `0`/`[]`. A zeroed
 * summary therefore cannot be distinguished from a genuinely empty platform,
 * so {@link summaryIsTrustworthy} gates whether callers may render its counts.
 */
export function useMonitoringSummaryQuery(): UseQueryResult<SystemInfo> {
  return useQuery({
    queryKey: queryKeys.monitoring.summary(),
    queryFn: getSystemInfo,
    refetchInterval: REFRESH.telemetry,
    retry: false,
  });
}

/**
 * Whether the summary's counters may be shown as real numbers.
 *
 * The all-zero shape is exactly what the API emits when `observability == nil`,
 * so we corroborate against a source we trust: if any node or server exists in
 * inventory while the summary claims zero of everything, the summary is not
 * reporting — it is unwired — and its counts must be suppressed rather than
 * displayed as zeros.
 */
export function summaryIsTrustworthy(
  summary: SystemInfo | undefined,
  corroboration: { nodeCount?: number; serverCount?: number },
): boolean {
  if (!summary) return false;
  const allZero =
    (summary.totalServers ?? 0) === 0 &&
    (summary.totalUsers ?? 0) === 0 &&
    (summary.unacknowledgedAlerts ?? 0) === 0 &&
    (summary.nodes?.length ?? 0) === 0 &&
    (summary.recentHealthChecks?.length ?? 0) === 0;
  if (!allZero) return true;
  const inventoryNonEmpty = (corroboration.nodeCount ?? 0) > 0 || (corroboration.serverCount ?? 0) > 0;
  return !inventoryNonEmpty;
}

/**
 * Metric history.
 *
 * The control plane only serves history per node: `GET /monitoring/nodes/metrics`
 * returns the latest row per node when `nodeId` is omitted and ignores
 * `limit`/`since`. Fleet-wide history therefore requires a client fan-out, and
 * the fan-out reports its own failures rather than hiding them.
 */
export const FLEET_HISTORY_NODE_CAP = 12;

export function useNodeMetricsHistoryQuery(
  period: MetricPeriod,
  nodeId: string | null,
  nodes: ApiNode[],
  enabled: boolean,
): UseQueryResult<PartialFleet<NodeMetrics>> {
  const window = metricWindow(period);
  const fleetIds = nodes.map((node) => node.id);
  const scope = nodeId ?? `fleet:${fleetIds.slice(0, FLEET_HISTORY_NODE_CAP).join(",")}`;

  return useQuery({
    queryKey: queryKeys.monitoring.history(scope, period),
    enabled,
    refetchInterval: REFRESH.history,
    retry: false,
    queryFn: async (): Promise<PartialFleet<NodeMetrics>> => {
      if (nodeId) {
        const rows = await getNodeMetrics({ nodeId, period, limit: window.limit, since: window.since });
        return { rows, failedNodeIds: [], skippedNodeIds: [] };
      }

      const targets = fleetIds.slice(0, FLEET_HISTORY_NODE_CAP);
      const skippedNodeIds = fleetIds.slice(FLEET_HISTORY_NODE_CAP);
      const settled = await Promise.allSettled(
        targets.map((id) => getNodeMetrics({ nodeId: id, period, limit: window.limit, since: window.since })),
      );

      const rows: NodeMetrics[] = [];
      const failedNodeIds: string[] = [];
      settled.forEach((result, index) => {
        if (result.status === "fulfilled") rows.push(...result.value);
        else failedNodeIds.push(targets[index]);
      });

      // Every requested node failed: that is an error, not an empty series.
      if (targets.length > 0 && failedNodeIds.length === targets.length) {
        throw new Error(`Metric history unavailable for all ${targets.length} nodes`);
      }
      return { rows, failedNodeIds, skippedNodeIds };
    },
  });
}

export function useActivityQuery(limit: number) {
  return useQuery({
    queryKey: queryKeys.activity.admin(limit),
    queryFn: () => fetchAdminActivity({ limit }),
    refetchInterval: REFRESH.activity,
    retry: false,
  });
}

export function useAuditQuery() {
  return useQuery({
    queryKey: queryKeys.activity.audit(),
    queryFn: fetchAdminAudit,
    refetchInterval: REFRESH.activity,
    retry: false,
  });
}

export function useReservationsQuery() {
  return useQuery({
    queryKey: queryKeys.reservations.all,
    queryFn: fetchReservations,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
}

export function useRecoveryPlansQuery() {
  return useQuery({
    queryKey: queryKeys.recovery.all,
    queryFn: fetchRecoveryPlans,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
}

export function useHealthCheckTargetsQuery() {
  return useQuery({
    queryKey: queryKeys.health.targets(),
    queryFn: listUnhealthyTargets,
    refetchInterval: REFRESH.telemetry,
    retry: false,
  });
}

/**
 * Counters from the health-check runner.
 *
 * `/target-health/targets` returns only *failing* targets, so an empty list
 * alone cannot distinguish "everything is passing" from "nothing is being
 * probed". `/target-health/metrics` carries the denominator, and reading both
 * is the only way to describe the runner honestly.
 */
export type HealthCheckRunnerCounts = {
  total: number | undefined;
  healthy: number | undefined;
  suspected: number | undefined;
  unhealthy: number | undefined;
};

function readCount(source: HealthCheckMetrics, key: string): number | undefined {
  const value = source[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

export function useHealthCheckMetricsQuery(): UseQueryResult<HealthCheckRunnerCounts> {
  return useQuery({
    queryKey: [...queryKeys.health.all, "runner-metrics"] as const,
    queryFn: async (): Promise<HealthCheckRunnerCounts> => {
      const metrics = await fetchHealthCheckMetrics();
      return {
        total: readCount(metrics, "total"),
        healthy: readCount(metrics, "healthy"),
        suspected: readCount(metrics, "suspected"),
        unhealthy: readCount(metrics, "unhealthy"),
      };
    },
    refetchInterval: REFRESH.telemetry,
    retry: false,
  });
}

/* ------------------------------------------------------------------ *
 * Invalidation
 * ------------------------------------------------------------------ */

/**
 * Refresh every source the admin operational pages read. Used by the manual
 * refresh control and on tab re-focus, where the data on screen may have aged
 * past its interval while the tab was hidden.
 */
export function invalidateAdminOperational(queryClient: QueryClient): void {
  for (const key of [
    queryKeys.nodes.all,
    queryKeys.servers.all,
    queryKeys.health.all,
    queryKeys.monitoring.all,
    queryKeys.activity.all,
    queryKeys.reservations.all,
    queryKeys.recovery.all,
  ]) {
    void queryClient.invalidateQueries({ queryKey: key });
  }
}

export function useAdminOperationalRefresh(): () => void {
  const queryClient = useQueryClient();
  return useCallback(() => invalidateAdminOperational(queryClient), [queryClient]);
}

/**
 * Re-read on tab re-focus. A dashboard left open in a background tab is the
 * single most common way stale telemetry gets read as current.
 */
export function useRefreshOnVisible(): void {
  const refresh = useAdminOperationalRefresh();
  useEffect(() => {
    if (typeof document === "undefined") return;
    const onVisible = () => {
      if (document.visibilityState === "visible") refresh();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [refresh]);
}

/* ------------------------------------------------------------------ *
 * Derived fleet views
 * ------------------------------------------------------------------ */

/** Latest metric row per node, keyed by node id. */
export function indexLatestByNode(rows: NodeMetrics[] | undefined): Map<string, NodeMetrics> {
  const index = new Map<string, NodeMetrics>();
  for (const row of rows ?? []) {
    const existing = index.get(row.nodeId);
    if (!existing || Date.parse(row.observedAt) > Date.parse(existing.observedAt)) {
      index.set(row.nodeId, row);
    }
  }
  return index;
}

/**
 * Mean of a reported metric across the nodes that actually reported it.
 *
 * The divisor is the number of reporting nodes, not the fleet size: averaging
 * over non-reporting nodes would silently dilute the reading toward zero and
 * make a hot fleet look idle.
 */
export function fleetAverage(
  rows: Iterable<NodeMetrics>,
  read: (row: NodeMetrics) => number | null | undefined,
): { value: number | undefined; reporting: number } {
  let sum = 0;
  let reporting = 0;
  for (const row of rows) {
    const raw = finite(read(row));
    if (raw !== undefined) {
      sum += raw;
      reporting += 1;
    }
  }
  return { value: reporting > 0 ? sum / reporting : undefined, reporting };
}

/**
 * Configured capacity across nodes — allocation, not utilisation.
 *
 * Kept as a separate function from the telemetry readers above so the two can
 * never be confused at a call site: this is what an operator configured, and it
 * says nothing about what the host is currently doing.
 */
export function configuredCapacity(nodes: ApiNode[]): {
  memoryMb: ReportedTotal;
  diskMb: ReportedTotal;
  cpuCores: ReportedTotal;
} {
  return {
    memoryMb: reportedTotal(nodes, (node) => node.memoryMb),
    diskMb: reportedTotal(nodes, (node) => node.diskMb),
    cpuCores: reportedTotal(nodes, (node) => node.cpuCores ?? node.cpuThreads),
  };
}

/** Quota committed to servers — again allocation, not live usage. */
export function allocatedQuota(servers: ApiServer[]): {
  memoryMb: ReportedTotal;
  diskMb: ReportedTotal;
  cpuLimit: ReportedTotal;
} {
  return {
    memoryMb: reportedTotal(servers, (server) => server.memoryMb),
    diskMb: reportedTotal(servers, (server) => server.diskMb),
    cpuLimit: reportedTotal(servers, (server) => server.cpuLimit),
  };
}

export type { MetricPeriod, NodeMetrics, SystemInfo };
