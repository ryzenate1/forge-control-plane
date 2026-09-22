"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { Boxes, Pencil, Plus, Rocket, Trash2, Eye, X } from "lucide-react";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  Btn,
  Card,
  EmptyState,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { ForgeDialog } from "@/components/ui/forge/overlay";
import { useNodesQuery } from "@/lib/admin/telemetry";
import {
  createComposeTemplate,
  deleteComposeTemplate,
  instantiateComposeTemplate,
  listComposeTemplates,
  previewComposeTemplate,
  updateComposeTemplate,
  type ComposeTemplate,
  type ComposeTemplateInstantiateResult,
  type ComposeTemplateParameter,
} from "@/lib/api/compose-templates";

const PARAM_TYPES = [
  { value: "text", label: "Text" },
  { value: "number", label: "Number" },
  { value: "select", label: "Select" },
  { value: "password", label: "Password" },
  { value: "env-file", label: "Env file" },
];

const CONTROL_CLASS =
  "w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-slate-200 placeholder:text-slate-400 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]";

type Draft = {
  name: string;
  description: string;
  category: string;
  logoUrl: string;
  composeYaml: string;
  visibility: "private" | "public";
  parameters: ComposeTemplateParameter[];
};

const emptyDraft: Draft = {
  name: "",
  description: "",
  category: "",
  logoUrl: "",
  composeYaml: "",
  visibility: "private",
  parameters: [],
};

function draftFromTemplate(t: ComposeTemplate): Draft {
  return {
    name: t.name ?? "",
    description: t.description ?? "",
    category: t.category ?? "",
    logoUrl: t.logoUrl ?? "",
    composeYaml: t.composeYaml ?? "",
    visibility: t.visibility === "public" ? "public" : "private",
    parameters: Array.isArray(t.parameters) ? t.parameters.map((p) => ({ ...p, options: p.options ? [...p.options] : undefined })) : [],
  };
}

function fieldLabelFor(type: string): string {
  return type === "env-file" ? "path" : type;
}

export default function ComposeTemplatesPage() {
  const router = useRouter();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();

  const templatesQ = useQuery<ComposeTemplate[]>({
    queryKey: ["compose-templates"],
    queryFn: () => listComposeTemplates(),
  });
  const nodesQ = useNodesQuery();
  const safeTemplates = useMemo(() => (Array.isArray(templatesQ.data) ? templatesQ.data : []), [templatesQ.data]);
  const safeNodes = useMemo(() => (Array.isArray(nodesQ.data) ? nodesQ.data : []), [nodesQ.data]);

  // ---- create / edit modal ----
  const [editing, setEditing] = useState<ComposeTemplate | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [previewYaml, setPreviewYaml] = useState<string | null>(null);

  function openCreate() {
    setEditing(null);
    setDraft(emptyDraft);
    setPreviewYaml(null);
    setModalOpen(true);
  }

  function openEdit(t: ComposeTemplate) {
    setEditing(t);
    setDraft(draftFromTemplate(t));
    setPreviewYaml(null);
    setModalOpen(true);
  }

  function updateParameter(index: number, patch: Partial<ComposeTemplateParameter>) {
    setDraft((d) => {
      const next = d.parameters.map((p, i) => (i === index ? { ...p, ...patch } : p));
      return { ...d, parameters: next };
    });
  }

  function addParameter() {
    setDraft((d) => ({
      ...d,
      parameters: [...d.parameters, { key: "", label: "", type: "text", required: false, default: "", options: [] }],
    }));
  }

  function removeParameter(index: number) {
    setDraft((d) => ({ ...d, parameters: d.parameters.filter((_, i) => i !== index) }));
  }

  const saveMutation = useMutation({
    mutationFn: () =>
      editing
        ? updateComposeTemplate(editing.id, {
            name: draft.name,
            description: draft.description,
            category: draft.category,
            logoUrl: draft.logoUrl,
            composeYaml: draft.composeYaml,
            visibility: draft.visibility,
            parameters: draft.parameters,
          })
        : createComposeTemplate({
            name: draft.name,
            description: draft.description,
            category: draft.category,
            logoUrl: draft.logoUrl,
            composeYaml: draft.composeYaml,
            visibility: draft.visibility,
            parameters: draft.parameters,
          }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-templates"] });
      toast({ tone: "success", title: editing ? "Template updated" : "Template created" });
      setModalOpen(false);
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  const previewMutation = useMutation({
    mutationFn: () =>
      previewComposeTemplate({
        templateId: editing?.id,
        composeYaml: editing ? undefined : draft.composeYaml,
        parameters: editing ? undefined : draft.parameters,
      }),
    onSuccess: (data) => setPreviewYaml(data?.composeYaml ?? ""),
    onError: (e: Error) => toast({ tone: "error", title: "Preview failed", message: e.message }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteComposeTemplate(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-templates"] });
      toast({ tone: "success", title: "Template deleted" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  async function handleDelete(t: ComposeTemplate) {
    const ok = await confirm({
      title: "Delete this template?",
      description: `"${t.name}" will be removed. Stacks already deployed from it are unaffected.`,
      danger: true,
      confirmLabel: "Delete",
    });
    if (ok) deleteMutation.mutate(t.id);
  }

  // ---- deploy (instantiate) modal ----
  const [deploying, setDeploying] = useState<ComposeTemplate | null>(null);
  const [deployName, setDeployName] = useState("");
  const [deployNodeId, setDeployNodeId] = useState("");
  const [deployValues, setDeployValues] = useState<Record<string, string>>({});

  function openDeploy(t: ComposeTemplate) {
    setDeploying(t);
    setDeployName("");
    setDeployNodeId("");
    const values: Record<string, string> = {};
    for (const p of t.parameters ?? []) {
      values[p.key] = p.default ?? "";
    }
    setDeployValues(values);
  }

  const instantiateMutation = useMutation({
    mutationFn: () =>
      instantiateComposeTemplate(deploying!.id, {
        name: deployName,
        nodeId: deployNodeId || undefined,
        values: deployValues,
      }),
    onSuccess: (result: ComposeTemplateInstantiateResult) => {
      queryClient.invalidateQueries({ queryKey: ["compose-templates"] });
      queryClient.invalidateQueries({ queryKey: ["compose-stacks"] });
      toast({ tone: "success", title: "Stack deployed from template" });
      setDeploying(null);
      const stackId = (result?.stack as { id?: string } | undefined)?.id;
      if (stackId) router.push(`/admin/compose/${stackId}`);
    },
    onError: (e: Error) => toast({ tone: "error", title: "Deploy failed", message: e.message }),
  });

  const canSave = draft.name.trim() !== "" && draft.composeYaml.trim() !== "";
  const canDeploy = deploying !== null && deployName.trim() !== "";

  return (
    <AdminPageLayout>
      <AdminPageHeader
        title="Stack Templates"
        description="Reusable, parameterized compose templates that instantiate into deployable stacks."
        action={
          <Btn tone="primary" onClick={openCreate}>
            <Plus className="h-4 w-4" /> New template
          </Btn>
        }
      />
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.015] px-4 py-2 text-xs leading-5 text-slate-400">
        <span className="font-semibold text-slate-300">DEPLOY</span> · Templates store a compose document with <code className="font-mono text-[11px]">{"${PARAM_KEY}"}</code> placeholders plus parameter fields (text / number / select / password / env-file). Deploying renders the YAML with your values and creates a stack via <code className="font-mono">/compose</code>. Manage the resulting stacks under <button type="button" onClick={() => router.push("/admin/compose")} className="underline hover:text-slate-200">Compose Stacks</button>.
      </div>

      {templatesQ.isLoading ? (
        <AdminLoadingState label="Loading stack templates…" />
      ) : templatesQ.isError ? (
        <AdminErrorState message={(templatesQ.error as Error)?.message ?? "Failed to load templates"} retry={() => void templatesQ.refetch()} />
      ) : safeTemplates.length === 0 ? (
        <Card className="p-8">
          <EmptyState title="No stack templates" message="Create a reusable, parameterized compose template to get started." />
          <div className="mt-4 flex justify-center">
            <Btn tone="primary" onClick={openCreate}>
              <Plus className="h-4 w-4" /> Create template
            </Btn>
          </div>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {safeTemplates.map((t) => (
            <Card key={t.id} className="flex flex-col p-4">
              <div className="flex items-start gap-3">
                {t.logoUrl ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={t.logoUrl} alt={`${t.name} logo`} className="h-10 w-10 shrink-0 rounded-lg bg-white/[0.04] object-contain" />
                ) : (
                  <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-white/[0.04]">
                    <Boxes className="h-5 w-5 text-slate-400" />
                  </div>
                )}
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-slate-100">{t.name}</p>
                  <p className="truncate text-xs text-slate-400">{t.category || "Uncategorized"}</p>
                </div>
                <Pill tone={t.visibility === "public" ? "blue" : "neutral"} className="ml-auto shrink-0">{t.visibility}</Pill>
              </div>
              <p className="mt-3 line-clamp-3 min-h-[2.5rem] text-xs leading-5 text-slate-400">{t.description || "No description."}</p>
              <div className="mt-2 text-[11px] text-slate-500">
                {(t.parameters?.length ?? 0) > 0 ? `${t.parameters.length} parameter(s)` : "No parameters"}
              </div>
              <div className="mt-4 flex items-center gap-2">
                <Btn size="sm" tone="primary" onClick={() => openDeploy(t)}>
                  <Rocket className="h-4 w-4" /> Deploy
                </Btn>
                <Btn size="sm" tone="ghost" onClick={() => openEdit(t)} ariaLabel="Edit">
                  <Pencil className="h-4 w-4" />
                </Btn>
                <Btn size="sm" tone="danger" onClick={() => void handleDelete(t)} ariaLabel="Delete">
                  <Trash2 className="h-4 w-4" />
                </Btn>
              </div>
            </Card>
          ))}
        </div>
      )}

      {/* Create / edit modal */}
      <ForgeDialog
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editing ? "Edit template" : "New template"}
        description="Define the compose document and the parameters operators fill in on deploy."
        size="lg"
        footer={
          <>
            <Btn tone="ghost" onClick={() => setModalOpen(false)}>Cancel</Btn>
            <Btn tone="primary" onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending || !canSave}>
              {saveMutation.isPending ? "Saving…" : editing ? "Save changes" : "Create template"}
            </Btn>
          </>
        }
      >
          <div className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <Input label="Name" value={draft.name} onChange={(v) => setDraft((d) => ({ ...d, name: v }))} placeholder="PostgreSQL + PgBouncer" />
              <Input label="Category" value={draft.category} onChange={(v) => setDraft((d) => ({ ...d, category: v }))} placeholder="Databases" />
            </div>
            <Input label="Description" value={draft.description} onChange={(v) => setDraft((d) => ({ ...d, description: v }))} placeholder="What this stack does" />
            <div className="grid gap-3 sm:grid-cols-2">
              <Input label="Logo URL" value={draft.logoUrl} onChange={(v) => setDraft((d) => ({ ...d, logoUrl: v }))} placeholder="https://…/icon.svg" />
              <label className="block text-sm font-medium text-slate-300">
                <span className="mb-1.5 block">Visibility</span>
                <select value={draft.visibility} onChange={(e) => setDraft((d) => ({ ...d, visibility: e.target.value === "public" ? "public" : "private" }))} className={CONTROL_CLASS}>
                  <option value="private">Private</option>
                  <option value="public">Public</option>
                </select>
              </label>
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">Compose YAML</label>
              <textarea
                value={draft.composeYaml}
                onChange={(e) => setDraft((d) => ({ ...d, composeYaml: e.target.value }))}
                rows={10}
                spellCheck={false}
                placeholder={"services:\n  web:\n    image: ${IMAGE}\n    ports:\n      - \"${HOST_PORT}:80\""}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-3 font-mono text-xs text-slate-200 placeholder:text-slate-500 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
              />
              <p className="mt-1 text-[11px] text-slate-500">Use {"${PARAM_KEY}"} placeholders; unknown variables are left intact at deploy time.</p>
            </div>

            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-sm font-medium text-slate-300">Parameters</span>
                <Btn size="sm" tone="subtle" onClick={addParameter}>
                  <Plus className="h-4 w-4" /> Add
                </Btn>
              </div>
              {draft.parameters.length === 0 ? (
                <p className="text-xs text-slate-500">No parameters defined.</p>
              ) : (
                <div className="space-y-3">
                  {draft.parameters.map((p, i) => (
                    <div key={i} className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
                      <div className="grid gap-2 sm:grid-cols-2">
                        <input value={p.key} onChange={(e) => updateParameter(i, { key: e.target.value })} placeholder="KEY (e.g. HOST_PORT)" className={CONTROL_CLASS} />
                        <input value={p.label ?? ""} onChange={(e) => updateParameter(i, { label: e.target.value })} placeholder="Label" className={CONTROL_CLASS} />
                      </div>
                      <div className="mt-2 grid gap-2 sm:grid-cols-2">
                        <select value={p.type} onChange={(e) => updateParameter(i, { type: e.target.value })} className={CONTROL_CLASS}>
                          {PARAM_TYPES.map((t) => (
                            <option key={t.value} value={t.value}>{t.label}</option>
                          ))}
                        </select>
                        <input value={p.default ?? ""} onChange={(e) => updateParameter(i, { default: e.target.value })} placeholder={`Default ${fieldLabelFor(p.type)}`} className={CONTROL_CLASS} />
                      </div>
                      {p.type === "select" && (
                        <input
                          value={(p.options ?? []).join(", ")}
                          onChange={(e) => updateParameter(i, { options: e.target.value.split(",").map((o) => o.trim()).filter(Boolean) })}
                          placeholder="Options (comma, separated)"
                          className={`${CONTROL_CLASS} mt-2`}
                        />
                      )}
                      <div className="mt-2 flex items-center gap-4 text-xs text-slate-400">
                        <label className="flex items-center gap-1.5">
                          <input type="checkbox" checked={p.required} onChange={(e) => updateParameter(i, { required: e.target.checked })} className="accent-[var(--brand)]" /> Required
                        </label>
                        <label className="flex items-center gap-1.5">
                          <input type="checkbox" checked={!!p.secret} onChange={(e) => updateParameter(i, { secret: e.target.checked })} className="accent-[var(--brand)]" /> Secret
                        </label>
                        <button type="button" onClick={() => removeParameter(i)} className="ml-auto inline-flex items-center gap-1 text-red-400 hover:text-red-300">
                          <X className="h-3.5 w-3.5" /> Remove
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="flex items-center gap-2">
              <Btn size="sm" tone="ghost" onClick={() => previewMutation.mutate()} disabled={previewMutation.isPending || !draft.composeYaml.trim()}>
                <Eye className="h-4 w-4" /> {previewMutation.isPending ? "Rendering…" : "Preview render"}
              </Btn>
            </div>
            {previewYaml !== null && (
              <div>
                <span className="mb-1 block text-xs font-medium text-slate-400">Rendered (defaults applied)</span>
                <pre className="max-h-64 overflow-auto rounded-lg border border-white/[0.06] bg-black/30 p-3 font-mono text-[11px] leading-5 text-slate-300">{previewYaml || "(empty)"}</pre>
              </div>
            )}
          </div>
      </ForgeDialog>

      {/* Deploy (instantiate) modal */}
      <ForgeDialog
        open={deploying !== null}
        onClose={() => setDeploying(null)}
        title={`Deploy “${deploying?.name ?? ""}”`}
        description="Fill in the template parameters, then create a new stack."
        size="md"
        footer={
          <>
            <Btn tone="ghost" onClick={() => setDeploying(null)}>Cancel</Btn>
            <Btn tone="primary" onClick={() => instantiateMutation.mutate()} disabled={instantiateMutation.isPending || !canDeploy}>
              <Rocket className="h-4 w-4" /> {instantiateMutation.isPending ? "Deploying…" : "Deploy stack"}
            </Btn>
          </>
        }
      >
          <div className="space-y-4">
            <Input label="Stack name" value={deployName} onChange={setDeployName} placeholder="my-postgres-prod" />
            <label className="block text-sm font-medium text-slate-300">
              <span className="mb-1.5 block">Target node</span>
              <select value={deployNodeId} onChange={(e) => setDeployNodeId(e.target.value)} className={CONTROL_CLASS}>
                <option value="">Auto-select</option>
                {safeNodes.map((n: { id: string; name: string }) => (
                  <option key={n.id} value={n.id}>{n.name || n.id.slice(0, 8)}</option>
                ))}
              </select>
            </label>
            {(deploying?.parameters ?? []).length > 0 && (
              <div className="space-y-3">
                <span className="text-sm font-medium text-slate-300">Parameters</span>
                {deploying!.parameters.map((p) => (
                  <div key={p.key}>
                    <label className="mb-1 block text-xs font-medium text-slate-400">
                      {p.label || p.key}
                      {p.required ? <span className="text-red-400"> *</span> : null}
                      {p.type === "env-file" ? <span className="ml-1 text-slate-500">(env file path)</span> : null}
                    </label>
                    {p.type === "select" ? (
                      <select
                        value={deployValues[p.key] ?? ""}
                        onChange={(e) => setDeployValues((v) => ({ ...v, [p.key]: e.target.value }))}
                        className={CONTROL_CLASS}
                      >
                        <option value="">—</option>
                        {(p.options ?? []).map((opt) => (
                          <option key={opt} value={opt}>{opt}</option>
                        ))}
                      </select>
                    ) : (
                      <input
                        type={p.type === "password" ? "password" : p.type === "number" ? "number" : "text"}
                        value={deployValues[p.key] ?? ""}
                        onChange={(e) => setDeployValues((v) => ({ ...v, [p.key]: e.target.value }))}
                        placeholder={p.default || ""}
                        className={CONTROL_CLASS}
                      />
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
      </ForgeDialog>

      {renderConfirm()}
    </AdminPageLayout>
  );
}
