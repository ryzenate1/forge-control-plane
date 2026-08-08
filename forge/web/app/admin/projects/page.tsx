"use client";

import { useMemo, useState, useEffect } from "react";
import { FolderKanban, Plus } from "lucide-react";
import { AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, EmptyState } from "@/components/admin/admin-ui";
import { fetchOrganizations, fetchProjects, createProject } from "@/lib/api/tenancy";
import type { Organization, Project } from "@/lib/api/tenancy";
import { useToast } from "@/components/ui/toast";

export default function AdminProjectsPage() {
  const { toast } = useToast();
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [selectedOrg, setSelectedOrg] = useState<string>("");
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const safeOrgs = useMemo(() => Array.isArray(orgs) ? orgs : [], [orgs]);
  const safeProjects = useMemo(() => Array.isArray(projects) ? projects : [], [projects]);

  useEffect(() => {
    fetchOrganizations().then(setOrgs).catch((err) => {
      setOrgs([]);
      toast({ tone: "error", title: "Failed to load organizations", message: err instanceof Error ? err.message : "An error occurred" });
    });
  }, [toast]);

  const loadProjects = async (orgId: string) => {
    if (!orgId) { setProjects([]); setLoading(false); return; }
    setLoading(true);
    try { setProjects(await fetchProjects(orgId)); }
    finally { setLoading(false); }
  };

  useEffect(() => { loadProjects(selectedOrg); }, [selectedOrg]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !selectedOrg) return;
    await createProject(selectedOrg, name.trim(), undefined, desc.trim());
    setName(""); setDesc("");
    await loadProjects(selectedOrg);
  };

  return (
    <AdminPageLayout>
      <AdminPageHeader title="Projects" description="Manage projects within organizations" />

      <div className="mb-4 flex flex-col gap-3 sm:flex-row">
        <select value={selectedOrg} onChange={(e) => setSelectedOrg(e.target.value)} className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white">
          <option value="">Select organization...</option>
          {Array.isArray(safeOrgs) && safeOrgs.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}
        </select>
      </div>

      {selectedOrg && (
        <form onSubmit={handleCreate} className="mb-4 flex flex-col gap-2 sm:flex-row">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Project name" className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:outline-none focus:border-red-400/70" required />
          <input value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="Description" className="w-full rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:outline-none focus:border-red-400/70 sm:w-48" />
          <Btn type="submit"><Plus size={14} /> Create</Btn>
        </form>
      )}

      <Card>
        <CardHeader title="All Projects" icon={FolderKanban} />
        {!selectedOrg ? <EmptyState message="Select an organization to view projects" /> :
         loading ? <div className="p-6 text-sm text-slate-400">Loading...</div> :
         !Array.isArray(safeProjects) || safeProjects.length === 0 ? <EmptyState message="No projects in this organization" /> :
         <div className="divide-y divide-white/[0.06]">
           {Array.isArray(safeProjects) && safeProjects.map((p) => (
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
    </AdminPageLayout>
  );
}
