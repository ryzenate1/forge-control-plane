"use client";

import { useEffect, useRef, useState } from "react";
import type { HealthCheck } from "@/lib/api";
import { sanitizeError } from "@/lib/sanitize";

const POLL_INTERVAL_MS = 30000;

export function HealthDashboard() {
  const [health, setHealth] = useState<HealthCheck | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pollError, setPollError] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const serverTokenRef = useRef(0);
  const refresh = () => setRefreshKey((k) => k + 1);
  const dismissError = () => setError(null);

  useEffect(() => {
    const controller = new AbortController();
    const token = ++serverTokenRef.current;
    setError(null);
    fetch("/api/proxy/health", { signal: controller.signal, credentials: "include" })
      .then(async (r) => {
        if (!r.ok) {
          const body = await r.json().catch(() => null);
          throw new Error(body?.error || `Failed to load: ${r.status}`);
        }
        return r.json();
      })
      .then((data) => { if (token === serverTokenRef.current) setHealth(data); })
      .catch((e) => {
        if (token === serverTokenRef.current) {
          if (e instanceof DOMException && e.name === "AbortError") return;
          setError(sanitizeError(e instanceof Error ? e.message : "An unexpected error occurred"));
        }
      });
    return () => controller.abort();
  }, [refreshKey]);

  useEffect(() => {
    let lastController = new AbortController();
    const interval = setInterval(() => {
      lastController.abort();
      lastController = new AbortController();
      const token = ++serverTokenRef.current;
      fetch("/api/proxy/health", { signal: lastController.signal, credentials: "include" })
        .then((r) => {
          if (!r.ok) {
            const body = r.clone();
            return body.json().catch(() => null).then((parsed) => {
              throw new Error(parsed?.error || `Failed to load: ${r.status}`);
            });
          }
          return r.json();
        })
        .then((data) => { if (token === serverTokenRef.current) { setHealth(data); setPollError(false); } })
        .catch((e) => {
          if (token === serverTokenRef.current) {
            if (e instanceof DOMException && e.name === "AbortError") return;
            setPollError(true);
          }
        });
    }, POLL_INTERVAL_MS);
    return () => { clearInterval(interval); lastController.abort(); };
  }, []);

  if (error) {
    return (
      <div role="alert" className="rounded-xl border border-red-300 dark:border-red-800 bg-red-wash dark:bg-red-950/30 p-6">
        <div className="flex items-start justify-between">
          <div>
            <p className="font-bold text-red-dark dark:text-red-300">Failed to load health</p>
            <p className="mt-1 text-sm text-muted">{error}</p>
          </div>
          <button onClick={dismissError} aria-label="Dismiss error" className="ml-auto flex h-10 w-10 items-center justify-center text-xs text-muted hover:text-ink">×</button>
        </div>
      </div>
    );
  }

  if (!health) {
    return (
      <div role="status" aria-live="polite" className="rounded-xl border border-line bg-paper p-6">
        <p className="text-muted">Loading health status…</p>
      </div>
    );
  }

  const isHealthy = health.status === "healthy";

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <button onClick={refresh} aria-label="Refresh health status" className="text-xs text-muted hover:text-ink transition-colors">
          ↻ Refresh
        </button>
      </div>

      <div className={`rounded-xl border p-6 ${isHealthy ? "border-green-300 dark:border-green-800 bg-green-50 dark:bg-green-950/30" : "border-red-300 dark:border-red-800 bg-red-wash dark:bg-red-950/30"}`}>
        <div className="flex items-center gap-3">
          <span role="img" aria-label={isHealthy ? "Healthy" : "Unhealthy"} className={`inline-block h-4 w-4 rounded-full ${isHealthy ? "bg-green-500" : "bg-red"}`} />
          <div>
            <p className={`text-lg font-bold ${isHealthy ? "text-green-800 dark:text-green-300" : "text-red-dark dark:text-red-300"}`}>
              {isHealthy ? "All systems healthy" : "System unhealthy"}
            </p>
            <p className="text-sm text-muted">
              Last checked: {new Date(health.checkedAt).toLocaleString()}
            </p>
          </div>
        </div>
      </div>

      {pollError && !error && (
        <p className="text-amber-600 text-sm">Unable to refresh. Showing last known data.</p>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {health.checks.map((check) => {
          const ok = check.status === "healthy";
          return (
            <div key={check.name} className={`rounded-xl border p-5 ${ok ? "border-green-border dark:border-green-800 bg-paper" : "border-red-200 dark:border-red-800 bg-red-wash dark:bg-red-950/30"}`}>
              <div className="flex items-center gap-2">
                <span role="img" aria-label={ok ? "Healthy" : "Unhealthy"} className={`inline-block h-3 w-3 rounded-full ${ok ? "bg-green-500" : "bg-red"}`} />
                <span className="text-sm font-bold text-ink">{check.name}</span>
              </div>
              {check.message && (
                <p className="mt-2 text-xs text-muted">{check.message}</p>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
