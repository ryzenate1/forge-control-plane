"use client";

import { useState, useEffect, useCallback } from "react";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

interface GitDeployment {
  id: string;
  gitSourceId: string;
  commitSha: string;
  branch: string;
  status: string;
  statusMessage: string;
  imageTag: string;
  buildLog: string;
  deployLog: string;
  error: string;
  startedAt: string;
  completedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

interface GitDeploymentHook {
  id: string;
  gitSourceId: string;
  secret: string;
  events: string[];
  createdAt: string;
  updatedAt: string;
}

export default function GitDeployPage() {
  const [deployments, setDeployments] = useState<GitDeployment[]>([]);
  const [hooks, setHooks] = useState<GitDeploymentHook[]>([]);
  const [loading, setLoading] = useState(true);
  const [repoUrl, setRepoUrl] = useState("");
  const [branch, setBranch] = useState("main");
  const [serverId, setServerId] = useState<string | null>(null);

  const fetchData = useCallback(async (sid: string) => {
    try {
      const [deployRes, hookRes] = await Promise.all([
        fetch(`/api/v1/git/servers/${sid}/deployments`),
        fetch(`/api/v1/git/servers/${sid}/hooks`),
      ]);
      if (deployRes.ok) {
        const data = await deployRes.json();
        setDeployments(Array.isArray(data) ? data : data.data ?? []);
      }
      if (hookRes.ok) {
        const data = await hookRes.json();
        setHooks(Array.isArray(data) ? data : data.data ?? []);
      }
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const sid = window.location.pathname.split("/").at(-2);
    if (sid && sid !== "git") {
      setServerId(sid);
      fetchData(sid);
    }
  }, [fetchData]);

  const triggerDeploy = async () => {
    if (!serverId || !repoUrl) return;
    try {
      const res = await fetch(`/api/v1/git/servers/${serverId}/deployments`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ repoUrl, branch }),
      });
      if (res.ok) {
        setRepoUrl("");
        setBranch("main");
        fetchData(serverId);
      }
    } catch {
      // ignore
    }
  };

  const createHook = async () => {
    if (!serverId) return;
    try {
      await fetch(`/api/v1/git/servers/${serverId}/hooks`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ events: ["push"] }),
      });
      fetchData(serverId);
    } catch {
      // ignore
    }
  };

  const deleteHook = async (hookId: string) => {
    if (!serverId) return;
    try {
      await fetch(`/api/v1/git/servers/${serverId}/hooks/${hookId}`, {
        method: "DELETE",
      });
      fetchData(serverId);
    } catch {
      // ignore
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case "success": return "text-green-400";
      case "failed": return "text-red-400";
      case "building": return "text-yellow-400";
      default: return "text-gray-400";
    }
  };

  return (
    <ServerConsoleLayout activeTab="console">
      {(server) => (
        <div className="p-6 space-y-8">
          <h2 className="text-2xl font-bold">Git Deployments</h2>

          <div className="bg-gray-800 rounded-lg p-4 space-y-4">
            <h3 className="text-lg font-semibold">Trigger Deployment</h3>
            <div className="flex gap-3">
              <input
                type="text"
                placeholder="Repository URL"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                className="flex-1 bg-gray-700 rounded px-3 py-2 text-sm"
              />
              <input
                type="text"
                placeholder="Branch"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                className="w-32 bg-gray-700 rounded px-3 py-2 text-sm"
              />
              <button
                onClick={triggerDeploy}
                disabled={!repoUrl}
                className="bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded px-4 py-2 text-sm font-medium"
              >
                Deploy
              </button>
            </div>
          </div>

          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold">Deployments</h3>
            </div>
            {loading ? (
              <p className="text-gray-400">Loading...</p>
            ) : deployments.length === 0 ? (
              <p className="text-gray-500">No deployments yet.</p>
            ) : (
              <div className="space-y-2">
                {deployments.map((d) => (
                    <div key={d.id} className="bg-gray-700 rounded p-3 text-sm flex items-center justify-between">
                    <div className="flex-1 min-w-0">
                      <p className="truncate font-medium">{d.gitSourceId || "N/A"}</p>
                      <p className="text-gray-400 text-xs">
                        {d.branch} @ {d.commitSha.slice(0, 8)} — {new Date(d.createdAt).toLocaleString()}
                      </p>
                    </div>
                    <span className={`ml-4 font-semibold ${statusColor(d.status)}`}>
                      {d.status}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="bg-gray-800 rounded-lg p-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold">Webhook Hooks</h3>
              <button
                onClick={createHook}
                className="bg-green-600 hover:bg-green-700 rounded px-3 py-1.5 text-sm font-medium"
              >
                Create Hook
              </button>
            </div>
            {hooks.length === 0 ? (
              <p className="text-gray-500">No hooks configured.</p>
            ) : (
              <div className="space-y-2">
                {hooks.map((h) => (
                  <div key={h.id} className="bg-gray-700 rounded p-3 text-sm flex items-center justify-between">
                    <div>
                      <p className="font-medium">Hook ID: {h.id.slice(0, 8)}...</p>
                      <p className="text-gray-400 text-xs">
                        Events: {h.events.join(", ")} — Created {new Date(h.createdAt).toLocaleString()}
                      </p>
                    </div>
                    <button
                      onClick={() => deleteHook(h.id)}
                      className="text-red-400 hover:text-red-300 text-xs font-medium"
                    >
                      Delete
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </ServerConsoleLayout>
  );
}
