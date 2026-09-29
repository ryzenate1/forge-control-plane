"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Ban, FileCode, Gamepad2, Layers, Pencil, Plus, Terminal, Trash2 } from "lucide-react";
import { createEgg, deleteEgg, fetchTemplates, updateEgg, fetchNests } from "@/lib/api";
import type { ApiEgg, ApiNest, UpdateEggInput } from "@/lib/api";
import {
  AdminErrorState,
  AdminIconButton,
  AdminLoadingRows,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  Textarea,
} from "./admin-ui";
import { adminPageGuides } from "./admin-page-guides";
import { EGG_TEMPLATES, type EggTemplateItem } from "@/lib/egg-templates";
import { useSearchParams, useRouter } from "next/navigation";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";

const emptyForm = {
  nestId: "",
  name: "",
  description: "",
  dockerImages: "",
  startupCommand: "",
  stopCommand: "",
  defaultMemory: "1024",
  installContainer: "",
  installEntrypoint: "",
  installScript: "",
  features: "",
  fileDenylist: "",
};

function lines(value: string): string[] {
  return value.split("\n").map((s) => s.trim()).filter(Boolean);
}

function messageOf(err: unknown): string {
  return err instanceof Error && err.message ? err.message : "The request could not be completed.";
}

export function AdminTemplates() {
  const qc = useQueryClient();
  const router = useRouter();
  const templatesQuery = useQuery({ queryKey: ["templates"], queryFn: fetchTemplates });
  const templates = useMemo(() => (Array.isArray(templatesQuery.data) ? templatesQuery.data : []), [templatesQuery.data]);

  const searchParams = useSearchParams();
  const preselectedNestId = searchParams.get("nestId");
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  const [showCreate, setShowCreate] = useState(false);
  const [editing, setEditing] = useState<ApiEgg | null>(null);
  const [form, setForm] = useState({ ...emptyForm });
  const [formError, setFormError] = useState<string | null>(null);
  const [importTemplate, setImportTemplate] = useState<EggTemplateItem | null>(null);

  const nestsQuery = useQuery({ queryKey: ["nests"], queryFn: fetchNests });
  const nests = useMemo(() => (Array.isArray(nestsQuery.data) ? nestsQuery.data : []), [nestsQuery.data]);

  const importEggMut = useMutation({
    mutationFn: (params: { nestId: string; template: EggTemplateItem }) => {
      const { nestId, template } = params;
      const images = Object.values(template.images);
      return createEgg({
        nestId,
        name: template.name,
        description: template.description,
        dockerImages: images,
        startup: template.startup,
        config: { ...template.config, features: template.features },
        installScript: template.installScript,
        installContainer: template.installContainer,
        installEntrypoint: template.installEntrypoint,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["eggs"] });
      qc.invalidateQueries({ queryKey: ["templates"] });
      qc.invalidateQueries({ queryKey: ["nests"] });
      setImportTemplate(null);
      toast({ tone: "success", title: "Egg imported", message: "Game template has been imported as an egg." });
    },
    onError: (err: Error) => {
      toast({ tone: "error", title: "Import failed", message: messageOf(err) });
    },
  });

  const set = (key: keyof typeof emptyForm) => (val: string) => setForm((f) => ({ ...f, [key]: val }));

  const resetForm = () => setForm({ ...emptyForm });

  const openCreate = () => {
    resetForm();
    setFormError(null);
    setShowCreate(true);
    setEditing(null);
  };

  const openEdit = (tpl: ApiEgg) => {
    const images = tpl.dockerImages
      ? (Array.isArray(tpl.dockerImages) ? tpl.dockerImages : Object.values(tpl.dockerImages))
      : tpl.image
        ? [tpl.image]
        : [];
    const existingFeatures = tpl.config?.features
      ? (Array.isArray(tpl.config.features) ? (tpl.config.features as string[]).join("\n") : "")
      : "";
    setForm({
      nestId: tpl.nestId ?? "",
      name: tpl.name ?? "",
      description: tpl.description ?? "",
      dockerImages: images.join("\n"),
      startupCommand: tpl.startup ?? tpl.startupCommand ?? "",
      stopCommand: (tpl.config?.stop as string) ?? "",
      defaultMemory: String(tpl.defaultMemoryMb ?? ""),
      installContainer: tpl.installContainer ?? "",
      installEntrypoint: tpl.installEntrypoint ?? "",
      installScript: tpl.installScript ?? "",
      features: existingFeatures,
      fileDenylist: (tpl.fileDenylist ?? []).join("\n"),
    });
    setFormError(null);
    setEditing(tpl);
    setShowCreate(false);
  };

  const closeModal = () => {
    setShowCreate(false);
    setEditing(null);
    setFormError(null);
    resetForm();
  };

  /** Every field the form collects is sent. The previous create path hit
   * `POST /templates`, which files an egg into a nest literally named "Games"
   * and accepts only name/image/startup/memory — eight of the twelve fields
   * shown here were discarded server-side with no message (`store_templates.go`).
   * `POST /eggs` is the resource this list actually reads (`GET /eggs`), and it
   * takes the whole definition. */
  const createMut = useMutation({
    mutationFn: (values: typeof emptyForm) => {
      const images = lines(values.dockerImages);
      const features = lines(values.features);
      const config: Record<string, unknown> = {};
      if (values.stopCommand.trim()) config.stop = values.stopCommand.trim();
      if (features.length > 0) config.features = features;
      return createEgg({
        nestId: values.nestId,
        name: values.name.trim(),
        description: values.description.trim(),
        dockerImages: images,
        startup: values.startupCommand.trim(),
        config,
        defaultMemoryMb: Number(values.defaultMemory),
        installScript: values.installScript,
        installContainer: values.installContainer.trim(),
        installEntrypoint: values.installEntrypoint.trim(),
        fileDenylist: lines(values.fileDenylist),
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      qc.invalidateQueries({ queryKey: ["eggs"] });
      qc.invalidateQueries({ queryKey: ["nests"] });
      closeModal();
      toast({ tone: "success", title: "Definition created", message: "Saved as an egg in the nest you chose." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create template", message: messageOf(err) }),
  });

  const updateMut = useMutation({
    mutationFn: () => {
      if (!editing) throw new Error("No template being edited");
      const images = lines(form.dockerImages);
      const denylist = lines(form.fileDenylist);
      const features = lines(form.features);
      const input: UpdateEggInput = {
        name: form.name.trim() || undefined,
        description: form.description.trim() || undefined,
        startup: form.startupCommand.trim() || undefined,
        defaultMemoryMb: Number(form.defaultMemory) || undefined,
        installContainer: form.installContainer.trim() || undefined,
        installEntrypoint: form.installEntrypoint.trim() || undefined,
        installScript: form.installScript.trim() || undefined,
        fileDenylist: denylist.length > 0 ? denylist : undefined,
      };
      if (images.length > 0) {
        input.dockerImages = images;
      }
      const config: Record<string, unknown> = {};
      if (editing.config && typeof editing.config === "object") {
        Object.assign(config, editing.config);
      }
      if (form.stopCommand.trim()) {
        config.stop = form.stopCommand.trim();
      } else {
        delete config.stop;
      }
      if (features.length > 0) {
        config.features = features;
      } else {
        delete config.features;
      }
      input.config = config;
      return updateEgg(editing.id, input);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      qc.invalidateQueries({ queryKey: ["eggs"] });
      closeModal();
      toast({ tone: "success", title: "Definition updated" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to update template", message: messageOf(err) }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteEgg(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      qc.invalidateQueries({ queryKey: ["eggs"] });
      toast({ tone: "success", title: "Definition deleted" });
    },
    onError: (err) => toast({ tone: "error", title: "Delete failed", message: messageOf(err) }),
  });

  const requestDelete = (tpl: ApiEgg) => {
    void (async () => {
      const ok = await confirm({
        title: `Delete “${tpl.name}”?`,
        description:
          "This deletes the egg definition. Servers already created from it keep running and keep their configuration. A definition still referenced by a server cannot be deleted — the API refuses, and those servers must be deleted or moved first.",
        danger: true,
        confirmLabel: "Delete definition",
      });
      if (ok) deleteMut.mutate(tpl.id);
    })();
  };

  const validateCreate = () => {
    if (!form.name.trim()) return "A name is required.";
    if (!form.nestId) return "Choose the nest this definition belongs to.";
    const memory = Number(form.defaultMemory);
    if (!Number.isInteger(memory) || memory <= 0) return "Default memory must be a whole number of MiB greater than 0.";
    return null;
  };

  const nestOptions = nests.map((n) => ({ value: n.id, label: n.name }));

  return (
    <AdminPageLayout>
      <SectionHeader
        info={adminPageGuides.compatibility}
        action={
          <Btn onClick={openCreate} disabled={nests.length === 0} title={nests.length === 0 ? "A nest must exist before a definition can be filed" : undefined}>
            <Plus size={14} /> New Template
          </Btn>
        }
      />

      <div className="ui-alert ui-alert-warning" role="note">
        <span>
          These are legacy compatibility projections of eggs. New definitions belong in{" "}
          <button
            type="button"
            className="font-semibold underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            onClick={() => router.push("/admin/nests")}
          >
            Service Definitions
          </button>
          . Creating one here files an egg in the nest you choose, exactly as Service Definitions does.
        </span>
      </div>

      <Card>
        <CardHeader
          title={templatesQuery.data ? `${templates.length} definitions` : "Compatibility definitions"}
          icon={Archive}
        />
        {templatesQuery.isLoading ? (
          <AdminLoadingRows rows={4} label="Loading definitions…" />
        ) : templatesQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Definitions could not be listed: ${messageOf(templatesQuery.error)}`}
              retry={() => void templatesQuery.refetch()}
            />
          </div>
        ) : templates.length === 0 ? (
          <EmptyState
            icon={Archive}
            title="No definitions returned"
            message="The request succeeded and the list is empty. Import a bundled game template below, or create one in Service Definitions."
          />
        ) : (
          <div className="grid gap-4 p-4 md:grid-cols-2 lg:grid-cols-3">
            {templates.map((tpl) => {
              const images = tpl.dockerImages
                ? (Array.isArray(tpl.dockerImages) ? tpl.dockerImages : Object.values(tpl.dockerImages))
                : tpl.image
                  ? [tpl.image]
                  : [];
              const startupCmd = tpl.startup ?? tpl.startupCommand ?? "";
              const varCount = tpl.variables?.length;
              const denyCount = tpl.fileDenylist?.length;
              return (
                <div key={tpl.id} className="space-y-3 rounded-xl border border-line bg-overlay-subtle p-4 transition hover:border-[color-mix(in_srgb,var(--brand)_30%,transparent)]">
                  <div className="flex items-center justify-between gap-2">
                    <h3 className="min-w-0 break-words text-sm font-semibold text-text">{tpl.name}</h3>
                    <Pill tone="neutral">template</Pill>
                  </div>

                  {tpl.description ? (
                    <p className="text-xs leading-5 text-text-subtle">{tpl.description}</p>
                  ) : null}

                  <div className="space-y-2">
                    {images.length > 0 ? (
                      <div className="space-y-1">
                        <p className="t-meta">{images.length} image{images.length !== 1 ? "s" : ""}</p>
                        {images.slice(0, 3).map((img, i) => (
                          <p key={`${img}-${i}`} className="flex items-start gap-1.5 font-mono text-xs text-text-subtle">
                            <FileCode size={12} className="mt-0.5 shrink-0" />
                            <span className="break-all">{img}</span>
                          </p>
                        ))}
                        {images.length > 3 ? <Pill tone="neutral">+{images.length - 3} more</Pill> : null}
                      </div>
                    ) : <p className="t-meta">No images recorded</p>}

                    {startupCmd ? (
                      <p className="flex items-start gap-1.5 text-xs text-text-subtle">
                        <Terminal size={12} className="mt-0.5 shrink-0" />
                        <span className="break-all font-mono">{startupCmd}</span>
                      </p>
                    ) : null}

                    {tpl.installContainer || tpl.installEntrypoint ? (
                      <div className="flex flex-wrap gap-1.5">
                        {tpl.installContainer ? <Pill tone="yellow">Install container: {tpl.installContainer}</Pill> : null}
                        {tpl.installEntrypoint ? <Pill tone="green">Install entrypoint: {tpl.installEntrypoint}</Pill> : null}
                      </div>
                    ) : null}
                  </div>

                  <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-line pt-2 text-xs text-text-subtle">
                    <span>{tpl.defaultMemoryMb ? `${tpl.defaultMemoryMb} MiB default` : "No default memory"}</span>
                    {varCount !== undefined ? <span className="flex items-center gap-1"><Layers size={11} /> {varCount} variable{varCount !== 1 ? "s" : ""}</span> : null}
                    {denyCount !== undefined ? <span className="flex items-center gap-1"><Ban size={11} /> {denyCount} denied pattern{denyCount !== 1 ? "s" : ""}</span> : null}
                  </div>

                  <div className="flex items-center gap-2 pt-1">
                    <Btn size="sm" tone="subtle" ariaLabel={`Edit ${tpl.name}`} onClick={() => openEdit(tpl)}>
                      <Pencil size={12} /> Edit
                    </Btn>
                    <AdminIconButton label={`Delete ${tpl.name}`} tone="danger" disabled={deleteMut.isPending} onClick={() => requestDelete(tpl)}>
                      <Trash2 size={12} />
                    </AdminIconButton>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>

      {(showCreate || editing) ? (
        <Modal
          title={editing ? `Edit ${editing.name}` : "Create definition"}
          description="Every field below is written to the egg. Nothing is dropped on save."
          onClose={closeModal}
          wide
        >
          <div className="grid gap-4 md:grid-cols-2">
            <Input label="Name" value={form.name} onChange={set("name")} placeholder="Minecraft Java" required />
            <Input label="Default memory (MiB)" value={form.defaultMemory} onChange={set("defaultMemory")} placeholder="1024" type="number" />

            {editing ? (
              <p className="ui-hint md:col-span-2">
                This definition lives in nest{" "}
                <span className="font-mono">{nests.find((n) => n.id === (editing.nestId ?? form.nestId))?.name ?? "unknown"}</span>.
                Editing here cannot move it to another nest.
              </p>
            ) : nestsQuery.isLoading ? (
              <div className="md:col-span-2"><AdminLoadingRows rows={1} label="Loading nests…" /></div>
            ) : nestsQuery.isError ? (
              <div className="md:col-span-2">
                <AdminErrorState message={`Nests could not be loaded: ${messageOf(nestsQuery.error)}`} retry={() => void nestsQuery.refetch()} />
              </div>
            ) : nests.length === 0 ? (
              <p className="ui-alert ui-alert-warning" role="alert">
                <span>The nest list loaded successfully and is empty. Create a nest in Service Definitions first.</span>
              </p>
            ) : (
              <div className="md:col-span-2">
                <AdminSelect
                  label="Nest"
                  value={form.nestId}
                  onChange={set("nestId")}
                  placeholder="Choose a nest"
                  options={nestOptions}
                />
                <p className="ui-hint mt-1.5">Nothing is selected for you — the nest decides where this definition is filed.</p>
              </div>
            )}

            <div className="md:col-span-2">
              <Textarea label="Description" value={form.description} onChange={set("description")} rows={2} placeholder="Optional description for this definition" />
            </div>

            <Textarea label="Docker images (one per line)" value={form.dockerImages} onChange={set("dockerImages")} rows={3} placeholder="ghcr.io/pterodactyl/yolks:java_21" />
            <Textarea label="Startup command" value={form.startupCommand} onChange={set("startupCommand")} rows={3} placeholder="java -Xms128M -XX:MaxRAMPercentage=95.0 -jar {{SERVER_JARFILE}}" />

            <Input label="Stop command" value={form.stopCommand} onChange={set("stopCommand")} placeholder="stop" mono />
            <Input label="Install container" value={form.installContainer} onChange={set("installContainer")} placeholder="ghcr.io/pterodactyl/installers:alpine" mono />
            <Input label="Install entrypoint" value={form.installEntrypoint} onChange={set("installEntrypoint")} placeholder="ash" mono />
            <Textarea label="Install script" value={form.installScript} onChange={set("installScript")} rows={4} placeholder="#!/bin/ash" />

            <div className="md:col-span-2">
              <Textarea label="Features (one per line)" value={form.features} onChange={set("features")} rows={2} placeholder={"eula\njava_version\npid_limit"} />
            </div>
            <div className="md:col-span-2">
              <Textarea label="File denylist (one per line)" value={form.fileDenylist} onChange={set("fileDenylist")} rows={2} placeholder={"*.exe\n*.bat"} />
            </div>

            {formError ? <div className="ui-alert ui-alert-danger md:col-span-2" role="alert"><span>{formError}</span></div> : null}
          </div>
          <ModalFooter
            onCancel={closeModal}
            onConfirm={() => {
              if (editing) {
                const err = form.name.trim() ? null : "A name is required.";
                if (err) { setFormError(err); return; }
                updateMut.mutate();
                return;
              }
              const err = validateCreate();
              if (err) { setFormError(err); return; }
              createMut.mutate(form);
            }}
            disabled={createMut.isPending || updateMut.isPending}
            confirmLabel={editing ? (updateMut.isPending ? "Saving…" : "Save changes") : createMut.isPending ? "Creating…" : "Create definition"}
          />
        </Modal>
      ) : null}

      <AdminSection
        title="Game Template Catalog"
        description="Definitions bundled with this build of Forge — they ship with the product, they are not inventory from your infrastructure. Importing one creates an egg in the nest you choose."
      >
      <Card>
        <CardHeader title={`${EGG_TEMPLATES.length} bundled game template${EGG_TEMPLATES.length !== 1 ? "s" : ""}`} icon={Gamepad2} />
        <div className="grid gap-4 p-4 md:grid-cols-2 lg:grid-cols-3">
          {EGG_TEMPLATES.map((t) => (
            <div key={t.id} className="space-y-3 rounded-xl border border-line bg-overlay-subtle p-4 transition hover:border-line-strong">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <h3 className="text-sm font-semibold text-text">{t.name}</h3>
                  <p className="mt-0.5 text-xs leading-5 text-text-subtle">{t.description}</p>
                </div>
                <Pill tone="neutral">{t.game}</Pill>
              </div>

              <div className="space-y-2">
                <div className="space-y-1">
                  {Object.entries(t.images).map(([label, img]) => (
                    <p key={label} className="flex items-start gap-1.5 font-mono text-xs text-text-subtle">
                      <FileCode size={11} className="mt-0.5 shrink-0" />
                      <span className="break-all">{label}: {img}</span>
                    </p>
                  ))}
                </div>
                {t.startup && (
                  <p className="flex items-start gap-1.5 text-xs text-text-subtle">
                    <Terminal size={12} className="mt-0.5 shrink-0" />
                    <span className="break-all font-mono">{t.startup}</span>
                  </p>
                )}
              </div>

              <div className="flex flex-wrap items-center gap-3 border-t border-line pt-2 text-xs text-text-subtle">
                <span className="flex items-center gap-1"><Layers size={11} /> {t.env.length} variable{t.env.length !== 1 ? "s" : ""}</span>
                <span className="flex items-center gap-1"><FileCode size={11} /> {t.features.length} feature{t.features.length !== 1 ? "s" : ""}</span>
              </div>

              <Btn size="sm" ariaLabel={`Import ${t.name} as an egg`} onClick={() => setImportTemplate(t)}>
                <Gamepad2 size={12} /> Import as Egg
              </Btn>
            </div>
          ))}
        </div>
      </Card>
      </AdminSection>

      {importTemplate ? (
        <ImportTemplateModal
          template={importTemplate}
          nests={nests}
          nestsError={nestsQuery.isError ? messageOf(nestsQuery.error) : null}
          nestsLoading={nestsQuery.isLoading}
          preselectedNestId={preselectedNestId}
          isPending={importEggMut.isPending}
          onImport={(nestId) => importEggMut.mutate({ nestId, template: importTemplate })}
          onClose={() => setImportTemplate(null)}
        />
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );
}

function ImportTemplateModal({
  template,
  nests,
  nestsError,
  nestsLoading,
  preselectedNestId,
  isPending,
  onImport,
  onClose,
}: {
  template: EggTemplateItem;
  nests: ApiNest[];
  nestsError: string | null;
  nestsLoading: boolean;
  preselectedNestId: string | null;
  isPending: boolean;
  onImport: (nestId: string) => void;
  onClose: () => void;
}) {
  // Only honour an explicit `?nestId=` that names a real nest. Never fall back
  // to `nests[0]`: picking the target for the operator is exactly the silent
  // ambiguity AGENTS.md forbids.
  const offeredDefault = preselectedNestId && nests.some((n) => n.id === preselectedNestId) ? preselectedNestId : "";
  const [selectedNest, setSelectedNest] = useState(offeredDefault);

  return (
    <Modal title={`Import "${template.name}"`} onClose={onClose} wide>
      <div className="space-y-4">
        <div>
          <h3 className="text-sm font-semibold text-text">{template.name}</h3>
          <p className="text-xs leading-5 text-text-subtle">{template.description}</p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <h4 className="t-meta">Images</h4>
            <div className="space-y-1">
              {Object.entries(template.images).map(([label, img]) => (
                <p key={label} className="break-all rounded bg-overlay-subtle px-2 py-1 font-mono text-xs text-text-subtle">{label}: {img}</p>
              ))}
            </div>
          </div>
          <div className="space-y-2">
            <h4 className="t-meta">Variables ({template.env.length})</h4>
            <div className="space-y-1">
              {template.env.slice(0, 5).map((v) => (
                <div key={v.envVariable} className="flex items-start gap-2 text-xs">
                  <code className="rounded bg-overlay px-1.5 py-0.5 font-mono text-text">{v.envVariable}</code>
                  <span className="break-words text-text-subtle">{v.name}</span>
                </div>
              ))}
              {template.env.length > 5 && (
                <p className="text-xs text-text-subtle">+{template.env.length - 5} more</p>
              )}
            </div>
          </div>
        </div>

        <div className="border-t border-line pt-4">
          {nestsLoading ? (
            <AdminLoadingRows rows={1} label="Loading nests…" />
          ) : nestsError ? (
            <AdminErrorState message={`Nests could not be loaded: ${nestsError}`} />
          ) : nests.length === 0 ? (
            <p className="ui-hint">The nest list loaded successfully and is empty. Create a nest in Service Definitions first.</p>
          ) : (
            <>
              <AdminSelect
                label="Target nest"
                value={selectedNest}
                onChange={setSelectedNest}
                placeholder="Choose a nest"
                options={nests.map((n) => ({ value: n.id, label: n.name }))}
              />
              <p className="ui-hint mt-1.5">
                {offeredDefault
                  ? "Preselected from the nest you came from; change it if that is not the target."
                  : "Nothing is chosen for you — the import runs only against the nest you pick."}
              </p>
            </>
          )}
        </div>
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={() => { if (selectedNest) onImport(selectedNest); }}
        disabled={!selectedNest || isPending}
        confirmLabel={isPending ? "Importing…" : "Import Egg"}
      />
    </Modal>
  );
}

export default AdminTemplates;
