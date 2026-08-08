import type { ReactElement, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { vi } from "vitest";

export function createTestQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } });
}

/**
 * Wraps a component in a QueryClientProvider only.
 *
 * NOTE: this does not provide any `next/navigation` context. Components
 * that call `useRouter`/`usePathname`/`useSearchParams` will throw unless
 * the test file mocks `next/navigation` itself (see the existing
 * `vi.mock("next/navigation", ...)` calls in test/auth-account.test.tsx and
 * test/ui-contracts.test.tsx). Prefer `renderWithProviders` below for new
 * tests of router-dependent components so navigation mocking is consistent
 * and doesn't need to be duplicated per file.
 */
export function renderWithQuery(ui: ReactElement, client = createTestQueryClient()) {
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  }
  return { ...render(ui, { wrapper: Wrapper }), client };
}

export type RouterMock = {
  replace: (...args: unknown[]) => void;
  push: (...args: unknown[]) => void;
  back: (...args: unknown[]) => void;
  forward: (...args: unknown[]) => void;
  refresh: (...args: unknown[]) => void;
  prefetch: (...args: unknown[]) => void;
};

export type ProvidersOptions = {
  client?: QueryClient;
  /** Value returned by `usePathname()`. Defaults to "/". */
  pathname?: string;
  /** Value returned by `useSearchParams()`. Defaults to an empty `URLSearchParams`. */
  searchParams?: URLSearchParams;
  /** Overrides for the router mock returned by `useRouter()`. */
  router?: Partial<RouterMock>;
};

/**
 * Like `renderWithQuery`, but also supplies a mocked `next/navigation`
 * router/pathname/search-params context, matching the shape already used
 * by `vi.mock("next/navigation", ...)` across the test suite. This lets
 * components using `useRouter`, `usePathname`, or `useSearchParams` be
 * rendered without every test file re-mocking navigation individually.
 *
 * IMPORTANT: because `next/navigation` is mocked at the module level via
 * `vi.mock`, callers must still add a top-level
 * `vi.mock("next/navigation", () => require("@/test/render").navigationMock)`
 * (or an equivalent inline mock) in their test file — Vitest module mocks
 * cannot be registered lazily inside a helper function. This helper's job
 * is to standardize the *values* returned by that mock and give tests a
 * single place to configure pathname/search params/router spies per case.
 */
export function renderWithProviders(ui: ReactElement, options: ProvidersOptions = {}) {
  const client = options.client ?? createTestQueryClient();
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  }
  return { ...render(ui, { wrapper: Wrapper }), client };
}

/**
 * Creates a fresh set of `next/navigation` mock functions and values,
 * intended to be returned from a `vi.mock("next/navigation", () => ({...}))`
 * factory. Standardizes on the same shape used by the existing
 * auth-account.test.tsx / ui-contracts.test.tsx mocks.
 *
 * Example:
 * ```ts
 * const nav = createNavigationMock();
 * vi.mock("next/navigation", () => nav);
 * ```
 */
export function createNavigationMock(initial: { pathname?: string; searchParams?: URLSearchParams } = {}) {
  const replace = vi.fn();
  const push = vi.fn();
  const back = vi.fn();
  const forward = vi.fn();
  const refresh = vi.fn();
  const prefetch = vi.fn();
  let pathname = initial.pathname ?? "/";
  let searchParams = initial.searchParams ?? new URLSearchParams();
  return {
    useRouter: () => ({ replace, push, back, forward, refresh, prefetch }),
    usePathname: () => pathname,
    useSearchParams: () => searchParams,
    __setPathname: (value: string) => { pathname = value; },
    __setSearchParams: (value: URLSearchParams) => { searchParams = value; },
    replace,
    push,
    back,
    forward,
    refresh,
    prefetch,
  };
}
