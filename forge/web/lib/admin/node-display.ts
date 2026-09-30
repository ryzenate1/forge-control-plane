import type { ApiNode } from "@/lib/api/types";
import { absoluteTime, relativeTime } from "./telemetry";

export type NodeRuntimeSummary = { label: string; mock: boolean; detail?: string };

type RuntimeFields = Partial<
  Pick<
    ApiNode,
    "runtimeProvider" | "runtimeStatus" | "dockerStatus" | "heartbeatError" | "schedulerType"
  >
>;

/**
 * Human summary of the container runtime a node heartbeat reported.
 *
 * macOS/native dev runs Beacon with `DAEMON_ALLOW_MOCK_RUNTIME=true` and no
 * Docker daemon, so heartbeats arrive with `runtimeStatus: "error"` and an
 * error like "docker runtime unavailable". That node is live (last-seen
 * refreshes every ~30s) with a mock runtime — it must read as degraded/mock,
 * never as offline and never as healthy docker.
 */
export function nodeRuntimeSummary(node: RuntimeFields): NodeRuntimeSummary {
  const provider = (node.runtimeProvider || node.schedulerType || "docker").toLowerCase();
  const status = (node.runtimeStatus || "").toLowerCase();
  const signal = `${node.dockerStatus ?? ""} ${node.heartbeatError ?? ""}`.toLowerCase();
  const mock = status === "error" || signal.includes("unavailable") || signal.includes("mock");
  const detail = node.heartbeatError || node.dockerStatus || node.runtimeStatus || undefined;
  return { label: mock ? `${provider} · mock` : provider, mock, detail };
}

type SeenFields = Partial<Pick<ApiNode, "lastSeenAt" | "lastHeartbeatAt">>;

/**
 * Newest usable heartbeat timestamp for a node, if any.
 *
 * Nodes that never reported carry the zero time (`0001-01-01T00:00:00Z`)
 * because the store leaves `LastHeartbeatAt` unset — that must read as
 * "not reported", not as a heartbeat billions of seconds old (or, after
 * clamping, as "just now").
 */
export function nodeHeartbeatIso(node: SeenFields): string | undefined {
  for (const value of [node.lastSeenAt, node.lastHeartbeatAt]) {
    if (!value) continue;
    const at = Date.parse(value);
    if (Number.isFinite(at) && at > 0) return value;
  }
  return undefined;
}

/** Relative last-seen for table cells. Absent input stays "Not reported", never "just now". */
export function nodeLastSeenLabel(node: SeenFields): string {
  const iso = nodeHeartbeatIso(node);
  return iso ? (relativeTime(iso) ?? "Not reported") : "Not reported";
}

/** Absolute last-seen for cell tooltips. */
export function nodeLastSeenTitle(node: SeenFields): string | undefined {
  const iso = nodeHeartbeatIso(node);
  return iso ? absoluteTime(iso) : undefined;
}
