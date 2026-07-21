"use client";

import { useState, useEffect } from "react";
import { Globe, Plus, Eye, EyeOff } from "lucide-react";
import { Btn, Card, CardHeader, SectionHeader, EmptyState, Pill } from "@/components/admin/admin-ui";
import type { Organization, Project, Environment, EnvironmentVariable } from "@/lib/api/tenancy";

export default function AdminEnvironmentsPage() {
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [selectedOrg, setSelectedOrg] = useState("");
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedProject, setSelectedProject] = useState("");
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [loading, setLoading] = useState(true);
  const [envName, setEnvName] = useState("");
  const [envColor, setEnvColor] = useState("#6366f1");
  const [envProtected, setEnvProtected] = useState(false);

  const [envVars, setEnvVars] = useState<EnvironmentVariable[]>([]);
  const [selectedEnv, setSelectedEnv] = useState("");
  const [varKey, setVarKey] = useState("");
  const [varValue, setVarValue] = useState("");
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  const colors = ["#6366f1", "#22c55e", "#f59e0b", "#ef4444", "#8b5cf6", "#06b6d4", "#ec4899", "#64748b"];

  useEffect(() => {
    fetch("/api/v1/organizations", { credentials: "include" })
      .then((r) => r.ok ? r.json() : [])
      .then(setOrgs);
  }, []);

  const fetchProjects = async (orgId: string) => {
    if (!orgId) { setProjects([]); return; }
    const res = await fetch(`/api/v1/organizations/${orgId}/projects`, { credentials: "include" });
    if (res.ok) setProjects(await res.json());
  };

  useEffect(() => { fetchProjects(selectedOrg); setSelectedProject(""); setEnvironments([]); setSelectedEnv(""); }, [selectedOrg]);

  const fetchEnvironments = async (projectId: string) => {
    if (!projectId) { setEnvironments([]); return; }
    setLoading(true);
    try {
      const res = await fetch(`/api/v1/projects/${projectId}/envs`, { credentials: "include" });
      if (res.ok) setEnvironments(await res.json());
    } finally { setLoading(false); }
  };

  useEffect(() => { fetchEnvironments(selectedProject); setSelectedEnv(""); setEnvVars([]); }, [selectedProject]);

  const fetchEnvVars = async (envId: string) => {
    if (!envId) { setEnvVars([]); return; }
    const res = await fetch(`/api/v1/environments/${envId}/env-vars`, { credentials: "include" });
    if (res.ok) setEnvVars(await res.json());
  };

  useEffect(() => { fetchEnvVars(selectedEnv); }, [selectedEnv]);

  const handleCreateEnv = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!envName.trim() || !selectedProject) return;
    const res = await fetch(`/api/v1/projects/${selectedProject}/envs`, {
      method: "POST", credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: envName.trim(), color: envColor, protected: envProtected }),
    });
    if (res.ok) { setEnvName(""); await fetchEnvironments(selectedProject); }
  };

  const handleAddVar = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!varKey.trim() || !selectedEnv) return;
    const res = await fetch(`/api/v1/environments/${selectedEnv}/env-vars`, {
      method: "POST", credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key: varKey.trim(), value: varValue, isSensitive: false }),
    });
    if (res.ok) { setVarKey(""); setVarValue(""); await fetchEnvVars(selectedEnv); }
  };

  const handleDeleteVar = async (varId: string) => {
    await fetch(`/api/v1/env-vars/${varId}`, { method: "DELETE", credentials: "include" });
    await fetchEnvVars(selectedEnv);
  };

  return (
    <div>
      <SectionHeader title="Environments" sub="Manage deployment environments and environment variables" />

      <div className="mb-4 flex gap-3">
        <select value={selectedOrg} onChange={(e) => setSelectedOrg(e.target.value)} className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white">
          <option value="">Select organization...</option>
          {orgs.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}
        </select>
        <select value={selectedProject} onChange={(e) => setSelectedProject(e.target.value)} className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white">
          <option value="">Select project...</option>
          {projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </div>

      {selectedProject && (
        <form onSubmit={handleCreateEnv} className="mb-4 flex flex-wrap gap-2 items-end">
          <div>
            <label className="block text-xs text-slate-500 mb-1">Name</label>
            <input value={envName} onChange={(e) => setEnvName(e.target.value)} placeholder="e.g. production" className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" required />
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
          <Btn type="submit"><Plus size={14} /> Create</Btn>
        </form>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Environments" icon={Globe} />
          {!selectedProject ? <EmptyState message="Select a project" /> :
           loading ? <div className="p-6 text-sm text-slate-400">Loading...</div> :
           environments.length === 0 ? <EmptyState message="No environments" /> :
           <div className="divide-y divide-white/[0.06]">
            {environments.map((env) => (
              <div
                key={env.id}
                className={`flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-white/[0.02] ${selectedEnv === env.id ? "bg-purple-500/10" : ""}`}
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
              <span className="text-xs text-slate-500">{envVars.length} variables</span>
            } />
            <form onSubmit={handleAddVar} className="flex gap-2 p-3 border-b border-white/[0.06]">
              <input value={varKey} onChange={(e) => setVarKey(e.target.value)} placeholder="KEY" className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm font-mono text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" required />
              <input value={varValue} onChange={(e) => setVarValue(e.target.value)} placeholder="value" className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" />
              <Btn type="submit"><Plus size={14} /></Btn>
            </form>
            {envVars.length === 0 ? <EmptyState message="No environment variables" /> :
             <div className="divide-y divide-white/[0.06]">
              {envVars.map((v) => (
                <div key={v.id} className="flex items-center justify-between px-4 py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-mono text-slate-200">{v.key}</span>
                    <span className="text-xs text-slate-500">v{v.version}</span>
                    {v.isSensitive && <Pill tone="yellow">Sensitive</Pill>}
                  </div>
                  <div className="flex items-center gap-2">
                    <button onClick={() => setRevealed({ ...revealed, [v.id]: !revealed[v.id] })} className="text-slate-500 hover:text-slate-300">
                      {revealed[v.id] ? <EyeOff size={14} /> : <Eye size={14} />}
                    </button>
                    <button onClick={() => handleDeleteVar(v.id)} className="text-red-500 hover:text-red-400 text-xs">Delete</button>
                  </div>
                </div>
              ))}
            </div>}
          </Card>
        )}
      </div>
    </div>
  );
}
