"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import {
  listGitDeployments,
  listGitDeploymentHooks,
  triggerGitDeployment,
  createGitDeploymentHook,
  deleteGitDeploymentHook,
} from "@/lib/api/git-deployments";
import { deploymentStatusTone } from "@/lib/api/status";
import type { GitDeploymentHook } from "@/lib/api/git-deployments";
import { errorMessage } from "@/lib/utils";
import { ConfirmDialog, EmptyState, StatusPill } from "@/components/ui/primitives";
import { CardSkeleton } from "@/components/ui/loading-skeleton";
import { useOptionalServerContext } from "@/components/server/server-context";

const DEPLOYMENTS_KEY = ["git-deployments"] as const;
const HOOKS_KEY = ["git-deployment-hooks"] as const;


export default function GitDeployPage() {
  const params = useParams();
  const serverId = String(params.id ?? "");
  const serverContext = useOptionalServerContext();
  const canManageHooks = Boolean(serverContext?.access?.isOwner || serverContext?.access?.isAdmin);
  const queryClient = useQueryClient();

  const [repoUrl, setRepoUrl] = useState("");
  const [branch, setBranch] = useState("main");
  const [hookToDelete, setHookToDelete] = useState<GitDeploymentHook | null>(null);

  // Server state lives in react-query; every write is a mutation that refreshes
  // the cache only once the panel has accepted it.
  const deploymentsQuery = useQuery({
    queryKey: [...DEPLOYMENTS_KEY, serverId],
    queryFn: () => listGitDeployments(serverId),
    enabled: Boolean(serverId),
  });
  const hooksQuery = useQuery({
    queryKey: [...HOOKS_KEY, serverId],
    queryFn: () => listGitDeploymentHooks(serverId),
    enabled: Boolean(serverId),
  });

  const deployments = deploymentsQuery.data ?? [];
  const hooks = hooksQuery.data ?? [];
  const loading = deploymentsQuery.isPending || hooksQuery.isPending;

  const deployMut = useMutation({
    mutationFn: (input: { repoUrl: string; branch: string }) => triggerGitDeployment(serverId, input),
    onSuccess: () => {
      setRepoUrl("");
      setBranch("main");
      void queryClient.invalidateQueries({ queryKey: [...DEPLOYMENTS_KEY, serverId] });
      void queryClient.invalidateQueries({ queryKey: [...HOOKS_KEY, serverId] });
    },
  });

  const createHookMut = useMutation({
    mutationFn: () => createGitDeploymentHook(serverId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [...HOOKS_KEY, serverId] });
    },
  });

  const deleteHookMut = useMutation({
    mutationFn: (hookId: string) => deleteGitDeploymentHook(serverId, hookId),
    onSuccess: () => {
      setHookToDelete(null);
      void queryClient.invalidateQueries({ queryKey: [...HOOKS_KEY, serverId] });
    },
  });

  const fetchError = deploymentsQuery.isError
    ? errorMessage(deploymentsQuery.error, "Failed to load git data")
    : hooksQuery.isError
      ? errorMessage(hooksQuery.error, "Failed to load git data")
      : null;
  const deployError = deployMut.isError ? errorMessage(deployMut.error, "Deploy failed") : null;
  const hookError = createHookMut.isError
    ? errorMessage(createHookMut.error, "Failed to create hook")
    : deleteHookMut.isError
      ? errorMessage(deleteHookMut.error, "Failed to delete hook")
      : null;
  const deletingHook = deleteHookMut.isPending;

  function triggerDeploy() {
    if (!serverId || !repoUrl) return;
    deployMut.mutate({ repoUrl, branch });
  }

  function retryLoad() {
    void deploymentsQuery.refetch();
    void hooksQuery.refetch();
  }

  return (
    <ServerConsoleLayout activeTab="git">
      {() => (
        <div className="space-y-8">
          <h2 className="text-2xl font-bold text-white">Git Deployments</h2>

          {fetchError && (
            <div className="ui-alert ui-alert-error" role="alert">
              <p className="text-sm">{fetchError}</p>
              <button className="ui-button ui-button-secondary" onClick={retryLoad} type="button">Retry</button>
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
                disabled={!repoUrl || deployMut.isPending}
                className="ui-button ui-button-primary"
              >
                {deployMut.isPending ? "Deploying…" : "Deploy"}
              </button>
            </div>
          </div>

          <div className="ui-card">
            <h3 className="mb-4 text-lg font-semibold text-slate-100">Deployments</h3>
            {loading ? (
              <CardSkeleton />
            ) : deploymentsQuery.isError ? (
              <p className="text-sm text-red-300" role="alert">Deployments could not be loaded, so this list is unknown rather than empty.</p>
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
                    <StatusPill tone={deploymentStatusTone(d.status)}>
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
                onClick={() => createHookMut.mutate()}
                disabled={createHookMut.isPending}
                className="ui-button ui-button-primary"
              >
                {createHookMut.isPending ? "Creating hook…" : "Create hook"}
              </button>
            </div>
            {hookError && (
              <div className="mb-4 ui-alert ui-alert-error" role="alert">
                <p className="text-sm">{hookError}</p>
              </div>
            )}
            {!canManageHooks ? (
              <p className="mb-4 text-xs text-slate-400">
                Only the server owner or an administrator can delete hooks.
              </p>
            ) : null}
            {hooksQuery.isError ? (
              <p className="mb-4 text-sm text-red-300" role="alert">Webhooks could not be loaded, so this list is unknown rather than empty.</p>
            ) : hooks.length === 0 ? (
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
                      disabled={!canManageHooks || deletingHook}
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
            confirmAction={() => { if (hookToDelete) deleteHookMut.mutate(hookToDelete.id); }}
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
