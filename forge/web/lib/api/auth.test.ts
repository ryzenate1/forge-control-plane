import { describe, expect, it, vi, afterEach } from "vitest";
import { jsonResponse, mockFetchByUrl } from "../../test/fetch-mock";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
  vi.restoreAllMocks();
});

describe("auth API client", () => {
  describe("login", () => {
    it("sends POST with correct body and headers", async () => {
      const { calls } = mockFetchByUrl({
        "/auth/login": jsonResponse({ complete: true, user: { id: "u1" } }),
      });

      const { login } = await import("./auth");
      const result = await login("user@example.com", "secret123");

      expect(result).toEqual({ complete: true, user: { id: "u1" } });
      expect(calls).toHaveLength(1);
      expect(calls[0].url).toContain("/auth/login");
      expect(calls[0].init.method).toBe("POST");
      expect(calls[0].init.headers).toMatchObject({
        "Content-Type": "application/json",
        "X-Forge-Session-Mode": "cookie",
      });
      expect(JSON.parse(String(calls[0].init.body))).toEqual({
        email: "user@example.com",
        password: "secret123",
      });
    });

    it("throws on 401 with invalid credentials message", async () => {
      mockFetchByUrl({
        "/auth/login": jsonResponse({ error: "unauthorized" }, 401),
      });

      const { login } = await import("./auth");
      await expect(login("u@e.com", "wrong")).rejects.toThrow("Invalid email or password");
    });

    it("throws on 429 with rate limit message", async () => {
      mockFetchByUrl({
        "/auth/login": jsonResponse({}, 429),
      });

      const { login } = await import("./auth");
      await expect(login("u@e.com", "pw")).rejects.toThrow("Too many login attempts");
    });

    it("throws generic error on 500", async () => {
      mockFetchByUrl({
        "/auth/login": jsonResponse({ error: "internal" }, 500),
      });

      const { login } = await import("./auth");
      await expect(login("u@e.com", "pw")).rejects.toThrow("Unable to sign in");
    });
  });

  describe("loginCheckpoint", () => {
    it("sends code and recovery token in body", async () => {
      const { calls } = mockFetchByUrl({
        "/auth/login/checkpoint": jsonResponse({ complete: true, user: { id: "u1" } }),
      });

      const { loginCheckpoint } = await import("./auth");
      const result = await loginCheckpoint("tok123", "123456", "recovery-tok");

      expect(result).toEqual({ complete: true, user: { id: "u1" } });
      expect(calls[0].init.method).toBe("POST");
      const body = JSON.parse(String(calls[0].init.body));
      expect(body.confirmationToken).toBe("tok123");
      expect(body.code).toBe("123456");
      expect(body.recoveryToken).toBe("recovery-tok");
    });

    it("throws on 400 with invalid code message", async () => {
      mockFetchByUrl({
        "/auth/login/checkpoint": jsonResponse({}, 400),
      });

      const { loginCheckpoint } = await import("./auth");
      await expect(loginCheckpoint("tok", "000000")).rejects.toThrow("Invalid authentication code");
    });

    it("throws on 429 with rate limit message", async () => {
      mockFetchByUrl({
        "/auth/login/checkpoint": jsonResponse({}, 429),
      });

      const { loginCheckpoint } = await import("./auth");
      await expect(loginCheckpoint("tok", "123456")).rejects.toThrow("Too many verification attempts");
    });
  });

  describe("logout", () => {
    it("sends POST to /auth/logout", async () => {
      const { calls } = mockFetchByUrl({
        "/auth/logout": jsonResponse({ ok: true }),
      });

      const { logout } = await import("./auth");
      await logout();

      expect(calls).toHaveLength(1);
      expect(calls[0].init.method).toBe("POST");
    });

    it("throws ApiError on failure", async () => {
      mockFetchByUrl({
        "/auth/logout": jsonResponse({}, 500),
      });

      const { logout } = await import("./auth");
      await expect(logout()).rejects.toThrow("Logout failed");
    });
  });

  describe("fetchCurrentUser", () => {
    it("returns user on success", async () => {
      mockFetchByUrl({
        "/auth/me": jsonResponse({ id: "u1", email: "a@b.com", role: "admin" }),
      });

      const { fetchCurrentUser } = await import("./auth");
      const user = await fetchCurrentUser();
      expect(user).toEqual({ id: "u1", email: "a@b.com", role: "admin" });
    });

    it("returns null on 401", async () => {
      mockFetchByUrl({
        "/auth/me": jsonResponse({}, 401),
      });

      const { fetchCurrentUser } = await import("./auth");
      const user = await fetchCurrentUser();
      expect(user).toBeNull();
    });

    it("rethrows non-401 errors", async () => {
      mockFetchByUrl({
        "/auth/me": jsonResponse({}, 500),
      });

      const { fetchCurrentUser } = await import("./auth");
      await expect(fetchCurrentUser()).rejects.toThrow();
    });
  });

  describe("requestPasswordReset", () => {
    it("sends email in POST body", async () => {
      const { calls } = mockFetchByUrl({
        "/auth/password/email": jsonResponse({ status: "sent" }),
      });

      const { requestPasswordReset } = await import("./auth");
      const result = await requestPasswordReset("user@example.com");
      expect(result.status).toBe("sent");
      const body = JSON.parse(String(calls[0].init.body));
      expect(body.email).toBe("user@example.com");
    });
  });

  describe("resetPassword", () => {
    it("sends email, token, and password", async () => {
      const { calls } = mockFetchByUrl({
        "/auth/password/reset": jsonResponse({ status: "ok" }),
      });

      const { resetPassword } = await import("./auth");
      const result = await resetPassword("u@e.com", "tok", "newpass");
      expect(result.status).toBe("ok");
      const body = JSON.parse(String(calls[0].init.body));
      expect(body).toEqual({ email: "u@e.com", token: "tok", password: "newpass" });
    });
  });

  describe("fetchUserSessions", () => {
    it("returns sessions array", async () => {
      const sessions = [{ id: "s1", ip: "1.2.3.4" }];
      mockFetchByUrl({
        "/auth/sessions": jsonResponse(sessions),
      });

      const { fetchUserSessions } = await import("./auth");
      const result = await fetchUserSessions();
      expect(result).toEqual(sessions);
    });
  });

  describe("revokeUserSession", () => {
    it("sends DELETE with correct session ID", async () => {
      const { calls } = mockFetchByUrl({
        "/auth/sessions/sess-abc?reason=security": jsonResponse({ ok: true }),
      });

      const { revokeUserSession } = await import("./auth");
      const result = await revokeUserSession("sess-abc", "security");
      expect(result.status).toBe("revoked");
      expect(calls[0].init.method).toBe("DELETE");
      expect(calls[0].url).toContain("sess-abc");
    });

    it("omits reason query param when not provided", async () => {
      const { calls } = mockFetchByUrl({
        "/auth/sessions/sess-123": jsonResponse({ ok: true }),
      });

      const { revokeUserSession } = await import("./auth");
      await revokeUserSession("sess-123");
      expect(calls[0].url).not.toContain("reason=");
    });
  });
});
