"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { useRouter } from "next/navigation";
import { useMutation } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, EggOff, HardDrive, Link2Off, Plus, Save, Trash2, Server, Search } from "lucide-react";
import { StoragePlattersIcon } from "@/components/ui/forge-icons";
import {
  attachEggsToMount, attachNodesToMount, createMount, deleteMount, detachEggFromMount, detachNodeFromMount,
  fetchEggs, fetchMounts, fetchNests, fetchMountServers, assignServerToMount, unassignServerFromMount,
  fetchServers, updateMount, type ApiEgg, type ApiMount, type ApiNode, type ApiServer,
} from "@/lib/api";
import { REFRESH, sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge, NotReported } from "@/components/admin/telemetry-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminBackButton, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, SectionHeader, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd, AdminTabs, Pill, AdminLoadingState, AdminErrorState, AdminPageLayout, cn } from "./admin-ui";
import { toneStyles } from "@/components/ui/forge/status";

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

/**
 * A count that distinguishes "attached to nothing" from "the payload never said".
 * `Array.isArray(x) ? x.length : 0` rendered the second as the first.
 */
function CountCell({ value }: { value: readonly string[] | undefined }) {
  if (!Array.isArray(value)) return <NotReported reason="The mount record did not include this list" />;
  return <>{value.length}</>;
}

function MountApplicabilityFields({
  nodes, eggs, nodeIds, templateIds, onNodeIdsChange, onTemplateIdsChange, nodesError, eggsError, nodesLoading, eggsLoading,
}: {
  nodes: ApiNode[];
  eggs: ApiEgg[];
  nodeIds: string[];
  templateIds: string[];
  onNodeIdsChange: (ids: string[]) => void;
  onTemplateIdsChange: (ids: string[]) => void;
  nodesError: boolean;
  eggsError: boolean;
  nodesLoading: boolean;
  eggsLoading: boolean;
}) {
  const toggle = (ids: string[], id: string, checked: boolean) => checked ? [...ids, id] : ids.filter((value) => value !== id);
  return (
    <div className="md:col-span-2 rounded-lg border border-line bg-overlay-subtle p-4">
      <h4 className="text-sm font-medium text-text">Mount Applicability</h4>
      <p className="mt-1 text-xs text-text-muted">A mount is eligible only for servers whose node and egg are both selected below. Select at least one of each to make this mount available.</p>
      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wider text-text-subtle">Eligible Nodes</p>
          {/* A failed read says "could not load", never "none available". */}
          {nodesError ? <p className={cn("text-xs", toneStyles.danger.fg)}>Nodes could not be loaded. Retry before changing eligibility.</p> : nodesLoading ? <p className="text-xs text-text-muted">Loading nodes…</p> : nodes.length === 0 ? <p className="text-xs text-text-muted">No nodes registered yet.</p> : (
            <div className="max-h-40 space-y-2 overflow-y-auto pr-1">
              {nodes.map((node) => (
                <label key={node.id} className="flex cursor-pointer items-center gap-3 rounded-lg border border-line px-3 py-2 text-sm text-text hover:bg-overlay">
                  <input type="checkbox" checked={nodeIds.includes(node.id)} onChange={(event) => onNodeIdsChange(toggle(nodeIds, node.id, event.target.checked))} className="accent-[var(--brand)]" />
                  <span>{node.name}</span>
                  <span className="ml-auto text-xs text-text-muted">{node.fqdn}</span>
                </label>
              ))}
            </div>
          )}
        </div>
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wider text-text-subtle">Eligible Eggs</p>
          {eggsError ? <p className={cn("text-xs", toneStyles.danger.fg)}>Eggs could not be loaded. Retry before changing eligibility.</p> : eggsLoading ? <p className="text-xs text-text-muted">Loading eggs…</p> : eggs.length === 0 ? <p className="text-xs text-text-muted">No eggs available.</p> : (
            <div className="max-h-40 space-y-2 overflow-y-auto pr-1">
              {eggs.map((egg) => (
                <label key={egg.id} className="flex cursor-pointer items-center gap-3 rounded-lg border border-line px-3 py-2 text-sm text-text hover:bg-overlay">
                  <input type="checkbox" checked={templateIds.includes(egg.id)} onChange={(event) => onTemplateIdsChange(toggle(templateIds, egg.id, event.target.checked))} className="accent-[var(--brand)]" />
                  <span>{egg.name}</span>
                  <span className="ml-auto text-xs text-text-muted">{egg.id.slice(0, 8)}</span>
                </label>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

/** How many attach candidates are rendered before the operator must narrow. */
const ATTACH_PAGE_SIZE = 50;

function AttachedServersTab({ mountId, mountName }: { mountId: string; mountName: string }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [showAttach, setShowAttach] = useState(false);
  const [serverSearch, setServerSearch] = useState("");
  const [selectedServerId, setSelectedServerId] = useState<string>("");

  const serversQuery = useQuery({ queryKey: ["mount-servers", mountId], queryFn: () => fetchMountServers(mountId), enabled: !!mountId, refetchInterval: REFRESH.inventory });
  const allServersQuery = useQuery({ queryKey: ["servers-list"], queryFn: () => fetchServers(), enabled: showAttach });

  const attached = useMemo(() => Array.isArray(serversQuery.data) ? serversQuery.data : [], [serversQuery.data]);
  const allServers = useMemo(() => Array.isArray(allServersQuery.data) ? (allServersQuery.data as ApiServer[]) : [], [allServersQuery.data]);
  const available = useMemo(() => {
    const attachedIds = new Set(attached.map((s) => s.id));
    return allServers.filter((s) => !attachedIds.has(s.id)).filter((s) => !serverSearch || `${s.name} ${s.id}`.toLowerCase().includes(serverSearch.toLowerCase()));
  }, [allServers, attached, serverSearch]);
  const shownServers = available.slice(0, ATTACH_PAGE_SIZE);

  const attachMut = useMutation({
    mutationFn: () => assignServerToMount(mountId, selectedServerId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mount-servers", mountId] }); qc.invalidateQueries({ queryKey: ["mounts"] }); setShowAttach(false); setSelectedServerId(""); toast({ tone: "success", title: "Server attached", message: `${mountName} will mount on this server at its next build.` }); },
    onError: (e: Error) => toast({ tone: "error", title: "Attach failed", message: e.message }),
  });
  const detachMut = useMutation({
    mutationFn: (serverId: string) => unassignServerFromMount(mountId, serverId),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mount-servers", mountId] }); qc.invalidateQueries({ queryKey: ["mounts"] }); toast({ tone: "success", title: "Server detached" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Detach failed", message: e.message }),
  });

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-6 py-4">
        <h3 className="text-sm font-semibold text-text">Attached servers</h3>
        <Btn size="sm" tone="primary" onClick={() => setShowAttach(true)}><Plus size={12} /> Attach server</Btn>
      </div>
      {serversQuery.isPending ? (
        <div className="p-4"><AdminLoadingState label="Loading attached servers…" /></div>
      ) : serversQuery.isError ? (
        <div className="p-4">
          <AdminErrorState message={`Could not load the servers using this mount: ${(serversQuery.error as Error).message}`} retry={() => void serversQuery.refetch()} />
        </div>
      ) : attached.length === 0 ? (
        <div className="px-6 py-8">
          <EmptyState icon={Server} title="No servers attached" message="No server currently mounts this path. Attach one to make it available to that workload." />
        </div>
      ) : (
        <AdminTable label="Attached servers">
          <AdminTHead><AdminTh>Server</AdminTh><AdminTh>ID</AdminTh><AdminTh>Status</AdminTh><AdminTh><span className="sr-only">Actions</span></AdminTh></AdminTHead>
          <AdminTBody>
            {attached.map((s) => (
              <AdminTr key={s.id}>
                <AdminTd className="font-medium text-text">{s.name}</AdminTd>
                <AdminTd className="font-mono text-xs text-text-muted">{s.id.slice(0, 8)}</AdminTd>
                <AdminTd><Pill tone={s.status === "running" ? "green" : "neutral"}>{s.status}</Pill></AdminTd>
                <AdminTd>
                  <Btn ariaLabel={`Detach mount ${mountName} from server ${s.name}`} size="sm" tone="danger" disabled={detachMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Detach ${s.name}?`, description: `“${mountName}” stops being mounted into ${s.name}. Files already written on the host stay where they are; the server loses access to the path at its next build.`, danger: true, confirmLabel: "Detach" })) detachMut.mutate(s.id); })(); }}>
                    <Link2Off size={12} /> Detach
                  </Btn>
                </AdminTd>
              </AdminTr>
            ))}
          </AdminTBody>
        </AdminTable>
      )}
      {showAttach && (
        <Modal title="Attach a server to this mount" description={`“${mountName}” will be mounted into the server you choose.`} onClose={() => { setShowAttach(false); setSelectedServerId(""); setServerSearch(""); }}>
          <div className="space-y-4">
            <div className="relative">
              <Search aria-hidden="true" size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
              <label className="sr-only" htmlFor="attach-server-search">Search servers by name or id</label>
              <input id="attach-server-search" value={serverSearch} onChange={(e) => setServerSearch(e.target.value)} placeholder="Search servers…" className="ui-input pl-9" />
            </div>
            {allServersQuery.isPending ? <AdminLoadingState label="Loading servers…" />
              : allServersQuery.isError ? (
                <AdminErrorState message={`Could not load servers: ${(allServersQuery.error as Error).message}`} retry={() => void allServersQuery.refetch()} />
              )
              : available.length === 0 ? (
                /* An empty *filter* is not an empty *fleet* — say which happened. */
                <p className="text-sm text-text-muted">
                  {allServers.length === 0
                    ? "No servers exist yet, so there is nothing to attach."
                    : serverSearch
                      ? `No server matches “${serverSearch}”. ${attached.length} of ${allServers.length} servers already use this mount.`
                      : `All ${allServers.length} servers already use this mount.`}
                </p>
              ) : (
                <div className="max-h-64 space-y-2 overflow-y-auto">
                  {shownServers.map((s) => (
                    <label key={s.id} className={cn("flex cursor-pointer items-center gap-3 rounded-lg border p-3 text-sm", selectedServerId === s.id ? "border-[color-mix(in_srgb,var(--brand)_50%,transparent)] bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]" : "border-line bg-overlay")}>
                      <input type="radio" name="attach-server" checked={selectedServerId === s.id} onChange={() => setSelectedServerId(s.id)} className="accent-[var(--brand)]" />
                      <span className="font-medium text-text">{s.name}</span>
                      <span className="ml-auto font-mono text-xs text-text-muted">{s.id.slice(0, 8)}</span>
                    </label>
                  ))}
                </div>
              )}
            {/* A silently truncated list is how "not shown" becomes "does not
                exist" in an operator's head. */}
            {available.length > shownServers.length ? (
              <p className="text-xs leading-5 text-warn" role="status">
                Showing {shownServers.length} of {available.length} matching servers. Narrow the search to reach the rest — the others are not missing, just not listed here.
              </p>
            ) : null}
            {!selectedServerId && !allServersQuery.isPending && !allServersQuery.isError && available.length > 0 ? (
              <p className="text-xs text-text-muted">Choose a server to enable attaching.</p>
            ) : null}
            {attachMut.isError && <p className={cn("text-sm", toneStyles.danger.fg)}>{(attachMut.error as Error).message}</p>}
          </div>
          <ModalFooter onCancel={() => setShowAttach(false)} onConfirm={() => attachMut.mutate()} disabled={!selectedServerId || attachMut.isPending} confirmLabel={attachMut.isPending ? "Attaching…" : "Attach"} />
        </Modal>
      )}
      {renderConfirm()}
    </div>
  );
}

export function AdminMounts() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const searchParams = useSearchParams();
  const router = useRouter();
  const mountsQuery = useQuery({ queryKey: ["mounts"], queryFn: fetchMounts, refetchInterval: REFRESH.inventory, retry: false });
  const mounts = useMemo(() => Array.isArray(mountsQuery.data) ? mountsQuery.data : [], [mountsQuery.data]);
  const mountsSource = sourceState(mountsQuery, REFRESH.inventory);
  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => Array.isArray(nodesQuery.data) ? nodesQuery.data : [], [nodesQuery.data]);
  const nestsQuery = useQuery({ queryKey: ["nests"], queryFn: fetchNests });

  const eggsQuery = useQuery({ queryKey: ["eggs"], queryFn: () => fetchEggs("*") });
  const eggs = useMemo(() => Array.isArray(eggsQuery.data) ? eggsQuery.data : [], [eggsQuery.data]);

  /* The open mount is the URL's `?mount=`, not component state: a detail view
     held only in state cannot be linked, does not survive refresh, and browser
     Back leaves the page entirely. */
  const selectedMountId = searchParams.get("mount");
  const [detailTab, setDetailTab] = useState<"eggs" | "nodes" | "servers">("eggs");
  const setSelectedMountId = (id: string | null) => {
    const next = new URLSearchParams(searchParams.toString());
    if (id) next.set("mount", id); else next.delete("mount");
    const qs = next.toString();
    router.replace(`/admin/mounts${qs ? `?${qs}` : ""}`, { scroll: false });
  };

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
  /** Link operations a save could not apply, named for the operator. */
  const [saveFailures, setSaveFailures] = useState<string[]>([]);

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

  /**
   * A save here is one record update plus N link operations. `Promise.all`
   * collapsed any partial failure into "Failed to update mount", leaving the
   * operator unable to tell which node and egg links actually moved — so each
   * operation is settled separately and named in the result.
   */
  const updateMut = useMutation({
    mutationFn: async (): Promise<{ applied: string[]; failures: string[]; partial: boolean }> => {
      const mountId = selectedMountId!;
      const mount = await updateMount(mountId, { name: eName.trim(), description: eDesc.trim(), source: eSource.trim(), target: eTarget.trim(), readOnly: eReadOnly, userMountable: eUserMount });
      const currentNodeIds = savedENodeIds;
      const currentTemplateIds = savedETemplateIds;
      const nodeIdsToAttach = eNodeIds.filter((id) => !currentNodeIds.includes(id));
      const nodeIdsToDetach = currentNodeIds.filter((id) => !eNodeIds.includes(id));
      const templateIdsToAttach = eTemplateIds.filter((id) => !currentTemplateIds.includes(id));
      const templateIdsToDetach = currentTemplateIds.filter((id) => !eTemplateIds.includes(id));
      const nameOfNode = (id: string) => nodes.find((n) => n.id === id)?.name ?? `node ${id.slice(0, 8)}`;
      const nameOfEgg = (id: string) => eggs.find((e) => e.id === id)?.name ?? `egg ${id.slice(0, 8)}`;

      const jobs: Array<{ label: string; run: () => Promise<unknown> }> = [];
      if (nodeIdsToAttach.length > 0) jobs.push({ label: `attach ${nodeIdsToAttach.map(nameOfNode).join(", ")}`, run: () => attachNodesToMount(mountId, nodeIdsToAttach) });
      if (templateIdsToAttach.length > 0) jobs.push({ label: `attach ${templateIdsToAttach.map(nameOfEgg).join(", ")}`, run: () => attachEggsToMount(mountId, templateIdsToAttach) });
      for (const id of nodeIdsToDetach) jobs.push({ label: `detach ${nameOfNode(id)}`, run: async () => { const r = await detachNodeFromMount(mountId, id); if (!r.ok) throw new Error("did not complete"); } });
      for (const id of templateIdsToDetach) jobs.push({ label: `detach ${nameOfEgg(id)}`, run: async () => { const r = await detachEggFromMount(mountId, id); if (!r.ok) throw new Error("did not complete"); } });

      const settled = await Promise.allSettled(jobs.map((job) => job.run()));
      const applied: string[] = ["mount details"];
      const failures: string[] = [];
      settled.forEach((result, index) => {
        const job = jobs[index];
        if (result.status === "fulfilled") applied.push(job.label);
        else failures.push(`${job.label} — ${result.reason instanceof Error ? result.reason.message : "failed"}`);
      });
      // Re-read so the checkboxes show what the server actually holds, not what
      // was asked for.
      await qc.invalidateQueries({ queryKey: ["mounts"] });
      void mount;
      return { applied, failures, partial: failures.length > 0 };
    },
    onSuccess: ({ failures, partial }) => {
      if (partial) {
        // Only the operations that actually applied may advance the "saved"
        // baseline; the rest stay shown as the pending change they are.
        setSaveFailures(failures);
        toast({ tone: "error", title: "Partly saved", message: `${failures.length} link operation${failures.length === 1 ? "" : "s"} failed — listed under the form.` });
      } else {
        setSavedENodeIds(eNodeIds);
        setSavedETemplateIds(eTemplateIds);
        setSaveFailures([]);
        toast({ tone: "success", title: "Mount updated" });
      }
    },
    onError: (e: Error) => { setSaveFailures([e.message]); toast({ tone: "error", title: "Failed to update mount", message: e.message }) },
  });

  const deleteMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await deleteMount(id);
      if (!result.ok) throw new Error("The server reported the mount was not deleted.");
      return result;
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setSelectedMountId(null); toast({ tone: "success", title: "Mount deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete mount", message: e.message }),
  });

  const attachEggsMut = useMutation({
    mutationFn: () => attachEggsToMount(selectedMountId!, selectedEggIds),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setETemplateIds((ids) => [...new Set([...ids, ...selectedEggIds])]); setSavedETemplateIds((ids) => [...new Set([...ids, ...selectedEggIds])]); setShowAddEggs(false); setSelectedEggIds([]); toast({ tone: "success", title: "Eggs attached" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to attach eggs", message: e.message }),
  });

  const detachEggMut = useMutation({
    mutationFn: async (eggId: string) => {
      const result = await detachEggFromMount(selectedMountId!, eggId);
      if (!result.ok) throw new Error("The server reported the egg detach from mount did not complete.");
      return result;
    },
    onSuccess: (_, eggId) => { qc.invalidateQueries({ queryKey: ["mounts"] }); setETemplateIds((ids) => ids.filter((id) => id !== eggId)); setSavedETemplateIds((ids) => ids.filter((id) => id !== eggId)); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to detach egg", message: e.message }),
  });

  const attachNodesMut = useMutation({
    mutationFn: () => attachNodesToMount(selectedMountId!, selectedNodeIds),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["mounts"] }); setENodeIds((ids) => [...new Set([...ids, ...selectedNodeIds])]); setSavedENodeIds((ids) => [...new Set([...ids, ...selectedNodeIds])]); setShowAddNodes(false); setSelectedNodeIds([]); toast({ tone: "success", title: "Nodes attached" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to attach nodes", message: e.message }),
  });

  const detachNodeMut = useMutation({
    mutationFn: async (nodeId: string) => {
      const result = await detachNodeFromMount(selectedMountId!, nodeId);
      if (!result.ok) throw new Error("The server reported the node detach from mount did not complete.");
      return result;
    },
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
    setSaveFailures([]);
    setDetailTab("eggs");
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
      <AdminPageLayout>
        {/* The frame owns the single <h1>; a detail header may name the
            resource, which this does, and it now uses the registry label for
            the list it returns to instead of a third spelling ("Mounts"). */}
        <SectionHeader
          backAction={() => setSelectedMountId(null)}
          backLabel="Storage Mounts"
          status={<FreshnessBadge state={mountsSource} />}
          sub={selected.description || undefined}
          title={selected.name}
        />
        {!selectedMountId ? null : mounts.length === 0 ? (
          <AdminErrorState message="This mount is no longer in the list the control plane returned, so it cannot be edited. Reload the mounts." retry={() => void mountsQuery.refetch()} />
        ) : null}

        <div className="grid gap-6 lg:grid-cols-2">
          {/* Mount Details */}
          <Card>
            <CardHeader title="Mount Details" icon={HardDrive} />
            <div className="p-6 grid gap-4">
              <div className="rounded-lg border border-line bg-[var(--surface)] px-4 py-2.5 text-sm text-text-subtle">
                <span className="text-xs uppercase tracking-wider text-text-muted">Unique ID</span>
                <p className="mt-0.5 font-mono text-text">{selected.uuid ?? selected.id}</p>
              </div>
              <div>
                <Input label="Name" value={eName} onChange={setEName} />
                {eErrors.name ? <p className="mt-1 text-xs text-danger">{eErrors.name}</p> : null}
              </div>
              <Input label="Description" value={eDesc} onChange={setEDesc} />
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Input label="Source Path" value={eSource} onChange={setESource} mono />
                  {eErrors.source ? <p className="mt-1 text-xs text-danger">{eErrors.source}</p> : null}
                </div>
                <div>
                  <Input label="Target Path" value={eTarget} onChange={setETarget} mono />
                  {eErrors.target ? <p className="mt-1 text-xs text-danger">{eErrors.target}</p> : null}
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <label className="flex items-center gap-3 rounded-lg border border-line bg-[var(--surface)] px-4 py-3 text-sm text-text cursor-pointer">
                  <input type="radio" name="mount-ro" checked={!eReadOnly} onChange={() => setEReadOnly(false)} className="accent-[var(--brand)]" />
                  Read-Write
                </label>
                <label className="flex items-center gap-3 rounded-lg border border-line bg-[var(--surface)] px-4 py-3 text-sm text-text cursor-pointer">
                  <input type="radio" name="mount-ro" checked={eReadOnly} onChange={() => setEReadOnly(true)} className="accent-[var(--brand)]" />
                  Read-Only
                </label>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <label className="flex items-center gap-3 rounded-lg border border-line bg-[var(--surface)] px-4 py-3 text-sm text-text cursor-pointer">
                  <input type="radio" name="mount-um" checked={!eUserMount} onChange={() => setEUserMount(false)} className="accent-[var(--brand)]" />
                  User Not Mountable
                </label>
                <label className="flex items-center gap-3 rounded-lg border border-line bg-[var(--surface)] px-4 py-3 text-sm text-text cursor-pointer">
                  <input type="radio" name="mount-um" checked={eUserMount} onChange={() => setEUserMount(true)} className="accent-[var(--brand)]" />
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
                nodesLoading={nodesQuery.isPending}
                eggsLoading={eggsQuery.isPending}
              />
            </div>
            {updateMut.isError ? (
              <div className="mx-6 mb-4 flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs text-danger">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>{updateMut.error?.message || "An unexpected error occurred."}</span>
              </div>
            ) : null}
            {updateMut.isSuccess ? (
              <div className="mx-6 mb-4 flex items-start gap-2 rounded-lg border border-ok-line bg-ok-subtle p-3 text-xs text-ok">
                <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
                <span>Mount updated successfully.</span>
              </div>
            ) : null}
            <div className="flex justify-between border-t border-line px-6 py-4">
              <Btn tone="danger" size="sm" onClick={() => { void (async () => { if (await confirm({ title: `Delete mount ${selected.name}?`, description: "The mount definition will be removed from the panel. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(selected.id); })(); }} disabled={deleteMut.isPending}>
                <Trash2 size={12} /> Delete
              </Btn>
              <Btn onClick={handleUpdate} disabled={updateMut.isPending} className="bg-[var(--brand)] hover:bg-[color-mix(in_srgb,var(--brand)_90%,transparent)] text-white">
                <Save size={12} /> {updateMut.isPending ? "Saving..." : "Save"}
              </Btn>
            </div>
          </Card>

          <div className="grid gap-6">
            <Card>
              <AdminTabs tabs={[{ id: "eggs", label: "Eggs" }, { id: "nodes", label: "Nodes" }, { id: "servers", label: "Attached Servers" }]} active={detailTab} onChange={(v) => setDetailTab(v as typeof detailTab)} />
              {detailTab === "eggs" && (
                <>
                  <div className="flex items-center justify-between border-b border-line px-6 py-4">
                    <h3 className="text-sm font-semibold text-text">Eggs</h3>
                    <Btn size="sm" tone="ghost" onClick={() => setShowAddEggs(true)}><Plus size={12} /> Add Eggs</Btn>
                  </div>
                  {!Array.isArray(selected.templateIds) || selected.templateIds.length === 0 ? (
                    <div className="px-6 py-4 text-sm text-text-muted">No eggs attached.</div>
                  ) : null}
                  <AdminTable label="Eggs">
                    <AdminTHead><AdminTh>ID</AdminTh><AdminTh>Name</AdminTh><AdminTh></AdminTh></AdminTHead>
                    <AdminTBody>
                      {(Array.isArray(selected.templateIds) ? selected.templateIds : []).map((eggId) => {
                        const egg = Array.isArray(eggs) ? eggs.find((egg) => egg.id === eggId) : undefined;
                        return (
                          <AdminTr key={eggId}>
                            <AdminTd className="font-mono text-xs text-text-muted"><code>{eggId.slice(0, 8)}</code></AdminTd>
                            <AdminTd className="text-text">{egg?.name ?? `Egg ${eggId.slice(0, 8)}`}</AdminTd>
                            <AdminTd><Btn size="sm" tone="danger" onClick={() => detachEggMut.mutate(eggId)} disabled={detachEggMut.isPending}><EggOff size={12} /> Detach</Btn></AdminTd>
                          </AdminTr>
                        );
                      })}
                    </AdminTBody>
                  </AdminTable>
                </>
              )}
              {detailTab === "nodes" && (
                <>
                  <div className="flex items-center justify-between border-b border-line px-6 py-4">
                    <h3 className="text-sm font-semibold text-text">Nodes</h3>
                    <Btn size="sm" tone="ghost" onClick={() => setShowAddNodes(true)}><Plus size={12} /> Add Nodes</Btn>
                  </div>
                  {!Array.isArray(selected.nodeIds) || selected.nodeIds.length === 0 ? (
                    <div className="px-6 py-4 text-sm text-text-muted">No nodes attached.</div>
                  ) : null}
                  <AdminTable label="Nodes">
                    <AdminTHead><AdminTh>ID</AdminTh><AdminTh>Name</AdminTh><AdminTh>FQDN</AdminTh><AdminTh></AdminTh></AdminTHead>
                    <AdminTBody>
                      {(Array.isArray(selected.nodeIds) ? selected.nodeIds : []).map((nodeId) => {
                        const node = Array.isArray(nodes) ? nodes.find(n => n.id === nodeId) : undefined;
                        return (
                          <AdminTr key={nodeId}>
                            <AdminTd className="font-mono text-xs text-text-muted"><code>{nodeId.slice(0, 8)}</code></AdminTd>
                            <AdminTd className="text-text">{node?.name ?? `Node ${nodeId.slice(0, 8)}`}</AdminTd>
                            <AdminTd><code className="text-xs text-text-subtle">{node?.fqdn}</code></AdminTd>
                            <AdminTd><Btn size="sm" tone="danger" onClick={() => detachNodeMut.mutate(nodeId)} disabled={detachNodeMut.isPending}><Link2Off size={12} /> Detach</Btn></AdminTd>
                          </AdminTr>
                        );
                      })}
                    </AdminTBody>
                  </AdminTable>
                </>
              )}
              {detailTab === "servers" && <AttachedServersTab mountId={selected.id} mountName={selected.name} />}
            </Card>
          </div>
        </div>

        {/* Add Eggs Modal */}
        {showAddEggs && selected && (
          <Modal title="Add Eggs" onClose={() => { setShowAddEggs(false); setSelectedEggIds([]); }}>
            <div className="space-y-4">
            {eggsQuery.isError ? (
              <div className="flex items-start justify-between gap-4 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
                <span>Could not load eggs: {eggsQuery.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void eggsQuery.refetch()}>Retry</Btn>
              </div>
            ) : nestsQuery.isError ? (
              <div className="flex items-start justify-between gap-4 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
                <span>Could not load nests: {nestsQuery.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void nestsQuery.refetch()}>Retry</Btn>
              </div>
            ) : null}
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {eggsQuery.isError ? null : Array.isArray(eggs) ? eggs.filter((egg) => !(Array.isArray(selected.templateIds) ? selected.templateIds : []).includes(egg.id)).map((egg) => (
                <label key={egg.id} className="flex items-center gap-3 rounded-lg border border-line bg-[var(--surface)] px-4 py-2.5 text-sm cursor-pointer hover:bg-overlay">
                  <input type="checkbox" checked={selectedEggIds.includes(egg.id)} onChange={(e) => setSelectedEggIds(e.target.checked ? [...selectedEggIds, egg.id] : selectedEggIds.filter(id => id !== egg.id))} className="accent-[var(--brand)]" />
                  <span className="text-text">{egg.name}</span>
                  <span className="ml-auto text-xs text-text-muted">{egg.id.slice(0, 8)}</span>
                </label>
              )) : null}
            </div>
            {attachEggsMut.isError ? (
              <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs text-danger">
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
            </div>
          </Modal>
        )}

        {/* Add Nodes Modal */}
        {showAddNodes && selected && (
          <Modal title="Add Nodes" onClose={() => { setShowAddNodes(false); setSelectedNodeIds([]); }}>
            <div className="space-y-4">
            {nodesQuery.isError ? (
              <div className="flex items-start justify-between gap-4 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
                <span>Could not load nodes: {nodesQuery.error.message}</span>
                <Btn size="sm" tone="ghost" onClick={() => void nodesQuery.refetch()}>Retry</Btn>
              </div>
            ) : null}
            <div className="space-y-2 max-h-80 overflow-y-auto">
              {nodesQuery.isError ? null : Array.isArray(nodes) ? nodes.filter(n => !(Array.isArray(selected.nodeIds) ? selected.nodeIds : []).includes(n.id)).map((node) => (
                <label key={node.id} className="flex items-center gap-3 rounded-lg border border-line bg-[var(--surface)] px-4 py-2.5 text-sm cursor-pointer hover:bg-overlay">
                  <input type="checkbox" checked={selectedNodeIds.includes(node.id)} onChange={(e) => setSelectedNodeIds(e.target.checked ? [...selectedNodeIds, node.id] : selectedNodeIds.filter(id => id !== node.id))} className="accent-[var(--brand)]" />
                  <span className="text-text">{node.name}</span>
                  <span className="ml-auto text-xs text-text-subtle">{node.fqdn}</span>
                </label>
              )) : null}
            </div>
            {attachNodesMut.isError ? (
              <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs text-danger">
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
            </div>
          </Modal>
        )}
      </AdminPageLayout>
    );
  }

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Storage Mounts"
        sub="Shared host volumes mounted into workloads. Mounts attach to beacons and templates with automatic inheritance for eligible servers."
        action={<Btn tone="primary" onClick={() => { setShowCreate(true); setCNodeIds([]); setCTemplateIds([]); setCErrors({}); }}><Plus size={14} /> New Mount</Btn>}
      />
      <div className="rounded-xl border border-line bg-overlay-subtle px-4 py-2 text-xs leading-5 text-text-subtle">
        <span className="font-semibold text-text">INFRA</span> · <span className="font-semibold text-text">Storage</span> — <code className="font-mono text-[11px]">Mounts</code> (this page) · <code className="font-mono">Volumes</code> · <code className="font-mono">Database Hosts</code> · <code className="font-mono">Backups</code> + providers. Mounts are <code className="font-mono">source → target</code> host paths with <code className="font-mono">nodeIds/templateIds</code> eligibility. See <code className="font-mono">/admin/databases</code> for DB hosts and <code className="font-mono">/admin/backups</code> for retention.
      </div>

      <Card>
        <CardHeader title="Mount List" icon={HardDrive} />
        {mountsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-text-muted">Loading</div>
        ) : mountsQuery.isError ? (
          <div className="p-4">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
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
                  <AdminTd className="font-mono text-xs text-text-muted"><code>{mount.id.slice(0, 8)}</code></AdminTd>
                  <AdminTd className="font-medium text-text">{mount.name}</AdminTd>
                  <AdminTd className="font-mono text-xs text-text-subtle">{mount.source}</AdminTd>
                  <AdminTd className="font-mono text-xs text-text-subtle">{mount.target}</AdminTd>
                  <AdminTd className="text-center text-text-subtle">{Array.isArray(mount.templateIds) ? mount.templateIds.length : 0}</AdminTd>
                  <AdminTd className="text-center text-text-subtle">{Array.isArray(mount.nodeIds) ? mount.nodeIds.length : 0}</AdminTd>
                  <AdminTd className="text-center text-text-subtle">{Array.isArray(mount.serverIds) ? mount.serverIds.length : 0}</AdminTd>
                </AdminTr>
              )) : null}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {showCreate ? (
        <Modal title="Create Mount" onClose={() => setShowCreate(false)}>
          <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="md:col-span-2">
              <Input label="Name" value={cName} onChange={setCName} placeholder="Shared Plugins" />
              {cErrors.name ? <p className="mt-1 text-sm text-danger">{cErrors.name}</p> : null}
              <p className="mt-1 text-xs text-text-subtle">Unique name used to separate this mount from another.</p>
            </div>
            <div className="md:col-span-2">
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Description</label>
              <textarea className="h-20 w-full rounded-lg border border-line bg-[var(--surface)] px-3 py-2 text-sm text-text" value={cDesc} onChange={(e) => setCDesc(e.target.value)} />
              <p className="mt-1 text-xs text-text-subtle">A longer description for this mount.</p>
            </div>
            <div>
              <Input label="Source Path" value={cSource} onChange={setCSource} placeholder="/mnt/shared/plugins" mono />
              {cErrors.source ? <p className="mt-1 text-sm text-danger">{cErrors.source}</p> : null}
            </div>
            <div>
              <Input label="Target Path" value={cTarget} onChange={setCTarget} placeholder="/plugins" mono />
              {cErrors.target ? <p className="mt-1 text-sm text-danger">{cErrors.target}</p> : null}
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Read Only</label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 rounded-lg border border-line bg-[var(--surface)] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-readonly" checked={!cReadOnly} onChange={() => setCReadOnly(false)} className="accent-[var(--brand)]" /> False
                </label>
                <label className="flex items-center gap-2 rounded-lg border border-line bg-[var(--surface)] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-readonly" checked={cReadOnly} onChange={() => setCReadOnly(true)} className="accent-[var(--brand)]" /> True
                </label>
              </div>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">User Mountable</label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 rounded-lg border border-line bg-[var(--surface)] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-usermount" checked={!cUserMount} onChange={() => setCUserMount(false)} className="accent-[var(--brand)]" /> False
                </label>
                <label className="flex items-center gap-2 rounded-lg border border-line bg-[var(--surface)] px-4 py-2 text-sm cursor-pointer">
                  <input type="radio" name="c-usermount" checked={cUserMount} onChange={() => setCUserMount(true)} className="accent-[var(--brand)]" /> True
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
              nodesLoading={nodesQuery.isPending}
              eggsLoading={eggsQuery.isPending}
            />
          </div>
          {createMut.isError ? (
            <div className="flex items-start gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs text-danger">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              <span>{createMut.error?.message || "An unexpected error occurred."}</span>
            </div>
          ) : null}
          {createMut.isSuccess ? (
            <div className="flex items-start gap-2 rounded-lg border border-ok-line bg-ok-subtle p-3 text-xs text-ok">
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
          </div>
        </Modal>
      ) : null}
      {renderConfirm()}
    </div>
  );
}
