"use client";

import { useState, useEffect } from "react";
import { FolderKanban, Plus } from "lucide-react";
import { Btn, Card, CardHeader, SectionHeader, EmptyState } from "@/components/admin/admin-ui";
import type { Organization, Project } from "@/lib/api/tenancy";

export default function AdminProjectsPage() {
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [selectedOrg, setSelectedOrg] = useState<string>("");
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");

  useEffect(() => {
    fetch("/api/v1/organizations", { credentials: "include" })
      .then((r) => r.ok ? r.json() : [])
      .then(setOrgs);
  }, []);

  const fetchProjects = async (orgId: string) => {
    if (!orgId) { setProjects([]); setLoading(false); return; }
    setLoading(true);
    try {
      const res = await fetch(`/api/v1/organizations/${orgId}/projects`, { credentials: "include" });
      if (res.ok) setProjects(await res.json());
    } finally { setLoading(false); }
  };

  useEffect(() => { fetchProjects(selectedOrg); }, [selectedOrg]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !selectedOrg) return;
    const res = await fetch(`/api/v1/organizations/${selectedOrg}/projects`, {
      method: "POST", credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: name.trim(), description: desc.trim() }),
    });
    if (res.ok) { setName(""); setDesc(""); await fetchProjects(selectedOrg); }
  };

  return (
    <div>
      <SectionHeader title="Projects" sub="Manage projects within organizations" />

      <div className="mb-4 flex gap-3">
        <select value={selectedOrg} onChange={(e) => setSelectedOrg(e.target.value)} className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white">
          <option value="">Select organization...</option>
          {orgs.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}
        </select>
      </div>

      {selectedOrg && (
        <form onSubmit={handleCreate} className="mb-4 flex gap-2">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Project name" className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" required />
          <input value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="Description" className="w-48 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" />
          <Btn type="submit"><Plus size={14} /> Create</Btn>
        </form>
      )}

      <Card>
        <CardHeader title="All Projects" icon={FolderKanban} />
        {!selectedOrg ? <EmptyState message="Select an organization to view projects" /> :
         loading ? <div className="p-6 text-sm text-slate-400">Loading...</div> :
         projects.length === 0 ? <EmptyState message="No projects in this organization" /> :
         <div className="divide-y divide-white/[0.06]">
           {projects.map((p) => (
            <div key={p.id} className="flex items-center justify-between px-4 py-3">
              <div>
                <span className="text-sm font-medium text-slate-200">{p.name}</span>
                {p.description && <span className="ml-2 text-xs text-slate-500">{p.description}</span>}
              </div>
              <span className="text-xs text-slate-500">{p.slug}</span>
            </div>
          ))}
        </div>}
      </Card>
    </div>
  );
}
