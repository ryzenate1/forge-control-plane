"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Map, Plus, Trash2 } from "lucide-react";
import { createRegion, deleteRegion, fetchRegions, updateRegion, type ApiRegion } from "@/lib/api";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminFormSection, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea, AdminLoadingState, AdminErrorState, AdminPageLayout } from "./admin-ui";

export function AdminRegions() {
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const regionsQuery = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });

  const [editing, setEditing] = useState<"new" | ApiRegion | null>(null);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [formError, setFormError] = useState<string | null>(null);

  const close = () => setEditing(null);
  const open = (region?: ApiRegion) => {
    setEditing(region ?? "new"); setName(region?.name ?? ""); setSlug(region?.slug ?? "");
    setDescription(region?.description ?? ""); setEnabled(region?.enabled ?? true);
    setFormError(null);
  };

  const normalizeSlug = (value: string): string => value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
  const validateSlug = (value: string): boolean => /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(normalizeSlug(value));

  const createMut = useMutation({
    mutationFn: () => createRegion({
      name: name.trim(),
      slug: normalizeSlug(slug),
      description: description.trim(),
      enabled
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["regions"] });
      close();
    },
    onError: (e: Error) => { console.error("Failed to create region:", e); setFormError(e.message || "Unknown error"); },
  });
  const updateMut = useMutation({
    mutationFn: () => updateRegion((editing as ApiRegion).id, {
      name: name.trim(),
      slug: normalizeSlug(slug),
      description: description.trim(),
      enabled
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["regions"] });
      close();
    },
    onError: (e: Error) => { console.error("Failed to update region:", e); setFormError(e.message || "Unknown error"); },
  });
  const deleteMut = useMutation({
    mutationFn: deleteRegion,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["regions"] }),
    onError: (e: Error) => { console.error("Failed to delete region:", e); setFormError(e.message || "Unknown error"); },
  });
  const regions = useMemo(() => Array.isArray(regionsQuery.data) ? regionsQuery.data : [], [regionsQuery.data]);

  return <AdminPageLayout>
    <SectionHeader title="Regions" sub="Cluster regions and placement zones." action={<Btn onClick={() => open()}><Plus size={14}/> New Region</Btn>} />
    {regionsQuery.isError ? <div className="mb-4"><AdminErrorState message={`Could not load regions: ${regionsQuery.error.message}`} retry={() => void regionsQuery.refetch()} /></div> : null}
    <Card><CardHeader title={`${regions.length} regions`} icon={Map}/>
      {regionsQuery.isLoading ? <AdminLoadingState label="Loading regions…" /> : regions.length === 0 ? <EmptyState icon={Map} message="No regions configured."/> :
      <div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="border-b border-line text-left text-xs uppercase text-text-muted"><th className="px-4 py-3">Region</th><th className="px-4 py-3">Slug</th><th className="px-4 py-3">Nodes</th><th className="px-4 py-3">Status</th><th/></tr></thead><tbody className="divide-y divide-line">{Array.isArray(regions) && regions.map((region) => {
        return <tr key={region.id}><td className="px-4 py-3"><button className="text-left font-semibold text-text hover:text-text" onClick={() => open(region)}>{region.name}</button><p className="text-xs text-text-muted">{region.description || "No description"}</p></td><td className="px-4 py-3 font-mono text-xs">{region.slug}</td><td className="px-4 py-3">{region.nodeCount}</td><td className="px-4 py-3"><Pill tone={region.enabled ? "green" : "yellow"}>{region.enabled ? "Enabled" : "Disabled"}</Pill></td><td className="px-4 py-3 text-right"><Btn size="sm" tone="danger" disabled={region.nodeCount > 0 || deleteMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Delete region ${region.name}?`, description: "This region will be removed from placement and recovery planning. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(region.id); })(); }} ariaLabel={`Delete region ${region.name}`}><Trash2 size={12}/></Btn></td></tr>;
      })}</tbody></table></div>}
    </Card>
    {editing ? <Modal title={editing === "new" ? "Create Region" : "Edit Region"} onClose={close}><div className="space-y-4"><AdminFormSection title="Region Details"><Input label="Name" value={name} onChange={setName}/><Input label="Slug (lowercase letters, numbers, and hyphens)" value={slug} onChange={setSlug} placeholder="us-east" mono/><Textarea label="Description" value={description} onChange={setDescription}/><label className="flex items-center gap-2 text-sm text-text"><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)}/> Enabled for placement</label></AdminFormSection>{!validateSlug(slug) && slug.trim() !== "" ? <p className="text-xs text-text-subtle">Slug may contain lowercase letters, numbers, and single hyphens only.</p> : null}{formError ? <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger"><AlertCircle size={14} className="mt-0.5 shrink-0" /> <span>{formError}</span></div> : null}</div><ModalFooter onCancel={close} onConfirm={() => editing === "new" ? createMut.mutate() : updateMut.mutate()} disabled={!name.trim() || !validateSlug(slug) || createMut.isPending || updateMut.isPending} confirmLabel={editing === "new" ? "Create" : "Save"}/></Modal> : null}
    {renderConfirm()}
  </AdminPageLayout>;
}
