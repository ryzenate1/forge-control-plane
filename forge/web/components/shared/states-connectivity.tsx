"use client";

import { AlertTriangle, CloudOff, Cog, RefreshCw, ServerCrash, WifiOff } from "lucide-react";
import {} from "@/lib/utils";
import { ApiError } from "@/lib/api/http";

// --- Helpers to classify errors without hiding behind empty arrays ---

export type ErrorKind = "offline" | "permission" | "not-found" | "beacon" | "docker" | "api" | "rate-limit" | "generic";

export function classifyError(error: unknown): ErrorKind {
  if (error instanceof ApiError) {
    if (error.status === 0) return "offline";
    if (error.status === 401 || error.status === 403) return "permission";
    if (error.status === 404) return "not-found";
    if (error.status === 429) return "rate-limit";
    if (error.status === 502 || error.status === 504) return "beacon";
    if (error.status === 503) return "api";
    if (error.status >= 500) return "api";
  }
  const msg = error instanceof Error ? error.message.toLowerCase() : String(error).toLowerCase();
  if (msg.includes("network error") || msg.includes("failed to fetch") || msg.includes("unreachable")) return "offline";
  if (msg.includes("beacon") && (msg.includes("unreachable") || msg.includes("offline") || msg.includes("timeout"))) return "beacon";
  if (msg.includes("docker") && (msg.includes("unavailable") || msg.includes("not running") || msg.includes("daemon"))) return "docker";
  if (msg.includes("not found") || msg.includes("404")) return "not-found";
  if (msg.includes("permission") || msg.includes("access denied") || msg.includes("403")) return "permission";
  return "generic";
}

export function isNetworkError(error: unknown): boolean {
  return classifyError(error) === "offline";
}

export function isPermissionError(error: unknown): boolean {
  return classifyError(error) === "permission";
}

export function isBeaconError(error: unknown): boolean {
  return classifyError(error) === "beacon";
}

export function isDockerError(error: unknown): boolean {
  return classifyError(error) === "docker";
}

// --- Degraded warning: amber banner when service is partially degraded ---

export function DegradedBanner({
  title = "Degraded performance",
  message,
  onRetry,
}: {
  title?: string;
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <div
      className="flex items-center justify-between gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-100"
      role="alert"
    >
      <span className="flex items-center gap-2">
        <AlertTriangle size={16} className="shrink-0 text-amber-400" aria-hidden="true" />
        <span>
          <strong className="font-semibold">{title}</strong>
          {message ? <span className="ml-2 opacity-80">{message}</span> : null}
        </span>
      </span>
      {onRetry ? (
        <button
          className="inline-flex items-center gap-1 rounded-lg border border-amber-500/30 px-2.5 py-1 text-xs font-semibold hover:bg-amber-500/15"
          onClick={onRetry}
          type="button"
        >
          <RefreshCw size={12} />
          Retry
        </button>
      ) : null}
    </div>
  );
}

// --- Beacon unavailable: daemon offline ---

export function BeaconUnavailableState({
  message = "Beacon daemon is unreachable. The node agent is offline or cannot be contacted.",
  onRetry,
}: {
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-amber-500/20 bg-amber-500/[0.06] px-6 py-10 text-center" role="alert">
      <div className="grid h-11 w-11 place-items-center rounded-full bg-amber-500/10 text-amber-400">
        <ServerCrash size={20} aria-hidden="true" />
      </div>
      <h3 className="mt-3 text-sm font-semibold text-amber-200">Beacon unavailable</h3>
      <p className="mt-1 max-w-md text-sm leading-6 text-amber-300/80">{message}</p>
      {onRetry ? (
        <button
          className="mt-4 inline-flex items-center gap-2 rounded-lg bg-amber-600 px-4 py-2 text-sm font-semibold text-white hover:bg-amber-500"
          onClick={onRetry}
          type="button"
        >
          <RefreshCw size={14} />
          Retry
        </button>
      ) : null}
    </div>
  );
}

// --- Docker unavailable ---
export function DockerUnavailableState({
  message = "Docker daemon is unavailable. Container operations cannot be performed until Docker is running on the node.",
  onRetry,
}: {
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-sky-500/20 bg-sky-500/[0.06] px-6 py-10 text-center" role="alert">
      <div className="grid h-11 w-11 place-items-center rounded-full bg-sky-500/10 text-sky-400">
        <Cog size={20} aria-hidden="true" />
      </div>
      <h3 className="mt-3 text-sm font-semibold text-sky-200">Docker unavailable</h3>
      <p className="mt-1 max-w-md text-sm leading-6 text-sky-300/80">{message}</p>
      {onRetry ? (
        <button
          className="mt-4 inline-flex items-center gap-2 rounded-lg bg-sky-600 px-4 py-2 text-sm font-semibold text-white hover:bg-sky-500"
          onClick={onRetry}
          type="button"
        >
          <RefreshCw size={14} />
          Retry
        </button>
      ) : null}
    </div>
  );
}

// --- API unavailable (generic backend down) ---
export function ApiUnavailableState({
  message = "API server is unreachable. Check that the Go backend is running and the API URL is reachable.",
  onRetry,
}: {
  message?: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-[var(--danger)]/20 bg-[var(--danger-subtle)] px-6 py-10 text-center" role="alert">
      <div className="grid h-11 w-11 place-items-center rounded-full bg-[var(--danger-subtle)] text-[var(--danger)] border border-[var(--danger)]/20">
        <CloudOff size={20} aria-hidden="true" />
      </div>
      <h3 className="mt-3 text-sm font-semibold text-[var(--text)]">API unavailable</h3>
      <p className="mt-1 max-w-md text-sm leading-6 text-[var(--text-subtle)]">{message}</p>
      {onRetry ? (
        <button
          className="mt-4 inline-flex items-center gap-2 rounded-lg bg-[var(--danger)] px-4 py-2 text-sm font-semibold text-white hover:bg-[var(--brand-hover)]"
          onClick={onRetry}
          type="button"
        >
          <RefreshCw size={14} />
          Retry
        </button>
      ) : null}
    </div>
  );
}

// --- Operation failed (mutation error) ---
export function OperationFailedState({
  error,
  title = "Operation failed",
  onRetry,
  onDismiss,
}: {
  error: unknown;
  title?: string;
  onRetry?: () => void;
  onDismiss?: () => void;
}) {
  const msg = error instanceof Error ? error.message : String(error);
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-[var(--danger)]/25 bg-[var(--danger-subtle)] p-4 text-sm text-[var(--danger)]" role="alert">
      <span>
        <strong className="font-semibold">{title}:</strong> {msg}
      </span>
      <span className="flex gap-2">
        {onRetry ? (
          <button className="rounded-lg border border-[var(--line)] px-3 py-1 text-xs font-semibold text-[var(--text)] hover:bg-[var(--surface-raised)]" onClick={onRetry} type="button">
            Retry
          </button>
        ) : null}
        {onDismiss ? (
          <button className="rounded-lg border border-[var(--line)] px-3 py-1 text-xs font-semibold text-[var(--text)] hover:bg-[var(--surface-raised)]" onClick={onDismiss} type="button">
            Dismiss
          </button>
        ) : null}
      </span>
    </div>
  );
}

// --- Retrying indicator ---
export function RetryingBanner({ label = "Retrying…" }: { label?: string }) {
  return (
    <div className="flex items-center gap-2 rounded-lg border border-sky-500/20 bg-sky-500/10 px-3 py-2 text-xs text-sky-200" role="status">
      <RefreshCw size={12} className="animate-spin" aria-hidden="true" />
      {label}
    </div>
  );
}

// --- Reconnecting banner for websockets / live data ---
export function ReconnectingBanner({
  label = "Reconnecting…",
  onRetry,
}: {
  label?: string;
  onRetry?: () => void;
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-100" role="status">
      <span className="inline-flex items-center gap-2">
        <WifiOff size={12} className="animate-pulse" aria-hidden="true" />
        {label}
      </span>
      {onRetry ? (
        <button className="rounded bg-amber-600 px-2 py-1 text-[11px] font-semibold text-white hover:bg-amber-500" onClick={onRetry} type="button">
          Retry now
        </button>
      ) : null}
    </div>
  );
}

// --- Generic helper: never hide network errors behind empty states ---
export function shouldShowEmpty(isLoading: boolean, isError: boolean, data: unknown): boolean {
  if (isLoading || isError) return false;
  return !Array.isArray(data) || data.length === 0;
}
