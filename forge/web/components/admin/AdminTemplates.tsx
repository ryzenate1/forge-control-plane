"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, Box, Cpu, FileCode, Gamepad2, HardDrive, Layers, Pencil, Plus, Terminal, Trash2 } from "lucide-react";
import { createTemplate, deleteEgg, fetchTemplates, updateEgg, fetchNests, createEgg } from "@/lib/api";
import type { ApiEgg, ApiNest, UpdateEggInput } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea, StatsRow } from "./admin-ui";
import { EGG_TEMPLATES, type EggTemplateItem } from "@/lib/egg-templates";
import { useSearchParams } from "next/navigation";
import { useToast } from "@/components/ui/toast";

const emptyForm = {
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

export function AdminTemplates() {
  const qc = useQueryClient();
  const templatesQuery = useQuery({ queryKey: ["templates"], queryFn: fetchTemplates });
  const templates = templatesQuery.data ?? [];
  const isLoading = templatesQuery.isLoading;

  const searchParams = useSearchParams();
  const preselectedNestId = searchParams.get("nestId");
  const { toast } = useToast();

  const [showCreate, setShowCreate] = useState(false);
  const [editing, setEditing] = useState<ApiEgg | null>(null);
  const [form, setForm] = useState({ ...emptyForm });
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [importTemplate, setImportTemplate] = useState<EggTemplateItem | null>(null);

  const nestsQuery = useQuery({ queryKey: ["nests"], queryFn: fetchNests });
  const nests = nestsQuery.data ?? [];

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
      qc.invalidateQueries({ queryKey: ["nests"] });
      setImportTemplate(null);
      toast({ tone: "success", title: "Egg imported", message: "Game template has been imported as an egg." });
    },
    onError: (err: Error) => {
      toast({ tone: "error", title: "Import failed", message: err.message });
    },
  });

  const set = (key: keyof typeof emptyForm) => (val: string) => setForm((f) => ({ ...f, [key]: val }));

  const resetForm = () => setForm({ ...emptyForm });

  const openCreate = () => {
    resetForm();
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
      name: tpl.name ?? "",
      description: tpl.description ?? "",
      dockerImages: images.join("\n"),
      startupCommand: tpl.startup ?? tpl.startupCommand ?? "",
      stopCommand: (tpl.config?.stop as string) ?? "",
      defaultMemory: String(tpl.defaultMemoryMb ?? 1024),
      installContainer: tpl.installContainer ?? "",
      installEntrypoint: tpl.installEntrypoint ?? "",
      installScript: tpl.installScript ?? "",
      features: existingFeatures,
      fileDenylist: (tpl.fileDenylist ?? []).join("\n"),
    });
    setEditing(tpl);
    setShowCreate(false);
  };

  const closeModal = () => {
    setShowCreate(false);
    setEditing(null);
    resetForm();
  };

  const createMut = useMutation({
    mutationFn: () => {
      const images = form.dockerImages.split("\n").map((s) => s.trim()).filter(Boolean);
      return createTemplate({
        name: form.name.trim(),
        image: images[0] ?? "",
        startupCommand: form.startupCommand.trim(),
        defaultMemoryMb: parseInt(form.defaultMemory, 10) || 1024,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      closeModal();
    },
  });

  const updateMut = useMutation({
    mutationFn: () => {
      if (!editing) throw new Error("No template being edited");
      const images = form.dockerImages.split("\n").map((s) => s.trim()).filter(Boolean);
      const denylist = form.fileDenylist.split("\n").map((s) => s.trim()).filter(Boolean);
      const features = form.features.split("\n").map((s) => s.trim()).filter(Boolean);
      const input: UpdateEggInput = {
        name: form.name.trim() || undefined,
        description: form.description.trim() || undefined,
        startup: form.startupCommand.trim() || undefined,
        defaultMemoryMb: parseInt(form.defaultMemory, 10) || undefined,
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
      closeModal();
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteEgg(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["templates"] });
      setDeletingId(null);
    },
  });

  const confirmDelete = (id: string) => {
    if (deletingId === id) {
      deleteMut.mutate(id);
    } else {
      setDeletingId(id);
      setTimeout(() => setDeletingId((curr) => (curr === id ? null : curr)), 3000);
    }
  };

  return (
    <div>
      <SectionHeader
        title="Templates"
        sub="Browse game templates from our catalog. Import them into your nests as eggs."
        action={<Btn onClick={openCreate}><Plus size={14} /> New Template</Btn>}
      />

      <StatsRow items={[
        { label: "Templates", value: templates?.length ?? 0, icon: Box, tone: "neutral" },
      ]} />

      <Card>
        <CardHeader title={`${templates?.length ?? 0} template${templates?.length !== 1 ? "s" : ""}`} icon={Box} />
        {isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading...</div>
        ) : !templates || templates.length === 0 ? (
          <EmptyState icon={Box} message="No templates configured. Create one to start deploying servers." />
        ) : (
          <div className="grid gap-4 p-4 md:grid-cols-2 lg:grid-cols-3">
            {templates.map((tpl) => {
              const images = tpl.dockerImages
                ? (Array.isArray(tpl.dockerImages) ? tpl.dockerImages : Object.values(tpl.dockerImages))
                : tpl.image
                  ? [tpl.image]
                  : [];
              const startupCmd = tpl.startup ?? tpl.startupCommand ?? "";
              const varCount = tpl.variables?.length ?? 0;
              const denyCount = tpl.fileDenylist?.length ?? 0;
              return (
                <div key={tpl.id} className="rounded-xl border border-white/[0.06] bg-[#161b28] p-4 space-y-3 hover:border-[#dc2626]/30 transition">
                  <div className="flex items-center justify-between">
                    <h3 className="text-sm font-semibold text-slate-100">{tpl.name}</h3>
                    <Pill tone="blue">template</Pill>
                  </div>

                  {tpl.description ? (
                    <p className="text-xs text-slate-400 leading-relaxed line-clamp-2">{tpl.description}</p>
                  ) : null}

                  <div className="space-y-2">
                    {images.length > 0 ? (
                      <div className="flex items-start gap-2 text-xs text-slate-400">
                        <HardDrive size={12} className="shrink-0 mt-0.5" />
                        <div className="flex flex-wrap gap-1">
                          {images.slice(0, 3).map((img, i) => (
                            <span key={i} className="font-mono truncate max-w-[160px] block" title={img}>{img}</span>
                          ))}
                          {images.length > 3 ? <Pill tone="neutral">+{images.length - 3}</Pill> : null}
                        </div>
                      </div>
                    ) : null}

                    {startupCmd ? (
                      <div className="flex items-center gap-2 text-xs text-slate-400">
                        <Terminal size={12} className="shrink-0" />
                        <span className="font-mono truncate" title={startupCmd}>{startupCmd}</span>
                      </div>
                    ) : null}

                    {tpl.installContainer || tpl.installEntrypoint ? (
                      <div className="flex flex-wrap gap-1.5">
                        {tpl.installContainer ? (
                          <Pill tone="yellow">C: {tpl.installContainer}</Pill>
                        ) : null}
                        {tpl.installEntrypoint ? (
                          <Pill tone="green">E: {tpl.installEntrypoint}</Pill>
                        ) : null}
                      </div>
                    ) : null}
                  </div>

                  <div className="flex items-center gap-4 pt-2 border-t border-white/[0.04] text-xs text-slate-500">
                    {tpl.defaultMemoryMb ? (
                      <span className="flex items-center gap-1"><Cpu size={11} /> {tpl.defaultMemoryMb} MB</span>
                    ) : null}
                    {varCount > 0 ? (
                      <span className="flex items-center gap-1"><Layers size={11} /> {varCount} var{varCount !== 1 ? "s" : ""}</span>
                    ) : null}
                    {denyCount > 0 ? (
                      <span className="flex items-center gap-1"><Ban size={11} /> {denyCount} denied</span>
                    ) : null}
                    {images.length > 0 ? (
                      <span className="flex items-center gap-1"><FileCode size={11} /> {images.length} image{images.length !== 1 ? "s" : ""}</span>
                    ) : null}
                  </div>

                  <div className="flex items-center gap-2 pt-1">
                    <Btn size="sm" tone="subtle" onClick={() => openEdit(tpl)}>
                      <Pencil size={12} /> Edit
                    </Btn>
                    <Btn
                      size="sm"
                      tone={deletingId === tpl.id ? "danger" : "subtle"}
                      disabled={deleteMut.isPending && deletingId === tpl.id}
                      onClick={() => confirmDelete(tpl.id)}
                    >
                      {deletingId === tpl.id ? <><Trash2 size={12} /> Confirm?</> : <><Trash2 size={12} /> Delete</>}
                    </Btn>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>

      {(showCreate || editing) ? (
        <Modal title={editing ? `Edit ${editing.name}` : "Create Template"} onClose={closeModal} wide>
          <div className="grid gap-4 md:grid-cols-2">
            <Input label="Template Name *" value={form.name} onChange={set("name")} placeholder="Minecraft Java" required />
            <Input label="Default Memory (MB)" value={form.defaultMemory} onChange={set("defaultMemory")} placeholder="1024" type="number" />

            <div className="md:col-span-2">
              <Textarea label="Description" value={form.description} onChange={set("description")} rows={2} placeholder="Optional description for this template..." />
            </div>

            <Textarea label="Docker Images (one per line)" value={form.dockerImages} onChange={set("dockerImages")} rows={3} placeholder="ghcr.io/pterodactyl/yolks:java_21" />
            <Textarea label="Startup Command" value={form.startupCommand} onChange={set("startupCommand")} rows={3} placeholder="java -Xms128M -XX:MaxRAMPercentage=95.0 -jar {{SERVER_JARFILE}}" />

            <Input label="Stop Command" value={form.stopCommand} onChange={set("stopCommand")} placeholder="stop" mono />
            <Input label="Install Container" value={form.installContainer} onChange={set("installContainer")} placeholder="ghcr.io/pterodactyl/installers:alpine" mono />
            <Input label="Install Entrypoint" value={form.installEntrypoint} onChange={set("installEntrypoint")} placeholder="ash" mono />
            <Textarea label="Install Script" value={form.installScript} onChange={set("installScript")} rows={4} placeholder="#!/bin/ash" />

            <div className="md:col-span-2">
              <Textarea label="Features (one per line)" value={form.features} onChange={set("features")} rows={2} placeholder="eula&#10;java_version&#10;pid_limit" />
            </div>
            <div className="md:col-span-2">
              <Textarea label="File Denylist (one per line)" value={form.fileDenylist} onChange={set("fileDenylist")} rows={2} placeholder="*.exe&#10;*.bat" />
            </div>
          </div>
          <ModalFooter
            onCancel={closeModal}
            onConfirm={() => (editing ? updateMut : createMut).mutate()}
            disabled={form.name.trim() === "" || createMut.isPending || updateMut.isPending}
            confirmLabel={editing ? "Save Changes" : "Create Template"}
          />
        </Modal>
      ) : null}

      {/* Game Template Catalog */}
      <SectionHeader
        title="Game Template Catalog"
        sub="Pre-configured game server definitions. Import one into a nest to get started quickly."
      />

      <Card>
        <CardHeader title={`${EGG_TEMPLATES.length} game template${EGG_TEMPLATES.length !== 1 ? "s" : ""}`} icon={Gamepad2} />
        <div className="grid gap-4 p-4 md:grid-cols-2 lg:grid-cols-3">
          {EGG_TEMPLATES.map((t) => (
            <div key={t.id} className="rounded-xl border border-white/[0.06] bg-[#161b28] p-4 space-y-3 hover:border-sky-400/30 transition">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <h3 className="text-sm font-semibold text-slate-100">{t.name}</h3>
                  <p className="mt-0.5 line-clamp-2 text-xs text-slate-500">{t.description}</p>
                </div>
                <Pill tone="blue">{t.game}</Pill>
              </div>

              <div className="space-y-2">
                <div className="flex flex-wrap gap-1.5">
                  {Object.entries(t.images).map(([label, img]) => (
                    <span key={label} className="inline-flex items-center gap-1 rounded-md bg-white/[0.04] px-2 py-0.5 font-mono text-[10px] text-slate-500" title={img}>
                      <HardDrive size={10} /> {label}
                    </span>
                  ))}
                </div>
                {t.startup && (
                  <div className="flex items-center gap-2 text-xs text-slate-400">
                    <Terminal size={12} className="shrink-0" />
                    <span className="font-mono truncate text-[11px]" title={t.startup}>{t.startup}</span>
                  </div>
                )}
              </div>

              <div className="flex items-center gap-3 pt-2 border-t border-white/[0.04] text-xs text-slate-500">
                <span className="flex items-center gap-1"><Layers size={11} /> {t.env.length} var{t.env.length !== 1 ? "s" : ""}</span>
                <span className="flex items-center gap-1"><FileCode size={11} /> {t.features.length} feature{t.features.length !== 1 ? "s" : ""}</span>
              </div>

              <Btn size="sm" onClick={() => setImportTemplate(t)}>
                <Gamepad2 size={12} /> Import as Egg
              </Btn>
            </div>
          ))}
        </div>
      </Card>

      {/* Import Game Template Modal */}
      {importTemplate ? (
        <ImportTemplateModal
          template={importTemplate}
          nests={nests}
          preselectedNestId={preselectedNestId}
          isPending={importEggMut.isPending}
          onImport={(nestId) => importEggMut.mutate({ nestId, template: importTemplate })}
          onClose={() => setImportTemplate(null)}
        />
      ) : null}
    </div>
  );
}

function ImportTemplateModal({
  template,
  nests,
  preselectedNestId,
  isPending,
  onImport,
  onClose,
}: {
  template: EggTemplateItem;
  nests: ApiNest[];
  preselectedNestId: string | null;
  isPending: boolean;
  onImport: (nestId: string) => void;
  onClose: () => void;
}) {
  const initialNest = preselectedNestId && nests.find((n) => n.id === preselectedNestId)
    ? preselectedNestId
    : nests.length > 0
      ? nests[0].id
      : "";
  const [selectedNest, setSelectedNest] = useState(initialNest);

  return (
    <Modal title={`Import "${template.name}"`} onClose={onClose} wide>
      <div className="space-y-5">
        <div>
          <h4 className="text-sm font-semibold text-slate-200 mb-1">{template.name}</h4>
          <p className="text-xs text-slate-400">{template.description}</p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <h4 className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Images</h4>
            <div className="space-y-1">
              {Object.entries(template.images).map(([label, img]) => (
                <code key={label} className="block truncate rounded bg-white/[0.04] px-2 py-1 font-mono text-[11px] text-slate-300">{label}: {img}</code>
              ))}
            </div>
          </div>
          <div className="space-y-2">
            <h4 className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">Variables ({template.env.length})</h4>
            <div className="space-y-1">
              {template.env.slice(0, 5).map((v) => (
                <div key={v.envVariable} className="flex items-center gap-2 text-[11px]">
                  <code className="rounded bg-amber-500/10 px-1.5 py-0.5 font-mono text-amber-300">{v.envVariable}</code>
                  <span className="truncate text-slate-500">{v.name}</span>
                </div>
              ))}
              {template.env.length > 5 && (
                <p className="text-[11px] text-slate-600">+{template.env.length - 5} more</p>
              )}
            </div>
          </div>
        </div>

        <div className="border-t border-white/[0.06] pt-4">
          <label className="mb-1.5 block text-xs font-semibold text-slate-300">Target Nest</label>
          {nests.length === 0 ? (
            <p className="text-xs text-slate-500">No nests available. Create a nest first.</p>
          ) : (
            <select
              value={selectedNest}
              onChange={(e) => setSelectedNest(e.target.value)}
              className="w-full rounded-lg border border-white/[0.06] bg-[#111722] px-3 py-2 text-sm text-slate-200 focus:border-sky-500/50 focus:outline-none"
            >
              {nests.map((n) => (
                <option key={n.id} value={n.id}>{n.name}</option>
              ))}
            </select>
          )}
        </div>
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={() => { if (selectedNest) onImport(selectedNest); }}
        disabled={!selectedNest || isPending}
        confirmLabel={isPending ? "Importing..." : "Import Egg"}
      />
    </Modal>
  );
}
