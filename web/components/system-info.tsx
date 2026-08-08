"use client";

import { useEffect, useRef, useState } from "react";
import type { SystemInfo } from "@/lib/api";
import { sanitizeError } from "@/lib/sanitize";
import { formatBytes, formatUptime } from "@/lib/format";
import { redirectOnUnauthorized } from "@/lib/client-auth";

const POLL_INTERVAL_MS = 30000;

export function SystemInfoDisplay() {
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const setPollError = (v: boolean) => { void v; };
  const [refreshKey, setRefreshKey] = useState(0);
  const serverTokenRef = useRef(0);
  const refresh = () => setRefreshKey((k) => k + 1);
  const dismissError = () => setError(null);

  useEffect(() => {
    const controller = new AbortController();
    const token = ++serverTokenRef.current;
    setError(null);
    fetch("/api/proxy/system", { signal: controller.signal, credentials: "include" })
      .then(async (r) => {
		redirectOnUnauthorized(r);
        if (!r.ok) {
          const body = await r.json().catch(() => null);
          throw new Error(body?.error || `Failed to load: ${r.status}`);
        }
        return r.json();
      })
      .then((data) => { if (token === serverTokenRef.current) setInfo(data); })
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
      fetch("/api/proxy/system", { signal: lastController.signal, credentials: "include" })
        .then((r) => {
			redirectOnUnauthorized(r);
          if (!r.ok) {
            const body = r.clone();
            return body.json().catch(() => null).then((parsed) => {
              throw new Error(parsed?.error || `Failed to load: ${r.status}`);
            });
          }
          return r.json();
        })
        .then((data) => { if (token === serverTokenRef.current) setInfo(data); })
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
            <p className="font-bold text-red-dark dark:text-red-300">Failed to load system info</p>
            <p className="mt-1 text-sm text-muted">{error}</p>
          </div>
          <button onClick={dismissError} aria-label="Dismiss error" className="ml-auto flex h-10 w-10 items-center justify-center text-xs text-muted hover:text-ink">×</button>
        </div>
      </div>
    );
  }

  if (!info) {
    return (
      <div role="status" aria-live="polite" className="rounded-xl border border-line bg-paper p-6">
        <p className="text-muted">Loading system information…</p>
      </div>
    );
  }

  const memPercent = info.memory.total > 0 ? ((info.memory.used / info.memory.total) * 100).toFixed(1) : "0";
  const diskPercent = info.disk.total > 0 ? ((info.disk.used / info.disk.total) * 100).toFixed(1) : "0";

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <button onClick={refresh} aria-label="Refresh system information" className="text-xs text-muted hover:text-ink transition-colors">
          ↻ Refresh
        </button>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div className="rounded-xl border border-line bg-paper p-5">
          <p className="text-xs font-bold uppercase text-muted">Version</p>
          <p className="mt-1 text-lg font-bold text-ink">{info.version}</p>
        </div>
        <div className="rounded-xl border border-line bg-paper p-5">
          <p className="text-xs font-bold uppercase text-muted">Operating System</p>
          <p className="mt-1 text-lg font-bold text-ink">{info.os}</p>
        </div>
        <div className="rounded-xl border border-line bg-paper p-5">
          <p className="text-xs font-bold uppercase text-muted">Architecture</p>
          <p className="mt-1 text-lg font-bold text-ink">{info.architecture}</p>
        </div>
        <div className="rounded-xl border border-line bg-paper p-5">
          <p className="text-xs font-bold uppercase text-muted">CPU Threads</p>
          <p className="mt-1 text-lg font-bold text-ink">{info.cpuThreads}</p>
        </div>
        <div className="rounded-xl border border-line bg-paper p-5">
          <p className="text-xs font-bold uppercase text-muted">Beacon Runtime</p>
          <div className="mt-1 flex items-center gap-2">
            <span role="img" aria-label={info.runtime.reachable ? "Healthy" : "Unhealthy"} className={`inline-block h-3 w-3 rounded-full ${info.runtime.reachable ? "bg-green-500" : "bg-red"}`} />
            <span className="text-lg font-bold text-ink">
              {info.runtime.reachable ? "Reachable" : "Unavailable"}
            </span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-paper p-5">
          <p className="text-xs font-bold uppercase text-muted">Active Sessions</p>
          <p className="mt-1 text-lg font-bold text-ink">{info.activeSessions}</p>
        </div>
      </div>

      <div className="rounded-xl border border-line bg-paper p-6">
        <p className="text-xs font-bold uppercase text-muted">Uptime</p>
        <p className="mt-1 font-bold text-ink">{formatUptime(info.uptime)}</p>
      </div>

      <div className="grid gap-6 sm:grid-cols-2">
        <div className="rounded-xl border border-line bg-paper p-6">
          <p className="text-xs font-bold uppercase text-muted">Memory</p>
          <p className="mt-1 text-2xl font-bold text-ink">{formatBytes(info.memory.used)}</p>
          <div
            role="progressbar"
            aria-valuenow={parseFloat(memPercent)}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label={`Memory usage: ${memPercent}%`}
            className="mt-2 h-2 overflow-hidden rounded-full bg-surface"
          >
            <div className={`h-full rounded-full transition-colors ${parseFloat(memPercent) > 90 ? 'bg-red-500' : parseFloat(memPercent) > 70 ? 'bg-amber-500' : 'bg-green-500'}`} style={{ width: `${memPercent}%`, transition: "width 0.4s ease, background-color 0.3s ease" }} />
          </div>
          <p className="mt-1 text-xs text-muted">{formatBytes(info.memory.free)} free of {formatBytes(info.memory.total)}</p>
        </div>

        <div className="rounded-xl border border-line bg-paper p-6">
          <p className="text-xs font-bold uppercase text-muted">Disk</p>
          <p className="mt-1 text-2xl font-bold text-ink">{formatBytes(info.disk.used)}</p>
          <div
            role="progressbar"
            aria-valuenow={parseFloat(diskPercent)}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label={`Disk usage: ${diskPercent}%`}
            className="mt-2 h-2 overflow-hidden rounded-full bg-surface"
          >
            <div className={`h-full rounded-full transition-colors ${parseFloat(diskPercent) > 90 ? 'bg-red-500' : parseFloat(diskPercent) > 70 ? 'bg-amber-500' : 'bg-green-500'}`} style={{ width: `${diskPercent}%`, transition: "width 0.4s ease, background-color 0.3s ease" }} />
          </div>
          <p className="mt-1 text-xs text-muted">{formatBytes(info.disk.free)} free of {formatBytes(info.disk.total)}</p>
        </div>
      </div>
    </div>
  );
}
