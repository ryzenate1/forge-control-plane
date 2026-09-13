"use client";

/**
 * Deprecated. Use `useNodeMetricsHistoryQuery` from `@/lib/admin/telemetry`.
 *
 * This module used to own a second reader of `GET /monitoring/nodes/metrics`.
 * It cached a bare `NodeMetrics[]` under `queryKeys.monitoring.history(scope,
 * period)` — the same key `lib/admin/telemetry.ts` uses for a
 * `PartialFleet<NodeMetrics>` (`{rows, failedNodeIds, skippedNodeIds}`). Two
 * incompatible shapes, one cache entry: whichever query resolved first won, and
 * the other consumer read a shape it was not written for.
 *
 * It also differed semantically in a way that mattered. This reader used
 * `scope = nodeId ?? "all"` and returned whatever the fan-out produced, so a
 * fleet request where most nodes failed looked like a complete, quiet fleet. The
 * canonical hook reports `failedNodeIds` and `skippedNodeIds` and throws when
 * every node failed, because a fan-out that read nothing is an error, not an
 * empty series.
 *
 * Re-exported rather than kept as a parallel implementation so there is exactly
 * one reader, one key and one shape.
 */

export {
  useNodeMetricsHistoryQuery,
  FLEET_HISTORY_NODE_CAP,
  type PartialFleet,
} from "@/lib/admin/telemetry";
export type { MetricPeriod, NodeMetrics } from "@/lib/api/monitoring";
