import { describe, expect, it, vi } from "vitest";
import {
  assignServerMount, changePassword, connectServerWebSocket, createAdminOAuthClient, createDatabaseHost, createMigration, createNode, createRole, createServer, createServerScheduleTask,
  exportAdminActivity, fetchActivityLogs, fetchAdminActivity, fetchAllocationNodes, fetchBackups, fetchCurrentUser, fetchHealthStatus, fetchNodes, fetchPermissions, fetchPlugins, fetchRecoveryPlans, fetchReservations, fetchServer, fetchServerStartup, fetchUsers,
  fetchWebhookDeliveries, getServerFileDownloadURL, login, loginCheckpoint, logout, reinstallServer, serverWebSocketURL, setAllocationAlias, updateNode, updateServer, updateServerScheduleTask,
  deleteAllocations, deleteAllocationsBulk, deleteServerDatabase, fetchOrphanRemediations, previewEvacuation, createEvacuationPlan, createRecoveryPlan, executeRecoveryPlan, fetchRecoveryPlan, resolveDatabaseOrphanRemediation, resolveServerOrphanRemediation, startRecoveryPlan, cancelRecoveryPlan, setAdminAllocationAlias, testDatabaseHostConnection,
} from "@/lib/api";
import { jsonResponse, mockFetch, requestJSON } from "@/test/fetch-mock";



describe("authentication contracts", () => {
  it("uses cookie sessions without persisting login or checkpoint tokens", async () => {
    const { calls } = mockFetch(
      jsonResponse({ complete: false, confirmationToken: "checkpoint" }),
      jsonResponse({ complete: true, token: "jwt-final", user: { id: "u1", email: "a@example.com", role: "user" } }),
    );
    const first = await login("a@example.com", "secret");
    expect(first.confirmationToken).toBe("checkpoint");
    await loginCheckpoint("checkpoint", "123456");
    expect(requestJSON(calls[1])).toEqual({ confirmationToken: "checkpoint", code: "123456" });
  });

  it("reports a transient /auth/me failure without treating it as unauthorized", async () => {
    mockFetch(jsonResponse({ message: "gateway unavailable" }, 503));
    await expect(fetchCurrentUser()).rejects.toThrow("gateway unavailable");
  });

  it("returns null only for an explicit unauthorized /auth/me response", async () => {
    mockFetch(jsonResponse({}, 401));
    await expect(fetchCurrentUser()).resolves.toBeNull();
  });

  it("revokes the backend cookie session", async () => {
    const { calls } = mockFetch(new Response(null, { status: 204 }));
    await logout();
    expect(calls[0].url).toContain("/auth/logout");
    expect(calls[0].init?.method).toBe("POST");
    expect(new Headers(calls[0].init?.headers).get("Authorization")).toBeNull();
  });

  it("sends reinstall requests to the permission-scoped endpoint", async () => {
    const { calls } = mockFetch(jsonResponse({ accepted: true }));
    await reinstallServer("server/id");
    expect(calls[0].url).toContain("/servers/server%2Fid/reinstall");
  });

  it("sends the password contract field names", async () => {
    const { calls } = mockFetch(jsonResponse({ status: "ok" }));
    await changePassword("old", "new-password");
    expect(requestJSON(calls[0])).toEqual({ currentPassword: "old", newPassword: "new-password" });
  });
});

describe("monitoring response contracts", () => {
  it("accepts the diagnostic health report and rejects malformed monitoring collections", async () => {
    const report = { status: "ok", ok: true, service: "api", checks: [], checkedAt: "2026-07-17T00:00:00Z" };
    mockFetch(
      jsonResponse(report),
      jsonResponse({ reservations: [] }),
      jsonResponse({ plans: [] }),
    );

    await expect(fetchHealthStatus()).resolves.toEqual(report);
    await expect(fetchReservations()).rejects.toThrow("Unexpected response from /reservations: expected an array");
    await expect(fetchRecoveryPlans()).rejects.toThrow("Unexpected response from /recovery-plans: expected an array");
  });

  it("rejects partial health responses instead of rendering mock-era defaults", async () => {
    mockFetch(jsonResponse({ status: "ok" }));

    await expect(fetchHealthStatus()).rejects.toThrow("Unexpected response from /health: expected a diagnostic report");
  });
});

describe("WebSocket ticket contract", () => {
  it("uses a short-lived ticket in the URL and never places the JWT in the query", async () => {
    mockFetch(jsonResponse({ token: "short-ticket", expiresAt: "soon", server: "srv", stream: "console" }));
    const sockets: Array<{ url: string; protocols?: string | string[] }> = [];
    class FakeWebSocket {
      static OPEN = 1;
      constructor(public url: string, public protocols?: string | string[]) { sockets.push({ url, protocols }); }
    }
    vi.stubGlobal("WebSocket", FakeWebSocket);
    await connectServerWebSocket("server/id", "console");
    expect(sockets[0].url).toContain("/servers/server%2Fid/ws/console?token=short-ticket");
    expect(sockets[0].protocols).toBeUndefined(); // JWT removed from subprotocol for security
  });

  it("reports ticket issuance failures without opening a socket", async () => {
    mockFetch(jsonResponse({ message: "daemon unavailable" }, 503));
    const socket = vi.fn();
    vi.stubGlobal("WebSocket", socket);
    await expect(connectServerWebSocket("srv", "console")).rejects.toThrow("daemon unavailable");
    expect(socket).not.toHaveBeenCalled();
  });

  it("constructs relative API websocket URLs", () => {
    expect(serverWebSocketURL("srv", "stats")).toBe("/api/v1/servers/srv/ws/stats");
  });
});

describe("file download contracts", () => {
  it("uses a short-lived ticket URL instead of placing the session token in the download URL", async () => {
    const { calls } = mockFetch(jsonResponse({ url: "/api/v1/download/file?token=single-use-ticket", expires: "soon" }));
    const { url } = await getServerFileDownloadURL("server/id", "mods/game.bin");
    expect(calls[0].url).toContain("/servers/server%2Fid/files/download-url?path=mods%2Fgame.bin");
    expect(url).toContain("/download/file?token=single-use-ticket");
  });
});

describe("backend DTO and mutation field names", () => {
  it("uses distinct account and canonical admin activity endpoints", async () => {
    const events = [{ id: "event-1", event: "server.created", timestamp: "2026-01-01T00:00:00Z", subjectType: "server", subjectId: "server-1" }];
    const { calls } = mockFetch(jsonResponse(events), jsonResponse({ events, total: 1 }));
    await expect(fetchActivityLogs()).resolves.toEqual(events);
    await expect(fetchAdminActivity({ event: "server.created", level: "info", limit: 100 })).resolves.toEqual({ events, total: 1 });
    expect(calls[0].url).toContain("/account/activity");
    expect(calls[1].url).toContain("/admin/activity?event=server.created&level=info&limit=100");
  });

  it("sends every supported admin activity filter and exports the same filtered dataset", async () => {
    const filter = {
      actorId: "user-1", subjectType: "server", subjectId: "server-1", event: "server.created",
      level: "info", source: "panel", from: "2026-01-01T00:00:00.000Z", to: "2026-01-31T23:59:59.999Z", limit: 25, offset: 50,
    };
    const { calls } = mockFetch(
      jsonResponse({ events: [], total: 0 }),
      new Response("id,event\nevent-1,server.created\n", { headers: { "Content-Type": "text/csv" } }),
    );

    await fetchAdminActivity(filter);
    await expect(exportAdminActivity("csv", filter)).resolves.toBeInstanceOf(Blob);

    expect(calls[0].url).toContain("/admin/activity?actorId=user-1&subjectType=server&subjectId=server-1&event=server.created&level=info&source=panel&from=2026-01-01T00%3A00%3A00.000Z&to=2026-01-31T23%3A59%3A59.999Z&limit=25&offset=50");
    expect(calls[1].url).toContain("/admin/activity/export?actorId=user-1&subjectType=server&subjectId=server-1&event=server.created&level=info&source=panel&from=2026-01-01T00%3A00%3A00.000Z&to=2026-01-31T23%3A59%3A59.999Z&limit=25&offset=50&format=csv");
    expect(new Headers(calls[1].init?.headers).get("Accept")).toBe("text/csv");
  });

  it("parses backup, startup, server, user, and node response fields without renaming them", async () => {
    const backup = { uuid: "b1", name: "daily.zip", checksum: "sha256:abc", size: 42, status: "running", createdAt: "2026-01-01T00:00:00Z", completedAt: "" };
    const startup = { startupCommand: "java -jar app.jar", rawStartupCommand: "java -jar {{JAR}}", dockerImages: { Java: "java:21" }, variables: [{ name: "Jar", description: "", envVariable: "JAR", defaultValue: "app.jar", serverValue: "server.jar", isEditable: true, rules: "required" }] };
    const server = { id: "s1", name: "Server", owner: "owner", ownerId: "u1", template: "egg", node: "node", nodeId: "n1", status: "offline", primaryAllocationId: "a2", memoryMb: 2048, cpuShares: 1024, diskMb: 4096 };
    const user = { id: "u1", email: "u@example.com", role: "user", memoryMbLimit: 8192, serverLimit: 2 };
    const node = { id: "n1", name: "Node", region: "eu", regionId: "r1", status: "online", desiredState: "active", actualState: "active", heartbeatState: "healthy", memoryMb: 16384, diskMb: 50000 };
    mockFetch(
      jsonResponse({ data: [backup], pagination: { page: 1, per_page: 20, total: 1, total_pages: 1 } }),
      jsonResponse(startup),
      jsonResponse(server),
      jsonResponse([user]),
      jsonResponse([node])
    );
    expect((await fetchBackups("s1")).data[0]).toEqual(backup);
    expect(await fetchServerStartup("s1")).toEqual(startup);
    expect(await fetchServer("s1")).toEqual(server);
    expect((await fetchUsers())[0]).toEqual(user);
    expect((await fetchNodes())[0]).toEqual(node);
  });

  it("sends the admin server create payload with backend names and numeric values", async () => {
    const { calls } = mockFetch(jsonResponse({ id: "s1" }));
    await createServer({ name: "Game", ownerId: "u1", nodeId: "n1", regionId: "r1", templateId: "e1", allocationId: "a1", memoryMb: 2048, cpuShares: 1024, diskMb: 4096 });
    expect(requestJSON(calls[0])).toEqual({ name: "Game", ownerId: "u1", nodeId: "n1", regionId: "r1", templateId: "e1", allocationId: "a1", memoryMb: 2048, cpuShares: 1024, diskMb: 4096 });
  });

  it("uses separate node location and region IDs for create and update", async () => {
    const { calls } = mockFetch(jsonResponse({ node: { id: "n1" }, token: "token" }), jsonResponse({ id: "n1" }));
    await createNode({ name: "Node", region: "us-east", regionId: "r1", locationId: "l1", fqdn: "node.example.test", baseUrl: "https://node.example.test" });
    await updateNode("n1", { locationId: "l2" });
    expect(requestJSON(calls[0])).toMatchObject({ regionId: "r1", locationId: "l1" });
    expect(requestJSON(calls[1])).toEqual({ locationId: "l2" });
  });

  it("keeps safe details/build PATCH payloads explicit and uses mount assignment contract", async () => {
    const { calls } = mockFetch(jsonResponse({ id: "s1" }), jsonResponse({ ok: true }));
    await updateServer("s1", { name: "Renamed", description: "safe", ownerId: "u1", memoryMb: 2048, cpuShares: 1024, diskMb: 4096, primaryAllocationId: "a2" });
    await assignServerMount("s1", "m1");
    expect(requestJSON(calls[0])).toEqual({ name: "Renamed", description: "safe", ownerId: "u1", memoryMb: 2048, cpuShares: 1024, diskMb: 4096, primaryAllocationId: "a2" });
    expect(requestJSON(calls[1])).toEqual({ mountId: "m1" });
  });

  it("sends command, power, backup, offset, and failure-policy schedule task fields", async () => {
    const { calls } = mockFetch(jsonResponse({ id: "t1" }), jsonResponse({ id: "t1" }));
    await createServerScheduleTask("s1", "sc1", { action: "power", payload: { signal: "restart" }, sequence: 2, timeOffsetSeconds: 30, continueOnFailure: false });
    await updateServerScheduleTask("s1", "sc1", "t1", { action: "backup", payload: {}, sequence: 3, timeOffsetSeconds: 60, continueOnFailure: true });
    expect(requestJSON(calls[0])).toEqual({ action: "power", payload: { signal: "restart" }, sequence: 2, timeOffsetSeconds: 30, continueOnFailure: false });
    expect(requestJSON(calls[1])).toEqual({ action: "backup", payload: {}, sequence: 3, timeOffsetSeconds: 60, continueOnFailure: true });
  });

  it("uses exact admin role, OAuth, migration, TLS host, and allocation alias contracts", async () => {
    const { calls } = mockFetch(
      jsonResponse({ id: "r1" }), jsonResponse({ client: { id: "c1" }, clientSecret: "once" }),
      jsonResponse({ id: "m1" }), jsonResponse({ id: "d1" }), jsonResponse({ ok: true }),
    );
    await createRole({ key: "support", name: "Support", isAdmin: false });
    await createAdminOAuthClient({ userId: "u1", name: "Automation", description: "CI", scopes: ["server.read"], serverId: "s1", allowedScopes: ["server.read"] });
    await createMigration({ serverId: "s1", sourceNodeId: "n1", targetNodeId: "n2" });
    await createDatabaseHost({ name: "DB", engine: "postgresql", host: "db.internal", port: 5432, username: "panel", password: "secret", tlsMode: "verify-full", tlsCa: "PEM", tlsServerName: "db.internal", maxDatabases: 20 });
    await setAllocationAlias("node/id", "a1", "games.example.com");
    expect(requestJSON(calls[0])).toEqual({ key: "support", name: "Support", isAdmin: false });
    expect(requestJSON(calls[1])).toEqual({ userId: "u1", name: "Automation", description: "CI", scopes: ["server.read"], serverId: "s1", allowedScopes: ["server.read"] });
    expect(requestJSON(calls[2])).toEqual({ serverId: "s1", sourceNodeId: "n1", targetNodeId: "n2" });
    expect(requestJSON(calls[3])).toMatchObject({ tlsMode: "verify-full", tlsCa: "PEM", tlsServerName: "db.internal" });
    expect(calls[4].url).toContain("/nodes/node%2Fid/allocations/alias");
    expect(requestJSON(calls[4])).toEqual({ allocation_id: "a1", alias: "games.example.com" });
  });

  it("posts prospective database-host connection tests with the full host payload", async () => {
    const { calls } = mockFetch(jsonResponse({ ok: true }));
    const input = { name: "DB", engine: "postgresql", host: "db.internal", port: 5432, username: "panel", password: "secret", tlsMode: "verify-full", tlsServerName: "db.internal", maxDatabases: 20 };

    await expect(testDatabaseHostConnection(input)).resolves.toEqual({ ok: true });

    expect(calls[0].url).toContain("/database-hosts/test");
    expect(calls[0].init?.method).toBe("POST");
    expect(requestJSON(calls[0])).toEqual(input);
  });

  it("posts saved database-host connection tests without a configuration payload", async () => {
    const { calls } = mockFetch(jsonResponse({ ok: true }));

    await expect(testDatabaseHostConnection("host/id")).resolves.toEqual({ ok: true });

    expect(calls[0].url).toContain("/database-hosts/host%2Fid/test");
    expect(calls[0].init?.method).toBe("POST");
    expect(calls[0].init?.body).toBeUndefined();
  });

  it("uses orphan remediation status, resolution, and forced database delete contracts", async () => {
    const { calls } = mockFetch(
      jsonResponse({ serverRemediations: [], databaseRemediations: [] }),
      jsonResponse({ id: "database-remediation", status: "resolved" }),
      jsonResponse({ id: "server-remediation", status: "resolved" }),
      jsonResponse({ ok: true, orphanRemediation: true }),
    );

    await expect(fetchOrphanRemediations("resolved")).resolves.toEqual({ serverRemediations: [], databaseRemediations: [] });
    await resolveDatabaseOrphanRemediation("database/remediation");
    await resolveServerOrphanRemediation("server/remediation");
    await expect(deleteServerDatabase("server/id", "database/id", true)).resolves.toEqual({ ok: true, orphanRemediation: true });

    expect(calls[0].url).toContain("/admin/orphan-remediations/?status=resolved");
    expect(calls[1].url).toContain("/admin/orphan-remediations/databases/database%2Fremediation/resolve");
    expect(calls[1].init?.method).toBe("POST");
    expect(calls[2].url).toContain("/admin/orphan-remediations/servers/server%2Fremediation/resolve");
    expect(calls[2].init?.method).toBe("POST");
    expect(calls[3].url).toContain("/servers/server%2Fid/databases/database%2Fid?force=true");
    expect(calls[3].init?.method).toBe("DELETE");
  });

  it("uses the evacuation and recovery route contracts", async () => {
    const { calls } = mockFetch(
      jsonResponse({ id: "preview-1" }), jsonResponse({ id: "plan-1" }),
      jsonResponse({ id: "recovery-1" }), jsonResponse({ id: "recovery-1" }),
      jsonResponse({ id: "recovery-1", status: "executing" }), jsonResponse({ id: "recovery-1", status: "executing" }),
      jsonResponse({ id: "recovery-1", status: "cancelled" }),
    );
    await previewEvacuation("node/id");
    await createEvacuationPlan("node/id");
    await createRecoveryPlan({ nodeId: "node/id", reason: "offline" });
    await fetchRecoveryPlan("recovery/id");
    await executeRecoveryPlan("recovery/id");
    await startRecoveryPlan("recovery/id");
    await cancelRecoveryPlan("recovery/id");

    expect(calls[0].url).toContain("/nodes/node%2Fid/evacuation-preview");
    expect(calls[1].url).toContain("/nodes/node%2Fid/evacuation-plan");
    expect(calls[1].init?.method).toBe("POST");
    expect(calls[2].url).toContain("/recovery-plans");
    expect(requestJSON(calls[2])).toEqual({ nodeId: "node/id", reason: "offline" });
    expect(calls[3].url).toContain("/recovery-plans/recovery%2Fid");
    expect(calls[4].url).toContain("/recovery");
    expect(calls[4].init?.method).toBe("POST");
    expect(requestJSON(calls[4])).toEqual({ planId: "recovery/id" });
    expect(calls[5].url).toContain("/recovery/recovery%2Fid/start");
    expect(calls[5].init?.method).toBe("POST");
    expect(calls[6].url).toContain("/recovery/recovery%2Fid/cancel");
    expect(calls[6].init?.method).toBe("POST");
  });

  it("uses the allocation-scoped node picker endpoint", async () => {
    const nodes = [{ id: "node-1", name: "Amsterdam" }];
    const { calls } = mockFetch(jsonResponse(nodes));

    await expect(fetchAllocationNodes()).resolves.toEqual(nodes);

    expect(calls[0].url).toContain("/allocations/nodes");
    expect(calls[0].init?.method).toBeUndefined();
  });

  it("uses the admin allocation bulk-delete contract", async () => {
    const { calls } = mockFetch(jsonResponse({ ok: true }));
    await expect(deleteAllocations(["a1", "a2"])).resolves.toEqual({ ok: true });

    expect(calls[0].url).toContain("/allocations/bulk");
    expect(calls[0].init?.method).toBe("DELETE");
    expect(requestJSON(calls[0])).toEqual({ ids: ["a1", "a2"] });
  });

  it("uses the global admin allocation alias contract", async () => {
    const { calls } = mockFetch(jsonResponse({ ok: true }));

    await setAdminAllocationAlias("allocation/id", "games.example.com");

    expect(calls[0].url).toContain("/allocations/allocation%2Fid/alias");
    expect(calls[0].init.method).toBe("POST");
    expect(requestJSON(calls[0])).toEqual({ alias: "games.example.com" });
  });

  it("uses node allocation mutation payloads and empty response contracts", async () => {
    const { calls } = mockFetch(
      new Response(null, { status: 204 }),
      new Response(null, { status: 204 }),
    );
    await expect(setAllocationAlias("node/id", "a1", "games.example.com")).resolves.toBeUndefined();
    await expect(deleteAllocationsBulk("node/id", ["a1", "a2"])).resolves.toBeUndefined();

    expect(calls[0].url).toContain("/nodes/node%2Fid/allocations/alias");
    expect(requestJSON(calls[0])).toEqual({ allocation_id: "a1", alias: "games.example.com" });
    expect(calls[1].url).toContain("/nodes/node%2Fid/allocations/bulk");
    expect(requestJSON(calls[1])).toEqual({ allocations: [{ id: "a1" }, { id: "a2" }] });
  });

  it("parses plugin metadata and webhook delivery DTOs without inventing lifecycle or retry state", async () => {
    const plugin = { id: "p1", name: "Metrics", description: "", kind: "integration", version: "1.0.0", manifest: {}, installPath: "", installed: false, enabled: false, source: "url:x", createdAt: "now", updatedAt: "now" };
    const delivery = { id: "d1", webhookId: "w1", eventName: "server:created", targetUrl: "https://example.com", webhookType: "regular", attempts: 2, responseStatus: 500, lastError: "failed", nextAttemptAt: "later", state: "pending", createdAt: "now" };
    mockFetch(jsonResponse([plugin]), jsonResponse([delivery]));
    await expect(fetchPlugins()).resolves.toEqual([plugin]);
    await expect(fetchWebhookDeliveries("w1", 25)).resolves.toEqual([delivery]);
  });

  it("unwraps the backend permission catalog", async () => {
    mockFetch(jsonResponse({ permissions: { file: { read: "Read files" }, backup: { create: "Create backups" } } }));
    await expect(fetchPermissions()).resolves.toEqual({ file: { read: "Read files" }, backup: { create: "Create backups" } });
  });
});
