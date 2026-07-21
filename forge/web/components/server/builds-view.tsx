"use client";

import { useState } from "react";
import { AlertCircle, Code2, Eye, EyeOff, Play, RefreshCw } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiServer } from "@/lib/api/types";
import {
  type ApiAppBuild,
  type ApiBuildpack,
  type ApiServerBuildpack,
  triggerBuild,
  fetchServerBuilds,
  fetchBuild,
  fetchBuildpacks,
  fetchServerBuildpacks,
  assignBuildpackToServer,
  removeBuildpackFromServer,
} from "@/lib/api/builds";
import { formatDate } from "@/lib/utils";

const statusColors: Record<string, string> = {
  pending: "text-amber-400",
  running: "text-blue-400",
  succeeded: "text-emerald-400",
  failed: "text-red-400",
  canceled: "text-slate-400",
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

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["server-builds", serverId] });
  };

  const triggerMutation = useMutation({
    mutationFn: () => triggerBuild(serverId, selectedBuildpackId || undefined),
    onSuccess: invalidate,
  });

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

  const actionError = triggerMutation.error ?? assignMutation.error ?? removeMutation.error;
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
            className="rounded-lg border border-white/[0.08] bg-[#1e2536] px-3 py-1.5 text-sm text-slate-300"
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
            className="inline-flex items-center gap-1.5 rounded-lg bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-500 disabled:opacity-50"
            disabled={triggerMutation.isPending}
            onClick={() => triggerMutation.mutate()}
            type="button"
          >
            {triggerMutation.isPending ? (
              <RefreshCw size={14} className="animate-spin" />
            ) : (
              <Play size={14} />
            )}
            Build
          </button>
          <button
            className="inline-flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-[#1e2536] px-3 py-1.5 text-sm text-slate-300 hover:bg-white/[0.05]"
            onClick={() => setShowBuildpackPanel((v) => !v)}
            type="button"
          >
            {showBuildpackPanel ? <EyeOff size={14} /> : <Eye size={14} />}
            Buildpacks
          </button>
        </div>
      </div>

      {actionError ? (
        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200" role="alert">
          {errorText(actionError, "Build action failed.")}
        </div>
      ) : null}

      {showBuildpackPanel ? (
        <div className="rounded-xl border border-white/[0.08] bg-[#1e2536] p-4">
          <h3 className="mb-3 text-sm font-semibold text-slate-300">Assigned Buildpacks</h3>
          {bpAssignments.length === 0 ? (
            <p className="text-sm text-slate-500">No buildpacks assigned.</p>
          ) : (
            <div className="mb-3 space-y-2">
              {bpAssignments.map((sb: ApiServerBuildpack) => (
                <div key={sb.id} className="flex items-center justify-between rounded-lg bg-white/[0.04] px-3 py-2">
                  <div>
                    <span className="text-sm text-slate-200">{sb.buildpack?.name ?? sb.buildpackId}</span>
                    <span className="ml-2 text-xs text-slate-500">priority {sb.priority}</span>
                  </div>
                  <button
                    className="text-xs text-red-400 hover:text-red-300"
                    disabled={removeMutation.isPending}
                    onClick={() => removeMutation.mutate(sb.buildpackId)}
                    type="button"
                  >
                    Remove
                  </button>
                </div>
              ))}
            </div>
          )}
          <div className="flex gap-2">
            <select
              className="flex-1 rounded-lg border border-white/[0.08] bg-[#0f1419] px-3 py-1.5 text-sm text-slate-300"
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
              className="rounded-lg bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-500 disabled:opacity-50"
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
        <div className="rounded-xl bg-[#1e2536] px-4 py-5 text-sm text-slate-400">Loading builds…</div>
      ) : builds.isError ? (
        <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200">
          {errorText(builds.error, "Failed to load builds.")}
        </div>
      ) : buildList.length === 0 ? (
        <div className="rounded-xl bg-[#1e2536] px-4 py-5 text-sm text-slate-400">
          No builds have been triggered yet.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <div className="lg:col-span-1 space-y-2">
            {buildList.map((build) => (
              <button
                key={build.id}
                className={`w-full rounded-lg border px-4 py-3 text-left transition ${
                  selectedBuildId === build.id
                    ? "border-red-500/40 bg-red-500/10"
                    : "border-white/[0.06] bg-[#1e2536] hover:bg-white/[0.04]"
                }`}
                onClick={() => setSelectedBuildId(build.id)}
                type="button"
              >
                <div className="flex items-center justify-between">
                  <span className="text-xs text-slate-400">{formatDate(build.createdAt)}</span>
                  <span className={`text-xs font-medium ${statusColors[build.status] ?? "text-slate-400"}`}>
                    {build.status}
                  </span>
                </div>
                <div className="mt-1 truncate text-xs text-slate-500">
                  {build.buildpack?.name ?? "auto-detect"}
                </div>
              </button>
            ))}
          </div>

          <div className="lg:col-span-2">
            {buildDetail ? (
              <div className="rounded-xl border border-white/[0.08] bg-[#1e2536]">
                <div className="border-b border-white/[0.06] px-4 py-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium text-slate-200">
                      Build {buildDetail.id.slice(0, 8)}
                    </span>
                    <span
                      className={`text-sm font-medium ${statusColors[buildDetail.status] ?? "text-slate-400"}`}
                    >
                      {buildDetail.status}
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-slate-500">
                    {formatDate(buildDetail.createdAt)}
                    {buildDetail.imageTag ? ` · ${buildDetail.imageTag}` : ""}
                  </p>
                </div>
                <div className="p-4">
                  <pre className="overflow-auto rounded-lg bg-[#0f1419] p-4 font-mono text-xs leading-relaxed text-slate-300">
                    {buildDetail.buildLog || "No build log available."}
                  </pre>
                </div>
              </div>
            ) : (
              <div className="flex items-center justify-center rounded-xl border border-white/[0.06] bg-[#1e2536] p-12">
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
