"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpDown, GripVertical, Plus, Settings, Trash2, Eye, EyeOff, Lock, Unlock,
  Variable,
} from "lucide-react";
import { type ApiEgg, type ApiEggVariable, fetchEggVariables, createEggVariable, updateEggVariable, deleteEggVariable, reorderEggVariables } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, cn } from "./admin-ui";

function VariableCard({
  v,
  onEdit,
  onDelete,
  dragHandlers,
  isDragging,
}: {
  v: ApiEggVariable;
  onEdit: () => void;
  onDelete: () => void;
  dragHandlers: { onDragStart: () => void; onDragOver: (e: React.DragEvent) => void; onDragEnd: () => void };
  isDragging: boolean;
}) {
  return (
    <div
      draggable
      onDragStart={dragHandlers.onDragStart}
      onDragOver={dragHandlers.onDragOver}
      onDragEnd={dragHandlers.onDragEnd}
      className={cn(
        "flex items-start gap-3 rounded-lg border border-white/[0.06] bg-[#111722] p-3 transition hover:border-white/[0.12] sm:p-4",
        isDragging && "opacity-40 ring-2 ring-red-500/30",
      )}
    >
      <button
        className="mt-0.5 cursor-grab touch-none text-slate-600 hover:text-slate-400 active:cursor-grabbing"
        onMouseDown={(e) => e.currentTarget.parentElement?.draggable && void 0}
        type="button"
        aria-label="Drag to reorder"
      >
        <GripVertical size={16} />
      </button>

      <div className="min-w-0 flex-1 space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <code className="rounded-md bg-amber-500/10 px-2 py-0.5 font-mono text-xs font-semibold text-amber-300">
            {v.envVariable}
          </code>
          <span className="text-sm font-medium text-slate-200">{v.name}</span>
        </div>

        {v.description && (
          <p className="text-xs leading-relaxed text-slate-500">{v.description}</p>
        )}

        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px]">
          <span className="font-mono text-slate-500">
            Default: <span className="text-slate-300">{v.defaultValue || "\u2014"}</span>
          </span>
          <code className="rounded bg-white/[0.04] px-1.5 py-0.5 text-slate-500">
            {v.rules || "\u2014"}
          </code>
        </div>

        <div className="flex items-center gap-3">
          {v.userViewable ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-400">
              <Eye size={10} /> Viewable
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 rounded-full bg-slate-500/10 px-2 py-0.5 text-[10px] font-medium text-slate-500">
              <EyeOff size={10} /> Hidden
            </span>
          )}
          {v.userEditable ? (
            <span className="inline-flex items-center gap-1 rounded-full bg-blue-500/10 px-2 py-0.5 text-[10px] font-medium text-blue-400">
              <Unlock size={10} /> Editable
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 rounded-full bg-slate-500/10 px-2 py-0.5 text-[10px] font-medium text-slate-500">
              <Lock size={10} /> Locked
            </span>
          )}
        </div>
      </div>

      <div className="flex shrink-0 flex-col gap-1">
        <Btn size="sm" tone="ghost" onClick={onEdit}>
          <Settings size={12} />
        </Btn>
        <Btn size="sm" tone="danger" onClick={onDelete}>
          <Trash2 size={12} />
        </Btn>
      </div>
    </div>
  );
}

export function AdminEggVariables({ egg }: { egg: ApiEgg }) {
  const qc = useQueryClient();
  const varsQuery = useQuery({
    queryKey: ["egg-variables", egg.id],
    queryFn: () => fetchEggVariables(egg.id),
  });
  const variables = varsQuery.data ?? [];
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
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["egg-variables", egg.id] }); setModal(null); },
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
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["egg-variables", egg.id] }); setModal(null); },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteEggVariable(egg.id, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["egg-variables", egg.id] }),
  });

  const reorderMut = useMutation({
    mutationFn: (ids: string[]) => reorderEggVariables(egg.id, ids),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["egg-variables", egg.id] }),
  });

  const [dragIndex, setDragIndex] = useState<number | null>(null);

  const handleDragStart = (index: number) => setDragIndex(index);
  const handleDragOver = (e: React.DragEvent, index: number) => {
    e.preventDefault();
    if (dragIndex === null || dragIndex === index) return;
    const items = [...variables];
    const [moved] = items.splice(dragIndex, 1);
    items.splice(index, 0, moved);
    setDragIndex(index);
    reorderMut.mutate(items.map((item) => item.id));
  };
  const handleDragEnd = () => setDragIndex(null);

  return (
    <div className="space-y-6">
      <SectionHeader
        title={`Variables: ${egg.name}`}
        sub="Environment variables for this egg. Presented to users when creating or managing servers."
      />

      <Card>
        <CardHeader
          title={`${variables.length} variable${variables.length === 1 ? "" : "s"}`}
          icon={Variable}
          action={<Btn size="sm" onClick={openCreate}><Plus size={12} /> New Variable</Btn>}
        />

        {isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading variables\u2026</div>
        ) : isError ? (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load variables: {error?.message ?? "Unknown error"}</span>
              <Btn size="sm" tone="ghost" onClick={() => void refetch()}>Retry</Btn>
            </div>
          </div>
        ) : variables.length === 0 ? (
          <EmptyState icon={Variable} message="No variables defined for this egg." />
        ) : (
          <div>
            {/* Desktop table — hidden on small screens */}
            <div className="hidden sm:block overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                    <th className="w-8 px-3 py-3" />
                    <th className="px-3 py-3">Env Variable</th>
                    <th className="px-3 py-3">Name</th>
                    <th className="px-3 py-3">Default</th>
                    <th className="px-3 py-3">Rules</th>
                    <th className="px-3 py-3">Access</th>
                    <th className="px-3 py-3" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {variables.map((v, index) => (
                    <tr
                      key={v.id}
                      className={cn("hover:bg-white/[0.02]", dragIndex === index && "opacity-40")}
                      draggable
                      onDragEnd={handleDragEnd}
                      onDragOver={(e) => handleDragOver(e, index)}
                      onDragStart={() => handleDragStart(index)}
                    >
                      <td className="w-8 px-3 py-3">
                        <span className="inline-flex cursor-grab text-slate-500 active:cursor-grabbing">
                          <GripVertical size={14} />
                        </span>
                      </td>
                      <td className="px-3 py-3">
                        <code className="rounded-md bg-amber-500/10 px-2 py-0.5 font-mono text-xs font-semibold text-amber-300">
                          {v.envVariable}
                        </code>
                      </td>
                      <td className="px-3 py-3">
                        <p className="font-medium text-slate-200">{v.name}</p>
                        {v.description && (
                          <p className="max-w-[220px] truncate text-xs text-slate-500">{v.description}</p>
                        )}
                      </td>
                      <td className="max-w-[140px] truncate px-3 py-3 font-mono text-xs text-slate-400">
                        {v.defaultValue || <span className="text-slate-600">\u2014</span>}
                      </td>
                      <td className="px-3 py-3">
                        <code className="rounded bg-white/5 px-1.5 py-0.5 text-[10px] text-slate-400">
                          {v.rules || "\u2014"}
                        </code>
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center gap-2">
                          {v.userViewable ? (
                            <span className="inline-flex items-center gap-1 text-[10px] text-emerald-400"><Eye size={10} /> Viewable</span>
                          ) : (
                            <span className="inline-flex items-center gap-1 text-[10px] text-slate-500"><EyeOff size={10} /> Hidden</span>
                          )}
                          {v.userEditable ? (
                            <span className="inline-flex items-center gap-1 text-[10px] text-blue-400"><Unlock size={10} /> Editable</span>
                          ) : (
                            <span className="inline-flex items-center gap-1 text-[10px] text-slate-500"><Lock size={10} /> Locked</span>
                          )}
                        </div>
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center justify-end gap-1">
                          <Btn size="sm" tone="ghost" onClick={() => openEdit(v)}><Settings size={12} /></Btn>
                          <Btn size="sm" tone="danger" onClick={() => { if (confirm(`Delete variable ${v.envVariable}?`)) deleteMut.mutate(v.id); }}><Trash2 size={12} /></Btn>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Mobile cards — shown on small screens only */}
            <div className="space-y-2 p-3 sm:hidden">
              {variables.map((v, index) => (
                <VariableCard
                  key={v.id}
                  v={v}
                  isDragging={dragIndex === index}
                  onEdit={() => openEdit(v)}
                  onDelete={() => { if (confirm(`Delete variable ${v.envVariable}?`)) deleteMut.mutate(v.id); }}
                  dragHandlers={{
                    onDragStart: () => handleDragStart(index),
                    onDragOver: (e: React.DragEvent) => handleDragOver(e, index),
                    onDragEnd: handleDragEnd,
                  }}
                />
              ))}
            </div>

            <div className="flex items-center gap-2 border-t border-white/[0.06] px-4 py-2.5 text-[10px] text-slate-500">
              <ArrowUpDown size={10} /> Drag rows to reorder
            </div>
          </div>
        )}
      </Card>

      {modal !== null && (
        <Modal
          title={modal === "create" ? "Create Variable" : "Edit Variable"}
          onClose={() => setModal(null)}
          wide
        >
          <div className="grid gap-4 md:grid-cols-2">
            <Input label="Name" value={varName} onChange={setVarName} placeholder="Server Port" />
            <Input
              label="Environment Variable"
              value={varEnvVariable}
              onChange={(v) => setVarEnvVariable(v.toUpperCase())}
              placeholder="SERVER_PORT"
              mono
            />
            <div className="md:col-span-2">
              <Input label="Description" value={varDesc} onChange={setVarDesc} placeholder="The port the server will listen on" />
            </div>
            <Input label="Default Value" value={varDefaultValue} onChange={setVarDefaultValue} placeholder="25565" />
            <Input label="Validation Rules" value={varRules} onChange={setVarRules} placeholder="required|integer|min:1024|max:65535" mono />
            <label className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0d131d] px-4 py-3 transition hover:border-white/20">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[#dc2626]"
                checked={varUserViewable}
                onChange={(e) => setVarUserViewable(e.target.checked)}
              />
              <span className="text-sm text-slate-300">User viewable</span>
            </label>
            <label className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0d131d] px-4 py-3 transition hover:border-white/20">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[#dc2626]"
                checked={varUserEditable}
                onChange={(e) => setVarUserEditable(e.target.checked)}
              />
              <span className="text-sm text-slate-300">User editable</span>
            </label>
          </div>
          <ModalFooter
            onCancel={() => setModal(null)}
            onConfirm={() => modal === "create" ? createMut.mutate() : updateMut.mutate(modal)}
            disabled={varName.trim() === "" || varEnvVariable.trim() === "" || createMut.isPending || updateMut.isPending}
            confirmLabel={modal === "create" ? "Create" : "Save"}
          />
        </Modal>
      )}
    </div>
  );
}
