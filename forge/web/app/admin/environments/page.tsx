"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Globe, Plus, Eye, EyeOff, History } from "lucide-react";
import { AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, EmptyState, Pill } from "@/components/admin/admin-ui";
import { Dialog } from "@/components/ui/primitives";
import { fetchOrganizations, fetchProjects, fetchEnvironments, createEnvironment, deleteEnvVar, fetchEnvVarRevisions, type EnvVarRevision } from "@/lib/api/tenancy";
import { fetchEnvVars, createEnvVar, type EnvVarResponse } from "@/lib/api/env-vars";
import { useToast } from "@/components/ui/toast";
import { environmentColorChoices } from "@/lib/design-tokens";

export default function AdminEnvironmentsPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [selectedOrg, setSelectedOrg] = useState("");
  const [selectedProject, setSelectedProject] = useState("");
  const [envName, setEnvName] = useState("");
  const [envColor, setEnvColor] = useState<string>(environmentColorChoices[0]);
  const [envProtected, setEnvProtected] = useState(false);

  const [selectedEnv, setSelectedEnv] = useState("");
  const [varKey, setVarKey] = useState("");
  const [varValue, setVarValue] = useState("");
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [historyVar, setHistoryVar] = useState<EnvVarResponse | null>(null);
  const [revisions, setRevisions] = useState<EnvVarRevision[]>([]);
  const [revisionsLoading, setRevisionsLoading] = useState(false);

  // Persisted data values — centralised in lib/design-tokens (environmentColorChoices)
  const colors = environmentColorChoices;

  const orgsQuery = useQuery({
    queryKey: ["organizations"],
    queryFn: fetchOrganizations,
  });

  const projectsQuery = useQuery({
    queryKey: ["projects", selectedOrg],
    queryFn: () => fetchProjects(selectedOrg),
    enabled: Boolean(selectedOrg),
  });

  const environmentsQuery = useQuery({
    queryKey: ["environments", selectedProject],
    queryFn: () => fetchEnvironments(selectedProject),
    enabled: Boolean(selectedProject),
  });

  const envVarsQuery = useQuery({
    queryKey: ["env-vars", selectedEnv],
    queryFn: () => fetchEnvVars(selectedEnv),
    enabled: Boolean(selectedEnv),
  });

  const createEnvMutation = useMutation({
    mutationFn: () => createEnvironment(selectedProject, envName.trim(), envColor, envProtected),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["environments", selectedProject] });
      setEnvName("");
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create environment", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const addVarMutation = useMutation({
    mutationFn: () => createEnvVar(selectedEnv, varKey.trim(), varValue, false),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["env-vars", selectedEnv] });
      setVarKey("");
      setVarValue("");
    },
    onError: (err) => toast({ tone: "error", title: "Failed to add variable", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteVarMutation = useMutation({
    mutationFn: (varId: string) => deleteEnvVar(varId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["env-vars", selectedEnv] });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to delete variable", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const handleCreateEnv = (e: React.FormEvent) => {
    e.preventDefault();
    if (!envName.trim() || !selectedProject) return;
    createEnvMutation.mutate();
  };

  const handleAddVar = (e: React.FormEvent) => {
    e.preventDefault();
    if (!varKey.trim() || !selectedEnv) return;
    addVarMutation.mutate();
  };

  const handleDeleteVar = (varId: string) => {
    deleteVarMutation.mutate(varId);
  };

  const openRevisions = async (v: EnvVarResponse) => {
    setHistoryVar(v);
    setRevisionsLoading(true);
    try {
      const data = await fetchEnvVarRevisions(v.id);
      setRevisions(data ?? []);
    } catch {
      setRevisions([]);
    } finally {
      setRevisionsLoading(false);
    }
  };

  const orgs = useMemo(() => orgsQuery.data ?? [], [orgsQuery.data]);
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);
  const environments = useMemo(() => environmentsQuery.data ?? [], [environmentsQuery.data]);
  const envVars = useMemo(() => envVarsQuery.data ?? [], [envVarsQuery.data]);

  return (
    <AdminPageLayout>
      <AdminPageHeader title="Environments" description="Manage deployment environments and environment variables" />

      <div className="mb-4 flex flex-col gap-3 sm:flex-row">
        <select value={selectedOrg} onChange={(e) => { setSelectedOrg(e.target.value); setSelectedProject(""); setSelectedEnv(""); }} className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white">
          <option value="">Select organization...</option>
          {Array.isArray(orgs) && orgs.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}
        </select>
        <select value={selectedProject} onChange={(e) => { setSelectedProject(e.target.value); setSelectedEnv(""); }} className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white">
          <option value="">Select project...</option>
          {Array.isArray(projects) && projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </div>

      {selectedProject && (
        <form onSubmit={handleCreateEnv} className="mb-4 flex flex-wrap gap-2 items-end">
          <div>
            <label className="block text-xs text-slate-500 mb-1">Name</label>
            <input value={envName} onChange={(e) => setEnvName(e.target.value)} placeholder="e.g. production" className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-red-400/70 focus:outline-none" required />
          </div>
          <div>
            <label className="block text-xs text-slate-500 mb-1">Color</label>
            <div className="flex gap-1">
              {colors.map((c) => (
                <button key={c} type="button" onClick={() => setEnvColor(c)} className={`w-6 h-6 rounded-full border-2 ${envColor === c ? "border-white" : "border-transparent"}`} style={{ backgroundColor: c }} />
              ))}
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input type="checkbox" checked={envProtected} onChange={(e) => setEnvProtected(e.target.checked)} className="rounded" />
            Protected
          </label>
          <Btn type="submit" loading={createEnvMutation.isPending}><Plus size={14} /> Create</Btn>
        </form>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Environments" icon={Globe} />
          {!selectedProject ? <EmptyState message="Select a project" /> :
           environmentsQuery.isLoading ? <div className="p-6 text-sm text-slate-400">Loading...</div> :
           environmentsQuery.isError ? <div className="p-4 text-sm text-red-300">Could not load environments: {environmentsQuery.error instanceof Error ? environmentsQuery.error.message : "unknown error"}</div> :
           !Array.isArray(environments) || environments.length === 0 ? <EmptyState message="No environments" /> :
           <div className="divide-y divide-white/[0.06]">
            {Array.isArray(environments) && environments.map((env) => (
              <div
                key={env.id}
                className={`flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-white/[0.02] ${selectedEnv === env.id ? "bg-red-500/10" : ""}`}
                onClick={() => setSelectedEnv(env.id)}
              >
                <div className="flex items-center gap-2">
                  <span className="w-3 h-3 rounded-full" style={{ backgroundColor: env.color }} />
                  <span className="text-sm font-medium text-slate-200">{env.name}</span>
                  {env.protected && <Pill tone="yellow">Protected</Pill>}
                </div>
              </div>
            ))}
          </div>}
        </Card>

        {selectedEnv && (
          <Card>
            <CardHeader title="Environment Variables" icon={Globe} action={
              <span className="text-xs text-slate-500">{(Array.isArray(envVars) ? envVars : []).length} variables</span>
            } />
            <form onSubmit={handleAddVar} className="flex gap-2 p-3 border-b border-white/[0.06]">
              <input value={varKey} onChange={(e) => setVarKey(e.target.value)} placeholder="KEY" className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm font-mono text-white placeholder:text-gray-500 focus:border-[var(--brand)]/70 focus:outline-none" required />
              <input value={varValue} onChange={(e) => setVarValue(e.target.value)} placeholder="value" className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-[var(--brand)]/70 focus:outline-none" />
              <Btn type="submit" loading={addVarMutation.isPending}><Plus size={14} /></Btn>
            </form>
            {envVarsQuery.isLoading ? <div className="p-6 text-sm text-slate-400">Loading...</div> :
             envVarsQuery.isError ? <div className="p-4 text-sm text-red-300">Could not load variables: {envVarsQuery.error instanceof Error ? envVarsQuery.error.message : "unknown error"}</div> :
             !Array.isArray(envVars) || envVars.length === 0 ? <EmptyState message="No environment variables" /> :
             <div className="divide-y divide-white/[0.06]">
              {Array.isArray(envVars) && envVars.map((v: EnvVarResponse) => (
                <div key={v.id} className="flex items-center justify-between px-4 py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-mono text-slate-200">{v.key}</span>
                    <span className="text-xs text-slate-500">v{v.version}</span>
                    {v.isSensitive && <Pill tone="yellow">Sensitive</Pill>}
                  </div>
                  <div className="flex items-center gap-2">
                    <button onClick={() => setRevealed({ ...revealed, [v.id]: !revealed[v.id] })} className="text-slate-500 hover:text-slate-300" title={revealed[v.id] ? "Hide" : "Show"} type="button">
                      {revealed[v.id] ? <EyeOff size={14} /> : <Eye size={14} />}
                    </button>
                    <button onClick={() => openRevisions(v)} className="text-slate-500 hover:text-[var(--brand)]" title="Revision history" type="button">
                      <History size={14} />
                    </button>
                    <button onClick={() => handleDeleteVar(v.id)} className="text-red-500 hover:text-red-400 text-xs" type="button">Delete</button>
                  </div>
                </div>
              ))}
            </div>}
            {historyVar && (
              <Dialog open={Boolean(historyVar)} title={`Revision History — ${historyVar.key}`} description={`Version history for ${historyVar.key} (current v${historyVar.version})`} closeAction={() => { setHistoryVar(null); setRevisions([]); }} className="max-w-lg">
                {revisionsLoading ? (
                  <div className="p-6 text-sm text-slate-400">Loading revisions...</div>
                ) : revisions.length === 0 ? (
                  <p className="p-4 text-sm text-slate-500">No revision history available.</p>
                ) : (
                  <div className="divide-y divide-white/[0.06] rounded-lg border border-white/10">
                    {revisions.map((r) => (
                      <div key={r.id} className="flex items-center justify-between px-4 py-2.5">
                        <div className="flex items-center gap-2">
                          <span className="rounded bg-[var(--brand)]/20 px-2 py-0.5 text-xs font-semibold text-[var(--brand)]">v{r.version}</span>
                          {r.createdBy && <span className="text-xs text-slate-400">by {r.createdBy}</span>}
                        </div>
                        <span className="text-xs text-slate-500">{new Date(r.createdAt).toLocaleString()}</span>
                      </div>
                    ))}
                  </div>
                )}
              </Dialog>
            )}
          </Card>
        )}
      </div>
    </AdminPageLayout>
  );
}
