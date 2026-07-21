"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { listSourceDeployments, createSourceDeployment, deploySourceDeployment, cancelSourceDeployment, deleteSourceDeployment, listGitProviders, listProviderRepos, type SourceDeployment, type GitProvider, type GitProviderRepo } from "@/lib/api/source-deployments";
import { Plus, Play, XCircle, Trash2, GitBranch, Github, RefreshCw, CheckCircle, Loader2, Clock, AlertTriangle } from "lucide-react";
import Link from "next/link";

const statusIcons: Record<string, typeof Clock> = {
  pending: Clock,
  queued: Loader2,
  cloning: Loader2,
  building: Loader2,
  pushing: Loader2,
  deploying: Loader2,
  healthy: CheckCircle,
  completed: CheckCircle,
  failed: XCircle,
  canceled: XCircle,
  unhealthy: AlertTriangle,
};

const statusColors: Record<string, string> = {
  pending: "text-yellow-400",
  queued: "text-blue-400",
  cloning: "text-blue-400",
  building: "text-purple-400",
  pushing: "text-purple-400",
  deploying: "text-indigo-400",
  healthy: "text-green-400",
  completed: "text-green-400",
  failed: "text-red-400",
  canceled: "text-gray-400",
  unhealthy: "text-orange-400",
};

export default function SourceDeploymentsPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({
    repository: "",
    branch: "main",
    buildType: "dockerfile",
    buildContext: ".",
    dockerfilePath: "Dockerfile",
    gitProviderId: "",
    serverId: "",
    autoDeploy: false,
    registry: "",
  });

  const { data: deployments, isLoading } = useQuery({
    queryKey: ["sourceDeployments"],
    queryFn: listSourceDeployments,
  });

  const { data: providers } = useQuery({
    queryKey: ["gitProviders"],
    queryFn: listGitProviders,
  });

  const createMutation = useMutation({
    mutationFn: () => createSourceDeployment({
      repository: form.repository,
      branch: form.branch,
      buildType: form.buildType,
      buildContext: form.buildContext,
      dockerfilePath: form.dockerfilePath,
      gitProviderId: form.gitProviderId || undefined,
      autoDeploy: form.autoDeploy,
      registry: form.registry || undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      setShowCreate(false);
      setForm({ repository: "", branch: "main", buildType: "dockerfile", buildContext: ".", dockerfilePath: "Dockerfile", gitProviderId: "", serverId: "", autoDeploy: false, registry: "" });
      toast({ title: "Deployment created", tone: "success" });
    },
    onError: () => {
      toast({ title: "Create failed", tone: "error" });
    },
  });

  const deployMutation = useMutation({
    mutationFn: (id: string) => deploySourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      toast({ title: "Deployment triggered", tone: "success" });
    },
    onError: () => {
      toast({ title: "Deploy failed", tone: "error" });
    },
  });

  const cancelMutation = useMutation({
    mutationFn: (id: string) => cancelSourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      toast({ title: "Deployment canceled", tone: "success" });
    },
    onError: () => {
      toast({ title: "Cancel failed", tone: "error" });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSourceDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sourceDeployments"] });
      toast({ title: "Deployment deleted", tone: "success" });
    },
    onError: () => {
      toast({ title: "Delete failed", tone: "error" });
    },
  });

  const StatusIcon = ({ status }: { status: string }) => {
    const Icon = statusIcons[status] || AlertTriangle;
    return <Icon className={`w-4 h-4 ${statusColors[status] || ''} ${status === 'queued' || status === 'building' || status === 'pushing' || status === 'deploying' ? 'animate-spin' : ''}`} />;
  };

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Source Deployments</h1>
          <p className="text-sm opacity-70">Deploy applications from git repositories</p>
        </div>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="btn btn-primary flex items-center gap-2"
        >
          <Plus className="w-4 h-4" /> New Deployment
        </button>
      </div>

      {showCreate && (
        <div className="card p-4 mb-6">
          <h2 className="text-lg font-semibold mb-4">Create Source Deployment</h2>
          <div className="grid gap-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium mb-1">Repository URL</label>
                <input
                  className="input w-full"
                  placeholder="https://github.com/user/repo.git"
                  value={form.repository}
                  onChange={(e) => setForm({ ...form, repository: e.target.value })}
                />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Branch</label>
                <input
                  className="input w-full"
                  placeholder="main"
                  value={form.branch}
                  onChange={(e) => setForm({ ...form, branch: e.target.value })}
                />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium mb-1">Build Type</label>
                <select
                  className="input w-full"
                  value={form.buildType}
                  onChange={(e) => setForm({ ...form, buildType: e.target.value })}
                >
                  <option value="dockerfile">Dockerfile</option>
                  <option value="nixpacks">Nixpacks</option>
                  <option value="heroku">Heroku Buildpacks</option>
                  <option value="paketo">Paketo</option>
                  <option value="static">Static</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">Git Provider (optional)</label>
                <select
                  className="input w-full"
                  value={form.gitProviderId}
                  onChange={(e) => setForm({ ...form, gitProviderId: e.target.value })}
                >
                  <option value="">None</option>
                  {providers?.map((p: GitProvider) => (
                    <option key={p.id} value={p.id}>{p.username || p.name} ({p.type})</option>
                  ))}
                </select>
              </div>
            </div>
            <div>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.autoDeploy}
                  onChange={(e) => setForm({ ...form, autoDeploy: e.target.checked })}
                  className="checkbox"
                />
                <span className="text-sm">Auto-deploy on push</span>
              </label>
            </div>
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setShowCreate(false)}
                className="btn btn-ghost"
              >
                Cancel
              </button>
              <button
                onClick={() => createMutation.mutate()}
                className="btn btn-primary"
                disabled={!form.repository.trim() || createMutation.isPending}
              >
                {createMutation.isPending ? "Creating..." : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="text-center py-8 opacity-50">Loading deployments...</div>
      ) : deployments && deployments.length > 0 ? (
        <div className="grid gap-3">
          {deployments.map((d: SourceDeployment) => (
            <div key={d.id} className="card p-4">
              <div className="flex items-center justify-between">
                <Link href={`/admin/source-deployments/${d.id}`} className="flex items-center gap-3 flex-1 hover:opacity-80">
                  <StatusIcon status={d.status} />
                  <div className="flex-1">
                    <div className="font-medium flex items-center gap-2">
                      <GitBranch className="w-3 h-3" />
                      {d.repository.split('/').pop()?.replace('.git', '')}
                      <span className="text-xs opacity-50">{d.branch}</span>
                    </div>
                    <div className="text-sm opacity-60 flex items-center gap-2">
                      <span className="capitalize">{d.buildType}</span>
                      {d.commitMessage && <span className="truncate max-w-xs">{d.commitMessage}</span>}
                    </div>
                  </div>
                  <span className={`text-xs px-2 py-1 rounded-full capitalize ${statusColors[d.status]} bg-current/10`}>
                    {d.status}
                  </span>
                </Link>
                <div className="flex items-center gap-1 ml-4">
                  {!(["completed", "failed", "canceled"].includes(d.status)) && (
                    <button
                      onClick={() => cancelMutation.mutate(d.id)}
                      className="btn btn-ghost btn-sm"
                      title="Cancel"
                    >
                      <XCircle className="w-4 h-4" />
                    </button>
                  )}
                  <button
                    onClick={() => deployMutation.mutate(d.id)}
                    className="btn btn-ghost btn-sm"
                    title="Deploy"
                  >
                    <Play className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => deleteMutation.mutate(d.id)}
                    className="btn btn-ghost btn-sm text-red-400"
                    title="Delete"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="text-center py-12 card">
          <GitBranch className="w-12 h-12 mx-auto mb-3 opacity-30" />
          <p className="opacity-60">No source deployments yet.</p>
          <p className="text-sm opacity-40">Create a deployment to build and deploy from a git repository.</p>
        </div>
      )}
    </div>
  );
}
