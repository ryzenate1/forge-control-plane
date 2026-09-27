"use client";

import { useState } from "react";
import { OfflineBanner } from "@/components/shared/states-offline";
import { AdminCard, AdminPageLayout } from "@/components/admin/admin-layout";
import * as api from "@/lib/api/zerodowntime";
import { ApiError } from "@/lib/api/http";
import { sanitizeError } from "@/lib/sanitize";

export function ZerodowntimeManager() {
  const [serverId, setServerId] = useState("");
  const [releases, setReleases] = useState<api.Release[]>([]);
  const [selected, setSelected] = useState<api.Release | null>(null);
  const [health, setHealth] = useState<api.HealthCheckConfig | null>(null);
  const [events, setEvents] = useState<api.DeploymentEvent[]>([]);
  const [healthResults, setHealthResults] = useState<api.HealthCheckResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [imageTag, setImageTag] = useState("");

  const [hcForm, setHcForm] = useState<api.HealthCheckConfig>({
    path: "/health",
    port: 8080,
    protocol: "http",
    intervalSeconds: 10,
    timeoutSeconds: 5,
    healthyThreshold: 2,
    unhealthyThreshold: 3,
  });

  async function loadReleases() {
    if (!serverId.trim()) {
      setError("Server ID required");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const [list, h] = await Promise.all([
        api.listReleases(serverId.trim()),
        // A server with no health-check config yet answers 404, which is a real
        // "not configured" state; any other failure must surface instead of
        // quietly leaving the previous/default form on screen.
        api.getHealthCheckConfig(serverId.trim()).catch((e: unknown) => {
          if (e instanceof ApiError && e.status === 404) return null;
          throw e;
        }),
      ]);
      setReleases(list);
      if (h) {
        setHealth(h);
        setHcForm(h);
      }
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Failed to load releases"));
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate() {
    if (!serverId.trim() || !imageTag.trim()) {
      setError("serverId and imageTag required");
      return;
    }
    try {
      await api.createRelease(serverId.trim(), imageTag.trim());
      setSuccess(`Release ${imageTag} created (deploy + health + promote runs async)`);
      setImageTag("");
      await loadReleases();
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Create release failed"));
    }
  }

  async function handleRollback(releaseId: string) {
    if (!confirm("Rollback this release?")) return;
    try {
      await api.rollbackRelease(serverId.trim(), releaseId);
      setSuccess("Rolled back");
      await loadReleases();
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Rollback failed"));
    }
  }

  async function handlePromote(releaseId: string) {
    try {
      await api.promoteRelease(serverId.trim(), releaseId);
      setSuccess("Promoted to live");
      await loadReleases();
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Promote failed"));
    }
  }

  async function handleSaveHc() {
    try {
      const saved = await api.upsertHealthCheckConfig(serverId.trim(), hcForm);
      setHealth(saved);
      setSuccess("Health check config saved");
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Save health check failed"));
    }
  }

  async function handleSelect(release: api.Release) {
    setSelected(release);
    // Drop the previous release's details first: if either request fails we
    // surface the error below rather than showing stale rows as if they were
    // this release's (empty) events and health results.
    setEvents([]);
    setHealthResults([]);
    try {
      const [ev, hr] = await Promise.all([
        api.getDeploymentEvents(serverId.trim(), release.id),
        api.getHealthCheckResults(serverId.trim(), release.id),
      ]);
      setEvents(ev);
      setHealthResults([...hr].sort((a, b) => new Date(b.checkTimestamp).getTime() - new Date(a.checkTimestamp).getTime()));
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Load details failed"));
    }
  }

  // Auto-load when serverId changes? No, explicit.

  return (
    <AdminPageLayout
      title="Zero-Downtime Releases"
      description="522L service: CreateRelease → DeployRelease → RunHealthChecks (ticker + thresholds, 2m max) → PromoteRelease / RollbackRelease. Health checks hit allocation IP:port + path. Requires server + allocation."
    >
      <OfflineBanner onRetry={() => { if (serverId.trim()) void loadReleases(); }} />
      {error && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-500/[0.09] p-4 text-sm text-red-200">
          <span>{error}</span> <button onClick={() => setError(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}
      {success && (
        <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.09] p-4 text-sm text-emerald-200">
          <span>{success}</span> <button onClick={() => setSuccess(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}

      <AdminCard title="Server Selector" description="Enter a server ID (from Management > Servers) to manage its deployment releases.">
        <div className="flex gap-2">
          <input value={serverId} onChange={(e) => setServerId(e.target.value)} placeholder="serverId (uuid)" className="flex-1 rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm font-mono" />
          <button onClick={() => void loadReleases()} className="rounded bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white">Load</button>
        </div>
        <p className="mt-2 text-xs text-[var(--text-subtle)]">Endpoints: POST /servers/:id/deployments, GET /servers/:id/deployments, GET /servers/:id/deployments/:releaseId, POST /servers/:id/deployments/:releaseId/rollback, POST /servers/:id/deployments/:releaseId/promote, PUT/GET /servers/:id/health-check</p>
      </AdminCard>

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title={`Releases ${releases.length ? `(${releases.length})` : ""}`} description="Ordered by version DESC. Status: pending → deploying → health_checking → live | rolled_back | failed. Auto-deploy runs async on create.">
          {loading ? (
            <p className="text-sm text-[var(--text-subtle)]">Loading…</p>
          ) : releases.length === 0 ? (
            <p className="text-sm text-[var(--text-subtle)]">{serverId ? "No releases for this server." : "Load a server to see releases."}</p>
          ) : (
            <div className="space-y-2 max-h-[520px] overflow-auto">
              {releases.map((r) => (
                <div key={r.id} className={`rounded-lg border p-3 ${selected?.id === r.id ? "border-red-400 bg-[var(--brand)]-wash" : "border-[var(--line)] bg-surface"}`}>
                  <div className="flex items-center justify-between">
                    <p className="text-sm font-bold text-[var(--text)]">v{r.version} · {r.imageTag} <span className={`ml-2 rounded-full px-2 py-0.5 text-xs ${r.status === "live" ? "bg-green-100 text-emerald-200" : r.status === "failed" ? "bg-[var(--brand)]-100 text-red-700" : "bg-amber-100 text-amber-700"}`}>{r.status}</span></p>
                    <span className="text-xs text-[var(--text-subtle)]">{new Date(r.createdAt).toLocaleString()}</span>
                  </div>
                  <div className="mt-2 flex gap-1">
                    <button onClick={() => void handleSelect(r)} className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-2 py-1 text-xs">Inspect</button>
                    <button onClick={() => void handlePromote(r.id)} className="rounded border border-[var(--line)] px-2 py-1 text-xs">Promote</button>
                    <button onClick={() => void handleRollback(r.id)} className="rounded border border-red-200 px-2 py-1 text-xs text-[var(--brand)]">Rollback</button>
                  </div>
                </div>
              ))}
            </div>
          )}

          <div className="mt-4 flex gap-2">
            <input value={imageTag} onChange={(e) => setImageTag(e.target.value)} placeholder="imageTag (e.g. myapp:1.2.3)" className="flex-1 rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
            <button onClick={() => void handleCreate()} className="rounded bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white">Deploy</button>
          </div>
          <p className="mt-2 text-xs text-[var(--text-subtle)]">Create enqueues deploy → health checks → promote async; check events for progress.</p>
        </AdminCard>

        <div className="space-y-6">
          <AdminCard title="Health Check Config" description="Per-server. Protocol http|https, path must be absolute URL path, port 1-65535. Interval/timeout seconds, healthy/unhealthy thresholds.">
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-xs font-bold uppercase text-[var(--text-subtle)]">Path</label>
                  <input value={hcForm.path} onChange={(e) => setHcForm({ ...hcForm, path: e.target.value })} className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
                </div>
                <div>
                  <label className="text-xs font-bold uppercase text-[var(--text-subtle)]">Port</label>
                  <input type="number" value={hcForm.port} onChange={(e) => setHcForm({ ...hcForm, port: Number(e.target.value) })} className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-xs font-bold uppercase text-[var(--text-subtle)]">Protocol</label>
                  <select value={hcForm.protocol} onChange={(e) => setHcForm({ ...hcForm, protocol: e.target.value })} className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm">
                    <option value="http">http</option>
                    <option value="https">https</option>
                  </select>
                </div>
                <div>
                  <label className="text-xs font-bold uppercase text-[var(--text-subtle)]">Interval (s)</label>
                  <input type="number" value={hcForm.intervalSeconds} onChange={(e) => setHcForm({ ...hcForm, intervalSeconds: Number(e.target.value) })} className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="text-xs font-bold uppercase text-[var(--text-subtle)]">Timeout (s)</label>
                  <input type="number" value={hcForm.timeoutSeconds} onChange={(e) => setHcForm({ ...hcForm, timeoutSeconds: Number(e.target.value) })} className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
                </div>
                <div>
                  <label className="text-xs font-bold uppercase text-[var(--text-subtle)]">Healthy threshold</label>
                  <input type="number" value={hcForm.healthyThreshold} onChange={(e) => setHcForm({ ...hcForm, healthyThreshold: Number(e.target.value) })} className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
                </div>
                <div>
                  <label className="text-xs font-bold uppercase text-[var(--text-subtle)]">Unhealthy threshold</label>
                  <input type="number" value={hcForm.unhealthyThreshold} onChange={(e) => setHcForm({ ...hcForm, unhealthyThreshold: Number(e.target.value) })} className="w-full rounded border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm" />
                </div>
              </div>
              {health && <p className="text-xs text-[var(--text-subtle)]">Current: {health.protocol}://allocation:{health.port}{health.path} · {health.intervalSeconds}s interval · {health.healthyThreshold}/{health.unhealthyThreshold}</p>}
              <button onClick={() => void handleSaveHc()} className="rounded bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white">Save Health Config</button>
            </div>
          </AdminCard>

          <AdminCard title={selected ? `Release v${selected.version} — Details` : "Release Details"} description={selected ? `ID ${selected.id.slice(0, 8)}… · created ${new Date(selected.createdAt).toLocaleString()}` : "Select a release to see events and health results"}>
            {!selected ? (
              <p className="text-sm text-[var(--text-subtle)]">No selection.</p>
            ) : (
              <div className="space-y-4">
                <div>
                  <p className="text-xs font-bold uppercase text-[var(--text-subtle)]">Deployment Events</p>
                  {events.length === 0 ? <p className="text-xs text-[var(--text-subtle)]">No events.</p> : (
                    <div className="mt-2 space-y-1 max-h-40 overflow-auto">
                      {events.map((ev) => (
                        <div key={ev.id} className="flex justify-between rounded border border-[var(--line)] bg-surface px-3 py-1.5 text-xs">
                          <span><b>{ev.eventType}</b> — {ev.message}</span>
                          <span className="text-[var(--text-subtle)]">{new Date(ev.createdAt).toLocaleTimeString()}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                <div>
                  <p className="text-xs font-bold uppercase text-[var(--text-subtle)]">Health Check Results</p>
                  {healthResults.length === 0 ? <p className="text-xs text-[var(--text-subtle)]">No health checks (config missing or not yet run).</p> : (
                    <div className="mt-2 space-y-1 max-h-40 overflow-auto">
                      {healthResults.map((h) => (
                        <div key={h.id} className={`flex justify-between rounded border px-3 py-1.5 text-xs ${h.status === "healthy" ? "border-green-200 bg-green-50" : "border-red-200 bg-[var(--brand)]-wash"}`}>
                          <span>{h.status} · {h.responseCode || "—"} · {h.responseTimeMs}ms {h.errorMessage && `· ${h.errorMessage}`}</span>
                          <span className="text-[var(--text-subtle)]">{new Date(h.checkTimestamp).toLocaleTimeString()}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            )}
          </AdminCard>
        </div>
      </div>
    </AdminPageLayout>
  );
}
