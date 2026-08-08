import { describe, expect, it, vi, afterEach } from "vitest";
import { jsonResponse, mockFetchByUrl } from "../../test/fetch-mock";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("backup policy API client", () => {
  describe("listBackupProviders", () => {
    it("returns providers list", async () => {
      mockFetchByUrl({
        "/backup/providers": jsonResponse({ providers: ["local", "s3"] }),
      });

      const { listBackupProviders } = await import("./backup");
      const result = await listBackupProviders();
      expect(result.providers).toEqual(["local", "s3"]);
    });
  });

  describe("listBackupPolicies", () => {
    it("returns policies for a server", async () => {
      mockFetchByUrl({
        "/servers/s1/backups/policies": jsonResponse({
          policies: [{ id: "pol1", serverId: "s1", interval: "24h", maxBackups: 5, retentionDays: 30, storage: "local", compress: true, enabled: true, createdAt: "2024-01-01", updatedAt: "2024-01-01" }],
        }),
      });

      const { listBackupPolicies } = await import("./backup");
      const result = await listBackupPolicies("s1");
      expect(result.policies).toHaveLength(1);
      expect(result.policies[0].id).toBe("pol1");
    });
  });

  describe("createBackupPolicy", () => {
    it("sends POST with policy config", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups/policies": jsonResponse({
          policy: { id: "new-pol", serverId: "s1" },
        }),
      });

      const { createBackupPolicy } = await import("./backup");
      const result = await createBackupPolicy("s1", {
        interval: "48h",
        maxBackups: 10,
        retentionDays: 60,
        storage: "local",
      });
      expect(result.policy.id).toBe("new-pol");
      expect(calls[0].init.method).toBe("POST");
      const body = JSON.parse(String(calls[0].init.body));
      expect(body.interval).toBe("48h");
      expect(body.maxBackups).toBe(10);
    });
  });

  describe("deleteBackupPolicy", () => {
    it("sends DELETE to correct path", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups/policies/pol1": jsonResponse(null, 204),
      });

      const { deleteBackupPolicy } = await import("./backup");
      await deleteBackupPolicy("s1", "pol1");
      expect(calls[0].init.method).toBe("DELETE");
      expect(calls[0].url).toContain("policies/pol1");
    });
  });

  describe("triggerBackup", () => {
    it("sends POST with optional ignored files", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups": jsonResponse({ uuid: "b-uuid", name: "backup-1", status: "running" }),
      });

      const { triggerBackup } = await import("./backup");
      const result = await triggerBackup("s1", ["*.log"]);
      expect(result.status).toBe("running");
      expect(result.uuid).toBe("b-uuid");
      const body = JSON.parse(String(calls[0].init.body));
      expect(body.ignored).toEqual(["*.log"]);
    });

    it("sends POST without ignored when not provided", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups": jsonResponse({ uuid: "b2", name: "backup-2", status: "pending" }),
      });

      const { triggerBackup } = await import("./backup");
      await triggerBackup("s1");
      const body = JSON.parse(String(calls[0].init.body));
      expect(body).toEqual({});
    });
  });

  describe("cleanupExpiredBackups", () => {
    it("sends POST to cleanup endpoint", async () => {
      const { calls } = mockFetchByUrl({
        "/servers/s1/backups/cleanup": jsonResponse({ ok: true, cleaned: 3 }),
      });

      const { cleanupExpiredBackups } = await import("./backup");
      const result = await cleanupExpiredBackups("s1");
      expect(result.cleaned).toBe(3);
      expect(calls[0].init.method).toBe("POST");
    });
  });

  describe("error handling", () => {
    it("propagates API errors", async () => {
      mockFetchByUrl({
        "/backup/providers": jsonResponse({ error: "unavailable" }, 503),
      });

      const { listBackupProviders } = await import("./backup");
      await expect(listBackupProviders()).rejects.toThrow();
    });

    it("handles network failures", async () => {
      mockFetchByUrl({
        "/servers/s1/backups/policies": () => {
          throw new TypeError("Failed to fetch");
        },
      });

      const { listBackupPolicies } = await import("./backup");
      await expect(listBackupPolicies("s1")).rejects.toThrow("Network error");
    });
  });
});
