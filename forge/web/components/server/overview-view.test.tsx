import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import type { ReactElement } from "react";
import { OverviewView } from "./overview-view";
import { jsonResponse, mockFetchByUrl, requestJSON } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";
import type { ApiServer } from "@/lib/api";
import { ServerProvider } from "./server-context";

const server: ApiServer = {
  id: "s1",
  name: "Survival SMP",
  description: "A community survival server.",
  owner: "owner@example.com",
  ownerEmail: "owner@example.com",
  nodeId: "n1",
  node: "Ubuntu Demo Node",
  status: "running",
  desiredState: "running",
  actualState: "running",
  allocationId: "a1",
  dockerImage: "itzg/minecraft-server:java21",
  memoryMb: 4096,
  diskMb: 20480,
  cpuLimit: 100,
  databaseLimit: 2,
  backupLimit: 3,
  allocationLimit: 3,
  createdAt: "2025-09-20T14:22:00Z",
  generation: 0,
};

function renderServer(ui: ReactElement) {
  return renderWithQuery(
    <ServerProvider value={{ server, access: { user: null, permissions: ["*"], isOwner: true, isAdmin: false }, refreshServer: async () => {} }}>
      {ui}
    </ServerProvider>
  );
}

function installOverviewFetch() {
  return mockFetchByUrl({
    "/nodes/n1": jsonResponse({ id: "n1", name: "Ubuntu Demo Node", heartbeatState: "healthy", lastSeenAt: new Date().toISOString() }),
    "/servers/s1/allocations": jsonResponse([{ id: "a1", ip: "192.168.1.10", port: 25565, isPrimary: true }]),
    "/servers/s1/activity?page=1&per_page=50": jsonResponse({
      data: [{ id: "e1", action: "server.start", actorEmail: "owner@example.com", targetType: "server", targetId: "s1", createdAt: "2025-09-23T09:14:00Z" }],
      pagination: { page: 1, per_page: 50, total: 1, total_pages: 1 },
    }),
    "/servers/s1/stats": jsonResponse({
      cpuPercent: 12.5,
      memoryBytes: 1174405120,
      memoryLimit: 4294967296,
      diskBytes: 3355443200,
      diskLimit: 10737418240,
      networkRxBytes: 1048576,
      networkTxBytes: 2097152,
      uptime: 367782,
    }),
    "/servers/s1/startup": jsonResponse({
      startupCommand: "java -jar server.jar",
      rawStartupCommand: "java -jar server.jar",
      dockerImages: {},
      variables: [
        { name: "Minecraft Version", description: "Version", envVariable: "VANILLA_VERSION", defaultValue: "latest", serverValue: "1.21.1", isEditable: true, rules: "" },
        { name: "Server JAR", description: "JAR file", envVariable: "SERVER_JAR", defaultValue: "server.jar", serverValue: "paper-1.21.1-123.jar", isEditable: true, rules: "" },
      ],
    }),
    "/servers/s1/power": { method: "POST", response: jsonResponse({ serverId: "s1", signal: "restart", accepted: true }) },
  });
}

beforeEach(() => {
  // jsdom ships a native clipboard whose instances can't be spied reliably;
  // the copy test below asserts the visible copied state instead.
});

describe("server overview", () => {
  it("renders status, information, limits, quick actions and recent activity from real endpoints", async () => {
    installOverviewFetch();
    renderServer(<OverviewView server={server} />);

    expect(await screen.findByRole("heading", { name: "Survival SMP", level: 1 })).toBeInTheDocument();
    expect(await screen.findAllByText("192.168.1.10:25565")).not.toHaveLength(0);
    expect(screen.getAllByText("Ubuntu Demo Node").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("4,096 MiB")).toBeInTheDocument();
    expect(screen.getByText("Recent activity")).toBeInTheDocument();
    expect(await screen.findByText("start")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Console/ })).toHaveAttribute("href", "/server/s1/console");
    expect(screen.getByRole("link", { name: /Files/ })).toHaveAttribute("href", "/server/s1/files");
  });

  it("sends a restart power signal and copies the server ID", async () => {
    const mocked = installOverviewFetch();
    renderServer(<OverviewView server={server} />);
    const user = userEvent.setup();

    await screen.findAllByText("192.168.1.10:25565");
    await user.click(screen.getByRole("button", { name: "Restart" }));
    await waitFor(() => {
      const powerCall = mocked.calls.find((call) => call.url.endsWith("/servers/s1/power"));
      expect(powerCall).toBeDefined();
      expect(requestJSON(powerCall!)).toEqual({ signal: "restart" });
    });

    await user.click(screen.getByRole("button", { name: "Copy server ID" }));
    const copyButton = screen.getByRole("button", { name: "Copy server ID" });
    await waitFor(() => {
      expect(copyButton.querySelector(".text-emerald-400")).not.toBeNull();
    });
  });
});
