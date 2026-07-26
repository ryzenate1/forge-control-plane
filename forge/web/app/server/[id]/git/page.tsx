"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams } from "next/navigation";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import { fetchJSON, postJSON, deleteJSON } from "@/lib/api";
import { errorMessage } from "@/lib/utils";

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
  const params = useParams();
  const serverId = String(params.id ?? "");

  const fetchData = useCallback(async (sid: string) => {
    try {
      const [deployments, hooks] = await Promise.all([
        fetchJSON<GitDeployment[]>(`/git/servers/${sid}/deployments`),
        fetchJSON<GitDeploymentHook[]>(`/git/servers/${sid}/hooks`),
      ]);
      setDeployments(Array.isArray(deployments) ? deployments : []);
      setHooks(Array.isArray(hooks) ? hooks : []);
    } catch (err) {
      console.error("Failed to load git data:", errorMessage(err, ""));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (serverId) {
      fetchData(serverId);
    }
  }, [serverId, fetchData]);

  const triggerDeploy = async () => {
    if (!serverId || !repoUrl) return;
    try {
      await postJSON(`/git/servers/${serverId}/deployments`, { repoUrl, branch });
      setRepoUrl("");
      setBranch("main");
      fetchData(serverId);
    } catch (err) {
      console.error("Deploy failed:", errorMessage(err, ""));
    }
  };

  const createHook = async () => {
    if (!serverId) return;
    try {
      await postJSON(`/git/servers/${serverId}/hooks`, { events: ["push"] });
      fetchData(serverId);
    } catch (err) {
      console.error("Failed to create hook:", errorMessage(err, ""));
    }
  };

  const deleteHook = async (hookId: string) => {
    if (!serverId) return;
    try {
      await deleteJSON(`/git/servers/${serverId}/hooks/${hookId}`);
      fetchData(serverId);
    } catch (err) {
      console.error("Failed to delete hook:", errorMessage(err, ""));
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case "success": return "text-green-400";
      case "failed": return "text-red-400";
      case "building": return "text-yellow-400";
      default: return "text-slate-400";
    }
  };

  return (
    <ServerConsoleLayout activeTab="git">
      {() => (
        <div className="p-6 space-y-8">
          <h2 className="text-2xl font-bold text-slate-100">Git Deployments</h2>

          <div className="bg-[#1e2536] rounded-lg p-4 space-y-4">
            <h3 className="text-lg font-semibold text-slate-100">Trigger Deployment</h3>
            <div className="flex gap-3">
              <input
                type="text"
                placeholder="Repository URL"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                className="flex-1 bg-[#151b27] rounded px-3 py-2 text-sm text-slate-100 placeholder-slate-500"
              />
              <input
                type="text"
                placeholder="Branch"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                className="w-32 bg-[#151b27] rounded px-3 py-2 text-sm text-slate-100 placeholder-slate-500"
              />
              <button
                onClick={triggerDeploy}
                disabled={!repoUrl}
                className="bg-red-600 hover:bg-red-500 disabled:opacity-40 rounded px-4 py-2 text-sm font-bold text-white"
              >
                Deploy
              </button>
            </div>
          </div>

          <div className="bg-[#1e2536] rounded-lg p-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-slate-100">Deployments</h3>
            </div>
            {loading ? (
              <p className="text-slate-400">Loading...</p>
            ) : deployments.length === 0 ? (
              <p className="text-slate-500">No deployments yet.</p>
            ) : (
              <div className="space-y-2">
                {deployments.map((d) => (
                    <div key={d.id} className="bg-[#151b27] rounded p-3 text-sm flex items-center justify-between">
                    <div className="flex-1 min-w-0">
                      <p className="truncate font-medium text-slate-100">{d.gitSourceId || "N/A"}</p>
                      <p className="text-slate-400 text-xs">
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

          <div className="bg-[#1e2536] rounded-lg p-4">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-semibold text-slate-100">Webhook Hooks</h3>
              <button
                onClick={createHook}
                className="bg-green-700 hover:bg-green-600 rounded px-3 py-1.5 text-sm font-bold text-white"
              >
                Create Hook
              </button>
            </div>
            {hooks.length === 0 ? (
              <p className="text-slate-500">No hooks configured.</p>
            ) : (
              <div className="space-y-2">
                {hooks.map((h) => (
                  <div key={h.id} className="bg-[#151b27] rounded p-3 text-sm flex items-center justify-between">
                    <div>
                      <p className="font-medium text-slate-100">Hook ID: {h.id.slice(0, 8)}...</p>
                      <p className="text-slate-400 text-xs">
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
