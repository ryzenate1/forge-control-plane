"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Boxes, Plus, ShieldCheck, Trash2 } from "lucide-react";
import {
  AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader,
} from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  listRegistries, createRegistry, deleteRegistry, verifyRegistry,
  type DockerRegistry, type RegistryInput,
} from "@/lib/api";

export default function AdminRegistriesPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState<RegistryInput>({ name: "", serverAddress: "", username: "", credential: "", email: "", isGlobal: false });

  const registries = useQuery({ queryKey: ["admin", "registries"], queryFn: () => listRegistries() });

  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "registries"] });

  const createMut = useMutation({
    mutationFn: (input: RegistryInput) => createRegistry(input),
    onSuccess: () => {
      toast({ title: "Registry added", tone: "success" });
      invalidate();
      setShowCreate(false);
      setForm({ name: "", serverAddress: "", username: "", credential: "", email: "", isGlobal: false });
    },
    onError: (e: unknown) => toast({ title: "Failed to add registry", message: e instanceof Error ? e.message : String(e), tone: "error" }),
  });

  const verifyMut = useMutation({
    mutationFn: (id: string) => verifyRegistry(id),
    onSuccess: (res) => {
      if (res.ok && res.verified) toast({ title: "Registry login succeeded", tone: "success" });
      else toast({ title: "Registry login failed", message: res.error, tone: "error" });
    },
    onError: (e: unknown) => toast({ title: "Verify failed", message: e instanceof Error ? e.message : String(e), tone: "error" }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteRegistry(id),
    onSuccess: () => { toast({ title: "Registry removed", tone: "success" }); invalidate(); },
    onError: (e: unknown) => toast({ title: "Failed to remove", message: e instanceof Error ? e.message : String(e), tone: "error" }),
  });

  const rows: DockerRegistry[] = Array.isArray(registries.data) ? registries.data : [];

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Image Registries"
        sub="Private Docker registry credentials used to pull and push images on nodes."
        action={
          <Btn size="sm" tone="primary" onClick={() => setShowCreate(true)}>
            <Plus size={12} /> Add Registry
          </Btn>
        }
      />

      <Card>
        <CardHeader title="Registries" icon={Boxes} />
        {registries.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading registries…</div>
        ) : rows.length === 0 ? (
          <EmptyState icon={Boxes} message="No registries configured. Add one to pull private images." />
        ) : (
          <div className="divide-y divide-white/[0.06]">
            {rows.map((r) => (
              <div key={r.id} className="flex items-center gap-4 px-5 py-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-semibold text-slate-200">{r.name}</span>
                    {r.isGlobal && <Pill tone="blue">global</Pill>}
                  </div>
                  <p className="truncate font-mono text-xs text-slate-500">{r.serverAddress}{r.username ? ` · ${r.username}` : ""}</p>
                </div>
                <Btn size="sm" tone="ghost" onClick={() => verifyMut.mutate(r.id)} disabled={verifyMut.isPending}>
                  <ShieldCheck size={14} /> Verify
                </Btn>
                <Btn
                  size="sm"
                  tone="danger"
                  onClick={() => { void (async () => { if (await confirm({ title: `Remove registry “${r.name}”?`, danger: true, confirmLabel: "Remove" })) deleteMut.mutate(r.id); })(); }}
                >
                  <Trash2 size={14} />
                </Btn>
              </div>
            ))}
          </div>
        )}
      </Card>

      {showCreate && (
        <Modal title="Add Registry" onClose={() => setShowCreate(false)}>
          <div className="grid gap-4">
            <Input label="Name" value={form.name} onChange={(v) => setForm({ ...form, name: v })} placeholder="ghcr" />
            <Input label="Server Address" value={form.serverAddress} onChange={(v) => setForm({ ...form, serverAddress: v })} placeholder="ghcr.io" />
            <Input label="Username" value={form.username ?? ""} onChange={(v) => setForm({ ...form, username: v })} placeholder="user or _jsonkey" />
            <Input label="Password / Token" value={form.credential ?? ""} onChange={(v) => setForm({ ...form, credential: v })} placeholder="••••••••" />
            <Input label="Email (optional)" value={form.email ?? ""} onChange={(v) => setForm({ ...form, email: v })} placeholder="bot@example.com" />
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
              <input type="checkbox" checked={!!form.isGlobal} onChange={(e) => setForm({ ...form, isGlobal: e.target.checked })} className="rounded border-white/10 bg-[var(--surface-input)]" />
              Available to all users
            </label>
          </div>
          <ModalFooter
            onCancel={() => setShowCreate(false)}
            onConfirm={() => createMut.mutate(form)}
            confirmLabel={createMut.isPending ? "Saving…" : "Add"}
            disabled={createMut.isPending || !form.name.trim() || !form.serverAddress.trim()}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
