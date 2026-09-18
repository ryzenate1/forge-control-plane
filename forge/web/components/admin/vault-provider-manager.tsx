"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plug, Plus, RefreshCw, Trash2 } from "lucide-react";

import {
  createVaultConnection,
  deleteVaultConnection,
  listVaultConnections,
  testVaultConnection,
  updateVaultConnection,
  type CreateVaultConnectionInput,
  type VaultAuthMethod,
  type VaultConnection,
} from "@/lib/api/vault-provider";
import { errorMessage, formatDate } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Badge,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
} from "./admin-ui";

// Admin "HashiCorp Vault" provider panel. Registers external Vault
// connections (endpoint + KV mount + auth method) whose secrets environment
// variables can reference as `vault:<connection-id>/<path>#<field>` and have
// resolved live at deploy time. Credentials are encrypted at rest and never
// returned by the API — the table shows only a masked hint. Test performs a
// real auth + reachability check; failures surface honestly as errors.

const AUTH_METHODS: Array<{ value: VaultAuthMethod; label: string }> = [
  { value: "token", label: "Token" },
  { value: "approle", label: "AppRole" },
];

const ENGINE_VERSIONS = [
  { value: "2", label: "KV v2 (modern)" },
  { value: "1", label: "KV v1 (legacy)" },
];

type ConnectionForm = {
  name: string;
  baseUrl: string;
  mountPath: string;
  namespace: string;
  engineVersion: number;
  authMethod: VaultAuthMethod;
  token: string;
  roleId: string;
  secretId: string;
  enabled: boolean;
};

const EMPTY_FORM: ConnectionForm = {
  name: "",
  baseUrl: "",
  mountPath: "secret",
  namespace: "",
  engineVersion: 2,
  authMethod: "token",
  token: "",
  roleId: "",
  secretId: "",
  enabled: true,
};

export function VaultProviderManager() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [modal, setModal] = useState<{ mode: "create" | "edit"; connection?: VaultConnection } | null>(null);

  const connectionsQuery = useQuery({
    queryKey: ["admin", "vault", "connections"],
    queryFn: listVaultConnections,
  });
  const connections = useMemo(() => connectionsQuery.data ?? [], [connectionsQuery.data]);

  const refresh = () => qc.invalidateQueries({ queryKey: ["admin", "vault"] });

  const toggleMut = useMutation({
    mutationFn: (vars: { id: string; enabled: boolean }) => updateVaultConnection(vars.id, { enabled: vars.enabled }),
    onSuccess: () => {
      toast({ tone: "success", title: "Connection updated" });
      refresh();
    },
    onError: (err) => toast({ tone: "error", title: "Update failed", message: errorMessage(err) }),
  });

  const testMut = useMutation({
    mutationFn: (id: string) => testVaultConnection(id),
    onSuccess: () => toast({ tone: "success", title: "Vault reachable", message: "Authentication and connectivity verified." }),
    onError: (err) => toast({ tone: "error", title: "Test failed", message: errorMessage(err) }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteVaultConnection(id),
    onSuccess: () => {
      toast({ tone: "success", title: "Connection deleted" });
      refresh();
    },
    onError: (err) => toast({ tone: "error", title: "Delete failed", message: errorMessage(err) }),
  });

  const handleDelete = async (conn: VaultConnection) => {
    const ok = await confirm({
      title: "Delete Vault connection?",
      description: `Environment variables referencing "${conn.name}" will fail to resolve until repointed. The stored credential is discarded.`,
      danger: true,
      confirmLabel: "Delete connection",
    });
    if (ok) deleteMut.mutate(conn.id);
  };

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Vault"
        sub="Register HashiCorp Vault connections so environment variables can reference secrets fetched live at deploy time, instead of duplicating them into Forge's own encrypted store."
        action={
          <div className="flex items-center gap-2">
            <Btn tone="ghost" size="sm" onClick={() => void refresh()} loading={connectionsQuery.isFetching}>
              <RefreshCw size={14} className="mr-1.5" /> Refresh
            </Btn>
            <Btn size="sm" onClick={() => setModal({ mode: "create" })}>
              <Plus size={14} className="mr-1.5" /> New connection
            </Btn>
          </div>
        }
      />

      <Card>
        <CardHeader title="Connections" icon={KeyRound} />
        {connectionsQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading Vault connections…" /></div>
        ) : connectionsQuery.isError ? (
          <div className="p-4"><AdminErrorState message={errorMessage(connectionsQuery.error, "Vault connections could not be loaded.")} retry={() => void connectionsQuery.refetch()} /></div>
        ) : connections.length === 0 ? (
          <EmptyState icon={KeyRound} title="No connections" sub="Register a Vault endpoint to start referencing external secrets." />
        ) : (
          <AdminTable label="Vault connections">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Endpoint</AdminTh>
              <AdminTh>Auth</AdminTh>
              <AdminTh>Credential</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {connections.map((c) => {
                const hint = c.authMethod === "approle" ? (c.secretIdHint || "—") : (c.tokenHint || "—");
                return (
                  <AdminTr key={c.id}>
                    <AdminTd>
                      <div className="font-medium text-slate-100">{c.name}</div>
                      <div className="font-mono text-xs text-slate-500">
                        mount <span className="text-slate-400">{c.mountPath}</span>
                        {c.namespace ? <> · ns <span className="text-slate-400">{c.namespace}</span></> : null}
                        {" "}· KV v{c.engineVersion}
                      </div>
                    </AdminTd>
                    <AdminTd className="max-w-xs truncate font-mono text-xs text-slate-300" title={c.baseUrl}>{c.baseUrl}</AdminTd>
                    <AdminTd><Badge className="bg-blue-500/15 text-blue-300">{c.authMethod}</Badge></AdminTd>
                    <AdminTd className="font-mono text-xs text-slate-400">
                      {hint}
                      {c.authMethod === "approle" && c.roleId ? <div className="text-[11px] text-slate-500">role {c.roleId.slice(0, 8)}</div> : null}
                    </AdminTd>
                    <AdminTd>
                      <Pill tone={c.enabled ? "green" : "neutral"}>{c.enabled ? "enabled" : "disabled"}</Pill>
                      <div className="pt-1 text-xs text-slate-500">{formatDate(c.updatedAt)}</div>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex items-center justify-end gap-1.5">
                        <Btn size="sm" tone="ghost" onClick={() => toggleMut.mutate({ id: c.id, enabled: !c.enabled })}>
                          {c.enabled ? "Disable" : "Enable"}
                        </Btn>
                        <Btn size="sm" tone="subtle" loading={testMut.isPending && testMut.variables === c.id} onClick={() => testMut.mutate(c.id)} title="Test connection">
                          <Plug size={14} className="mr-1.5" /> Test
                        </Btn>
                        <Btn size="sm" tone="ghost" onClick={() => setModal({ mode: "edit", connection: c })}>Edit</Btn>
                        <Btn size="sm" tone="danger" onClick={() => handleDelete(c)} title="Delete connection">
                          <Trash2 size={14} />
                        </Btn>
                      </div>
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {modal ? (
        <ConnectionModal
          mode={modal.mode}
          initial={modal.connection}
          onClose={() => setModal(null)}
          onDone={() => {
            setModal(null);
            refresh();
          }}
        />
      ) : null}

      {renderConfirm()}
    </div>
  );
}

function ConnectionModal({
  mode,
  initial,
  onClose,
  onDone,
}: {
  mode: "create" | "edit";
  initial?: VaultConnection;
  onClose: () => void;
  onDone: () => void;
}) {
  const { toast } = useToast();
  const [form, setForm] = useState<ConnectionForm>(() =>
    initial
      ? {
          name: initial.name,
          baseUrl: initial.baseUrl,
          mountPath: initial.mountPath,
          namespace: initial.namespace ?? "",
          engineVersion: initial.engineVersion,
          authMethod: initial.authMethod,
          // Credentials are never returned; leave blank so an edit preserves the
          // stored secret unless the operator types a new value.
          token: "",
          roleId: initial.roleId ?? "",
          secretId: "",
          enabled: initial.enabled,
        }
      : { ...EMPTY_FORM },
  );

  const set = <K extends keyof ConnectionForm,>(key: K, value: ConnectionForm[K]) => setForm((prev) => ({ ...prev, [key]: value }));

  const saveMut = useMutation({
    mutationFn: () => {
      const base: CreateVaultConnectionInput = {
        name: form.name.trim(),
        baseUrl: form.baseUrl.trim(),
        mountPath: form.mountPath.trim() || "secret",
        namespace: form.namespace.trim() || undefined,
        engineVersion: form.engineVersion,
        authMethod: form.authMethod,
        enabled: form.enabled,
      };
      if (form.authMethod === "token") {
        if (form.token.trim()) base.token = form.token.trim();
      } else {
        if (form.roleId.trim()) base.roleId = form.roleId.trim();
        if (form.secretId.trim()) base.secretId = form.secretId.trim();
      }
      if (mode === "create") return createVaultConnection(base);
      return updateVaultConnection(initial!.id, base);
    },
    onSuccess: () => {
      toast({ tone: "success", title: mode === "create" ? "Connection created" : "Connection updated" });
      onDone();
    },
    onError: (err) => toast({ tone: "error", title: "Could not save connection", message: errorMessage(err) }),
  });

  const needsCredential = mode === "create";
  const tokenMissing = form.authMethod === "token" && needsCredential && !form.token.trim();
  const approleMissing = form.authMethod === "approle" && needsCredential && (!form.roleId.trim() || !form.secretId.trim());
  const valid = form.name.trim().length > 0 && form.baseUrl.trim().length > 0 && !tokenMissing && !approleMissing;

  return (
    <Modal
      open
      wide
      title={mode === "create" ? "New Vault connection" : "Edit Vault connection"}
      onClose={onClose}
      description="An environment variable value of the form vault:&lt;connection-id&gt;/&lt;secret-path&gt;#&lt;field&gt; resolves live against this connection."
    >
      <div className="space-y-4">
        <Input label="Name" value={form.name} onChange={(v) => set("name", v)} placeholder="production-vault" required />
        <Input label="Base URL" value={form.baseUrl} onChange={(v) => set("baseUrl", v)} placeholder="https://vault.example.com:8200" mono required />
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Mount path" value={form.mountPath} onChange={(v) => set("mountPath", v)} placeholder="secret" mono />
          <Input label="Namespace" value={form.namespace} onChange={(v) => set("namespace", v)} placeholder="(optional, Vault Enterprise)" mono />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <AdminSelect
            label="Auth method"
            value={form.authMethod}
            onChange={(v) => set("authMethod", v === "approle" ? "approle" : "token")}
            options={AUTH_METHODS}
          />
          <AdminSelect
            label="KV engine version"
            value={String(form.engineVersion)}
            onChange={(v) => set("engineVersion", v === "1" ? 1 : 2)}
            options={ENGINE_VERSIONS}
          />
        </div>

        {form.authMethod === "token" ? (
          <div className="space-y-1.5">
            <Input
              label="Token"
              value={form.token}
              onChange={(v) => set("token", v)}
              type="password"
              autoComplete="new-password"
              placeholder={mode === "edit" ? "••••••••  (leave blank to keep current)" : "s.abc123…"}
              mono
              required={needsCredential}
            />
            {mode === "edit" ? <p className="text-xs text-slate-500">Stored token hint: <span className="font-mono">{initial?.tokenHint || "—"}</span></p> : null}
          </div>
        ) : (
          <div className="space-y-4">
            <Input label="Role ID" value={form.roleId} onChange={(v) => set("roleId", v)} placeholder="e.g. 7f2c…" mono required={needsCredential} />
            <div className="space-y-1.5">
              <Input
                label="Secret ID"
                value={form.secretId}
                onChange={(v) => set("secretId", v)}
                type="password"
                autoComplete="new-password"
                placeholder={mode === "edit" ? "••••••••  (leave blank to keep current)" : "AppRole secret id"}
                mono
                required={needsCredential}
              />
              {mode === "edit" ? <p className="text-xs text-slate-500">Stored secret hint: <span className="font-mono">{initial?.secretIdHint || "—"}</span></p> : null}
            </div>
          </div>
        )}

        <div className="rounded-lg border border-[var(--line)] p-3">
          <ToggleRow label="Enabled" checked={form.enabled} onChange={(v) => set("enabled", v)} />
        </div>
        {!valid ? (
          <p className="text-xs text-amber-300">
            {mode === "create" && form.authMethod === "token" && tokenMissing
              ? "A token is required for a new token-auth connection."
              : mode === "create" && form.authMethod === "approle" && approleMissing
                ? "Role id and secret id are required for a new AppRole connection."
                : "Name and base URL are required."}
          </p>
        ) : null}
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={() => saveMut.mutate()}
        confirmLabel={mode === "create" ? "Create connection" : "Save changes"}
        disabled={!valid || saveMut.isPending}
      />
    </Modal>
  );
}

function ToggleRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-center justify-between gap-3 text-sm text-slate-300">
      <span>{label}</span>
      <input type="checkbox" className="h-4 w-4 accent-[var(--brand)]" checked={checked} onChange={(e) => onChange(e.target.checked)} />
    </label>
  );
}
