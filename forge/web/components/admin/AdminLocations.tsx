"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, MapPin, Plus, Trash2 } from "lucide-react";
import { type ApiLocation, createLocation, deleteLocation, fetchLocations, updateLocation } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "./admin-ui";

export function AdminLocations() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const locationsQuery = useQuery({ queryKey: ["locations"], queryFn: fetchLocations });
  const locations = locationsQuery.data ?? [];

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
  onSuccess: () => qc.invalidateQueries({ queryKey: ["locations"] }),
  onError: (e: Error) => toast({ tone: "error", title: "Failed to delete location", message: e.message }),
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
 sub="Geographic groupings for nodes."
 action={<Btn onClick={openCreate}><Plus size={14} /> New Location</Btn>}
 />

 <Card>
 <CardHeader title="All locations" icon={MapPin} />
 {locationsQuery.isLoading ? (
	 <div className="py-10 text-center text-sm text-slate-500">Loading</div>
	 ) : locationsQuery.isError ? (
	 <div className="p-4">
	   <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
	     <span>Could not load locations: {locationsQuery.error.message}</span>
	     <Btn size="sm" tone="ghost" onClick={() => void locationsQuery.refetch()}>Retry</Btn>
	   </div>
	 </div>
	 ) : locations.length === 0 ? (
 <EmptyState icon={MapPin} message="No locations yet. Create one to group nodes geographically." />
 ) : (
 <table className="w-full text-sm">
 <thead>
 <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
 <th className="px-4 py-3">Short</th>
 <th className="px-4 py-3">Description</th>
 <th className="px-4 py-3">Nodes</th>
 <th className="px-4 py-3">Servers</th>
 <th className="px-4 py-3">ID</th>
 <th className="px-4 py-3" />
 </tr>
 </thead>
 <tbody className="divide-y divide-white/[0.04]">
 {locations.map((loc) => (
 <tr key={loc.id} className="hover:bg-white/[0.02] transition-colors">
 <td className="px-4 py-3 font-mono font-semibold text-[#dc2626]">{loc.short}</td>
 <td className="px-4 py-3 text-slate-300">{loc.long || <span className="text-slate-600">-</span>}</td>
 <td className="px-4 py-3"><Pill>{loc.nodeCount}</Pill></td>
 <td className="px-4 py-3"><Pill tone={(loc.serverCount ?? 0) > 0 ? "green" : "neutral"}>{loc.serverCount ?? 0}</Pill></td>
 <td className="px-4 py-3 font-mono text-xs text-slate-500">{loc.id.slice(0, 8)}</td>
 <td className="px-4 py-3">
 <div className="flex items-center justify-end gap-1">
 <Btn size="sm" tone="ghost" onClick={() => openEdit(loc)}>Edit</Btn>
 <Btn size="sm" tone="danger" onClick={() => deleteMut.mutate(loc.id)} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
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
 <div className="grid gap-4">
 <Input label="Short code (e.g. US)" value={short} onChange={setShort} placeholder="US" mono required />
 <Input label="Description (e.g. United States)" value={long} onChange={setLong} placeholder="United States" required />
 {(short.trim() === "" || long.trim() === "") ? <p className="text-xs text-amber-300">Short code and description are required.</p> : null}
 {formError ? <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200"><AlertCircle size={14} className="mt-0.5 shrink-0" /> <span>{formError}</span></div> : null}
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
 </div>
 );
}
