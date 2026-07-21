"use client";

import { useCallback, useEffect, useState, type ReactNode } from "react";
import { fetchJSON, postJSON, putJSON, type ApiServer } from "@/lib/api";
import { errorMessage } from "@/lib/utils";
import { AlertCircle, CheckCircle, Clock, Loader2, Play, RotateCcw, XCircle } from "lucide-react";

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
  live: <CheckCircle className="w-4 h-4 text-green-500" />,
  failed: <XCircle className="w-4 h-4 text-red-500" />,
  rolled_back: <RotateCcw className="w-4 h-4 text-yellow-500" />,
  pending: <Clock className="w-4 h-4 text-gray-400" />,
  building: <Loader2 className="w-4 h-4 text-blue-500 animate-spin" />,
  deploying: <Loader2 className="w-4 h-4 text-blue-500 animate-spin" />,
  health_checking: <Loader2 className="w-4 h-4 text-purple-500 animate-spin" />,
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

export function DeploymentsView({ server }: DeploymentsViewProps) {
  const [releases, setReleases] = useState<Release[]>([]);
  const [activeRelease, setActiveRelease] = useState<Release | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deploying, setDeploying] = useState(false);
  const [imageTag, setImageTag] = useState("");
  const [selectedRelease, setSelectedRelease] = useState<Release | null>(null);
  const [healthResults, setHealthResults] = useState<HealthCheckResult[]>([]);
  const [events, setEvents] = useState<DeploymentEvent[]>([]);
  const [hcConfig, setHcConfig] = useState<HealthCheckConfig>({
    path: "/health",
    port: 8080,
    protocol: "http",
    intervalSeconds: 10,
    timeoutSeconds: 5,
    healthyThreshold: 2,
    unhealthyThreshold: 3,
  });
  const [configSaved, setConfigSaved] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);
  const [showConfig, setShowConfig] = useState(false);

  const serverId = server.id;
  const apiPath = (path: string) => `/servers/${serverId}${path}`;

  const loadReleases = useCallback(async () => {
    try {
      const res = await fetchJSON<{ data: Release[] }>(apiPath("/deployments"));
      setReleases(res.data);
      const live = res.data.find((r) => r.status === "live");
      setActiveRelease(live ?? null);
    } catch (err) {
      setError(errorMessage(err, "Failed to load releases"));
    } finally {
      setLoading(false);
    }
  }, [serverId]);

  const loadHealthConfig = useCallback(async () => {
    try {
      const res = await fetchJSON<{ data: HealthCheckConfig }>(apiPath("/health-check"));
      setHcConfig(res.data);
    } catch {
      // No config yet - use defaults
    }
  }, [serverId]);

  const loadHealthResults = useCallback(async (releaseId: string) => {
    try {
      const res = await fetchJSON<{ data: HealthCheckResult[] }>(apiPath(`/deployments/${releaseId}/health`));
      setHealthResults(res.data);
    } catch {
      setHealthResults([]);
    }
  }, [serverId]);

  const loadEvents = useCallback(async (releaseId: string) => {
    try {
      const res = await fetchJSON<{ data: DeploymentEvent[] }>(apiPath(`/deployments/${releaseId}/events`));
      setEvents(res.data);
    } catch {
      setEvents([]);
    }
  }, [serverId]);

  useEffect(() => {
    loadReleases();
    loadHealthConfig();
  }, [loadReleases, loadHealthConfig]);

  const handleDeploy = async () => {
    if (!imageTag.trim()) return;
    setDeploying(true);
    setError(null);
    try {
      await postJSON(apiPath("/deployments"), { imageTag: imageTag.trim() });
      setImageTag("");
      setTimeout(loadReleases, 1000);
    } catch (err) {
      setError(errorMessage(err, "Deployment failed"));
    } finally {
      setDeploying(false);
    }
  };

  const handleRollback = async (releaseId: string) => {
    try {
      await postJSON(apiPath(`/deployments/${releaseId}/rollback`));
      setTimeout(loadReleases, 1000);
    } catch (err) {
      setError(errorMessage(err, "Rollback failed"));
    }
  };

  const handleForcePromote = async (releaseId: string) => {
    try {
      await postJSON(apiPath(`/deployments/${releaseId}/promote`));
      setTimeout(loadReleases, 1000);
    } catch (err) {
      setError(errorMessage(err, "Promotion failed"));
    }
  };

  const handleSaveConfig = async () => {
    setConfigError(null);
    try {
      await putJSON(apiPath("/health-check"), hcConfig);
      setConfigSaved(true);
      setTimeout(() => setConfigSaved(false), 2000);
    } catch (err) {
      setConfigError(errorMessage(err, "Failed to save config"));
    }
  };

  const selectRelease = async (release: Release) => {
    setSelectedRelease(release);
    loadHealthResults(release.id);
    loadEvents(release.id);
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-6 h-6 animate-spin mr-2" /> Loading deployments...
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-gray-100">Deployments</h2>
        <button
          onClick={() => setShowConfig(!showConfig)}
          className="text-xs text-gray-400 hover:text-gray-200 underline"
        >
          {showConfig ? "Hide" : "Configure"} Health Checks
        </button>
      </div>

      {error && (
        <div className="flex items-center gap-2 p-3 bg-red-900/20 border border-red-700 rounded text-red-400 text-sm">
          <AlertCircle className="w-4 h-4 shrink-0" /> {error}
        </div>
      )}

      <div className="flex gap-2">
        <input
          type="text"
          value={imageTag}
          onChange={(e) => setImageTag(e.target.value)}
          placeholder="Docker image tag (e.g. myapp:v2)"
          className="flex-1 px-3 py-2 bg-gray-800 border border-gray-700 rounded text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500"
          onKeyDown={(e) => e.key === "Enter" && handleDeploy()}
        />
        <button
          onClick={handleDeploy}
          disabled={deploying || !imageTag.trim()}
          className="flex items-center gap-1.5 px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-gray-700 disabled:text-gray-500 rounded text-sm text-white transition-colors"
        >
          {deploying ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
          Deploy
        </button>
      </div>

      {activeRelease && (
        <div className="flex items-center gap-2 p-3 bg-green-900/10 border border-green-800 rounded text-sm">
          <CheckCircle className="w-4 h-4 text-green-500 shrink-0" />
          <span className="text-green-300 font-medium">Active Release:</span>
          <span className="text-gray-200">
            v{activeRelease.version} — {activeRelease.imageTag}
          </span>
        </div>
      )}

      {showConfig && (
        <div className="p-4 bg-gray-800/50 border border-gray-700 rounded space-y-3">
          <h3 className="text-sm font-medium text-gray-200">Health Check Configuration</h3>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            <div>
              <label className="block text-xs text-gray-400 mb-1">Path</label>
              <input
                type="text"
                value={hcConfig.path}
                onChange={(e) => setHcConfig({ ...hcConfig, path: e.target.value })}
                className="w-full px-2 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-100"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Port</label>
              <input
                type="number"
                value={hcConfig.port}
                onChange={(e) => setHcConfig({ ...hcConfig, port: parseInt(e.target.value) || 0 })}
                className="w-full px-2 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-100"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Interval (s)</label>
              <input
                type="number"
                value={hcConfig.intervalSeconds}
                onChange={(e) => setHcConfig({ ...hcConfig, intervalSeconds: parseInt(e.target.value) || 10 })}
                className="w-full px-2 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-100"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Timeout (s)</label>
              <input
                type="number"
                value={hcConfig.timeoutSeconds}
                onChange={(e) => setHcConfig({ ...hcConfig, timeoutSeconds: parseInt(e.target.value) || 5 })}
                className="w-full px-2 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-100"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Healthy Threshold</label>
              <input
                type="number"
                value={hcConfig.healthyThreshold}
                onChange={(e) => setHcConfig({ ...hcConfig, healthyThreshold: parseInt(e.target.value) || 2 })}
                className="w-full px-2 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-100"
              />
            </div>
            <div>
              <label className="block text-xs text-gray-400 mb-1">Unhealthy Threshold</label>
              <input
                type="number"
                value={hcConfig.unhealthyThreshold}
                onChange={(e) => setHcConfig({ ...hcConfig, unhealthyThreshold: parseInt(e.target.value) || 3 })}
                className="w-full px-2 py-1.5 bg-gray-800 border border-gray-700 rounded text-sm text-gray-100"
              />
            </div>
          </div>
          {configError && <p className="text-xs text-red-400">{configError}</p>}
          <button
            onClick={handleSaveConfig}
            className="px-3 py-1.5 bg-green-700 hover:bg-green-600 rounded text-xs text-white transition-colors"
          >
            {configSaved ? "Saved!" : "Save Configuration"}
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2 space-y-2">
          {releases.length === 0 ? (
            <p className="text-gray-500 text-sm py-8 text-center">No deployments yet.</p>
          ) : (
            releases.map((release) => (
              <div
                key={release.id}
                onClick={() => selectRelease(release)}
                className={`flex items-center justify-between p-3 rounded border cursor-pointer transition-colors ${
                  selectedRelease?.id === release.id
                    ? "bg-blue-900/20 border-blue-700"
                    : "bg-gray-800/50 border-gray-700 hover:border-gray-600"
                }`}
              >
                <div className="flex items-center gap-3">
                  {statusIcons[release.status] || <Clock className="w-4 h-4 text-gray-400" />}
                  <div>
                    <span className="text-sm font-mono text-gray-100">
                      v{release.version}
                    </span>
                    <span className="text-xs text-gray-500 ml-2">{release.imageTag}</span>
                  </div>
                  <span className="text-xs px-2 py-0.5 rounded bg-gray-700 text-gray-300">
                    {statusLabels[release.status] || release.status}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-xs text-gray-500">
                    {new Date(release.createdAt).toLocaleString()}
                  </span>
                  {release.status === "live" && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleRollback(release.id); }}
                      className="flex items-center gap-1 px-2 py-1 bg-yellow-700/50 hover:bg-yellow-700 rounded text-xs text-yellow-300"
                    >
                      <RotateCcw className="w-3 h-3" /> Rollback
                    </button>
                  )}
                  {(release.status === "deploying" || release.status === "health_checking") && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleForcePromote(release.id); }}
                      className="flex items-center gap-1 px-2 py-1 bg-blue-700/50 hover:bg-blue-700 rounded text-xs text-blue-300"
                    >
                      <Play className="w-3 h-3" /> Force Promote
                    </button>
                  )}
                </div>
              </div>
            ))
          )}
        </div>

        {selectedRelease && (
          <div className="space-y-4">
            <div className="p-3 bg-gray-800/50 border border-gray-700 rounded">
              <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">Events</h3>
              {events.length === 0 ? (
                <p className="text-xs text-gray-500">No events</p>
              ) : (
                <div className="space-y-1.5 max-h-48 overflow-y-auto">
                  {events.map((ev) => (
                    <div key={ev.id} className="flex items-start gap-2 text-xs">
                      <span className="text-gray-500 shrink-0 font-mono">
                        {new Date(ev.createdAt).toLocaleTimeString()}
                      </span>
                      <span className="text-gray-300">{ev.message}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {healthResults.length > 0 && (
              <div className="p-3 bg-gray-800/50 border border-gray-700 rounded">
                <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-2">Health Results</h3>
                <div className="space-y-1">
                  {healthResults.slice(0, 20).map((hr) => (
                    <div key={hr.id} className="flex items-center justify-between text-xs">
                      <div className="flex items-center gap-1.5">
                        {hr.status === "healthy" ? (
                          <CheckCircle className="w-3 h-3 text-green-500" />
                        ) : (
                          <XCircle className="w-3 h-3 text-red-500" />
                        )}
                        <span className="text-gray-300">{hr.responseCode}</span>
                      </div>
                      <span className="text-gray-500">{hr.responseTimeMs}ms</span>
                      <span className="text-gray-500 font-mono">
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
