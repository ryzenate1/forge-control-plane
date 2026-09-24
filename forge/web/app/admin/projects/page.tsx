"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderKanban, Plus } from "lucide-react";
import { AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, EmptyState } from "@/components/admin/admin-ui";
import { fetchOrganizations, fetchProjects, createProject } from "@/lib/api/tenancy";
import { useToast } from "@/components/ui/toast";

export default function AdminProjectsPage() {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [selectedOrg, setSelectedOrg] = useState<string>("");
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");

  const orgsQuery = useQuery({
    queryKey: ["organizations"],
    queryFn: fetchOrganizations,
    retry: 1,
  });

  const projectsQuery = useQuery({
    queryKey: ["projects", selectedOrg],
    queryFn: () => fetchProjects(selectedOrg),
    enabled: Boolean(selectedOrg),
    retry: 1,
  });

  const createMut = useMutation({
    mutationFn: () => createProject(selectedOrg, name.trim(), undefined, desc.trim()),
    onSuccess: () => {
      setName("");
      setDesc("");
      toast({ tone: "success", title: "Project created" });
      void queryClient.invalidateQueries({ queryKey: ["projects", selectedOrg] });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create project", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const safeOrgs = Array.isArray(orgsQuery.data) ? orgsQuery.data : [];
  const safeProjects = Array.isArray(projectsQuery.data) ? projectsQuery.data : [];

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !selectedOrg || createMut.isPending) return;
    createMut.mutate();
  };

  return (
    <AdminPageLayout>
      <AdminPageHeader title="Projects" description="Manage projects within organizations" />

      <div className="mb-4 flex flex-col gap-3 sm:flex-row">
        <select value={selectedOrg} onChange={(e) => setSelectedOrg(e.target.value)} className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white">
          <option value="">Select organization...</option>
          {safeOrgs.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}
        </select>
      </div>

      {orgsQuery.isError && (
        <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
          <span>Could not load organizations: {orgsQuery.error instanceof Error ? orgsQuery.error.message : "unknown error"}</span>
          <Btn size="sm" tone="ghost" onClick={() => void orgsQuery.refetch()}>Retry</Btn>
        </div>
      )}

      {selectedOrg && (
        <form onSubmit={handleCreate} className="mb-4 flex flex-col gap-2 sm:flex-row">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Project name" className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:outline-none focus:border-red-400/70" required />
          <input value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="Description" className="w-full rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:outline-none focus:border-red-400/70 sm:w-48" />
          <Btn type="submit" disabled={createMut.isPending}><Plus size={14} /> {createMut.isPending ? "Creating..." : "Create"}</Btn>
        </form>
      )}

      <Card>
        <CardHeader title="All Projects" icon={FolderKanban} />
        {!selectedOrg ? <EmptyState message="Select an organization to view projects" /> :
         projectsQuery.isPending ? <div className="p-6 text-sm text-slate-400">Loading...</div> :
         projectsQuery.isError ? (
           <div className="p-4">
             <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
               <span>Could not load projects: {projectsQuery.error instanceof Error ? projectsQuery.error.message : "unknown error"}</span>
               <Btn size="sm" tone="ghost" onClick={() => void projectsQuery.refetch()}>Retry</Btn>
             </div>
           </div>
         ) :
         safeProjects.length === 0 ? <EmptyState message="No projects in this organization" /> :
         <div className="divide-y divide-white/[0.06]">
           {safeProjects.map((p) => (
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
