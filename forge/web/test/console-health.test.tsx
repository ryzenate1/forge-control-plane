import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import ConsoleHealthPage from "@/app/console/health/page";
import { HealthStatusGauge, aggregateHealth } from "@/components/health/health-status-gauge";
import { HealthSummary } from "@/components/health/health-summary";
import { jsonResponse, mockFetchByUrl } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";
import type { ApiEndpointHealthRecord } from "@/lib/api";

function check(overrides: Partial<ApiEndpointHealthRecord>): ApiEndpointHealthRecord {
  return {
    id: "h1",
    endpointId: "forge-api-1",
    status: "healthy",
    reachable: true,
    healthScore: 92,
    containers: 3,
    images: 5,
    volumes: 2,
    observedAt: "2026-08-16T00:00:00Z",
    ...overrides,
  };
}

describe("aggregateHealth", () => {
  it("scores healthy when every endpoint is reachable", () => {
    expect(aggregateHealth([check({}), check({ id: "h2" })]).level).toBe("healthy");
  });

  it("scores critical when no endpoint is reachable", () => {
    expect(aggregateHealth([check({ reachable: false })]).level).toBe("critical");
  });

  it("scores degraded on partial reachability and unknown when empty", () => {
    expect(aggregateHealth([check({}), check({ id: "h2", reachable: false })]).level).toBe("degraded");
    expect(aggregateHealth([]).level).toBe("unknown");
  });

  it("includes measured zero scores and never invents missing scores", () => {
    expect(aggregateHealth([check({ healthScore: 0 }), check({ healthScore: 100 })]).score).toBe(50);
    expect(aggregateHealth([{ status: "healthy" }]).score).toBeNull();
    expect(aggregateHealth([{}]).level).toBe("unknown");
    expect(aggregateHealth([check({}), { status: "healthy" }]).score).toBeNull();
  });

  it("does not label a reachable but degraded endpoint healthy", () => {
    expect(aggregateHealth([check({ status: "degraded", reachable: true })]).level).toBe("degraded");
  });
});

describe("health status gauge", () => {
  it("shows the aggregate score and reachability summary", () => {
    renderWithQuery(<HealthStatusGauge checks={[check({ healthScore: 80 })]} />);
    expect(screen.getByText("80%")).toBeInTheDocument();
    expect(screen.getByText("1 of 1 endpoints reachable")).toBeInTheDocument();
    expect(screen.getByText("Healthy")).toBeInTheDocument();
  });

  it("shows the empty-data state", () => {
    renderWithQuery(<HealthStatusGauge checks={[]} />);
    expect(screen.getByText("No data")).toBeInTheDocument();
    expect(screen.getByText("No endpoint health checks recorded yet.")).toBeInTheDocument();
  });
});

describe("health summary", () => {
  it("renders endpoint rows with reachable metadata and the empty state", () => {
    renderWithQuery(<HealthSummary checks={[check({})]} />);
    expect(screen.getByText("forge-api-1")).toBeInTheDocument();
    expect(screen.getByText(/3 containers · 5 images · 2 volumes/)).toBeInTheDocument();
  });

  it("surfaces the error message of an unreachable endpoint", () => {
    renderWithQuery(<HealthSummary checks={[check({ reachable: false, error: "connection refused" })]} />);
    expect(screen.getByText("connection refused")).toBeInTheDocument();
    expect(screen.getByText("Unreachable")).toBeInTheDocument();
  });

  it("renders the empty state when no checks exist", () => {
    renderWithQuery(<HealthSummary checks={[]} />);
    expect(screen.getByText("No health checks yet")).toBeInTheDocument();
  });
});

describe("console health page", () => {
  it("renders gauge, alert queue and node list from the summary endpoint", async () => {
    // Health-history status is reported, but numeric scores and inventory are not.
    mockFetchByUrl({
      "/monitoring/summary": jsonResponse({
        nodes: [],
        unacknowledgedAlerts: 2,
        recentHealthChecks: [
          { id: "h1", checkName: "forge-api-1", status: "healthy", message: "", latencyMs: 12, critical: false, observedAt: "2026-08-16T00:00:00Z" },
        ],
      }),
      "/nodes": jsonResponse([]),
      "/monitoring/nodes/metrics": jsonResponse([]),
    });
    renderWithQuery(<ConsoleHealthPage />);
    expect(await screen.findByText("1 of 1 checks healthy; reachability unreported")).toBeInTheDocument();
    expect(screen.queryByText("100%")).not.toBeInTheDocument();
    expect(screen.queryByText(/0 containers/)).not.toBeInTheDocument();
    expect(screen.queryByText("Degraded performance")).not.toBeInTheDocument();
    expect(screen.getByText("2 unacknowledged alerts")).toBeInTheDocument();
    expect(screen.getByText("2 unacknowledged alerts require attention.")).toBeInTheDocument();
    expect(screen.getByText("forge-api-1")).toBeInTheDocument();
  });

  it("does not report an empty alert queue when the count is missing", async () => {
    mockFetchByUrl({
      "/monitoring/summary": jsonResponse({ nodes: [], recentHealthChecks: [] }),
      "/nodes": jsonResponse([]),
      "/monitoring/nodes/metrics": jsonResponse([]),
    });
    renderWithQuery(<ConsoleHealthPage />);
    expect(await screen.findByText("No health checks yet")).toBeInTheDocument();
    expect(screen.getByText("Alert count is unavailable.")).toBeInTheDocument();
    expect(screen.queryByText(/All alerts are acknowledged/)).not.toBeInTheDocument();
  });

  it("shows the error state when the summary endpoint fails", async () => {
    mockFetchByUrl({
      "/monitoring/summary": jsonResponse({ message: "boom" }, 500),
      "/nodes": jsonResponse([]),
      "/monitoring/nodes/metrics": jsonResponse([]),
    });
    renderWithQuery(<ConsoleHealthPage />);
    const alert = await screen.findByRole("alert", {}, { timeout: 5000 });
    // Accept either generic AdminErrorState or ApiUnavailableState depending on error classification;
    // both indicate health fetch failure and contain actionable text.
    expect(alert).toHaveTextContent(/Health data is unavailable|API unavailable|Network error|unavailable/i);
  });
});
