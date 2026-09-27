import { describe, expect, it } from "vitest";
import { hasDrift, nodeStatus, partitionOffline } from "@/lib/admin/telemetry";
import type { ApiNode } from "@/lib/api";

/**
 * Truthfulness guards for the admin telemetry layer.
 *
 * lib/admin/telemetry.ts states its own rules: not-reported is never 0, a
 * stale reading is not a healthy one, and an offline node is only "expected"
 * to be down when an operator asked for it. These tests hold it to them.
 */

function node(overrides: Partial<ApiNode>): ApiNode {
  return { id: "node-1", name: "node-1", ...overrides } as ApiNode;
}

describe("partitionOffline", () => {
  it("does not report an online maintenance node as down", () => {
    // Regression: partitionOffline keyed off nodeStatus().expected, which is
    // true for any maintenance/draining node regardless of observed state, so
    // a node that was up and serving was counted as "down by operator intent".
    const { expected, unexpected } = partitionOffline([
      node({ maintenanceMode: true, actualState: "online" }),
    ]);
    expect(expected).toEqual([]);
    expect(unexpected).toEqual([]);
  });

  it("does not report an online draining node as down", () => {
    const { expected, unexpected } = partitionOffline([
      node({ draining: true, actualState: "online" }),
    ]);
    expect(expected).toEqual([]);
    expect(unexpected).toEqual([]);
  });

  it("counts an offline maintenance node as expected", () => {
    const subject = node({ maintenanceMode: true, actualState: "offline" });
    const { expected, unexpected } = partitionOffline([subject]);
    expect(expected).toEqual([subject]);
    expect(unexpected).toEqual([]);
  });

  it("counts an offline draining node as expected", () => {
    const subject = node({ desiredState: "draining", actualState: "offline" });
    expect(partitionOffline([subject]).expected).toEqual([subject]);
  });

  it("counts an offline node with no operator intent as unexpected", () => {
    const subject = node({ actualState: "offline" });
    const { expected, unexpected } = partitionOffline([subject]);
    expect(expected).toEqual([]);
    expect(unexpected).toEqual([subject]);
  });

  it("places unknown-state nodes in neither bucket", () => {
    // We have not observed these down; claiming either way would be invented.
    const { expected, unexpected } = partitionOffline([
      node({ actualState: "unknown" }),
      node({}),
    ]);
    expect(expected).toEqual([]);
    expect(unexpected).toEqual([]);
  });

  it("falls back to status when actualState is absent", () => {
    const subject = node({ status: "offline" });
    expect(partitionOffline([subject]).unexpected).toEqual([subject]);
    expect(partitionOffline([node({ status: "online", maintenanceMode: true })]).expected).toEqual([]);
  });

  it("keeps degraded nodes out of the offline populations", () => {
    const { expected, unexpected } = partitionOffline([node({ actualState: "degraded" })]);
    expect(expected).toEqual([]);
    expect(unexpected).toEqual([]);
  });
});

describe("nodeStatus", () => {
  it("does not read an unknown node as online or offline", () => {
    for (const subject of [node({}), node({ actualState: "unknown" })]) {
      const verdict = nodeStatus(subject);
      expect(verdict.label).toBe("Unknown");
      // `unknown`, not `neutral`: the canonical vocabulary gives an absent
      // reading its own grey/dashed treatment so it can never be mistaken for
      // a plain, healthy value.
      expect(verdict.tone).toBe("unknown");
      expect(verdict.expected).toBe(false);
    }
  });

  it("lets operator intent win the label", () => {
    expect(nodeStatus(node({ maintenanceMode: true, actualState: "offline" })).label).toBe("Maintenance");
    expect(nodeStatus(node({ desiredState: "draining", actualState: "offline" })).label).toBe("Draining");
  });

  it("reports an unconfigured offline node as an incident", () => {
    const verdict = nodeStatus(node({ actualState: "offline" }));
    expect(verdict.label).toBe("Offline");
    expect(verdict.tone).toBe("danger");
    expect(verdict.expected).toBe(false);
  });
});

describe("hasDrift", () => {
  it("reports nothing when either side of the comparison is missing", () => {
    expect(hasDrift(node({ actualState: "online" }))).toBe(false);
    expect(hasDrift(node({ desiredState: "active" }))).toBe(false);
  });

  it("flags an active node that is not online", () => {
    expect(hasDrift(node({ desiredState: "active", actualState: "offline" }))).toBe(true);
    expect(hasDrift(node({ desiredState: "active", actualState: "online" }))).toBe(false);
  });
});
