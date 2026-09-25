"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, Plus, Save, Tags as TagsIcon, Trash2 } from "lucide-react";
import {
  createTag,
  deleteTag,
  fetchTags,
  updateTag,
  type CreateTagInput,
  type Tag,
  type UpdateTagInput,
} from "@/lib/api/tags";
import { TagBadge, sanitizeTagColor } from "@/components/ui/tag-badge";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
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
} from "./admin-ui";

// A curated palette offered as one-click swatches. These are seed *data* values
// for a tag's color field (they end up rendered through TagBadge's sanitized
// inline style), not hardcoded CSS classes.
const PALETTE = [
  "#ef4444", "#f97316", "#f59e0b", "#84cc16", "#22c55e",
  "#14b8a6", "#06b6d4", "#3b82f6", "#6366f1", "#8b5cf6",
  "#a855f7", "#d946ef", "#ec4899", "#64748b",
];

const DEFAULT_COLOR = "#6366f1";

type FormState = { name: string; color: string; description: string };

const emptyForm: FormState = { name: "", color: DEFAULT_COLOR, description: "" };

function validate(form: FormState): { name?: string; color?: string } {
  const errors: { name?: string; color?: string } = {};
  if (!form.name.trim()) errors.name = "Name is required";
  else if (form.name.trim().length > 64) errors.name = "Keep the name under 64 characters";
  if (!sanitizeTagColor(form.color)) errors.color = "Pick a valid color";
  return errors;
}

export function TagsManager() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  const tagsQuery = useQuery({ queryKey: ["tags"], queryFn: fetchTags });
  const tags = useMemo(() => tagsQuery.data ?? [], [tagsQuery.data]);

  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<Tag | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [errors, setErrors] = useState<{ name?: string; color?: string }>({});

  const invalidate = () => qc.invalidateQueries({ queryKey: ["tags"] });

  const openCreate = () => {
    setEditing(null);
    setForm(emptyForm);
    setErrors({});
    setShowForm(true);
  };

  const openEdit = (tag: Tag) => {
    setEditing(tag);
    setForm({ name: tag.name, color: tag.color, description: tag.description ?? "" });
    setErrors({});
    setShowForm(true);
  };

  const saveMut = useMutation({
    mutationFn: (input: CreateTagInput | UpdateTagInput) =>
      editing ? updateTag(editing.id, input as UpdateTagInput) : createTag(input as CreateTagInput),
    onSuccess: () => {
      invalidate();
      setShowForm(false);
      toast({ tone: "success", title: editing ? "Tag updated" : "Tag created" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteTag(id),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Tag deleted" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  const handleSubmit = () => {
    const nextErrors = validate(form);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;
    const payload = {
      name: form.name.trim(),
      color: (sanitizeTagColor(form.color) ?? DEFAULT_COLOR),
      description: form.description.trim(),
    };
    saveMut.mutate(payload);
  };

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Tags"
        sub="Color-coded labels you can attach to applications, servers, and environments to filter the dashboard and run bulk actions such as “restart every server tagged maintenance-window”."
        action={
          <Btn tone="primary" onClick={openCreate}>
            <Plus size={14} /> New Tag
          </Btn>
        }
      />

      <Card>
        <CardHeader title="Tag Catalog" icon={TagsIcon} />
        {tagsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading…</div>
        ) : tagsQuery.isError ? (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load tags: {(tagsQuery.error as Error).message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void tagsQuery.refetch()}>
                Retry
              </Btn>
            </div>
          </div>
        ) : tags.length === 0 ? (
          <EmptyState
            icon={TagsIcon}
            title="No tags yet"
            message="Create your first tag to start grouping and filtering resources."
          />
        ) : (
          <AdminTable label="Tags">
            <AdminTHead>
              <AdminTh>Tag</AdminTh>
              <AdminTh>Description</AdminTh>
              <AdminTh>Created</AdminTh>
              <AdminTh></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {tags.map((tag) => (
                <AdminTr key={tag.id}>
                  <AdminTd>
                    <TagBadge name={tag.name} color={tag.color} description={tag.description} />
                  </AdminTd>
                  <AdminTd className="max-w-md truncate text-slate-400">
                    {tag.description || <span className="text-slate-600">—</span>}
                  </AdminTd>
                  <AdminTd className="whitespace-nowrap text-xs text-slate-500">
                    {tag.createdAt ? new Date(tag.createdAt).toLocaleDateString() : "—"}
                  </AdminTd>
                  <AdminTd>
                    <div className="flex justify-end gap-2">
                      <Btn size="sm" tone="ghost" onClick={() => openEdit(tag)}>
                        Edit
                      </Btn>
                      <Btn
                        size="sm"
                        tone="danger"
                        disabled={deleteMut.isPending}
                        onClick={() => {
                          void (async () => {
                            if (
                              await confirm({
                                title: `Delete tag “${tag.name}”?`,
                                description:
                                  "The tag is removed from every resource using it. This cannot be undone.",
                                danger: true,
                                confirmLabel: "Delete",
                              })
                            ) {
                              deleteMut.mutate(tag.id);
                            }
                          })();
                        }}
                      >
                        <Trash2 size={12} /> Delete
                      </Btn>
                    </div>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {showForm ? (
        <Modal
          title={editing ? `Edit “${editing.name}”` : "Create Tag"}
          description="Tags are shared across the whole panel and can be assigned to applications, servers, and environments."
          onClose={() => setShowForm(false)}
        >
          <div className="space-y-4">
            <div>
              <Input
                label="Name"
                value={form.name}
                onChange={(v) => setForm((f) => ({ ...f, name: v }))}
                placeholder="production"
              />
              {errors.name ? <p className="mt-1 text-xs text-red-400">{errors.name}</p> : null}
              <p className="mt-1 text-xs text-slate-400">Shown on the pill; matched case-insensitively.</p>
            </div>

            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">Color</label>
              <div className="flex items-center gap-2">
                <input
                  type="color"
                  aria-label="Tag color picker"
                  value={sanitizeTagColor(form.color) ?? DEFAULT_COLOR}
                  onChange={(e) => setForm((f) => ({ ...f, color: e.target.value }))}
                  className="h-9 w-12 cursor-pointer rounded-md border border-white/10 bg-transparent p-1"
                />
                <Input
                  value={form.color}
                  onChange={(v) => setForm((f) => ({ ...f, color: v }))}
                  mono
                />
                <TagBadge name={form.name || "preview"} color={form.color} />
              </div>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {PALETTE.map((hex) => (
                  <button
                    key={hex}
                    type="button"
                    aria-label={`Use ${hex}`}
                    onClick={() => setForm((f) => ({ ...f, color: hex }))}
                    className="h-6 w-6 rounded-full border transition hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                    style={{
                      backgroundColor: hex,
                      borderColor:
                        sanitizeTagColor(form.color) === hex ? "var(--brand)" : "rgba(255,255,255,0.15)",
                    }}
                  />
                ))}
              </div>
              {errors.color ? <p className="mt-1 text-xs text-red-400">{errors.color}</p> : null}
            </div>

            <div>
              <Textarea
                label="Description"
                value={form.description}
                onChange={(v) => setForm((f) => ({ ...f, description: v }))}
                rows={2}
                placeholder="What is this tag for?"
              />
            </div>

            {saveMut.isError ? (
              <div className="flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>{(saveMut.error as Error).message || "An unexpected error occurred."}</span>
              </div>
            ) : null}
            {saveMut.isSuccess ? (
              <div className="flex items-start gap-2 rounded-lg border border-emerald-500/20 bg-emerald-950/10 p-3 text-xs text-emerald-200">
                <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
                <span>Saved.</span>
              </div>
            ) : null}

            <ModalFooter
              onCancel={() => setShowForm(false)}
              onConfirm={handleSubmit}
              disabled={!form.name.trim() || saveMut.isPending}
              confirmLabel={saveMut.isPending ? "Saving…" : editing ? "Save" : "Create"}
            />
          </div>
        </Modal>
      ) : null}

      {renderConfirm()}
    </div>
  );
}

export default TagsManager;
