import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { renderWithQuery } from "@/test/render";
import { makeServer, makeServerAccess } from "@/test/fixtures";
import { ServerProvider } from "@/components/server/server-context";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const fetchServerLogsMock = vi.fn().mockResolvedValue("");
const sendPowerSignalMock = vi.fn().mockResolvedValue({ serverId: "s1", signal: "start", accepted: true });
const reinstallServerMock = vi.fn().mockResolvedValue({ accepted: true });
const fetchOperationMock = vi.fn();

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api");
  return {
    ...actual,
    fetchServerLogs: (...args: unknown[]) => fetchServerLogsMock(...args),
    sendPowerSignal: (...args: unknown[]) => sendPowerSignalMock(...args),
    reinstallServer: (...args: unknown[]) => reinstallServerMock(...args),
    fetchOperation: (...args: unknown[]) => fetchOperationMock(...args),
    connectServerWebSocket: vi.fn().mockImplementation(async () => {
      // Return a dummy websocket — the manager mock ignores this factory
      return {
        close: vi.fn(),
        send: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        readyState: 1,
      } as unknown as WebSocket;
    }),
  };
});

// WebSocketManager mock — capture instances so tests can drive onMessage/onStatusChange
const managerInstances: Array<{ config: Record<string, unknown>; status: string; connect: () => Promise<void>; disconnect: () => void; send: (d: string) => void }> = [];

vi.mock("@/lib/api/ws/websocket-manager", () => {
  class MockWebSocketManager {
    config: Record<string, unknown>;
    status = "disconnected" as string;
    constructor(config: Record<string, unknown>) {
      this.config = config;
      managerInstances.push(this as unknown as { config: Record<string, unknown>; status: string; connect: () => Promise<void>; disconnect: () => void; send: (d: string) => void });
    }
    async connect() {
      this.status = "connected";
      (this.config.onStatusChange as ((s: string) => void) | undefined)?.("connected");
    }
    disconnect() {
      this.status = "disconnected";
      (this.config.onStatusChange as ((s: string) => void) | undefined)?.("disconnected");
    }
    send() {}
  }
  return { WebSocketManager: MockWebSocketManager };
});

// Exercise production helpers, not copies of the intended implementation.
import {
  ConsoleView,
  computeNetworkDelta,
  decodeConsoleFrame,
  decodeStatsFrame,
  deriveSessionState,
  extractServerTimestamp,
  getChartMax,
} from "@/components/server/console-view";
function getConsoleManager() {
  // first manager is console (factory for "console"), second is stats
  return managerInstances[0] as unknown as { config: { onMessage?: (data: unknown) => void; onStatusChange?: (s: string) => void } };
}
function getStatsManager() {
  // Heuristic: stats manager's onMessage expects an object with cpuPercent/network fields
  // We have two managers after mount; the second one is stats. But after each test we clear.
  // If only one manager exists, return it (fallback). Otherwise second is stats.
  if (managerInstances.length === 1) return managerInstances[0] as unknown as { config: { onMessage?: (data: unknown) => void } };
  return managerInstances[1] as unknown as { config: { onMessage?: (data: unknown) => void } };
}

beforeEach(() => {
  managerInstances.length = 0;
  fetchServerLogsMock.mockReset();
  fetchServerLogsMock.mockResolvedValue("");
  fetchOperationMock.mockReset();
  sendPowerSignalMock.mockReset();
  sendPowerSignalMock.mockResolvedValue({ serverId: "s1", signal: "start", accepted: true });
  vi.useRealTimers();
  // jsdom doesn't implement scrollTo — console-view autoScroll uses it
  if (!Element.prototype.scrollTo) {
    Object.defineProperty(Element.prototype, "scrollTo", { value: vi.fn(), configurable: true, writable: true });
  } else {
    vi.spyOn(Element.prototype, "scrollTo").mockImplementation(() => {});
  }
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  managerInstances.length = 0;
});

// ---------------------------------------------------------------------------
// Unit: extractServerTimestamp + synthetic frozen vs serverTs
// ---------------------------------------------------------------------------

describe("console-view synthetic timestamps frozen vs serverTs", () => {
  it("extracts bracket and ISO server timestamps, else null", () => {
    expect(extractServerTimestamp("[12:34:56] Server started")).toBe("12:34:56");
    expect(extractServerTimestamp("[01:02:03.456] tick")).toBe("01:02:03.456");
    expect(extractServerTimestamp("2026-01-15 12:34:56 Server ready")).toBe("2026-01-15 12:34:56");
    expect(extractServerTimestamp("2026-01-15T12:34:56Z Booting")).toBe("2026-01-15T12:34:56Z");
    expect(extractServerTimestamp("plain line without ts")).toBeNull();
    expect(extractServerTimestamp(" [12:34:56] indented")).toBeNull();
  });

  it("source file implements serverTs extraction before synthetic fallback", () => {
    const src = readFileSync(resolve(__dirname, "../components/server/console-view.tsx"), "utf8");
    expect(src).toContain("extractServerTimestamp");
    expect(src).toContain("serverTs");
    expect(src).toContain("new Date(entry.ts).toLocaleTimeString()");
    // Ensures UI prefers serverTs: `entry.serverTs ?? new Date(entry.ts)...`
    expect(src).toMatch(/entry\.serverTs\s*\?\?\s*new Date\(entry\.ts\)/);
  });

  it("freezes Date.now per appendLines batch (all lines in same tick share ts)", async () => {
    const frozen = new Date("2026-02-01T10:00:00.000Z").getTime();
    const dateNowSpy = vi.spyOn(Date, "now").mockReturnValue(frozen);
    // Two plain lines in same fetch batch -> same synthetic ts
    fetchServerLogsMock.mockResolvedValue("plain-a\nplain-b");

    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );

    // wait for lines to appear (without timestamps initially hidden)
    expect(await screen.findByText("plain-a")).toBeInTheDocument();
    expect(screen.getByText("plain-b")).toBeInTheDocument();

    // enable timestamps — should show synthetic for both, same value
    const toggle = screen.getByLabelText(/Show timestamps|Hide timestamps/);
    await userEvent.click(toggle);

    const expected = new Date(frozen).toLocaleTimeString();
    const stamps = screen.getAllByText(expected);
    // Both lines now show the frozen synthetic time
    expect(stamps.length).toBeGreaterThanOrEqual(2);

    // Verify that a second batch with advanced time gets a different frozen ts,
    // while lines within that batch still share it.
    const frozen2 = frozen + 5_000;
    dateNowSpy.mockReturnValue(frozen2);
    const consoleMgr = getConsoleManager();
    const onMessage = consoleMgr?.config.onMessage;
    expect(onMessage).toBeDefined();
    // Trigger a new batch containing one plain line
    onMessage?.("late-line" as unknown);
    await waitFor(() => expect(screen.getByText("late-line")).toBeInTheDocument());
    const expected2 = new Date(frozen2).toLocaleTimeString();
    // late-line should show frozen2, not frozen
    expect(screen.getByText(expected2)).toBeInTheDocument();
    // original lines still show original frozen
    expect(screen.getAllByText(expected).length).toBeGreaterThanOrEqual(2);
    dateNowSpy.mockRestore();
  });

  it("prefers serverTs over synthetic when line carries a timestamp prefix", async () => {
    const frozen = new Date("2026-02-01T11:00:00.000Z").getTime();
    const dateNowSpy = vi.spyOn(Date, "now").mockReturnValue(frozen);
    fetchServerLogsMock.mockResolvedValue("[09:01:02] Server started\nplain without ts");

    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );

    expect(await screen.findByText("[09:01:02] Server started")).toBeInTheDocument();
    expect(screen.getByText("plain without ts")).toBeInTheDocument();

    await userEvent.click(screen.getByLabelText(/Show timestamps/));

    // server-prefixed line should render serverTs directly, not synthetic
    expect(screen.getByText("09:01:02")).toBeInTheDocument();
    // plain line should render synthetic frozen time
    const synthetic = new Date(frozen).toLocaleTimeString();
    expect(screen.getByText(synthetic)).toBeInTheDocument();
    // Ensure we don't show synthetic where serverTs should be — count of synthetic stamps should be exactly 1 (for plain line)
    // The second line's synthetic is not duplicated for serverTs line
    expect(screen.getAllByText(synthetic).length).toBe(1);
    dateNowSpy.mockRestore();
  });

  it("truncates to MAX_LINES (500) — file appendLines uses slice(-MAX_LINES)", async () => {
    const src = readFileSync(resolve(__dirname, "../components/server/console-view.tsx"), "utf8");
    expect(src).toContain("MAX_LINES = 500");
    expect(src).toContain("slice(-MAX_LINES)");
    // Behavioral: generate 502 lines via fetch, only 500 should remain
    const many = Array.from({ length: 502 }, (_, i) => `line-${i}`).join("\n");
    fetchServerLogsMock.mockResolvedValue(many);
    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );
    // First lines (0,1) should be dropped; last should remain
    expect(await screen.findByText("line-501")).toBeInTheDocument();
    expect(screen.queryByText("line-0")).not.toBeInTheDocument();
    expect(screen.queryByText("line-1")).not.toBeInTheDocument();
    expect(screen.getByText("line-2")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Network delta per tick not cumulative
// ---------------------------------------------------------------------------

describe("console-view network delta per tick not cumulative", () => {
  it("first tick has no delta (pushes 0 to avoid spike)", () => {
    const { delta } = computeNetworkDelta(1000, 2000, null, null);
    expect(delta).toBe(0);
  });

  it("computes per-tick delta as sum of Rx+Tx differences", () => {
    // second tick: prev 1000/2000, now 1100/2150 => drx 100, dtx 150 => 250
    let prevRx: number | null = 1000;
    let prevTx: number | null = 2000;
    const _first = computeNetworkDelta(1100, 2150, prevRx, prevTx);
    let delta = _first.delta;
    const nextPrevRx = _first.nextPrevRx;
    const nextPrevTx = _first.nextPrevTx;
    expect(delta).toBe(250);
    prevRx = nextPrevRx;
    prevTx = nextPrevTx;
    // third tick cumulative would be 3250 if cumulative, but delta should be 50
    ({ delta } = computeNetworkDelta(1120, 2180, prevRx, prevTx));
    expect(delta).toBe(50);
    // Ensure cumulative total is not pushed: cumulative total after 3 ticks = 3300, but deltas sum = 0+250+50=300
    // This test documents the intended non-cumulative behavior
  });

  it("handles counter reset (negative drx/dtx) by using current total and not stalling at 0", () => {
    // daemon restart resets counters to small values; naive drx would be negative
    const prevRx = 5000;
    const prevTx = 7000;
    const { delta } = computeNetworkDelta(100, 200, prevRx, prevTx);
    // Component: if drx<0 || dtx<0 then delta = max(0, rx+tx) = 300
    expect(delta).toBe(300);
  });

  it("clamps negative delta to 0", () => {
    // if one counter goes backwards but total still positive, we clamp via max(0,...)
    // Already tested reset case; also test that small negative sum without reset fallback still clamps
    // This is technically covered by Math.max(0, drx+dtx) when both negative small
    const prevRx = 100;
    const prevTx = 100;
    // rx goes 90 (-10), tx 95 (-5) => drx+dtx -15 => max 0, but then drx<0 triggers reset path => rx+tx 185
    // So negative case actually takes reset path; direct negative without reset shouldn't happen in code
    // Verify that normal positive delta never negative
    const { delta } = computeNetworkDelta(90, 95, prevRx, prevTx);
    expect(delta).toBe(185); // because reset branch triggers
  });

  it("handles independently reset counters without adding the other lifetime total", () => {
    expect(computeNetworkDelta(100, 7150, 5000, 7000).delta).toBe(250);
    expect(computeNetworkDelta(5150, 100, 5000, 7000).delta).toBe(250);
  });

  it("integration: stats websocket pushes deltas not cumulative totals into history (via Chart points)", async () => {
    fetchServerLogsMock.mockResolvedValue("");
    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );
    // wait for mount
    await waitFor(() => expect(managerInstances.length).toBe(2));

    const statsMgr = getStatsManager();
    const onMessage = statsMgr?.config.onMessage as (data: unknown) => void;
    expect(onMessage).toBeDefined();

    // Feed three stats ticks: cumulative totals 1000/2000, 1100/2150, 1120/2180
    // Expected deltas: 0, 250, 50
    await act(async () => {
      onMessage({ cpuPercent: 10, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 1000, networkTxBytes: 2000 });
      onMessage({ cpuPercent: 12, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 1100, networkTxBytes: 2150 });
      onMessage({ cpuPercent: 11, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 1120, networkTxBytes: 2180 });
    });

    // Verify network chart's history via its SVG points reflects deltas not totals
    await waitFor(() => {
      const networkChart = document.querySelector('[aria-label="Network chart"]') as HTMLElement | null;
      expect(networkChart).toBeTruthy();
      const polyline = networkChart?.querySelector("polyline");
      expect(polyline).toBeTruthy();
      const pointsAttr = polyline?.getAttribute("points") ?? "";
      const pointTokens = pointsAttr.trim().split(/\s+/).filter(Boolean);
      expect(pointTokens.length).toBe(3);
      const ys = pointTokens.map((p) => Number(p.split(",")[1]));
      // first y 100 (0 value), second near 8 (250), third near 81.6 (50)
      expect(ys[0]).toBeCloseTo(100, 0);
      expect(ys[1]).toBeCloseTo(8, 0);
      expect(ys[2]).toBeCloseTo(81.6, 0);
    });
  });

  it("integration: counter reset does not stall network chart", async () => {
    fetchServerLogsMock.mockResolvedValue("");
    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const statsMgr = getStatsManager();
    const onMessage = statsMgr?.config.onMessage as (data: unknown) => void;
    await act(async () => {
      onMessage({ cpuPercent: 10, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 5000, networkTxBytes: 7000 });
      onMessage({ cpuPercent: 10, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 100, networkTxBytes: 200 });
    });
    await waitFor(() => {
      const networkChart = document.querySelector('[aria-label="Network chart"]') as HTMLElement;
      expect(networkChart).toBeTruthy();
      const polyline = networkChart.querySelector("polyline")!;
      const attr = polyline.getAttribute("points") ?? "";
      const points = attr.trim().split(/\s+/).filter(Boolean).map((p) => Number(p.split(",")[1]));
      expect(points.length).toBe(2);
      // values [0,300] => max = max(300,100)=300 => y for 300 => 8, for 0 =>100
      expect(points[0]).toBeCloseTo(100, 0);
      expect(points[1]).toBeCloseTo(8, 0);
    });
  });

  it("resets telemetry and the network baseline when switching servers", async () => {
    const first = makeServer({ id: "first", status: "running" });
    const second = makeServer({ id: "second", status: "running" });
    const view = (server: typeof first) => (
      <ServerProvider value={{ server, access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={server} />
      </ServerProvider>
    );
    const { rerender } = renderWithQuery(view(first));
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const tick = getStatsManager().config.onMessage!;
    await act(async () => {
      tick({ cpuPercent: 10, memoryBytes: 10, memoryLimit: 100, networkRxBytes: 1000, networkTxBytes: 1000 });
      tick({ cpuPercent: 11, memoryBytes: 10, memoryLimit: 100, networkRxBytes: 1100, networkTxBytes: 1150 });
    });
    expect(screen.queryByText("Waiting for next sample")).not.toBeInTheDocument();
    rerender(view(second));
    await waitFor(() => expect(managerInstances.length).toBe(4));
    expect(screen.getByText("Waiting for next sample")).toBeInTheDocument();
    const newTick = managerInstances[3].config.onMessage as (data: unknown) => void;
    await act(async () => {
      newTick({ cpuPercent: 5, memoryBytes: 10, memoryLimit: 100, networkRxBytes: 5000, networkTxBytes: 5000 });
    });
    expect(screen.getByText("Waiting for next sample")).toBeInTheDocument();
    const network = document.querySelector('[aria-label="Network chart"] polyline');
    expect(network?.getAttribute("points")).toBe("0,100");
  });
});

// ---------------------------------------------------------------------------
// Auto-max with limit
// ---------------------------------------------------------------------------

describe("console-view Chart auto-max with limit (limitOr100)", () => {
  it("uses limitOr100 ceiling: max = max(observedMax, limitOr100, 1)", () => {
    expect(getChartMax([], 100)).toBe(100);
    expect(getChartMax([], undefined)).toBe(100);
    expect(getChartMax([5, 10], 100)).toBe(100); // tiny values not exaggerated
    expect(getChartMax([150, 120], 100)).toBe(150); // observed exceeds limit
    expect(getChartMax([0], 100)).toBe(100);
    expect(getChartMax([0], 0)).toBe(100); // 0 limit falls back to 100
    expect(getChartMax([0], -5)).toBe(100);
    expect(getChartMax([0], NaN)).toBe(100);
    expect(getChartMax([0, 0, 0], 50)).toBe(50);
    expect(getChartMax([80], 50)).toBe(80);
    expect(getChartMax([], 1)).toBe(1);
  });

  it("ensures max at least 1 to avoid divide-by-zero", () => {
    expect(getChartMax([], 0)).toBe(100); // fallback still >=1
    expect(getChartMax([], 0.1)).toBe(1);
  });

  it("keeps observed samples unchanged while applying a ceiling", () => {
    const samples = [1, 2];
    expect(getChartMax(samples, 100)).toBe(100);
    expect(samples).toEqual([1, 2]);
  });

  it("integration: CPU/Memory charts with limit 100 do not exaggerate tiny values", async () => {
    fetchServerLogsMock.mockResolvedValue("");
    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const statsMgr = getStatsManager();
    const onMessage = statsMgr?.config.onMessage as (data: unknown) => void;
    await act(async () => {
      onMessage({ cpuPercent: 1, memoryBytes: 10, memoryLimit: 1000, networkRxBytes: 0, networkTxBytes: 0 });
      onMessage({ cpuPercent: 2, memoryBytes: 20, memoryLimit: 1000, networkRxBytes: 0, networkTxBytes: 0 });
    });
    await waitFor(() => {
      const cpuChart = document.querySelector('[aria-label="CPU chart"]') as HTMLElement;
      expect(cpuChart).toBeTruthy();
      const polyline = cpuChart.querySelector("polyline")!;
      const attr = polyline.getAttribute("points") ?? "";
      const points = attr.trim().split(/\s+/).filter(Boolean).map((p) => Number(p.split(",")[1]));
      expect(points.length).toBe(2);
      // With limit 100, tiny values 1,2 should map to y ~ 99 and ~98 (not exaggerated)
      expect(points[0]).toBeCloseTo(99.08, 0);
      expect(points[1]).toBeCloseTo(98.16, 0);
      expect(points[0]).not.toBeCloseTo(54, 0);
    });
  });

  it("integration: chart with values exceeding limit scales to observed max", async () => {
    fetchServerLogsMock.mockResolvedValue("");
    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const statsMgr = getStatsManager();
    const onMessage = statsMgr?.config.onMessage as (data: unknown) => void;
    await act(async () => {
      onMessage({ cpuPercent: 150, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 0, networkTxBytes: 0 });
      onMessage({ cpuPercent: 10, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 100, networkTxBytes: 100 });
    });
    await waitFor(() => {
      const cpuChart = document.querySelector('[aria-label="CPU chart"]') as HTMLElement;
      expect(cpuChart).toBeTruthy();
      const polyline = cpuChart.querySelector("polyline")!;
      const attr = polyline.getAttribute("points") ?? "";
      const points = attr.trim().split(/\s+/).filter(Boolean).map((p) => Number(p.split(",")[1]));
      expect(points.length).toBe(2);
      // For max 150: y for 150 = 8, y for 10 = 93.87
      expect(points[0]).toBeCloseTo(8, 0);
      expect(points[1]).toBeCloseTo(93.87, 0);
    });
  });

  it("caps history to MAX_POINTS (60) — slice(-(MAX_POINTS-1))", async () => {
    const src = readFileSync(resolve(__dirname, "../components/server/console-view.tsx"), "utf8");
    expect(src).toContain("MAX_POINTS = 60");
    expect(src).toContain("slice(-(MAX_POINTS - 1))");
    fetchServerLogsMock.mockResolvedValue("");
    renderWithQuery(
      <ServerProvider value={{ server: makeServer({ status: "running" }), access: makeServerAccess(), refreshServer: async () => {} }}>
        <ConsoleView server={makeServer({ status: "running" })} />
      </ServerProvider>,
    );
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const statsMgr = getStatsManager();
    const onMessage = statsMgr?.config.onMessage as (data: unknown) => void;
    await act(async () => {
      for (let i = 0; i < 70; i++) {
        onMessage({ cpuPercent: i, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: i * 10, networkTxBytes: i * 5 });
      }
    });
    await waitFor(() => {
      const cpuChart = document.querySelector('[aria-label="CPU chart"]') as HTMLElement;
      expect(cpuChart).toBeTruthy();
      const polyline = cpuChart.querySelector("polyline")!;
      const attr = polyline.getAttribute("points") ?? "";
      const points = attr.trim().split(/\s+/).filter(Boolean);
      expect(points.length).toBe(60);
    });
  });
});

// ---------------------------------------------------------------------------
// Frame decoding — the shapes beacon actually puts on the wire
// ---------------------------------------------------------------------------

describe("console frame decoding", () => {
  it("decodes an output frame object, never stringifying it", () => {
    // WebSocketManager parses JSON before handing the frame over, so the
    // console receives an object. Stringifying it produced "[object Object]".
    const frame = decodeConsoleFrame({ serverId: "s1", type: "output", data: "hello world" });
    expect(frame).toEqual({ kind: "output", text: "hello world" });
    expect(JSON.stringify(frame)).not.toContain("[object Object]");
  });

  it("decodes a lifecycle state frame without defaulting absent fields to zero", () => {
    const frame = decodeConsoleFrame({ serverId: "s1", type: "state", exists: true, running: false, status: "exited" }, 1000);
    expect(frame).toEqual({
      kind: "state",
      lifecycle: { exists: true, running: false, status: "exited", startedAt: null, uptimeMs: null, observedAt: 1000 },
    });
  });

  it("decodes an error frame and keeps the daemon's code", () => {
    expect(decodeConsoleFrame({ serverId: "s1", type: "error", code: "inspect_failed", data: "docker unreachable" }))
      .toEqual({ kind: "error", code: "inspect_failed", message: "docker unreachable" });
    // The legacy auth failure frame carries no code.
    expect(decodeConsoleFrame({ type: "error", data: "token not valid for this server" }))
      .toEqual({ kind: "error", code: "stream_error", message: "token not valid for this server" });
    // The proxy's permission rejection is a bare {error} object.
    expect(decodeConsoleFrame({ error: "missing server permission: control.console" }))
      .toEqual({ kind: "error", code: "stream_error", message: "missing server permission: control.console" });
  });

  it("treats a raw string as output rather than dropping it", () => {
    expect(decodeConsoleFrame("not json but still output")).toEqual({ kind: "output", text: "not json but still output" });
    expect(decodeConsoleFrame("")).toEqual({ kind: "ignored" });
    expect(decodeConsoleFrame(null)).toEqual({ kind: "ignored" });
    expect(decodeConsoleFrame({ serverId: "s1" })).toEqual({ kind: "ignored" });
  });

  it("reads a stats sample and separates it from a metrics-less lifecycle report", () => {
    const sample = decodeStatsFrame({ serverId: "s1", running: true, metrics: true, cpuPercent: 4, memoryBytes: 1, memoryLimit: 2, networkRxBytes: 0, networkTxBytes: 0 }, 5);
    expect(sample.kind).toBe("sample");
    expect(sample.kind === "sample" && sample.lifecycle?.running).toBe(true);

    const stopped = decodeStatsFrame({ serverId: "s1", exists: true, running: false, status: "exited", metrics: false }, 5);
    expect(stopped).toEqual({
      kind: "lifecycle",
      lifecycle: { exists: true, running: false, status: "exited", startedAt: null, uptimeMs: null, observedAt: 5 },
    });

    expect(decodeStatsFrame({ serverId: "s1", type: "error", code: "stats_unavailable", data: "no stats stream" }))
      .toEqual({ kind: "error", code: "stats_unavailable", message: "no stats stream" });
  });

  it("distinguishes transport health from workload lifecycle", () => {
    const base = { permitted: true, blockedReason: null, link: "connected" as const, streamError: null, lifecycle: null };
    const running = { exists: true, running: true, status: "running", startedAt: null, uptimeMs: null, observedAt: 0 };
    const stopped = { ...running, running: false, status: "exited" };

    expect(deriveSessionState({ ...base, permitted: false })).toBe("forbidden");
    expect(deriveSessionState({ ...base, blockedReason: "suspended" })).toBe("unavailable");
    expect(deriveSessionState({ ...base, link: "connecting" })).toBe("connecting");
    expect(deriveSessionState({ ...base, link: "reconnecting" })).toBe("reconnecting");
    expect(deriveSessionState({ ...base, link: "disconnected" })).toBe("disconnected");
    expect(deriveSessionState({ ...base, streamError: "inspect failed" })).toBe("stream-error");
    // A stopped workload on a healthy socket is not a broken connection.
    expect(deriveSessionState({ ...base, lifecycle: stopped })).toBe("workload-stopped");
    expect(deriveSessionState({ ...base, lifecycle: running })).toBe("connected");
  });
});

// ---------------------------------------------------------------------------
// Integration: real frames end to end
// ---------------------------------------------------------------------------

function mount(server = makeServer({ status: "running" })) {
  return renderWithQuery(
    <ServerProvider value={{ server, access: makeServerAccess(), refreshServer: async () => {} }}>
      <ConsoleView server={server} />
    </ServerProvider>,
  );
}

describe("console-view against real daemon frames", () => {
  it("renders output frames as text and never as [object Object]", async () => {
    mount();
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const onMessage = getConsoleManager().config.onMessage!;
    await act(async () => {
      onMessage({ serverId: "s1", type: "output", data: "[10:00:00] Done (3.2s)! For help, type \"help\"" });
    });
    expect(await screen.findByText('[10:00:00] Done (3.2s)! For help, type "help"')).toBeInTheDocument();
    expect(screen.queryByText("[object Object]")).not.toBeInTheDocument();
  });

  it("reports a stopped workload as stopped and refuses to accept commands", async () => {
    mount();
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const onMessage = getConsoleManager().config.onMessage!;
    await act(async () => {
      onMessage({ serverId: "s1", type: "state", exists: true, running: false, status: "exited" });
    });
    expect(await screen.findByText("Workload stopped")).toBeInTheDocument();
    expect(screen.getByLabelText("Console command")).toBeDisabled();
    expect(screen.getByPlaceholderText("The workload is not running")).toBeInTheDocument();
  });

  it("surfaces a stream error with a way to reconnect", async () => {
    mount();
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const onMessage = getConsoleManager().config.onMessage!;
    await act(async () => {
      onMessage({ serverId: "s1", type: "error", code: "inspect_failed", data: "docker daemon unreachable" });
    });
    expect(await screen.findByText("Stream error")).toBeInTheDocument();
    expect(screen.getByText("docker daemon unreachable")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reconnect" })).toBeInTheDocument();
  });

  it("shows telemetry as not running instead of zeros when the node reports no metrics", async () => {
    mount();
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const tick = getStatsManager().config.onMessage!;
    await act(async () => {
      tick({ serverId: "s1", exists: true, running: false, status: "exited", metrics: false });
    });
    await waitFor(() => expect(screen.getAllByText("Not running").length).toBeGreaterThanOrEqual(2));
    // Nothing was plotted: a stopped workload contributes no sample.
    expect(document.querySelector('[aria-label="CPU chart"] polyline')?.getAttribute("points")).toBe("");
    expect(screen.queryByText("0 B")).not.toBeInTheDocument();
  });

  it("clears a live sample when the workload stops rather than leaving a stale reading", async () => {
    mount();
    await waitFor(() => expect(managerInstances.length).toBe(2));
    const tick = getStatsManager().config.onMessage!;
    await act(async () => {
      tick({ serverId: "s1", running: true, metrics: true, cpuPercent: 42.5, memoryBytes: 512, memoryLimit: 1024, networkRxBytes: 0, networkTxBytes: 0 });
    });
    expect(await screen.findByText("42.5%")).toBeInTheDocument();
    await act(async () => {
      tick({ serverId: "s1", exists: true, running: false, status: "exited", metrics: false });
    });
    await waitFor(() => expect(screen.queryByText("42.5%")).not.toBeInTheDocument());
  });
});

// ---------------------------------------------------------------------------
// Operation progress — accepted is not completed
// ---------------------------------------------------------------------------

describe("console-view power operation progress", () => {
  it("tracks the durable operation the API created and reports its terminal status", async () => {
    sendPowerSignalMock.mockResolvedValue({ serverId: "s1", signal: "start", accepted: true, mode: "durable", operationId: "op-1" });
    fetchOperationMock.mockResolvedValue({ id: "op-1", status: "running", rawStatus: "running", terminal: false });

    mount(makeServer({ status: "stopped" }));
    await waitFor(() => expect(managerInstances.length).toBe(2));
    await userEvent.click(screen.getByLabelText("Start workload"));

    expect(await screen.findByText("operation op-1")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("Running")).toBeInTheDocument());

    fetchOperationMock.mockResolvedValue({ id: "op-1", status: "succeeded", rawStatus: "completed", terminal: true });
    await waitFor(() => expect(screen.getByText("Completed")).toBeInTheDocument(), { timeout: 4000 });
    expect(fetchOperationMock).toHaveBeenCalledWith("op-1");
  });

  it("says progress is untrackable when the API returns no operation id", async () => {
    sendPowerSignalMock.mockResolvedValue({ serverId: "s1", signal: "start", accepted: true, mode: "queued" });
    mount(makeServer({ status: "stopped" }));
    await waitFor(() => expect(managerInstances.length).toBe(2));
    await userEvent.click(screen.getByLabelText("Start workload"));

    expect(await screen.findByText("Accepted, progress not trackable")).toBeInTheDocument();
    expect(fetchOperationMock).not.toHaveBeenCalled();
  });

  it("reports a rejected signal as failed", async () => {
    sendPowerSignalMock.mockRejectedValue(new Error("node offline"));
    mount(makeServer({ status: "stopped" }));
    await waitFor(() => expect(managerInstances.length).toBe(2));
    await userEvent.click(screen.getByLabelText("Start workload"));

    expect(await screen.findByText("Failed")).toBeInTheDocument();
    expect(screen.getByText("node offline")).toBeInTheDocument();
  });
});
