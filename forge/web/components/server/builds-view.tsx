"use client";

import { useState } from "react";
import { Code2, Eye, EyeOff, Play } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiServer } from "@/lib/api/types";
import {
  type ApiAppBuild,
  type ApiServerBuildpack,
  fetchServerBuilds,
  fetchBuild,
  fetchBuildpacks,
  fetchServerBuildpacks,
  assignBuildpackToServer,
  removeBuildpackFromServer,
} from "@/lib/api/builds";
import { formatDate } from "@/lib/utils";
import { EmptyState, StatusPill } from "@/components/ui/primitives";
import { CardSkeleton } from "@/components/ui/loading-skeleton";

const statusTone: Record<string, "neutral" | "success" | "warning" | "danger" | "info"> = {
  pending: "warning",
  running: "info",
  succeeded: "success",
  failed: "danger",
  canceled: "neutral",
};

function errorText(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function BuildsView({ server }: { server?: ApiServer }) {
  const queryClient = useQueryClient();
  const [selectedBuildId, setSelectedBuildId] = useState<string | null>(null);
  const [selectedBuildpackId, setSelectedBuildpackId] = useState<string>("");
  const [showBuildpackPanel, setShowBuildpackPanel] = useState(false);
  const [newBpInput, setNewBpInput] = useState("");

  const serverId = server?.id ?? "";

  const builds = useQuery({
    queryKey: ["server-builds", serverId],
    queryFn: () => fetchServerBuilds(serverId),
    enabled: Boolean(serverId),
    refetchInterval: (query) =>
      (query.state.data as ApiAppBuild[] | undefined)?.some((b) => b.status === "pending" || b.status === "running")
        ? 3000
        : false,
  });

  const buildpacks = useQuery({
    queryKey: ["buildpacks"],
    queryFn: fetchBuildpacks,
    staleTime: 60000,
  });

  const serverBps = useQuery({
    queryKey: ["server-buildpacks", serverId],
    queryFn: () => fetchServerBuildpacks(serverId),
    enabled: Boolean(serverId),
  });

  const selectedBuild = useQuery({
    queryKey: ["server-build", serverId, selectedBuildId],
    queryFn: () => fetchBuild(serverId, selectedBuildId!),
    enabled: Boolean(serverId && selectedBuildId),
  });

  // Buildpack builds are not wired to a real build executor yet; the backend
  // fails them honestly, so the trigger is disabled here rather than offering
  // an action guaranteed to fail.
  const assignMutation = useMutation({
    mutationFn: (buildpackId: string) => assignBuildpackToServer(serverId, buildpackId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["server-buildpacks", serverId] });
    },
  });

  const removeMutation = useMutation({
    mutationFn: (buildpackId: string) => removeBuildpackFromServer(serverId, buildpackId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["server-buildpacks", serverId] });
    },
  });

  const actionError = assignMutation.error ?? removeMutation.error;
  const buildList = builds.data ?? [];
  const bpList = buildpacks.data ?? [];
  const bpAssignments = serverBps.data ?? [];
  const buildDetail = selectedBuild.data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-bold text-white">Builds</h2>
        <div className="flex items-center gap-2">
          <select
            className="ui-input min-h-9 w-52 px-3 py-1.5"
            value={selectedBuildpackId}
            onChange={(e) => setSelectedBuildpackId(e.target.value)}
          >
            <option value="">Auto-detect</option>
            {bpList.map((bp) => (
              <option key={bp.id} value={bp.id}>
                {bp.name} ({bp.builderType})
              </option>
            ))}
          </select>
          <button
            className="ui-button ui-button-primary"
            disabled
            title="Buildpack builds are not implemented yet. Deploy this application from a git source or a compose stack instead."
            type="button"
          >
            <Play size={14} />
            Build
          </button>
          <button
            className="ui-button ui-button-secondary"
            onClick={() => setShowBuildpackPanel((v) => !v)}
            type="button"
          >
            {showBuildpackPanel ? <EyeOff size={14} /> : <Eye size={14} />}
            Buildpacks
          </button>
        </div>
      </div>

      {actionError ? (
        <div className="ui-alert ui-alert-error" role="alert">
          <p className="text-sm">{errorText(actionError, "Build action failed.")}</p>
        </div>
      ) : null}

      {showBuildpackPanel ? (
        <div className="ui-card">
          <h3 className="mb-3 text-sm font-semibold text-slate-300">Assigned Buildpacks</h3>
          {bpAssignments.length === 0 ? (
            <p className="text-sm text-slate-500">No buildpacks assigned.</p>
          ) : (
            <div className="mb-3 space-y-2">
              {bpAssignments.map((sb: ApiServerBuildpack) => (
                <div key={sb.id} className="flex items-center justify-between gap-3 rounded-lg bg-white/[0.03] px-3 py-2">
                  <div className="min-w-0">
                    <span className="text-sm text-slate-200">{sb.buildpack?.name ?? sb.buildpackId}</span>
                    <span className="ml-2 text-xs text-slate-500">priority {sb.priority}</span>
                  </div>
                  <button
                    className="ui-button ui-button-danger"
                    disabled={removeMutation.isPending}
                    onClick={() => removeMutation.mutate(sb.buildpackId)}
                    type="button"
                  >
                    Remove {sb.buildpack?.name ?? sb.buildpackId}
                  </button>
                </div>
              ))}
            </div>
          )}
          <div className="flex gap-2">
            <select
              className="ui-input min-h-9 flex-1 px-3 py-1.5"
              value={newBpInput}
              onChange={(e) => setNewBpInput(e.target.value)}
            >
              <option value="">Select buildpack…</option>
              {bpList
                .filter((bp) => !bpAssignments.some((sb) => sb.buildpackId === bp.id))
                .map((bp) => (
                  <option key={bp.id} value={bp.id}>
                    {bp.name} ({bp.builderType})
                  </option>
                ))}
            </select>
            <button
              className="ui-button ui-button-primary"
              disabled={!newBpInput || assignMutation.isPending}
              onClick={() => {
                assignMutation.mutate(newBpInput);
                setNewBpInput("");
              }}
              type="button"
            >
              Assign
            </button>
          </div>
        </div>
      ) : null}

      {builds.isLoading ? (
        <CardSkeleton />
      ) : builds.isError ? (
        <div className="ui-alert ui-alert-error" role="alert">
          <p className="text-sm">{errorText(builds.error, "Failed to load builds. Check the API connection and your permissions, then retry.")}</p>
        </div>
      ) : buildList.length === 0 ? (
        <EmptyState icon={<Code2 size={20} />} title="No builds yet" description="No builds have been triggered yet. Deploy this application from a git source or a compose stack to see builds here." />
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <div className="space-y-2 lg:col-span-1">
            {buildList.map((build) => (
              <button
                key={build.id}
                className={`w-full rounded-lg border px-4 py-3 text-left transition ${
                  selectedBuildId === build.id
                    ? "border-red-500/40 bg-red-500/10"
                    : "border-white/[0.06] bg-white/[0.02] hover:bg-white/[0.04]"
                }`}
                onClick={() => setSelectedBuildId(build.id)}
                type="button"
              >
                <div className="flex items-center justify-between">
                  <span className="font-mono text-xs text-slate-400">{formatDate(build.createdAt)}</span>
                  <StatusPill tone={statusTone[build.status] ?? "neutral"}>{build.status}</StatusPill>
                </div>
                <div className="mt-1 truncate text-xs text-slate-500">
                  {build.buildpack?.name ?? "auto-detect"}
                </div>
              </button>
            ))}
          </div>

          <div className="lg:col-span-2">
            {buildDetail ? (
              <div className="ui-card">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium text-slate-200">
                    Build <span className="font-mono">{buildDetail.id.slice(0, 8)}</span>
                  </span>
                  <StatusPill tone={statusTone[buildDetail.status] ?? "neutral"}>{buildDetail.status}</StatusPill>
                </div>
                <p className="mt-1 font-mono text-xs text-slate-500">
                  {formatDate(buildDetail.createdAt)}
                  {buildDetail.imageTag ? ` · ${buildDetail.imageTag}` : ""}
                </p>
                <pre className="mt-3 overflow-auto rounded-lg bg-surface-input p-4 font-mono text-xs leading-relaxed text-slate-300">
                  {buildDetail.buildLog || "No build log available."}
                </pre>
              </div>
            ) : (
              <div className="flex items-center justify-center rounded-xl border border-dashed border-white/10 bg-black/10 p-12">
                <div className="text-center">
                  <Code2 className="mx-auto mb-2 h-8 w-8 text-slate-500" />
                  <p className="text-sm text-slate-400">Select a build to view details</p>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
