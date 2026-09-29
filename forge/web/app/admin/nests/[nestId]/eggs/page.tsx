"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Copy, Download, FileCode, Layers, Plus, Settings, Terminal, Trash2,
} from "lucide-react";
import { type ApiEgg, fetchNest, fetchEggs, createEgg, updateEgg, deleteEgg } from "@/lib/api";
import { AdminErrorState, AdminIconButton, AdminLoadingRows, AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, Textarea } from "@/components/admin/admin-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { useBreadcrumbLabel } from "@/lib/nav/breadcrumb-context";

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
  onEdit,
  onClone,
  onExport,
  onDelete,
  onVariables,
}: {
  egg: ApiEgg;
  onEdit: () => void;
  onClone: () => void;
  onExport: () => void;
  onDelete: () => void;
  onVariables: () => void;
}) {
  const images = dockerImageLines(egg.dockerImages);
  const primaryImage = images[0] ?? egg.dockerImage;
  return (
    <div className="rounded-xl border border-line bg-overlay-subtle p-4 transition hover:border-line-strong sm:p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 space-y-1.5">
          <h3 className="break-words text-base font-semibold text-text">{egg.name}</h3>
          {egg.description && (
            <p className="text-sm leading-relaxed text-text-subtle">{egg.description}</p>
          )}
        </div>
      </div>

      <div className="mt-3 space-y-1.5">
        {primaryImage ? (
          <p className="flex items-start gap-1.5 font-mono text-xs text-text-subtle">
            <FileCode size={12} className="mt-0.5 shrink-0" />
            <span className="break-all">{primaryImage}</span>
            {images.length > 1 ? <span className="text-text-muted">+{images.length - 1} more</span> : null}
          </p>
        ) : <p className="t-meta">No image recorded</p>}
        {egg.startup ? (
          <p className="flex items-start gap-1.5 font-mono text-xs text-text-subtle">
            <Terminal size={12} className="mt-0.5 shrink-0" />
            <span className="break-all">{egg.startup}</span>
          </p>
        ) : <p className="t-meta">No startup command</p>}
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-1.5 border-t border-line pt-3">
        <Btn size="sm" tone="ghost" ariaLabel={`Manage variables for ${egg.name}`} onClick={onVariables}>
          <FileCode size={12} /> Variables
        </Btn>
        <div className="ml-auto flex items-center gap-0.5">
          <AdminIconButton label={`Edit ${egg.name}`} onClick={onEdit}><Settings size={12} /></AdminIconButton>
          <AdminIconButton label={`Clone ${egg.name}`} onClick={onClone}><Copy size={12} /></AdminIconButton>
          <AdminIconButton label={`Export ${egg.name} as JSON`} onClick={onExport}><Download size={12} /></AdminIconButton>
          <AdminIconButton label={`Delete ${egg.name}`} tone="danger" onClick={onDelete}><Trash2 size={12} /></AdminIconButton>
        </div>
      </div>
    </div>
  );
}

export default function NestEggsPage() {
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const params = useParams();
  const router = useRouter();
  const nestId = params.nestId as string;
  const qc = useQueryClient();

  const nestQuery = useQuery({ queryKey: ["nest", nestId], queryFn: () => fetchNest(nestId) });
  const eggsQuery = useQuery({ queryKey: ["eggs", nestId], queryFn: () => fetchEggs(nestId) });
  const nest = nestQuery.data;

  // One breadcrumb trail, rendered by the shell. This names the dynamic
  // segment so it reads as the resource rather than an opaque id.
  useBreadcrumbLabel(nestId, nest?.name ?? null);
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
    onError: (err) => toast({ tone: "error", title: "Failed to create egg", message: err instanceof Error ? err.message : "An error occurred" }),
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
    onError: (err) => toast({ tone: "error", title: "Failed to update egg", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteEggMut = useMutation({
    mutationFn: deleteEgg,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["eggs", nestId] }); qc.invalidateQueries({ queryKey: ["nests"] }); },
    onError: (err) => toast({ tone: "error", title: "Failed to delete egg", message: err instanceof Error ? err.message : "An error occurred" }),
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
    onError: (err) => toast({ tone: "error", title: "Failed to clone egg", message: err instanceof Error ? err.message : "An error occurred" }),
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
    <AdminPageLayout>
      <SectionHeader
        title={nest ? `Eggs: ${nest.name}` : "Eggs"}
        info={adminPageGuides.eggs}
        backAction={() => router.push("/admin/nests")}
        backLabel="Service Definitions"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <Btn tone="primary" onClick={openEggCreate}><Plus size={14} /> New Egg</Btn>
            <Btn
              tone="subtle"
              title="Opens the bundled game-template catalog with this nest preselected as the import target"
              onClick={() => router.push(`/admin/templates?nestId=${nestId}`)}
            >
              Game template catalog
            </Btn>
          </div>
        }
      />

      {nestQuery.isError ? (
        <AdminErrorState
          message={`The nest for this page could not be loaded: ${nestQuery.error instanceof Error ? nestQuery.error.message : "Unknown error"}. The definitions below may not belong to the nest you expected.`}
          retry={() => void nestQuery.refetch()}
        />
      ) : null}

      <Card>
        <CardHeader
          title={eggsQuery.data ? `${eggs.length} egg${eggs.length === 1 ? "" : "s"}` : "Egg definitions"}
          icon={Layers}
        />

        {isLoading ? (
          <AdminLoadingRows rows={4} label="Loading eggs…" />
        ) : isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Could not load eggs: ${error instanceof Error ? error.message : "Unknown error"}`}
              retry={() => void eggsQuery.refetch()}
            />
          </div>
        ) : eggs.length === 0 ? (
          <div className="p-8">
            <EmptyState icon={Layers} title="No eggs in this nest" message="Create one from scratch, or import a bundled definition from the game template catalog." />
            <div className="mt-4 flex justify-center gap-3">
              <Btn onClick={openEggCreate}><Plus size={14} /> New Egg</Btn>
              <Btn tone="subtle" onClick={() => router.push(`/admin/templates?nestId=${nestId}`)}>Game template catalog</Btn>
            </div>
          </div>
        ) : (
          <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-3">
            {eggs.map((egg) => (
              <EggCard
                key={egg.id}
                egg={egg}
                onEdit={() => openEggEdit(egg)}
                onClone={() => cloneEggMut.mutate(egg)}
                onExport={() => exportEgg(egg)}
                onDelete={() => { void (async () => { if (await confirm({ title: `Delete egg "${egg.name}"?`, description: "Servers already created from this egg keep running. A definition still referenced by a server cannot be deleted — the API refuses, so those servers must be deleted or moved first.", danger: true, confirmLabel: "Delete" })) deleteEggMut.mutate(egg.id); })(); }}
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
      {renderConfirm()}
    </AdminPageLayout>
  );
}
