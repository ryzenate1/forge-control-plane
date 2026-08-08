"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams } from "next/navigation";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import { fetchJSON, postJSON, deleteJSON } from "@/lib/api";
import { errorMessage } from "@/lib/utils";
import { ConfirmDialog, EmptyState, StatusPill } from "@/components/ui/primitives";
import { CardSkeleton } from "@/components/ui/loading-skeleton";

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

const statusTone: Record<string, "neutral" | "success" | "warning" | "danger"> = {
  success: "success",
  failed: "danger",
  building: "warning",
};

export default function GitDeployPage() {
  const [deployments, setDeployments] = useState<GitDeployment[]>([]);
  const [hooks, setHooks] = useState<GitDeploymentHook[]>([]);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [deployError, setDeployError] = useState<string | null>(null);
  const [hookError, setHookError] = useState<string | null>(null);
  const [repoUrl, setRepoUrl] = useState("");
  const [branch, setBranch] = useState("main");
  const [hookToDelete, setHookToDelete] = useState<GitDeploymentHook | null>(null);
  const [deletingHook, setDeletingHook] = useState(false);
  const params = useParams();
  const serverId = String(params.id ?? "");

  const fetchData = useCallback(async (sid: string) => {
    try {
      setFetchError(null);
      const [deployments, hooks] = await Promise.all([
        fetchJSON<GitDeployment[]>(`/git/servers/${sid}/deployments`),
        fetchJSON<GitDeploymentHook[]>(`/git/servers/${sid}/hooks`),
      ]);
      setDeployments(Array.isArray(deployments) ? deployments : []);
      setHooks(Array.isArray(hooks) ? hooks : []);
    } catch (err) {
      setFetchError(errorMessage(err, "Failed to load git data"));
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
      setDeployError(null);
      await postJSON(`/git/servers/${serverId}/deployments`, { repoUrl, branch });
      setRepoUrl("");
      setBranch("main");
      fetchData(serverId);
    } catch (err) {
      setDeployError(errorMessage(err, "Deploy failed"));
    }
  };

  const createHook = async () => {
    if (!serverId) return;
    try {
      setHookError(null);
      await postJSON(`/git/servers/${serverId}/hooks`, { events: ["push"] });
      fetchData(serverId);
    } catch (err) {
      setHookError(errorMessage(err, "Failed to create hook"));
    }
  };

  const deleteHook = async (hookId: string) => {
    if (!serverId) return;
    setDeletingHook(true);
    try {
      setHookError(null);
      await deleteJSON(`/git/servers/${serverId}/hooks/${hookId}`);
      setHookToDelete(null);
      fetchData(serverId);
    } catch (err) {
      setHookError(errorMessage(err, "Failed to delete hook"));
    } finally {
      setDeletingHook(false);
    }
  };

  return (
    <ServerConsoleLayout activeTab="git">
      {() => (
        <div className="space-y-8">
          <h2 className="text-2xl font-bold text-white">Git Deployments</h2>

          {fetchError && (
            <div className="ui-alert ui-alert-error" role="alert">
              <p className="text-sm">{fetchError}</p>
              <button className="ui-button ui-button-secondary" onClick={() => { setLoading(true); void fetchData(serverId); }} type="button">Retry</button>
            </div>
          )}

          <div className="ui-card space-y-4">
            <h3 className="text-lg font-semibold text-slate-100">Trigger deployment</h3>
            {deployError && (
              <div className="ui-alert ui-alert-error" role="alert">
                <p className="text-sm">{deployError} Verify the repository URL and branch are reachable by the daemon, then retry.</p>
              </div>
            )}
            <div className="flex flex-col gap-3 sm:flex-row">
              <input
                type="text"
                placeholder="Repository URL"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                className="ui-input min-w-0 flex-1 font-mono"
              />
              <input
                type="text"
                placeholder="Branch"
                value={branch}
                onChange={(e) => setBranch(e.target.value)}
                className="ui-input w-full font-mono sm:w-32"
              />
              <button
                onClick={triggerDeploy}
                disabled={!repoUrl}
                className="ui-button ui-button-primary"
              >
                Deploy
              </button>
            </div>
          </div>

          <div className="ui-card">
            <h3 className="mb-4 text-lg font-semibold text-slate-100">Deployments</h3>
            {loading ? (
              <CardSkeleton />
            ) : deployments.length === 0 ? (
              <EmptyState icon={<GitBranchIcon />} title="No deployments yet" description="Trigger a deployment above or push to a connected repository to see deployments here." />
            ) : (
              <div className="space-y-2">
                {deployments.map((d) => (
                    <div key={d.id} className="flex items-center justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 text-sm">
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium text-slate-100">{d.gitSourceId || "N/A"}</p>
                      <p className="font-mono text-xs text-slate-400">
                        {d.branch} @ <span className="font-mono">{d.commitSha.slice(0, 8)}</span> — {new Date(d.createdAt).toLocaleString()}
                      </p>
                    </div>
                    <StatusPill tone={statusTone[d.status] ?? "neutral"}>
                      {d.status}
                    </StatusPill>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="ui-card">
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-lg font-semibold text-slate-100">Webhook hooks</h3>
              <button
                onClick={createHook}
                className="ui-button ui-button-primary"
              >
                Create hook
              </button>
            </div>
            {hookError && (
              <div className="mb-4 ui-alert ui-alert-error" role="alert">
                <p className="text-sm">{hookError}</p>
              </div>
            )}
            {hooks.length === 0 ? (
              <EmptyState icon={<GitBranchIcon />} title="No hooks configured" description="Create a hook to let your git provider notify this server of pushes automatically." />
            ) : (
              <div className="space-y-2">
                {hooks.map((h) => (
                  <div key={h.id} className="flex items-center justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 text-sm">
                    <div className="min-w-0">
                      <p className="font-medium text-slate-100">Hook ID: <span className="font-mono">{h.id.slice(0, 8)}…</span></p>
                      <p className="font-mono text-xs text-slate-400">
                        Events: {h.events.join(", ")} — Created {new Date(h.createdAt).toLocaleString()}
                      </p>
                    </div>
                    <button
                      onClick={() => setHookToDelete(h)}
                      className="ui-button ui-button-danger"
                    >
                      Delete hook
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <ConfirmDialog
            confirmAction={() => { if (hookToDelete) void deleteHook(hookToDelete.id); }}
            confirmLabel={hookToDelete ? `Delete hook ${hookToDelete.id.slice(0, 8)}` : "Delete hook"}
            description="This removes the webhook URL and its secret. The git provider will no longer be able to trigger deployments until a new hook is created."
            destructive
            loading={deletingHook}
            closeAction={() => { if (!deletingHook) setHookToDelete(null); }}
            open={Boolean(hookToDelete)}
            title={hookToDelete ? `Delete hook ${hookToDelete.id.slice(0, 8)}?` : ""}
          />
        </div>
      )}
    </ServerConsoleLayout>
  );
}

function GitBranchIcon() {
  return (
    <svg aria-hidden="true" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.5" viewBox="0 0 24 24">
      <path d="M6 3v12m0 0a3 3 0 1 0 0 6 3 3 0 0 0 0-6Zm0 0h12m0 0a3 3 0 1 0 0 6 3 3 0 0 0 0-6Zm0 0V9a4 4 0 0 0-4-4H9" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
