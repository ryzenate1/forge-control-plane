import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";
import { middleware } from "./middleware";

const SESSION_COOKIE_VALUES = ["__Host-forge_session", "forge_session"];

function makeRequest(path: string, opts: { cookie?: string } = {}) {
  const headers = new Headers();
  if (opts.cookie) headers.set("cookie", opts.cookie);
  return new NextRequest(new URL(path, "https://example.com"), { headers });
}

describe("middleware", () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  describe("public paths", () => {
    // /setup is covered separately below: it performs one setup-status fetch
    // to decide whether the wizard is still relevant.
    const publicPaths = ["/", "/forgot-password", "/reset-password", "/favicon.ico"];

    it.each(publicPaths)("never redirects %s and sets a CSP header with a nonce", async (path) => {
      const request = makeRequest(path);
      const response = await middleware(request);

      expect(response.status).not.toBe(307);
      expect(response.headers.get("location")).toBeNull();
      const csp = response.headers.get("Content-Security-Policy");
      expect(csp).toBeTruthy();
      expect(csp).toMatch(/'nonce-[a-f0-9]+'/);
      expect(global.fetch).not.toHaveBeenCalled();
    });

    describe("/setup", () => {
      function mockSetupStatus(body: unknown, status = 200) {
        (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
          new Response(typeof body === "string" ? body : JSON.stringify(body), { status }),
        );
      }

      it("renders (no redirect) while setup is still required", async () => {
        mockSetupStatus({ required: true, hasAdmin: false });
        const response = await middleware(makeRequest("/setup"));

        expect(response.headers.get("location")).toBeNull();
        expect(response.headers.get("Content-Security-Policy")).toMatch(/'nonce-[a-f0-9]+'/);
        expect(global.fetch).toHaveBeenCalledTimes(1);
      });

      it("redirects to / when setup is no longer required", async () => {
        mockSetupStatus({ required: false, hasAdmin: true });
        const response = await middleware(makeRequest("/setup"));

        expect(response.status).toBe(307);
        const location = new URL(response.headers.get("location")!);
        expect(location.pathname).toBe("/");
        expect(location.searchParams.get("setup")).toBe("complete");
      });

      it("renders (no redirect) when setup status cannot be determined", async () => {
        (global.fetch as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("api down"));
        const response = await middleware(makeRequest("/setup"));

        // Fail open: an unreachable API must not brick first-run bootstrap.
        // The setup page itself renders its own unreachable state.
        expect(response.headers.get("location")).toBeNull();
        expect(response.headers.get("Content-Security-Policy")).toBeTruthy();
      });
    });

    it("does not redirect paths under /_next", async () => {
      const request = makeRequest("/_next/static/chunk.js");
      const response = await middleware(request);
      expect(response.headers.get("location")).toBeNull();
      expect(response.headers.get("Content-Security-Policy")).toBeTruthy();
    });

    it("does not redirect paths under /api/", async () => {
      const request = makeRequest("/api/v1/health");
      const response = await middleware(request);
      expect(response.headers.get("location")).toBeNull();
      expect(response.headers.get("Content-Security-Policy")).toBeTruthy();
    });
  });

  describe("protected paths without a session cookie", () => {
    const protectedPaths = ["/servers", "/servers/123", "/server/1", "/account", "/admin", "/admin/users", "/organizations"];

    it.each(protectedPaths)("redirects %s to / with a session-expired reason", async (path) => {
      const request = makeRequest(path);
      const response = await middleware(request);

      expect(response.status).toBe(307);
      const location = new URL(response.headers.get("location")!);
      expect(location.pathname).toBe("/");
      expect(location.searchParams.get("reason")).toBe("session-expired");
      expect(location.searchParams.get("next")).toBe(path);
      expect(global.fetch).not.toHaveBeenCalled();
    });
  });

  describe("protected paths with a session cookie", () => {
    it("redirects when the backend validation fetch returns non-OK", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce(new Response(null, { status: 401 }));
      const request = makeRequest("/servers", { cookie: `${SESSION_COOKIE_VALUES[0]}=abc` });

      const response = await middleware(request);

      expect(response.status).toBe(307);
      const location = new URL(response.headers.get("location")!);
      expect(location.searchParams.get("reason")).toBe("session-expired");
      expect(global.fetch).toHaveBeenCalledTimes(1);
    });

    it("allows access when the validation fetch returns OK, and sets CSP with a nonce", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce(new Response(null, { status: 200 }));
      const request = makeRequest("/servers/42", { cookie: `${SESSION_COOKIE_VALUES[0]}=abc` });

      const response = await middleware(request);

      expect(response.headers.get("location")).toBeNull();
      const csp = response.headers.get("Content-Security-Policy");
      expect(csp).toBeTruthy();
      expect(csp).toMatch(/'nonce-[a-f0-9]+'/);
    });

    it("fails closed (redirects) when the validation fetch throws", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("network error"));
      const request = makeRequest("/admin", { cookie: `${SESSION_COOKIE_VALUES[0]}=abc` });

      const response = await middleware(request);

      expect(response.status).toBe(307);
      const location = new URL(response.headers.get("location")!);
      expect(location.searchParams.get("reason")).toBe("session-expired");
    });

    it("forwards the cookie header to the validation request", async () => {
      (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce(new Response(null, { status: 200 }));
      const request = makeRequest("/account", { cookie: `${SESSION_COOKIE_VALUES[0]}=abc; other=1` });

      await middleware(request);

      const [url, init] = (global.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
      expect(url.toString()).toBe("http://127.0.0.1:8080/api/v1/auth/me");
      expect(init.headers.cookie).toBe(`${SESSION_COOKIE_VALUES[0]}=abc; other=1`);
    });
  });

  describe("next redirect parameter round-tripping", () => {
    it("encodes pathname and search params together", async () => {
      const request = makeRequest("/servers/42?tab=console&x=1 2");
      const response = await middleware(request);

      const location = new URL(response.headers.get("location")!);
      const next = location.searchParams.get("next");
      expect(next).toBe("/servers/42?tab=console&x=1%202");
    });

    it("round-trips a bare path with no search params", async () => {
      const request = makeRequest("/organizations");
      const response = await middleware(request);
      const location = new URL(response.headers.get("location")!);
      expect(location.searchParams.get("next")).toBe("/organizations");
    });
  });
});
