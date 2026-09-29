"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Boxes, Eye, EyeOff, Plus, ShieldCheck, Trash2, Pencil } from "lucide-react";
import {
  AdminErrorState,
  AdminIconButton,
  AdminLoadingRows,
  AdminPageLayout,
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
} from "@/components/admin/admin-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { isApiError } from "@/lib/api/http";
import {
  createRegistry,
  deleteRegistry,
  listRegistries,
  updateRegistry,
  verifyRegistry,
  type DockerRegistry,
  type RegistryInput,
  type RegistryVerifyResult,
} from "@/lib/api/registries";

const blankForm: RegistryInput = { name: "", serverAddress: "", username: "", credential: "", email: "", isGlobal: false };

type VerifyReport = { registryName: string; results: NonNullable<RegistryVerifyResult["results"]>; okCount: number; total: number };

function messageOf(err: unknown): string {
  return err instanceof Error && err.message ? err.message : "The request could not be completed.";
}

export default function AdminRegistriesPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<DockerRegistry | null>(null);
  const [form, setForm] = useState<RegistryInput>(blankForm);
  const [revealSecret, setRevealSecret] = useState(false);
  const [report, setReport] = useState<VerifyReport | null>(null);

  const registries = useQuery({ queryKey: ["admin", "registries"], queryFn: () => listRegistries() });

  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "registries"] });

  const rows: DockerRegistry[] = Array.isArray(registries.data) ? registries.data : [];

  const openCreate = () => {
    setEditing(null);
    setForm(blankForm);
    setRevealSecret(false);
    setShowForm(true);
  };

  const openEdit = (registry: DockerRegistry) => {
    // The API returns the credential masked ("********"), so it is never put
    // into the field: a masked value round-tripped back would be a lie about
    // what the operator is storing. Leaving the field blank means "keep the
    // stored secret", which is what the server does with an empty credential.
    setEditing(registry);
    setForm({
      name: registry.name,
      serverAddress: registry.serverAddress,
      username: registry.username ?? "",
      credential: "",
      email: registry.email ?? "",
      isGlobal: registry.isGlobal,
    });
    setRevealSecret(false);
    setShowForm(true);
  };

  const saveMut = useMutation({
    mutationFn: (input: RegistryInput) => (editing ? updateRegistry(editing.id, input) : createRegistry(input)),
    onSuccess: () => {
      toast({ title: editing ? "Registry updated" : "Registry added", tone: "success" });
      invalidate();
      setShowForm(false);
      setEditing(null);
      setForm(blankForm);
    },
    onError: (e: unknown) => toast({ title: editing ? "Failed to update registry" : "Failed to add registry", message: messageOf(e), tone: "error" }),
  });

  const verifyMut = useMutation({
    mutationFn: (input: { id: string; name: string }) => verifyRegistry(input.id),
    onSuccess: (res, input) => {
      const results = Array.isArray(res.results) ? res.results : [];
      const okCount = results.filter((r) => r.ok).length;
      if (results.length > 0) {
        setReport({ registryName: input.name, results, okCount, total: results.length });
        toast({
          tone: okCount === results.length ? "success" : "error",
          title: okCount === results.length ? "Registry login succeeded on every node" : "Registry login failed on some nodes",
          message: `${okCount} of ${results.length} node(s) logged in.`,
        });
        return;
      }
      // A single-node verify answers without a per-node list.
      if (res.ok && res.verified) {
        setReport(null);
        toast({ title: "Registry login succeeded", message: res.nodeId ? `Verified on node ${res.nodeId}.` : "Verified on the selected node.", tone: "success" });
      } else {
        setReport(null);
        toast({ title: "Registry login failed", message: res.error || "The node reported a failure without an error message.", tone: "error" });
      }
    },
    onError: (e: unknown) => {
      setReport(null);
      // The API answers 502 only when every node failed, and the per-node list
      // is lost with the error body — say exactly that rather than "failed".
      const allNodesFailed = isApiError(e) && e.status === 502;
      toast({
        title: allNodesFailed ? "Registry login failed on every node" : "Verify could not be run",
        message: allNodesFailed
          ? messageOf(e)
          : `${messageOf(e)} No node reported a result, so the credential is unverified.`,
        tone: "error",
      });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteRegistry(id),
    onSuccess: () => { toast({ title: "Registry removed", tone: "success" }); invalidate(); },
    onError: (e: unknown) => toast({ title: "Failed to remove", message: messageOf(e), tone: "error" }),
  });

  return (
    <AdminPageLayout>
      <SectionHeader
        info={adminPageGuides.registries}
        action={
          <Btn size="sm" tone="primary" onClick={openCreate}>
            <Plus size={12} /> Add Registry
          </Btn>
        }
      />

      <Card>
        <CardHeader title={registries.data ? `${registries.data.length} registries` : "Registries"} icon={Boxes} />
        {registries.isLoading ? (
          <AdminLoadingRows rows={3} label="Loading registries…" />
        ) : registries.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Registries could not be listed: ${messageOf(registries.error)} Credentials may still exist and still be used for pulls.`}
              retry={() => void registries.refetch()}
            />
          </div>
        ) : rows.length === 0 ? (
          <EmptyState icon={Boxes} title="No registries configured" message="The list loaded successfully and contains no entries. Add one to pull private images." />
        ) : (
          <div className="divide-y divide-line">
            {rows.map((r) => (
              <div key={r.id} className="flex flex-wrap items-center gap-3 px-5 py-3">
                <div className="min-w-0 flex-1 basis-48">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate text-sm font-semibold text-text">{r.name}</span>
                    {r.isGlobal && <Pill tone="blue">global</Pill>}
                    {!r.credential && <Pill tone="neutral">no stored credential</Pill>}
                  </div>
                  <p className="truncate font-mono text-xs text-text-subtle">{r.serverAddress}{r.username ? ` · ${r.username}` : ""}</p>
                </div>
                <div className="flex items-center gap-2">
                  <Btn
                    size="sm"
                    tone="ghost"
                    ariaLabel={`Verify credentials for ${r.name} on every node`}
                    loading={verifyMut.isPending && verifyMut.variables?.id === r.id}
                    disabled={verifyMut.isPending}
                    onClick={() => verifyMut.mutate({ id: r.id, name: r.name })}
                  >
                    <ShieldCheck size={14} /> Verify
                  </Btn>
                  <Btn
                    size="sm"
                    tone="subtle"
                    ariaLabel={`Edit ${r.name}`}
                    onClick={() => openEdit(r)}
                  >
                    <Pencil size={14} /> Edit
                  </Btn>
                  <AdminIconButton
                    label={`Remove ${r.name}`}
                    tone="danger"
                    onClick={() => {
                      void (async () => {
                        if (
                          await confirm({
                            title: `Remove registry “${r.name}”?`,
                            description: `The stored credential for ${r.serverAddress} is deleted. Workloads already pulled keep running; the next pull or push from ${r.serverAddress} will have no credentials and will fail.`,
                            danger: true,
                            confirmLabel: "Remove",
                          })
                        ) {
                          deleteMut.mutate(r.id);
                        }
                      })();
                    }}
                  >
                    <Trash2 size={14} />
                  </AdminIconButton>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      {report ? (
        <Card>
          <CardHeader title={`Verification — ${report.registryName}`} icon={ShieldCheck} />
          <div className="p-4 space-y-3">
            <p className="text-meta text-text-subtle">
              Verified on {report.okCount} of {report.total} node(s). A credential that works on one
              node is not valid everywhere; the failures below are the nodes that cannot log in.
            </p>
            <AdminTable label={`Verification results for ${report.registryName}`}>
              <AdminTHead>
                <AdminTh>Node</AdminTh>
                <AdminTh>Result</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {report.results.map((result) => (
                  <AdminTr key={result.nodeId}>
                    <AdminTd className="font-medium">{result.nodeName || result.nodeId}</AdminTd>
                    <AdminTd>
                      <div className="flex flex-wrap items-center gap-2">
                        <Pill tone={result.ok ? "green" : "red"}>{result.ok ? "Logged in" : "Failed"}</Pill>
                        {result.error ? <span className="text-meta text-text-subtle">{result.error}</span> : null}
                      </div>
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          </div>
        </Card>
      ) : null}

      {showForm && (
        <Modal
          title={editing ? `Edit ${editing.name}` : "Add Registry"}
          description="Credentials are encrypted at rest with the panel keyring; list and detail reads return them masked."
          onClose={() => { setShowForm(false); setEditing(null); }}
        >
          <div className="grid gap-4">
            <Input label="Name" value={form.name} onChange={(v) => setForm({ ...form, name: v })} placeholder="ghcr" />
            <Input label="Server address" value={form.serverAddress} onChange={(v) => setForm({ ...form, serverAddress: v })} placeholder="ghcr.io" />
            <p className="ui-hint -mt-2">
              An image pulls with these credentials when its registry host equals this address
              (for example <span className="font-mono">ghcr.io/team/app:1</span>).
            </p>
            <Input label="Username" value={form.username ?? ""} onChange={(v) => setForm({ ...form, username: v })} placeholder="user or _jsonkey" />
            <div>
              <Input
                label={editing ? "Password / token (blank keeps the stored one)" : "Password / token"}
                value={form.credential ?? ""}
                onChange={(v) => setForm({ ...form, credential: v })}
                type={revealSecret ? "text" : "password"}
                autoComplete={editing ? "off" : "new-password"}
                placeholder={editing ? "Unchanged — type a new secret to rotate it" : "Paste the registry secret"}
              />
              <div className="mt-2 flex items-center gap-2">
                <Btn
                  size="sm"
                  tone="ghost"
                  ariaLabel={revealSecret ? "Hide the secret" : "Show the secret"}
                  onClick={() => setRevealSecret((v) => !v)}
                >
                  {revealSecret ? <><EyeOff size={12} /> Hide</> : <><Eye size={12} /> Show</>}
                </Btn>
                <span className="ui-hint">Shown as dots unless you reveal it.</span>
              </div>
            </div>
            <Input label="Email (optional)" value={form.email ?? ""} onChange={(v) => setForm({ ...form, email: v })} placeholder="bot@example.com" />
            <label className="flex items-center gap-2 text-sm font-medium text-text">
              <input
                type="checkbox"
                checked={!!form.isGlobal}
                onChange={(e) => setForm({ ...form, isGlobal: e.target.checked })}
                className="rounded border-line bg-overlay-subtle"
              />
              Record as available to all users
            </label>
            <p className="ui-hint -mt-2">
              This flag is stored on the entry. Credential resolution at provision time matches on
              the server address above, not on this flag.
            </p>
            {saveMut.isError ? (
              <div className="ui-alert ui-alert-danger" role="alert"><span>{messageOf(saveMut.error)}</span></div>
            ) : null}
          </div>
          <ModalFooter
            onCancel={() => { setShowForm(false); setEditing(null); }}
            onConfirm={() => saveMut.mutate(form)}
            confirmLabel={saveMut.isPending ? "Saving…" : editing ? "Save changes" : "Add"}
            disabled={saveMut.isPending || !form.name.trim() || !form.serverAddress.trim()}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
