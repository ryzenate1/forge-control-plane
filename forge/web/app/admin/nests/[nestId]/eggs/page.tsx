"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft, ChevronRight, Copy, Cpu, Download, ExternalLink, FileCode, Plus, Settings, Tag, Terminal, Trash2,
} from "lucide-react";
import { type ApiEgg, fetchNest, fetchEggs, createEgg, updateEgg, deleteEgg } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, Textarea, cn } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function dockerImageLines(value: unknown): string[] {
  if (Array.isArray(value)) return value.filter((image): image is string => typeof image === "string" && image.trim() !== "");
  if (isRecord(value)) return Object.values(value).filter((image): image is string => typeof image === "string" && image.trim() !== "");
  return [];
}

function EggCard({
  egg,
  nestId,
  onEdit,
  onClone,
  onExport,
  onDelete,
  onVariables,
}: {
  egg: ApiEgg;
  nestId: string;
  onEdit: () => void;
  onClone: () => void;
  onExport: () => void;
  onDelete: () => void;
  onVariables: () => void;
}) {
  const primaryImage = dockerImageLines(egg.dockerImages)[0] ?? egg.dockerImage;
  return (
    <div className="group rounded-xl border border-white/[0.06] bg-[#111722] p-4 transition hover:border-white/[0.12] hover:bg-[#141b2b] sm:p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-1.5">
          <h3 className="truncate text-base font-semibold text-slate-100">{egg.name}</h3>
          {egg.description && (
            <p className="line-clamp-2 text-sm leading-relaxed text-slate-500">{egg.description}</p>
          )}
        </div>
        <button
          onClick={onVariables}
          className="shrink-0 rounded-lg border border-white/10 bg-white/[0.04] p-2 text-slate-400 opacity-0 transition hover:border-sky-400/40 hover:bg-sky-500/10 hover:text-sky-400 group-hover:opacity-100"
          title="Manage variables"
        >
          <Settings size={14} />
        </button>
      </div>

      <div className="mt-3 flex flex-wrap gap-1.5">
        {primaryImage && (
          <span className="inline-flex items-center gap-1 rounded-md bg-white/[0.04] px-2 py-0.5 font-mono text-[10px] text-slate-500">
            <Cpu size={10} /> {primaryImage}
          </span>
        )}
        {egg.startup && (
          <span className="inline-flex items-center gap-1 rounded-md bg-white/[0.04] px-2 py-0.5 font-mono text-[10px] text-slate-500">
            <Terminal size={10} /> {egg.startup.length > 30 ? egg.startup.slice(0, 30) + "\u2026" : egg.startup}
          </span>
        )}
      </div>

      <div className="mt-4 flex items-center gap-1.5 border-t border-white/[0.06] pt-3">
        <Btn size="sm" tone="ghost" onClick={onVariables}>
          <FileCode size={12} /> Variables
        </Btn>
        <div className="ml-auto flex items-center gap-0.5">
          <Btn size="sm" tone="ghost" onClick={onEdit}><Settings size={12} /></Btn>
          <Btn size="sm" tone="ghost" onClick={onClone}><Copy size={12} /></Btn>
          <Btn size="sm" tone="ghost" onClick={onExport}><Download size={12} /></Btn>
          <Btn size="sm" tone="danger" onClick={onDelete}><Trash2 size={12} /></Btn>
        </div>
      </div>
    </div>
  );
}

export default function NestEggsPage() {
  const params = useParams();
  const router = useRouter();
  const { toast } = useToast();
  const nestId = params.nestId as string;
  const qc = useQueryClient();

  const nestQuery = useQuery({ queryKey: ["nest", nestId], queryFn: () => fetchNest(nestId) });
  const eggsQuery = useQuery({ queryKey: ["eggs", nestId], queryFn: () => fetchEggs(nestId) });
  const nest = nestQuery.data;
  const eggs = eggsQuery.data ?? [];
  const isLoading = eggsQuery.isLoading;
  const isError = eggsQuery.isError;
  const error = eggsQuery.error;

  const [eggModal, setEggModal] = useState<null | "create" | ApiEgg>(null);

  const [eggName, setEggName] = useState("");
  const [eggDesc, setEggDesc] = useState("");
  const [eggImages, setEggImages] = useState("eclipse-temurin:21-jdk");
  const [eggStartup, setEggStartup] = useState("");
  const [eggStop, setEggStop] = useState("stop");
  const [eggFeatures, setEggFeatures] = useState("");
  const [eggInstallScript, setEggInstallScript] = useState("");
  const [eggInstallContainer, setEggInstallContainer] = useState("alpine:3.21");
  const [eggInstallEntry, setEggInstallEntry] = useState("sh");

  const resetEggForm = () => {
    setEggName(""); setEggDesc(""); setEggImages("eclipse-temurin:21-jdk");
    setEggStartup(""); setEggStop("stop"); setEggFeatures("");
    setEggInstallScript(""); setEggInstallContainer("alpine:3.21"); setEggInstallEntry("sh");
  };

  const openEggCreate = () => { resetEggForm(); setEggModal("create"); };
  const openEggEdit = (e: ApiEgg) => {
    setEggName(e.name); setEggDesc(e.description ?? "");
    setEggImages(dockerImageLines(e.dockerImages).join("\n") || e.dockerImage || "");
    setEggStartup(e.startup ?? e.startupCommand ?? "");
    const config = isRecord(e.config) ? e.config : {};
    const stop = typeof config.stop === "string" ? config.stop : "stop";
    const features = Array.isArray(config.features) ? config.features.filter((item): item is string => typeof item === "string") : [];
    setEggStop(stop);
    setEggFeatures(features.join("\n"));
    setEggInstallScript(e.installScript ?? "");
    setEggInstallContainer(e.installContainer ?? "alpine:3.21");
    setEggInstallEntry(e.installEntrypoint ?? "sh");
    setEggModal(e);
  };

  const createEggMut = useMutation({
    mutationFn: (input?: Parameters<typeof createEgg>[0]) => createEgg(input || {
      nestId, name: eggName.trim(), description: eggDesc.trim(),
      dockerImages: eggImages.split("\n").map((s) => s.trim()).filter(Boolean),
      startup: eggStartup.trim(),
      config: { stop: eggStop.trim(), features: eggFeatures.split("\n").map((value) => value.trim()).filter(Boolean) },
      installScript: eggInstallScript, installContainer: eggInstallContainer.trim(), installEntrypoint: eggInstallEntry.trim(),
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["eggs", nestId] }); qc.invalidateQueries({ queryKey: ["nests"] }); setEggModal(null); },
  });

  const updateEggMut = useMutation({
    mutationFn: (id: string) => updateEgg(id, {
      name: eggName.trim(), description: eggDesc.trim(),
      dockerImages: eggImages.split("\n").map((s) => s.trim()).filter(Boolean),
      startup: eggStartup.trim(),
      config: { stop: eggStop.trim(), features: eggFeatures.split("\n").map((value) => value.trim()).filter(Boolean) },
      installScript: eggInstallScript, installContainer: eggInstallContainer.trim(), installEntrypoint: eggInstallEntry.trim(),
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["eggs", nestId] }); qc.invalidateQueries({ queryKey: ["nests"] }); setEggModal(null); },
  });

  const deleteEggMut = useMutation({
    mutationFn: deleteEgg,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["eggs", nestId] }); qc.invalidateQueries({ queryKey: ["nests"] }); },
  });

  const cloneEggMut = useMutation({
    mutationFn: (egg: ApiEgg) => {
      const images = dockerImageLines(egg.dockerImages);
      return createEgg({
        nestId, name: `${egg.name} Copy`, description: egg.description,
        dockerImages: images.length > 0 ? images : (egg.dockerImage ? [egg.dockerImage] : []),
        startup: egg.startup ?? egg.startupCommand ?? "",
        config: isRecord(egg.config) ? egg.config : {},
        installScript: egg.installScript, installContainer: egg.installContainer, installEntrypoint: egg.installEntrypoint,
      });
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["eggs", nestId] }),
  });

  const exportEgg = (egg: ApiEgg) => {
    const blob = new Blob([JSON.stringify({
      name: egg.name, description: egg.description, dockerImages: egg.dockerImages,
      startup: egg.startup, config: egg.config, installScript: egg.installScript,
      installContainer: egg.installContainer, installEntrypoint: egg.installEntrypoint,
    }, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${egg.name.replace(/[^a-z0-9]/gi, "_").toLowerCase()}_egg.json`;
    document.body.appendChild(a); a.click(); document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-6">
      <nav className="flex items-center gap-1.5 text-xs text-slate-500">
        <button onClick={() => router.push("/admin/nests")} className="transition hover:text-slate-300" type="button">Nests</button>
        <ChevronRight size={12} className="text-slate-600" />
        <span className="text-slate-300">{nest?.name ?? "Nest"}</span>
        <ChevronRight size={12} className="text-slate-600" />
        <span className="text-slate-400">Eggs</span>
      </nav>

      <SectionHeader
        title={nest ? `Eggs: ${nest.name}` : "Eggs"}
        sub="Service definitions that define game server behavior."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Btn tone="ghost" onClick={() => router.push("/admin/nests")}>
              <ArrowLeft size={14} /> Back to Nests
            </Btn>
            <Btn tone="subtle" onClick={() => router.push(`/admin/templates?nestId=${nestId}`)}>
              Browse Templates →
            </Btn>
            <Btn onClick={openEggCreate}><Plus size={14} /> New Egg</Btn>
          </div>
        }
      />

      <Card>
        <CardHeader
          title={`${eggs.length} egg${eggs.length === 1 ? "" : "s"}`}
          icon={Tag}
          action={
            eggs.length > 0 ? (
              <Btn size="sm" tone="subtle" onClick={() => router.push(`/admin/templates?nestId=${nestId}`)}>
                Browse Templates →
              </Btn>
            ) : undefined
          }
        />

        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading eggs\u2026</div>
        ) : isError ? (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load eggs: {error?.message ?? "Unknown error"}</span>
              <Btn size="sm" tone="ghost" onClick={() => void eggsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : eggs.length === 0 ? (
          <div className="p-8">
            <EmptyState icon={Tag} message="No eggs in this nest." title="Empty Nest" sub="Create a new egg from scratch or import one from our template gallery." />
            <div className="mt-4 flex justify-center gap-3">
              <Btn onClick={openEggCreate}><Plus size={14} /> New Egg</Btn>
              <Btn tone="subtle" onClick={() => router.push(`/admin/templates?nestId=${nestId}`)}>Browse Templates →</Btn>
            </div>
          </div>
        ) : (
          <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
            {eggs.map((egg) => (
              <EggCard
                key={egg.id}
                egg={egg}
                nestId={nestId}
                onEdit={() => openEggEdit(egg)}
                onClone={() => cloneEggMut.mutate(egg)}
                onExport={() => exportEgg(egg)}
                onDelete={() => { if (confirm(`Delete egg "${egg.name}"?`)) deleteEggMut.mutate(egg.id); }}
                onVariables={() => router.push(`/admin/nests/${nestId}/eggs/${egg.id}/variables`)}
              />
            ))}
          </div>
        )}
      </Card>

      {/* Create/Edit Egg Modal */}
      {eggModal !== null ? (
        <Modal title={eggModal === "create" ? "Create Egg" : "Edit Egg"} onClose={() => setEggModal(null)} wide>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="md:col-span-2">
              <Input label="Name" value={eggName} onChange={setEggName} placeholder="Minecraft Java Edition" />
            </div>
            <div className="md:col-span-2">
              <Input label="Description" value={eggDesc} onChange={setEggDesc} placeholder="Minecraft Java Edition server" />
            </div>
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

    </div>
  );
}
