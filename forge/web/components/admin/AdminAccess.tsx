"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, ShieldCheck, Trash2 } from "lucide-react";
import {
  createAdminOAuthClient, createRole, deleteAdminOAuthClient, deleteRole,
  fetchAdminOAuthClients, fetchRoles, fetchUsers, type ApiOAuthClientCreation,
} from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea } from "./admin-ui";

export function AdminRoles() {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["admin-roles"], queryFn: fetchRoles });
  const [open, setOpen] = useState(false); const [key, setKey] = useState(""); const [name, setName] = useState(""); const [isAdmin, setIsAdmin] = useState(false);
  const createMut = useMutation({ mutationFn: () => createRole({ key: key.trim(), name: name.trim(), isAdmin }), onSuccess: () => { void qc.invalidateQueries({ queryKey: ["admin-roles"] }); setOpen(false); } });
  const deleteMut = useMutation({ mutationFn: deleteRole, onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin-roles"] }) });
  const roles = query.data ?? [];
  return <div><SectionHeader title="Roles" sub="Additional Forge Control Plane roles. User assignment is available from the Users page." action={<Btn onClick={() => setOpen(true)}><Plus size={14}/> New Role</Btn>}/>
    <Card><CardHeader title={`${roles.length} roles`} icon={ShieldCheck}/>{query.isError ? <p className="p-4 text-sm text-red-300">Roles could not be loaded.</p> : roles.length === 0 ? <EmptyState icon={ShieldCheck} message="No additional roles configured."/> : <div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="border-b border-white/[0.06] text-left text-xs uppercase text-slate-500"><th className="px-4 py-3">Name</th><th className="px-4 py-3">Key</th><th className="px-4 py-3">Privilege</th><th/></tr></thead><tbody className="divide-y divide-white/[0.04]">{roles.map((role) => <tr key={role.id}><td className="px-4 py-3 font-semibold">{role.name}</td><td className="px-4 py-3 font-mono text-xs">{role.key}</td><td className="px-4 py-3"><Pill tone={role.isAdmin ? "red" : "blue"}>{role.isAdmin ? "Administrator" : "Scoped"}</Pill></td><td className="px-4 py-3 text-right"><Btn size="sm" tone="danger" onClick={() => { if (confirm(`Delete role ${role.name}? Assigned users may be affected.`)) deleteMut.mutate(role.id); }}><Trash2 size={12}/></Btn></td></tr>)}</tbody></table></div>}</Card>
    {open ? <Modal title="Create Role" onClose={() => setOpen(false)}><div className="grid gap-4"><Input label="Name" value={name} onChange={setName}/><Input label="Stable key" value={key} onChange={setKey} mono/><label className="flex gap-2 text-sm text-slate-300"><input type="checkbox" checked={isAdmin} onChange={(e) => setIsAdmin(e.target.checked)}/> Grants administrator role</label><p className="text-xs text-amber-300">The backend currently supports role creation/deletion and assignment, but not editing role permissions.</p></div><ModalFooter onCancel={() => setOpen(false)} onConfirm={() => createMut.mutate()} disabled={!name.trim() || !key.trim() || createMut.isPending} confirmLabel="Create Role"/></Modal> : null}
  </div>;
}

export function AdminOAuthClients() {
  const qc = useQueryClient();
  const usersQuery = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
  const [ownerId, setOwnerId] = useState(""); const [open, setOpen] = useState(false); const [created, setCreated] = useState<ApiOAuthClientCreation | null>(null);
  const [name, setName] = useState(""); const [description, setDescription] = useState(""); const [scope, setScope] = useState<"account" | "server">("account"); const [serverId, setServerId] = useState(""); const [scopes, setScopes] = useState("");
  const clientsQuery = useQuery({ queryKey: ["oauth-clients", ownerId], queryFn: () => fetchAdminOAuthClients(ownerId), enabled: Boolean(ownerId) });
  const createMut = useMutation({ mutationFn: () => createAdminOAuthClient({ userId: ownerId, name: name.trim(), description: description.trim(), scopes: scopes.split(/[,\n]/).map((v) => v.trim()).filter(Boolean), scope, ownerId, serverId: scope === "server" ? serverId.trim() : undefined, allowedScopes: scopes.split(/[,\n]/).map((v) => v.trim()).filter(Boolean) }), onSuccess: (result) => { setOpen(false); setCreated(result); void qc.invalidateQueries({ queryKey: ["oauth-clients", ownerId] }); } });
  const deleteMut = useMutation({ mutationFn: deleteAdminOAuthClient, onSuccess: () => void qc.invalidateQueries({ queryKey: ["oauth-clients", ownerId] }) });
  const clients = clientsQuery.data ?? [];
  return <div><SectionHeader title="OAuth Clients" sub="OAuth clients are owner-scoped; select a user to inspect or create credentials." action={<Btn disabled={!ownerId} onClick={() => setOpen(true)}><Plus size={14}/> New Client</Btn>}/>
    <label className="mb-4 block max-w-xl text-sm text-slate-300">Owner<select className="mt-1 h-10 w-full rounded-lg border border-white/10 bg-[#161b28] px-3" value={ownerId} onChange={(e) => setOwnerId(e.target.value)}><option value="">Select a user…</option>{(usersQuery.data ?? []).map((u) => <option value={u.id} key={u.id}>{u.email}</option>)}</select></label>
    <Card><CardHeader title="Registered clients" icon={KeyRound}/>{!ownerId ? <EmptyState icon={KeyRound} message="Select an owner to load OAuth clients."/> : clientsQuery.isError ? <p className="p-4 text-sm text-red-300">OAuth clients could not be loaded.</p> : clients.length === 0 ? <EmptyState icon={KeyRound} message="This user has no OAuth clients."/> : <div className="overflow-x-auto"><table className="w-full text-sm"><tbody className="divide-y divide-white/[0.04]">{clients.map((client) => <tr key={client.id}><td className="px-4 py-3"><p className="font-semibold">{client.name}</p><p className="font-mono text-xs text-slate-500">{client.clientId}</p></td><td className="px-4 py-3"><Pill tone="blue">{client.scopes?.[0] || "account"}</Pill></td><td className="px-4 py-3 text-xs text-slate-400">{client.allowedScopes?.join(", ") || "No scopes"}</td><td className="px-4 py-3 text-right"><Btn size="sm" tone="danger" onClick={() => { if (confirm("Revoke this OAuth client?")) deleteMut.mutate(client.id); }}><Trash2 size={12}/></Btn></td></tr>)}</tbody></table></div>}</Card>
    {open ? <Modal title="Create OAuth Client" onClose={() => setOpen(false)}><div className="grid gap-4"><Input label="Name" value={name} onChange={setName}/><Textarea label="Description" value={description} onChange={setDescription}/><label className="text-sm text-slate-300">Scope<select className="mt-1 h-9 w-full rounded border border-white/10 bg-[#161b28] px-3" value={scope} onChange={(e) => setScope(e.target.value as "account" | "server")}><option value="account">Account</option><option value="server">Server</option></select></label>{scope === "server" ? <Input label="Server ID" value={serverId} onChange={setServerId} mono/> : null}<Textarea label="Allowed scopes (comma or newline separated)" value={scopes} onChange={setScopes}/></div><ModalFooter onCancel={() => setOpen(false)} onConfirm={() => createMut.mutate()} disabled={!name.trim() || (scope === "server" && !serverId.trim()) || createMut.isPending} confirmLabel="Create Client"/></Modal> : null}
    {created ? <Modal title="OAuth Secret — shown once" onClose={() => setCreated(null)}><p className="text-sm text-amber-300">Store this secret now. It cannot be retrieved later.</p><pre className="mt-3 overflow-auto rounded bg-black/30 p-3 text-xs text-emerald-300">{created.clientSecret}</pre><div className="mt-4 flex justify-end"><Btn onClick={() => setCreated(null)}>I stored it</Btn></div></Modal> : null}
  </div>;
}
