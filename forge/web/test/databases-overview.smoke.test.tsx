import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { renderWithQuery } from "@/test/render";
import { jsonResponse, mockFetchByUrl } from "@/test/fetch-mock";
import { DatabasesOverview } from "@/components/database/databases-overview";
import { findAdminPage } from "@/components/admin/admin-registry";

const hosts = [{ id: "h1", name: "macos-mysql", host: "127.0.0.1", port: 52982, username: "root", engine: "mysql", databases: 2 }];
const containers = [
  { id: "fdf79e8c-aaaa", engine: "mysql", version: "8.0", containerId: "", status: "pending", port: 0, volumeId: "", memoryMb: 512, cpuShares: 1024, createdAt: "2026-09-25T22:08:03+05:30", updatedAt: "" },
  // Backed by the postgres catalog instance below — must fold into the catalog row, not double-count.
  { id: "catcont0-cccc", engine: "postgresql", version: "16", containerId: "abc123", status: "running", port: 5432, volumeId: "v1", memoryMb: 512, cpuShares: 512, createdAt: "", updatedAt: "" },
];
const entries = [
  { key: "postgres", displayName: "PostgreSQL", category: "database", versions: ["16"], defaultVersion: "16", enabled: true },
];
const pgInstances = [
  { id: "ci1", entryKey: "postgres", kind: "postgresql", version: "16", refType: "db_container", instanceRef: "catcont0-cccc", host: "10.0.0.5", port: 5432, connString: "postgresql://x", status: "running", nodeId: "n1", createdAt: "" },
];
const servers = [{ id: "s1", name: "Smoke" }];
const serverDbs = [
  { id: "db1", serverId: "s1", database: "s1_game", username: "u1", engine: "mysql", host: "127.0.0.1", port: 52982, provisioningState: "ready", createdAt: "" },
];

const replace = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace, push: replace }), usePathname: () => "/admin/databases" }));

function mockAll() {
  mockFetchByUrl({
    "/database-hosts": jsonResponse(hosts),
    "/databases/containers": jsonResponse(containers),
    "/managed-databases": jsonResponse([]),
    "/admin/database-services": jsonResponse([]),
    "/catalog": jsonResponse(entries),
    "/catalog/postgres/instances": jsonResponse(pgInstances),
    "/servers?page=1&per_page=100": jsonResponse(servers),
    "/servers/s1/databases": jsonResponse(serverDbs),
  });
}

describe("databases overview smoke", () => {
  it("merges 6 sources with origin badges and folds catalog-backed containers", async () => {
    mockAll();
    const onOpenTab = vi.fn();
    const { container } = renderWithQuery(<DatabasesOverview onOpenTab={onOpenTab} />);
    // 1 host + 2 containers (one folded) + 1 catalog + 1 serverdb = 4 rows
    expect(await screen.findByText("macos-mysql")).toBeInTheDocument();
    expect((await screen.findAllByText("fdf79e8c")).length).toBeGreaterThan(0);
    expect(await screen.findByText("s1_game")).toBeInTheDocument();
    expect(await screen.findByText("postgres 16")).toBeInTheDocument();
    expect(await screen.findByText("Databases (4)")).toBeInTheDocument();
    // Folded container must not appear as its own row (its id survives only
    // inside the catalog row's runtime cross-reference).
    expect(screen.queryByText("catcont0-cccc")).toBeNull();
    expect(screen.getAllByText("Raw Container")[0]).toBeInTheDocument();
    // Cross-system rows link out instead of switching tabs.
    expect(container.querySelector('a[href="/admin/catalog"]')).not.toBeNull();
    expect(container.querySelector('a[href="/server/s1/databases"]')).not.toBeNull();
  });

  it("labels the unified page Databases, not Database Hosts", () => {
    expect(findAdminPage("/admin/databases")?.label).toBe("Databases");
    expect(findAdminPage("/admin/database-services")?.href).toBe("/admin/databases");
  });
});
