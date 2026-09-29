"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderKanban, Plus } from "lucide-react";
import { AdminErrorState, AdminLoadingState, AdminPageHeader, AdminPageLayout, AdminSelect, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { fetchOrganizations, fetchProjects, createProject } from "@/lib/api/tenancy";
import { useToast } from "@/components/ui/toast";

export default function AdminProjectsPage() {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [selectedOrg, setSelectedOrg] = useState<string>("");
  const [open, setOpen] = useState(false);
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
      setOpen(false);
      toast({ tone: "success", title: "Project created" });
      void queryClient.invalidateQueries({ queryKey: ["projects", selectedOrg] });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create project", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const safeOrgs = Array.isArray(orgsQuery.data) ? orgsQuery.data : [];
  const safeProjects = Array.isArray(projectsQuery.data) ? projectsQuery.data : [];

  return (
    <AdminPageLayout>
      <AdminPageHeader action={<Btn onClick={() => setOpen(true)} disabled={!selectedOrg}><Plus size={14} /> New project</Btn>} />
      {/* Disabled with the reason next to it, never hidden and never
          enabled-and-lying: a project belongs to an organization, so there is
          nowhere to create one until an organization is chosen. */}
      {!selectedOrg ? (
        <p className="ui-hint">New project is disabled until an organization is selected below — projects are created inside one organization.</p>
      ) : null}

      <div className="max-w-xl">
        <AdminSelect label="Organization" value={selectedOrg} onChange={setSelectedOrg} placeholder={orgsQuery.isLoading ? "Loading organizations…" : "Select organization..."} options={safeOrgs.map((org) => ({ value: org.id, label: org.name }))} />
      </div>

      {orgsQuery.isError ? (
        <AdminErrorState message={orgsQuery.error instanceof Error ? orgsQuery.error.message : "Organizations could not be loaded."} retry={() => void orgsQuery.refetch()} />
      ) : null}

      <Card>
        <CardHeader title={selectedOrg && projectsQuery.isSuccess ? `Projects (${safeProjects.length})` : "Projects"} icon={FolderKanban} />
        {!selectedOrg ? <EmptyState icon={FolderKanban} title="Select an organization" message="Select an organization to view its projects." /> :
         projectsQuery.isLoading ? <div className="p-4"><AdminLoadingState label="Loading projects…" /></div> :
         projectsQuery.isError ? (
           <div className="p-4"><AdminErrorState message={projectsQuery.error instanceof Error ? projectsQuery.error.message : "Projects could not be loaded."} retry={() => void projectsQuery.refetch()} /></div>
         ) :
         safeProjects.length === 0 ? <EmptyState icon={FolderKanban} title="No projects" message="This organization has no projects yet. Create one to group workloads." /> :
         <div className="divide-y divide-line">
           {safeProjects.map((proj) => (
             <div key={proj.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
               <div className="min-w-0">
                 <span className="text-sm font-medium text-text">{proj.name}</span>
                 {proj.description ? <span className="ml-2 text-xs text-text-subtle">{proj.description}</span> : null}
               </div>
               <span className="font-mono text-xs text-text-subtle">{proj.slug}</span>
             </div>
           ))}
         </div>}
      </Card>

      {open ? (
        <Modal title="New project" onClose={() => setOpen(false)}>
          <div className="space-y-4">
            <AdminSelect label="Organization" value={selectedOrg} onChange={setSelectedOrg} placeholder="Select organization..." options={safeOrgs.map((org) => ({ value: org.id, label: org.name }))} />
            <Input label="Project name" value={name} onChange={setName} placeholder="Project name" />
            <Input label="Description" value={desc} onChange={setDesc} placeholder="Description" />
            {createMut.isError ? <AdminErrorState message={createMut.error instanceof Error ? createMut.error.message : "Project could not be created."} retry={() => createMut.mutate()} /> : null}
          </div>
          <ModalFooter onCancel={() => setOpen(false)} onConfirm={() => createMut.mutate()} disabled={!name.trim() || !selectedOrg || createMut.isPending} confirmLabel={createMut.isPending ? "Creating…" : "Create project"} />
        </Modal>
      ) : null}
    </AdminPageLayout>
  );
}
