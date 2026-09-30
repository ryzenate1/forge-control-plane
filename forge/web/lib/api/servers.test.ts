import { describe, expect, it, vi, afterEach } from "vitest";
import { jsonResponse, mockFetchByUrl } from "../../test/fetch-mock";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("servers API client", () => {
  describe("fetchServers", () => {
    it("returns servers from a single page", async () => {
      mockFetchByUrl({
        "/servers?page=1&per_page=100": jsonResponse({
          data: [{ id: "s1", name: "MC" }],
          meta: { pagination: { total: 1, current_page: 1, per_page: 100, total_pages: 1 } },
        }),
      });

      const { fetchServers } = await import("./servers");
      const servers = await fetchServers();
      expect(servers).toHaveLength(1);
      expect(servers[0].id).toBe("s1");
    });

    it("fetches multiple pages when total > 1", async () => {
      const { addRoute } = mockFetchByUrl({});

      addRoute(
        (url) => url.includes("page=1&per_page=100"),
        jsonResponse({
          data: [{ id: "s1" }],
          meta: { pagination: { total: 2, current_page: 1, per_page: 100, total_pages: 2 } },
        }),
      );
      addRoute(
        (url) => url.includes("page=2&per_page=100"),
        jsonResponse({
          data: [{ id: "s2" }],
          meta: { pagination: { total: 2, current_page: 2, per_page: 100, total_pages: 2 } },
        }),
      );

      const { fetchServers } = await import("./servers");
      const servers = await fetchServers();
      expect(servers).toHaveLength(2);
      expect(servers.map((s) => s.id)).toEqual(["s1", "s2"]);
    });
  });

  describe("fetchServer", () => {
    it("returns a single server", async () => {
      mockFetchByUrl({
        "/servers/s1": jsonResponse({ id: "s1", name: "Test Server" }),
      });

      const { fetchServer } = await import("./servers");
      const server = await fetchServer("s1");
      expect(server.id).toBe("s1");
    });
  });

  describe("createServer", () => {
    it("sends POST with server input", async () => {
      const { calls } = mockFetchByUrl({
        "/servers": jsonResponse({ id: "new-1" }),
      });

      const { createServer } = await import("./servers");
      const result = await createServer({ name: "New" } as never);
      expect(result.id).toBe("new-1");
      expect(calls[0].init.method).toBe("POST");
    });
  });

  describe("deleteServer", () => {
    it("sends DELETE without force param by default", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1": jsonResponse(null, 204),
      });

      const { deleteServer } = await import("./servers");
      await deleteServer("s1");
      expect(calls[0].init.method).toBe("DELETE");
      expect(calls[0].url).not.toContain("force=true");
    });

    it("includes force=true when force flag set", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1?force=true": jsonResponse(null, 204),
      });

      const { deleteServer } = await import("./servers");
      await deleteServer("s1", true);
      expect(calls[0].url).toContain("force=true");
    });
  });

  describe("sendPowerSignal", () => {
    it("sends POST with signal type", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/power": jsonResponse({ serverId: "s1", signal: "restart", accepted: true }),
      });

      const { sendPowerSignal } = await import("./servers");
      const result = await sendPowerSignal("s1", "restart");
      expect(result.accepted).toBe(true);
      expect(result.signal).toBe("restart");
      const body = JSON.parse(String(calls[0].init.body));
      expect(body.signal).toBe("restart");
    });
  });

  describe("fetchServerDatabases", () => {
    it("returns databases list", async () => {
      mockFetchByUrl({
        "/servers/s1/databases": jsonResponse([{ id: "db1" }]),
      });

      const { fetchServerDatabases } = await import("./servers");
      const dbs = await fetchServerDatabases("s1");
      expect(dbs).toHaveLength(1);
    });
  });

  describe("fetchBackups", () => {
    it("returns paginated backup data", async () => {
      mockFetchByUrl({
        "/servers/s1/backups?page=1&per_page=20": jsonResponse({
          data: [{ id: "b1" }],
          pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 },
        }),
      });

      const { fetchBackups } = await import("./servers");
      const result = await fetchBackups("s1");
      expect(result.data).toHaveLength(1);
      expect(result.pagination.total).toBe(1);
    });
  });

  describe("createBackup", () => {
    it("sends POST with empty body when no input", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups": jsonResponse({ id: "b1", status: "running" }),
      });

      const { createBackup } = await import("./servers");
      const result = await createBackup("s1");
      expect(result.status).toBe("running");
      expect(calls[0].init.method).toBe("POST");
    });
  });

  describe("deleteBackup", () => {
    it("sends DELETE to correct path", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups/b1": jsonResponse(null, 204),
      });

      const { deleteBackup } = await import("./servers");
      await deleteBackup("s1", "b1");
      expect(calls[0].init.method).toBe("DELETE");
      expect(calls[0].url).toContain("backups/b1");
    });
  });

  describe("restoreBackup", () => {
    it("sends POST with name and truncate", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups/restore": jsonResponse({ ok: true, status: "restoring", name: "b1" }),
      });

      const { restoreBackup } = await import("./servers");
      const result = await restoreBackup("s1", "b1", true);
      expect(result.ok).toBe(true);
      const body = JSON.parse(String(calls[0].init.body));
      expect(body.name).toBe("b1");
      expect(body.truncate).toBe(true);
    });
  });

  describe("error handling", () => {
    it("throws ApiError on API failure", async () => {
      mockFetchByUrl({
        "/servers/bad": jsonResponse({ error: "not found" }, 404),
      });

      const { fetchServer } = await import("./servers");
      await expect(fetchServer("bad")).rejects.toThrow();
    });

    it("wraps network errors", async () => {
      mockFetchByUrl({
        "/servers/s1": () => { throw new TypeError("Failed to fetch"); },
      });

      const { fetchServer } = await import("./servers");
      await expect(fetchServer("s1")).rejects.toThrow("Network error");
    });
  });
});
