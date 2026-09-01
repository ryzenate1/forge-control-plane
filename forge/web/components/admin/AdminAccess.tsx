"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, KeyRound, Plus, ShieldCheck, Trash2 } from "lucide-react";
import {
  createAdminOAuthClient, createRole, deleteAdminOAuthClient, deleteRole,
  fetchAdminOAuthClients, fetchRoles, fetchUsers, type ApiOAuthClient, type ApiOAuthClientCreation,
} from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { copySecret } from "@/lib/clipboard";
import { AdminErrorState, AdminFormSection, AdminIconButton, AdminLoadingState, AdminPageLayout, AdminSelect, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea } from "./admin-ui";

export function AdminRoles() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const query = useQuery({ queryKey: ["admin-roles"], queryFn: fetchRoles });
  const roles = useMemo(() => (Array.isArray(query.data) ? query.data : []), [query.data]);
  const [open, setOpen] = useState(false);
  const [key, setKey] = useState("");
  const [name, setName] = useState("");
  const [isAdmin, setIsAdmin] = useState(false);
  const createMut = useMutation({
    mutationFn: () => createRole({ key: key.trim(), name: name.trim(), isAdmin }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["admin-roles"] }); setOpen(false); toast({ tone: "success", title: "Role created" }); },
    onError: (error) => toast({ tone: "error", title: "Create failed", message: error instanceof Error ? error.message : "Could not create role" }),
  });
  const deleteMut = useMutation({
    mutationFn: deleteRole,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin-roles"] }),
    onError: (error) => toast({ tone: "error", title: "Delete failed", message: error instanceof Error ? error.message : "Could not delete role" }),
  });
  const deleteRoleConfirmed = async (roleName: string, roleId: string) => {
    const ok = await confirm({
      title: `Delete role ${roleName}?`,
      description: `Users assigned "${roleName}" lose the permissions it grants immediately, and the role cannot be restored with its assignments intact.`,
      danger: true,
      confirmLabel: "Delete role",
    });
    if (ok) deleteMut.mutate(roleId);
  };

  return (
    <AdminPageLayout>
      <SectionHeader action={<Btn onClick={() => setOpen(true)}><Plus size={14} /> New Role</Btn>} />
      <Card>
        <CardHeader title={query.isSuccess ? `${roles.length} role${roles.length === 1 ? "" : "s"}` : "Roles"} icon={ShieldCheck} />
        {/* Stated as a visible limit, not hidden inside a modal footnote: the
            registry describes this page as carrying "permission sets", and the
            backend cannot edit them. */}
        <p className="ui-hint border-b border-line px-4 pb-3">
          This page lists and creates role definitions and deletes them. The control plane does not expose per-role
          permission sets, so there is no matrix to edit here; assigning a role to a user happens on the Users page.
        </p>
        {query.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading roles…" /></div>
        ) : query.isError ? (
          <div className="p-4"><AdminErrorState message={query.error instanceof Error ? query.error.message : "Roles could not be loaded."} retry={() => void query.refetch()} /></div>
        ) : roles.length === 0 ? (
          <EmptyState icon={ShieldCheck} title="No roles" message="No additional roles are defined. Create one to group access for assignment." />
        ) : (
          <AdminTable label="Roles">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Key</AdminTh>
              <AdminTh>Privilege</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {roles.map((role) => (
                <AdminTr key={role.id}>
                  <AdminTd><span className="font-semibold">{role.name}</span></AdminTd>
                  <AdminTd><span className="font-mono text-xs">{role.key}</span></AdminTd>
                  <AdminTd><Pill tone={role.isAdmin ? "danger" : "neutral"}>{role.isAdmin ? "Administrator" : "Scoped"}</Pill></AdminTd>
                  <AdminTd className="text-right">
                    <div className="flex justify-end">
                      <AdminIconButton label={`Delete role ${role.name}`} tone="danger" disabled={deleteMut.isPending} onClick={() => void deleteRoleConfirmed(role.name, role.id)}>
                        <Trash2 size={14} />
                      </AdminIconButton>
                    </div>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
      {open ? (
        <Modal title="Create Role" onClose={() => setOpen(false)}>
          <div className="space-y-4">
            <AdminFormSection title="Role Details">
              <Input label="Name" value={name} onChange={setName} required />
              <Input label="Stable key" value={key} onChange={setKey} mono required />
              <label className="flex gap-2 text-sm text-text">
                <input type="checkbox" checked={isAdmin} onChange={(e) => setIsAdmin(e.target.checked)} className="accent-[var(--brand)]" />
                Grants administrator role
              </label>
            </AdminFormSection>
            <p className="ui-hint">A role stores a name and a key. Permissions are not editable here — the backend does not expose them for editing.</p>
            {createMut.isError ? <AdminErrorState message={createMut.error instanceof Error ? createMut.error.message : "Role could not be created."} /> : null}
          </div>
          <ModalFooter onCancel={() => setOpen(false)} onConfirm={() => createMut.mutate()} disabled={!name.trim() || !key.trim() || createMut.isPending} confirmLabel={createMut.isPending ? "Creating…" : "Create Role"} />
        </Modal>
      ) : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

/**
 * The client's binding, read from the field the API actually sends.
 *
 * `GET /admin/oauth-clients` serialises `store.OAuthClient`, whose scope lives on
 * `scope` ("account" | "server") — there is no `scopes` array on the wire
 * (`forge/api/internal/store/store_oauth2.go:22`). This column used to render
 * `client.scopes?.[0] || "account"`, so every row — including server-bound ones —
 * wore an "account" pill the page had never measured. An absent value now reads
 * as unknown instead of defaulting to the friendlier answer.
 */
function ScopeBadge({ client }: { client: ApiOAuthClient }) {
  const scope = typeof client.scope === "string" ? client.scope.trim() : "";
  if (!scope) return <Pill tone="unknown">Scope not reported</Pill>;
  if (scope === "server") return <Pill tone="warn">server-bound</Pill>;
  if (scope === "account") return <Pill tone="info">account</Pill>;
  return <Pill tone="neutral">{scope}</Pill>;
}

export function AdminOAuthClients() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const usersQuery = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
  const [ownerId, setOwnerId] = useState("");
  const [open, setOpen] = useState(false);
  const [created, setCreated] = useState<ApiOAuthClientCreation | null>(null);
  const [copied, setCopied] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [scope, setScope] = useState<"account" | "server">("account");
  const [serverId, setServerId] = useState("");
  const [scopes, setScopes] = useState("");
  const clientsQuery = useQuery({ queryKey: ["oauth-clients", ownerId], queryFn: () => fetchAdminOAuthClients(ownerId), enabled: Boolean(ownerId) });
  const createMut = useMutation({
    mutationFn: () => createAdminOAuthClient({ userId: ownerId, name: name.trim(), description: description.trim(), scopes: scopes.split(/[,\n]/).map((v) => v.trim()).filter(Boolean), scope, ownerId, serverId: scope === "server" ? serverId.trim() : undefined, allowedScopes: scopes.split(/[,\n]/).map((v) => v.trim()).filter(Boolean) }),
    onSuccess: (result) => { setOpen(false); setCreated(result); void qc.invalidateQueries({ queryKey: ["oauth-clients", ownerId] }); },
    onError: (error) => toast({ tone: "error", title: "Create failed", message: error instanceof Error ? error.message : "Could not create OAuth client" }),
  });
  const deleteMut = useMutation({
    mutationFn: deleteAdminOAuthClient,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["oauth-clients", ownerId] }),
    onError: (error) => toast({ tone: "error", title: "Delete failed", message: error instanceof Error ? error.message : "Could not delete OAuth client" }),
  });
  const clients = useMemo(() => (Array.isArray(clientsQuery.data) ? clientsQuery.data : []), [clientsQuery.data]);
  const users = useMemo(() => (Array.isArray(usersQuery.data) ? usersQuery.data : []), [usersQuery.data]);
  const createdSecret = created?.clientSecret ?? "";

  const revokeClient = async (client: ApiOAuthClient) => {
    const bound = client.scope === "server" ? " It is bound to one server, so only that workload's integration stops." : "";
    const ok = await confirm({
      title: `Revoke OAuth client "${client.name}"?`,
      description: `Nothing can exchange tokens for client_id ${client.clientId || "(id not reported)"} afterwards${bound}. The secret is not recoverable and a revoked client cannot be un-revoked — it has to be created again, which issues a new secret.`,
      danger: true,
      confirmLabel: "Revoke",
    });
    if (ok) deleteMut.mutate(client.id);
  };

  return (
    <AdminPageLayout>
      {/* Creating is not gated on the page-level owner picker: the form collects
          the owner itself, and a client cannot be issued without one. Disabling
          this button hid the only path to the owner field (asserted by
          `test/ui-contracts.test.tsx:93`) and the Create action below carries the
          visible reason instead. */}
      <SectionHeader action={<Btn onClick={() => setOpen(true)}><Plus size={14} /> New Client</Btn>} />
      <div className="max-w-xl">
        <AdminSelect
          label="Owner"
          value={ownerId}
          onChange={setOwnerId}
          placeholder={usersQuery.isLoading ? "Loading users…" : "Select a user…"}
          options={users.map((u) => ({ value: u.id, label: u.email }))}
        />
      </div>
      {/* Owner-scoped reads: no owner chosen means nothing has been fetched, and
          the table must not imply an empty result set. */}
      {!ownerId ? (
        <p className="ui-hint">Clients are owner-scoped. Select an owner above to load their credentials — the list below stays empty because nothing has been read yet, not because the owner has none.</p>
      ) : null}
      {usersQuery.isError ? (
        <AdminErrorState message={`Users could not be loaded: ${usersQuery.error instanceof Error ? usersQuery.error.message : "unknown error"}`} retry={() => void usersQuery.refetch()} />
      ) : null}

      <Card>
        <CardHeader title={clientsQuery.isSuccess ? `${clients.length} client${clients.length === 1 ? "" : "s"}` : "Registered clients"} icon={KeyRound} />
        {!ownerId ? (
          <EmptyState icon={KeyRound} title="Select an owner" message="Nothing has been loaded yet — choose an owner above to list their OAuth clients." />
        ) : clientsQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading OAuth clients…" /></div>
        ) : clientsQuery.isError ? (
          <div className="p-4"><AdminErrorState message={clientsQuery.error instanceof Error ? clientsQuery.error.message : "OAuth clients could not be loaded."} retry={() => void clientsQuery.refetch()} /></div>
        ) : clients.length === 0 ? (
          <EmptyState icon={KeyRound} title="No OAuth clients" message="This user has no OAuth clients. Create one to get started." />
        ) : (
          <AdminTable label="OAuth clients">
            <AdminTHead>
              <AdminTh>Client</AdminTh>
              <AdminTh>Scope</AdminTh>
              <AdminTh>Allowed scopes</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {clients.map((client) => (
                <AdminTr key={client.id}>
                  <AdminTd>
                    <p className="font-semibold">{client.name}</p>
                    <p className="font-mono text-xs text-text-subtle">{client.clientId}</p>
                  </AdminTd>
                  <AdminTd><ScopeBadge client={client} /></AdminTd>
                  <AdminTd><span className="text-xs text-text-subtle">{client.allowedScopes?.length ? client.allowedScopes.join(", ") : "None recorded"}</span></AdminTd>
                  <AdminTd className="text-right">
                    <div className="flex justify-end">
                      <AdminIconButton label={`Revoke OAuth client ${client.name}`} tone="danger" disabled={deleteMut.isPending} onClick={() => void revokeClient(client)}>
                        <Trash2 size={14} />
                      </AdminIconButton>
                    </div>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {open ? (
        <Modal title="Create OAuth Client" onClose={() => setOpen(false)} wide>
          <div className="space-y-4">
            {!ownerId ? <p className="ui-hint">Select an owner below — a client is always issued for one account.</p> : null}
            <AdminSelect label="OAuth client owner" value={ownerId} onChange={setOwnerId} placeholder={usersQuery.isLoading ? "Loading users…" : "Select a user…"} options={users.map((u) => ({ value: u.id, label: u.email }))} />
            <Input label="Name" value={name} onChange={setName} required />
            <Textarea label="Description" value={description} onChange={setDescription} />
            <AdminSelect label="Scope" value={scope} onChange={(v) => setScope(v as "account" | "server")} options={[{ value: "account", label: "Account" }, { value: "server", label: "Server" }]} />
            {scope === "server" ? <Input label="Server ID" value={serverId} onChange={setServerId} mono required /> : null}
            <Textarea label="Allowed scopes (comma or newline separated)" value={scopes} onChange={setScopes} />
            {createMut.isError ? <AdminErrorState message={createMut.error instanceof Error ? createMut.error.message : "OAuth client could not be created."} /> : null}
          </div>
          <ModalFooter onCancel={() => setOpen(false)} onConfirm={() => createMut.mutate()} disabled={!ownerId || !name.trim() || (scope === "server" && !serverId.trim()) || createMut.isPending} confirmLabel={createMut.isPending ? "Creating…" : "Create Client"} />
          {!ownerId || !name.trim() || (scope === "server" && !serverId.trim()) ? (
            <p className="ui-hint mt-2">
              {!ownerId
                ? "Choose the account the client is issued for — credentials are owner-scoped and cannot be reassigned later."
                : !name.trim()
                  ? "Give the client a name so the integration is recognisable when it comes to revoking it."
                  : "A server-scoped client must name the server it is bound to."}
            </p>
          ) : null}
        </Modal>
      ) : null}

      {created ? (
        <Modal title="OAuth client secret" description="Shown once — the server does not store it recoverably" onClose={() => setCreated(null)}>
          <div className="space-y-4">
            <p className="ui-alert ui-alert-warning">
              Copy this now. It cannot be retrieved later, and there is no rotate action for a client — if you dismiss this dialog
              without storing it you will need to revoke the client and create a new one.
            </p>
            <pre className="ui-code-block break-all">{createdSecret || "The create response carried no client secret."}</pre>
            <p className="ui-hint">
              {createdSecret
                ? copied
                  ? "Copied. The clipboard is wiped 15s after a secret copy."
                  : "Copy uses the shared secret clipboard helper, which clears the clipboard after 15 seconds."
                : "Without a secret there is nothing to store — revoke this client and create it again."}
            </p>
          </div>
          <div className="ui-dialog-footer sticky bottom-0 z-10 -mx-5 -mb-4 mt-4">
            <Btn
              disabled={!createdSecret}
              onClick={() => {
                void copySecret(createdSecret).then((ok) => {
                  if (ok) {
                    setCopied(true);
                    setTimeout(() => setCopied(false), 2000);
                  }
                });
              }}
              tone="ghost"
            >
              {copied ? <Check size={14} /> : <Copy size={14} />} Copy secret
            </Btn>
            <Btn onClick={() => setCreated(null)}>I stored it</Btn>
          </div>
        </Modal>
      ) : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
