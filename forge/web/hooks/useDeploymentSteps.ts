"use client";

import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchDeploymentSteps } from "@/lib/api/deployments";
import type { DeploymentStep } from "@/lib/api/deployments";

/**
 * Single source for deployment step polling.
 * Consolidates previously duplicated pollers:
 *  - components/app/deployment-progress.tsx:41 (POLL_INTERVAL_MS = 2000)
 *  - components/charts/DeploymentTimeline.tsx:27 (refetchInterval 5000)
 *
 * Dedup strategy: adaptive 5s polling (fixed interval, no 2s duplicate),
 * stops when all steps reach terminal status or after max duration.
 * Both views now share the same queryKey ["deployment-steps", id] so only
 * one network poller runs even if both are mounted.
 */

const POLL_INTERVAL_MS = 5000;
const MAX_POLL_DURATION_MS = 10 * 60 * 1000;

function isTerminalStatus(status: string): boolean {
  return (
    status === "completed" ||
    status === "failed" ||
    status === "cancelled" ||
    status === "skipped" ||
    status === "done" ||
    status === "error"
  );
}

export function isDeploymentStepsTerminal(steps: DeploymentStep[] | undefined): boolean {
  if (!steps || steps.length === 0) return false;
  return steps.every((s) => isTerminalStatus(s.status));
}

export function useDeploymentSteps(
  deploymentId: string,
  opts?: { enabled?: boolean },
) {
  const hookStartRef = useRef<number>(Date.now());
  useEffect(() => {
    hookStartRef.current = Date.now();
  }, [deploymentId]);

  return useQuery({
    queryKey: ["deployment-steps", deploymentId],
    queryFn: ({ signal }) => fetchDeploymentSteps(deploymentId, { signal }),
    enabled: Boolean(deploymentId) && (opts?.enabled ?? true),
    // Adaptive 5s polling: keep polling while any step is non-terminal.
    refetchInterval: (query) => {
      const elapsed = Date.now() - hookStartRef.current;
      if (elapsed > MAX_POLL_DURATION_MS) return false;
      const data = query.state.data as DeploymentStep[] | undefined;
      if (!data || data.length === 0) return POLL_INTERVAL_MS;
      if (isDeploymentStepsTerminal(data)) return false;
      return POLL_INTERVAL_MS;
    },
    refetchIntervalInBackground: false,
    placeholderData: (prev) => prev,
    retry: 1,
    staleTime: 1000,
    gcTime: 5 * 60 * 1000,
  });
}

export const DEPLOYMENT_STEPS_POLL_INTERVAL_MS = POLL_INTERVAL_MS;
export const DEPLOYMENT_STEPS_MAX_DURATION_MS = MAX_POLL_DURATION_MS;
