"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Box, Cpu, HardDrive, Plus, Terminal } from "lucide-react";
import { createTemplate, fetchTemplates } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, StatsRow } from "./admin-ui";

export function AdminTemplates() {
 const qc = useQueryClient();
 const { data: templates = [], isLoading } = useQuery({ queryKey: ["templates"], queryFn: fetchTemplates });

 const [modal, setModal] = useState(false);
 const [name, setName] = useState("");
 const [image, setImage] = useState("");
 const [startup, setStartup] = useState("");
 const [memory, setMemory] = useState("1024");

 const createMut = useMutation({
 mutationFn: () => createTemplate({
 name: name.trim(),
 image: image.trim(),
 startupCommand: startup.trim(),
 defaultMemoryMb: parseInt(memory, 10) || 1024,
 }),
 onSuccess: () => {
 qc.invalidateQueries({ queryKey: ["templates"] });
 setModal(false);
 setName("");
 setImage("");
 setStartup("");
 setMemory("1024");
 },
 });

 return (
 <div>
 <SectionHeader
 title="Templates"
 sub="Server templates define the Docker image, startup command, and default resources for new servers."
 action={<Btn onClick={() => setModal(true)}><Plus size={14} /> New Template</Btn>}
 />

 <StatsRow items={[
 { label: "Templates", value: templates.length, icon: Box, tone: "neutral" },
 ]} />

 <Card>
 <CardHeader title={`${templates.length} template${templates.length !== 1 ? "s" : ""}`} icon={Box} />
 {isLoading ? (
 <div className="py-10 text-center text-sm text-slate-500">Loading...</div>
 ) : templates.length === 0 ? (
 <EmptyState icon={Box} message="No templates configured. Create one to start deploying servers." />
 ) : (
 <div className="grid gap-4 p-4 md:grid-cols-2 lg:grid-cols-3">
 {templates.map((tpl) => (
 <div key={tpl.id} className="rounded-xl border border-white/[0.06] bg-[#161b28] p-4 space-y-3 hover:border-[#dc2626]/30 transition">
 <div className="flex items-center justify-between">
 <h3 className="text-sm font-semibold text-slate-100">{tpl.name}</h3>
 <Pill tone="blue">template</Pill>
 </div>

 <div className="space-y-2">
 <div className="flex items-center gap-2 text-xs text-slate-400">
 <HardDrive size={12} className="shrink-0" />
 <span className="font-mono truncate">{tpl.image}</span>
 </div>
 {tpl.startupCommand ? (
 <div className="flex items-center gap-2 text-xs text-slate-400">
 <Terminal size={12} className="shrink-0" />
 <span className="font-mono truncate">{tpl.startupCommand}</span>
 </div>
 ) : null}
 <div className="flex items-center gap-2 text-xs text-slate-400">
 <Cpu size={12} className="shrink-0" />
 <span>{tpl.defaultMemoryMb} MB default memory</span>
 </div>
 </div>

 <div className="pt-2 border-t border-white/[0.04]">
 <p className="text-[10px] font-mono text-slate-600 truncate">{tpl.id}</p>
 </div>
 </div>
 ))}
 </div>
 )}
 </Card>

 {modal ? (
 <Modal title="Create Template" onClose={() => setModal(false)} wide>
 <div className="grid gap-4 md:grid-cols-2">
 <Input label="Template Name" value={name} onChange={setName} placeholder="Minecraft Java" />
 <Input label="Docker Image" value={image} onChange={setImage} placeholder="itzg/minecraft-server:latest" mono />
 <div className="md:col-span-2">
 <Input label="Startup Command" value={startup} onChange={setStartup} placeholder="java -Xms128M -Xmx{{SERVER_MEMORY}}M -jar server.jar" mono />
 </div>
 <Input label="Default Memory (MB)" value={memory} onChange={setMemory} placeholder="1024" type="number" />
 </div>
 <ModalFooter
 onCancel={() => setModal(false)}
 onConfirm={() => createMut.mutate()}
 disabled={name.trim() === "" || image.trim() === "" || createMut.isPending}
 confirmLabel="Create Template"
 />
 </Modal>
 ) : null}
 </div>
 );
}
