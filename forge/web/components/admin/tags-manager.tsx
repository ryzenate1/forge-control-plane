"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Plus, Tags as TagsIcon, Trash2 } from "lucide-react";
import {
  assignTag,
  bulkActionByTag,
  createTag,
  deleteTag,
  fetchTagResources,
  fetchTags,
  unassignTag,
  updateTag,
  type CreateTagInput,
  type Tag,
  type TagBulkAction,
  type TagBulkResult,
  type TagResourceType,
  type UpdateTagInput,
} from "@/lib/api/tags";
import { fetchApps } from "@/lib/api/apps";
import { fetchAllServers } from "@/lib/api/servers";
import { formatDate } from "@/lib/utils";
import { TagBadge, sanitizeTagColor } from "@/components/ui/tag-badge";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { adminPageGuides } from "./admin-page-guides";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminSelect,
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
  Pill,
  SectionHeader,
  Textarea,
} from "./admin-ui";

// A curated palette offered as one-click swatches. These are seed *data* values
// for a tag's color field (they end up rendered through TagBadge's sanitized
// inline style), not hardcoded CSS classes. Each entry carries a name so the
// swatch can be announced to a screen reader as a colour, not a hex triplet.
const PALETTE: Array<{ hex: string; name: string }> = [
  { hex: "#ef4444", name: "red" },
  { hex: "#f97316", name: "orange" },
  { hex: "#f59e0b", name: "amber" },
  { hex: "#84cc16", name: "lime" },
  { hex: "#22c55e", name: "green" },
  { hex: "#14b8a6", name: "teal" },
  { hex: "#06b6d4", name: "cyan" },
  { hex: "#3b82f6", name: "blue" },
  { hex: "#6366f1", name: "indigo" },
  { hex: "#8b5cf6", name: "violet" },
  { hex: "#a855f7", name: "purple" },
  { hex: "#d946ef", name: "fuchsia" },
  { hex: "#ec4899", name: "pink" },
  { hex: "#64748b", name: "slate" },
];

const DEFAULT_COLOR = "#6366f1";

/**
 * Kinds the assignment picker can actually offer. The API mounts assignment
 * routes for applications, servers and environments
 * (`forge/api/internal/http/handlers_tags.go`), but environments are only
 * reachable through a project (`fetchEnvironments(projectId)`), so this page
 * cannot list them without guessing a project — which the "never resolve an
 * ambiguous target silently" rule forbids. Environments therefore stay
 * read-only here: they appear in the tagged-resource list and their bulk rows
 * fail closed with the server's own reason.
 */
const ASSIGNABLE_KINDS: Array<{ value: TagResourceType; label: string }> = [
  { value: "application", label: "Application" },
  { value: "server", label: "Server" },
];

const KIND_LABELS: Record<string, string> = {
  application: "Application",
  server: "Server",
  environment: "Environment",
};

const BULK_ACTIONS: Array<{ value: TagBulkAction; label: string }> = [
  { value: "start", label: "Start" },
  { value: "stop", label: "Stop" },
  { value: "restart", label: "Restart" },
  { value: "deploy", label: "Deploy" },
];

type FormState = { name: string; color: string; description: string };

const emptyForm: FormState = { name: "", color: DEFAULT_COLOR, description: "" };

function validate(form: FormState): { name?: string; color?: string } {
  const errors: { name?: string; color?: string } = {};
  if (!form.name.trim()) errors.name = "Name is required";
  else if (form.name.trim().length > 64) errors.name = "Keep the name under 64 characters";
  if (!sanitizeTagColor(form.color)) errors.color = "Pick a valid colour";
  return errors;
}

function messageOf(err: unknown): string {
  return err instanceof Error && err.message ? err.message : "The request could not be completed.";
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

  // ---- Assignments / bulk panel ----
  const [selectedTagId, setSelectedTagId] = useState("");
  const [pickKind, setPickKind] = useState<TagResourceType | "">("");
  const [pickResourceId, setPickResourceId] = useState("");
  const [bulkAction, setBulkAction] = useState<TagBulkAction>("restart");
  const [bulkResults, setBulkResults] = useState<TagBulkResult[] | null>(null);

  const selectedTag = useMemo(
    () => tags.find((tag) => tag.id === selectedTagId) ?? null,
    [tags, selectedTagId],
  );

  const resourcesQuery = useQuery({
    queryKey: ["tag-resources", selectedTagId],
    queryFn: () => fetchTagResources(selectedTagId),
    enabled: selectedTagId !== "",
  });
  const resources = useMemo(() => resourcesQuery.data ?? [], [resourcesQuery.data]);

  const appsQuery = useQuery({
    queryKey: ["tags", "picker", "applications"],
    queryFn: fetchApps,
    enabled: pickKind === "application",
  });
  const serversQuery = useQuery({
    queryKey: ["tags", "picker", "servers"],
    queryFn: fetchAllServers,
    enabled: pickKind === "server",
  });

  const pickerOptions = useMemo(() => {
    if (pickKind === "application") {
      return (appsQuery.data ?? []).map((app) => ({ value: app.id, label: app.name }));
    }
    if (pickKind === "server") {
      return (serversQuery.data ?? []).map((server) => ({ value: server.id, label: server.name }));
    }
    return [];
  }, [pickKind, appsQuery.data, serversQuery.data]);

  const pickerLoading = pickKind === "application" ? appsQuery.isLoading : pickKind === "server" ? serversQuery.isLoading : false;
  const pickerError =
    pickKind === "application" ? appsQuery.error : pickKind === "server" ? serversQuery.error : null;

  const invalidateAssignments = () => {
    qc.invalidateQueries({ queryKey: ["tags"] });
    qc.invalidateQueries({ queryKey: ["tag-resources", selectedTagId] });
  };

  const openCreate = () => {
    setEditing(null);
    setForm(emptyForm);
    setErrors({});
    // Clear any earlier mutation state: the panels below read `isError`, and a
    // finished create must not be shown again as a fresh failure (or a stale
    // success) in the next dialog.
    saveMut.reset();
    setShowForm(true);
  };

  const openEdit = (tag: Tag) => {
    setEditing(tag);
    setForm({ name: tag.name, color: tag.color, description: tag.description ?? "" });
    setErrors({});
    saveMut.reset();
    setShowForm(true);
  };

  const saveMut = useMutation({
    mutationFn: (input: CreateTagInput | UpdateTagInput) =>
      editing ? updateTag(editing.id, input as UpdateTagInput) : createTag(input as CreateTagInput),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tags"] });
      qc.invalidateQueries({ queryKey: ["tag-resources"] });
      setShowForm(false);
      toast({ tone: "success", title: editing ? "Tag updated" : "Tag created" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: messageOf(e) }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteTag(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tags"] });
      qc.invalidateQueries({ queryKey: ["tag-resources"] });
      if (selectedTagId === deleteTargetId) {
        setSelectedTagId("");
        setBulkResults(null);
      }
      toast({ tone: "success", title: "Tag deleted" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: messageOf(e) }),
  });

  const [deleteTargetId, setDeleteTargetId] = useState("");

  /** Read the real attachment list before asking, so the confirm dialog can
   * state a measured blast radius instead of an assumed one. */
  const requestDelete = async (tag: Tag) => {
    setDeleteTargetId(tag.id);
    let usage: string;
    try {
      const attached = await fetchTagResources(tag.id);
      usage = attached.length === 0
        ? "No resources currently carry this tag."
        : `This detaches the tag from ${attached.length} resource(s): ${attached
            .slice(0, 3)
            .map((r) => `${KIND_LABELS[r.resourceType] ?? r.resourceType} “${r.name}”`)
            .join(", ")}${attached.length > 3 ? `, +${attached.length - 3} more` : ""}.`;
    } catch {
      usage = "Usage could not be read, so the number of affected resources is unknown.";
    }
    const ok = await confirm({
      title: `Delete tag “${tag.name}”?`,
      description: `${usage} The tag itself cannot be recovered. Resources and servers keep running.`,
      danger: true,
      confirmLabel: "Delete",
    });
    if (ok) deleteMut.mutate(tag.id);
  };

  const assignMut = useMutation({
    mutationFn: (input: { resourceType: TagResourceType; resourceId: string }) =>
      assignTag(input.resourceType, input.resourceId, selectedTagId),
    onSuccess: () => {
      invalidateAssignments();
      setPickResourceId("");
      toast({ tone: "success", title: "Tag attached" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Attach failed", message: messageOf(e) }),
  });

  const unassignMut = useMutation({
    mutationFn: (input: { resourceType: string; resourceId: string }) =>
      unassignTag(input.resourceType as TagResourceType, input.resourceId, selectedTagId),
    onSuccess: () => {
      invalidateAssignments();
      toast({ tone: "success", title: "Tag detached" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Detach failed", message: messageOf(e) }),
  });

  const bulkMut = useMutation({
    mutationFn: () => bulkActionByTag(selectedTagId, bulkAction),
    onSuccess: (res) => {
      const results = Array.isArray(res.data) ? res.data : [];
      setBulkResults(results);
      const okCount = results.filter((r) => r.ok).length;
      toast({
        tone: okCount === results.length && results.length > 0 ? "success" : "error",
        title: `Bulk ${bulkAction} reported`,
        message: `${okCount} of ${results.length} tagged resource(s) accepted the action.`,
      });
    },
    onError: (e: Error) => {
      setBulkResults(null);
      toast({ tone: "error", title: `Bulk ${bulkAction} failed`, message: messageOf(e) });
    },
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

  const okBulkCount = bulkResults ? bulkResults.filter((r) => r.ok).length : 0;

  return (
    <>
      <SectionHeader
        info={adminPageGuides.tags}
        action={
          <Btn tone="primary" onClick={openCreate}>
            <Plus size={14} /> New Tag
          </Btn>
        }
      />

      <Card>
        <CardHeader title={tagsQuery.data ? `${tags.length} tags` : "Tag catalog"} icon={TagsIcon} />
        {tagsQuery.isLoading ? (
          <AdminLoadingRows rows={4} label="Loading tags…" />
        ) : tagsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={messageOf(tagsQuery.error) }
              retry={() => void tagsQuery.refetch()}
            />
          </div>
        ) : tags.length === 0 ? (
          <EmptyState
            icon={TagsIcon}
            title="No tags yet"
            message="Create a tag, then attach it to applications or servers below."
          />
        ) : (
          <AdminTable label="Tags">
            <AdminTHead>
              <AdminTh>Tag</AdminTh>
              <AdminTh>Description</AdminTh>
              <AdminTh>Created</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {tags.map((tag) => (
                <AdminTr key={tag.id}>
                  <AdminTd>
                    <TagBadge name={tag.name} color={tag.color} description={tag.description} />
                  </AdminTd>
                  <AdminTd className="max-w-md truncate text-text-subtle">
                    {tag.description || <span className="text-text-muted">—</span>}
                  </AdminTd>
                  <AdminTd className="whitespace-nowrap text-xs text-text-subtle">
                    {formatDate(tag.createdAt, "Unknown")}
                  </AdminTd>
                  <AdminTd>
                    <div className="flex justify-end gap-2">
                      <Btn size="sm" tone="ghost" onClick={() => openEdit(tag)} ariaLabel={`Edit tag ${tag.name}`}>
                        Edit
                      </Btn>
                      <Btn
                        size="sm"
                        tone="danger"
                        disabled={deleteMut.isPending}
                        onClick={() => void requestDelete(tag)}
                        ariaLabel={`Delete tag ${tag.name}`}
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

      <Card>
        <CardHeader title="Attachments and bulk actions" icon={TagsIcon} />
        <div className="space-y-4 p-4">
          <p className="max-w-prose text-meta text-text-subtle">
            A tag is attached to one resource at a time here. Tags are not shown on
            server, application or environment screens yet, and no list on those pages
            filters by tag — this panel is where a tag is attached, reviewed and acted on.
          </p>

          <AdminSelect
            label="Tag"
            value={selectedTagId}
            onChange={(value) => {
              setSelectedTagId(value);
              setBulkResults(null);
              setPickResourceId("");
            }}
            placeholder={tags.length === 0 ? "No tags in the catalog" : "Choose a tag"}
            options={tags.map((tag) => ({ value: tag.id, label: tag.name }))}
            disabled={tags.length === 0}
          />

          {!selectedTagId ? (
            <p className="ui-hint">Choose a tag to see what carries it.</p>
          ) : resourcesQuery.isLoading ? (
            <AdminLoadingRows rows={3} label="Loading tagged resources…" />
          ) : resourcesQuery.isError ? (
            <AdminErrorState
              message={`Resources for “${selectedTag?.name ?? selectedTagId}” could not be loaded: ${messageOf(resourcesQuery.error)}`}
              retry={() => void resourcesQuery.refetch()}
            />
          ) : resources.length === 0 ? (
            <p className="ui-hint">
              Nothing carries “{selectedTag?.name ?? "this tag"}” yet. Attach a resource below.
            </p>
          ) : (
            <AdminTable label="Resources carrying this tag">
              <AdminTHead>
                <AdminTh>Resource</AdminTh>
                <AdminTh>Kind</AdminTh>
                <AdminTh className="text-right">Detach</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {resources.map((resource) => (
                  <AdminTr key={`${resource.resourceType}:${resource.resourceId}`}>
                    <AdminTd className="font-medium">{resource.name}</AdminTd>
                    <AdminTd>
                      <Pill tone="neutral">{KIND_LABELS[resource.resourceType] ?? resource.resourceType}</Pill>
                    </AdminTd>
                    <AdminTd className="text-right">
                      <Btn
                        size="sm"
                        tone="ghost"
                        disabled={unassignMut.isPending}
                        ariaLabel={`Detach ${resource.name} from ${selectedTag?.name ?? "tag"}`}
                        onClick={() =>
                          unassignMut.mutate({
                            resourceType: resource.resourceType,
                            resourceId: resource.resourceId,
                          })
                        }
                      >
                        Detach
                      </Btn>
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}

          <div className="grid gap-3 border-t border-line pt-4 sm:grid-cols-2 lg:grid-cols-3">
            <AdminSelect
              label="Attach a resource"
              value={pickKind}
              onChange={(value) => {
                setPickKind(value as TagResourceType);
                setPickResourceId("");
              }}
              placeholder="Choose a kind"
              options={ASSIGNABLE_KINDS}
              disabled={selectedTagId === ""}
            />
            <AdminSelect
              label="Resource"
              value={pickResourceId}
              onChange={setPickResourceId}
              placeholder={
                !pickKind
                  ? "Choose a kind first"
                  : pickerLoading
                    ? "Loading resources…"
                    : pickerOptions.length === 0
                      ? "No resources returned"
                      : "Choose a resource"
              }
              options={pickerOptions}
              disabled={!pickKind || pickerLoading || pickerOptions.length === 0}
            />
            <div className="flex items-end">
              <Btn
                tone="primary"
                disabled={
                  selectedTagId === "" || !pickKind || pickResourceId === "" || assignMut.isPending
                }
                onClick={() =>
                  assignMut.mutate({
                    resourceType: pickKind as TagResourceType,
                    resourceId: pickResourceId,
                  })
                }
              >
                <Plus size={14} /> Attach
              </Btn>
            </div>
            {pickerError ? (
              <div className="sm:col-span-2 lg:col-span-3">
                <AdminErrorState
                  message={`Resources could not be loaded: ${messageOf(pickerError)}`}
                  retry={() =>
                    void (pickKind === "application" ? appsQuery.refetch() : serversQuery.refetch())
                  }
                />
              </div>
            ) : null}
            <p className="ui-hint sm:col-span-2 lg:col-span-3">
              Environments are not offered here: the API lists them per project, so this
              page would have to guess a project to attach one. Servers and applications are
              listed fleet-wide and can be attached directly.
            </p>
          </div>

          <div className="grid gap-3 border-t border-line pt-4 sm:grid-cols-[minmax(0,14rem)_auto] sm:items-end">
            <AdminSelect
              label="Bulk action"
              value={bulkAction}
              onChange={(value) => setBulkAction(value as TagBulkAction)}
              options={BULK_ACTIONS}
              disabled={selectedTagId === ""}
            />
            <Btn
              tone="danger"
              disabled={
                selectedTagId === "" ||
                resourcesQuery.isLoading ||
                resourcesQuery.isError ||
                resources.length === 0 ||
                bulkMut.isPending
              }
              onClick={() => bulkMut.mutate()}
            >
              {bulkMut.isPending ? "Running…" : `Run on ${resourcesQuery.isFetched ? resources.length : "?"} tagged resource(s)`}
            </Btn>
            <p className="ui-hint sm:col-span-2">
              Dispatches the action on the server against every resource carrying this tag.
              Environment rows report their own refusal — only servers and applications have a
              runtime action. Results are listed below; a partial run is never reported as success.
            </p>
            {bulkMut.isError ? (
              <div className="sm:col-span-2">
                <AdminErrorState
                  message={`Bulk ${bulkAction} could not be dispatched: ${messageOf(bulkMut.error)}`}
                  retry={() => bulkMut.mutate()}
                />
              </div>
            ) : null}
          </div>

          {bulkResults ? (
            bulkResults.length === 0 ? (
              <p className="ui-hint">The server returned no per-resource result, so nothing was dispatched.</p>
            ) : (
              <div className="space-y-2">
                <p className="text-meta text-text-subtle">
                  Bulk {bulkAction}: {okBulkCount} of {bulkResults.length} resource(s) accepted.
                </p>
                <AdminTable label="Bulk action results">
                  <AdminTHead>
                    <AdminTh>Resource</AdminTh>
                    <AdminTh>Kind</AdminTh>
                    <AdminTh>Result</AdminTh>
                  </AdminTHead>
                  <AdminTBody>
                    {bulkResults.map((result) => (
                      <AdminTr key={`${result.resourceType}:${result.resourceId}`}>
                        <AdminTd className="font-medium">{result.name}</AdminTd>
                        <AdminTd>{KIND_LABELS[result.resourceType] ?? result.resourceType}</AdminTd>
                        <AdminTd>
                          <div className="flex flex-wrap items-center gap-2">
                            <Pill tone={result.ok ? "green" : "red"}>{result.ok ? "Accepted" : "Failed"}</Pill>
                            {result.error ? <span className="text-meta text-text-subtle">{result.error}</span> : null}
                          </div>
                        </AdminTd>
                      </AdminTr>
                    ))}
                  </AdminTBody>
                </AdminTable>
              </div>
            )
          ) : null}
        </div>
      </Card>

      {showForm ? (
        <Modal
          title={editing ? `Edit “${editing.name}”` : "Create Tag"}
          description="Tags are shared across the whole panel and can be attached to applications and servers."
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
              {errors.name ? <p className="ui-field-error mt-1.5">{errors.name}</p> : null}
              <p className="ui-hint mt-1.5">Shown on the pill; matched case-insensitively.</p>
            </div>

            <div>
              <span className="ui-label mb-1.5">Colour</span>
              <div className="flex items-center gap-2">
                <input
                  type="color"
                  aria-label="Colour picker"
                  value={sanitizeTagColor(form.color) ?? DEFAULT_COLOR}
                  onChange={(e) => setForm((f) => ({ ...f, color: e.target.value }))}
                  className="h-9 w-12 cursor-pointer rounded-md border border-line bg-transparent p-1"
                />
                <Input
                  label="Hex value"
                  value={form.color}
                  onChange={(v) => setForm((f) => ({ ...f, color: v }))}
                  mono
                />
                <TagBadge name={form.name || "preview"} color={form.color} />
              </div>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {PALETTE.map((swatch) => {
                  const selected = sanitizeTagColor(form.color) === swatch.hex;
                  return (
                    <button
                      key={swatch.hex}
                      type="button"
                      aria-label={`Use ${swatch.name} (${swatch.hex})`}
                      aria-pressed={selected}
                      title={swatch.name}
                      onClick={() => setForm((f) => ({ ...f, color: swatch.hex }))}
                      className="grid h-6 w-6 place-items-center rounded-full border text-text transition hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                      style={{
                        backgroundColor: swatch.hex,
                        borderColor: selected ? "var(--brand)" : "var(--line)",
                        boxShadow: selected ? "0 0 0 2px var(--brand)" : undefined,
                      }}
                    >
                      {selected ? <span aria-hidden="true" className="font-semibold">✓</span> : null}
                    </button>
                  );
                })}
              </div>
              {errors.color ? <p className="ui-field-error mt-1.5">{errors.color}</p> : null}
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
              <div className="ui-alert ui-alert-danger" role="alert">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>{messageOf(saveMut.error)}</span>
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
    </>
  );
}

export default TagsManager;
