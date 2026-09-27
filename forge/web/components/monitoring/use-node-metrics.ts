"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { getNodeMetrics, metricWindow, type MetricPeriod, type NodeMetrics } from "@/lib/api/monitoring";
import { queryKeys } from "@/lib/api/query-keys";

interface UseNodeMetricsOptions {
  nodeId?: string;
  period?: MetricPeriod;
  refetchInterval?: number;
}

/**
 * Single react-query stream for node metric history. Every widget on the
 * monitoring page derives from `getNodeMetrics({nodeId, period, ...})`, so
 * they must share one query key — otherwise the four summary charts,
 * SystemMetrics and the trend explorer each issue their own fetch of the
 * same endpoint with the same (nodeId, period) pair.
 */
export function useNodeMetricsQuery(options: UseNodeMetricsOptions = {}) {
  const { nodeId, period = "1h" } = options;
  const window = useMemo(() => metricWindow(period), [period]);
  const scope = nodeId ?? "all";
  return useQuery<NodeMetrics[]>({
    queryKey: queryKeys.monitoring.history(scope, period),
    queryFn: () => getNodeMetrics({ nodeId, period, limit: window.limit, since: window.since }),
    refetchInterval: options.refetchInterval ?? 30_000,
  });
}