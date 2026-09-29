"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpDown, ChevronDown, ChevronUp, GripVertical, Plus, Settings, Trash2, Eye, EyeOff, Lock, Unlock,
  Variable,
} from "lucide-react";
import { type ApiEgg, type ApiEggVariable, fetchEggVariables, createEggVariable, updateEggVariable, deleteEggVariable, reorderEggVariables } from "@/lib/api";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import {
  AdminErrorState,
  AdminIconButton,
  AdminLoadingRows,
  AdminSection,
  AdminTable,
  AdminTBody,
  AdminTh,
  AdminTHead,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
} from "./admin-ui";

function messageOf(err: unknown): string {
  return err instanceof Error && err.message ? err.message : "The request could not be completed.";
}

/** The user-facing consequence of each flag, spelled out: `userViewable` and
 * `userEditable` gate what a server owner sees and changes, which the old
 * "Viewable" / "Hidden" chips never said. */
function AccessChips({ viewable, editable }: { viewable: boolean; editable: boolean }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {viewable ? (
        <Pill tone="green"><Eye size={10} /> Visible to users</Pill>
      ) : (
        <Pill tone="neutral"><EyeOff size={10} /> Hidden from users</Pill>
      )}
      {editable ? (
        <Pill tone="blue"><Unlock size={10} /> Users can change it</Pill>
      ) : (
        <Pill tone="neutral"><Lock size={10} /> Users cannot change it</Pill>
      )}
    </div>
  );
}

function VariableCard({
  v,
  onEdit,
  onDelete,
  onMoveUp,
  onMoveDown,
  dragHandlers,
  isDragging,
}: {
  v: ApiEggVariable;
  onEdit: () => void;
  onDelete: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
  dragHandlers: { onDragStart: () => void; onDragOver: (e: React.DragEvent) => void; onDragEnd: () => void };
  isDragging: boolean;
}) {
  return (
    <div
      draggable
      onDragStart={dragHandlers.onDragStart}
      onDragOver={dragHandlers.onDragOver}
      onDragEnd={dragHandlers.onDragEnd}
      className={cnRow(isDragging)}
    >
      <div className="flex shrink-0 flex-col gap-1">
        {/* Keyboard/touch path for ordering; the pointer drag alone was the only
            advertised way to reorder. */}
        <AdminIconButton label={`Move ${v.envVariable} up`} onClick={onMoveUp}><ChevronUp size={12} /></AdminIconButton>
        <AdminIconButton label={`Move ${v.envVariable} down`} onClick={onMoveDown}><ChevronDown size={12} /></AdminIconButton>
      </div>

      <div className="min-w-0 flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <code className="rounded-md bg-overlay-strong px-2 py-0.5 font-mono text-xs font-semibold text-text">{v.envVariable}</code>
          <span className="text-sm font-medium text-text">{v.name}</span>
        </div>

        {v.description && (
          <p className="text-xs leading-relaxed text-text-subtle">{v.description}</p>
        )}

        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
          <span className="font-mono text-text-subtle">
            Default: <span className="text-text">{v.defaultValue || "—"}</span>
          </span>
          <code className="rounded bg-overlay px-1.5 py-0.5 font-mono text-xs text-text-subtle">
            {v.rules || "—"}
          </code>
        </div>

        <AccessChips viewable={v.userViewable} editable={v.userEditable} />
      </div>

      <div className="flex shrink-0 flex-col gap-1">
        <AdminIconButton label={`Edit ${v.envVariable}`} onClick={onEdit}><Settings size={12} /></AdminIconButton>
        <AdminIconButton label={`Delete ${v.envVariable}`} tone="danger" onClick={onDelete}><Trash2 size={12} /></AdminIconButton>
      </div>
    </div>
  );
}

function cnRow(isDragging: boolean) {
  return [
    "flex items-start gap-3 rounded-lg border border-line bg-overlay-subtle p-3 transition hover:border-line-strong sm:p-4",
    isDragging ? "opacity-40 ring-2 ring-[var(--focus)]" : "",
  ].join(" ");
}

export function AdminEggVariables({ egg }: { egg: ApiEgg }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const varsQuery = useQuery({
    queryKey: ["egg-variables", egg.id],
    queryFn: () => fetchEggVariables(egg.id),
  });
  const variables = useMemo(() => varsQuery.data ?? [], [varsQuery.data]);
  const isLoading = varsQuery.isLoading;
  const isError = varsQuery.isError;
  const error = varsQuery.error;
  const refetch = varsQuery.refetch;

  const [modal, setModal] = useState<null | "create" | ApiEggVariable>(null);

  const [varName, setVarName] = useState("");
  const [varDesc, setVarDesc] = useState("");
  const [varEnvVariable, setVarEnvVariable] = useState("");
  const [varDefaultValue, setVarDefaultValue] = useState("");
  const [varUserViewable, setVarUserViewable] = useState(true);
  const [varUserEditable, setVarUserEditable] = useState(true);
  const [varRules, setVarRules] = useState("required|string");

  const resetForm = () => {
    setVarName("");
    setVarDesc("");
    setVarEnvVariable("");
    setVarDefaultValue("");
    setVarUserViewable(true);
    setVarUserEditable(true);
    setVarRules("required|string");
  };

  const openCreate = () => { resetForm(); setModal("create"); };
  const openEdit = (v: ApiEggVariable) => {
    setVarName(v.name);
    setVarDesc(v.description ?? "");
    setVarEnvVariable(v.envVariable);
    setVarDefaultValue(v.defaultValue);
    setVarUserViewable(v.userViewable);
    setVarUserEditable(v.userEditable);
    setVarRules(v.rules);
    setModal(v);
  };

  const invalidateVars = () => qc.invalidateQueries({ queryKey: ["egg-variables", egg.id] });

  const createMut = useMutation({
    mutationFn: () => createEggVariable(egg.id, {
      name: varName.trim(),
      description: varDesc.trim(),
      envVariable: varEnvVariable.trim().toUpperCase(),
      defaultValue: varDefaultValue,
      userViewable: varUserViewable,
      userEditable: varUserEditable,
      rules: varRules.trim(),
    }),
    onSuccess: () => { invalidateVars(); setModal(null); toast({ tone: "success", title: "Variable created" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to create variable", message: messageOf(err) }),
  });

  const updateMut = useMutation({
    mutationFn: (v: ApiEggVariable) => updateEggVariable(egg.id, v.id, {
      name: varName.trim(),
      description: varDesc.trim(),
      envVariable: varEnvVariable.trim().toUpperCase(),
      defaultValue: varDefaultValue,
      userViewable: varUserViewable,
      userEditable: varUserEditable,
      rules: varRules.trim(),
    }),
    onSuccess: () => { invalidateVars(); setModal(null); toast({ tone: "success", title: "Variable updated" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to update variable", message: messageOf(err) }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteEggVariable(egg.id, id),
    onSuccess: () => { invalidateVars(); toast({ tone: "success", title: "Variable deleted" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to delete variable", message: messageOf(err) }),
  });

  /**
   * Reordering commits exactly once, on drop. The previous implementation called
   * `reorderEggVariables` from every `dragover`, so dragging across N rows
   * issued N POSTs and the last one to land silently won.
   */
  const reorderMut = useMutation({
    mutationFn: (ids: string[]) => reorderEggVariables(egg.id, ids),
    onSuccess: () => { invalidateVars(); setPendingOrder(null); },
    onError: (err) => { setPendingOrder(null); toast({ tone: "error", title: "Order not saved", message: messageOf(err) }); },
  });

  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [pendingOrder, setPendingOrder] = useState<string[] | null>(null);

  const rows = useMemo(() => {
    if (!pendingOrder) return variables;
    const byId = new Map(variables.map((v) => [v.id, v]));
    const ordered = pendingOrder
      .map((id) => byId.get(id))
      .filter((v): v is ApiEggVariable => Boolean(v));
    const unknown = variables.filter((v) => !pendingOrder.includes(v.id));
    return [...ordered, ...unknown];
  }, [variables, pendingOrder]);

  const commitOrder = (ids: string[]) => reorderMut.mutate(ids);

  const move = (index: number, delta: -1 | 1) => {
    const ids = rows.map((v) => v.id);
    const target = index + delta;
    if (target < 0 || target >= ids.length) return;
    const next = [...ids];
    const [moved] = next.splice(index, 1);
    next.splice(target, 0, moved);
    setPendingOrder(next);
    commitOrder(next);
  };

  const handleDragStart = (index: number) => setDragIndex(index);
  const handleDragOver = (e: React.DragEvent, index: number) => {
    e.preventDefault();
    if (dragIndex === null || dragIndex === index) return;
    const ids = (pendingOrder ?? rows.map((v) => v.id));
    const next = [...ids];
    const [moved] = next.splice(dragIndex, 1);
    next.splice(index, 0, moved);
    setDragIndex(index);
    // Local preview only — no request until the row is dropped.
    setPendingOrder(next);
  };
  const handleDragEnd = () => {
    setDragIndex(null);
    if (pendingOrder) commitOrder(pendingOrder);
  };

  const confirmDelete = (v: ApiEggVariable) => {
    void (async () => {
      if (await confirm({
        title: `Delete variable ${v.envVariable}?`,
        description: "Servers created from this definition keep their current value for it; new servers will not be given the variable. This cannot be undone.",
        danger: true,
        confirmLabel: "Delete",
      })) deleteMut.mutate(v.id);
    })();
  };

  return (
    <div className="space-y-6">
      <AdminSection
        title="Environment variables"
        description="Values presented to users when a server is created from this definition, and the rules the panel enforces on them."
      >
        <Card>
          <CardHeader
            title={varsQuery.data ? `${variables.length} variable${variables.length === 1 ? "" : "s"}` : "Variables"}
            icon={Variable}
            action={<Btn size="sm" onClick={openCreate}><Plus size={12} /> New Variable</Btn>}
          />

          {isLoading ? (
            <AdminLoadingRows rows={4} label="Loading variables…" />
          ) : isError ? (
            <div className="p-4">
              <AdminErrorState
                message={`Could not load variables: ${messageOf(error)}`}
                retry={() => void refetch()}
              />
            </div>
          ) : variables.length === 0 ? (
            <EmptyState icon={Variable} title="No variables defined" message="This egg exposes nothing at creation time. Add a variable to ask for a value." />
          ) : (
            <div>
              {/* Desktop table — hidden on small screens */}
              <div className="hidden sm:block">
                <AdminTable label={`Variables for ${egg.name}`}>
                  <AdminTHead>
                    <AdminTh className="w-20">Order</AdminTh>
                    <AdminTh>Env variable</AdminTh>
                    <AdminTh>Name</AdminTh>
                    <AdminTh>Default</AdminTh>
                    <AdminTh>Rules</AdminTh>
                    <AdminTh>Access</AdminTh>
                    <AdminTh className="text-right">Actions</AdminTh>
                  </AdminTHead>
                  <AdminTBody>
                    {rows.map((v, index) => (
                      <tr
                        key={v.id}
                        className={dragIndex === index ? "opacity-40" : undefined}
                        draggable
                        onDragEnd={handleDragEnd}
                        onDragOver={(e) => handleDragOver(e, index)}
                        onDragStart={() => handleDragStart(index)}
                      >
                        <td className="ui-td">
                          <div className="flex items-center gap-1">
                            <span aria-hidden="true" className="inline-flex cursor-grab text-text-muted active:cursor-grabbing">
                              <GripVertical size={14} />
                            </span>
                            <Btn size="sm" tone="ghost" ariaLabel={`Move ${v.envVariable} up`} disabled={index === 0 || reorderMut.isPending} onClick={() => move(index, -1)}>
                              <ChevronUp size={12} />
                            </Btn>
                            <Btn size="sm" tone="ghost" ariaLabel={`Move ${v.envVariable} down`} disabled={index === rows.length - 1 || reorderMut.isPending} onClick={() => move(index, 1)}>
                              <ChevronDown size={12} />
                            </Btn>
                          </div>
                        </td>
                        <td className="ui-td">
                          <code className="rounded-md bg-overlay-strong px-2 py-0.5 font-mono text-xs font-semibold text-text">{v.envVariable}</code>
                        </td>
                        <td className="ui-td">
                          <p className="font-medium text-text">{v.name}</p>
                          {v.description && (
                            <p className="max-w-[26ch] break-words text-xs text-text-subtle">{v.description}</p>
                          )}
                        </td>
                        <td className="ui-td max-w-[20ch] break-all font-mono text-xs text-text-subtle">
                          {v.defaultValue || <span className="text-text-muted">No default</span>}
                        </td>
                        <td className="ui-td">
                          <code className="rounded bg-overlay px-1.5 py-0.5 font-mono text-xs text-text-subtle">
                            {v.rules || "No rules"}
                          </code>
                        </td>
                        <td className="ui-td"><AccessChips viewable={v.userViewable} editable={v.userEditable} /></td>
                        <td className="ui-td">
                          <div className="flex items-center justify-end gap-1">
                            <AdminIconButton label={`Edit ${v.envVariable}`} onClick={() => openEdit(v)}><Settings size={12} /></AdminIconButton>
                            <AdminIconButton label={`Delete ${v.envVariable}`} tone="danger" onClick={() => confirmDelete(v)}><Trash2 size={12} /></AdminIconButton>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </AdminTBody>
                </AdminTable>
              </div>

              {/* Mobile cards — shown on small screens only */}
              <div className="space-y-2 p-3 sm:hidden">
                {rows.map((v, index) => (
                  <VariableCard
                    key={v.id}
                    v={v}
                    isDragging={dragIndex === index}
                    onEdit={() => openEdit(v)}
                    onDelete={() => confirmDelete(v)}
                    onMoveUp={() => move(index, -1)}
                    onMoveDown={() => move(index, 1)}
                    dragHandlers={{
                      onDragStart: () => handleDragStart(index),
                      onDragOver: (e: React.DragEvent) => handleDragOver(e, index),
                      onDragEnd: handleDragEnd,
                    }}
                  />
                ))}
              </div>

              <div className="flex items-center gap-2 border-t border-line px-4 py-2.5 text-meta text-text-subtle">
                <ArrowUpDown />
                <span>
                  {reorderMut.isPending
                    ? "Saving the new order…"
                    : pendingOrder
                      ? "New order staged — it is saved when the row is dropped."
                      : "Drag a row, or use the up and down buttons, to change the order variables are shown in."}
                </span>
              </div>
            </div>
          )}
        </Card>
      </AdminSection>

      {modal !== null && (
        <Modal
          title={modal === "create" ? "Create Variable" : "Edit Variable"}
          onClose={() => setModal(null)}
          wide
        >
          <div className="grid gap-4 md:grid-cols-2">
            <Input label="Name" value={varName} onChange={setVarName} placeholder="Server Port" />
            <Input
              label="Environment variable"
              value={varEnvVariable}
              onChange={(v) => setVarEnvVariable(v.toUpperCase())}
              placeholder="SERVER_PORT"
              mono
            />
            <div className="md:col-span-2">
              <Input label="Description" value={varDesc} onChange={setVarDesc} placeholder="The port the server will listen on" />
            </div>
            <Input label="Default value" value={varDefaultValue} onChange={setVarDefaultValue} placeholder="25565" />
            <Input label="Validation rules" value={varRules} onChange={setVarRules} placeholder="required|integer|min:1024|max:65535" mono />
            <label className="flex items-center gap-3 rounded-lg border border-line bg-overlay-subtle px-4 py-3 transition hover:border-line-strong">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[var(--brand)]"
                checked={varUserViewable}
                onChange={(e) => setVarUserViewable(e.target.checked)}
              />
              <span className="text-sm text-text">Visible to users</span>
            </label>
            <label className="flex items-center gap-3 rounded-lg border border-line bg-overlay-subtle px-4 py-3 transition hover:border-line-strong">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[var(--brand)]"
                checked={varUserEditable}
                onChange={(e) => setVarUserEditable(e.target.checked)}
              />
              <span className="text-sm text-text">Changeable by users</span>
            </label>
            {createMut.isError || updateMut.isError ? (
              <div className="ui-alert ui-alert-danger md:col-span-2" role="alert">
                <span>{messageOf(createMut.error ?? updateMut.error)}</span>
              </div>
            ) : null}
          </div>
          <ModalFooter
            onCancel={() => setModal(null)}
            onConfirm={() => modal === "create" ? createMut.mutate() : updateMut.mutate(modal)}
            disabled={!varName.trim() || !varEnvVariable.trim() || createMut.isPending || updateMut.isPending}
            confirmLabel={modal === "create" ? (createMut.isPending ? "Creating…" : "Create") : (updateMut.isPending ? "Saving…" : "Save")}
          />
        </Modal>
      )}
      {renderConfirm()}
    </div>
  );
}
