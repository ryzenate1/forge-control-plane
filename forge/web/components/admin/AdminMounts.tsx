"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, EggOff, HardDrive, Link2Off, Plus, Save, Trash2 } from "lucide-react";
import {
  attachEggsToMount, attachNodesToMount, createMount,
  deleteMount, detachEggFromMount, detachNodeFromMount, fetchEggs, fetchMounts, fetchNests, fetchNodes,
  updateMount, type ApiEgg, type ApiMount, type ApiNode,
} from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminBackButton, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd } from "./admin-ui";

type FieldErrors = {
  name?: string;
  source?: string;
  target?: string;
};

function validateMount(name: string, source: string, target: string): FieldErrors {
  const errors: FieldErrors = {};
  if (!name.trim()) errors.name = "Name is required";
  if (!source.trim()) errors.source = "Source path is required";
  if (!target.trim()) errors.target = "Target path is required";
  return errors;
}

function MountApplicabilityFields({
  nodes, eggs, nodeIds, templateIds, onNodeIdsChange, onTemplateIdsChange, nodesError, eggsError,
}: {
  nodes: ApiNode[];
  eggs: ApiEgg[];
  nodeIds: string[];
  templateIds: string[];
  onNodeIdsChange: (ids: string[]) => void;
  onTemplateIdsChange: (ids: string[]) => void;
  nodesError: boolean;
  eggsError: boolean;
}) {
  const toggle = (ids: string[], id: string, checked: boolean) => checked ? [...ids, id] : ids.filter((value) => value !== id);

  return (
    <div className="md:col-span-2 rounded-lg border border-white/[0.06] bg-[#0f1419] p-4">
      <h4 className="text-sm font-medium text-slate-200">Mount Applicability</h4>
      <p className="mt-1 text-xs text-slate-500">A mount is eligible only for servers whose node and egg are both selected below. Select at least one of each to make this mount available.</p>
      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wider text-slate-500">Eligible Nodes</p>
          {nodesError ? <p className="text-xs text-red-400">Nodes could not be loaded. Retry before changing eligibility.</p> : nodes.length === 0 ? <p className="text-xs text-slate-500">No nodes available.</p> : (
            <div className="max-h-40 space-y-2 overflow-y-auto pr-1">
              {nodes.map((node) => (
                <label key={node.id} className="flex items-center gap-3 rounded-lg border border-white/10 px-3 py-2 text-sm text-slate-300 cursor-pointer hover:bg-white/[0.03]">
                  <input type="checkbox" checked={nodeIds.includes(node.id)} onChange={(event) => onNodeIdsChange(toggle(nodeIds, node.id, event.target.checked))} className="accent-[#dc2626]" />
                  <span>{node.name}</span>
                  <span className="ml-auto text-xs text-slate-500">{node.fqdn}</span>
                </label>
              ))}
            </div>
          )}
        </div>
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wider text-slate-500">Eligible Eggs</p>
          {eggsError ? <p className="text-xs text-red-400">Eggs could not be loaded. Retry before changing eligibility.</p> : eggs.length === 0 ? <p className="text-xs text-slate-500">No eggs available.</p> : (
            <div className="max-h-40 space-y-2 overflow-y-auto pr-1">
              {eggs.map((egg) => (
                <label key={egg.id} className="flex items-center gap-3 rounded-lg border border-white/10 px-3 py-2 text-sm text-slate-300 cursor-pointer hover:bg-white/[0.03]">
                  <input type="checkbox" checked={templateIds.includes(egg.id)} onChange={(event) => onTemplateIdsChange(toggle(templateIds, egg.id, event.target.checked))} className="accent-[#dc2626]" />
                  <span>{egg.name}</span>
                  <span className="ml-auto text-xs text-slate-500">{egg.id.slice(0, 8)}</span>
                </label>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export function AdminMounts() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const mountsQuery = useQuery({ queryKey: ["mounts"], queryFn: fetchMounts });
  const mounts = useMemo(() => mountsQuery.data ?? [], [mountsQuery.data]);
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const nestsQuery = useQuery({ queryKey: ["nests"], queryFn: fetchNests });

  const eggsQuery = useQuery({ queryKey: ["eggs"], queryFn: () => fetchEggs("*") });
  const eggs = useMemo(() => eggsQuery.data ?? [], [eggsQuery.data]);

  const [selectedMountId, setSelectedMountId] = useState<string | null>(null);

  const [showCreate, setShowCreate] = useState(false);

  const selected = selectedMountId ? mounts.find(m => m.id === selectedMountId) ?? null : null;

  // Create form
  const [cName, setCName] = useState("");
  const [cDesc, setCDesc] = useState("");
  const [cSource, setCSource] = useState("");
  const [cTarget, setCTarget] = useState("");
  const [cReadOnly, setCReadOnly] = useState(false);
  const [cUserMount, setCUserMount] = useState(false);
  const [cNodeIds, setCNodeIds] = useState<string[]>([]);
  const [cTemplateIds, setCTemplateIds] = useState<string[]>([]);
  const [cErrors, setCErrors] = useState<FieldErrors>({});

  // Edit form
  const [eName, setEName] = useState("");
  const [eDesc, setEDesc] = useState("");
  const [eSource, setESource] = useState("");
  const [eTarget, setETarget] = useState("");
  const [eReadOnly, setEReadOnly] = useState(false);
  const [eUserMount, setEUserMount] = useState(false);
  const [eNodeIds, setENodeIds] = useState<string[]>([]);
  const [eTemplateIds, setETemplateIds] = useState<string[]>([]);
  const [savedENodeIds, setSavedENodeIds] = useState<string[]>([]);
  const [savedETemplateIds, setSavedETemplateIds] = useState<string[]>([]);
  const [eErrors, setEErrors] = useState<FieldErrors>({});

  // Attachment modals
  const [showAddEggs, setShowAddEggs] = useState(false);
  const [showAddNodes, setShowAddNodes] = useState(false);
  const [selectedEggIds, setSelectedEggIds] = useState<string[]>([]);
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);

  const createMut = useMutation({
    mutationFn: () => createMount({
      name: cName.trim(), description: cDesc.trim(), source: cSource.trim(), target: cTarget.trim(),
      readOnly: cReadOnly, userMountable: cUserMount, nodeIds: cNodeIds, templateIds: cTemplateIds,
    }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setShowCreate(false); setCName(""); setCDesc(""); setCSource(""); setCTarget(""); setCReadOnly(false); setCUserMount(false); setCNodeIds([]); setCTemplateIds([]); setCErrors({}); toast({ tone: "success", title: "Mount created" }); },
    onError: (e: Error) => { console.error("Failed to create mount:", e); toast({ tone: "error", title: "Failed to create mount", message: e.message || "Unknown error" }); },
  });

  const updateMut = useMutation({
    mutationFn: async () => {
      const mount = await updateMount(selectedMountId!, { name: eName.trim(), description: eDesc.trim(), source: eSource.trim(), target: eTarget.trim(), readOnly: eReadOnly, userMountable: eUserMount });
      const currentNodeIds = savedENodeIds;
      const currentTemplateIds = savedETemplateIds;
      const nodeIdsToAttach = eNodeIds.filter((id) => !currentNodeIds.includes(id));
      const nodeIdsToDetach = currentNodeIds.filter((id) => !eNodeIds.includes(id));
      const templateIdsToAttach = eTemplateIds.filter((id) => !currentTemplateIds.includes(id));
      const templateIdsToDetach = currentTemplateIds.filter((id) => !eTemplateIds.includes(id));
      await Promise.all([
        nodeIdsToAttach.length > 0 ? attachNodesToMount(selectedMountId!, nodeIdsToAttach) : Promise.resolve(),
        ...nodeIdsToDetach.map((id) => detachNodeFromMount(selectedMountId!, id)),
        templateIdsToAttach.length > 0 ? attachEggsToMount(selectedMountId!, templateIdsToAttach) : Promise.resolve(),
        ...templateIdsToDetach.map((id) => detachEggFromMount(selectedMountId!, id)),
      ]);
      return mount;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setSavedENodeIds(eNodeIds); setSavedETemplateIds(eTemplateIds); toast({ tone: "success", title: "Mount updated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to update mount", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: deleteMount,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setSelectedMountId(null); toast({ tone: "success", title: "Mount deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete mount", message: e.message }),
  });

  const attachEggsMut = useMutation({
    mutationFn: () => attachEggsToMount(selectedMountId!, selectedEggIds),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setETemplateIds((ids) => [...new Set([...ids, ...selectedEggIds])]); setSavedETemplateIds((ids) => [...new Set([...ids, ...selectedEggIds])]); setShowAddEggs(false); setSelectedEggIds([]); toast({ tone: "success", title: "Eggs attached" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to attach eggs", message: e.message }),
  });

  const detachEggMut = useMutation({
    mutationFn: (eggId: string) => detachEggFromMount(selectedMountId!, eggId),
    onSuccess: (_, eggId) => { qc.invalidateQueries({ queryKey: ["mounts"] }); setETemplateIds((ids) => ids.filter((id) => id !== eggId)); setSavedETemplateIds((ids) => ids.filter((id) => id !== eggId)); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to detach egg", message: e.message }),
  });

  const attachNodesMut = useMutation({
    mutationFn: () => attachNodesToMount(selectedMountId!, selectedNodeIds),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setENodeIds((ids) => [...new Set([...ids, ...selectedNodeIds])]); setSavedENodeIds((ids) => [...new Set([...ids, ...selectedNodeIds])]); setShowAddNodes(false); setSelectedNodeIds([]); toast({ tone: "success", title: "Nodes attached" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to attach nodes", message: e.message }),
  });

  const detachNodeMut = useMutation({
    mutationFn: (nodeId: string) => detachNodeFromMount(selectedMountId!, nodeId),
    onSuccess: (_, nodeId) => { qc.invalidateQueries({ queryKey: ["mounts"] }); setENodeIds((ids) => ids.filter((id) => id !== nodeId)); setSavedENodeIds((ids) => ids.filter((id) => id !== nodeId)); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to detach node", message: e.message }),
  });

  const openMount = (mount: ApiMount) => {
    setSelectedMountId(mount.id);
    setEName(mount.name);
    setEDesc(mount.description ?? "");
    setESource(mount.source);
    setETarget(mount.target);
    setEReadOnly(mount.readOnly);
    setEUserMount(mount.userMountable ?? false);
    setENodeIds(mount.nodeIds ?? []);
    setETemplateIds(mount.templateIds ?? []);
    setSavedENodeIds(mount.nodeIds ?? []);
    setSavedETemplateIds(mount.templateIds ?? []);
    setEErrors({});
  };

  const handleCreate = () => {
    const errors = validateMount(cName, cSource, cTarget);
    setCErrors(errors);
    if (Object.keys(errors).length > 0) return;
    createMut.mutate();
  };

  const handleUpdate = () => {
    const errors = validateMount(eName, eSource, eTarget);
    setEErrors(errors);
    if (Object.keys(errors).length > 0) return;
    updateMut.mutate();
  };

  if (selectedMountId && selected) {
    return (
      <div>
        <div className="mb-6 flex items-center gap-4">
          <AdminBackButton label="Mounts" onClick={() => setSelectedMountId(null)} />
          <div>
            <h2 className="text-lg font-semibold text-slate-100">{selected.name}</h2>
            <p className="text-sm text-slate-400">{selected.description}</p>
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-2">
          {/* Mount Details */}
          <Card>
            <CardHeader title="Mount Details" icon={HardDrive} />
            <div className="p-6 grid gap-4">
              <div className="rounded-lg border border-white/[0.06] bg-[#0f1419] px-4 py-2.5 text-sm text-slate-400">
                <span className="text-xs uppercase tracking-wider text-slate-500">Unique ID</span>
                <p className="mt-0.5 font-mono text-slate-200">{selected.uuid ?? selected.id}</p>
              </div>
              <div>
                <Input label="Name" value={eName} onChange={setEName} />
                {eErrors.name ? <p className="mt-1 text-xs text-red-400">{eErrors.name}</p> : null}
              </div>
              <Input label="Description" value={eDesc} onChange={setEDesc} />
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Input label="Source Path" value={eSource} onChange={setESource} mono />
                  {eErrors.source ? <p className="mt-1 text-xs text-red-400">{eErrors.source}</p> : null}
                </div>
                <div>
                  <Input label="Target Path" value={eTarget} onChange={setETarget} mono />
                  {eErrors.target ? <p className="mt-1 text-xs text-red-400">{eErrors.target}</p> : null}
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <label className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-3 text-sm text-slate-300 cursor-pointer">
                  <input type="radio" name="mount-ro" checked={!eReadOnly} onChange={() => setEReadOnly(false)} className="accent-[#dc2626]" />
                  Read-Write
                </label>
                <label className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-3 text-sm text-slate-300 cursor-pointer">
                  <input type="radio" name="mount-ro" checked={eReadOnly} onChange={() => setEReadOnly(true)} className="accent-[#dc2626]" />
                  Read-Only
                </label>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <label className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-3 text-sm text-slate-300 cursor-pointer">
                  <input type="radio" name="mount-um" checked={!eUserMount} onChange={() => setEUserMount(false)} className="accent-[#dc2626]" />
                  User Not Mountable
                </label>
                <label className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-3 text-sm text-slate-300 cursor-pointer">
                  <input type="radio" name="mount-um" checked={eUserMount} onChange={() => setEUserMount(true)} className="accent-[#dc2626]" />
                  User Mountable
                </label>
              </div>
              <MountApplicabilityFields
                nodes={nodes}
                eggs={eggs}
                nodeIds={eNodeIds}
                templateIds={eTemplateIds}
                onNodeIdsChange={setENodeIds}
                onTemplateIdsChange={setETemplateIds}
                nodesError={nodesQuery.isError}
                eggsError={eggsQuery.isError}
              />
            </div>
            {updateMut.isError ? (
              <div className="mx-6 mb-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>{updateMut.error?.message || "An unexpected error occurred."}</span>
              </div>
            ) : null}
            {updateMut.isSuccess ? (
              <div className="mx-6 mb-4 flex items-start gap-2 rounded-lg border border-emerald-500/20 bg-emerald-950/10 p-3 text-xs text-emerald-200">
                <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
                <span>Mount updated successfully.</span>
              </div>
            ) : null}
            <div className="flex justify-between border-t border-white/[0.06] px-6 py-4">
              <Btn tone="danger" size="sm" onClick={() => { void (async () => { if (await confirm({ title: `Delete mount ${selected.name}?`, description: "The mount definition will be removed from the panel. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(selected.id); })(); }} disabled={deleteMut.isPending}>
                <Trash2 size={12} /> Delete
              </Btn>
              <Btn onClick={handleUpdate} disabled={updateMut.isPending}>
                <Save size={12} /> {updateMut.isPending ? "Saving..." : "Save"}
              </Btn>
            </div>
          </Card>

          <div className="grid gap-6">
            {/* Eggs Panel */}
            <Card>
              <div className="flex items-center justify-between border-b border-white/[0.06] px-6 py-4">
                <h3 className="text-sm font-semibold text-slate-200">Eggs</h3>
                <Btn size="sm" tone="ghost" onClick={() => setShowAddEggs(true)}><Plus size={12} /> Add Eggs</Btn>
              </div>
              {!Array.isArray(selected.templateIds) || selected.templateIds.length === 0 ? (
                <div className="px-6 py-4 text-sm text-slate-500">No eggs attached.</div>
              ) : null}
              <AdminTable label="Eggs">
                <AdminTHead>
                  <AdminTh>ID</AdminTh>
                  <AdminTh>Name</AdminTh>
                  <AdminTh>{' '}</AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {(Array.isArray(selected.templateIds) ? selected.templateIds : []).map((eggId) => {
                    const egg = Array.isArray(eggs) ? eggs.find((egg) => egg.id === eggId) : undefined;
                    return (
                      <AdminTr key={eggId}>
                        <AdminTd className="font-mono text-xs text-slate-500"><code>{eggId.slice(0, 8)}</code></AdminTd>
                        <AdminTd className="text-slate-200">{egg?.name ?? `Egg ${eggId.slice(0, 8)}`}</AdminTd>
                        <AdminTd>
                          <Btn size="sm" tone="danger" onClick={() => detachEggMut.mutate(eggId)} disabled={detachEggMut.isPending}>
                            <EggOff size={12} /> Detach
                          </Btn>
                        </AdminTd>
                      </AdminTr>
                    );
                  })}
                </AdminTBody>
              </AdminTable>
            </Card>

            {/* Nodes Panel */}
            <Card>
              <div className="flex items-center justify-between border-b border-white/[0.06] px-6 py-4">
                <h3 className="text-sm font-semibold text-slate-200">Nodes</h3>
                <Btn size="sm" tone="ghost" onClick={() => setShowAddNodes(true)}><Plus size={12} /> Add Nodes</Btn>
              </div>
              {!Array.isArray(selected.nodeIds) || selected.nodeIds.length === 0 ? (
                <div className="px-6 py-4 text-sm text-slate-500">No nodes attached.</div>
              ) : null}
              <AdminTable label="Nodes">
                <AdminTHead>
                  <AdminTh>ID</AdminTh>
                  <AdminTh>Name</AdminTh>
                  <AdminTh>FQDN</AdminTh>
                  <AdminTh>{' '}</AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {(Array.isArray(selected.nodeIds) ? selected.nodeIds : []).map((nodeId) => {
                    const node = Array.isArray(nodes) ? nodes.find(n => n.id === nodeId) : undefined;
                    return (
                      <AdminTr key={nodeId}>
                        <AdminTd className="font-mono text-xs text-slate-500"><code>{nodeId.slice(0, 8)}</code></AdminTd>
                        <AdminTd className="text-slate-200">{node?.name ?? `Node ${nodeId.slice(0, 8)}`}</AdminTd>
                        <AdminTd><code className="text-xs text-slate-400">{node?.fqdn}</code></AdminTd>
                        <AdminTd>
                          <Btn size="sm" tone="danger" onClick={() => detachNodeMut.mutate(nodeId)} disabled={detachNodeMut.isPending}>
                            <Link2Off size={12} /> Detach
                          </Btn>
                        </AdminTd>
                      </AdminTr>
                    );
                  })}
                </AdminTBody>
              </AdminTable>
            </Card>
          </div>
        </div>

        {/* Add Eggs Modal */}
        {showAddEggs && selected && (
          <Modal title="Add Eggs" onClose={() => { setShowAddEggs(false); setSelectedEggIds([]); }}>
            {eggsQuery.isError ? (
              <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                <span>Could not load eggs: {eggsQuery.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void eggsQuery.refetch()}>Retry</Btn>
              </div>
            ) : nestsQuery.isError ? (
              <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                <span>Could not load nests: {nestsQuery.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void nestsQuery.refetch()}>Retry</Btn>
              </div>
            ) : null}
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {eggsQuery.isError ? null : Array.isArray(eggs) ? eggs.filter((egg) => !(Array.isArray(selected.templateIds) ? selected.templateIds : []).includes(egg.id)).map((egg) => (
                <label key={egg.id} className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-2.5 text-sm cursor-pointer hover:bg-white/[0.03]">
                  <input type="checkbox" checked={selectedEggIds.includes(egg.id)} onChange={(e) => setSelectedEggIds(e.target.checked ? [...selectedEggIds, egg.id] : selectedEggIds.filter(id => id !== egg.id))} className="accent-[#dc2626]" />
                  <span className="text-slate-200">{egg.name}</span>
                  <span className="ml-auto text-xs text-slate-500">{egg.id.slice(0, 8)}</span>
                </label>
              )) : null}
            </div>
            {attachEggsMut.isError ? (
              <div className="mt-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>{attachEggsMut.error?.message || "An unexpected error occurred."}</span>
              </div>
            ) : null}
            <ModalFooter
              onCancel={() => { setShowAddEggs(false); setSelectedEggIds([]); }}
              onConfirm={() => attachEggsMut.mutate()}
              disabled={selectedEggIds.length === 0 || attachEggsMut.isPending || eggsQuery.isError || nestsQuery.isError}
              confirmLabel={attachEggsMut.isPending ? "Attaching..." : "Add Selected Eggs"}
            />
          </Modal>
        )}

        {/* Add Nodes Modal */}
        {showAddNodes && selected && (
          <Modal title="Add Nodes" onClose={() => { setShowAddNodes(false); setSelectedNodeIds([]); }}>
            {nodesQuery.isError ? (
              <div className="mb-4 flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                <span>Could not load nodes: {nodesQuery.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void nodesQuery.refetch()}>Retry</Btn>
              </div>
            ) : null}
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {nodesQuery.isError ? null : Array.isArray(nodes) ? nodes.filter(n => !(Array.isArray(selected.nodeIds) ? selected.nodeIds : []).includes(n.id)).map((node) => (
                <label key={node.id} className="flex items-center gap-3 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-2.5 text-sm cursor-pointer hover:bg-white/[0.03]">
                  <input type="checkbox" checked={selectedNodeIds.includes(node.id)} onChange={(e) => setSelectedNodeIds(e.target.checked ? [...selectedNodeIds, node.id] : selectedNodeIds.filter(id => id !== node.id))} className="accent-[#dc2626]" />
                  <span className="text-slate-200">{node.name}</span>
                  <span className="ml-auto text-xs text-slate-400">{node.fqdn}</span>
                </label>
              )) : null}
            </div>
            {attachNodesMut.isError ? (
              <div className="mt-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>{attachNodesMut.error?.message || "An unexpected error occurred."}</span>
              </div>
            ) : null}
            <ModalFooter
              onCancel={() => { setShowAddNodes(false); setSelectedNodeIds([]); }}
              onConfirm={() => attachNodesMut.mutate()}
              disabled={selectedNodeIds.length === 0 || attachNodesMut.isPending || nodesQuery.isError}
              confirmLabel={attachNodesMut.isPending ? "Attaching..." : "Add Selected Nodes"}
            />
          </Modal>
        )}
      </div>
    );
  }

  return (
    <div>
      <SectionHeader
        title="Mounts"
        sub="Configure and manage additional mount points for servers."
        action={<Btn onClick={() => { setShowCreate(true); setCNodeIds([]); setCTemplateIds([]); setCErrors({}); }}><Plus size={14} /> New Mount</Btn>}
      />

      <Card>
        <CardHeader title="Mount List" icon={HardDrive} />
        {mountsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading</div>
        ) : mountsQuery.isError ? (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
              <span>Could not load mounts: {mountsQuery.error.message}</span>
              <Btn size="sm" tone="ghost" onClick={() => void mountsQuery.refetch()}>Retry</Btn>
            </div>
          </div>
        ) : mounts.length === 0 ? (
          <EmptyState icon={HardDrive} message="No mounts configured." />
        ) : (
          <AdminTable label="Mounts">
            <AdminTHead>
              <AdminTh>ID</AdminTh>
              <AdminTh>Name</AdminTh>
              <AdminTh>Source</AdminTh>
              <AdminTh>Target</AdminTh>
              <AdminTh className="text-center">Eggs</AdminTh>
              <AdminTh className="text-center">Nodes</AdminTh>
              <AdminTh className="text-center">Servers</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {Array.isArray(mounts) ? mounts.map((mount) => (
                <AdminTr key={mount.id} onClick={() => openMount(mount)}>
                  <AdminTd className="font-mono text-xs text-slate-500"><code>{mount.id.slice(0, 8)}</code></AdminTd>
                  <AdminTd className="font-medium text-slate-200">{mount.name}</AdminTd>
                  <AdminTd className="font-mono text-xs text-slate-400">{mount.source}</AdminTd>
                  <AdminTd className="font-mono text-xs text-slate-400">{mount.target}</AdminTd>
                  <AdminTd className="text-center text-slate-400">{Array.isArray(mount.templateIds) ? mount.templateIds.length : 0}</AdminTd>
                  <AdminTd className="text-center text-slate-400">{Array.isArray(mount.nodeIds) ? mount.nodeIds.length : 0}</AdminTd>
                  <AdminTd className="text-center text-slate-400">{Array.isArray(mount.serverIds) ? mount.serverIds.length : 0}</AdminTd>
                </AdminTr>
              )) : null}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {showCreate ? (
        <Modal title="Create Mount" onClose={() => setShowCreate(false)}>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="md:col-span-2">
              <Input label="Name" value={cName} onChange={setCName} placeholder="Shared Plugins" />
              {cErrors.name ? <p className="mt-1 text-xs text-red-400">{cErrors.name}</p> : null}
              <p className="mt-1 text-xs text-slate-500">Unique name used to separate this mount from another.</p>
            </div>
            <div className="md:col-span-2">
              <label className="mb-1.5 block text-sm font-medium text-slate-300">Description</label>
              <textarea className="h-20 w-full rounded-lg border border-white/10 bg-[#0f1419] px-3 py-2 text-sm text-slate-100" value={cDesc} onChange={(e) => setCDesc(e.target.value)} />
              <p className="mt-1 text-xs text-slate-500">A longer description for this mount.</p>
            </div>
            <div>
              <Input label="Source Path" value={cSource} onChange={setCSource} placeholder="/mnt/shared/plugins" mono />
              {cErrors.source ? <p className="mt-1 text-xs text-red-400">{cErrors.source}</p> : null}
            </div>
            <div>
              <Input label="Target Path" value={cTarget} onChange={setCTarget} placeholder="/plugins" mono />
              {cErrors.target ? <p className="mt-1 text-xs text-red-400">{cErrors.target}</p> : null}
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">Read Only</label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-readonly" checked={!cReadOnly} onChange={() => setCReadOnly(false)} className="accent-[#dc2626]" /> False
                </label>
                <label className="flex items-center gap-2 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-readonly" checked={cReadOnly} onChange={() => setCReadOnly(true)} className="accent-[#dc2626]" /> True
                </label>
              </div>
            </div>
            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">User Mountable</label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-usermount" checked={!cUserMount} onChange={() => setCUserMount(false)} className="accent-[#dc2626]" /> False
                </label>
                <label className="flex items-center gap-2 rounded-lg border border-white/10 bg-[#0f1419] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-usermount" checked={cUserMount} onChange={() => setCUserMount(true)} className="accent-[#dc2626]" /> True
                </label>
              </div>
            </div>
            <MountApplicabilityFields
              nodes={nodes}
              eggs={eggs}
              nodeIds={cNodeIds}
              templateIds={cTemplateIds}
              onNodeIdsChange={setCNodeIds}
              onTemplateIdsChange={setCTemplateIds}
              nodesError={nodesQuery.isError}
              eggsError={eggsQuery.isError}
            />
          </div>
          {createMut.isError ? (
            <div className="mt-4 flex items-start gap-2 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-xs text-red-200">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              <span>{createMut.error?.message || "An unexpected error occurred."}</span>
            </div>
          ) : null}
          {createMut.isSuccess ? (
            <div className="mt-4 flex items-start gap-2 rounded-lg border border-emerald-500/20 bg-emerald-950/10 p-3 text-xs text-emerald-200">
              <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
              <span>Mount created successfully.</span>
            </div>
          ) : null}
          <ModalFooter
            onCancel={() => setShowCreate(false)}
            onConfirm={handleCreate}
            disabled={!cName.trim() || !cSource.trim() || !cTarget.trim() || createMut.isPending}
            confirmLabel={createMut.isPending ? "Creating..." : "Create"}
          />
        </Modal>
      ) : null}
      {renderConfirm()}
    </div>
  );
}
