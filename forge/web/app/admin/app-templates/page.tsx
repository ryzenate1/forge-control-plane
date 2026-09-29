"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Plus, Pencil, Trash2, FileCode, Terminal, Container,
} from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminSection,
  Btn,
  Card,
  EmptyState,
  AdminIconButton,
  AdminPageLayout,
  AdminSelect,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  Textarea,
  cn,
} from "@/components/admin/admin-ui";
import { fetchAppTemplates, typeLabel, type AppPort, type AppTemplate, type AppType } from "@/lib/api/apps";
import {
  DEFAULT_APP_TEMPLATES,
  readStoredTemplates,
  saveUserTemplates,
  type StoredTemplatesResult,
} from "@/lib/app-templates-data";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";

type FormData = {
  name: string;
  description: string;
  type: AppType;
  image: string;
  gitUrl: string;
  composeContent: string;
  ports: string;
  envVars: string;
  cpu: string;
  memory: string;
  disk: string;
};

const emptyForm: FormData = {
  name: "", description: "", type: "image", image: "", gitUrl: "",
  composeContent: "", ports: "", envVars: "",
  cpu: "1.0", memory: "512", disk: "1024",
};

function generateId(): string {
  return "tpl_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2, 6);
}

/** Ports are entered as `host:container` (or a bare port). A malformed entry is
 * reported, never silently rewritten: the old `parseInt(x) || 8080` turned a typo
 * into a plausible-but-wrong port that the operator then deployed. */
function parsePorts(text: string): { ports: AppPort[]; error: string | null } {
  const ports: AppPort[] = [];
  for (const rawEntry of text.split(",")) {
    const entry = rawEntry.trim();
    if (!entry) continue;
    const parts = entry.split(":").map((p) => p.trim());
    const invalid = parts.some((p) => p === "" || !Number.isInteger(Number(p)) || Number(p) < 1 || Number(p) > 65535);
    if (invalid || parts.length > 2) {
      return { ports: [], error: `“${entry}” is not a valid port or host:container pair (1-65535).` };
    }
    const hostPort = Number(parts[0]);
    const containerPort = parts.length === 2 ? Number(parts[1]) : hostPort;
    ports.push({ hostPort, containerPort, protocol: "tcp" });
  }
  return { ports, error: null };
}

function parseEnvVars(text: string): { envVars: Record<string, string>; error: string | null } {
  const envVars: Record<string, string> = {};
  for (const rawEntry of text.split(",")) {
    const entry = rawEntry.trim();
    if (!entry) continue;
    const eqIdx = entry.indexOf("=");
    const key = (eqIdx > 0 ? entry.slice(0, eqIdx) : entry).trim();
    if (!key) return { envVars: {}, error: `“${entry}” has no variable name.` };
    envVars[key] = eqIdx > 0 ? entry.slice(eqIdx + 1).trim() : "";
  }
  return { envVars, error: null };
}

function formToTemplate(form: FormData, id?: string): AppTemplate {
  return {
    id: id ?? generateId(),
    name: form.name.trim(),
    description: form.description.trim(),
    type: form.type,
    image: form.type === "image" ? form.image.trim() : undefined,
    gitUrl: form.type === "git" ? form.gitUrl.trim() : undefined,
    composeContent: form.type === "compose" ? form.composeContent : undefined,
    defaultPorts: parsePorts(form.ports).ports,
    defaultEnvVars: parseEnvVars(form.envVars).envVars,
    defaultResources: { cpu: form.cpu.trim() || "1", memory: form.memory.trim() || "512", disk: form.disk.trim() || "1024" },
  };
}

function templateToForm(tpl: AppTemplate): FormData {
  return {
    name: tpl.name ?? "",
    description: tpl.description ?? "",
    type: tpl.type,
    image: tpl.image ?? "",
    gitUrl: tpl.gitUrl ?? "",
    composeContent: tpl.composeContent ?? "",
    ports: (tpl.defaultPorts ?? []).map((p) => `${p.hostPort}:${p.containerPort}`).join(", "),
    envVars: Object.entries(tpl.defaultEnvVars ?? {}).map(([k, v]) => `${k}=${v}`).join(", "),
    cpu: tpl.defaultResources?.cpu ?? "",
    memory: tpl.defaultResources?.memory ?? "",
    disk: tpl.defaultResources?.disk ?? "",
  };
}

function messageOf(err: unknown): string {
  return err instanceof Error && err.message ? err.message : "The request could not be completed.";
}

export default function AppTemplatesPage() {
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  const catalogQuery = useQuery({ queryKey: ["app-templates"], queryFn: fetchAppTemplates });

  // `null` means "not read yet" — rendering an empty browser list before the
  // read would claim the operator has no saved templates.
  const [stored, setStored] = useState<StoredTemplatesResult | null>(null);
  useEffect(() => { setStored(readStoredTemplates()); }, []);

  const [showModal, setShowModal] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<FormData>(emptyForm);
  const [formError, setFormError] = useState<string | null>(null);

  const browserTemplates = useMemo(() => stored?.templates ?? [], [stored]);
  const browserIds = useMemo(() => new Set(browserTemplates.map((t) => t.id)), [browserTemplates]);

  const catalog = useMemo(() => {
    const rows = Array.isArray(catalogQuery.data) ? catalogQuery.data : [];
    // The API is authoritative; the client falls back to this browser's store
    // only when the request cannot reach the API, so drop anything that is
    // really a browser-local entry rather than showing it twice.
    return rows.filter((t) => !browserIds.has(t.id));
  }, [catalogQuery.data, browserIds]);

  const isBundled = (id: string) => DEFAULT_APP_TEMPLATES.some((t) => t.id === id);

  const openCreate = () => {
    setEditingId(null);
    setForm(emptyForm);
    setFormError(null);
    setShowModal(true);
  };

  const openEdit = (tpl: AppTemplate) => {
    setEditingId(tpl.id);
    setForm(templateToForm(tpl));
    setFormError(null);
    setShowModal(true);
  };

  const save = () => {
    const ports = parsePorts(form.ports);
    if (ports.error) { setFormError(ports.error); return; }
    const env = parseEnvVars(form.envVars);
    if (env.error) { setFormError(env.error); return; }
    if (!form.name.trim()) { setFormError("A name is required."); return; }
    if (form.type === "image" && !form.image.trim()) { setFormError("A Docker image is required for an image template."); return; }
    if (form.type === "git" && !form.gitUrl.trim()) { setFormError("A Git repository URL is required for a Git template."); return; }
    if (form.type === "compose" && !form.composeContent.trim()) { setFormError("Compose YAML is required for a Compose template."); return; }

    const previous = browserTemplates.filter((t) => t.id !== editingId);
    const next = [...previous, formToTemplate(form, editingId ?? undefined)];
    try {
      saveUserTemplates(next);
    } catch (e) {
      // Quota exceeded / private-mode storage failures must be visible: the
      // dialog staying open with no message was how a save was lost before.
      setFormError(`Could not save in this browser: ${messageOf(e)}`);
      return;
    }
    setStored(readStoredTemplates());
    setShowModal(false);
    toast({ tone: "success", title: editingId ? "Template updated in this browser" : "Template saved in this browser" });
  };

  const remove = async (tpl: AppTemplate) => {
    const ok = await confirm({
      title: `Delete “${tpl.name}”?`,
      description: "This removes the template from this browser only. Applications already created from it are untouched.",
      danger: true,
      confirmLabel: "Delete",
    });
    if (!ok) return;
    try {
      saveUserTemplates(browserTemplates.filter((t) => t.id !== tpl.id));
      setStored(readStoredTemplates());
      toast({ tone: "success", title: "Template removed from this browser" });
    } catch (e) {
      toast({ tone: "error", title: "Could not delete", message: messageOf(e) });
    }
  };

  return (
    <AdminPageLayout>
      <SectionHeader info={adminPageGuides.appTemplates} action={<Btn tone="primary" onClick={openCreate}><Plus size={14} /> New browser template</Btn>} />

      <AdminSection
        title="Deployment catalog"
        description="Served by the API at GET /admin/app-templates. This is what the Create Application wizard offers, and it is read-only here — the API exposes no write route for it."
      >
        <Card>
          {catalogQuery.isLoading ? (
            <AdminLoadingRows rows={3} label="Loading the deployment catalog…" />
          ) : catalogQuery.isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`The deployment catalog could not be loaded: ${messageOf(catalogQuery.error)}`}
                retry={() => void catalogQuery.refetch()}
              />
            </div>
          ) : catalog.length === 0 ? (
            <EmptyState
              icon={FileCode}
              title="Catalog returned no templates"
              message="The request succeeded but the catalog is empty, so the wizard has nothing to offer."
            />
          ) : (
            <div className="grid gap-4 p-4 sm:grid-cols-2 lg:grid-cols-3">
              {catalog.map((tpl) => (
                <TemplateCard key={tpl.id} tpl={tpl} badge={<Pill tone={isBundled(tpl.id) ? "blue" : "neutral"}>{isBundled(tpl.id) ? "Bundled" : "Server"}</Pill>} />
              ))}
            </div>
          )}
        </Card>
      </AdminSection>

      <AdminSection
        title="Saved in this browser"
        description="Templates created here are stored in this browser's localStorage and are not a shared catalog. Other administrators and other browsers never see them, and the Create Application wizard only reads them when the API cannot be reached."
      >
        {stored?.error ? (
          <div className="ui-alert ui-alert-warning" role="alert">
            <span>
              {stored.error} Saved templates may be missing from this list; nothing has been overwritten.
            </span>
          </div>
        ) : null}
        <Card>
          {stored === null ? (
            <AdminLoadingRows rows={2} label="Reading templates saved in this browser…" />
          ) : stored.templates.length === 0 ? (
            <EmptyState
              icon={FileCode}
              title="No browser-local templates"
              message="Create one with “New browser template”. It stays on this machine."
            />
          ) : (
            <div className="grid gap-4 p-4 sm:grid-cols-2 lg:grid-cols-3">
              {stored.templates.map((tpl) => (
                <TemplateCard
                  key={tpl.id}
                  tpl={tpl}
                  badge={<Pill tone="yellow">This browser</Pill>}
                  actions={
                    <div className="flex gap-1.5">
                      <AdminIconButton label={`Edit ${tpl.name}`} onClick={() => openEdit(tpl)}>
                        <Pencil size={12} />
                      </AdminIconButton>
                      <AdminIconButton label={`Delete ${tpl.name}`} tone="danger" onClick={() => void remove(tpl)}>
                        <Trash2 size={12} />
                      </AdminIconButton>
                    </div>
                  }
                />
              ))}
            </div>
          )}
        </Card>
      </AdminSection>

      {showModal && (
        <Modal
          title={editingId ? "Edit browser template" : "New browser template"}
          description="Saved in this browser only. It does not become part of the shared catalog."
          onClose={() => setShowModal(false)}
          wide
        >
          <div className="space-y-4">
            <Input label="Name" value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} placeholder="My template" required />
            <Textarea label="Description" value={form.description} onChange={(v) => setForm((f) => ({ ...f, description: v }))} rows={2} placeholder="Brief description of this template" />
            <AdminSelect
              label="Type"
              value={form.type}
              onChange={(v) => setForm((f) => ({ ...f, type: v as AppType }))}
              options={[
                { value: "image", label: "Docker image" },
                { value: "git", label: "Git repository" },
                { value: "compose", label: "Docker Compose" },
              ]}
            />

            {form.type === "image" && (
              <Input label="Docker image" value={form.image} onChange={(v) => setForm((f) => ({ ...f, image: v }))} placeholder="nginx:1.27-alpine" required />
            )}
            {form.type === "git" && (
              <Input label="Git repository URL" value={form.gitUrl} onChange={(v) => setForm((f) => ({ ...f, gitUrl: v }))} placeholder="https://github.com/user/repo.git" required />
            )}
            {form.type === "compose" && (
              <Textarea label="Compose YAML" value={form.composeContent} onChange={(v) => setForm((f) => ({ ...f, composeContent: v }))} rows={8} placeholder={"services:\n  web:\n    image: nginx:latest"} />
            )}

            <div className="grid gap-4 sm:grid-cols-3">
              <Input label="CPU (cores)" value={form.cpu} onChange={(v) => setForm((f) => ({ ...f, cpu: v }))} placeholder="1.0" />
              <Input label="Memory (MiB)" value={form.memory} onChange={(v) => setForm((f) => ({ ...f, memory: v }))} placeholder="512" />
              <Input label="Disk (MiB)" value={form.disk} onChange={(v) => setForm((f) => ({ ...f, disk: v }))} placeholder="1024" />
            </div>

            <Input label="Ports (host:container, comma-separated)" value={form.ports} onChange={(v) => setForm((f) => ({ ...f, ports: v }))} placeholder="8080:80, 3000:3000" />
            <Input label="Env vars (KEY=value, comma-separated)" value={form.envVars} onChange={(v) => setForm((f) => ({ ...f, envVars: v }))} placeholder="NODE_ENV=production, PORT=3000" />

            {formError ? (
              <div className="ui-alert ui-alert-danger" role="alert"><span>{formError}</span></div>
            ) : null}

            <ModalFooter onCancel={() => setShowModal(false)} onConfirm={save} confirmLabel={editingId ? "Save changes" : "Save in this browser"} disabled={!form.name.trim()} />
          </div>
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

function TemplateCard({ tpl, badge, actions }: { tpl: AppTemplate; badge: ReactNode; actions?: ReactNode }) {
  const ports = tpl.defaultPorts ?? [];
  return (
    <div className={cn("rounded-xl border p-4 text-left", "border-line bg-overlay-subtle")}>
      <div className="flex items-start justify-between gap-2">
        <p className="min-w-0 font-semibold text-sm text-text">{tpl.name}</p>
        {badge}
      </div>
      {tpl.description ? <p className="mt-1 text-xs leading-5 text-text-subtle line-clamp-2">{tpl.description}</p> : null}
      <p className="mt-2"><Pill tone="neutral">{typeLabel(tpl.type)}</Pill></p>
      {tpl.image ? (
        <p className="mt-2 flex items-start gap-1.5 font-mono text-xs text-text-subtle">
          <Container size={12} className="mt-0.5 shrink-0" />
          <span className="break-all">{tpl.image}</span>
        </p>
      ) : null}
      {tpl.gitUrl ? (
        <p className="mt-2 flex items-start gap-1.5 font-mono text-xs text-text-subtle">
          <Terminal size={12} className="mt-0.5 shrink-0" />
          <span className="break-all">{tpl.gitUrl}</span>
        </p>
      ) : null}
      <p className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-text-subtle">
        <span>{tpl.defaultResources?.cpu || "—"} cores</span>
        <span>{tpl.defaultResources?.memory || "—"} MiB memory</span>
        <span>{tpl.defaultResources?.disk || "—"} MiB disk</span>
      </p>
      <p className="mt-1 font-mono text-xs text-text-muted">
        {ports.length > 0 ? ports.map((p) => `${p.hostPort}:${p.containerPort}`).join(", ") : "No ports declared"}
      </p>
      {actions ? <div className="mt-3 flex justify-end gap-1.5">{actions}</div> : null}
    </div>
  );
}
