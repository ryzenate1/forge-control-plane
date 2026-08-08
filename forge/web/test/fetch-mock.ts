import { vi } from "vitest";

export type FetchCall = { url: string; init: RequestInit };

export type FetchResponder = Response | ((url: string, init: RequestInit) => Response | Promise<Response>);

export function jsonResponse(body: unknown, status = 200) {
  return new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * Sequential fetch mock: responses are consumed in the order requests
 * arrive (via `Array.shift()`), regardless of URL/method.
 *
 * CAVEAT: because ordering is not tied to the actual request, tests whose
 * components issue requests concurrently or in a different order than
 * expected (e.g. due to React re-renders, retries, or Promise.all) can
 * silently receive the wrong response for a given request. Prefer
 * `mockFetchByUrl` for new tests, especially anything with more than one
 * in-flight request. This function remains for backward compatibility
 * with existing call sites.
 */
export function mockFetch(...responses: FetchResponder[]) {
  const calls: FetchCall[] = [];
  const queue = responses.slice();
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input);
    calls.push({ url, init });
    const response = queue.shift();
    if (!response) throw new Error(`Unexpected fetch: ${url}`);
    return typeof response === "function" ? response(url, init) : response;
  });
  vi.stubGlobal("fetch", fetchMock);
  return { calls, fetchMock };
}

export type UrlMatcher = string | RegExp | ((url: string, init: RequestInit) => boolean);

type UrlRoute = { matcher: UrlMatcher; method?: string; responder: FetchResponder };

function matches(route: UrlRoute, url: string, init: RequestInit): boolean {
  const method = (init.method ?? "GET").toUpperCase();
  if (route.method && route.method.toUpperCase() !== method) return false;
  if (typeof route.matcher === "string") return url === route.matcher || url.endsWith(route.matcher);
  if (route.matcher instanceof RegExp) return route.matcher.test(url);
  return route.matcher(url, init);
}

/**
 * URL/method-keyed fetch mock (preferred over `mockFetch` for new tests).
 *
 * Responses are looked up by matching the request's URL (exact match,
 * suffix match, RegExp, or predicate) and optional HTTP method, instead of
 * relying on request order. This avoids incorrect responses when requests
 * happen concurrently or out of the order a test author expected.
 *
 * Falls back to a sequential fallback queue (via `fallback`/`mockFetchOnce`)
 * for any request that doesn't match a registered route, so tests can mix
 * both styles where needed.
 *
 * @example
 * const { calls } = mockFetchByUrl({
 *   "/users": jsonResponse([{ id: "u1" }]),
 *   "/users/u1": { method: "DELETE", response: jsonResponse({ ok: true }) },
 * });
 */
export function mockFetchByUrl(
  routes: Record<string, FetchResponder | { method?: string; response: FetchResponder }>,
) {
  const calls: FetchCall[] = [];
  const parsedRoutes: UrlRoute[] = Object.entries(routes).map(([matcher, value]) => {
    if (value && typeof value === "object" && "response" in value) {
      return { matcher, method: value.method, responder: value.response };
    }
    return { matcher, responder: value as FetchResponder };
  });
  const fallbackQueue: FetchResponder[] = [];

  const fetchMock = vi.fn(async (input: RequestInfo | URL, init: RequestInit = {}) => {
    const url = String(input);
    calls.push({ url, init });
    const route = parsedRoutes.find((candidate) => matches(candidate, url, init));
    const response = route ? route.responder : fallbackQueue.shift();
    if (!response) throw new Error(`Unexpected fetch (no matching route or fallback): ${init.method ?? "GET"} ${url}`);
    return typeof response === "function" ? response(url, init) : response;
  });
  vi.stubGlobal("fetch", fetchMock);

  return {
    calls,
    fetchMock,
    /** Registers a one-off response consumed in order for requests that don't match any route. */
    mockFetchOnce(response: FetchResponder) {
      fallbackQueue.push(response);
    },
    /** Adds or overrides a route after initial setup. */
    addRoute(matcher: UrlMatcher, responder: FetchResponder, method?: string) {
      parsedRoutes.push({ matcher, method, responder });
    },
  };
}

export function requestJSON(call: FetchCall) {
  return call.init.body ? JSON.parse(String(call.init.body)) as unknown : undefined;
}
