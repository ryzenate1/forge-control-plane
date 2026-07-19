"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { GanttChart, HeartPulse, Network, Plus, Target, Trash2, Zap, type LucideIcon } from "lucide-react";
import { deleteJSON, fetchJSON, patchJSON, postJSON, putJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";

type Algorithm = "round_robin" | "least_connections" | "ip_hash" | "weighted_round_robin";
type TargetStatus = "healthy" | "unhealthy" | "draining";

type Target = {
  id: string;
  serverId: string;
  nodeId: string;
  ip: string;
  port: number;
  weight: number;
  status: TargetStatus;
  connections: number;
};

type TargetGroup = {
  id: string;
  name: string;
  algorithm: Algorithm;
  port: number;
  protocol: string;
  targets: Target[];
  createdAt: string;
  updatedAt: string;
};

type ApiResponse<T> = { data: T };
type GroupForm = Pick<TargetGroup, "name" | "algorithm" | "port" | "protocol">;
type TargetForm = { serverId: string; nodeId: string; ip: string; port: number; weight: number };

const defaultGroupForm: GroupForm = {
  name: "",
  algorithm: "round_robin",
  port: 25565,
  protocol: "tcp",
};

const defaultTargetForm: TargetForm = {
  serverId: "",
  nodeId: "",
  ip: "",
  port: 25565,
  weight: 1,
};

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "The request failed. Please try again.";
}

function formatAlgorithm(algorithm: Algorithm): string {
  return algorithm.replaceAll("_", " ");
}

export default function AdminLoadBalancerPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [showCreateGroup, setShowCreateGroup] = useState(false);
  const [editingGroup, setEditingGroup] = useState<TargetGroup | null>(null);
  const [groupForm, setGroupForm] = useState<GroupForm>(defaultGroupForm);
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [showAddTarget, setShowAddTarget] = useState(false);
  const [targetForm, setTargetForm] = useState<TargetForm>(defaultTargetForm);
  const [testResult, setTestResult] = useState<Target | null>(null);

  const groupsQuery = useQuery({
    queryKey: ["admin", "load-balancer", "groups"],
    queryFn: () => fetchJSON<TargetGroup[]>("/admin/load-balancer/groups"),
  });

  const groups = useMemo(() => groupsQuery.data ?? [], [groupsQuery.data]);
  const selectedGroup = groups.find((group) => group.id === selectedGroupId);
  const filtered = useMemo(
    () => groups.filter((group) => !search || group.name.toLowerCase().includes(search.toLowerCase())),
    [groups, search],
  );
  const allTargets = useMemo(() => groups.flatMap((group) => group.targets ?? []), [groups]);

  const refreshGroups = () => queryClient.invalidateQueries({ queryKey: ["admin", "load-balancer", "groups"] });

  const createGroupMutation = useMutation({
    mutationFn: (data: GroupForm) => postJSON<ApiResponse<TargetGroup>>("/admin/load-balancer/groups", data),
    onSuccess: () => {
      refreshGroups();
      setShowCreateGroup(false);
      setGroupForm(defaultGroupForm);
    },
  });

  const updateGroupMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: GroupForm }) =>
      putJSON<ApiResponse<TargetGroup>>(`/admin/load-balancer/groups/${encodeURIComponent(id)}`, data),
    onSuccess: () => {
      refreshGroups();
      setEditingGroup(null);
      setGroupForm(defaultGroupForm);
    },
  });

  const deleteGroupMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/load-balancer/groups/${encodeURIComponent(id)}`),
    onSuccess: (_data, id) => {
      refreshGroups();
      if (selectedGroupId === id) {
        setSelectedGroupId(null);
        setTestResult(null);
      }
    },
  });

  const addTargetMutation = useMutation({
    mutationFn: ({ groupId, data }: { groupId: string; data: TargetForm }) =>
      postJSON<ApiResponse<Target>>(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/targets`, data),
    onSuccess: () => {
      refreshGroups();
      setShowAddTarget(false);
      setTargetForm(defaultTargetForm);
    },
  });

  const removeTargetMutation = useMutation({
    mutationFn: ({ groupId, targetId }: { groupId: string; targetId: string }) =>
      deleteJSON(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(targetId)}`),
    onSuccess: refreshGroups,
  });

  const targetStatusMutation = useMutation({
    mutationFn: ({ groupId, targetId, status }: { groupId: string; targetId: string; status: TargetStatus }) =>
      patchJSON(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/targets/${encodeURIComponent(targetId)}`, { status }),
    onSuccess: refreshGroups,
  });

  const testSelectionMutation = useMutation({
    mutationFn: (groupId: string) =>
      fetchJSON<ApiResponse<Target>>(`/admin/load-balancer/groups/${encodeURIComponent(groupId)}/next`),
    onSuccess: ({ data }) => setTestResult(data),
  });

  const groupError = groupsQuery.isError ? errorMessage(groupsQuery.error) : null;
  const mutationError = [createGroupMutation, updateGroupMutation, deleteGroupMutation, addTargetMutation, removeTargetMutation, targetStatusMutation, testSelectionMutation]
    .find((mutation) => mutation.isError)?.error;

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Load Balancer"
        sub="Manage target groups and traffic routing for game servers."
        action={<Btn tone="primary" onClick={() => { setGroupForm(defaultGroupForm); setShowCreateGroup(true); }}><Plus size={14} /> Create Target Group</Btn>}
      />

      {(groupError || mutationError) && (
        <div role="alert" className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-200">
          {groupError ?? errorMessage(mutationError)}
          {groupError && <button className="ml-3 underline hover:text-white" onClick={() => groupsQuery.refetch()}>Retry</button>}
        </div>
      )}

      <div className="grid gap-4 md:grid-cols-3">
        <MetricCard icon={GanttChart} label="Target Groups" value={groups.length} />
        <MetricCard icon={Target} label="Targets" value={allTargets.length} />
        <MetricCard icon={HeartPulse} label="Healthy" value={`${allTargets.filter((target) => target.status === "healthy").length} / ${allTargets.length}`} tone="text-emerald-400" />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader title="Target Groups" icon={GanttChart} />
          <div className="flex items-center gap-3 p-4"><Input placeholder="Search groups" value={search} onChange={setSearch} /></div>
          {groupsQuery.isLoading ? <Loading message="Loading groups..." /> : groupError ? <EmptyState icon={GanttChart} message="Target groups could not be loaded." /> : filtered.length === 0 ? <EmptyState icon={GanttChart} message={search ? "No target groups match your search." : "No target groups."} /> : (
            <div className="divide-y divide-white/[0.04]">
              {filtered.map((group) => (
                <div key={group.id} className={`flex items-center justify-between px-4 py-3 cursor-pointer hover:bg-white/[0.02] ${selectedGroupId === group.id ? "bg-white/[0.04]" : ""}`} onClick={() => { setSelectedGroupId(group.id); setTestResult(null); }}>
                  <div>
                    <p className="text-sm font-medium text-slate-200">{group.name}</p>
                    <p className="text-xs text-slate-500">{group.protocol.toUpperCase()} :{group.port} — {group.targets?.length ?? 0} targets</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone={group.algorithm === "round_robin" ? "blue" : group.algorithm === "least_connections" ? "green" : group.algorithm === "ip_hash" ? "yellow" : "neutral"}>{formatAlgorithm(group.algorithm)}</Pill>
                    <div onClick={(event) => event.stopPropagation()}><Btn size="sm" tone="ghost" onClick={() => { setEditingGroup(group); setGroupForm({ name: group.name, algorithm: group.algorithm, port: group.port, protocol: group.protocol }); }}>Edit</Btn></div>
                    <div onClick={(event) => event.stopPropagation()}><Btn size="sm" tone="danger" disabled={deleteGroupMutation.isPending} onClick={() => { if (confirm(`Delete target group ${group.name}?`)) deleteGroupMutation.mutate(group.id); }}><Trash2 size={12} /></Btn></div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card>
          <CardHeader title={selectedGroup ? `Targets: ${selectedGroup.name}` : "Select a Group"} action={selectedGroup ? <div className="flex gap-2"><Btn size="sm" tone="ghost" onClick={() => testSelectionMutation.mutate(selectedGroup.id)} disabled={testSelectionMutation.isPending}><Zap size={12} /> Test Next</Btn><Btn size="sm" tone="primary" onClick={() => { setTargetForm({ ...defaultTargetForm, port: selectedGroup.port }); setShowAddTarget(true); }}><Plus size={12} /> Add Target</Btn></div> : null} />
          {!selectedGroup ? <EmptyState icon={Network} message="Select a target group to manage its targets." /> : selectedGroup.targets?.length === 0 ? <EmptyState icon={Target} message="No targets in this group." /> : (
            <div className="divide-y divide-white/[0.04]">
              {selectedGroup.targets.map((target) => <TargetRow key={target.id} target={target} onStatus={(status) => targetStatusMutation.mutate({ groupId: selectedGroup.id, targetId: target.id, status })} onRemove={() => removeTargetMutation.mutate({ groupId: selectedGroup.id, targetId: target.id })} removing={removeTargetMutation.isPending || targetStatusMutation.isPending} />)}
            </div>
          )}
          {testResult && <div className="border-t border-white/[0.06] px-4 py-3"><p className="text-xs text-slate-400">Next target selected: <span className="font-mono text-emerald-400">{testResult.ip}:{testResult.port}</span><button className="ml-2 text-slate-500 hover:text-slate-200" onClick={() => setTestResult(null)}>✕</button></p></div>}
        </Card>
      </div>

      {showCreateGroup && <TargetGroupFormModal title="Create Target Group" form={groupForm} onChange={setGroupForm} onSave={() => createGroupMutation.mutate(groupForm)} onClose={() => setShowCreateGroup(false)} saving={createGroupMutation.isPending} />}
      {editingGroup && <TargetGroupFormModal title="Edit Target Group" form={groupForm} onChange={setGroupForm} onSave={() => updateGroupMutation.mutate({ id: editingGroup.id, data: groupForm })} onClose={() => setEditingGroup(null)} saving={updateGroupMutation.isPending} />}
      {showAddTarget && selectedGroup && <TargetFormModal form={targetForm} onChange={setTargetForm} onSave={() => addTargetMutation.mutate({ groupId: selectedGroup.id, data: targetForm })} onClose={() => setShowAddTarget(false)} saving={addTargetMutation.isPending} />}
    </div>
  );
}

function MetricCard({ icon: Icon, label, value, tone = "text-slate-100" }: { icon: LucideIcon; label: string; value: string | number; tone?: string }) {
  return <Card className="p-4"><div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1"><Icon size={12} /> {label}</div><div className={`text-2xl font-bold ${tone}`}>{value}</div></Card>;
}

function Loading({ message }: { message: string }) { return <div className="p-8 text-center text-sm text-slate-500">{message}</div>; }

function TargetRow({ target, onStatus, onRemove, removing }: { target: Target; onStatus: (status: TargetStatus) => void; onRemove: () => void; removing: boolean }) {
  const color = target.status === "healthy" ? "bg-emerald-400" : target.status === "draining" ? "bg-amber-400" : "bg-red-400";
  return <div className="flex items-center justify-between px-4 py-3"><div className="flex items-center gap-3"><div className={`h-2 w-2 rounded-full ${color}`} /><div><p className="text-sm font-mono text-slate-200">{target.ip}:{target.port}</p><p className="text-xs text-slate-500">{target.status} — weight: {target.weight} — connections: {target.connections}</p></div></div><div className="flex items-center gap-2"><select aria-label="Target status" className="h-8 rounded-md border border-white/10 bg-[#161b28] px-2 text-xs text-slate-200" disabled={removing} value={target.status} onChange={(event) => onStatus(event.target.value as TargetStatus)}><option value="healthy">Healthy</option><option value="draining">Draining</option><option value="unhealthy">Unhealthy</option></select><Btn size="sm" tone="danger" disabled={removing} onClick={onRemove}><Trash2 size={12} /></Btn></div></div>;
}

function TargetGroupFormModal({ title, form, onChange, onSave, onClose, saving }: { title: string; form: GroupForm; onChange: (form: GroupForm) => void; onSave: () => void; onClose: () => void; saving: boolean }) {
  return <Modal title={title} onClose={onClose}><div className="grid gap-4"><Input label="Name" value={form.name} onChange={(name) => onChange({ ...form, name })} placeholder="prod-game-servers" /><div><label className="block text-sm font-medium text-slate-300 mb-1.5">Algorithm</label><select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30" value={form.algorithm} onChange={(event) => onChange({ ...form, algorithm: event.target.value as Algorithm })}><option value="round_robin">Round Robin</option><option value="least_connections">Least Connections</option><option value="ip_hash">IP Hash</option><option value="weighted_round_robin">Weighted Round Robin</option></select></div><Input label="Listening Port" type="number" value={String(form.port)} onChange={(port) => onChange({ ...form, port: Number(port) })} /><Input label="Protocol" value={form.protocol} onChange={(protocol) => onChange({ ...form, protocol })} placeholder="tcp" /></div><ModalFooter onCancel={onClose} onConfirm={onSave} confirmLabel={saving ? "Saving..." : "Save"} disabled={saving || !form.name.trim() || !form.protocol.trim() || form.port < 1 || form.port > 65535} /></Modal>;
}

function TargetFormModal({ form, onChange, onSave, onClose, saving }: { form: TargetForm; onChange: (form: TargetForm) => void; onSave: () => void; onClose: () => void; saving: boolean }) {
  return <Modal title="Add Target" onClose={onClose}><div className="grid gap-4"><Input label="Server ID" value={form.serverId} onChange={(serverId) => onChange({ ...form, serverId })} placeholder="server UUID" /><Input label="Node ID" value={form.nodeId} onChange={(nodeId) => onChange({ ...form, nodeId })} placeholder="node UUID (optional)" /><Input label="IP Address" value={form.ip} onChange={(ip) => onChange({ ...form, ip })} placeholder="192.168.1.10" /><Input label="Port" type="number" value={String(form.port)} onChange={(port) => onChange({ ...form, port: Number(port) })} /><Input label="Weight" type="number" value={String(form.weight)} onChange={(weight) => onChange({ ...form, weight: Number(weight) })} /></div><ModalFooter onCancel={onClose} onConfirm={onSave} confirmLabel={saving ? "Adding..." : "Add"} disabled={saving || !form.serverId.trim() || !form.ip.trim() || form.port < 1 || form.port > 65535 || form.weight < 1} /></Modal>;
}
