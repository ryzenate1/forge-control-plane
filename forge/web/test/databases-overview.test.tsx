import { describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn(), replace: vi.fn() }) }));
import userEvent from "@testing-library/user-event";
import { renderWithQuery } from "@/test/render";
import { jsonResponse, mockFetchByUrl } from "@/test/fetch-mock";
import { DatabasesOverview } from "@/components/database/databases-overview";

const hosts = [{ id: "h1", name: "macos-mysql", host: "127.0.0.1", port: 52982, username: "root", engine: "mysql" }];
const containers = [
  { id: "fdf79e8c-aaaa", engine: "mysql", version: "8.0", containerId: "", status: "pending", port: 0, volumeId: "", memoryMb: 512, cpuShares: 1024, createdAt: "2026-09-25T10:00:00Z", updatedAt: "" },
];

function route() {
  mockFetchByUrl({
    "/database-hosts": jsonResponse(hosts),
    "/databases/containers": jsonResponse(containers),
    "/managed-databases": jsonResponse([]),
    "/admin/database-services": jsonResponse([]),
  });
}

describe("databases overview", () => {
  it("renders KPI counts and unified rows", async () => {
    route();
    renderWithQuery(<DatabasesOverview onOpenTab={() => {}} />);
    expect(await screen.findByText("macos-mysql")).toBeInTheDocument();
    expect(await screen.findByText("fdf79e8c")).toBeInTheDocument();
    const hostRow = screen.getByText("macos-mysql").closest("tr")!;
    expect(within(hostRow).getByText("External Host")).toBeInTheDocument();
    const containerRow = screen.getByText("fdf79e8c").closest("tr")!;
    expect(within(containerRow).getByText("Raw Container")).toBeInTheDocument();
  });

  it("filters rows by search", async () => {
    route();
    renderWithQuery(<DatabasesOverview onOpenTab={() => {}} />);
    expect(await screen.findByText("macos-mysql")).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText("Search databases"), "macos");
    expect(screen.queryByText("fdf79e8c")).not.toBeInTheDocument();
    expect(screen.getByText("macos-mysql")).toBeInTheDocument();
  });
});
