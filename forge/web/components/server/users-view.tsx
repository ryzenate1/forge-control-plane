"use client";

import { useState } from "react";
import { User, Users } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiServer, type ApiServerSubuser, deleteServerUser, fetchPermissions, fetchServerUsers, updateServerUser, upsertServerUser } from "@/lib/api";
import { PanelCard } from "@/components/ui/panel-card";
import { ConfirmDialog, EmptyState } from "@/components/ui/primitives";
import { Skeleton } from "@/components/ui/loading-skeleton";
import { useToast } from "@/components/ui/toast";
import { hasServerPermission, useOptionalServerContext } from "./server-context";



export function ServerUsersView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canCreate = hasServerPermission(access, "user.create");
  const canUpdate = hasServerPermission(access, "user.update");
  const canDelete = hasServerPermission(access, "user.delete");
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [email, setEmail] = useState("");
  const [search, setSearch] = useState("");
  const [selectedPermissions, setSelectedPermissions] = useState<string[]>(["websocket.connect", "control.console", "file.read", "file.sftp"]);
  const [editing, setEditing] = useState<ApiServerSubuser | null>(null);
  const [removeTarget, setRemoveTarget] = useState<ApiServerSubuser | null>(null);
  const usersQuery = useQuery({ queryKey: ["server-users", server?.id], queryFn: () => fetchServerUsers(server?.id ?? ""), enabled: Boolean(server?.id) });
  const permissionsQuery = useQuery({ queryKey: ["permissions"], queryFn: fetchPermissions });
  const permissionGroups = Object.entries(permissionsQuery.data ?? {}).map(([group, permissions]) => ({
    title: group,
    permissions: Object.entries(permissions).map(([permission, description]) => ({ key: `${group}.${permission}`, description })),
  }));
  const allServerPermissions = permissionGroups.flatMap((group) => group.permissions.map((permission) => permission.key));
  const saveMutation = useMutation({
    mutationFn: () => { const permissions = Array.from(new Set(["websocket.connect", ...selectedPermissions])); return editing ? updateServerUser(server?.id ?? "", editing.userId ?? editing.id, { permissions }) : upsertServerUser(server?.id ?? "", { email: email.trim().toLowerCase(), permissions }); },
    onSuccess: () => { setEmail(""); setEditing(null); setSelectedPermissions(["websocket.connect", "control.console", "file.read", "file.sftp"]); void queryClient.invalidateQueries({ queryKey: ["server-users", server?.id] }); },
    onError: (error) => toast({ tone: "error", title: "Could not save user", message: error instanceof Error ? error.message : "Check the email address and your permissions, then try again." })
  });
  const deleteMutation = useMutation({
    mutationFn: (userId: string) => deleteServerUser(server?.id ?? "", userId),
    onSuccess: () => { setRemoveTarget(null); void queryClient.invalidateQueries({ queryKey: ["server-users", server?.id] }); },
    onError: (error) => toast({ tone: "error", title: "Could not remove user", message: error instanceof Error ? error.message : "The user was not removed. Try again." })
  });
  const rows = (usersQuery.data ?? []).filter((subuser) => subuser.email.toLowerCase().includes(search.trim().toLowerCase()));
  const actionError = usersQuery.error ?? permissionsQuery.error ?? saveMutation.error ?? deleteMutation.error;
  const togglePermission = (p: string) => setSelectedPermissions((c) => c.includes(p) ? c.filter((i) => i !== p) : [...c, p]);
  const editRow = (subuser: ApiServerSubuser) => { setEditing(subuser); setEmail(subuser.email); setSelectedPermissions(subuser.permissions); };

  return (
    <div className="grid gap-5 lg:grid-cols-[1fr_380px]">
      <PanelCard title="Server Users" icon={Users}>
        <div className="space-y-4">
          <label className="ui-label" htmlFor="user-search">Search users</label>
          <input id="user-search" className="ui-input" onChange={(event) => setSearch(event.target.value)} placeholder="Filter by email" value={search} />
          {actionError ? <div className="ui-alert ui-alert-error" role="alert"><p className="text-sm">{actionError instanceof Error ? actionError.message : "The user action failed. Check your permissions and try again."}</p></div> : null}
          {usersQuery.isLoading ? <div className="space-y-3">{Array.from({ length: 3 }).map((_, index) => <Skeleton className="h-16 w-full" key={index} />)}</div> : null}
          {!usersQuery.isLoading && !usersQuery.isError && rows.length === 0 ? <EmptyState icon={<Users size={20} />} title={search ? "No users match this filter" : "No subusers with access"} description={search ? "Try a different email filter." : "Add a user with the form to grant server access."} /> : null}
          <div className="space-y-3">
            {rows.map((subuser) => (
              <div className="grid gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 md:grid-cols-[1fr_auto]" key={subuser.id}>
                <div className="min-w-0">
                  <div className="font-semibold text-slate-100">{subuser.email}</div>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {subuser.permissions.slice(0, 8).map((p) => <span className="rounded bg-white/[0.04] px-2 py-1 font-mono text-xs text-slate-400" key={p}>{p}</span>)}
                    {subuser.permissions.length > 8 ? <span className="rounded bg-white/[0.04] px-2 py-1 text-xs text-slate-500">+{subuser.permissions.length - 8}</span> : null}
                  </div>
                </div>
                <div className="flex items-start gap-2">
                  <button className="ui-button ui-button-secondary" disabled={!canUpdate} onClick={() => editRow(subuser)} type="button">Edit</button>
                  <button className="ui-button ui-button-danger" disabled={!canDelete || deleteMutation.isPending || context?.access.user?.id === (subuser.userId ?? "")} title={context?.access.user?.id === subuser.userId ? "You cannot remove your own access" : "Remove user"} onClick={() => setRemoveTarget(subuser)} type="button">Remove</button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </PanelCard>
      <PanelCard title={editing ? "Edit User Permissions" : "Add User"} icon={User}>
        <div className="space-y-4">
          <label className="ui-label" htmlFor="user-email">Email</label>
          <input id="user-email" className="ui-input" disabled={Boolean(editing)} onChange={(e) => setEmail(e.target.value)} value={email} />
          <div className="flex gap-2">
            <button className="ui-button ui-button-secondary" disabled={permissionsQuery.isLoading || permissionsQuery.isError} onClick={() => setSelectedPermissions(allServerPermissions)} type="button">Select all</button>
            <button className="ui-button ui-button-secondary" onClick={() => setSelectedPermissions([])} type="button">Clear</button>
          </div>
          <div className="max-h-[520px] space-y-4 overflow-y-auto pr-1">
            {permissionsQuery.isLoading && <p className="ui-hint">Loading permissions…</p>}
            {permissionsQuery.isError && <div className="ui-alert ui-alert-error" role="alert"><p className="text-sm">The permission catalog could not be loaded, so permission editing is disabled. Reload the page or check the API connection.</p></div>}
            {permissionGroups.map((group) => (
              <div key={group.title}>
                <div className="mb-2 text-xs font-bold uppercase tracking-wider text-slate-500">{group.title}</div>
                <div className="space-y-2">
                  {group.permissions.map((permission) => (
                    <label className="block rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 py-2 text-sm" key={permission.key}>
                      <span className="flex items-center gap-2 text-slate-200"><input checked={selectedPermissions.includes(permission.key)} className="accent-red-600" onChange={() => togglePermission(permission.key)} type="checkbox" /><span className="font-mono">{permission.key}</span></span>
                      <span className="mt-1 block text-xs text-slate-500">{permission.description}</span>
                    </label>
                  ))}
                </div>
              </div>
            ))}
          </div>
          <div className="flex justify-end gap-2">
            {editing ? <button className="ui-button ui-button-ghost" onClick={() => { setEditing(null); setEmail(""); }} type="button">Cancel</button> : null}
            <button className="ui-button ui-button-primary" disabled={(editing ? !canUpdate : !canCreate) || permissionsQuery.isLoading || permissionsQuery.isError || saveMutation.isPending || email.trim() === "" || selectedPermissions.length === 0} onClick={() => saveMutation.mutate()} type="button">
              {saveMutation.isPending ? "Saving..." : editing ? "Save permissions" : "Add user"}
            </button>
          </div>
        </div>
      </PanelCard>
      <ConfirmDialog
        confirmAction={() => { if (removeTarget) deleteMutation.mutate(removeTarget.userId ?? removeTarget.id); }}
        confirmLabel={removeTarget ? `Remove ${removeTarget.email}` : "Remove user"}
        description={`${removeTarget?.email ?? "This user"} will immediately lose all access to this server. This action cannot be undone.`}
        destructive
        loading={deleteMutation.isPending}
        closeAction={() => { if (!deleteMutation.isPending) setRemoveTarget(null); }}
        open={Boolean(removeTarget)}
        title={removeTarget ? `Remove ${removeTarget.email}?` : ""}
      />
    </div>
  );
}
