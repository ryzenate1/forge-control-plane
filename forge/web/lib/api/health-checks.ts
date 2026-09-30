import { fetchJSON } from "./http";

// Read-only client for the live health-check runner (services/healthcheckrunner),
// exposed by forge/api/internal/http/handlers_health_checks.go at /target-health.
// The runner is a background prober that feeds the reconciler; this surfaces its
// per-target state for the admin Health dashboard.

export type HealthCheckStatus = "healthy" | "suspected" | "unhealthy";

export type HealthCheckTarget = {
  id: string;
  groupId?: string;
  serverId?: string;
  status: HealthCheckStatus | string;
  consecutiveFailures: number;
  consecutiveSuccesses: number;
  suspectedSince?: string | null;
  lastCheckAt: string;
  lastSuccessAt?: string | null;
  lastFailureAt?: string | null;
  healthyThreshold: number;
  unhealthyThreshold: number;
};

export type HealthCheckMetrics = {
  [key: string]: unknown;
};

export async function listUnhealthyTargets(): Promise<HealthCheckTarget[]> {
  const res = await fetchJSON<{ data: HealthCheckTarget[] }>("/target-health/targets");
  return res.data ?? [];
}

export async function fetchHealthCheckMetrics(): Promise<HealthCheckMetrics> {
  const res = await fetchJSON<{ data: HealthCheckMetrics }>("/target-health/metrics");
  return res.data ?? {};
}
