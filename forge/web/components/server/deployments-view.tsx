"use client";

import { useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchJSON, postJSON, putJSON, type ApiServer } from "@/lib/api";
import { errorMessage } from "@/lib/utils";
import { AlertCircle, CheckCircle, Clock, Loader2, Play, RotateCcw, XCircle } from "lucide-react";
import { EmptyState, StatusPill } from "@/components/ui/primitives";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import { useOptionalServerContext, hasServerPermission } from "@/components/server/server-context";

interface Release {
  id: string;
  serverId: string;
  version: number;
  imageTag: string;
  status: string;
  createdAt: string;
  completedAt: string | null;
}

interface HealthCheckConfig {
  path: string;
  port: number;
  protocol: string;
  intervalSeconds: number;
  timeoutSeconds: number;
  healthyThreshold: number;
  unhealthyThreshold: number;
}

interface HealthCheckResult {
  id: string;
  deploymentId: string;
  checkTimestamp: string;
  status: string;
  responseCode: number;
  responseTimeMs: number;
  errorMessage: string;
}

interface DeploymentEvent {
  id: string;
  eventType: string;
  message: string;
  createdAt: string;
}

const statusIcons: Record<string, ReactNode> = {
  live: <CheckCircle className="h-4 w-4 text-emerald-500" />,
  failed: <XCircle className="h-4 w-4 text-red-400" />,
  rolled_back: <RotateCcw className="h-4 w-4 text-amber-400" />,
  pending: <Clock className="h-4 w-4 text-slate-500" />,
  building: <Loader2 className="h-4 w-4 animate-spin text-sky-400" />,
  deploying: <Loader2 className="h-4 w-4 animate-spin text-sky-400" />,
  health_checking: <Loader2 className="h-4 w-4 animate-spin text-purple-400" />,
};

const statusTone: Record<string, "neutral" | "success" | "warning" | "danger" | "info"> = {
  pending: "neutral",
  building: "info",
  deploying: "info",
  health_checking: "info",
  live: "success",
  rolled_back: "warning",
  failed: "danger",
};

const statusLabels: Record<string, string> = {
  pending: "Pending",
  building: "Building",
  deploying: "Deploying",
  health_checking: "Health Checking",
  live: "Live",
  rolled_back: "Rolled Back",
  failed: "Failed",
};

interface DeploymentsViewProps {
  server: ApiServer;
}

const DEFAULT_HC_CONFIG: HealthCheckConfig = {
  path: "/health",
  port: 8080,
  protocol: "http",
  intervalSeconds: 10,
  timeoutSeconds: 5,
  healthyThreshold: 2,
  unhealthyThreshold: 3,
};

const IN_FLIGHT_STATUSES = ["pending", "building", "deploying", "health_checking"];

export function DeploymentsView({ server }: DeploymentsViewProps) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canReinstall = hasServerPermission(access, "settings.reinstall");
  const qc = useQueryClient();

  const [error, setError] = useState<string | null>(null);
  const [imageTag, setImageTag] = useState("");
  const [selectedReleaseId, setSelectedReleaseId] = useState<string | null>(null);
  const [hcDraft, setHcDraft] = useState<HealthCheckConfig | null>(null);
  const [configSaved, setConfigSaved] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);
  const [showConfig, setShowConfig] = useState(false);

  const serverId = server.id;

  const releasesQuery = useQuery({
    queryKey: ["server-deployments", serverId],
    queryFn: async () => {
      const res = await fetchJSON<Release[] | { data: Release[] }>(`/servers/${serverId}/deployments`);
      return Array.isArray(res) ? res : (res?.data ?? []);
    },
    // Replaces the previous manual `setTimeout(loadReleases, 1000)` refresh:
    // poll while any release is in-flight, then stop.
    refetchInterval: (query) => {
      const rows = query.state.data ?? [];
      return rows.some((r) => IN_FLIGHT_STATUSES.includes(r.status)) ? 5000 : false;
    },
  });
  const releases = releasesQuery.data ?? [];
  const activeRelease = releases.find((r) => r.status === "live") ?? null;
  const selectedRelease = releases.find((r) => r.id === selectedReleaseId) ?? null;

  const healthConfigQuery = useQuery({
    queryKey: ["server-health-check", serverId],
    queryFn: async () => {
      const res = await fetchJSON<{ data: HealthCheckConfig }>(`/servers/${serverId}/health-check`);
      return res.data;
    },
    retry: false,
  });
  const hcConfig = hcDraft ?? healthConfigQuery.data ?? DEFAULT_HC_CONFIG;
  const setHcConfig = (next: HealthCheckConfig) => {
    setConfigSaved(false);
    setHcDraft(next);
  };

  const healthResultsQuery = useQuery({
    queryKey: ["deployment-health", serverId, selectedRelease?.id],
    queryFn: async () => {
      const res = await fetchJSON<{ data: HealthCheckResult[] }>(`/servers/${serverId}/deployments/${selectedRelease!.id}/health`);
      return res.data ?? [];
    },
    enabled: !!selectedRelease,
  });
  const healthResults = healthResultsQuery.data ?? [];

  const eventsQuery = useQuery({
    queryKey: ["deployment-events", serverId, selectedRelease?.id],
    queryFn: async () => {
      const res = await fetchJSON<{ data: DeploymentEvent[] }>(`/servers/${serverId}/deployments/${selectedRelease!.id}/events`);
      return res.data ?? [];
    },
    enabled: !!selectedRelease,
  });
  const events = eventsQuery.data ?? [];

  const deployMut = useMutation({
    mutationFn: (tag: string) => postJSON(`/servers/${serverId}/deployments`, { imageTag: tag }),
    onSuccess: () => {
      setImageTag("");
      setError(null);
      void qc.invalidateQueries({ queryKey: ["server-deployments", serverId] });
    },
    onError: (err) => setError(errorMessage(err, "Deployment failed")),
  });

  const rollbackMut = useMutation({
    mutationFn: (releaseId: string) => postJSON(`/servers/${serverId}/deployments/${releaseId}/rollback`),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ["server-deployments", serverId] });
    },
    onError: (err) => setError(errorMessage(err, "Rollback failed")),
  });

  const promoteMut = useMutation({
    mutationFn: (releaseId: string) => postJSON(`/servers/${serverId}/deployments/${releaseId}/promote`),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ["server-deployments", serverId] });
    },
    onError: (err) => setError(errorMessage(err, "Promotion failed")),
  });

  const saveConfigMut = useMutation({
    mutationFn: (config: HealthCheckConfig) => putJSON(`/servers/${serverId}/health-check`, config),
    onSuccess: () => {
      setConfigError(null);
      setConfigSaved(true);
      setHcDraft(null);
      void healthConfigQuery.refetch();
    },
    onError: (err) => setConfigError(errorMessage(err, "Failed to save config")),
  });

  const deploying = deployMut.isPending;
  const loading = releasesQuery.isLoading;
  const queryError = releasesQuery.isError ? errorMessage(releasesQuery.error, "Failed to load releases") : null;

  const handleDeploy = () => {
    const tag = imageTag.trim();
    if (!tag || deployMut.isPending) return;
    deployMut.mutate(tag);
  };

  const handleRollback = (releaseId: string) => rollbackMut.mutate(releaseId);
  const handleForcePromote = (releaseId: string) => promoteMut.mutate(releaseId);
  const handleSaveConfig = () => saveConfigMut.mutate(hcConfig);

  const selectRelease = (release: Release) => setSelectedReleaseId(release.id);

  if (loading) {
    return <CardSkeleton />;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-bold text-white">Deployments</h2>
        <button
          onClick={() => setShowConfig(!showConfig)}
          className="ui-button ui-button-ghost"
        >
          {showConfig ? "Hide" : "Configure"} health checks
        </button>
      </div>

      {(error || queryError) && (
        <div className="ui-alert ui-alert-error" role="alert">
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" /> {error || queryError}
        </div>
      )}

      <div className="flex gap-2">
        <input
          type="text"
          value={imageTag}
          onChange={(e) => setImageTag(e.target.value)}
          placeholder="Docker image tag (e.g. myapp:v2)"
          className="ui-input min-w-0 flex-1 font-mono"
          onKeyDown={(e) => e.key === "Enter" && handleDeploy()}
        />
        <button
          onClick={handleDeploy}
          disabled={!canReinstall || deploying || !imageTag.trim()}
          className="ui-button ui-button-primary"
        >
          {deploying ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
          Deploy
        </button>
      </div>

      {activeRelease && (
        <div className="ui-alert ui-alert-success">
          <CheckCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <div className="text-sm">
            <span className="font-semibold">Active release: </span>
            <span className="font-mono">v{activeRelease.version} — {activeRelease.imageTag}</span>
          </div>
        </div>
      )}

      {showConfig && (
        <div className="ui-card space-y-3">
          <h3 className="text-sm font-medium text-slate-200">Health check configuration</h3>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <div>
              <label className="ui-label" htmlFor="hc-path">Path</label>
              <input
                id="hc-path"
                type="text"
                value={hcConfig.path}
                onChange={(e) => setHcConfig({ ...hcConfig, path: e.target.value })}
                className="ui-input mt-1 font-mono"
              />
            </div>
            <div>
              <label className="ui-label" htmlFor="hc-port">Port</label>
              <input
                id="hc-port"
                type="number"
                value={hcConfig.port}
                onChange={(e) => setHcConfig({ ...hcConfig, port: parseInt(e.target.value) || 0 })}
                className="ui-input mt-1 font-mono"
              />
            </div>
            <div>
              <label className="ui-label" htmlFor="hc-interval">Interval (s)</label>
              <input
                id="hc-interval"
                type="number"
                value={hcConfig.intervalSeconds}
                onChange={(e) => setHcConfig({ ...hcConfig, intervalSeconds: parseInt(e.target.value) || 10 })}
                className="ui-input mt-1 font-mono"
              />
            </div>
            <div>
              <label className="ui-label" htmlFor="hc-timeout">Timeout (s)</label>
              <input
                id="hc-timeout"
                type="number"
                value={hcConfig.timeoutSeconds}
                onChange={(e) => setHcConfig({ ...hcConfig, timeoutSeconds: parseInt(e.target.value) || 5 })}
                className="ui-input mt-1 font-mono"
              />
            </div>
            <div>
              <label className="ui-label" htmlFor="hc-healthy">Healthy threshold</label>
              <input
                id="hc-healthy"
                type="number"
                value={hcConfig.healthyThreshold}
                onChange={(e) => setHcConfig({ ...hcConfig, healthyThreshold: parseInt(e.target.value) || 2 })}
                className="ui-input mt-1 font-mono"
              />
            </div>
            <div>
              <label className="ui-label" htmlFor="hc-unhealthy">Unhealthy threshold</label>
              <input
                id="hc-unhealthy"
                type="number"
                value={hcConfig.unhealthyThreshold}
                onChange={(e) => setHcConfig({ ...hcConfig, unhealthyThreshold: parseInt(e.target.value) || 3 })}
                className="ui-input mt-1 font-mono"
              />
            </div>
          </div>
          {configError && <p className="text-xs text-red-300">{configError}</p>}
          <button
            onClick={handleSaveConfig}
            className="ui-button ui-button-primary"
          >
            {configSaved ? "Saved!" : "Save configuration"}
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="space-y-2 lg:col-span-2">
          {releases.length === 0 ? (
            <EmptyState icon={<RotateCcw size={20} />} title="No deployments yet" description="Deploy an image tag above to create the first release. Releases appear here with their health-check results." />
          ) : (
            releases.map((release) => (
              <div
                key={release.id}
                onClick={() => selectRelease(release)}
                className={`flex cursor-pointer items-center justify-between gap-3 rounded-lg border p-3 transition-colors ${
                  selectedRelease?.id === release.id
                    ? "border-red-500/40 bg-red-500/10"
                    : "border-white/[0.06] bg-white/[0.02] hover:border-white/[0.14]"
                }`}
              >
                <div className="flex min-w-0 items-center gap-3">
                  {statusIcons[release.status] || <Clock className="h-4 w-4 text-slate-500" />}
                  <div className="min-w-0">
                    <span className="text-sm font-mono text-slate-100">
                      v{release.version}
                    </span>
                    <span className="ml-2 truncate text-xs text-slate-500">{release.imageTag}</span>
                  </div>
                  <StatusPill tone={statusTone[release.status] ?? "neutral"}>{statusLabels[release.status] || release.status}</StatusPill>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <span className="font-mono text-xs text-slate-500">
                    {new Date(release.createdAt).toLocaleString()}
                  </span>
                  {canReinstall && release.status === "live" && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleRollback(release.id); }}
                      className="inline-flex items-center gap-1 rounded-lg border border-amber-500/40 bg-amber-500/10 px-2 py-1 text-xs font-bold text-amber-200 hover:bg-amber-500/20"
                    >
                      <RotateCcw className="h-3 w-3" /> Rollback
                    </button>
                  )}
                  {(release.status === "deploying" || release.status === "health_checking") && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleForcePromote(release.id); }}
                      className="ui-button ui-button-secondary"
                    >
                      <Play className="h-3 w-3" /> Force promote
                    </button>
                  )}
                </div>
              </div>
            ))
          )}
        </div>

        {selectedRelease && (
          <div className="space-y-4">
            <div className="ui-card">
              <h3 className="mb-2 text-xs font-medium uppercase tracking-wider text-slate-400">Events</h3>
              {events.length === 0 ? (
                <p className="text-xs text-slate-500">No events</p>
              ) : (
                <div className="max-h-48 space-y-1.5 overflow-y-auto">
                  {events.map((ev) => (
                    <div key={ev.id} className="flex items-start gap-2 text-xs">
                      <span className="shrink-0 font-mono text-slate-500">
                        {new Date(ev.createdAt).toLocaleTimeString()}
                      </span>
                      <span className="text-slate-300">{ev.message}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {healthResults.length > 0 && (
              <div className="ui-card">
                <h3 className="mb-2 text-xs font-medium uppercase tracking-wider text-slate-400">Health results</h3>
                <div className="space-y-1">
                  {healthResults.slice(0, 20).map((hr) => (
                    <div key={hr.id} className="flex items-center justify-between text-xs">
                      <div className="flex items-center gap-1.5">
                        {hr.status === "healthy" ? (
                          <CheckCircle className="h-3 w-3 text-emerald-500" />
                        ) : (
                          <XCircle className="h-3 w-3 text-red-400" />
                        )}
                        <span className="font-mono text-slate-300">{hr.responseCode}</span>
                      </div>
                      <span className="font-mono text-slate-500">{hr.responseTimeMs}ms</span>
                      <span className="font-mono text-slate-500">
                        {new Date(hr.checkTimestamp).toLocaleTimeString()}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
