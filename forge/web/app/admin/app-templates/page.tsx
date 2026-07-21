"use client";

import { useState, useEffect, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, Plus, Pencil, Trash2, Layers,
} from "lucide-react";
import { Btn, Card, CardHeader, Input, SectionHeader, Pill, cn, Modal } from "@/components/admin/admin-ui";
import { type AppType, type AppTemplate, type AppPort } from "@/lib/api/apps";
import { DEFAULT_APP_TEMPLATES, loadUserTemplates, saveUserTemplates, getAllTemplates } from "@/lib/app-templates-data";

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
    gitBranch: tpl.defaultResources.cpu,
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

  const remove = (id: string) => {
    if (!confirm("Delete this template?")) return;
    const users = loadUserTemplates().filter((t) => t.id !== id);
    saveUserTemplates(users);
    refresh();
  };

  const isDefault = (id: string) => DEFAULT_APP_TEMPLATES.some((t) => t.id === id);

  return (
    <div className="mx-auto max-w-5xl space-y-6 px-1 sm:px-0">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <Btn tone="ghost" size="sm" onClick={() => router.push("/admin/apps")}>
            <ArrowLeft size={14} />
          </Btn>
          <SectionHeader title="App Templates" sub="Manage templates for the Create Application wizard" />
        </div>
        <Btn tone="primary" onClick={openCreate}>
          <Plus size={14} />
          New Template
        </Btn>
      </div>

      <Card>
        <CardHeader title={`${templates.length} templates`} icon={Layers} />
        <div className="grid gap-4 p-4 sm:grid-cols-2 lg:grid-cols-3">
          {templates.map((tpl) => (
            <div
              key={tpl.id}
              className={cn(
                "rounded-xl border p-4 text-left",
                isDefault(tpl.id)
                  ? "border-white/[0.08] bg-white/[0.02]"
                  : "border-[#dc2626]/20 bg-[#dc2626]/[0.02]",
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
                      className="rounded p-1 text-slate-500 transition hover:bg-white/[0.06] hover:text-white"
                      onClick={() => openEdit(tpl)}
                    >
                      <Pencil size={12} />
                    </button>
                    <button
                      type="button"
                      className="rounded p-1 text-slate-500 transition hover:bg-white/[0.06] hover:text-red-400"
                      onClick={() => remove(tpl.id)}
                    >
                      <Trash2 size={12} />
                    </button>
                  </div>
                )}
              </div>
              <p className="mt-2 font-semibold text-slate-200 text-sm">{tpl.name}</p>
              <p className="mt-1 text-xs text-slate-500 line-clamp-2">{tpl.description}</p>
              {tpl.image && (
                <p className="mt-2 text-[10px] font-mono text-slate-600 truncate">{tpl.image}</p>
              )}
              {tpl.gitUrl && (
                <p className="mt-2 text-[10px] font-mono text-slate-600 truncate">{tpl.gitUrl}</p>
              )}
              <div className="mt-2 flex flex-wrap gap-1.5">
                <span className="text-[10px] text-slate-600">
                  {tpl.defaultResources.cpu} CPU / {tpl.defaultResources.memory} MiB
                </span>
              </div>
            </div>
          ))}
          {templates.length === 0 && (
            <div className="col-span-full py-12 text-center text-sm text-slate-500">
              No templates yet. Create your first template to get started.
            </div>
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
              <div className="sm:col-span-2">
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Description</label>
                <input
                  className="h-10 w-full rounded-lg border border-white/10 bg-[#0d131d] px-3.5 text-sm text-slate-100 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
                  value={form.description}
                  onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                  placeholder="Brief description of this template"
                />
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Type</label>
                <select
                  className="h-10 w-full rounded-lg border border-white/10 bg-[#0d131d] px-3 text-sm text-slate-100 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
                  value={form.type}
                  onChange={(e) => setForm((f) => ({ ...f, type: e.target.value as AppType }))}
                >
                  <option value="image">Docker Image</option>
                  <option value="git">Git Repository</option>
                  <option value="compose">Docker Compose</option>
                </select>
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
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Compose YAML</label>
                <textarea
                  className="h-40 w-full rounded-lg border border-white/10 bg-[#0d131d] p-3 font-mono text-xs text-slate-100 outline-none resize-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
                  value={form.composeContent}
                  onChange={(e) => setForm((f) => ({ ...f, composeContent: e.target.value }))}
                  placeholder={`services:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"`}
                />
              </div>
            )}

            <div className="grid gap-4 sm:grid-cols-3">
              <Input label="CPU (cores)" value={form.cpu} onChange={(v) => setForm((f) => ({ ...f, cpu: v }))} placeholder="1.0" />
              <Input label="Memory (MiB)" value={form.memory} onChange={(v) => setForm((f) => ({ ...f, memory: v }))} placeholder="512" />
              <Input label="Disk (MiB)" value={form.disk} onChange={(v) => setForm((f) => ({ ...f, disk: v }))} placeholder="1024" />
            </div>

            <Input label="Ports (host:container, comma-separated)" value={form.ports} onChange={(v) => setForm((f) => ({ ...f, ports: v }))} placeholder="8080:80, 3000:3000" />
            <Input label="Env Vars (KEY=value, comma-separated)" value={form.envVars} onChange={(v) => setForm((f) => ({ ...f, envVars: v }))} placeholder="NODE_ENV=production, PORT=3000" />

            <div className="flex justify-end gap-2 border-t border-white/[0.06] pt-4">
              <Btn tone="ghost" onClick={() => setShowModal(false)}>Cancel</Btn>
              <Btn tone="primary" onClick={save} disabled={!form.name.trim()}>Save Template</Btn>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
