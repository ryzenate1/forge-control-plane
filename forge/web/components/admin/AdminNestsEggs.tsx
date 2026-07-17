"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Box, ChevronRight, Copy, Plus, Settings, Tag, Trash2, Download, Upload } from "lucide-react";
import { type ApiNest, type ApiEgg, createEgg, createNest, deleteEgg, deleteNest, fetchEggs, fetchNests, updateEgg, updateNest } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, Textarea, cn } from "./admin-ui";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function dockerImageLines(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.filter((image): image is string => typeof image === "string" && image.trim() !== "");
  }

  if (isRecord(value)) {
    return Object.values(value).filter((image): image is string => typeof image === "string" && image.trim() !== "");
  }

  return [];
}

export function AdminNestsEggs() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const { data: nests = [], isLoading } = useQuery({ queryKey: ["nests"], queryFn: fetchNests });

 const [selectedNest, setSelectedNest] = useState<ApiNest | null>(null);
 const [nestModal, setNestModal] = useState<null | "create" | ApiNest>(null);
 const [eggModal, setEggModal] = useState<null | "create" | ApiEgg>(null);
 const [importExportModal, setImportExportModal] = useState(false);
 const [importJson, setImportJson] = useState("");

 // Nest form state
 const [nestName, setNestName] = useState("");
 const [nestDesc, setNestDesc] = useState("");

 // Egg form state
 const [eggName, setEggName] = useState("");
 const [eggDesc, setEggDesc] = useState("");
  const [eggImages, setEggImages] = useState("eclipse-temurin:21-jdk");
 const [eggStartup, setEggStartup] = useState("");
 const [eggStop, setEggStop] = useState("stop");
 const [eggFeatures, setEggFeatures] = useState("");
 const [eggInstallScript, setEggInstallScript] = useState("");
 const [eggInstallContainer, setEggInstallContainer] = useState("alpine:3.21");
 const [eggInstallEntry, setEggInstallEntry] = useState("sh");

 const { data: eggs = [], isLoading: eggsLoading } = useQuery({
 queryKey: ["eggs", selectedNest?.id],
 queryFn: () => fetchEggs(selectedNest!.id),
 enabled: Boolean(selectedNest?.id),
 });

 const createNestMut = useMutation({
 mutationFn: () => createNest({ name: nestName.trim(), description: nestDesc.trim() }),
 onSuccess: () => { qc.invalidateQueries({ queryKey: ["nests"] }); setNestModal(null); setNestName(""); setNestDesc(""); },
 });
 const updateNestMut = useMutation({
 mutationFn: (id: string) => updateNest(id, { name: nestName.trim(), description: nestDesc.trim() }),
 onSuccess: () => { qc.invalidateQueries({ queryKey: ["nests"] }); setNestModal(null); },
 });
 const deleteNestMut = useMutation({
 mutationFn: (id: string) => deleteNest(id),
 onSuccess: () => { qc.invalidateQueries({ queryKey: ["nests"] }); if (selectedNest) setSelectedNest(null); },
 });
  const createEggMut = useMutation({
    mutationFn: (importData?: Parameters<typeof createEgg>[0]) => createEgg(importData || {
      nestId: selectedNest!.id,
      name: eggName.trim(),
      description: eggDesc.trim(),
      dockerImages: eggImages.split("\n").map((s) => s.trim()).filter(Boolean),
      startup: eggStartup.trim(),
      config: { stop: eggStop.trim(), features: eggFeatures.split("\n").map((value) => value.trim()).filter(Boolean) },
      installScript: eggInstallScript,
      installContainer: eggInstallContainer.trim(),
      installEntrypoint: eggInstallEntry.trim(),
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["eggs", selectedNest?.id] }); setEggModal(null); resetEggForm(); },
  });
 const updateEggMut = useMutation({
 mutationFn: (id: string) => updateEgg(id, {
 name: eggName.trim(),
 description: eggDesc.trim(),
 dockerImages: eggImages.split("\n").map((s) => s.trim()).filter(Boolean),
 startup: eggStartup.trim(),
 config: { stop: eggStop.trim(), features: eggFeatures.split("\n").map((value) => value.trim()).filter(Boolean) },
 installScript: eggInstallScript,
 installContainer: eggInstallContainer.trim(),
 installEntrypoint: eggInstallEntry.trim(),
 }),
 onSuccess: () => { qc.invalidateQueries({ queryKey: ["eggs", selectedNest?.id] }); setEggModal(null); },
 });
 const deleteEggMut = useMutation({
 mutationFn: deleteEgg,
 onSuccess: () => qc.invalidateQueries({ queryKey: ["eggs", selectedNest?.id] }),
 });
 const cloneEggMut = useMutation({
 mutationFn: (egg: ApiEgg) => {
   const dockerImages = dockerImageLines(egg.dockerImages);
   return createEgg({
     nestId: selectedNest!.id,
     name: `${egg.name} Copy`,
     description: egg.description,
      dockerImages: dockerImages.length > 0 ? dockerImages : (egg.dockerImage ? [egg.dockerImage] : []),
     startup: egg.startup ?? egg.startupCommand ?? "",
     config: isRecord(egg.config) ? egg.config : {},
     installScript: egg.installScript,
     installContainer: egg.installContainer,
     installEntrypoint: egg.installEntrypoint,
   });
 }, 
 onSuccess: () => qc.invalidateQueries({ queryKey: ["eggs", selectedNest?.id] }),
 });

 const exportEgg = (egg: ApiEgg) => {
  const exportData = {
    name: egg.name,
    description: egg.description,
    dockerImages: egg.dockerImages,
    startup: egg.startup,
    config: egg.config,
    installScript: egg.installScript,
    installContainer: egg.installContainer,
    installEntrypoint: egg.installEntrypoint,
  };
  const blob = new Blob([JSON.stringify(exportData, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${egg.name.replace(/[^a-z0-9]/gi, "_").toLowerCase()}_egg.json`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
 };

 const importEgg = () => {
  try {
    const data: unknown = JSON.parse(importJson);
    if (!isRecord(data)) throw new Error("Import data must be an object.");

    const dockerImages = dockerImageLines(data.dockerImages);
    if (dockerImages.length === 0) {
      toast({ tone: "error", title: "Missing Docker image", message: "The imported egg must include at least one Docker image." });
      return;
    }

    createEggMut.mutate({
      nestId: selectedNest!.id,
      name: typeof data.name === "string" && data.name.trim() ? data.name.trim() : "Imported Egg",
      description: typeof data.description === "string" ? data.description : "",
      dockerImages,
      startup: typeof data.startup === "string" ? data.startup : "",
      config: isRecord(data.config) ? data.config : {},
      installScript: typeof data.installScript === "string" ? data.installScript : undefined,
      installContainer: typeof data.installContainer === "string" ? data.installContainer : undefined,
      installEntrypoint: typeof data.installEntrypoint === "string" ? data.installEntrypoint : undefined,
    });
    setImportExportModal(false);
    setImportJson("");
  } catch {
    toast({ tone: "error", title: "Invalid JSON format", message: "The pasted content must be a JSON object." });
  }
 };


 const resetEggForm = () => {
 setEggName("");
 setEggDesc("");
  setEggImages("eclipse-temurin:21-jdk");
 setEggStartup("");
 setEggStop("stop");
 setEggFeatures("");
 setEggInstallScript("");
 setEggInstallContainer("alpine:3.21");
 setEggInstallEntry("sh");
 };

 const readEggConfig = (egg: ApiEgg) => {
 const config = isRecord(egg.config) ? egg.config : {};
 return {
 stop: typeof config.stop === "string" ? config.stop : "stop",
  features: Array.isArray(config.features) ? config.features.filter((item): item is string => typeof item === "string") : [],
  installScript: egg.installScript ?? "",
  installContainer: egg.installContainer ?? "alpine:3.21",
  installEntry: egg.installEntrypoint ?? "sh",
 };
 };

 const openNestCreate = () => { setNestName(""); setNestDesc(""); setNestModal("create"); };
 const openNestEdit = (n: ApiNest) => { setNestName(n.name); setNestDesc(n.description ?? ""); setNestModal(n); };
 const openEggCreate = () => { resetEggForm(); setEggModal("create"); };
 const openEggEdit = (e: ApiEgg) => {
 setEggName(e.name); setEggDesc(e.description ?? "");
  setEggImages(dockerImageLines(e.dockerImages).join("\n") || e.dockerImage || "");
 setEggStartup(e.startup ?? e.startupCommand ?? "");
 const config = readEggConfig(e);
 setEggStop(config.stop);
 setEggFeatures(config.features.join("\n"));
 setEggInstallScript(config.installScript);
 setEggInstallContainer(config.installContainer);
 setEggInstallEntry(config.installEntry);
 setEggModal(e);
 };


 return (
 <div>
 <SectionHeader
 title="Nests & Eggs"
 sub="Game server definitions."
 action={<Btn onClick={openNestCreate}><Plus size={14} /> New Nest</Btn>}
 />

 <div className="grid gap-6 lg:grid-cols-[280px_1fr]">
 {/* Nests sidebar */}
 <Card>
 <CardHeader title="Nests" icon={Box} />
 {isLoading ? (
 <div className="py-8 text-center text-sm text-slate-500">Loading</div>
 ) : nests.length === 0 ? (
 <EmptyState icon={Box} message="No nests yet." />
 ) : (
 <ul className="divide-y divide-white/[0.04]">
 {nests.map((nest) => {
 const eggCount = nest.eggCount ?? nest.eggs ?? 0;
 const isSelected = selectedNest?.id === nest.id;
 return (
 <li key={nest.id} className={cn("flex items-center transition hover:bg-white/[0.03]", isSelected && "border-l-2 border-[#dc2626] bg-[#dc2626]/10")}>
 <button
 aria-pressed={isSelected}
 className="min-w-0 flex-1 px-4 py-3 text-left"
 onClick={() => setSelectedNest(nest)}
 type="button"
 >
 <p className="truncate text-sm font-medium text-slate-200">{nest.name}</p>
 <p className="text-xs text-slate-500">{eggCount} egg{eggCount !== 1 ? "s" : ""}</p>
 </button>
 <div aria-label={`${nest.name} actions`} className="flex shrink-0 items-center gap-1 px-4" role="group">
 <button aria-label={`Edit ${nest.name}`} className="p-1 text-slate-500 hover:text-slate-200" onClick={() => openNestEdit(nest)} type="button"><Settings size={12} /></button>
 <button aria-label={`Delete ${nest.name}`} className="p-1 text-slate-500 hover:text-red-400" onClick={() => deleteNestMut.mutate(nest.id)} type="button"><Trash2 size={12} /></button>
 </div>
 </li>
 );
 })}
 </ul>
 )}
 </Card>

 {/* Eggs panel */}
 <Card>
 <div className="flex items-center justify-between border-b border-white/[0.06] bg-[#161b28] px-4 h-11">
 <span className="text-xs font-semibold uppercase tracking-widest text-slate-400">
 {selectedNest ? `Eggs: ${selectedNest.name}` : "Select a nest"}
 </span>
 {selectedNest ? (
 <div className="flex items-center gap-2"><Btn size="sm" onClick={() => setImportExportModal(true)}><Upload size={12} /> Import/Export</Btn><Btn size="sm" onClick={openEggCreate}><Plus size={12} /> New Egg</Btn></div>
 ) : null}
 </div>
 {!selectedNest ? (
 <EmptyState icon={ChevronRight} message="Select a nest on the left to see its eggs." />
 ) : eggsLoading ? (
 <div className="py-10 text-center text-sm text-slate-500">Loading</div>
 ) : eggs.length === 0 ? (
 <EmptyState icon={Tag} message="No eggs in this nest. Create one." />
 ) : (
 <table className="w-full text-sm">
 <thead>
 <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
 <th className="px-4 py-3">Name</th>
 <th className="px-4 py-3">Docker image(s)</th>
 <th className="px-4 py-3">Startup</th>
 <th className="px-4 py-3" />
 </tr>
 </thead>
 <tbody className="divide-y divide-white/[0.04]">
 {eggs.map((egg) => (
 <tr key={egg.id} className="hover:bg-white/[0.02]">
 <td className="px-4 py-3">
 <p className="font-medium text-slate-200">{egg.name}</p>
 <p className="text-xs text-slate-500">{egg.description}</p>
 </td>
 <td className="px-4 py-3 font-mono text-xs text-slate-400 max-w-[180px] truncate">
 {dockerImageLines(egg.dockerImages)[0] ?? egg.dockerImage ?? "-"}
 </td>
 <td className="px-4 py-3 font-mono text-xs text-slate-400 max-w-[160px] truncate">
 {egg.startup || <span className="text-slate-600">-</span>}
 </td>
 <td className="px-4 py-3">
 <div className="flex items-center justify-end gap-1">
 <Btn size="sm" tone="ghost" onClick={() => openEggEdit(egg)}>Edit</Btn>
 <Btn size="sm" tone="ghost" onClick={() => cloneEggMut.mutate(egg)}><Copy size={12} /></Btn>
 <Btn size="sm" tone="ghost" onClick={() => exportEgg(egg)}><Download size={12} /></Btn>
 <Btn size="sm" tone="danger" onClick={() => deleteEggMut.mutate(egg.id)}><Trash2 size={12} /></Btn>
 </div>
 </td>
 </tr>
 ))}
 </tbody>
 </table>
 )}
 </Card>
 </div>

 {/* Nest modal */}
 {nestModal !== null ? (
 <Modal title={nestModal === "create" ? "Create Nest" : "Edit Nest"} onClose={() => setNestModal(null)}>
 <div className="grid gap-4">
 <Input label="Name" value={nestName} onChange={setNestName} placeholder="Minecraft" />
 <Input label="Description" value={nestDesc} onChange={setNestDesc} placeholder="Games based on Minecraft" />
 </div>
 <ModalFooter
 onCancel={() => setNestModal(null)}
 onConfirm={() => nestModal === "create" ? createNestMut.mutate() : updateNestMut.mutate((nestModal as ApiNest).id)}
 disabled={nestName.trim() === "" || createNestMut.isPending || updateNestMut.isPending}
 confirmLabel={nestModal === "create" ? "Create" : "Save"}
 />
 </Modal>
 ) : null}

 {/* Egg modal */}
 {eggModal !== null ? (
 <Modal title={eggModal === "create" ? "Create Egg" : "Edit Egg"} onClose={() => setEggModal(null)} wide>
 <div className="grid gap-4 md:grid-cols-2">
 <Input label="Name" value={eggName} onChange={setEggName} placeholder="Minecraft Java Edition" />
 <Input label="Description" value={eggDesc} onChange={setEggDesc} placeholder="Minecraft Java Edition server" />
 <div className="md:col-span-2">
 <Textarea label="Docker images (one per line)" value={eggImages} onChange={setEggImages} rows={3} />
 </div>
 <div className="md:col-span-2">
 <Input label="Startup command" value={eggStartup} onChange={setEggStartup} placeholder="java -Xms128M -Xmx{{SERVER_MEMORY}}M -jar server.jar" mono />
 </div>
 <Input label="Stop command" value={eggStop} onChange={setEggStop} placeholder="stop" mono />
 <Input label="Install container" value={eggInstallContainer} onChange={setEggInstallContainer} placeholder="alpine:3.21" mono />
 <Input label="Install entrypoint" value={eggInstallEntry} onChange={setEggInstallEntry} placeholder="sh" mono />
 <div>
 <Textarea label="Features (one per line)" value={eggFeatures} onChange={setEggFeatures} rows={3} />
 </div>
 <div className="md:col-span-2">
 <Textarea label="Install script" value={eggInstallScript} onChange={setEggInstallScript} rows={8} />
 </div>
 </div>
 <ModalFooter
 onCancel={() => setEggModal(null)}
 onConfirm={() => eggModal === "create" ? createEggMut.mutate(undefined) : updateEggMut.mutate((eggModal as ApiEgg).id)}
 disabled={eggName.trim() === "" || createEggMut.isPending || updateEggMut.isPending}
 confirmLabel={eggModal === "create" ? "Create" : "Save"}
 />
 </Modal>
 ) : null}

 {/* Import/Export modal */}
 {importExportModal ? (
 <Modal title="Import/Export Egg" onClose={() => setImportExportModal(false)} wide>
 <div className="space-y-4">
 <div>
 <h4 className="text-sm font-semibold text-slate-200 mb-2">Export Egg</h4>
 <p className="text-xs text-slate-400 mb-3">Click the download icon next to any egg in the table to export its configuration as JSON.</p>
 </div>
 <div className="border-t border-white/[0.06] pt-4">
 <h4 className="text-sm font-semibold text-slate-200 mb-2">Import Egg</h4>
 <Textarea 
 label="Paste egg JSON configuration" 
 value={importJson} 
 onChange={setImportJson} 
 rows={8} 
 placeholder='{"name": "Minecraft", "description": "...", "dockerImages": [...], ...}'
 />
 </div>
 </div>
 <ModalFooter
 onCancel={() => setImportExportModal(false)}
 onConfirm={() => importEgg()}
 disabled={!importJson.trim() || createEggMut.isPending}
 confirmLabel="Import"
 />
 </Modal>
 ) : null}
 </div>
 );
}