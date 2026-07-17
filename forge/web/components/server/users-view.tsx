"use client";

import { useState } from "react";
import { User, Users } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiServer, type ApiServerSubuser, deleteServerUser, fetchPermissions, fetchServerUsers, updateServerUser, upsertServerUser } from "@/lib/api";
import { PanelCard } from "@/components/ui/panel-card";
import { hasServerPermission, useOptionalServerContext } from "./server-context";



export function ServerUsersView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canCreate = hasServerPermission(access, "user.create");
  const canUpdate = hasServerPermission(access, "user.update");
  const canDelete = hasServerPermission(access, "user.delete");
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [search, setSearch] = useState("");
  const [selectedPermissions, setSelectedPermissions] = useState<string[]>(["websocket.connect", "control.console", "file.read", "file.sftp"]);
  const [editing, setEditing] = useState<ApiServerSubuser | null>(null);
  const usersQuery = useQuery({ queryKey: ["server-users", server?.id], queryFn: () => fetchServerUsers(server?.id ?? ""), enabled: Boolean(server?.id) });
  const permissionsQuery = useQuery({ queryKey: ["permissions"], queryFn: fetchPermissions });
  const permissionGroups = Object.entries(permissionsQuery.data ?? {}).map(([group, permissions]) => ({
    title: group,
    permissions: Object.entries(permissions).map(([permission, description]) => ({ key: `${group}.${permission}`, description })),
  }));
  const allServerPermissions = permissionGroups.flatMap((group) => group.permissions.map((permission) => permission.key));
  const saveMutation = useMutation({
    mutationFn: () => { const permissions = Array.from(new Set(["websocket.connect", ...selectedPermissions])); return editing ? updateServerUser(server?.id ?? "", editing.userId ?? editing.id, { permissions }) : upsertServerUser(server?.id ?? "", { email: email.trim().toLowerCase(), permissions }); },
    onSuccess: () => { setEmail(""); setEditing(null); setSelectedPermissions(["websocket.connect", "control.console", "file.read", "file.sftp"]); void queryClient.invalidateQueries({ queryKey: ["server-users", server?.id] }); }
  });
  const deleteMutation = useMutation({
    mutationFn: (userId: string) => deleteServerUser(server?.id ?? "", userId),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["server-users", server?.id] })
  });
  const rows = (usersQuery.data ?? []).filter((subuser) => subuser.email.toLowerCase().includes(search.trim().toLowerCase()));
  const actionError = usersQuery.error ?? permissionsQuery.error ?? saveMutation.error ?? deleteMutation.error;
  const togglePermission = (p: string) => setSelectedPermissions((c) => c.includes(p) ? c.filter((i) => i !== p) : [...c, p]);
  const editRow = (subuser: ApiServerSubuser) => { setEditing(subuser); setEmail(subuser.email); setSelectedPermissions(subuser.permissions); };

  return (
    <div className="grid gap-5 lg:grid-cols-[1fr_380px]">
      <PanelCard title="Server Users" icon={Users}>
        <div className="bg-slate-50 p-4 text-slate-900">
          <label className="mb-4 block text-sm font-semibold">Search users<input className="mt-2 h-10 w-full rounded border border-slate-300 px-3" onChange={(event) => setSearch(event.target.value)} placeholder="Filter by email" value={search} /></label>
          {actionError ? <div className="mb-4 rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700" role="alert">{actionError instanceof Error ? actionError.message : "User action failed."}</div> : null}
          {usersQuery.isLoading ? <div className="text-sm text-slate-500">Loading users...</div> : null}
          {!usersQuery.isLoading && rows.length === 0 ? <div className="text-sm text-slate-500">No subusers have access to this server.</div> : null}
          <div className="space-y-3">
            {rows.map((subuser) => (
              <div className="grid gap-3 rounded border border-slate-200 bg-white p-3 md:grid-cols-[1fr_auto]" key={subuser.id}>
                <div>
                  <div className="font-semibold">{subuser.email}</div>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {subuser.permissions.slice(0, 8).map((p) => <span className="rounded bg-slate-100 px-2 py-1 font-mono text-xs text-slate-600" key={p}>{p}</span>)}
                    {subuser.permissions.length > 8 ? <span className="rounded bg-slate-100 px-2 py-1 text-xs text-slate-600">+{subuser.permissions.length - 8}</span> : null}
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <button className="rounded border border-slate-300 px-3 py-2 text-xs font-bold uppercase disabled:opacity-40" disabled={!canUpdate} onClick={() => editRow(subuser)} type="button">Edit</button>
                  <button className="rounded bg-red-600 px-3 py-2 text-xs font-bold uppercase text-white disabled:opacity-60" disabled={!canDelete || deleteMutation.isPending || context?.access.user?.id === (subuser.userId ?? "")} title={context?.access.user?.id === subuser.userId ? "You cannot remove your own access" : "Remove user"} onClick={() => { if (!window.confirm(`Remove ${subuser.email}?`)) return; deleteMutation.mutate(subuser.userId ?? ""); }} type="button">Remove</button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </PanelCard>
      <PanelCard title={editing ? "Edit User Permissions" : "Add User"} icon={User}>
        <div className="space-y-4 bg-slate-50 p-4 text-slate-900">
          <label className="block text-sm font-semibold">Email<input className="mt-2 h-10 w-full border border-slate-300 px-3" disabled={Boolean(editing)} onChange={(e) => setEmail(e.target.value)} value={email} /></label>
          <div className="flex gap-2">
            <button className="rounded border border-slate-300 px-3 py-2 text-xs font-bold uppercase disabled:opacity-60" disabled={permissionsQuery.isLoading || permissionsQuery.isError} onClick={() => setSelectedPermissions(allServerPermissions)} type="button">Select All</button>
            <button className="rounded border border-slate-300 px-3 py-2 text-xs font-bold uppercase" onClick={() => setSelectedPermissions([])} type="button">Clear</button>
          </div>
          <div className="max-h-[520px] space-y-4 overflow-y-auto pr-1">
            {permissionsQuery.isLoading && <p className="text-sm text-slate-500">Loading permissions…</p>}
            {permissionsQuery.isError && <p className="text-sm text-red-700">Permission catalog unavailable. Permission editing is disabled.</p>}
            {permissionGroups.map((group) => (
              <div key={group.title}>
                <div className="mb-2 text-xs font-bold uppercase text-slate-500">{group.title}</div>
                <div className="space-y-2">
                  {group.permissions.map((permission) => (
                    <label className="block rounded border border-slate-200 bg-white px-3 py-2 text-sm" key={permission.key}>
                      <span className="flex items-center gap-2"><input checked={selectedPermissions.includes(permission.key)} onChange={() => togglePermission(permission.key)} type="checkbox" /><span className="font-mono">{permission.key}</span></span>
                      <span className="mt-1 block text-xs text-slate-500">{permission.description}</span>
                    </label>
                  ))}
                </div>
              </div>
            ))}
          </div>
          <div className="flex justify-end gap-2">
            {editing ? <button className="rounded border border-slate-300 px-4 py-2 text-sm font-bold" onClick={() => { setEditing(null); setEmail(""); }} type="button">Cancel</button> : null}
            <button className="rounded bg-[#dc2626] px-4 py-2 text-sm font-bold text-white disabled:opacity-60" disabled={(editing ? !canUpdate : !canCreate) || permissionsQuery.isLoading || permissionsQuery.isError || saveMutation.isPending || email.trim() === "" || selectedPermissions.length === 0} onClick={() => saveMutation.mutate()} type="button">
              {saveMutation.isPending ? "Saving..." : editing ? "Save Permissions" : "Add User"}
            </button>
          </div>
        </div>
      </PanelCard>
    </div>
  );
}
