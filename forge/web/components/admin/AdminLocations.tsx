"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, MapPin, Plus, Trash2 } from "lucide-react";
import { type ApiLocation, createLocation, deleteLocation, fetchLocations, updateLocation } from "@/lib/api";
import { toast } from "@/components/ui/sonner";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminFormSection, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminLoadingState, AdminErrorState } from "./admin-ui";

export function AdminLocations() {
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const locationsQuery = useQuery({ queryKey: ["locations"], queryFn: fetchLocations });
  const locations = useMemo(() => Array.isArray(locationsQuery.data) ? locationsQuery.data : [], [locationsQuery.data]);

 const [modal, setModal] = useState<null | "create" | { id: string; short: string; long: string }>(null);
 const [short, setShort] = useState("");
 const [long, setLong] = useState("");
 const [formError, setFormError] = useState<string | null>(null);

  const createMut = useMutation({
   mutationFn: () => createLocation({ short: short.trim(), long: long.trim() }),
   onSuccess: () => { qc.invalidateQueries({ queryKey: ["locations"] }); setModal(null); setShort(""); setLong(""); setFormError(null); },
   onError: (e: Error) => { console.error("Create location error:", e); setFormError(e.message || "Failed to create location"); },
  });

 const updateMut = useMutation({
  mutationFn: (id: string) => updateLocation(id, { short: short.trim(), long: long.trim() }),
  onSuccess: () => { qc.invalidateQueries({ queryKey: ["locations"] }); setModal(null); setFormError(null); },
   onError: (e: Error) => { console.error("Failed to update location:", e); setFormError(e.message); },
  });

  const deleteMut = useMutation({
  mutationFn: deleteLocation,
  onSuccess: () => { qc.invalidateQueries({ queryKey: ["locations"] }); toast.success("Location deleted"); },
  onError: (e: Error) => toast.error(e.message || "Failed to delete location"),
 });

 const openEdit = (loc: ApiLocation) => {
 setShort(loc.short);
 setLong(loc.long);
 setModal({ id: loc.id, short: loc.short, long: loc.long });
 };

 const openCreate = () => { setShort(""); setLong(""); setFormError(null); setModal("create"); };

 return (
 <div>
 <SectionHeader
 title="Locations"
 sub="Physical and logical node locations."
 action={<Btn onClick={openCreate}><Plus size={14} /> New Location</Btn>}
 />

 <Card>
 <CardHeader title="All locations" icon={MapPin} />
 {locationsQuery.isLoading ? (
	 <AdminLoadingState label="Loading locationsâ¦" />
	 ) : locationsQuery.isError ? (
	 <div className="p-4">
	   <AdminErrorState message={`Could not load locations: ${locationsQuery.error.message}`} retry={() => void locationsQuery.refetch()} />
	 </div>
	 ) : locations.length === 0 ? (
 <EmptyState icon={MapPin} message="No locations yet. Create one to group nodes geographically." />
 ) : (
 <table className="w-full text-sm">
 <thead>
 <tr className="border-b border-line text-left text-xs text-text-muted uppercase tracking-wider">
 <th className="px-4 py-3">Short</th>
 <th className="px-4 py-3">Description</th>
 <th className="px-4 py-3">Nodes</th>
 <th className="px-4 py-3">Servers</th>
 <th className="px-4 py-3">ID</th>
 <th className="px-4 py-3" />
 </tr>
 </thead>
 <tbody className="divide-y divide-line">
  {Array.isArray(locations) && locations.map((loc) => (
 <tr key={loc.id} className="hover:bg-overlay-subtle transition-colors">
 <td className="px-4 py-3 font-mono font-semibold text-[var(--brand)]">{loc.short}</td>
 <td className="px-4 py-3 text-text">{loc.long || <span className="text-text-muted">-</span>}</td>
 <td className="px-4 py-3"><Pill>{loc.nodeCount}</Pill></td>
 <td className="px-4 py-3"><Pill tone={(loc.serverCount ?? 0) > 0 ? "green" : "neutral"}>{loc.serverCount ?? 0}</Pill></td>
 <td className="px-4 py-3 font-mono text-xs text-text-muted">{loc.id.slice(0, 8)}</td>
 <td className="px-4 py-3">
 <div className="flex items-center justify-end gap-1">
 <Btn size="sm" tone="ghost" onClick={() => openEdit(loc)}>Edit</Btn>
  <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete location "${loc.short}"?`, description: loc.long ? `"${loc.long}" and its placement metadata will be removed. This cannot be undone.` : "This placement will be removed. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(loc.id); })(); }} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
 </div>
 </td>
 </tr>
 ))}
 </tbody>
 </table>
 )}
 </Card>

  {modal !== null ? (
  <Modal title={modal === "create" ? "Create Location" : "Edit Location"} onClose={() => setModal(null)}>
  <div className="space-y-4">
  <AdminFormSection title="Location Details">
  <Input label="Short code (e.g. US)" value={short} onChange={setShort} placeholder="US" mono required />
  <Input label="Description (e.g. United States)" value={long} onChange={setLong} placeholder="United States" required />
  </AdminFormSection>
  {(short.trim() === "" || long.trim() === "") ? <p className="text-xs text-text-subtle">Short code and description are required.</p> : null}
  {formError ? <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger"><AlertCircle size={14} className="mt-0.5 shrink-0" /> <span>{formError}</span></div> : null}
  </div>
 <ModalFooter
 onCancel={() => setModal(null)}
 onConfirm={() => {
 if (short.trim() === "" || long.trim() === "") {
 setFormError("Short code and description are required.");
 return;
 }
 setFormError(null);
 if (modal === "create") createMut.mutate();
 else updateMut.mutate((modal as { id: string }).id);
 }}
  disabled={createMut.isPending || updateMut.isPending}
 confirmLabel={modal === "create" ? "Create" : "Save"}
 />
 </Modal>
  ) : null}
  {renderConfirm()}
  </div>
  );
}
