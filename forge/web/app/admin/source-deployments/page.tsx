"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { listSourceDeployments, createSourceDeployment, deploySourceDeployment, cancelSourceDeployment, deleteSourceDeployment, listGitProviders, type SourceDeployment, type GitProvider } from "@/lib/api/source-deployments";
import { Plus, Play, XCircle, Trash2, GitBranch, CheckCircle, Loader2, Clock, AlertTriangle } from "lucide-react";
import Link from "next/link";
import { AdminFormSection, AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader } from "@/components/admin/admin-ui";

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
  queued: "text-slate-300",
  cloning: "text-slate-300",
  building: "text-slate-300",
  pushing: "text-slate-300",
  deploying: "text-slate-300",
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

  const safeDeployments = useMemo(() => Array.isArray(deployments) ? deployments : [], [deployments]);
  const safeProviders = useMemo(() => Array.isArray(providers) ? providers : [], [providers]);

  const createMutation = useMutation({
    mutationFn: () => createSourceDeployment({
      serverId: form.serverId || undefined,
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
    <AdminPageLayout>
      <AdminPageHeader
        title="Source Deployments"
        description="Deploy applications from git repositories"
        action={<Btn onClick={() => setShowCreate(!showCreate)}><Plus className="w-4 h-4" /> New Deployment</Btn>}
      />

      {showCreate && (
        <Card>
          <CardHeader title="Create Source Deployment" icon={Plus} />
          <div className="p-4">
            <AdminFormSection title="Repository">
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label className="block text-sm font-medium mb-1">Repository URL</label>
                  <input
                    className="block min-h-11 w-full rounded-lg border border-white/10 bg-[#0d131d] px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
                    placeholder="https://github.com/user/repo.git"
                    value={form.repository}
                    onChange={(e) => setForm({ ...form, repository: e.target.value })}
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium mb-1">Branch</label>
                  <input
                    className="block min-h-11 w-full rounded-lg border border-white/10 bg-[#0d131d] px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
                    placeholder="main"
                    value={form.branch}
                    onChange={(e) => setForm({ ...form, branch: e.target.value })}
                  />
                </div>
              </div>
            </AdminFormSection>
            <AdminFormSection title="Build">
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <label className="block text-sm font-medium mb-1">Build Type</label>
                  <select
                    className="block min-h-11 w-full rounded-lg border border-white/10 bg-[#0d131d] px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
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
                    className="block min-h-11 w-full rounded-lg border border-white/10 bg-[#0d131d] px-3.5 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition placeholder:text-slate-600 hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
                    value={form.gitProviderId}
                    onChange={(e) => setForm({ ...form, gitProviderId: e.target.value })}
                  >
                    <option value="">None</option>
                    {safeProviders.map((p: GitProvider) => (
                      <option key={p.id} value={p.id}>{p.username || p.name} ({p.type})</option>
                    ))}
                  </select>
                </div>
              </div>
            </AdminFormSection>
            <AdminFormSection title="Target">
              <div>
                <label className="mb-1 block text-sm font-medium">Target server ID</label>
                <input className="block min-h-11 w-full rounded-lg border border-white/10 bg-[#0d131d] px-3.5 text-sm text-slate-100 outline-none focus:border-red-400/70" placeholder="Server UUID required for deployment" value={form.serverId} onChange={(e) => setForm({ ...form, serverId: e.target.value })} required />
              </div>
            </AdminFormSection>
            <AdminFormSection title="Options">
              <div>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={form.autoDeploy}
                    onChange={(e) => setForm({ ...form, autoDeploy: e.target.checked })}
                    className="rounded border-white/10 bg-[#161b28]"
                  />
                  <span className="text-sm">Auto-deploy on push</span>
                </label>
              </div>
            </AdminFormSection>
            <div className="flex gap-2 justify-end pt-4">
              <Btn tone="ghost" onClick={() => setShowCreate(false)}>Cancel</Btn>
              <Btn
                tone="primary"
                onClick={() => createMutation.mutate()}
                disabled={!form.repository.trim() || !form.serverId.trim() || createMutation.isPending}
              >
                {createMutation.isPending ? "Creating..." : "Create"}
              </Btn>
            </div>
          </div>
        </Card>
      )}

      {isLoading ? (
        <div className="text-center py-8 opacity-50">Loading deployments...</div>
      ) : safeDeployments.length > 0 ? (
        <div className="grid gap-3">
          {safeDeployments.map((d: SourceDeployment) => (
            <Card key={d.id} className="p-4">
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
                    <Btn size="sm" tone="ghost" onClick={() => cancelMutation.mutate(d.id)} ariaLabel="Cancel">
                      <XCircle className="w-4 h-4" />
                    </Btn>
                  )}
                  <Btn size="sm" tone="ghost" onClick={() => deployMutation.mutate(d.id)} ariaLabel="Deploy">
                    <Play className="w-4 h-4" />
                  </Btn>
                  <Btn size="sm" tone="danger" onClick={() => deleteMutation.mutate(d.id)} ariaLabel="Delete">
                    <Trash2 className="w-4 h-4" />
                  </Btn>
                </div>
              </div>
            </Card>
          ))}
        </div>
      ) : (
        <Card className="p-8">
          <div className="text-center">
            <GitBranch className="w-12 h-12 mx-auto mb-3 opacity-30" />
            <p className="opacity-60">No source deployments yet.</p>
            <p className="text-sm opacity-40">Create a deployment to build and deploy from a git repository.</p>
          </div>
        </Card>
      )}
    </AdminPageLayout>
  );
}
