"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Plus, Save, Shield, Trash2, Users } from "lucide-react";
import { type ApiUser, assignUserRoles, createUser, deleteUser, fetchRoles, fetchServers, fetchUserRoles, fetchUsers, removeUserRoles, updateUser } from "@/lib/api";
import { toast } from "@/components/ui/sonner";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminErrorState, AdminFormSection, AdminLoadingState, AdminPageLayout, AdminSelect, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, StatsRow } from "./admin-ui";
import { Skeleton } from "@/components/ui/loading-skeleton";
import { UserLimitsGrid, type LimitValue, type PasswordPolicy } from "./user-limits";

/** `POST/PUT /users` are validated by `store.ValidatePassword`
 * (`forge/api/internal/store/store_users.go:970`): ≥12 characters with an
 * uppercase, a lowercase, a digit and a special. The form previously gated on
 * `length < 8` and gave no reason at all, so it blocked valid passwords,
 * accepted invalid ones and then showed a server error. */
export const PASSWORD_POLICY: PasswordPolicy = {
  minLength: 12,
  requirements: [
    { label: "12 characters", metBy: (v) => v.length >= 12 },
    { label: "an uppercase letter", metBy: (v) => /[A-Z]/.test(v) },
    { label: "a lowercase letter", metBy: (v) => /[a-z]/.test(v) },
    { label: "a digit", metBy: (v) => /[0-9]/.test(v) },
    { label: "a special character", metBy: (v) => /[^A-Za-z0-9]/.test(v) },
  ],
};

/** A server row may carry the owner as an id, as an email, or as both. Resolving
 * it is a pure lookup over the two loaded lists, so it lives outside the render
 * body — nothing is guessed when neither field matches a known user. */
function ownerIdFor(server: { ownerId?: string; owner?: string }, users: ApiUser[]): string | undefined {
  return server.ownerId ?? users.find((user) => user.id === server.owner || user.email === server.owner)?.id;
}

export function AdminUsers() {  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const usersQuery = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
  const users = useMemo(() => (Array.isArray(usersQuery.data) ? usersQuery.data : []), [usersQuery.data]);
  const usersReady = usersQuery.isSuccess;
  const serversQuery = useQuery({ queryKey: ["servers"], queryFn: fetchServers });
  const servers = useMemo(() => (Array.isArray(serversQuery.data) ? serversQuery.data : []), [serversQuery.data]);
  /** A failed or empty-of-owner server list does not prove a user owns nothing.
   * Deleting on that assumption is how "Delete Safe" became unsafe. */
  const ownershipKnown = usersReady && serversQuery.isSuccess;

  const [modal, setModal] = useState(false);
  const [selectedUser, setSelectedUser] = useState<ApiUser | null>(null);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"admin" | "user">("user");
  const [search, setSearch] = useState("");
  const [roleFilter, setRoleFilter] = useState<"all" | "admin" | "user">("all");
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [editEmail, setEditEmail] = useState("");
  const [editRole, setEditRole] = useState<"admin" | "user">("user");
  const [editPassword, setEditPassword] = useState("");

  // Create: the API has not reported anything yet, so 0 is the operator's
  // explicit "no cap" choice.
  const [cServerLimit, setCServerLimit] = useState(0);
  const [cCPULimit, setCCpuLimit] = useState(0);
  const [cMemLimit, setCMemLimit] = useState(0);
  const [cDiskLimit, setCDiskLimit] = useState(0);
  const [cBackupLimit, setCBackupLimit] = useState(0);
  const [cDatabaseLimit, setCDatabaseLimit] = useState(0);
  const [cAllocationLimit, setCAllocationLimit] = useState(0);
  const [cSubuserLimit, setCSubuserLimit] = useState(0);
  const [cScheduleLimit, setCScheduleLimit] = useState(0);

  // Edit: `null` means the API sent no value. It renders as "Not reported" and
  // is omitted from the PATCH rather than written back as 0 (= unlimited).
  const [eServerLimit, setEServerLimit] = useState<LimitValue>(null);
  const [eCPULimit, setECpuLimit] = useState<LimitValue>(null);
  const [eMemLimit, setEMemLimit] = useState<LimitValue>(null);
  const [eDiskLimit, setEDiskLimit] = useState<LimitValue>(null);
  const [eBackupLimit, setEBackupLimit] = useState<LimitValue>(null);
  const [eDatabaseLimit, setEDatabaseLimit] = useState<LimitValue>(null);
  const [eAllocationLimit, setEAllocationLimit] = useState<LimitValue>(null);
  const [eSubuserLimit, setESubuserLimit] = useState<LimitValue>(null);
  const [eScheduleLimit, setEScheduleLimit] = useState<LimitValue>(null);

  const createMut = useMutation({
    mutationFn: () => createUser({
      email: email.trim(),
      password,
      role,
      cpuLimit: cCPULimit,
      memoryMbLimit: cMemLimit,
      diskMbLimit: cDiskLimit,
      backupLimit: cBackupLimit,
      databaseLimit: cDatabaseLimit,
      allocationLimit: cAllocationLimit,
      subuserLimit: cSubuserLimit,
      scheduleLimit: cScheduleLimit,
      serverLimit: cServerLimit,
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["users"] });
      setModal(false);
      setEmail("");
      setPassword("");
      setRole("user");
      setCServerLimit(0); setCCpuLimit(0); setCMemLimit(0); setCDiskLimit(0);
      setCBackupLimit(0); setCDatabaseLimit(0); setCAllocationLimit(0);
      setCSubuserLimit(0); setCScheduleLimit(0);
      toast.success("User created");
    },
    onError: (e: Error) => toast.error(e.message || "Failed to create user"),
  });

  const passwordProblems = PASSWORD_POLICY.requirements.filter((r) => !r.metBy(password));
  const createBlocked = email.trim() === "" || passwordProblems.length > 0;

  const updateMut = useMutation({
    mutationFn: () => {
      if (!selectedUser) return Promise.reject(new Error("No user selected."));
      return updateUser(selectedUser.id, {
        email: editEmail.trim(),
        role: editRole,
        password: editPassword.trim() || undefined,
        ...(eCPULimit !== null ? { cpuLimit: eCPULimit } : {}),
        ...(eMemLimit !== null ? { memoryMbLimit: eMemLimit } : {}),
        ...(eDiskLimit !== null ? { diskMbLimit: eDiskLimit } : {}),
        ...(eBackupLimit !== null ? { backupLimit: eBackupLimit } : {}),
        ...(eDatabaseLimit !== null ? { databaseLimit: eDatabaseLimit } : {}),
        ...(eAllocationLimit !== null ? { allocationLimit: eAllocationLimit } : {}),
        ...(eSubuserLimit !== null ? { subuserLimit: eSubuserLimit } : {}),
        ...(eScheduleLimit !== null ? { scheduleLimit: eScheduleLimit } : {}),
        ...(eServerLimit !== null ? { serverLimit: eServerLimit } : {}),
      });
    },
    onSuccess: (updated) => {
      qc.invalidateQueries({ queryKey: ["users"] });
      setSelectedUser(updated);
      setEditPassword("");
      toast.success("User updated");
    },
    onError: (e: Error) => toast.error(e.message || "Failed to update user"),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["users"] });
      setSelectedUser(null);
      toast.success("User deleted");
    },
    onError: (e: Error) => toast.error(e.message || "Failed to delete user"),
  });

  const bulkMut = useMutation({
    mutationFn: async (action: "role-admin" | "role-user" | "delete") => {
      for (const user of users.filter((item) => selectedIds.includes(item.id))) {
        if (action === "delete") {
          if (ownedCount(user) === 0) await deleteUser(user.id);
        } else {
          await updateUser(user.id, { email: user.email, role: action === "role-admin" ? "admin" : "user" });
        }
      }
    },
    onSuccess: () => {
      setSelectedIds([]);
      qc.invalidateQueries({ queryKey: ["users"] });
      toast.success("Selected users updated");
    },
    onError: (e: Error) => toast.error(e.message || "Bulk update failed"),
  });

  const admins = useMemo(() => users.filter((u) => u.role === "admin"), [users]);
  const ownedCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const server of servers) {
      const ownerId = ownerIdFor(server, users);
      if (ownerId) counts.set(ownerId, (counts.get(ownerId) ?? 0) + 1);
    }
    return counts;
  }, [servers, users]);
  const serverOwnershipResolvable = (user: ApiUser) =>
    ownershipKnown && servers.every((server) => ownerIdFor(server, users) !== undefined || server.owner !== user.email);
  const ownedCount = (user: ApiUser) => ownedCounts.get(user.id) ?? 0;
  const ownedCountLabel = (user: ApiUser) => (serverOwnershipResolvable(user) ? ownedCount(user) : "Not reported");

  const filtered = useMemo(() => users.filter((u) => {
    const q = search.trim().toLowerCase();
    const matchesSearch = !q || u.email.toLowerCase().includes(q) || u.role.includes(q) || u.id.toLowerCase().includes(q);
    const matchesRole = roleFilter === "all" || u.role === roleFilter;
    return matchesSearch && matchesRole;
  }), [roleFilter, search, users]);
  const pageSize = 50;
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const visibleUsers = filtered.slice((Math.min(page, totalPages) - 1) * pageSize, Math.min(page, totalPages) * pageSize);
  const filteredIds = filtered.map((user) => user.id);
  const allFilteredSelected = filteredIds.length > 0 && filteredIds.every((id) => selectedIds.includes(id));
  const toggleAllFiltered = () => {
    setSelectedIds((current) => (allFilteredSelected ? current.filter((id) => !filteredIds.includes(id)) : Array.from(new Set([...current, ...filteredIds]))));
  };
  const toggleSelected = (id: string) => {
    setSelectedIds((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  };
  const openUser = (user: ApiUser) => {
    setSelectedUser(user);
    setEditEmail(user.email);
    setEditRole(user.role === "admin" ? "admin" : "user");
    setEditPassword("");
    // An absent limit stays absent. Coercing it to 0 used to claim "unlimited"
    // for a field the API never sent — and then save that claim back.
    const read = (v?: number): LimitValue => (typeof v === "number" && Number.isFinite(v) ? v : null);
    setECpuLimit(read(user.cpuLimit));
    setEMemLimit(read(user.memoryMbLimit));
    setEDiskLimit(read(user.diskMbLimit));
    setEBackupLimit(read(user.backupLimit));
    setEDatabaseLimit(read(user.databaseLimit));
    setEAllocationLimit(read(user.allocationLimit));
    setESubuserLimit(read(user.subuserLimit));
    setEScheduleLimit(read(user.scheduleLimit));
    setEServerLimit(read(user.serverLimit));
  };

  const selectedUsers = users.filter((user) => selectedIds.includes(user.id));
  const bulkSafeToDelete = ownershipKnown && selectedUsers.length > 0 && selectedUsers.every((user) => serverOwnershipResolvable(user) && ownedCount(user) === 0);

  const runBulkRole = async (next: "admin" | "user") => {
    const count = selectedIds.length;
    const ok = await confirm({
      title: next === "admin"
        ? `Grant panel administrator to ${count} user${count === 1 ? "" : "s"}?`
        : `Demote ${count} user${count === 1 ? "" : "s"} to standard?`,
      description: next === "admin"
        ? "These accounts gain full control-plane access, including user management and node operations. This cannot be undone automatically."
        : "These accounts lose administrative access immediately. If this panel has few administrators, demoting the wrong account can lock you out of these controls.",
      danger: true,
      confirmLabel: next === "admin" ? "Set Admin" : "Set User",
    });
    if (ok) bulkMut.mutate(next === "admin" ? "role-admin" : "role-user");
  };

  const runBulkDelete = async () => {
    const count = selectedIds.length;
    const ok = await confirm({
      title: `Delete ${count} selected user${count === 1 ? "" : "s"}?`,
      description: "Only accounts the server list confirms own no servers are deleted; anyone with owned servers is skipped. Deletion cannot be undone.",
      danger: true,
      confirmLabel: "Delete",
    });
    if (ok) bulkMut.mutate("delete");
  };

  const deleteSelectedUser = async () => {
    if (!selectedUser) return;
    const ok = await confirm({
      title: `Delete user ${selectedUser.email}?`,
      description: serverOwnershipResolvable(selectedUser)
        ? `This account owns ${ownedCount(selectedUser)} server${ownedCount(selectedUser) === 1 ? "" : "s"}. ${ownedCount(selectedUser) === 0 ? "Deletion cannot be undone." : "Reassign those servers first — this action will not delete them."}`
        : "Server ownership for this account could not be confirmed from the servers list, so deletion is blocked rather than guessed.",
      danger: true,
      confirmLabel: "Delete",
    });
    if (ok) deleteMut.mutate(selectedUser.id);
  };

  return (
    <AdminPageLayout>
      <SectionHeader action={<Btn onClick={() => setModal(true)}><Plus size={14} /> New User</Btn>} />

      <StatsRow items={[
        { label: "Total Users", value: usersReady ? users.length : "Not reported", icon: Users },
        { label: "Administrators", value: usersReady ? admins.length : "Not reported", icon: Shield, tone: "warn" },
        { label: "Standard", value: usersReady ? users.length - admins.length : "Not reported", icon: Users },
      ]} />

      <div className="grid gap-3 md:grid-cols-[1fr_180px]">
        <Input label="Search" value={search} onChange={(value) => { setSearch(value); setPage(1); }} placeholder="Email, role or ID…" />
        <AdminSelect label="Role filter" value={roleFilter} onChange={(v) => { setRoleFilter(v as "all" | "admin" | "user"); setPage(1); }} options={[{ value: "all", label: "All roles" }, { value: "admin", label: "Administrators" }, { value: "user", label: "Standard users" }]} />
      </div>

      {selectedIds.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-3 ui-toolbar" aria-busy={bulkMut.isPending}>
          <p className="text-sm font-semibold text-text">{selectedIds.length} selected</p>
          <div className="flex flex-wrap gap-2">
            <Btn size="sm" tone="ghost" onClick={() => void runBulkRole("user")} disabled={bulkMut.isPending || !usersReady}>Set User</Btn>
            <Btn size="sm" tone="ghost" onClick={() => void runBulkRole("admin")} disabled={bulkMut.isPending || !usersReady}>Set Admin</Btn>
            <Btn size="sm" tone="danger" onClick={() => void runBulkDelete()} disabled={bulkMut.isPending || !bulkSafeToDelete}>Delete Safe</Btn>
          </div>
          {!bulkSafeToDelete && usersReady ? (
            <p className="ui-hint w-full">Delete Safe needs every selected account to own no servers. A selection with owned servers — or one whose ownership the servers list could not resolve — is skipped.</p>
          ) : null}
        </div>
      ) : null}

      {serversQuery.isError ? (
        <AdminErrorState message={`Server ownership could not be loaded: ${serversQuery.error instanceof Error ? serversQuery.error.message : "unknown error"}. Owned-server counts and Delete Safe stay unreported until this read succeeds.`} retry={() => void serversQuery.refetch()} />
      ) : null}

      <Card>
        <CardHeader title={usersReady ? `${filtered.length} user${filtered.length === 1 ? "" : "s"}` : "Users"} icon={Users} />
        {/* Precedence is loading → error → empty. A failed read used to fall
            through to the empty branch, so a panel that could not answer was
            reported as a panel with nobody in it. */}
        {usersQuery.isPending ? (
          <div className="space-y-3 p-4">
            {Array.from({ length: 6 }).map((_, index) => <Skeleton key={index} className="h-10 w-full" />)}
          </div>
        ) : usersQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Users could not be loaded: ${usersQuery.error instanceof Error ? usersQuery.error.message : "unknown error"}. This is an unread list, not a panel with no users.`}
              retry={() => void usersQuery.refetch()}
            />
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Users} title="No users" message={users.length === 0 ? "This panel has no user accounts yet." : "No users match the current search or role filter."} />
        ) : (
          <AdminTable label="Users">
            <AdminTHead>
              {/* The bulk handler this page already had, attached to nothing: the
                  header cell rendered a `sr-only` label and no control. It selects
                  every *matching* user, not just this page, and says so. */}
              <AdminTh>
                <input
                  aria-label={`Select all ${filtered.length} matching user${filtered.length === 1 ? "" : "s"}, including other pages`}
                  type="checkbox"
                  checked={allFilteredSelected}
                  disabled={filteredIds.length === 0}
                  onChange={toggleAllFiltered}
                />
              </AdminTh>
              <AdminTh>Email</AdminTh>
              <AdminTh>Role</AdminTh>
              <AdminTh>Servers</AdminTh>
              <AdminTh>ID</AdminTh>
              <AdminTh><span className="sr-only">Open</span></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {visibleUsers.map((user) => (
                <AdminTr key={user.id} onClick={() => openUser(user)}>
                  <AdminTd>
                    <input aria-label={`Select ${user.email}`} type="checkbox" checked={selectedIds.includes(user.id)} onChange={() => toggleSelected(user.id)} onClick={(event) => event.stopPropagation()} />
                  </AdminTd>
                  <AdminTd>
                    <div className="flex items-center gap-3">
                      <div aria-hidden="true" className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand-subtle font-bold text-brand text-xs">
                        {user.email.charAt(0).toUpperCase()}
                      </div>
                      <button type="button" className="truncate text-left font-medium text-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" onClick={(event) => { event.stopPropagation(); openUser(user); }}>{user.email}</button>
                    </div>
                  </AdminTd>
                  <AdminTd><Pill tone={user.role === "admin" ? "warn" : "neutral"}>{user.role}</Pill></AdminTd>
                  <AdminTd><span className="t-meta">{ownedCountLabel(user)}</span></AdminTd>
                  <AdminTd><span className="font-mono text-xs text-text-subtle" title={user.id}>{user.id.slice(0, 8)}…</span></AdminTd>
                  <AdminTd><ChevronRight aria-hidden="true" size={14} className="text-text-muted" /></AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {usersReady ? (
        <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-text-subtle">
          <span>Showing {visibleUsers.length} of {filtered.length} users</span>
          <div className="flex items-center gap-2">
            <Btn size="sm" tone="ghost" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>Previous</Btn>
            <span className="font-semibold">Page {Math.min(page, totalPages)} of {totalPages}</span>
            <Btn size="sm" tone="ghost" disabled={page >= totalPages} onClick={() => setPage((current) => Math.min(totalPages, current + 1))}>Next</Btn>
          </div>
        </div>
      ) : null}

      {selectedUser ? (
        <Modal title={selectedUser.email} description="User details" onClose={() => setSelectedUser(null)} maxWidth="max-w-6xl">
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <Pill tone={selectedUser.role === "admin" ? "warn" : "neutral"}>{selectedUser.role}</Pill>
              <span className="t-meta">Owned servers: {ownedCountLabel(selectedUser)} · ID <span className="font-mono">{selectedUser.id.slice(0, 8)}…</span></span>
            </div>
            <div className="grid gap-4 md:grid-cols-2" aria-busy={updateMut.isPending || deleteMut.isPending}>
              <AdminFormSection title="Account">
                <Input label="Email Address" value={editEmail} onChange={setEditEmail} type="email" autoComplete="email" />
                <AdminSelect label="Role" value={editRole} onChange={(v) => setEditRole(v as "admin" | "user")} options={[{ value: "user", label: "User" }, { value: "admin", label: "Administrator" }]} />
                <Input label="New Password" value={editPassword} onChange={setEditPassword} placeholder="Leave blank to keep current password" type="password" autoComplete="new-password" />
                {editPassword ? (
                  <p className={passwordProblems.length > 0 ? "ui-field-error" : "ui-hint"}>
                    {passwordProblems.length > 0 ? `Password still needs: ${passwordProblems.map((p) => p.label).join(", ")}.` : "Meets the panel password policy."}
                  </p>
                ) : null}
              </AdminFormSection>
              <AdminFormSection title="Owned Servers">
                <div className="ui-well p-3">
                  <p className="t-meta">Servers this account owns</p>
                  <p className="t-readout mt-1">{ownedCountLabel(selectedUser)}</p>
                  {!serverOwnershipResolvable(selectedUser) ? <p className="ui-hint mt-1">Ownership could not be resolved from the servers list — treated as unknown, not as zero.</p> : null}
                </div>
              </AdminFormSection>
            </div>
            <div><UserRoleAssignments userId={selectedUser.id} /></div>
            <AdminFormSection title="Resource Limits" description="0 means no cap. “Not reported” means the account record carried no value; leaving it untouched keeps whatever is stored.">
              <UserLimitsGrid
                serverLimit={eServerLimit} onServerLimit={setEServerLimit}
                cpuLimit={eCPULimit} onCpuLimit={setECpuLimit}
                memLimit={eMemLimit} onMemLimit={setEMemLimit}
                diskLimit={eDiskLimit} onDiskLimit={setEDiskLimit}
                backupLimit={eBackupLimit} onBackupLimit={setEBackupLimit}
                databaseLimit={eDatabaseLimit} onDatabaseLimit={setEDatabaseLimit}
                allocationLimit={eAllocationLimit} onAllocationLimit={setEAllocationLimit}
                subuserLimit={eSubuserLimit} onSubuserLimit={setESubuserLimit}
                scheduleLimit={eScheduleLimit} onScheduleLimit={setEScheduleLimit}
              />
            </AdminFormSection>
            {updateMut.isError ? <AdminErrorState message={updateMut.error instanceof Error ? updateMut.error.message : "User could not be updated."} retry={() => updateMut.mutate()} /> : null}
            {deleteMut.isError ? <AdminErrorState message={deleteMut.error instanceof Error ? deleteMut.error.message : "User could not be deleted."} /> : null}
          </div>
          <div className="ui-dialog-footer sticky bottom-0 z-10 -mx-5 -mb-4 mt-4">
            <Btn tone="danger" disabled={!serverOwnershipResolvable(selectedUser) || ownedCount(selectedUser) > 0 || deleteMut.isPending} onClick={() => void deleteSelectedUser()}>
              <Trash2 size={13} /> Delete
            </Btn>
            <div className="flex gap-2">
              <Btn tone="ghost" onClick={() => setSelectedUser(null)}>Close</Btn>
              <Btn disabled={editEmail.trim() === "" || updateMut.isPending} loading={updateMut.isPending} onClick={() => updateMut.mutate()}><Save size={13} /> Save User</Btn>
            </div>
          </div>
          {!serverOwnershipResolvable(selectedUser) || ownedCount(selectedUser) > 0 ? (
            <p className="ui-hint mt-2">Delete is disabled: {ownedCount(selectedUser) > 0 ? "this account still owns servers." : "server ownership for this account could not be confirmed."}</p>
          ) : null}
        </Modal>
      ) : null}

      {modal ? (
        <Modal title="Create User" onClose={() => setModal(false)}>
          <div className="space-y-4" aria-busy={createMut.isPending}>
            <AdminFormSection title="Account">
              <Input label="Email Address" value={email} onChange={setEmail} placeholder="user@example.com" type="email" autoComplete="email" required />
              <Input label="Password" value={password} onChange={setPassword} placeholder="********" type="password" autoComplete="new-password" required />
              <p className={password && passwordProblems.length > 0 ? "ui-field-error" : "ui-hint"}>
                {password === ""
                  ? "Needs 12+ characters with an uppercase, a lowercase, a digit and a special character."
                  : passwordProblems.length > 0
                    ? `Still needs: ${passwordProblems.map((p) => p.label).join(", ")}.`
                    : "Meets the panel password policy."}
              </p>
              <AdminSelect label="Role" value={role} onChange={(v) => setRole(v as "admin" | "user")} options={[{ value: "user", label: "User" }, { value: "admin", label: "Administrator" }]} />
            </AdminFormSection>
            <AdminFormSection title="Resource Limits" description="0 means no cap. Unset limits do not restrict this account.">
              <UserLimitsGrid
                serverLimit={cServerLimit} onServerLimit={(v) => setCServerLimit(v ?? 0)}
                cpuLimit={cCPULimit} onCpuLimit={(v) => setCCpuLimit(v ?? 0)}
                memLimit={cMemLimit} onMemLimit={(v) => setCMemLimit(v ?? 0)}
                diskLimit={cDiskLimit} onDiskLimit={(v) => setCDiskLimit(v ?? 0)}
                backupLimit={cBackupLimit} onBackupLimit={(v) => setCBackupLimit(v ?? 0)}
                databaseLimit={cDatabaseLimit} onDatabaseLimit={(v) => setCDatabaseLimit(v ?? 0)}
                allocationLimit={cAllocationLimit} onAllocationLimit={(v) => setCAllocationLimit(v ?? 0)}
                subuserLimit={cSubuserLimit} onSubuserLimit={(v) => setCSubuserLimit(v ?? 0)}
                scheduleLimit={cScheduleLimit} onScheduleLimit={(v) => setCScheduleLimit(v ?? 0)}
              />
            </AdminFormSection>
            {createMut.isError ? <AdminErrorState message={createMut.error instanceof Error ? createMut.error.message : "User could not be created."} /> : null}
          </div>
          <ModalFooter
            onCancel={() => setModal(false)}
            onConfirm={() => createMut.mutate()}
            disabled={createBlocked || createMut.isPending}
            confirmLabel={createMut.isPending ? "Creating…" : "Create User"}
          />
          {createBlocked && !createMut.isPending ? (
            <p className="ui-hint mt-2">
              {email.trim() === "" ? "An email address is required." : `Password still needs: ${passwordProblems.map((p) => p.label).join(", ")}.`}
            </p>
          ) : null}
        </Modal>
      ) : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

function UserRoleAssignments({ userId }: { userId: string }) {
  const qc = useQueryClient();
  const rolesQuery = useQuery({ queryKey: ["admin-roles"], queryFn: fetchRoles });
  const assignedQuery = useQuery({ queryKey: ["user-roles", userId], queryFn: () => fetchUserRoles(userId) });
  const roles = useMemo(() => (Array.isArray(rolesQuery.data) ? rolesQuery.data : []), [rolesQuery.data]);
  const assignedKeys = useMemo(() => (Array.isArray(assignedQuery.data) ? assignedQuery.data : []), [assignedQuery.data]);
  const assigned = useMemo(() => new Set(assignedKeys), [assignedKeys]);
  const refresh = () => void qc.invalidateQueries({ queryKey: ["user-roles", userId] });
  const assignMut = useMutation({ mutationFn: (roleKey: string) => assignUserRoles(userId, [roleKey]), onSuccess: refresh, onError: (e: Error) => toast.error(e.message || "Failed to assign role") });
  const removeMut = useMutation({ mutationFn: (roleKey: string) => removeUserRoles(userId, [roleKey]), onSuccess: refresh, onError: (e: Error) => toast.error(e.message || "Failed to remove role") });
  if (rolesQuery.isError || assignedQuery.isError) {
    const detail = rolesQuery.isError ? `: ${rolesQuery.error.message}` : assignedQuery.isError ? `: ${assignedQuery.error.message}` : "";
    return <AdminErrorState message={`Additional roles could not be loaded${detail}. Role assignment is unavailable until this read succeeds.`} retry={() => { void rolesQuery.refetch(); void assignedQuery.refetch(); }} />;
  }
  if (rolesQuery.isPending || assignedQuery.isPending) return <AdminLoadingState label="Loading role assignments…" />;
  return (
    <div className="ui-well p-3">
      <p className="t-meta mb-1.5 font-semibold uppercase tracking-wider">Additional Roles</p>
      {roles.length === 0 ? (
        <p className="text-xs text-text-subtle">No additional roles are defined. Create some under Roles &amp; Permissions.</p>
      ) : (
        <div className="flex flex-wrap gap-2">
          {roles.map((role) => (
            <label className="flex items-center gap-2 rounded border border-line px-2 py-1 text-xs text-text" key={role.id}>
              <input
                type="checkbox"
                checked={assigned.has(role.key)}
                disabled={assignMut.isPending || removeMut.isPending}
                onChange={(event) => (event.target.checked ? assignMut.mutate(role.key) : removeMut.mutate(role.key))}
              />
              {role.name}
            </label>
          ))}
        </div>
      )}
    </div>
  );
}
