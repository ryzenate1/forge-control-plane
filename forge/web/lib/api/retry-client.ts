"use client";

import { requestJSON, type ForgeRetryPolicy } from "./http";

/**
 * Retry knobs for the legacy retrying client. These are a 1:1 view onto the
 * canonical {@link ForgeRetryPolicy} in `lib/api/http.ts` — kept under the
 * historical names so existing call sites keep compiling.
 *
 * Retries are strictly opt-in: an empty config performs a single attempt.
 * Non-idempotent methods additionally require `idempotent: true`, otherwise
 * even an explicit `maxRetries` is ignored for POST/PUT/PATCH/DELETE.
 */
interface RetryConfig {
  maxRetries?: number;
  baseDelay?: number;
  maxDelay?: number;
  retryOnStatus?: number[];
  idempotent?: boolean;
}

function toRetryPolicy(config: RetryConfig): ForgeRetryPolicy | undefined {
  const policy: ForgeRetryPolicy = {};
  if (config.maxRetries !== undefined) policy.retries = config.maxRetries;
  if (config.baseDelay !== undefined) policy.baseDelay = config.baseDelay;
  if (config.maxDelay !== undefined) policy.maxDelay = config.maxDelay;
  if (config.retryOnStatus !== undefined) policy.retryOnStatus = config.retryOnStatus;
  if (config.idempotent !== undefined) policy.idempotent = config.idempotent;
  // No knobs set means no retries: `{}` must keep single-attempt semantics.
  if (
    policy.retries === undefined &&
    policy.baseDelay === undefined &&
    policy.maxDelay === undefined &&
    policy.retryOnStatus === undefined &&
    policy.idempotent === undefined
  ) {
    return undefined;
  }
  return policy;
}

/**
 * Retrying JSON request. This used to be a second, parallel HTTP client with its
 * own `fetch` call, its own CSRF signing and its own error shaping — which meant
 * a 401 here never reached `notifySessionExpired()`. It is now a thin wrapper
 * over the canonical primitive, so CSRF, cookie credentials, 401 session-expiry
 * signalling and error shaping are identical to `requestJSON`; only the retry
 * policy is added. Aborted requests are still re-thrown as `AbortError`.
 *
 * Paths are API-relative (e.g. `/servers/123`) and are resolved against
 * `API_BASE_URL`; absolute URLs are passed through untouched.
 */
export async function fetchWithRetry<T>(
  url: string,
  options: RequestInit & { signal?: AbortSignal } = {},
  config: RetryConfig = {},
): Promise<T> {
  const retry = toRetryPolicy(config);
  // Fail fast on an already-aborted (stale) signal instead of issuing a request
  // that is guaranteed to abort mid-flight.
  options.signal?.throwIfAborted?.();
  return requestJSON<T>(url, options, retry === undefined ? {} : { retry });
}

/**
 * Request factory bound to an {@link AbortSignal}, built on the canonical
 * primitive (see {@link fetchWithRetry}). The `path` argument of every method is
 * API-relative and retries are opt-in per call via {@link RetryConfig}.
 */
export function createAbortableFetch(signal?: AbortSignal) {
  return {
    fetchJSON: <T>(path: string, config?: RetryConfig) => {
      return fetchWithRetry<T>(path, { method: 'GET', signal }, config);
    },
    postJSON: <T>(path: string, body?: unknown, config?: RetryConfig) => {
      return fetchWithRetry<T>(
        path,
        {
          method: "POST",
          headers: body ? { "Content-Type": "application/json" } : undefined,
          body: body ? JSON.stringify(body) : undefined,
          signal,
        },
        config,
      );
    },
    putJSON: <T>(path: string, body?: unknown, config?: RetryConfig) => {
      return fetchWithRetry<T>(
        path,
        {
          method: "PUT",
          headers: body ? { "Content-Type": "application/json" } : undefined,
          body: body ? JSON.stringify(body) : undefined,
          signal,
        },
        config,
      );
    },
    patchJSON: <T>(path: string, body?: unknown, config?: RetryConfig) => {
      return fetchWithRetry<T>(
        path,
        {
          method: "PATCH",
          headers: body ? { "Content-Type": "application/json" } : undefined,
          body: body ? JSON.stringify(body) : undefined,
          signal,
        },
        config,
      );
    },
    deleteJSON: <T = void>(path: string, config?: RetryConfig) => {
      return fetchWithRetry<T>(
        path,
        { method: "DELETE", signal },
        config,
      );
    },
  };
}
