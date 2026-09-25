import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { AdminActivityLog } from "@/components/admin/AdminActivityLog";
import { jsonResponse } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn(), forward: vi.fn(), refresh: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/admin/activity",
  useSearchParams: () => new URLSearchParams(),
}));

beforeEach(() => {
  vi.restoreAllMocks();
});

function activityEvent(overrides: Record<string, unknown> = {}) {
  return {
    id: "e1",
    event: "server.created",
    description: "Server created",
    actorEmail: "admin@forge.local",
    actorType: "user",
    ip: "127.0.0.1",
    subjectType: "server",
    subjectId: "s1",
    subjectName: "mc-1",
    properties: { plan: "small" },
    level: "info",
    source: "api",
    timestamp: "2026-09-22T12:00:00Z",
    ...overrides,
  };
}

function installActivityFetch(events: unknown[], total: number, stats: unknown = {}) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes("/admin/activity/stats")) return jsonResponse(stats);
    if (url.includes("/admin/activity")) return jsonResponse({ events, total });
    throw new Error(`Unexpected fetch: ${url}`);
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const baseStats = {
  totalEvents: 120,
  eventsToday: 12,
  eventsThisHour: 2,
  uniqueActors: 3,
  byLevel: { info: 100, warning: 12, error: 6, critical: 2 },
};

describe("AdminActivityLog", () => {
  it("renders header, KPI strip and empty state when no events exist", async () => {
    installActivityFetch([], 0, baseStats);

    renderWithQuery(<AdminActivityLog />);

    expect(await screen.findByRole("heading", { name: "Activity", level: 1 })).toBeInTheDocument();
    expect(screen.getByText("Total events")).toBeInTheDocument();
    expect(await screen.findByText("120")).toBeInTheDocument();
    expect(screen.getByText("By level")).toBeInTheDocument();
    expect(await screen.findByText("No Events")).toBeInTheDocument();
  });

  it("renders event rows with level pills and opens the detail drawer on click", async () => {
    installActivityFetch(
      [activityEvent(), activityEvent({ id: "e2", event: "node.heartbeat.failed", level: "error", actorEmail: null, subjectName: "node-2" })],
      2,
      baseStats,
    );

    renderWithQuery(<AdminActivityLog />);
    const user = userEvent.setup();

    expect(await screen.findByText("server.created")).toBeInTheDocument();
    expect(screen.getByText("mc-1")).toBeInTheDocument();
    expect(screen.getByText("node.heartbeat.failed")).toBeInTheDocument();
    expect(screen.getAllByText("error").length).toBeGreaterThanOrEqual(1);

    await user.click(screen.getByText("server.created"));
    expect(await screen.findByText("Event detail")).toBeInTheDocument();
    expect(screen.getByText("Event ID")).toBeInTheDocument();
    expect(screen.getByText(/"plan": "small"/)).toBeInTheDocument();
  });

  it("applies level filter to the query and paginates", async () => {
    const fetchMock = installActivityFetch([activityEvent()], 65, baseStats);

    renderWithQuery(<AdminActivityLog />);
    const user = userEvent.setup();

    expect(await screen.findByText("server.created")).toBeInTheDocument();
    expect(screen.getByText("Page 1 of 2")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Error" }));
    await user.click(screen.getByRole("button", { name: "Next" }));

    const levelCalls = fetchMock.mock.calls
      .map(([input]) => String(input))
      .filter((url) => url.includes("/admin/activity?") || url.includes("/admin/activity?"));
    expect(levelCalls.some((url) => url.includes("level=error"))).toBe(true);
    expect(levelCalls.some((url) => url.includes("offset=50"))).toBe(true);
  });
});
