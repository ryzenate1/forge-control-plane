"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Boxes, HardDrive, Play, RefreshCw, Server, Square, Tag, Trash2, RotateCw, Users, ShieldAlert } from "lucide-react";
import {
  deleteIncusInstance,
  fetchIncusCluster,
  fetchIncusImages,
  fetchIncusInstances,
  fetchIncusNodes,
  fetchIncusProfiles,
  fetchIncusStoragePools,
  restartIncusInstance,
  startIncusInstance,
  stopIncusInstance,
  createIncusInstance,
  type IncusClusterMember,
  type IncusImage,
  type IncusInstance,
  type IncusProfile,
  type IncusStoragePool,
} from "@/lib/api/incus";
import { incusStatusTone } from "@/lib/api/status";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  AdminSelect,
  AdminTabs,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  Pill,
  SectionHeader,
  selectStyle,
} from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { errorMessage, formatDate } from "@/lib/utils";

const TABS = [
  { id: "instances", label: "Instances", icon: Boxes },
  { id: "images", label: "Images", icon: Tag },
  { id: "profiles", label: "Profiles", icon: Users },
  { id: "storage", label: "Storage", icon: HardDrive },
  { id: "cluster", label: "Cluster", icon: Server },
] as const;

type Tab = (typeof TABS)[number]["id"];

/**
 * The Go layer speaks in product names ("Forge Virtualization", and a raw
 * `nodeId is required: multiple … nodes are registered`) that appear nowhere in
 * this UI. Translate the two shapes an operator can actually act on, and let
 * everything else through with its real reason.
 */
function explainIncusError(error: unknown): string {
  const raw = errorMessage(error, "The Incus request failed.");
  if (/nodeId is required/i.test(raw)) return "No Incus node was named for this request, and the API will not choose one. Pick the node at the top of the page.";
  if (/service unavailable/i.test(raw)) return "Incus is not configured for this panel, so there is no instance data to show.";
  return raw.replace(/Forge Virtualization/g, "Incus").replace(/Forge Incus/g, "Incus");
}

const NOT_REPORTED = "Not reported";

export default function IncusAdminPage() {
  const [tab, setTab] = useState<Tab>("instances");
  // Empty until the operator names a node. Every read below is `enabled` on it:
  // an unqualified request falls through to the service's environment-configured
  // endpoint, which can list instances from an Incus server that is not a Forge
  // node at all — under a banner that says no Incus nodes exist.
  const [nodeId, setNodeId] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirmAction, renderConfirm] = useConfirm();

  const nodesQ = useQuery({ queryKey: ["incus-nodes"], queryFn: fetchIncusNodes, retry: 1 });
  const nodes = useMemo(() => nodesQ.data ?? [], [nodesQ.data]);
  const nodeName = nodes.find((node) => node.id === nodeId)?.name ?? (nodeId ? `${nodeId.slice(0, 12)}…` : NOT_REPORTED);

  const targetSet = Boolean(nodeId);
  const instancesQ = useQuery({ queryKey: ["incus-instances", nodeId], queryFn: () => fetchIncusInstances(nodeId), enabled: targetSet && tab === "instances" });
  const imagesQ = useQuery({ queryKey: ["incus-images", nodeId], queryFn: () => fetchIncusImages(nodeId), enabled: targetSet && tab === "images" });
  const profilesQ = useQuery({ queryKey: ["incus-profiles", nodeId], queryFn: () => fetchIncusProfiles(nodeId), enabled: targetSet && tab === "profiles" });
  const poolsQ = useQuery({ queryKey: ["incus-storage", nodeId], queryFn: () => fetchIncusStoragePools(nodeId), enabled: targetSet && tab === "storage" });
  const clusterQ = useQuery({ queryKey: ["incus-cluster", nodeId], queryFn: () => fetchIncusCluster(nodeId), enabled: targetSet && tab === "cluster" });

  const refresh = () => {
    for (const key of ["incus-nodes", "incus-instances", "incus-images", "incus-profiles", "incus-storage", "incus-cluster"]) {
      void qc.invalidateQueries({ queryKey: [key] });
    }
  };

  const onError = (action: string) => (err: unknown) =>
    toast({ tone: "error", title: `Failed to ${action}`, message: explainIncusError(err) });

  /** Every lifecycle toast names the instance it acted on, not the word "instance". */
  const notify = (action: string) => (name: string) => {
    toast({ tone: "success", title: `${action} · ${name}`, message: `on ${nodeName}` });
    void qc.invalidateQueries({ queryKey: ["incus-instances"] });
  };

  const startMut = useMutation({ mutationFn: (name: string) => startIncusInstance(name, nodeId), onSuccess: (_r, name) => notify("Started")(name), onError: onError("start instance") });
  const stopMut = useMutation({ mutationFn: (vars: { force: boolean; name: string }) => stopIncusInstance(vars.name, { force: vars.force, nodeId }), onSuccess: (_r, vars) => notify(vars.force ? "Force-stopped" : "Stopped")(vars.name), onError: onError("stop instance") });
  const restartMut = useMutation({ mutationFn: (name: string) => restartIncusInstance(name, nodeId), onSuccess: (_r, name) => notify("Restarted")(name), onError: onError("restart instance") });
  const deleteMut = useMutation({ mutationFn: (name: string) => deleteIncusInstance(name, { nodeId }), onSuccess: (_r, name) => notify("Deleted")(name), onError: onError("delete instance") });

  const handleStop = async (name: string, force: boolean) => {
    const ok = await confirmAction({
      confirmLabel: force ? "Force stop" : "Stop instance",
      danger: force,
      description: `${name} on ${nodeName} will ${force ? "be killed without a graceful shutdown — any unwritten state in its filesystem is lost" : "be asked to shut down gracefully"}. Workloads running inside it go with it.`,
      title: force ? `Force-stop ${name}?` : `Stop ${name}?`,
    });
    if (ok) stopMut.mutate({ force, name });
  };

  const handleDelete = async (name: string) => {
    const ok = await confirmAction({
      confirmLabel: "Delete instance",
      danger: true,
      description: `${name} on ${nodeName} and its disk are destroyed. This is not recoverable from this page — the instance and its root filesystem go with it.`,
      title: `Delete ${name}?`,
    });
    if (ok) deleteMut.mutate(name);
  };

  const nodesLoading = nodesQ.isPending;
  const nodesFailed = nodesQ.isError;
  const noIncusNodes = nodesQ.isSuccess && nodes.length === 0;

  // Capability gate: with zero Incus nodes there is no console to render. The
  // tabs used to show anyway, fire five untargeted requests, and could list
  // instances from an Incus endpoint Forge never registered.
  if (nodesLoading || nodesFailed || noIncusNodes) {
    return (
      <AdminPageLayout>
        {renderConfirm()}
        <SectionHeader
          info={{
            description: "System containers and virtual machines on Beacon nodes that report the Incus runtime, managed over the Incus REST API.",
            eyebrow: "Architecture & Semantics",
            sections: [
              {
                content: "Instance, image, profile, storage and cluster reads all scope to one named node. Nothing is requested until you pick it, because an untargeted request would be answered by whatever endpoint the panel is configured with — which may not be a Forge node at all.",
                icon: Server,
                title: "One named node",
              },
              {
                content: "A standalone host shows no cluster members — that is expected, not an error. The same empty list is also what an unreachable Incus API returns, so this page reports read failures rather than calling them standalone.",
                icon: Boxes,
                title: "Cluster vs standalone",
              },
            ],
            title: "Incus runtimes",
            triggerLabel: "About Incus",
          }}
        />
        {nodesLoading ? <AdminLoadingState label="Loading Incus nodes…" /> : null}
        {nodesFailed ? <AdminErrorState message={explainIncusError(nodesQ.error)} retry={() => void nodesQ.refetch()} /> : null}
        {noIncusNodes ? (
          <EmptyState
            icon={Server}
            message="No node reports the Incus runtime, so there is nothing to manage here. Register a node with the Incus provider and trust the panel's client certificate; controls stay disabled until then."
            title="No Incus nodes"
          />
        ) : null}
      </AdminPageLayout>
    );
  }

  const activeLabel = TABS.find((entry) => entry.id === tab)?.label ?? "";

  return (
    <AdminPageLayout>
      {renderConfirm()}
      <SectionHeader
        info={{
          description: "System containers and virtual machines on Beacon nodes that report the Incus runtime, managed over the Incus REST API.",
          eyebrow: "Architecture & Semantics",
          sections: [
            {
              content: "Instance, image, profile, storage and cluster reads all scope to one named node. Nothing is requested until you pick it, because an untargeted request would be answered by whatever endpoint the panel is configured with — which may not be a Forge node at all.",
              icon: Server,
              title: "One named node",
            },
            {
              content: "Start, stop, restart and delete run against the same named node. Deleting destroys the instance and its disk, so it asks first and names the machine.",
              icon: ShieldAlert,
              title: "Lifecycle actions",
            },
            {
              content: "A standalone host shows no cluster members — that is expected, not an error. The same empty list is also what an unreachable Incus API returns, so this page reports read failures rather than calling them standalone.",
              icon: Boxes,
              title: "Cluster vs standalone",
            },
          ],
          title: "Incus runtimes",
          triggerLabel: "About Incus",
        }}
        action={
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-48">
              <AdminSelect
                label="Target node"
                onChange={setNodeId}
                options={nodes.map((node) => ({ value: node.id, label: `${node.name} · ${node.runtimeProvider}` }))}
                placeholder="Select a node…"
                value={nodeId}
              />
            </div>
            <Btn onClick={refresh} size="sm" tone="ghost"><RefreshCw size={14} /> Refresh</Btn>
            <Btn
              disabled={!targetSet}
              onClick={() => setCreateOpen(true)}
              size="sm"
              tone="primary"
              title={targetSet ? `Create an instance on ${nodeName}` : "Select an Incus node first"}
            >
              New instance
            </Btn>
            <Pill tone="neutral">{nodes.length} Incus node{nodes.length === 1 ? "" : "s"}</Pill>
          </div>
        }
      />

      {!targetSet ? (
        <EmptyState
          icon={Server}
          message="Choose the node above. Instances, images, profiles, storage pools and cluster members are all read from that one Incus server, and nothing is requested until you name it."
          title="No node selected"
        />
      ) : (
        <>
          <AdminTabs active={tab} label="Incus sections" onChange={(id) => setTab(id as Tab)} tabs={TABS.map((entry) => ({ id: entry.id, label: entry.label, icon: entry.icon }))} />

          <div aria-label={`${activeLabel} on ${nodeName}`} role="tabpanel">
            {tab === "instances" && (
              <Card>
                <CardHeader icon={Boxes} title="Instances" />
                {instancesQ.isPending ? (
                  <AdminLoadingState label="Loading instances…" />
                ) : instancesQ.isError ? (
                  <div className="p-4"><AdminErrorState message={explainIncusError(instancesQ.error)} retry={() => void instancesQ.refetch()} /></div>
                ) : (instancesQ.data ?? []).length === 0 ? (
                  <EmptyState icon={Boxes} message={`This node reported no instances in the projects it exposes.`} title="No instances" />
                ) : (
                  <div className="overflow-x-auto">
                    <table aria-label={`Incus instances on ${nodeName}`} className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                          <th className="px-4 py-2.5 font-medium">Name</th>
                          <th className="px-4 py-2.5 font-medium">Project</th>
                          <th className="px-4 py-2.5 font-medium">Status</th>
                          <th className="px-4 py-2.5 font-medium">Type</th>
                          <th className="px-4 py-2.5 font-medium">Profiles</th>
                          <th className="px-4 py-2.5 font-medium">Actions</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-line">
                        {(instancesQ.data as IncusInstance[]).map((instance) => {
                          const running = instance.status === "Running";
                          // Only this instance's own in-flight action marks the row busy: a shared
                          // `pending` flag used to draw a spinner on every row and disable Stop,
                          // Restart and Delete for objects nobody had touched.
                          const rowBusy = (startMut.isPending && startMut.variables === instance.name)
                            || (stopMut.isPending && stopMut.variables?.name === instance.name)
                            || (restartMut.isPending && restartMut.variables === instance.name)
                            || (deleteMut.isPending && deleteMut.variables === instance.name);
                          return (
                            <tr className="hover:bg-overlay-subtle" key={`${instance.project ?? "default"}/${instance.name}`}>
                              <td className="px-4 py-3 font-mono text-xs text-text">{instance.name}</td>
                              <td className="px-4 py-3 font-mono text-xs text-text-subtle">{instance.project ?? "default"}</td>
                              <td className="px-4 py-3"><Pill tone={incusStatusTone(instance.status)}>{instance.status || NOT_REPORTED}</Pill></td>
                              <td className="px-4 py-3 text-xs text-text-subtle">{instance.instanceType || NOT_REPORTED}</td>
                              <td className="px-4 py-3 text-xs text-text-subtle">{(instance.profiles ?? []).join(", ") || NOT_REPORTED}</td>
                              <td className="px-4 py-3">
                                <div className="flex flex-wrap gap-1">
                                  <Btn ariaLabel={`Start ${instance.name}`} disabled={running || rowBusy} loading={startMut.isPending && startMut.variables === instance.name} onClick={() => startMut.mutate(instance.name)} size="sm" tone="ghost" title="Start"><Play size={12} /></Btn>
                                  <Btn ariaLabel={`Stop ${instance.name}`} disabled={!running || rowBusy} loading={stopMut.isPending && stopMut.variables?.name === instance.name && !stopMut.variables?.force} onClick={() => void handleStop(instance.name, false)} size="sm" tone="warning" title="Stop (graceful)"><Square size={12} /></Btn>
                                  <Btn ariaLabel={`Force stop ${instance.name}`} disabled={!running || rowBusy} loading={stopMut.isPending && stopMut.variables?.name === instance.name && Boolean(stopMut.variables?.force)} onClick={() => void handleStop(instance.name, true)} size="sm" tone="danger" title="Force stop — no graceful shutdown"><ShieldAlert size={12} /></Btn>
                                  <Btn ariaLabel={`Restart ${instance.name}`} disabled={!running || rowBusy} loading={restartMut.isPending && restartMut.variables === instance.name} onClick={() => restartMut.mutate(instance.name)} size="sm" tone="ghost" title="Restart"><RotateCw size={12} /></Btn>
                                  <Btn ariaLabel={`Delete ${instance.name}`} disabled={rowBusy} loading={deleteMut.isPending && deleteMut.variables === instance.name} onClick={() => void handleDelete(instance.name)} size="sm" tone="danger" title="Delete instance and its disk"><Trash2 size={12} /></Btn>
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>
            )}

            {tab === "images" && (
              <Card>
                <CardHeader icon={Tag} title="Images" />
                {imagesQ.isPending ? (
                  <AdminLoadingState label="Loading images…" />
                ) : imagesQ.isError ? (
                  <div className="p-4"><AdminErrorState message={explainIncusError(imagesQ.error)} retry={() => void imagesQ.refetch()} /></div>
                ) : (imagesQ.data ?? []).length === 0 ? (
                  <EmptyState icon={Tag} message="This Incus server reported no images, so there is no alias to create an instance from." title="No images" />
                ) : (
                  <div className="overflow-x-auto">
                    <table aria-label={`Incus images on ${nodeName}`} className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                          <th className="px-4 py-2.5 font-medium">Fingerprint</th>
                          <th className="px-4 py-2.5 font-medium">Aliases</th>
                          <th className="px-4 py-2.5 font-medium">Type</th>
                          <th className="px-4 py-2.5 font-medium">Size</th>
                          <th className="px-4 py-2.5 font-medium">Created</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-line">
                        {(imagesQ.data as IncusImage[]).map((image) => (
                          <tr className="hover:bg-overlay-subtle" key={image.fingerprint}>
                            <td className="px-4 py-3 font-mono text-xs text-text">{image.fingerprint.slice(0, 12)}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{(image.aliases ?? []).map((alias) => alias.name).join(", ") || NOT_REPORTED}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{image.type || NOT_REPORTED}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">
                              {typeof image.size === "number" && Number.isFinite(image.size) ? `${Math.round(image.size / (1024 * 1024))} MiB` : NOT_REPORTED}
                            </td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{formatDate(image.createdAt, NOT_REPORTED)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>
            )}

            {tab === "profiles" && (
              <Card>
                <CardHeader icon={Users} title="Profiles" />
                {profilesQ.isPending ? (
                  <AdminLoadingState label="Loading profiles…" />
                ) : profilesQ.isError ? (
                  <div className="p-4"><AdminErrorState message={explainIncusError(profilesQ.error)} retry={() => void profilesQ.refetch()} /></div>
                ) : (profilesQ.data ?? []).length === 0 ? (
                  <EmptyState icon={Users} message="This Incus server reported no profiles." title="No profiles" />
                ) : (
                  <div className="overflow-x-auto">
                    <table aria-label={`Incus profiles on ${nodeName}`} className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                          <th className="px-4 py-2.5 font-medium">Name</th>
                          <th className="px-4 py-2.5 font-medium">Description</th>
                          <th className="px-4 py-2.5 font-medium">Config keys</th>
                          <th className="px-4 py-2.5 font-medium">Devices</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-line">
                        {(profilesQ.data as IncusProfile[]).map((profile) => (
                          <tr className="hover:bg-overlay-subtle" key={profile.name}>
                            <td className="px-4 py-3 font-mono text-xs text-text">{profile.name}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{profile.description || NOT_REPORTED}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{Object.keys(profile.config ?? {}).join(", ") || "none"}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{Object.keys(profile.devices ?? {}).join(", ") || "none"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>
            )}

            {tab === "storage" && (
              <Card>
                <CardHeader icon={HardDrive} title="Storage pools" />
                {poolsQ.isPending ? (
                  <AdminLoadingState label="Loading storage pools…" />
                ) : poolsQ.isError ? (
                  <div className="p-4"><AdminErrorState message={explainIncusError(poolsQ.error)} retry={() => void poolsQ.refetch()} /></div>
                ) : (poolsQ.data ?? []).length === 0 ? (
                  <EmptyState icon={HardDrive} message="This Incus server reported no storage pools." title="No storage pools" />
                ) : (
                  <div className="overflow-x-auto">
                    <table aria-label={`Incus storage pools on ${nodeName}`} className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                          <th className="px-4 py-2.5 font-medium">Name</th>
                          <th className="px-4 py-2.5 font-medium">Driver</th>
                          <th className="px-4 py-2.5 font-medium">Status</th>
                          <th className="px-4 py-2.5 font-medium">Used by</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-line">
                        {(poolsQ.data as IncusStoragePool[]).map((pool) => (
                          <tr className="hover:bg-overlay-subtle" key={pool.name}>
                            <td className="px-4 py-3 font-mono text-xs text-text">{pool.name}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{pool.driver || NOT_REPORTED}</td>
                            <td className="px-4 py-3"><Pill tone={incusStatusTone(pool.status || "unknown")}>{pool.status || NOT_REPORTED}</Pill></td>
                            <td className="px-4 py-3 text-xs text-text-subtle">
                              {pool.usedBy === undefined
                                ? `Users ${NOT_REPORTED}`
                                : pool.usedBy.length === 0
                                  ? "No reported users"
                                  : pool.usedBy.join(", ")}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>
            )}

            {tab === "cluster" && (
              <Card>
                <CardHeader icon={Server} title="Cluster members" />
                {clusterQ.isPending ? (
                  <AdminLoadingState label="Loading cluster…" />
                ) : clusterQ.isError ? (
                  <div className="p-4"><AdminErrorState message={explainIncusError(clusterQ.error)} retry={() => void clusterQ.refetch()} /></div>
                ) : (clusterQ.data ?? []).length === 0 ? (
                  <EmptyState
                    icon={Server}
                    message={`${nodeName} reported no cluster members. That is what a standalone host looks like — it is also what an unreachable Incus API or a non-clustered project returns, so this is a reading of the list, not a health claim.`}
                    title="No cluster members reported"
                  />
                ) : (
                  <div className="overflow-x-auto">
                    <table aria-label={`Incus cluster members on ${nodeName}`} className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-line text-left text-xs uppercase tracking-wider text-text-subtle">
                          <th className="px-4 py-2.5 font-medium">Server</th>
                          <th className="px-4 py-2.5 font-medium">Roles</th>
                          <th className="px-4 py-2.5 font-medium">URL</th>
                          <th className="px-4 py-2.5 font-medium">Failure domain</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-line">
                        {(clusterQ.data as IncusClusterMember[]).map((member) => (
                          <tr className="hover:bg-overlay-subtle" key={member.serverName}>
                            <td className="px-4 py-3 font-mono text-xs text-text">{member.serverName}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{(member.roles ?? []).join(", ") || NOT_REPORTED}</td>
                            <td className="px-4 py-3 font-mono text-xs text-text-subtle">{member.url || NOT_REPORTED}</td>
                            <td className="px-4 py-3 text-xs text-text-subtle">{member.failureDomain || NOT_REPORTED}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </Card>
            )}
          </div>
        </>
      )}

      {createOpen && (
        <CreateInstanceModal
          nodeName={nodeName}
          onClose={() => setCreateOpen(false)}
          onCreated={(name) => { notify("Created")(name); setCreateOpen(false); }}
          onError={onError("create instance")}
          nodeId={nodeId}
        />
      )}
    </AdminPageLayout>
  );
}

function CreateInstanceModal({
  nodeName,
  nodeId,
  onClose,
  onCreated,
  onError,
}: {
  nodeName: string;
  nodeId: string;
  onClose: () => void;
  onCreated: (name: string) => void;
  onError: (err: Error) => void;
}) {
  const [name, setName] = useState("");
  const [source, setSource] = useState("");
  const [instanceType, setInstanceType] = useState("container");
  const qc = useQueryClient();

  const imagesQ = useQuery({ queryKey: ["incus-images", nodeId], queryFn: () => fetchIncusImages(nodeId), enabled: Boolean(nodeId) });
  const aliases = useMemo(() => {
    const found: string[] = [];
    for (const image of imagesQ.data ?? []) {
      for (const alias of image.aliases ?? []) if (alias.name) found.push(alias.name);
    }
    return Array.from(new Set(found)).sort();
  }, [imagesQ.data]);

  const createMut = useMutation({
    mutationFn: () => createIncusInstance({
      name: name.trim(),
      // Incus InstancesPost selects a VM with `source.vm`; the spec is forwarded
      // to /1.0/instances unchanged, so nothing outside that shape is invented.
      source: { type: "image", alias: source.trim(), vm: instanceType === "virtual-machine" },
    }, nodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["incus-instances"] });
      onCreated(name.trim());
    },
    onError: (err: Error) => onError(err),
  });

  const canSubmit = Boolean(nodeId) && Boolean(name.trim()) && Boolean(source.trim());

  return (
    <Modal
      description={`Creates one instance on ${nodeName} from an image alias. Profiles, devices and resource limits are not part of this form — the page would have to invent them, so it does not offer them.`}
      onClose={onClose}
      title="New Incus instance"
    >
      <div className="space-y-4">
        <Input label="Instance name" mono onChange={setName} placeholder="my-container" value={name} />
        <label className="block">
          <span className="ui-label mb-1.5">Image alias or fingerprint</span>
          <input
            className={selectStyle}
            list="incus-image-aliases"
            onChange={(event) => setSource(event.target.value)}
            placeholder="ubuntu/24.04"
            value={source}
          />
          <datalist id="incus-image-aliases">
            {aliases.map((alias) => <option key={alias} value={alias} />)}
          </datalist>
          <span className="mt-1.5 block text-xs text-text-subtle">
            {imagesQ.isPending
              ? "Reading this node's image list for suggestions…"
              : imagesQ.isError
                ? `Image list unavailable (${explainIncusError(imagesQ.error)}). You can still type an alias or fingerprint.`
                : aliases.length
                  ? `${aliases.length} alias${aliases.length === 1 ? "" : "es"} reported on ${nodeName}.`
                  : `This node reported no image aliases, so a typed value may not resolve.`}
          </span>
        </label>
        <AdminSelect
          label="Instance type"
          onChange={setInstanceType}
          options={[{ value: "container", label: "System container" }, { value: "virtual-machine", label: "Virtual machine" }]}
          value={instanceType}
        />
        <div className="flex justify-end gap-2 pt-2">
          <Btn onClick={onClose} tone="ghost">Cancel</Btn>
          <Btn
            disabled={!canSubmit}
            loading={createMut.isPending}
            onClick={() => void createMut.mutate()}
            tone="primary"
            title={canSubmit ? `Create ${name.trim() || "the instance"} on ${nodeName}` : "Name and image are both required"}
          >
            Create
          </Btn>
        </div>
      </div>
    </Modal>
  );
}
