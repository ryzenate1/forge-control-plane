"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Network, Plus, Shield, Trash2 } from "lucide-react";
import {
  fetchNodes,
} from "@/lib/api";
import {
  type AddForwardInput,
  type AddRuleInput,
  type FirewallRule,
  type PortForward,
  type UpdateRuleInput,
  addFirewallRule,
  addPortForward,
  deleteFirewallRule,
  deletePortForward,
  disableFirewall,
  enableFirewall,
  fetchFirewallRules,
  fetchFirewallStatus,
  fetchPortForwards,
  openFirewallPort,
  updateFirewallRule,
} from "@/lib/api/firewall";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminFormSection, AdminSelect, AdminTabs, AdminPageLayout, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd, AdminLoadingState, AdminErrorState, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "./admin-ui";

type FirewallTab = "rules" | "forwards";

export function AdminFirewall() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const [nodeId, setNodeId] = useState("");
  // Require an explicit node selection: falling back to nodes[0] silently
  // targets a host the operator did not choose.
  const activeNodeId = nodeId;

  const statusQuery = useQuery({
    queryKey: ["firewall-status", activeNodeId],
    queryFn: () => fetchFirewallStatus(activeNodeId),
    enabled: !!activeNodeId,
  });
  const status = statusQuery.data;
  const statusLoading = statusQuery.isLoading;
  const statusError = statusQuery.isError;

  const rulesQuery = useQuery({
    queryKey: ["firewall-rules", activeNodeId],
    queryFn: () => fetchFirewallRules(activeNodeId),
    enabled: !!activeNodeId,
  });
  const rules = useMemo(() => rulesQuery.data ?? [], [rulesQuery.data]);

  const forwardsQuery = useQuery({
    queryKey: ["firewall-forwards", activeNodeId],
    queryFn: () => fetchPortForwards(activeNodeId),
    enabled: !!activeNodeId,
  });
  const forwards = useMemo(() => forwardsQuery.data ?? [], [forwardsQuery.data]);

  const enableMut = useMutation({
    mutationFn: () => enableFirewall(activeNodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-status", activeNodeId] });
      toast({ tone: "success", title: "Firewall enabled" });
    },
    onError: (error) => toast({ tone: "error", title: "Enable failed", message: error instanceof Error ? error.message : "Could not enable firewall" }),
  });

  const disableMut = useMutation({
    mutationFn: () => disableFirewall(activeNodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-status", activeNodeId] });
      toast({ tone: "success", title: "Firewall disabled" });
    },
    onError: (error) => toast({ tone: "error", title: "Disable failed", message: error instanceof Error ? error.message : "Could not disable firewall" }),
  });

  const [tab, setTab] = useState<FirewallTab>("rules");
  const [showAddRule, setShowAddRule] = useState(false);
  const [editingRule, setEditingRule] = useState<FirewallRule | null>(null);
  const [showAddForward, setShowAddForward] = useState(false);
  const [showQuickOpen, setShowQuickOpen] = useState(false);

  const selectedNode = nodes.find((n) => n.id === activeNodeId);

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Firewall"
        sub="Firewall rules and port forwarding."
        action={
          <div className="flex gap-2">
            <Btn size="sm" tone="ghost" disabled={!activeNodeId} onClick={() => setShowQuickOpen(true)} className="border border-[color-mix(in_srgb,var(--brand)_20%,transparent)] hover:bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]">
              <Network size={12} /> Quick Open Port
            </Btn>
            <Btn size="sm" tone="primary" disabled={!activeNodeId} onClick={() => (tab === "forwards" ? setShowAddForward(true) : setShowAddRule(true))} className="bg-[var(--brand)] hover:bg-[color-mix(in_srgb,var(--brand)_90%,transparent)] text-white">
              <Plus size={14} /> {tab === "forwards" ? "Add Forward" : "Add Rule"}
            </Btn>
          </div>
        }
      />

      <div className="flex flex-wrap items-center gap-4">
        <AdminSelect label="Node" value={nodeId} onChange={setNodeId} placeholder="Select a node\u2026" options={Array.isArray(nodes) ? nodes.map((n) => ({ value: n.id, label: n.name })) : []} />
        {!activeNodeId ? (
          <Pill tone="neutral">Select a node to manage</Pill>
        ) : statusLoading ? (
          <Pill tone="neutral">Loading status…</Pill>
        ) : statusError ? (
          <Pill tone="red">Status error</Pill>
        ) : status ? (
          <div className="flex items-center gap-2">
            {status.enabled ? (
              <Pill tone="green">Firewall Enabled</Pill>
            ) : (
              <Pill tone="red">Firewall Disabled</Pill>
            )}
            <Btn
              size="sm"
              tone={status.enabled ? "danger" : "success"}
              disabled={enableMut.isPending || disableMut.isPending}
              onClick={() => status.enabled ? disableMut.mutate() : enableMut.mutate()}
            >
              {status.enabled
                ? disableMut.isPending ? "Disabling…" : "Disable"
                : enableMut.isPending ? "Enabling…" : "Enable"
              }
            </Btn>
          </div>
        ) : null}
        {selectedNode && (
          <a
            className="flex items-center gap-1 text-xs text-slate-400 hover:text-slate-200 underline ml-auto"
            href={`/admin/nodes`}
          >
            <ExternalLink size={12} />
            {selectedNode.name}
          </a>
        )}
      </div>

      {!activeNodeId ? (
        <EmptyState icon={Shield} title="No node selected" message="Select a node above to view firewall rules and port forwards." />
      ) : (
      <>
      <AdminTabs tabs={[{ id: "rules", label: "Firewall Rules" }, { id: "forwards", label: "Port Forwards" }]} active={tab} onChange={(id) => setTab(id as FirewallTab)} />

      {tab === "rules" && (
        <div className="space-y-4">
          <Card>
            <CardHeader
              title="Rules"
              icon={Shield}
            />
            {rulesQuery.isLoading ? (
              <div className="p-4"><AdminLoadingState label="Loading rules\u2026" /></div>
            ) : rulesQuery.isError ? (
              <div className="p-4">
                <AdminErrorState message={rulesQuery.error instanceof Error ? rulesQuery.error.message : "Could not load rules"} retry={() => void rulesQuery.refetch()} />
              </div>
            ) : !Array.isArray(rules) || rules.length === 0 ? (
              <EmptyState icon={Shield} title="No firewall rules" message="No firewall rules on this node." />
            ) : (
              <AdminTable label="Firewall rules">
                <AdminTHead><AdminTh>Port</AdminTh><AdminTh>Protocol</AdminTh><AdminTh>Source</AdminTh><AdminTh>Action</AdminTh><AdminTh>Description</AdminTh><AdminTh></AdminTh></AdminTHead>
                <AdminTBody>
                    {Array.isArray(rules) && rules.map((rule) => (
                      <RuleRow
                        key={rule.id}
                        rule={rule}
                        nodeId={activeNodeId}
                        onEdit={() => setEditingRule(rule)}
                      />
                    ))}
                </AdminTBody>
              </AdminTable>
            )}
          </Card>
        </div>
      )}

      {tab === "forwards" && (
        <div className="space-y-4">
          <Card>
            <CardHeader
              title="Port Forwards"
              icon={Network}
            />
            {forwardsQuery.isLoading ? (
              <div className="p-4"><AdminLoadingState label="Loading forwards\u2026" /></div>
            ) : forwardsQuery.isError ? (
              <div className="p-4">
                <AdminErrorState message={forwardsQuery.error instanceof Error ? forwardsQuery.error.message : "Could not load forwards"} retry={() => void forwardsQuery.refetch()} />
              </div>
            ) : !Array.isArray(forwards) || forwards.length === 0 ? (
              <EmptyState icon={Network} title="No port forwards" message="No port forwards on this node." />
            ) : (
              <AdminTable label="Port forwards">
                <AdminTHead><AdminTh>From Port</AdminTh><AdminTh>To Port</AdminTh><AdminTh>To IP</AdminTh><AdminTh>Protocol</AdminTh><AdminTh>Description</AdminTh><AdminTh></AdminTh></AdminTHead>
                <AdminTBody>
                    {Array.isArray(forwards) && forwards.map((pf) => (
                      <ForwardRow key={pf.id} forward={pf} nodeId={activeNodeId} />
                    ))}
                </AdminTBody>
              </AdminTable>
            )}
          </Card>
        </div>
      )}
      </>
      )}

      {showAddRule && activeNodeId && (
        <AddRuleModal
          nodeId={activeNodeId}
          onClose={() => setShowAddRule(false)}
        />
      )}

      {editingRule && activeNodeId && (
        <EditRuleModal
          rule={editingRule}
          nodeId={activeNodeId}
          onClose={() => setEditingRule(null)}
        />
      )}

      {showAddForward && activeNodeId && (
        <AddForwardModal
          nodeId={activeNodeId}
          onClose={() => setShowAddForward(false)}
        />
      )}

      {showQuickOpen && activeNodeId && (
        <QuickOpenPortModal
          nodeId={activeNodeId}
          onClose={() => setShowQuickOpen(false)}
        />
      )}
    </AdminPageLayout>
  );
}

function RuleRow({ rule, nodeId, onEdit }: { rule: FirewallRule; nodeId: string; onEdit: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const deleteMut = useMutation({
    mutationFn: () => deleteFirewallRule(rule.id, nodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-rules", nodeId] });
    },
    onError: (error) => toast({ tone: "error", title: "Delete failed", message: error instanceof Error ? error.message : "Could not delete rule" }),
  });

  return (
    <AdminTr>
      <AdminTd className="font-mono text-xs">{rule.port ?? "—"}</AdminTd>
      <AdminTd className="text-xs uppercase">{rule.protocol ?? "—"}</AdminTd>
      <AdminTd className="font-mono text-xs">{rule.sourceIp ?? "—"}</AdminTd>
      <AdminTd><Pill tone={rule.action === "allow" ? "green" : rule.action === "deny" ? "red" : "neutral"}>{rule.action ?? "—"}</Pill></AdminTd>
      <AdminTd className="text-xs text-slate-400">{rule.description ?? "—"}</AdminTd>
      <AdminTd>
        <div className="flex justify-end gap-2">
        <Btn size="sm" tone="ghost" onClick={onEdit}>Edit</Btn>
        <Btn size="sm" tone="danger" disabled={deleteMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Delete this firewall rule?", description: rule.description ?? `Rule for ${rule.protocol} on port ${rule.port}. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(); })(); }}>
          {deleteMut.isPending ? "…" : <Trash2 size={14} />}
        </Btn>
        </div>
        {renderConfirm()}
      </AdminTd>
    </AdminTr>
  );
}

function ForwardRow({ forward, nodeId }: { forward: PortForward; nodeId: string }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const deleteMut = useMutation({
    mutationFn: () => deletePortForward(forward.id, nodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-forwards", nodeId] });
    },
    onError: (error) => toast({ tone: "error", title: "Delete failed", message: error instanceof Error ? error.message : "Could not delete forward" }),
  });

  return (
    <AdminTr>
      <AdminTd className="font-mono text-xs">{forward.fromPort}</AdminTd>
      <AdminTd className="font-mono text-xs">{forward.toPort}</AdminTd>
      <AdminTd className="font-mono text-xs">{forward.toIp}</AdminTd>
      <AdminTd className="text-xs uppercase">{forward.protocol ?? "—"}</AdminTd>
      <AdminTd className="text-xs text-slate-400">{forward.description ?? "—"}</AdminTd>
      <AdminTd>
        <div className="flex justify-end">
        <Btn size="sm" tone="danger" disabled={deleteMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Delete this port forward?", description: `Forwarding ${forward.fromPort} → ${forward.toIp}:${forward.toPort} will be removed. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(); })(); }}>
          {deleteMut.isPending ? "…" : <Trash2 size={14} />}
        </Btn>
        </div>
        {renderConfirm()}
      </AdminTd>
    </AdminTr>
  );
}

function AddRuleModal({ nodeId, onClose }: { nodeId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [port, setPort] = useState("");
  const [protocol, setProtocol] = useState("tcp");
  const [source, setSource] = useState("");
  const [action, setAction] = useState("allow");
  const [description, setDescription] = useState("");

  const addMut = useMutation({
    mutationFn: () => addFirewallRule({
      port: port ? Number(port) : undefined,
      protocol: protocol || undefined,
      sourceIp: source || undefined,
      action: action || undefined,
      description: description || undefined,
    } as AddRuleInput, nodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-rules", nodeId] });
      onClose();
    },
    onError: (error) => toast({ tone: "error", title: "Add failed", message: error instanceof Error ? error.message : "Could not add rule" }),
  });

  return (
    <Modal title="Add Firewall Rule" onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); addMut.mutate(); }}>
        <AdminFormSection title="Rule Details">
          <Input label="Port" value={port} onChange={setPort} type="number" placeholder="e.g. 80" />
          <AdminSelect label="Protocol" value={protocol} onChange={setProtocol} options={[
            { value: "tcp", label: "TCP" },
            { value: "udp", label: "UDP" },
          ]} />
          <Input label="Source IP" value={source} onChange={setSource} placeholder="e.g. 0.0.0.0/0" />
          <AdminSelect label="Action" value={action} onChange={setAction} options={[
            { value: "allow", label: "ALLOW" },
            { value: "deny", label: "DENY" },
          ]} />
          <Input label="Description" value={description} onChange={setDescription} placeholder="Optional description" />
        </AdminFormSection>
        {addMut.error && <p className="text-sm text-red-300">{addMut.error instanceof Error ? addMut.error.message : "Failed to add rule."}</p>}
        <ModalFooter onCancel={onClose} onConfirm={() => addMut.mutate()} confirmLabel={addMut.isPending ? "Adding…" : "Add Rule"} disabled={addMut.isPending} />
      </form>
    </Modal>
  );
}

function EditRuleModal({ rule, nodeId, onClose }: { rule: FirewallRule; nodeId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [port, setPort] = useState(rule.port != null ? String(rule.port) : "");
  const [protocol, setProtocol] = useState(rule.protocol ?? "tcp");
  const [source, setSource] = useState(rule.sourceIp ?? "");
  const [action, setAction] = useState(rule.action ?? "allow");
  const [description, setDescription] = useState(rule.description ?? "");

  const updateMut = useMutation({
    mutationFn: () => updateFirewallRule(rule.id, {
      port: port ? Number(port) : undefined,
      protocol: protocol || undefined,
      sourceIp: source || undefined,
      action: action || undefined,
      description: description || undefined,
    } as UpdateRuleInput, nodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-rules", nodeId] });
      onClose();
    },
    onError: (error) => toast({ tone: "error", title: "Update failed", message: error instanceof Error ? error.message : "Could not update rule" }),
  });

  return (
    <Modal title="Edit Firewall Rule" onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); updateMut.mutate(); }}>
        <AdminFormSection title="Rule Details">
          <Input label="Port" value={port} onChange={setPort} type="number" placeholder="e.g. 80" />
          <AdminSelect label="Protocol" value={protocol} onChange={setProtocol} options={[
            { value: "tcp", label: "TCP" },
            { value: "udp", label: "UDP" },
          ]} />
          <Input label="Source IP" value={source} onChange={setSource} placeholder="e.g. 0.0.0.0/0" />
          <AdminSelect label="Action" value={action} onChange={setAction} options={[
            { value: "allow", label: "ALLOW" },
            { value: "deny", label: "DENY" },
          ]} />
          <Input label="Description" value={description} onChange={setDescription} placeholder="Optional description" />
        </AdminFormSection>
        {updateMut.error && <p className="text-sm text-red-300">{updateMut.error instanceof Error ? updateMut.error.message : "Failed to update rule."}</p>}
        <ModalFooter onCancel={onClose} onConfirm={() => updateMut.mutate()} confirmLabel={updateMut.isPending ? "Saving…" : "Save Changes"} disabled={updateMut.isPending} />
      </form>
    </Modal>
  );
}

function AddForwardModal({ nodeId, onClose }: { nodeId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [fromPort, setFromPort] = useState("");
  const [toPort, setToPort] = useState("");
  const [toIp, setToIp] = useState("");
  const [protocol, setProtocol] = useState("tcp");
  const [description, setDescription] = useState("");

  const addMut = useMutation({
    mutationFn: () => addPortForward({
      fromPort: Number(fromPort),
      toPort: Number(toPort),
      toIp: toIp.trim(),
      protocol,
      description: description || undefined,
    } as AddForwardInput, nodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-forwards", nodeId] });
      onClose();
    },
    onError: (error) => toast({ tone: "error", title: "Add failed", message: error instanceof Error ? error.message : "Could not add forward" }),
  });

  const validationError = !fromPort ? "From Port is required." : !toPort ? "To Port is required." : !toIp.trim() ? "To IP is required." : null;

  return (
    <Modal title="Add Port Forward" onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!validationError) addMut.mutate(); }}>
        <AdminFormSection title="Forward Details">
          <Input label="From Port" value={fromPort} onChange={setFromPort} type="number" required />
          <Input label="To Port" value={toPort} onChange={setToPort} type="number" required />
          <Input label="To IP" value={toIp} onChange={setToIp} placeholder="e.g. 10.0.0.5" required />
          <AdminSelect label="Protocol" value={protocol} onChange={setProtocol} options={[
            { value: "tcp", label: "TCP" },
            { value: "udp", label: "UDP" },
          ]} />
          <Input label="Description" value={description} onChange={setDescription} placeholder="Optional description" />
        </AdminFormSection>
        {validationError && <p className="text-sm text-red-300">{validationError}</p>}
        {addMut.error && <p className="text-sm text-red-300">{addMut.error instanceof Error ? addMut.error.message : "Failed to add forward."}</p>}
        <ModalFooter onCancel={onClose} onConfirm={() => { if (!validationError) addMut.mutate(); }} confirmLabel={addMut.isPending ? "Adding…" : "Add Forward"} disabled={Boolean(validationError) || addMut.isPending} />
      </form>
    </Modal>
  );
}

function QuickOpenPortModal({ nodeId, onClose }: { nodeId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [port, setPort] = useState("");
  const [protocol, setProtocol] = useState("tcp");

  const openMut = useMutation({
    mutationFn: () => openFirewallPort({ port: Number(port), protocol }, nodeId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["firewall-rules", nodeId] });
      void qc.invalidateQueries({ queryKey: ["firewall-status", nodeId] });
      toast({ tone: "success", title: `Port ${port}/${protocol} opened` });
      onClose();
    },
    onError: (error) => toast({ tone: "error", title: "Open failed", message: error instanceof Error ? error.message : "Could not open port" }),
  });

  const validationError = !port ? "Port is required." : Number(port) < 1 || Number(port) > 65535 ? "Port must be 1-65535." : null;

  return (
    <Modal title="Quick Open Port" onClose={onClose}>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!validationError) openMut.mutate(); }}>
        <AdminFormSection title="Open Port">
          <Input label="Port" value={port} onChange={setPort} type="number" placeholder="e.g. 25565" required />
          <AdminSelect label="Protocol" value={protocol} onChange={setProtocol} options={[
            { value: "tcp", label: "TCP" },
            { value: "udp", label: "UDP" },
            { value: "both", label: "TCP+UDP" },
          ]} />
          <p className="text-xs text-slate-400">Opens an allow rule for the selected node directly.</p>
        </AdminFormSection>
        {validationError && <p className="text-sm text-red-300">{validationError}</p>}
        {openMut.error && <p className="text-sm text-red-300">{openMut.error instanceof Error ? openMut.error.message : "Failed to open port."}</p>}
        <ModalFooter
          onCancel={onClose}
          onConfirm={() => { if (!validationError) openMut.mutate(); }}
          confirmLabel={openMut.isPending ? "Opening…" : "Open Port"}
          disabled={Boolean(validationError) || openMut.isPending}
        />
      </form>
    </Modal>
  );
}
