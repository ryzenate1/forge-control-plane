"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Box, Copy, Download, ExternalLink, Gamepad2, Layers, Plus, Settings, Trash2, Upload } from "lucide-react";
import { type ApiNest, type ApiEgg, createEgg, createNest, deleteEgg, deleteNest, fetchEggs, fetchNests, updateEgg, updateNest } from "@/lib/api";
import { useRouter } from "next/navigation";
import { useToast } from "@/components/ui/toast";
import {
  AdminConfirmDialog,
  AdminErrorState,
  AdminFormSection,
  AdminIconButton,
  AdminLoadingRows,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  SectionHeader,
  Textarea,
  cn,
} from "./admin-ui";
import { adminPageGuides } from "./admin-page-guides";

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

/** `GET /nests` always carries `eggCount`; the `eggs` field is the older spelling.
 * A nest whose payload reports neither is unknown, not empty — printing "0 eggs"
 * would be a measured zero the server never claimed. */
function eggCountOf(nest: ApiNest): number | null {
  if (typeof nest.eggCount === "number") return nest.eggCount;
  if (typeof nest.eggs === "number") return nest.eggs;
  return null;
}

function messageOf(err: unknown): string {
  return err instanceof Error && err.message ? err.message : "The request could not be completed.";
}

export function AdminNestsEggs() {
  const router = useRouter();
  const { toast } = useToast();
  const qc = useQueryClient();
  const nestsQuery = useQuery({ queryKey: ["nests"], queryFn: fetchNests });
  const nests = useMemo(() => Array.isArray(nestsQuery.data) ? nestsQuery.data : [], [nestsQuery.data]);

  const [selectedNest, setSelectedNest] = useState<ApiNest | null>(null);
  const [nestModal, setNestModal] = useState<null | "create" | ApiNest>(null);
  const [eggModal, setEggModal] = useState<null | "create" | ApiEgg>(null);
  const [importExportModal, setImportExportModal] = useState(false);
  const [importJson, setImportJson] = useState("");
  const [deleteNestTarget, setDeleteNestTarget] = useState<ApiNest | null>(null);
  const [deleteEggTarget, setDeleteEggTarget] = useState<ApiEgg | null>(null);

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

  const eggsQuery = useQuery({
    queryKey: ["eggs", selectedNest?.id],
    queryFn: () => fetchEggs(selectedNest!.id),
    enabled: Boolean(selectedNest?.id),
  });
  const eggs = useMemo(() => Array.isArray(eggsQuery.data) ? eggsQuery.data : [], [eggsQuery.data]);

  const invalidateDefinitions = () => {
    qc.invalidateQueries({ queryKey: ["nests"] });
    qc.invalidateQueries({ queryKey: ["eggs"] });
    qc.invalidateQueries({ queryKey: ["templates"] });
  };

  const createNestMut = useMutation({
    mutationFn: () => createNest({ name: nestName.trim(), description: nestDesc.trim() }),
    onSuccess: () => { invalidateDefinitions(); setNestModal(null); setNestName(""); setNestDesc(""); toast({ tone: "success", title: "Nest created" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to create nest", message: messageOf(err) }),
  });
  const updateNestMut = useMutation({
    mutationFn: (id: string) => updateNest(id, { name: nestName.trim(), description: nestDesc.trim() }),
    onSuccess: () => { invalidateDefinitions(); setNestModal(null); toast({ tone: "success", title: "Nest updated" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to update nest", message: messageOf(err) }),
  });
  const deleteNestMut = useMutation({
    mutationFn: (id: string) => deleteNest(id),
    onSuccess: () => { invalidateDefinitions(); setSelectedNest(null); setDeleteNestTarget(null); toast({ tone: "success", title: "Nest deleted" }); },
    onError: (err) => { toast({ tone: "error", title: "Nest not deleted", message: messageOf(err) }); setDeleteNestTarget(null); },
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
    onSuccess: () => { invalidateDefinitions(); setEggModal(null); resetEggForm(); toast({ tone: "success", title: "Egg created" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to create egg", message: messageOf(err) }),
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
    onSuccess: () => { invalidateDefinitions(); setEggModal(null); toast({ tone: "success", title: "Egg updated" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to update egg", message: messageOf(err) }),
  });
  const deleteEggMut = useMutation({
    mutationFn: deleteEgg,
    onSuccess: () => { invalidateDefinitions(); setDeleteEggTarget(null); toast({ tone: "success", title: "Egg deleted" }); },
    onError: (err) => { toast({ tone: "error", title: "Egg not deleted", message: messageOf(err) }); setDeleteEggTarget(null); },
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
    onSuccess: () => { invalidateDefinitions(); toast({ tone: "success", title: "Egg cloned" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to clone egg", message: messageOf(err) }),
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

  const nestDeleteCount = deleteNestTarget ? eggCountOf(deleteNestTarget) : null;

  return (
    <>
      <SectionHeader
        info={adminPageGuides.nests}
        action={<Btn tone="primary" onClick={openNestCreate}><Plus size={14} /> New Nest</Btn>}
      />

      <div className="grid gap-6 lg:grid-cols-[280px_1fr]">
        {/* Nests list */}
        <Card>
          <CardHeader title={nestsQuery.data ? `${nests.length} nests` : "Nests"} icon={Box} />
          {nestsQuery.isLoading ? (
            <AdminLoadingRows rows={4} label="Loading nests…" />
          ) : nestsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Nests could not be loaded: ${messageOf(nestsQuery.error)}`}
                retry={() => void nestsQuery.refetch()}
              />
            </div>
          ) : nests.length === 0 ? (
            <EmptyState icon={Box} title="No nests yet" message="A nest groups related game definitions. Create one to start." />
          ) : (
            <ul className="divide-y divide-line">
              {nests.map((nest) => {
                const eggCount = eggCountOf(nest);
                const isSelected = selectedNest?.id === nest.id;
                return (
                  <li
                    key={nest.id}
                    className={cn(
                      "flex items-center transition",
                      "hover:bg-overlay-subtle",
                      isSelected && "border-l-2 border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]",
                    )}
                  >
                    <button
                      aria-pressed={isSelected}
                      className="min-w-0 flex-1 rounded-md px-4 py-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                      onClick={() => setSelectedNest(nest)}
                      type="button"
                    >
                      <p className="truncate text-sm font-medium text-text">{nest.name}</p>
                      <p className="text-xs text-text-subtle">
                        {eggCount === null ? "Egg count not reported" : `${eggCount} egg${eggCount !== 1 ? "s" : ""}`}
                      </p>
                    </button>
                    {/* Always visible: the old hover-only group hid these from touch and
                        keyboard users, and `opacity-0` kept them focusable but invisible. */}
                    <div aria-label={`${nest.name} actions`} className="flex shrink-0 items-center gap-1 pr-3" role="group">
                      <AdminIconButton label={`Open ${nest.name}`} onClick={() => router.push(`/admin/nests/${nest.id}/eggs`)}>
                        <ExternalLink size={12} />
                      </AdminIconButton>
                      <AdminIconButton label={`Edit ${nest.name}`} onClick={() => openNestEdit(nest)}>
                        <Settings size={12} />
                      </AdminIconButton>
                      <AdminIconButton label={`Delete ${nest.name}`} tone="danger" onClick={() => setDeleteNestTarget(nest)}>
                        <Trash2 size={12} />
                      </AdminIconButton>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </Card>

        {/* Eggs panel */}
        <Card>
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line bg-overlay-subtle px-4 py-2.5">
            <span className="t-meta">
              {selectedNest ? `Eggs: ${selectedNest.name}` : "Select a nest"}
            </span>
            {selectedNest ? (
              <div className="flex items-center gap-2">
                <Btn size="sm" tone="primary" onClick={openEggCreate}><Plus size={12} /> New Egg</Btn>
                <Btn size="sm" tone="subtle" onClick={() => setImportExportModal(true)}><Upload size={12} /> Import/Export</Btn>
                <Btn
                  size="sm"
                  tone="ghost"
                  title="Opens the bundled game-template catalog; the nest you came from is preselected as the import target"
                  onClick={() => router.push(`/admin/templates?nestId=${selectedNest.id}`)}
                >
                  <Gamepad2 size={12} /> Game template catalog
                </Btn>
              </div>
            ) : null}
          </div>
          {!selectedNest ? (
            <EmptyState icon={Box} title="No nest selected" message="Choose a nest on the left to see the definitions inside it." />
          ) : eggsQuery.isLoading ? (
            <AdminLoadingRows rows={4} label="Loading eggs…" />
          ) : eggsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Eggs for ${selectedNest.name} could not be loaded: ${messageOf(eggsQuery.error)}`}
                retry={() => void eggsQuery.refetch()}
              />
            </div>
          ) : eggs.length === 0 ? (
            <EmptyState icon={Layers} title="No eggs in this nest" message="Create one, or import a bundled game template from the catalog." />
          ) : (
            <AdminTable label={`Eggs in ${selectedNest.name}`}>
              <AdminTHead>
                <AdminTh>Name</AdminTh>
                <AdminTh>Docker image(s)</AdminTh>
                <AdminTh>Startup</AdminTh>
                <AdminTh>Variables</AdminTh>
                <AdminTh className="text-right">Actions</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {eggs.map((egg) => (
                  <AdminTr key={egg.id}>
                    <AdminTd>
                      <p className="font-medium text-text">{egg.name}</p>
                      <p className="text-xs text-text-subtle">{egg.description || "No description"}</p>
                    </AdminTd>
                    <AdminTd className="max-w-[22ch] break-all font-mono text-xs text-text-subtle">
                      {dockerImageLines(egg.dockerImages)[0] ?? egg.dockerImage ?? <span className="text-text-muted">No image recorded</span>}
                      {dockerImageLines(egg.dockerImages).length > 1 ? (
                        <span className="ml-1 text-text-subtle">+{dockerImageLines(egg.dockerImages).length - 1}</span>
                      ) : null}
                    </AdminTd>
                    <AdminTd className="max-w-[24ch] break-all font-mono text-xs text-text-subtle">
                      {egg.startup || <span className="text-text-muted">No startup command</span>}
                    </AdminTd>
                    <AdminTd>
                      <Btn size="sm" tone="ghost" ariaLabel={`Manage variables for ${egg.name}`} onClick={() => router.push(`/admin/nests/${selectedNest!.id}/eggs/${egg.id}/variables`)}>
                        <ExternalLink size={12} /> Variables
                      </Btn>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex items-center justify-end gap-1">
                        <Btn size="sm" tone="ghost" ariaLabel={`Edit ${egg.name}`} onClick={() => openEggEdit(egg)}>Edit</Btn>
                        <AdminIconButton label={`Clone ${egg.name}`} disabled={cloneEggMut.isPending} onClick={() => cloneEggMut.mutate(egg)}>
                          <Copy size={12} />
                        </AdminIconButton>
                        <AdminIconButton label={`Export ${egg.name} as JSON`} onClick={() => exportEgg(egg)}>
                          <Download size={12} />
                        </AdminIconButton>
                        <AdminIconButton label={`Delete ${egg.name}`} tone="danger" onClick={() => setDeleteEggTarget(egg)}>
                          <Trash2 size={12} />
                        </AdminIconButton>
                      </div>
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      </div>

      {/* Nest modal */}
      {nestModal !== null ? (
        <Modal title={nestModal === "create" ? "Create Nest" : "Edit Nest"} onClose={() => setNestModal(null)}>
          <div className="space-y-4">
            <AdminFormSection title="Nest Details">
              <Input label="Name" value={nestName} onChange={setNestName} placeholder="Minecraft" />
              <Input label="Description" value={nestDesc} onChange={setNestDesc} placeholder="Games based on Minecraft" />
            </AdminFormSection>
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
            <AdminFormSection title="Basic Info">
              <Input label="Name" value={eggName} onChange={setEggName} placeholder="Minecraft Java Edition" />
              <Input label="Description" value={eggDesc} onChange={setEggDesc} placeholder="Minecraft Java Edition server" />
            </AdminFormSection>
            <AdminFormSection title="Configuration">
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
            </AdminFormSection>
            <AdminFormSection title="Install Script">
              <div className="md:col-span-2">
                <Textarea label="Install script" value={eggInstallScript} onChange={setEggInstallScript} rows={8} />
              </div>
            </AdminFormSection>
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
              <h3 className="text-sm font-semibold text-text">Export an egg</h3>
              <p className="text-xs leading-5 text-text-subtle">Use the download action beside any egg to save its configuration as JSON.</p>
              <p className="ui-hint mt-1">An import is filed into <span className="font-medium">{selectedNest?.name ?? "the selected nest"}</span>.</p>
            </div>
            <div className="border-t border-line pt-4">
              <h3 className="text-sm font-semibold text-text">Import an egg</h3>
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

      <AdminConfirmDialog
        title={`Delete nest "${deleteNestTarget?.name ?? ""}"?`}
        description={
          // `store_nests.go:173-177` refuses while the nest holds eggs, so the
          // old promise that "the nest and its eggs will be removed" was a scope
          // the API will not perform.
          nestDeleteCount && nestDeleteCount > 0
            ? `This nest reports ${nestDeleteCount} egg(s), so the API will refuse the delete: move or delete those eggs first. Servers built from this nest's definitions keep running either way.`
            : "The nest is removed. Its eggs must already be gone — the API refuses a nest that still holds definitions. Servers built from this nest's definitions keep running."
        }
        confirmLabel="Delete"
        destructive
        loading={deleteNestMut.isPending}
        onCancel={() => { if (!deleteNestMut.isPending) setDeleteNestTarget(null); }}
        onConfirm={() => { if (deleteNestTarget) deleteNestMut.mutate(deleteNestTarget.id); }}
        open={Boolean(deleteNestTarget)}
      />
      <AdminConfirmDialog
        title={`Delete egg "${deleteEggTarget?.name ?? ""}"?`}
        description="The egg definition is removed. Servers already created from it keep running and keep their configuration; a definition still referenced by a server cannot be deleted, so those servers must be deleted or moved first."
        confirmLabel="Delete"
        destructive
        loading={deleteEggMut.isPending}
        onCancel={() => { if (!deleteEggMut.isPending) setDeleteEggTarget(null); }}
        onConfirm={() => { if (deleteEggTarget) deleteEggMut.mutate(deleteEggTarget.id); }}
        open={Boolean(deleteEggTarget)}
      />

    </>
  );
}
