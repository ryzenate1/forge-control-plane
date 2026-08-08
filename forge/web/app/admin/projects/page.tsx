"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderKanban, Plus } from "lucide-react";
import { AdminPageLayout, Btn, Card, CardHeader, EmptyState, SectionHeader, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
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
      <SectionHeader title="Projects" sub="Projects grouping workloads inside an organization." />

      <div className="mb-4 flex flex-col gap-3 sm:flex-row">
        <select value={selectedOrg} onChange={(e) => setSelectedOrg(e.target.value)} className="rounded-lg border border-line bg-well px-3 py-1.5 text-sm text-text">
          <option value="">Select organization...</option>
          {safeOrgs.map((org) => <option key={org.id} value={org.id}>{org.name}</option>)}
        </select>
      </div>

      {orgsQuery.isError && (
        <div className="mb-4">
          <AdminErrorState message={`Could not load organizations: ${orgsQuery.error instanceof Error ? orgsQuery.error.message : "unknown error"}`} retry={() => void orgsQuery.refetch()} />
        </div>
      )}

      {selectedOrg && (
        <form onSubmit={handleCreate} className="mb-4 flex flex-col gap-2 sm:flex-row">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Project name" className="flex-1 rounded-lg border border-line bg-well px-3 py-1.5 text-sm text-text placeholder:text-text-muted focus:outline-none focus:border-danger-line" required />
          <input value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="Description" className="w-full rounded-lg border border-line bg-well px-3 py-1.5 text-sm text-text placeholder:text-text-muted focus:outline-none focus:border-danger-line sm:w-48" />
          <Btn type="submit" disabled={createMut.isPending}><Plus size={14} /> {createMut.isPending ? "Creating..." : "Create"}</Btn>
        </form>
      )}

      <Card>
        <CardHeader title="All Projects" icon={FolderKanban} />
        {!selectedOrg ? <EmptyState message="Select an organization to view projects" /> :
         projectsQuery.isPending ? <AdminLoadingState label="Loading projects\u2026" /> :
         projectsQuery.isError ? (
           <div className="p-4">
             <AdminErrorState message={`Could not load projects: ${projectsQuery.error instanceof Error ? projectsQuery.error.message : "unknown error"}`} retry={() => void projectsQuery.refetch()} />
           </div>
         ) :
         safeProjects.length === 0 ? <EmptyState message="No projects in this organization" /> :
         <div className="divide-y divide-line">
           {safeProjects.map((p) => (
             <div key={p.id} className="flex items-center justify-between px-4 py-3">
               <div>
                 <span className="text-sm font-medium text-text">{p.name}</span>
                 {p.description && <span className="ml-2 text-xs text-text-muted">{p.description}</span>}
               </div>
               <span className="text-xs text-text-muted">{p.slug}</span>
             </div>
           ))}
         </div>}
      </Card>
    </AdminPageLayout>
  );
}
