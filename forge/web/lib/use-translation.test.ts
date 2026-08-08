import { describe, expect, it } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { interpolate, useTranslation } from "./use-translation";
import { jsonResponse, mockFetch } from "@/test/fetch-mock";

describe("interpolate", () => {
  it("interpolates positional translation parameters", () => {
    expect(interpolate("Must be at least {0} characters", [12])).toBe("Must be at least 12 characters");
  });

  it("interpolates named translation parameters", () => {
    expect(interpolate("Welcome, {name}", { name: "Ada" })).toBe("Welcome, Ada");
  });

  it("leaves missing parameters visible for translation diagnostics", () => {
    expect(interpolate("{0} {name}", [])).toBe("{0} {name}");
  });
});

describe("useTranslation", () => {
  it("loads messages for locale and interpolates keys", async () => {
    mockFetch(
      jsonResponse({ welcome: "Hello {name}!" }),
    );

    const { result } = renderHook(() => useTranslation());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.t("welcome", { name: "Antigravity" })).toBe("Hello Antigravity!");
  });

  it("falls back to English when primary locale fetch fails", async () => {
    mockFetch(
      new Response("Not Found", { status: 404 }),
      jsonResponse({ welcome: "Hello from EN" }),
    );

    const { result } = renderHook(() => useTranslation());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.t("welcome")).toBe("Hello from EN");
  });

  it("handles failure of both primary and English fallback gracefully without crashing", async () => {
    mockFetch(
      new Response("Not Found", { status: 404 }),
      new Response("Not Found", { status: 404 }),
    );

    const { result } = renderHook(() => useTranslation());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.t("welcome")).toBe("welcome");
  });

  it("falls back to the bundled en.json when the loaded locale misses a key", async () => {
    mockFetch(
      jsonResponse({ welcome: "Hello {name}!" }),
    );

    const { result } = renderHook(() => useTranslation());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.t("auth.login")).toBe("Sign In");
  });

  it("returns the raw key when a key is missing from both the locale and en.json", async () => {
    mockFetch(
      jsonResponse({ welcome: "Hello" }),
    );

    const { result } = renderHook(() => useTranslation());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.t("no.such.key")).toBe("no.such.key");
  });
});
