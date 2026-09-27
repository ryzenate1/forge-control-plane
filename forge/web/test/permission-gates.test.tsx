import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TransferView } from "@/components/server/transfer-view";
import { DeploymentsView } from "@/components/server/deployments-view";
import { SchedulesView } from "@/components/server/schedules-view";
import { FilesView } from "@/components/server/files-view";
import GitDeployPage from "@/app/server/[id]/git/page";
import ServerDatabaseServicesPage from "@/app/server/[id]/databases/services/page";
import { jsonResponse, mockFetchByUrl } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";
import { ServerProvider } from "@/components/server/server-context";
import { fixtureServer, makeSchedule, makeTask, makeServerAccess } from "@/test/fixtures";
import type { ReactElement } from "react";

const replace = vi.fn();
const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace, push }), useParams: () => ({ id: "s1" }) }));
vi.mock("next/dynamic", () => ({ default: () => function Editor() { return <textarea aria-label="File content" />; } }));

function renderWithAccess(ui: ReactElement, access = makeServerAccess()) {
  return renderWithQuery(
    <ServerProvider value={{ server: fixtureServer, access, refreshServer: async () => {} }}>
      {ui}
    </ServerProvider>
  );
}

beforeEach(() => {
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } });
});

describe("transfer permission gate (settings.rename + admin)", () => {
  it("shows the initiate-transfer form to admins with settings.rename", async () => {
    mockFetchByUrl({
      "/servers/s1/transfer": jsonResponse({}),
      "/nodes": jsonResponse([]),
    });
    renderWithAccess(<TransferView server={fixtureServer} />, makeServerAccess({ isAdmin: true, permissions: ["settings.rename"] }));
    expect(await screen.findByText("Initiate transfer")).toBeInTheDocument();
  });

  it("hides the transfer form and cancel action from non-admin members", async () => {
    mockFetchByUrl({
      "/servers/s1/transfer": jsonResponse({}),
      "/nodes": jsonResponse([]),
    });
    renderWithAccess(<TransferView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["settings.rename"] }));
    expect(await screen.findByText("No active transfer")).toBeInTheDocument();
    expect(screen.queryByText("Initiate transfer")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Start transfer" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel transfer" })).not.toBeInTheDocument();
  });
});

describe("deployment permission gate (settings.reinstall)", () => {
  const liveRelease = {
    id: "r1",
    serverId: "s1",
    version: 3,
    imageTag: "myapp:v3",
    status: "live",
    createdAt: "2026-01-01T00:00:00Z",
    completedAt: "2026-01-01T00:05:00Z",
  };

  function mockDeployments() {
    return mockFetchByUrl({
      "/servers/s1/deployments": jsonResponse({ data: [liveRelease] }),
      "/servers/s1/health-check": jsonResponse({ data: { path: "/health", port: 8080, protocol: "http", intervalSeconds: 10, timeoutSeconds: 5, healthyThreshold: 2, unhealthyThreshold: 3 } }),
    });
  }

  it("disables deploy and hides rollback without the permission", async () => {
    mockDeployments();
    renderWithAccess(<DeploymentsView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["activity.read"] }));
    const deploy = await screen.findByRole("button", { name: "Deploy" });
    expect(deploy).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Rollback" })).not.toBeInTheDocument();
  });

  it("enables deploy and shows rollback with the permission", async () => {
    mockDeployments();
    renderWithAccess(<DeploymentsView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["settings.reinstall"] }));
    const input = await screen.findByPlaceholderText(/Docker image tag/);
    await userEvent.type(input, "myapp:v3");
    expect(screen.getByRole("button", { name: "Deploy" })).toBeEnabled();
    expect(await screen.findByRole("button", { name: "Rollback" })).toBeInTheDocument();
  });
});

describe("schedule task delete gate (schedule.delete)", () => {
  function mockSchedules() {
    return mockFetchByUrl({
      "/servers/s1/schedules": jsonResponse([makeSchedule({}, [makeTask()])]),
    });
  }

  it("disables task delete without the permission", async () => {
    mockSchedules();
    renderWithAccess(<SchedulesView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["schedule.update"] }));
    expect(await screen.findByLabelText("Delete task")).toBeDisabled();
    expect(screen.getByLabelText("Delete Daily backup")).toBeDisabled();
  });

  it("enables task delete with the permission", async () => {
    mockSchedules();
    renderWithAccess(<SchedulesView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["schedule.update", "schedule.delete"] }));
    expect(await screen.findByLabelText("Delete task")).toBeEnabled();
  });
});

describe("file action gates (file.archive, file.read-content)", () => {
  const entries = [
    { name: "mods.zip", path: "mods.zip", directory: false, size: 100, modifiedAt: "2026-01-01T00:00:00Z" },
    { name: "readme.txt", path: "readme.txt", directory: false, size: 5, modifiedAt: "2026-01-01T00:00:00Z" },
  ];

  it("disables archive/extract without file.archive", async () => {
    mockFetchByUrl({
      "/servers/s1/files": jsonResponse(entries),
    });
    renderWithAccess(<FilesView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["file.read"] }));
    expect(await screen.findByLabelText("Extract archive")).toBeDisabled();
    expect(screen.getAllByLabelText("Archive entry")[0]).toBeDisabled();
  });

  it("refuses to open file contents without file.read-content", async () => {
    mockFetchByUrl({
      "/servers/s1/files": jsonResponse(entries),
    });
    renderWithAccess(<FilesView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["file.read"] }));
    await userEvent.click(await screen.findByText("readme.txt"));
    expect(await screen.findByText("You do not have permission to view file contents on this server.")).toBeInTheDocument();
  });

  it("enables extract with file.archive", async () => {
    mockFetchByUrl({
      "/servers/s1/files": jsonResponse(entries),
    });
    renderWithAccess(<FilesView server={fixtureServer} />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["file.read", "file.archive"] }));
    expect(await screen.findByLabelText("Extract archive")).toBeEnabled();
  });
});

describe("git hooks page owner/admin gate", () => {
  const hook = { id: "hook-12345678", gitSourceId: "", secret: "s", events: ["push"], createdAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z" };

  it("disables hook deletion and shows the notice for plain members", async () => {
    mockFetchByUrl({
      "/git/servers/s1/deployments": jsonResponse([]),
      "/git/servers/s1/hooks": jsonResponse([hook]),
    });
    renderWithAccess(<GitDeployPage />, makeServerAccess({ isOwner: false, isAdmin: false, permissions: ["activity.read"] }));
    expect(await screen.findByRole("button", { name: "Delete hook" })).toBeDisabled();
    expect(screen.getByText("Only the server owner or an administrator can delete hooks.")).toBeInTheDocument();
  });

  it("enables hook deletion for the owner", async () => {
    mockFetchByUrl({
      "/git/servers/s1/deployments": jsonResponse([]),
      "/git/servers/s1/hooks": jsonResponse([hook]),
    });
    renderWithAccess(<GitDeployPage />, makeServerAccess({ isOwner: true, isAdmin: false }));
    expect(await screen.findByRole("button", { name: "Delete hook" })).toBeEnabled();
  });
});

describe("database services page admin gate", () => {
  it("hides Link Service and shows the notice for non-admins", async () => {
    mockFetchByUrl({
      "/servers/s1/database-services": jsonResponse([]),
    });
    renderWithAccess(<ServerDatabaseServicesPage />, makeServerAccess({ isAdmin: false, isOwner: true }));
    expect(await screen.findByText("No database services linked to this server.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Link Service/ })).not.toBeInTheDocument();
    expect(screen.getByText("Only administrators can link, unlink, back up, or view credentials for managed database services.")).toBeInTheDocument();
  });

  it("shows Link Service to admins", async () => {
    mockFetchByUrl({
      "/servers/s1/database-services": jsonResponse([]),
    });
    renderWithAccess(<ServerDatabaseServicesPage />, makeServerAccess({ isAdmin: true }));
    expect(await screen.findByRole("button", { name: /Link Service/ })).toBeInTheDocument();
  });
});