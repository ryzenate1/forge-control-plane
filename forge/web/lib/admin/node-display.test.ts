import { describe, expect, it } from "vitest";
import {
  nodeHeartbeatIso,
  nodeLastSeenLabel,
  nodeRuntimeSummary,
} from "./node-display";

describe("nodeRuntimeSummary", () => {
  it("marks the macOS mock runtime (docker provider, error status) as mock, not healthy", () => {
    const summary = nodeRuntimeSummary({
      runtimeProvider: "docker",
      runtimeStatus: "error",
      heartbeatError: "docker runtime unavailable",
      schedulerType: "docker",
    });
    expect(summary.mock).toBe(true);
    expect(summary.label).toBe("docker · mock");
    expect(summary.detail).toBe("docker runtime unavailable");
  });

  it("reads a healthy docker heartbeat as plain docker", () => {
    const summary = nodeRuntimeSummary({
      runtimeProvider: "docker",
      runtimeStatus: "ok",
      schedulerType: "docker",
    });
    expect(summary.mock).toBe(false);
    expect(summary.label).toBe("docker");
  });

  it("treats an unavailable signal as mock even when the status field is absent", () => {
    const summary = nodeRuntimeSummary({
      dockerStatus: "unavailable",
      schedulerType: "docker",
    });
    expect(summary.mock).toBe(true);
  });

  it("never claims a provider the node did not report", () => {
    const summary = nodeRuntimeSummary({});
    expect(summary.mock).toBe(false);
    expect(summary.label).toBe("docker");
  });
});

describe("nodeHeartbeatIso / nodeLastSeenLabel", () => {
  it("prefers lastSeenAt over lastHeartbeatAt", () => {
    const node = {
      lastSeenAt: "2026-09-30T12:00:30Z",
      lastHeartbeatAt: "2026-09-30T12:00:00Z",
    };
    expect(nodeHeartbeatIso(node)).toBe(node.lastSeenAt);
  });

  it("reads the zero time as not reported instead of an ancient heartbeat", () => {
    const node = { lastHeartbeatAt: "0001-01-01T00:00:00Z" };
    expect(nodeHeartbeatIso(node)).toBeUndefined();
    expect(nodeLastSeenLabel(node)).toBe("Not reported");
  });

  it("reads absent timestamps as not reported, never just now", () => {
    expect(nodeLastSeenLabel({})).toBe("Not reported");
  });
});
