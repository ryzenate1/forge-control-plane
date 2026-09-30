"use client";

import { useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, FileText, Globe, KeyRound, Network, Plus, Route, Shield, Tags, Trash2, Users } from "lucide-react";
import { useToast } from "@/components/ui/toast";
import {
  AdminErrorState, AdminLoadingState, AdminPageLayout, AdminSelect, AdminTabs, Btn, Card, CardHeader,
  EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea,
} from "@/components/admin/admin-ui";
import { errorMessage } from "@/lib/utils";
import {
  approveNetBirdPeer, createNetBirdACL, createNetBirdGroup, createNetBirdNetwork, createNetBirdRoute,
  createNetBirdSetupKey, deleteNetBirdACL, deleteNetBirdGroup, deleteNetBirdNetwork, deleteNetBirdPeer, deleteNetBirdRoute,
  denyNetBirdPeer, fetchNetBirdACLs, fetchNetBirdDNSSettings, fetchNetBirdGroups, fetchNetBirdNetworks,
  fetchNetBirdPeers, fetchNetBirdRoutes, updateNetBirdDNSSettings,
  type NetBirdACLRule, type NetBirdDNSConfig, type NetBirdGroup, type NetBirdNetwork, type NetBirdPeer,
  type NetBirdRoute, type NetBirdSetupKey,
} from "@/lib/api/netbird";

type Tab = "peers" | "networks" | "groups" | "routes" | "acls" | "dns" | "setup-keys";

const tabs: Array<{ id: Tab; label: string; icon: typeof Network }> = [
  { id: "peers", label: "Peers", icon: Network },
  { id: "networks", label: "Networks", icon: Globe },
  { id: "groups", label: "Groups", icon: Users },
  { id: "routes", label: "Routes", icon: Route },
  { id: "acls", label: "ACLs", icon: Shield },
  { id: "dns", label: "DNS", icon: FileText },
  { id: "setup-keys", label: "Setup keys", icon: KeyRound },
];

const netbirdKeys = {
  peers: ["admin", "netbird", "peers"],
  networks: ["admin", "netbird", "networks"],
  groups: ["admin", "netbird", "groups"],
  routes: ["admin", "netbird", "routes"],
  acls: ["admin", "netbird", "acls"],
  dns: ["admin", "netbird", "dns"],
};

function isPendingPeer(peer: NetBirdPeer): boolean {
  return Boolean(peer.pending_approval || peer.approval_required);
}

export default function AdminNetBirdPage() {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("peers");

  const [showCreateNetwork, setShowCreateNetwork] = useState(false);
  const [networkForm, setNetworkForm] = useState({ name: "", description: "" });
  const [showCreateGroup, setShowCreateGroup] = useState(false);
  const [groupForm, setGroupForm] = useState({ name: "" });
  const [showCreateRoute, setShowCreateRoute] = useState(false);
  const [routeForm, setRouteForm] = useState({ description: "", network: "", peer: "", metric: "100", masquerade: true, enabled: true });
  const [showCreateACL, setShowCreateACL] = useState(false);
  const [aclForm, setAclForm] = useState({ name: "", description: "", sourceGroup: "", destinationGroup: "", protocol: "tcp", port: "" });
  const [dnsGroups, setDnsGroups] = useState<string | null>(null);
  const [setupKeyForm, setSetupKeyForm] = useState({ name: "", description: "", type: "reusable", expiresInDays: "30", ephemeral: false });
  const [createdKey, setCreatedKey] = useState<NetBirdSetupKey | null>(null);

  const peersQuery = useQuery({ queryKey: netbirdKeys.peers, queryFn: fetchNetBirdPeers });
  const networksQuery = useQuery({ queryKey: netbirdKeys.networks, queryFn: fetchNetBirdNetworks });
  const groupsQuery = useQuery({ queryKey: netbirdKeys.groups, queryFn: fetchNetBirdGroups });
  const routesQuery = useQuery({ queryKey: netbirdKeys.routes, queryFn: fetchNetBirdRoutes });
  const aclsQuery = useQuery({ queryKey: netbirdKeys.acls, queryFn: fetchNetBirdACLs });
  const dnsQuery = useQuery({ queryKey: netbirdKeys.dns, queryFn: fetchNetBirdDNSSettings });

  const peers = useMemo(() => peersQuery.data ?? [], [peersQuery.data]);
  const networks = useMemo(() => networksQuery.data ?? [], [networksQuery.data]);
  const groups = useMemo(() => groupsQuery.data ?? [], [groupsQuery.data]);
  const routes = useMemo(() => routesQuery.data ?? [], [routesQuery.data]);
  const acls = useMemo(() => aclsQuery.data ?? [], [aclsQuery.data]);

  const groupOptions = groups.map((g) => ({ value: g.id, label: g.name }));
  const peerOptions = peers.map((p) => ({ value: p.id, label: `${p.name || p.id} (${p.ip})` }));

  // Draft wins once the user edits; until then mirror the loaded settings.
  const dnsDraft = dnsGroups ?? (dnsQuery.data?.disabled_management_groups ?? []).join(", ");

  const failToast = (title: string) => (err: unknown) =>
    toast({ tone: "error", title, message: errorMessage(err, "The NetBird management API is unreachable or returned an error.") });

  const approveMutation = useMutation({
    mutationFn: (id: string) => approveNetBirdPeer(id),
    onSuccess: (peer) => {
      if (!peer) {
        toast({ tone: "error", title: "Approve peer", message: "NetBird returned no peer; the action may not have applied." });
        return;
      }
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.peers });
      toast({ tone: "success", title: "Peer approved", message: `${peer.name || peer.id} joined the mesh.` });
    },
    onError: failToast("Failed to approve peer"),
  });

  const denyMutation = useMutation({
    mutationFn: (id: string) => denyNetBirdPeer(id),
    onSuccess: (peer) => {
      if (!peer) {
        toast({ tone: "error", title: "Deny peer", message: "NetBird returned no peer; the action may not have applied." });
        return;
      }
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.peers });
      toast({ tone: "success", title: "Peer denied", message: `${peer.name || peer.id} stays pending.` });
    },
    onError: failToast("Failed to deny peer"),
  });

  const deletePeerMutation = useMutation({
    mutationFn: (id: string) => deleteNetBirdPeer(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.peers });
      toast({ tone: "success", title: "Peer deleted" });
    },
    onError: failToast("Failed to delete peer"),
  });

  const createNetworkMutation = useMutation({
    mutationFn: () => createNetBirdNetwork({ name: networkForm.name.trim(), description: networkForm.description.trim() || undefined }),
    onSuccess: (network: NetBirdNetwork) => {
      if (!network?.id) {
        toast({ tone: "error", title: "Failed to create network", message: "NetBird returned no network id." });
        return;
      }
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.networks });
      setShowCreateNetwork(false);
      setNetworkForm({ name: "", description: "" });
      toast({ tone: "success", title: "Network created", message: `${network.name} is ready for resources and policies.` });
    },
    onError: failToast("Failed to create network"),
  });

  const deleteNetworkMutation = useMutation({
    mutationFn: (id: string) => deleteNetBirdNetwork(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.networks });
      toast({ tone: "success", title: "Network deleted" });
    },
    onError: failToast("Failed to delete network"),
  });

  const createGroupMutation = useMutation({
    mutationFn: () => createNetBirdGroup({ name: groupForm.name.trim() }),
    onSuccess: (group: NetBirdGroup) => {
      if (!group?.id) {
        toast({ tone: "error", title: "Failed to create group", message: "NetBird returned no group id." });
        return;
      }
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.groups });
      setShowCreateGroup(false);
      setGroupForm({ name: "" });
      toast({ tone: "success", title: "Group created", message: group.name });
    },
    onError: failToast("Failed to create group"),
  });

  const deleteGroupMutation = useMutation({
    mutationFn: (id: string) => deleteNetBirdGroup(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.groups });
      toast({ tone: "success", title: "Group deleted" });
    },
    onError: failToast("Failed to delete group"),
  });

  const createRouteMutation = useMutation({
    mutationFn: () => createNetBirdRoute({
      description: routeForm.description.trim(),
      network: routeForm.network.trim() || undefined,
      peer: routeForm.peer || undefined,
      enabled: routeForm.enabled,
      metric: Number(routeForm.metric) || 0,
      masquerade: routeForm.masquerade,
    }),
    onSuccess: (route: NetBirdRoute) => {
      if (!route?.id) {
        toast({ tone: "error", title: "Failed to create route", message: "NetBird returned no route id." });
        return;
      }
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.routes });
      setShowCreateRoute(false);
      setRouteForm({ description: "", network: "", peer: "", metric: "100", masquerade: true, enabled: true });
      toast({ tone: "success", title: "Route created", message: route.network || route.description || route.id });
    },
    onError: failToast("Failed to create route"),
  });

  const deleteRouteMutation = useMutation({
    mutationFn: (id: string) => deleteNetBirdRoute(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.routes });
      toast({ tone: "success", title: "Route deleted" });
    },
    onError: failToast("Failed to delete route"),
  });

  const createACLMutation = useMutation({
    mutationFn: () => {
      const source = groups.find((g) => g.id === aclForm.sourceGroup);
      const destination = groups.find((g) => g.id === aclForm.destinationGroup);
      const rule: NetBirdACLRule = {
        name: aclForm.name.trim(),
        description: aclForm.description.trim() || undefined,
        enabled: true,
        rules: [{
          id: crypto.randomUUID(),
          name: aclForm.name.trim(),
          enabled: true,
          action: "accept",
          protocol: aclForm.protocol,
          bidirectional: true,
          sources: source ? [{ id: source.id, name: source.name }] : [],
          destinations: destination ? [{ id: destination.id, name: destination.name }] : [],
          ports: aclForm.port.trim() ? aclForm.port.split(",").map((p) => p.trim()).filter(Boolean) : undefined,
        }],
      };
      return createNetBirdACL(rule);
    },
    onSuccess: (acl: NetBirdACLRule) => {
      if (!acl?.id) {
        toast({ tone: "error", title: "Failed to create ACL", message: "NetBird returned no policy id." });
        return;
      }
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.acls });
      setShowCreateACL(false);
      setAclForm({ name: "", description: "", sourceGroup: "", destinationGroup: "", protocol: "tcp", port: "" });
      toast({ tone: "success", title: "ACL created", message: acl.name });
    },
    onError: failToast("Failed to create ACL"),
  });

  const deleteACLMutation = useMutation({
    mutationFn: (id: string) => deleteNetBirdACL(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.acls });
      toast({ tone: "success", title: "ACL deleted" });
    },
    onError: failToast("Failed to delete ACL"),
  });

  const saveDnsMutation = useMutation({
    mutationFn: () => {
      const disabled = dnsDraft.split(",").map((s) => s.trim()).filter(Boolean);
      const body: NetBirdDNSConfig = { disabled_management_groups: disabled };
      return updateNetBirdDNSSettings(body);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: netbirdKeys.dns });
      toast({ tone: "success", title: "DNS settings saved" });
    },
    onError: failToast("Failed to save DNS settings"),
  });

  const createSetupKeyMutation = useMutation({
    mutationFn: () => createNetBirdSetupKey({
      name: setupKeyForm.name.trim(),
      description: setupKeyForm.description.trim() || undefined,
      type: setupKeyForm.type as "one-off" | "reusable",
      expires_in: Math.max(0, Number(setupKeyForm.expiresInDays) || 0) * 86400,
      ephemeral: setupKeyForm.ephemeral,
    }),
    onSuccess: (key: NetBirdSetupKey) => {
      if (!key?.key) {
        toast({ tone: "error", title: "Failed to create setup key", message: "NetBird returned no secret; the key was not created." });
        return;
      }
      setCreatedKey(key);
      setSetupKeyForm({ name: "", description: "", type: "reusable", expiresInDays: "30", ephemeral: false });
      toast({ tone: "success", title: "Setup key created", message: "Copy it now — NetBird only reveals the secret once." });
    },
    onError: failToast("Failed to create setup key"),
  });

  const copyKey = async () => {
    if (!createdKey?.key) return;
    try {
      await navigator.clipboard.writeText(createdKey.key);
      toast({ tone: "success", title: "Setup key copied to clipboard" });
    } catch (err) {
      toast({ tone: "error", title: "Copy failed", message: errorMessage(err, "Clipboard access was denied by the browser.") });
    }
  };

  const renderQuery = (
    query: { isLoading: boolean; isError: boolean; error: unknown; refetch: () => unknown },
    loadingLabel: string,
    errorFallback: string,
    body: ReactNode,
  ) => {
    if (query.isLoading) return <AdminLoadingState label={loadingLabel} />;
    if (query.isError) return <AdminErrorState message={errorMessage(query.error, errorFallback)} retry={() => void query.refetch()} />;
    return <>{body}</>;
  };

  return (
    <AdminPageLayout>
      <SectionHeader title="NetBird VPN" sub="WireGuard mesh VPN control plane: peers, networks, routes, ACLs, DNS and setup keys." />

      <AdminTabs tabs={tabs} active={tab} onChange={(id) => setTab(id as Tab)} />

      {tab === "peers" && (
        <Card>
          <CardHeader title="Peers" icon={Network} />
          {renderQuery(peersQuery, "Loading peers…", "Failed to load NetBird peers.", (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                    <th className="px-4 py-3">Name</th>
                    <th className="px-4 py-3">IP</th>
                    <th className="px-4 py-3">OS</th>
                    <th className="px-4 py-3">Last seen</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {peers.map((peer) => (
                    <tr key={peer.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-medium text-slate-200">{peer.name || peer.id}</td>
                      <td className="px-4 py-3 font-mono text-xs text-slate-400">{peer.ip}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{peer.os || "—"}</td>
                      <td className="px-4 py-3 text-xs text-slate-400">{peer.last_seen ? new Date(peer.last_seen).toLocaleString() : "—"}</td>
                      <td className="px-4 py-3">
                        {isPendingPeer(peer) ? (
                          <Pill tone="yellow">Pending approval</Pill>
                        ) : peer.connected ? (
                          <Pill tone="green">Connected</Pill>
                        ) : (
                          <Pill tone="neutral">Disconnected</Pill>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex justify-end gap-1">
                          {isPendingPeer(peer) ? (
                            <>
                              <Btn size="sm" tone="success" onClick={() => approveMutation.mutate(peer.id)} disabled={approveMutation.isPending}>
                                <Plus size={12} /> Approve
                              </Btn>
                              <Btn size="sm" tone="warning" onClick={() => denyMutation.mutate(peer.id)} disabled={denyMutation.isPending}>Deny</Btn>
                            </>
                          ) : null}
                          <Btn size="sm" tone="danger" onClick={() => deletePeerMutation.mutate(peer.id)} disabled={deletePeerMutation.isPending} title="Remove peer from the mesh">
                            <Trash2 size={12} />
                          </Btn>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {peers.length === 0 ? <EmptyState icon={Network} message="No peers are enrolled in the mesh yet." /> : null}
            </div>
          ))}
        </Card>
      )}

      {tab === "networks" && (
        <Card>
          <CardHeader
            title="Networks"
            icon={Globe}
            action={<Btn size="sm" onClick={() => setShowCreateNetwork(true)}><Plus size={12} /> Create network</Btn>}
          />
          {renderQuery(networksQuery, "Loading networks…", "Failed to load NetBird networks.", (
            <div className="divide-y divide-white/[0.04]">
              {networks.map((network) => (
                <div key={network.id} className="flex items-center justify-between gap-3 px-4 py-3">
                  <div>
                    <p className="text-sm font-medium text-slate-200">{network.name}</p>
                    <p className="text-xs text-slate-500">{network.description || network.id}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone="neutral">{network.routing_peers_count ?? 0} routing peers</Pill>
                    <Btn size="sm" tone="danger" onClick={() => deleteNetworkMutation.mutate(network.id)} disabled={deleteNetworkMutation.isPending}>
                      <Trash2 size={12} />
                    </Btn>
                  </div>
                </div>
              ))}
              {networks.length === 0 ? <EmptyState icon={Globe} message="No networks configured." /> : null}
            </div>
          ))}
        </Card>
      )}

      {tab === "groups" && (
        <Card>
          <CardHeader
            title="Groups"
            icon={Users}
            action={<Btn size="sm" onClick={() => setShowCreateGroup(true)}><Plus size={12} /> Create group</Btn>}
          />
          {renderQuery(groupsQuery, "Loading groups…", "Failed to load NetBird groups.", (
            <div className="divide-y divide-white/[0.04]">
              {groups.map((group) => (
                <div key={group.id} className="flex items-center justify-between gap-3 px-4 py-3">
                  <div className="flex items-center gap-2">
                    <Tags size={14} className="text-slate-500" />
                    <p className="text-sm font-medium text-slate-200">{group.name}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone="neutral">{group.peers_count ?? group.peers?.length ?? 0} peers</Pill>
                    <Btn size="sm" tone="danger" onClick={() => deleteGroupMutation.mutate(group.id)} disabled={deleteGroupMutation.isPending}>
                      <Trash2 size={12} />
                    </Btn>
                  </div>
                </div>
              ))}
              {groups.length === 0 ? <EmptyState icon={Users} message="No groups configured." /> : null}
            </div>
          ))}
        </Card>
      )}

      {tab === "routes" && (
        <Card>
          <CardHeader
            title="Routes"
            icon={Route}
            action={<Btn size="sm" onClick={() => setShowCreateRoute(true)}><Plus size={12} /> Create route</Btn>}
          />
          {renderQuery(routesQuery, "Loading routes…", "Failed to load NetBird routes.", (
            <div className="divide-y divide-white/[0.04]">
              {routes.map((route) => (
                <div key={route.id} className="flex items-center justify-between gap-3 px-4 py-3">
                  <div>
                    <p className="text-sm font-medium text-slate-200">{route.network || (route.domains ?? []).join(", ") || route.description || route.id}</p>
                    <p className="text-xs text-slate-500">
                      {route.description ? `${route.description} — ` : ""}{route.peer ? `via peer ${route.peer}` : `metric ${route.metric ?? 0}`}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone={route.enabled ? "green" : "neutral"}>{route.enabled ? "Enabled" : "Disabled"}</Pill>
                    <Btn size="sm" tone="danger" onClick={() => deleteRouteMutation.mutate(route.id)} disabled={deleteRouteMutation.isPending}>
                      <Trash2 size={12} />
                    </Btn>
                  </div>
                </div>
              ))}
              {routes.length === 0 ? <EmptyState icon={Route} message="No routes published." /> : null}
            </div>
          ))}
        </Card>
      )}

      {tab === "acls" && (
        <Card>
          <CardHeader
            title="Access-control policies"
            icon={Shield}
            action={<Btn size="sm" onClick={() => setShowCreateACL(true)}><Plus size={12} /> Create ACL</Btn>}
          />
          {renderQuery(aclsQuery, "Loading policies…", "Failed to load NetBird ACLs.", (
            <div className="divide-y divide-white/[0.04]">
              {acls.map((acl) => (
                <div key={acl.id} className="flex items-center justify-between gap-3 px-4 py-3">
                  <div>
                    <p className="text-sm font-medium text-slate-200">{acl.name || acl.id}</p>
                    <p className="text-xs text-slate-500">{acl.description || `${acl.rules?.length ?? 0} rule(s)`}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Pill tone={acl.enabled ? "green" : "neutral"}>{acl.enabled ? "Enabled" : "Disabled"}</Pill>
                    <Btn size="sm" tone="danger" onClick={() => { if (acl.id) deleteACLMutation.mutate(acl.id); }} disabled={deleteACLMutation.isPending || !acl.id}>
                      <Trash2 size={12} />
                    </Btn>
                  </div>
                </div>
              ))}
              {acls.length === 0 ? <EmptyState icon={Shield} message="No ACL policies defined." /> : null}
            </div>
          ))}
        </Card>
      )}

      {tab === "dns" && (
        <Card>
          <CardHeader title="DNS settings" icon={FileText} />
          {renderQuery(dnsQuery, "Loading DNS settings…", "Failed to load NetBird DNS settings.", (
            dnsQuery.data === null ? (
              <EmptyState icon={FileText} title="DNS unavailable" message="NetBird is not configured (NETBIRD_API_URL / NETBIRD_API_TOKEN are unset), so DNS settings cannot be read." />
            ) : (
            <div className="space-y-4 p-1">
              <Textarea
                label="Groups with DNS management disabled (comma-separated group IDs)"
                value={dnsDraft}
                onChange={setDnsGroups}
                rows={3}
                placeholder="group-id-1, group-id-2"
              />
              <p className="text-xs text-slate-500">
                Available groups: {groups.length ? groups.map((g) => `${g.name} (${g.id})`).join(", ") : "none — create a group first."}
              </p>
              <Btn onClick={() => saveDnsMutation.mutate()} disabled={saveDnsMutation.isPending}>
                {saveDnsMutation.isPending ? "Saving…" : "Save DNS settings"}
              </Btn>
            </div>
            )
          ))}
        </Card>
      )}

      {tab === "setup-keys" && (
        <Card>
          <CardHeader title="Setup keys" icon={KeyRound} />
          <div className="space-y-5 p-1">
            <p className="text-xs text-slate-500">
              Setup keys enroll new clients onto the mesh. The secret is only shown once — create a key below and copy it immediately.
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <Input label="Name" value={setupKeyForm.name} onChange={(v) => setSetupKeyForm({ ...setupKeyForm, name: v })} placeholder="Forge nodes" />
              <Input label="Description" value={setupKeyForm.description} onChange={(v) => setSetupKeyForm({ ...setupKeyForm, description: v })} placeholder="Optional" />
              <AdminSelect
                label="Type"
                value={setupKeyForm.type}
                onChange={(v) => setSetupKeyForm({ ...setupKeyForm, type: v })}
                options={[{ value: "reusable", label: "Reusable" }, { value: "one-off", label: "One-off" }]}
              />
              <Input label="Expires in (days)" type="number" value={setupKeyForm.expiresInDays} onChange={(v) => setSetupKeyForm({ ...setupKeyForm, expiresInDays: v })} />
            </div>
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
              <input
                type="checkbox"
                checked={setupKeyForm.ephemeral}
                onChange={(e) => setSetupKeyForm({ ...setupKeyForm, ephemeral: e.target.checked })}
                className="rounded border-white/10 bg-[var(--surface-input)]"
              />
              Ephemeral peers
            </label>
            <Btn onClick={() => createSetupKeyMutation.mutate()} disabled={createSetupKeyMutation.isPending || !setupKeyForm.name.trim()}>
              <Plus size={14} /> {createSetupKeyMutation.isPending ? "Creating…" : "Create setup key"}
            </Btn>

            {createdKey?.key ? (
              <div className="flex flex-wrap items-center gap-3 rounded-xl border border-emerald-500/25 bg-emerald-950/20 p-4">
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-semibold uppercase tracking-wider text-emerald-300">{createdKey.name}</p>
                  <p className="truncate font-mono text-sm text-emerald-100" title={createdKey.key}>{createdKey.key}</p>
                </div>
                <Btn size="sm" tone="ghost" onClick={() => void copyKey()}>
                  <Copy size={12} /> Copy
                </Btn>
              </div>
            ) : null}
          </div>
        </Card>
      )}

      {showCreateNetwork && (
        <Modal title="Create network" onClose={() => setShowCreateNetwork(false)}>
          <div className="space-y-4">
            <Input label="Name" value={networkForm.name} onChange={(v) => setNetworkForm({ ...networkForm, name: v })} placeholder="lab-network" required />
            <Input label="Description" value={networkForm.description} onChange={(v) => setNetworkForm({ ...networkForm, description: v })} placeholder="Optional" />
          </div>
          <ModalFooter
            onCancel={() => setShowCreateNetwork(false)}
            onConfirm={() => createNetworkMutation.mutate()}
            confirmLabel={createNetworkMutation.isPending ? "Creating..." : "Create"}
            disabled={createNetworkMutation.isPending || !networkForm.name.trim()}
          />
        </Modal>
      )}

      {showCreateGroup && (
        <Modal title="Create group" onClose={() => setShowCreateGroup(false)}>
          <div className="space-y-4">
            <Input label="Name" value={groupForm.name} onChange={(v) => setGroupForm({ name: v })} placeholder="all-servers" required />
          </div>
          <ModalFooter
            onCancel={() => setShowCreateGroup(false)}
            onConfirm={() => createGroupMutation.mutate()}
            confirmLabel={createGroupMutation.isPending ? "Creating..." : "Create"}
            disabled={createGroupMutation.isPending || !groupForm.name.trim()}
          />
        </Modal>
      )}

      {showCreateRoute && (
        <Modal title="Create route" onClose={() => setShowCreateRoute(false)}>
          <div className="space-y-4">
            <Input label="Description" value={routeForm.description} onChange={(v) => setRouteForm({ ...routeForm, description: v })} placeholder="Office LAN" required />
            <Input label="Network (CIDR)" value={routeForm.network} onChange={(v) => setRouteForm({ ...routeForm, network: v })} placeholder="192.168.10.0/24" mono />
            <AdminSelect label="Routing peer" value={routeForm.peer} onChange={(v) => setRouteForm({ ...routeForm, peer: v })} options={peerOptions} placeholder="Select a peer…" />
            <Input label="Metric" type="number" value={routeForm.metric} onChange={(v) => setRouteForm({ ...routeForm, metric: v })} />
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
              <input type="checkbox" checked={routeForm.masquerade} onChange={(e) => setRouteForm({ ...routeForm, masquerade: e.target.checked })} className="rounded border-white/10 bg-[var(--surface-input)]" />
              Masquerade
            </label>
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
              <input type="checkbox" checked={routeForm.enabled} onChange={(e) => setRouteForm({ ...routeForm, enabled: e.target.checked })} className="rounded border-white/10 bg-[var(--surface-input)]" />
              Enabled
            </label>
          </div>
          <ModalFooter
            onCancel={() => setShowCreateRoute(false)}
            onConfirm={() => createRouteMutation.mutate()}
            confirmLabel={createRouteMutation.isPending ? "Creating..." : "Create"}
            disabled={createRouteMutation.isPending || !routeForm.description.trim() || !routeForm.peer}
          />
        </Modal>
      )}

      {showCreateACL && (
        <Modal title="Create ACL" onClose={() => setShowCreateACL(false)}>
          <div className="space-y-4">
            <Input label="Name" value={aclForm.name} onChange={(v) => setAclForm({ ...aclForm, name: v })} placeholder="allow-ssh-to-servers" required />
            <Input label="Description" value={aclForm.description} onChange={(v) => setAclForm({ ...aclForm, description: v })} placeholder="Optional" />
            <AdminSelect label="Source group" value={aclForm.sourceGroup} onChange={(v) => setAclForm({ ...aclForm, sourceGroup: v })} options={groupOptions} placeholder="Select source group…" />
            <AdminSelect label="Destination group" value={aclForm.destinationGroup} onChange={(v) => setAclForm({ ...aclForm, destinationGroup: v })} options={groupOptions} placeholder="Select destination group…" />
            <AdminSelect
              label="Protocol"
              value={aclForm.protocol}
              onChange={(v) => setAclForm({ ...aclForm, protocol: v })}
              options={[{ value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }, { value: "icmp", label: "ICMP" }, { value: "all", label: "All" }]}
            />
            <Input label="Ports (comma separated, optional)" value={aclForm.port} onChange={(v) => setAclForm({ ...aclForm, port: v })} placeholder="22, 80, 443" mono />
          </div>
          <ModalFooter
            onCancel={() => setShowCreateACL(false)}
            onConfirm={() => createACLMutation.mutate()}
            confirmLabel={createACLMutation.isPending ? "Creating..." : "Create"}
            disabled={createACLMutation.isPending || !aclForm.name.trim() || !aclForm.sourceGroup || !aclForm.destinationGroup}
          />
        </Modal>
      )}
    </AdminPageLayout>
  );
}
