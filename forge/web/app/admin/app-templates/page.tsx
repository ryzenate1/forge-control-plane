"use client";
import { adminPageGuides } from "@/components/admin/admin-page-guides";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Plus, Pencil, Trash2, Layers,
} from "lucide-react";
import { AdminPageLayout, AdminSelect, Btn, Card, CardHeader, EmptyState, Input, SectionHeader, Pill, Textarea, cn, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { type AppType, type AppTemplate, type AppPort } from "@/lib/api/apps";
import { DEFAULT_APP_TEMPLATES, loadUserTemplates, saveUserTemplates, getAllTemplates } from "@/lib/app-templates-data";
import { useConfirm } from "@/components/ui/confirm-dialog";

type FormData = {
  name: string;
  description: string;
  type: AppType;
  image: string;
  gitUrl: string;
  gitBranch: string;
  composeContent: string;
  ports: string;
  envVars: string;
  cpu: string;
  memory: string;
  disk: string;
};

const emptyForm: FormData = {
  name: "", description: "", type: "image", image: "", gitUrl: "",
  gitBranch: "main", composeContent: "", ports: "", envVars: "",
  cpu: "1.0", memory: "512", disk: "1024",
};

function generateId(): string {
  return "tpl_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2, 6);
}

function formToTemplate(form: FormData): AppTemplate {
  const ports: AppPort[] = form.ports
    ? form.ports.split(",").map((p) => p.trim()).filter(Boolean).map((p) => {
        const parts = p.split(":");
        return { hostPort: parseInt(parts[0]) || 8080, containerPort: parseInt(parts[1]) || parseInt(parts[0]) || 80, protocol: "tcp" };
      })
    : [];
  const envVars: Record<string, string> = form.envVars
    ? Object.fromEntries(form.envVars.split(",").map((e) => e.trim()).filter(Boolean).map((e) => {
        const eqIdx = e.indexOf("=");
        return eqIdx > 0 ? [e.slice(0, eqIdx).trim(), e.slice(eqIdx + 1).trim()] : [e, ""];
      }))
    : {};
  return {
    id: generateId(),
    name: form.name,
    description: form.description,
    type: form.type,
    image: form.type === "image" ? form.image : undefined,
    gitUrl: form.type === "git" ? form.gitUrl : undefined,
    gitBranch: form.type === "git" ? form.gitBranch : undefined,
    composeContent: form.type === "compose" ? form.composeContent : undefined,
    defaultPorts: ports,
    defaultEnvVars: envVars,
    defaultResources: { cpu: form.cpu, memory: form.memory, disk: form.disk },
  };
}

function templateToForm(tpl: AppTemplate): FormData {
  return {
    name: tpl.name,
    description: tpl.description,
    type: tpl.type,
    image: tpl.image ?? "",
    gitUrl: tpl.gitUrl ?? "",
    gitBranch: tpl.gitBranch ?? "main",
    composeContent: tpl.composeContent ?? "",
    ports: tpl.defaultPorts.map((p) => `${p.hostPort}:${p.containerPort}`).join(", "),
    envVars: Object.entries(tpl.defaultEnvVars).map(([k, v]) => `${k}=${v}`).join(", "),
    cpu: tpl.defaultResources.cpu,
    memory: tpl.defaultResources.memory,
    disk: tpl.defaultResources.disk,
  };
}

function typeLabel(t: AppType): string {
  switch (t) {
    case "image": return "Docker Image";
    case "git": return "Git Repository";
    case "compose": return "Docker Compose";
    default: return t;
  }
}

export default function AppTemplatesPage() {
  const router = useRouter();
  const [confirm, renderConfirm] = useConfirm();
  const [search, setSearch] = useState("");
  const [templates, setTemplates] = useState<AppTemplate[]>([]);
  const [showModal, setShowModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<FormData>(emptyForm);

  const refresh = useCallback(() => {
    setTemplates(getAllTemplates());
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const openCreate = () => {
    setEditingId(null);
    setForm(emptyForm);
    setShowModal(true);
  };

  const openEdit = (tpl: AppTemplate) => {
    setEditingId(tpl.id);
    setForm(templateToForm(tpl));
    setShowModal(true);
  };

  const save = () => {
    if (!form.name.trim()) return;
    if (
      (form.type === "image" && !form.image.trim()) ||
      (form.type === "git" && !form.gitUrl.trim()) ||
      (form.type === "compose" && !form.composeContent.trim())
    ) return;
    const users = loadUserTemplates();
    const tpl = formToTemplate(form);
    if (editingId) {
      const idx = users.findIndex((t) => t.id === editingId);
      if (idx >= 0) {
        users[idx] = { ...tpl, id: editingId };
      } else {
        users.push(tpl);
      }
    } else {
      users.push(tpl);
    }
    saveUserTemplates(users);
    setShowModal(false);
    refresh();
  };

  const remove = async (id: string) => {
    const confirmed = await confirm({ title: "Delete this template?", description: "The template will be removed from this browser's saved templates. This cannot be undone.", danger: true, confirmLabel: "Delete" });
    if (!confirmed) return;
    const users = loadUserTemplates().filter((t) => t.id !== id);
    saveUserTemplates(users);
    refresh();
  };

  const visibleTemplates = templates.filter((template) => `${template.name} ${template.description}`.toLowerCase().includes(search.toLowerCase()));
  const sourceMissing =
    (form.type === "image" && !form.image.trim()) ||
    (form.type === "git" && !form.gitUrl.trim()) ||
    (form.type === "compose" && !form.composeContent.trim());

  const isDefault = (id: string) => DEFAULT_APP_TEMPLATES.some((t) => t.id === id);

  return (
    <AdminPageLayout>
      <SectionHeader
        info={adminPageGuides.appTemplates}
        title="App Templates"
        sub="Application deployment templates for the Create Application wizard"
        backAction={() => router.push("/admin/apps")}
        backLabel="Apps"
        action={
          <Btn tone="primary" onClick={openCreate}>
            <Plus size={14} />
            New Template
          </Btn>
        }
      />

      <Card>
        <CardHeader title={`${visibleTemplates.length} templates`} icon={Layers} />
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div className="w-full max-w-md"><Input label="Search app templates" value={search} onChange={setSearch} placeholder="Search by name or description" /></div>
          <p className="text-xs text-text-subtle">Custom templates are saved in this browser.</p>
        </div>
        <div className="grid gap-4 p-4 sm:grid-cols-2 lg:grid-cols-3">
          {visibleTemplates.map((tpl) => (
            <div
              key={tpl.id}
              className={cn(
                "flex min-h-48 flex-col rounded-xl border p-5 text-left",
                isDefault(tpl.id)
                  ? "border-line bg-overlay-subtle"
                  : "border-[color-mix(in_srgb,var(--brand)_20%,transparent)] bg-[color-mix(in_srgb,var(--brand)_2%,transparent)]",
              )}
            >
              <div className="flex items-start justify-between gap-2">
                <Pill tone="neutral">{typeLabel(tpl.type)}</Pill>
                {isDefault(tpl.id) ? (
                  <Pill tone="blue">Default</Pill>
                ) : (
                  <div className="flex gap-1">
                    <button
                      type="button"
                      className="rounded p-1 text-text-muted transition hover:bg-white/[0.06] hover:text-text"
                      aria-label={`Edit ${tpl.name}`}
                      onClick={() => openEdit(tpl)}
                    >
                      <Pencil size={12} />
                    </button>
                    <button
                      type="button"
                      className="rounded p-1 text-text-muted transition hover:bg-white/[0.06] hover:text-red-400"
                      aria-label={`Delete ${tpl.name}`}
                      onClick={() => remove(tpl.id)}
                    >
                      <Trash2 size={12} />
                    </button>
                  </div>
                )}
              </div>
              <p className="mt-2 font-semibold text-text text-sm">{tpl.name}</p>
              <p className="mt-1 text-xs text-text-muted line-clamp-2">{tpl.description}</p>
              {tpl.image && (
                <p className="mt-2 text-xs font-mono text-text-muted truncate">{tpl.image}</p>
              )}
              {tpl.gitUrl && (
                <p className="mt-2 text-xs font-mono text-text-muted truncate">{tpl.gitUrl}</p>
              )}
              <div className="mt-auto flex flex-wrap gap-1.5 border-t border-line pt-4">
                <span className="text-xs text-text-muted">
                  {tpl.defaultResources.cpu} CPU / {tpl.defaultResources.memory} MiB
                </span>
              </div>
            </div>
          ))}
          {visibleTemplates.length === 0 && (
            <div className="col-span-full"><EmptyState icon={Layers} title={search ? "No matching templates" : "No templates yet"} message={search ? "Try a different name or description." : "Create your first application template to get started."} /></div>
          )}
        </div>
      </Card>

      {showModal && (
        <Modal title={editingId ? "Edit Template" : "New Template"} onClose={() => setShowModal(false)} wide>
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <Input label="Name" value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} placeholder="My Template" required />
              </div>
              <div className="sm:col-span-2"><Textarea label="Description" value={form.description} onChange={(value) => setForm((f) => ({ ...f, description: value }))} placeholder="Brief description of this template" rows={3} /></div>
              <div>
                <AdminSelect label="Type" value={form.type} onChange={(value) => setForm((f) => ({ ...f, type: value as AppType }))} options={[
                  { value: "image", label: "Docker Image" },
                  { value: "git", label: "Git Repository" },
                  { value: "compose", label: "Docker Compose" },
                ]} />
              </div>
            </div>

            {form.type === "image" && (
              <Input label="Docker Image" value={form.image} onChange={(v) => setForm((f) => ({ ...f, image: v }))} placeholder="nginx:latest" required />
            )}
            {form.type === "git" && (
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <Input label="Git Repository URL" value={form.gitUrl} onChange={(v) => setForm((f) => ({ ...f, gitUrl: v }))} placeholder="https://github.com/user/repo.git" required />
                </div>
                <Input label="Branch" value={form.gitBranch} onChange={(v) => setForm((f) => ({ ...f, gitBranch: v }))} placeholder="main" />
              </div>
            )}
            {form.type === "compose" && (
              <Textarea label="Compose YAML" value={form.composeContent} onChange={(value) => setForm((f) => ({ ...f, composeContent: value }))} placeholder={`services:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"`} rows={10} />
            )}

            <div className="grid gap-4 sm:grid-cols-3">
              <Input label="CPU (cores)" value={form.cpu} onChange={(v) => setForm((f) => ({ ...f, cpu: v }))} placeholder="1.0" />
              <Input label="Memory (MiB)" value={form.memory} onChange={(v) => setForm((f) => ({ ...f, memory: v }))} placeholder="512" />
              <Input label="Disk (MiB)" value={form.disk} onChange={(v) => setForm((f) => ({ ...f, disk: v }))} placeholder="1024" />
            </div>

            <Input label="Ports (host:container, comma-separated)" value={form.ports} onChange={(v) => setForm((f) => ({ ...f, ports: v }))} placeholder="8080:80, 3000:3000" />
            <Input label="Env Vars (KEY=value, comma-separated)" value={form.envVars} onChange={(v) => setForm((f) => ({ ...f, envVars: v }))} placeholder="NODE_ENV=production, PORT=3000" />

            {sourceMissing ? <p role="alert" className="text-xs text-amber-300">Add the required source for this template type before saving.</p> : null}
            <ModalFooter onCancel={() => setShowModal(false)} onConfirm={save} confirmLabel="Save Template" disabled={!form.name.trim() || sourceMissing} />
          </div>
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
