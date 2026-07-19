"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Plus, Save, Shield, Trash2, Users } from "lucide-react";
import { type ApiUser, assignUserRoles, createUser, deleteUser, fetchRoles, fetchServers, fetchUserRoles, fetchUsers, removeUserRoles, updateUser } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, StatsRow } from "./admin-ui";
import { Skeleton } from "@/components/ui/loading-skeleton";
import { UserLimitsGrid } from "./user-limits";

export function AdminUsers() {
 const qc = useQueryClient();
 const { data: users = [], isLoading } = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
 const { data: servers = [] } = useQuery({ queryKey: ["servers"], queryFn: fetchServers });

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
 // Resource limits (create + edit). 0 = unlimited.
 const [cServerLimit, setCServerLimit] = useState(0);
 const [cCPULimit, setCCpuLimit] = useState(0);
 const [cMemLimit, setCMemLimit] = useState(0);
 const [cDiskLimit, setCDiskLimit] = useState(0);
 const [cBackupLimit, setCBackupLimit] = useState(0);
 const [cDatabaseLimit, setCDatabaseLimit] = useState(0);
 const [cAllocationLimit, setCAllocationLimit] = useState(0);
 const [cSubuserLimit, setCSubuserLimit] = useState(0);
 const [cScheduleLimit, setCScheduleLimit] = useState(0);
 const [eServerLimit, setEServerLimit] = useState(0);
 const [eCPULimit, setECpuLimit] = useState(0);
 const [eMemLimit, setEMemLimit] = useState(0);
 const [eDiskLimit, setEDiskLimit] = useState(0);
 const [eBackupLimit, setEBackupLimit] = useState(0);
 const [eDatabaseLimit, setEDatabaseLimit] = useState(0);
 const [eAllocationLimit, setEAllocationLimit] = useState(0);
 const [eSubuserLimit, setESubuserLimit] = useState(0);
 const [eScheduleLimit, setEScheduleLimit] = useState(0);

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
 },
 });
 const updateMut = useMutation({
 mutationFn: () => selectedUser ? updateUser(selectedUser.id, {
 email: editEmail.trim(),
 role: editRole,
 password: editPassword.trim() || undefined,
 cpuLimit: eCPULimit,
 memoryMbLimit: eMemLimit,
 diskMbLimit: eDiskLimit,
 backupLimit: eBackupLimit,
 databaseLimit: eDatabaseLimit,
 allocationLimit: eAllocationLimit,
 subuserLimit: eSubuserLimit,
 scheduleLimit: eScheduleLimit,
 serverLimit: eServerLimit,
 }) : Promise.reject(new Error("no user selected")),
 onSuccess: (updated) => {
 qc.invalidateQueries({ queryKey: ["users"] });
 setSelectedUser(updated);
 setEditPassword("");
 },
 });
 const deleteMut = useMutation({
 mutationFn: (id: string) => deleteUser(id),
 onSuccess: () => {
 qc.invalidateQueries({ queryKey: ["users"] });
 setSelectedUser(null);
 },
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
 },
 });

 const admins = useMemo(() => users.filter((u) => u.role === "admin"), [users]);
 const ownerIdForServer = (server: (typeof servers)[number]) =>
 server.ownerId ?? users.find((user) => user.id === server.owner || user.email === server.owner)?.id;
 const ownedCounts = useMemo(() => {
 const counts = new Map<string, number>();
 for (const server of servers) {
 const ownerId = server.ownerId ?? users.find((user) => user.id === server.owner || user.email === server.owner)?.id;
 if (ownerId) counts.set(ownerId, (counts.get(ownerId) ?? 0) + 1);
 }
 return counts;
 }, [servers, users]);
 const ownershipKnown = (user: ApiUser) => servers.every((server) => ownerIdForServer(server) !== undefined || server.owner !== user.email);
 const ownedCount = (user: ApiUser) => ownedCounts.get(user.id) ?? 0;
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
 setSelectedIds((current) => allFilteredSelected ? current.filter((id) => !filteredIds.includes(id)) : Array.from(new Set([...current, ...filteredIds])));
 };
 const toggleSelected = (id: string) => {
 setSelectedIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
 };
 const openUser = (user: ApiUser) => {
 setSelectedUser(user);
 setEditEmail(user.email);
 setEditRole(user.role === "admin" ? "admin" : "user");
 setEditPassword("");
 setECpuLimit(user.cpuLimit ?? 0);
 setEMemLimit(user.memoryMbLimit ?? 0);
 setEDiskLimit(user.diskMbLimit ?? 0);
 setEBackupLimit(user.backupLimit ?? 0);
 setEDatabaseLimit(user.databaseLimit ?? 0);
 setEAllocationLimit(user.allocationLimit ?? 0);
 setESubuserLimit(user.subuserLimit ?? 0);
 setEScheduleLimit(user.scheduleLimit ?? 0);
 setEServerLimit(user.serverLimit ?? 0);
 };

 return (
 <div>
 <SectionHeader
 title="Users"
 sub="All registered users on this panel."
 action={<Btn onClick={() => setModal(true)}><Plus size={14} /> New User</Btn>}
 />

 <StatsRow items={[
 { label: "Total Users", value: users.length, icon: Users, tone: "neutral" },
 { label: "Administrators", value: admins.length, icon: Shield, tone: "red" },
 { label: "Standard", value: users.length - admins.length, icon: Users, tone: "blue" },
 ]} />

 <div className="mb-4 grid gap-3 md:grid-cols-[1fr_180px]">
 <Input value={search} onChange={(value) => { setSearch(value); setPage(1); }} placeholder="Search by email or role..." />
 <select className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100" value={roleFilter} onChange={(event) => { setRoleFilter(event.target.value as "all" | "admin" | "user"); setPage(1); }}>
 <option value="all">All roles</option>
 <option value="admin">Administrators</option>
 <option value="user">Standard users</option>
 </select>
 </div>
 {selectedIds.length > 0 ? (
 <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-white/[0.08] bg-[#161b28] px-4 py-3">
 <p className="text-sm font-semibold text-slate-300">{selectedIds.length} selected</p>
 <div className="flex flex-wrap gap-2">
 <Btn size="sm" tone="ghost" onClick={() => bulkMut.mutate("role-user")} disabled={bulkMut.isPending}>Set User</Btn>
 <Btn size="sm" tone="ghost" onClick={() => bulkMut.mutate("role-admin")} disabled={bulkMut.isPending}>Set Admin</Btn>
 <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete selected users without owned servers?")) bulkMut.mutate("delete"); }} disabled={bulkMut.isPending || users.filter((user) => selectedIds.includes(user.id)).some((user) => !ownershipKnown(user) || ownedCount(user) > 0)}>Delete Safe</Btn>
 </div>
 </div>
 ) : null}

 <Card>
 <CardHeader title={`${filtered.length} user${filtered.length !== 1 ? "s" : ""}`} icon={Users} />
 {isLoading ? (
 <div className="space-y-3 p-4">
 {Array.from({ length: 6 }).map((_, index) => <Skeleton key={index} className="h-10 w-full" />)}
 </div>
 ) : filtered.length === 0 ? (
 <EmptyState icon={Users} message="No users found." />
 ) : (
 <table className="w-full text-sm">
 <thead>
 <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
 <th className="px-4 py-3">
 <input aria-label="Select all filtered users" type="checkbox" checked={allFilteredSelected} onChange={toggleAllFiltered} />
 </th>
 <th className="px-4 py-3">Email</th>
 <th className="px-4 py-3">Role</th>
 <th className="px-4 py-3">Servers</th>
 <th className="px-4 py-3">ID</th>
 <th className="px-4 py-3" />
 </tr>
 </thead>
 <tbody className="divide-y divide-white/[0.04]">
 {visibleUsers.map((user) => (
 <tr key={user.id} className="hover:bg-white/[0.02]">
 <td className="px-4 py-3">
 <input aria-label={`Select ${user.email}`} type="checkbox" checked={selectedIds.includes(user.id)} onChange={() => toggleSelected(user.id)} />
 </td>
 <td className="px-4 py-3">
 <div className="flex items-center gap-3">
 <div className="flex h-8 w-8 items-center justify-center rounded-full bg-gradient-to-br from-[#dc2626]/30 to-[#dc2626]/10 text-xs font-bold text-[#dc2626]">
 {user.email.charAt(0).toUpperCase()}
 </div>
 <button type="button" className="text-left font-medium text-slate-200 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#dc2626]" onClick={() => openUser(user)}>{user.email}</button>
 </div>
 </td>
 <td className="px-4 py-3">
 <Pill tone={user.role === "admin" ? "red" : "blue"}>{user.role}</Pill>
 </td>
 <td className="px-4 py-3 text-xs text-slate-400">{ownedCount(user)}</td>
 <td className="px-4 py-3 font-mono text-xs text-slate-500">{user.id.slice(0, 8)}...</td>
 <td className="px-4 py-3"><ChevronRight size={14} className="text-slate-500" /></td>
 </tr>
 ))}
 </tbody>
 </table>
 )}
 </Card>
 <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm text-slate-400">
 <span>Showing {visibleUsers.length} of {filtered.length} users</span>
 <div className="flex items-center gap-2">
 <Btn size="sm" tone="ghost" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>Previous</Btn>
 <span className="text-xs font-semibold">Page {Math.min(page, totalPages)} of {totalPages}</span>
 <Btn size="sm" tone="ghost" disabled={page >= totalPages} onClick={() => setPage((current) => Math.min(totalPages, current + 1))}>Next</Btn>
 </div>
 </div>

 {selectedUser ? (
 <Modal title="User Details" onClose={() => setSelectedUser(null)} wide>
 <div className="grid gap-4 md:grid-cols-2">
 <Input label="Email Address" value={editEmail} onChange={setEditEmail} type="email" />
 <div>
 <label className="mb-1.5 block text-sm font-medium text-slate-300">Role</label>
 <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100" value={editRole} onChange={(event) => setEditRole(event.target.value as "admin" | "user")}>
 <option value="user">User</option>
 <option value="admin">Administrator</option>
 </select>
 </div>
 <Input label="New Password" value={editPassword} onChange={setEditPassword} placeholder="Leave blank to keep current password" type="password" />
 <div className="rounded-lg border border-white/[0.06] bg-[#161b28] p-3 text-sm text-slate-300">
 <p className="text-xs uppercase tracking-wide text-slate-500">Owned Servers</p>
 <p className="mt-1 text-2xl font-bold text-slate-100">{ownedCount(selectedUser)}</p>
 </div>
 </div>
 <div className="mt-4"><UserRoleAssignments userId={selectedUser.id} /></div>
 <div className="mt-4">
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
 </div>
 <div className="mt-5 flex flex-wrap justify-between gap-2 border-t border-white/[0.06] pt-4">
 <Btn tone="danger" disabled={!ownershipKnown(selectedUser) || ownedCount(selectedUser) > 0 || deleteMut.isPending} onClick={() => { if (confirm("Delete this user?")) deleteMut.mutate(selectedUser.id); }}>
 <Trash2 size={13} /> Delete
 </Btn>
 <div className="flex gap-2">
 <Btn tone="ghost" onClick={() => setSelectedUser(null)}>Close</Btn>
 <Btn disabled={editEmail.trim() === "" || updateMut.isPending} onClick={() => updateMut.mutate()}><Save size={13} /> Save User</Btn>
 </div>
 </div>
 </Modal>
 ) : null}

 {modal ? (
 <Modal title="Create User" onClose={() => setModal(false)}>
 <div className="grid gap-4">
 <Input label="Email Address" value={email} onChange={setEmail} placeholder="user@example.com" type="email" />
 <Input label="Password" value={password} onChange={setPassword} placeholder="********" type="password" />
 <div>
 <label className="block text-sm font-medium text-slate-300 mb-1.5">Role</label>
 <select
 className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100 text-sm"
 value={role}
 onChange={(e) => setRole(e.target.value as "admin" | "user")}
 >
 <option value="user">User</option>
 <option value="admin">Administrator</option>
 </select>
 </div>
 <UserLimitsGrid
 serverLimit={cServerLimit} onServerLimit={setCServerLimit}
 cpuLimit={cCPULimit} onCpuLimit={setCCpuLimit}
 memLimit={cMemLimit} onMemLimit={setCMemLimit}
 diskLimit={cDiskLimit} onDiskLimit={setCDiskLimit}
 backupLimit={cBackupLimit} onBackupLimit={setCBackupLimit}
 databaseLimit={cDatabaseLimit} onDatabaseLimit={setCDatabaseLimit}
 allocationLimit={cAllocationLimit} onAllocationLimit={setCAllocationLimit}
 subuserLimit={cSubuserLimit} onSubuserLimit={setCSubuserLimit}
 scheduleLimit={cScheduleLimit} onScheduleLimit={setCScheduleLimit}
 />
 </div>
 <ModalFooter
 onCancel={() => setModal(false)}
 onConfirm={() => createMut.mutate()}
 disabled={email.trim() === "" || password.length < 6 || createMut.isPending}
 confirmLabel="Create User"
 />
 </Modal>
 ) : null}
 </div>
 );
}

function UserRoleAssignments({ userId }: { userId: string }) {
 const qc = useQueryClient();
 const rolesQuery = useQuery({ queryKey: ["admin-roles"], queryFn: fetchRoles });
 const assignedQuery = useQuery({ queryKey: ["user-roles", userId], queryFn: () => fetchUserRoles(userId) });
 const assigned = new Set(assignedQuery.data ?? []);
 const refresh = () => void qc.invalidateQueries({ queryKey: ["user-roles", userId] });
 const assignMut = useMutation({ mutationFn: (roleKey: string) => assignUserRoles(userId, [roleKey]), onSuccess: refresh });
 const removeMut = useMutation({ mutationFn: (roleKey: string) => removeUserRoles(userId, [roleKey]), onSuccess: refresh });
 if (rolesQuery.isError || assignedQuery.isError) return <p className="text-xs text-red-300">Additional roles could not be loaded.</p>;
 return <div className="rounded-lg border border-white/[0.06] bg-[#161b28] p-3"><p className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">Additional Roles</p>{(rolesQuery.data ?? []).length === 0 ? <p className="text-xs text-slate-400">No additional roles configured.</p> : <div className="flex flex-wrap gap-2">{(rolesQuery.data ?? []).map((role) => <label className="flex items-center gap-2 rounded border border-white/10 px-2 py-1 text-xs text-slate-300" key={role.id}><input type="checkbox" checked={assigned.has(role.key)} disabled={assignMut.isPending || removeMut.isPending} onChange={(event) => event.target.checked ? assignMut.mutate(role.key) : removeMut.mutate(role.key)}/>{role.name}</label>)}</div>}</div>;
}
