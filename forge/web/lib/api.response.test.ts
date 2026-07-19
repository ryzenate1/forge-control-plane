import { describe, expect, it } from "vitest";
import { fetchServerActivity, fetchUsers } from "@/lib/api";
import { jsonResponse, mockFetch } from "@/test/fetch-mock";

describe("API response conventions", () => {
  it("normalizes direct and data-enveloped list responses", async () => {
    const users = [{ id: "u1", email: "a@example.com", role: "user" }];
    mockFetch(jsonResponse(users), jsonResponse({ data: users }));

    await expect(fetchUsers()).resolves.toEqual(users);
    await expect(fetchUsers()).resolves.toEqual(users);
  });

  it("preserves list envelopes for callers that expose pagination", async () => {
    const response = {
      data: [{ id: "event-1" }],
      pagination: { page: 1, per_page: 50, total: 1, total_pages: 1 },
    };
    mockFetch(jsonResponse(response));

    await expect(fetchServerActivity("server-1")).resolves.toEqual(response);
  });

  it("reports invalid successful JSON with request context", async () => {
    mockFetch(new Response("not json", {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));

    await expect(fetchUsers()).rejects.toThrow("API GET /users returned invalid JSON");
  });
});
