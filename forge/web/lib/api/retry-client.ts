"use client";

import { ApiError } from "./http";

interface RetryConfig {
  maxRetries?: number;
  baseDelay?: number;
  maxDelay?: number;
  retryOnStatus?: number[];
}

const DEFAULT_CONFIG: Required<RetryConfig> = {
  maxRetries: 3,
  baseDelay: 500,
  maxDelay: 10000,
  retryOnStatus: [408, 429, 500, 502, 503, 504],
};

function getDelay(attempt: number, baseDelay: number, maxDelay: number): number {
  const delay = Math.min(baseDelay * Math.pow(2, attempt), maxDelay);
  return delay + Math.random() * 500;
}

export async function fetchWithRetry<T>(
  url: string,
  options: RequestInit & { signal?: AbortSignal } = {},
  config: RetryConfig = {},
): Promise<T> {
  const { maxRetries, baseDelay, maxDelay, retryOnStatus } = { ...DEFAULT_CONFIG, ...config };
  let lastError: Error | null = null;

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const response = await fetch(url, { ...options });

      if (!response.ok && retryOnStatus.includes(response.status) && attempt < maxRetries) {
        const delay = getDelay(attempt, baseDelay, maxDelay);
        await new Promise((resolve) => setTimeout(resolve, delay));
        continue;
      }

      if (!response.ok) {
        const errorMessage = await getErrorMessage(response);
        throw new ApiError(errorMessage, response.status);
      }

      if (response.status === 204) return undefined as T;
      const text = await response.text();
      return text ? (JSON.parse(text) as T) : (undefined as T);
    } catch (error) {
      if (error instanceof ApiError) throw error;
      if (error instanceof DOMException && error.name === "AbortError") throw error;
      lastError = error as Error;
      if (attempt < maxRetries) {
        const delay = getDelay(attempt, baseDelay, maxDelay);
        await new Promise((resolve) => setTimeout(resolve, delay));
      }
    }
  }

  throw lastError ?? new Error("Request failed");
}

async function getErrorMessage(response: Response): Promise<string> {
  try {
    const error = await response.json();
    return error.message || error.error || `Request failed with status ${response.status}`;
  } catch {
    return `Request failed with status ${response.status}`;
  }
}

export function createAbortableFetch(signal?: AbortSignal) {
  return {
    fetchJSON: <T>(path: string, config?: RetryConfig) => {
      return fetchWithRetry<T>(path, { signal }, config);
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
