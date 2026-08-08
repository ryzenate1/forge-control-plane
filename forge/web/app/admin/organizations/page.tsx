"use client";

import { useState, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { Building2, Plus } from "lucide-react";
import { createOrganization, fetchOrganizations } from "@/lib/api/tenancy";
import { useToast } from "@/components/ui/toast";
import { AdminErrorState, AdminLoadingState, AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill } from "@/components/admin/admin-ui";

function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "");
}

export default function AdminOrganizationsPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const organizationsQuery = useQuery({ queryKey: ["organizations"], queryFn: fetchOrganizations });
  const organizations = useMemo(() => organizationsQuery.data ?? [], [organizationsQuery.data]);
  const createMutation = useMutation({
    mutationFn: () => createOrganization(name.trim(), slugify(slug || name)),
    onSuccess: async (organization) => {
      await queryClient.invalidateQueries({ queryKey: ["organizations"] });
      setOpen(false);
      setName("");
      setSlug("");
      setSlugTouched(false);
      toast({ tone: "success", title: "Organization created", message: `${organization.name} is ready to configure.` });
      router.push(`/organizations/${organization.slug}`);
    },
    onError: (error) => toast({ tone: "error", title: "Could not create organization", message: error instanceof Error ? error.message : "Try a different name or slug." }),
  });
  const canCreate = Boolean(name.trim() && slugify(slug || name));

  return <AdminPageLayout>
    <AdminPageHeader title="Organizations" description="Create tenant boundaries, then group their projects, environments, and members." action={<Btn onClick={() => setOpen(true)}><Plus size={15} /> New organization</Btn>} />
    {organizationsQuery.isError ? <AdminErrorState message={organizationsQuery.error instanceof Error ? organizationsQuery.error.message : "Organizations could not be loaded."} retry={() => void organizationsQuery.refetch()} /> : null}
    <Card>
      <CardHeader title="Organizations" icon={Building2} />
      {organizationsQuery.isLoading ? <AdminLoadingState label="Loading organizations…" /> : !Array.isArray(organizations) || organizations.length === 0 ? <div className="space-y-4"><EmptyState icon={Building2} title="Create your first organization" message="Organizations separate teams, projects, environments, and member access." /><div className="flex justify-center"><Btn onClick={() => setOpen(true)}><Plus size={15} /> New organization</Btn></div></div> : <div className="divide-y divide-white/[0.06]">{Array.isArray(organizations) && organizations.map((organization) => <button className="flex w-full flex-wrap items-center justify-between gap-3 px-5 py-4 text-left transition hover:bg-white/[0.025]" key={organization.id} onClick={() => router.push(`/organizations/${organization.slug}`)} type="button"><div><p className="font-medium text-slate-100">{organization.name}</p><p className="mt-1 font-mono text-xs text-slate-500">{organization.slug}</p></div><div className="flex items-center gap-3"><span className="text-xs text-slate-500">Owner: {organization.ownerName || "Unknown"}</span>{organization.memberCount !== undefined ? <Pill>{organization.memberCount} members</Pill> : null}</div></button>)}</div>}
    </Card>
    {open ? <Modal title="Create organization" onClose={() => setOpen(false)}><form className="grid gap-4" onSubmit={(event) => { event.preventDefault(); createMutation.mutate(); }}><Input label="Organization name" onChange={(value) => { setName(value); if (!slugTouched) setSlug(slugify(value)); }} placeholder="Acme Games" value={name} /><Input label="URL slug" onChange={(value) => { setSlugTouched(true); setSlug(slugify(value)); }} placeholder="acme-games" value={slug} /><p className="text-xs leading-5 text-slate-500">The slug is used in URLs and must be unique. You can keep the suggested value or enter a different one.</p>{createMutation.isError ? <AdminErrorState message={createMutation.error instanceof Error ? createMutation.error.message : "Organization creation failed."} retry={() => createMutation.mutate()} /> : null}<ModalFooter confirmLabel={createMutation.isPending ? "Creating…" : "Create organization"} disabled={!canCreate || createMutation.isPending} onCancel={() => setOpen(false)} onConfirm={() => createMutation.mutate()} /></form></Modal> : null}
  </AdminPageLayout>;
}
