"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Map, Plus, Trash2 } from "lucide-react";
import { createRegion, deleteRegion, fetchRegions, updateRegion, type ApiRegion } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea } from "./admin-ui";

export function AdminRegions() {
  const qc = useQueryClient();
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
  const regions = regionsQuery.data ?? [];

  return <div>
    <SectionHeader title="Regions" sub="Cluster regions used for placement, capacity, and recovery planning." action={<Btn onClick={() => open()}><Plus size={14}/> New Region</Btn>} />
    {regionsQuery.isError ? <p className="mb-4 rounded-lg border border-red-500/30 bg-red-950/20 p-3 text-sm text-red-300">Regions could not be loaded.</p> : null}
    <Card><CardHeader title={`${regions.length} regions`} icon={Map}/>
      {regionsQuery.isLoading ? <p className="p-6 text-sm text-slate-500">Loading regions…</p> : regions.length === 0 ? <EmptyState icon={Map} message="No regions configured."/> :
      <div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="border-b border-white/[0.06] text-left text-xs uppercase text-slate-500"><th className="px-4 py-3">Region</th><th className="px-4 py-3">Slug</th><th className="px-4 py-3">Nodes</th><th className="px-4 py-3">Status</th><th/></tr></thead><tbody className="divide-y divide-white/[0.04]">{regions.map((region) => {
        return <tr key={region.id}><td className="px-4 py-3"><button className="text-left font-semibold text-slate-200 hover:text-white" onClick={() => open(region)}>{region.name}</button><p className="text-xs text-slate-500">{region.description || "No description"}</p></td><td className="px-4 py-3 font-mono text-xs">{region.slug}</td><td className="px-4 py-3">{region.nodeCount}</td><td className="px-4 py-3"><Pill tone={region.enabled ? "green" : "yellow"}>{region.enabled ? "Enabled" : "Disabled"}</Pill></td><td className="px-4 py-3 text-right"><Btn size="sm" tone="danger" disabled={region.nodeCount > 0 || deleteMut.isPending} onClick={() => { if (confirm(`Delete ${region.name}?`)) deleteMut.mutate(region.id); }}><Trash2 size={12}/></Btn></td></tr>;
      })}</tbody></table></div>}
    </Card>
    {editing ? <Modal title={editing === "new" ? "Create Region" : "Edit Region"} onClose={close}><div className="grid gap-4"><Input label="Name" value={name} onChange={setName}/><Input label="Slug (lowercase letters, numbers, and hyphens)" value={slug} onChange={setSlug} placeholder="us-east" mono/><Textarea label="Description" value={description} onChange={setDescription}/><label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)}/> Enabled for placement</label>{!validateSlug(slug) && slug.trim() !== "" ? <p className="text-xs text-amber-300">Slug may contain lowercase letters, numbers, and single hyphens only.</p> : null}{formError ? <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200"><AlertCircle size={14} className="mt-0.5 shrink-0" /> <span>{formError}</span></div> : null}</div><ModalFooter onCancel={close} onConfirm={() => editing === "new" ? createMut.mutate() : updateMut.mutate()} disabled={!name.trim() || !validateSlug(slug) || createMut.isPending || updateMut.isPending} confirmLabel={editing === "new" ? "Create" : "Save"}/></Modal> : null}
  </div>;
}
