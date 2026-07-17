import { vi } from "vitest";

export type FetchCall = { url: string; init: RequestInit };

export function jsonResponse(body: unknown, status = 200) {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export function mockFetch(...responses: Array<Response | ((url: string, init: RequestInit) => Response | Promise<Response>)>) {
  const calls: FetchCall[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input);
    calls.push({ url, init });
    const response = responses.shift();
    if (!response) throw new Error(`Unexpected fetch: ${url}`);
    return typeof response === "function" ? response(url, init) : response;
  });
  vi.stubGlobal("fetch", fetchMock);
  return { calls, fetchMock };
}

export function requestJSON(call: FetchCall) {
  return call.init.body ? JSON.parse(String(call.init.body)) as unknown : undefined;
}
