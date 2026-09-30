import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, mockFetchByUrl } from "@/test/fetch-mock";
import { getSystemInfo } from "./monitoring";

const originalFetch = global.fetch;
afterEach(() => {
  global.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("monitoring summary adaptation", () => {
  it("maps the recorded check label without synthesizing telemetry", async () => {
    mockFetchByUrl({
      "/monitoring/summary": jsonResponse({
        recentHealthChecks: [{ checkName: "database", status: "healthy", message: "query succeeded" }],
      }),
    });
    const result = await getSystemInfo();
    const record = result.recentHealthChecks?.[0];
    expect(record?.endpointId).toBe("database");
    expect(record?.message).toBe("query succeeded");
    expect(record?.error).toBeUndefined();
    for (const field of ["id", "reachable", "healthScore", "containers", "images", "volumes", "observedAt"] as const) {
      expect(record?.[field]).toBeUndefined();
    }
    expect(result.unacknowledgedAlerts).toBeUndefined();
  });

  it("preserves reported zero scores, false reachability, and observation time", async () => {
    const record = { id: "h1", endpointId: "node-1", status: "critical", healthScore: 0, reachable: false, containers: 0, observedAt: "2026-01-01T00:00:00Z" };
    mockFetchByUrl({ "/monitoring/summary": jsonResponse({ recentHealthChecks: [record] }) });
    expect((await getSystemInfo()).recentHealthChecks?.[0]).toMatchObject(record);
  });
});
