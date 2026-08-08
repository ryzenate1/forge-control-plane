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
  updateFirewallRule,
} from "@/lib/api/firewall";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminFormSection, AdminSelect, AdminTabs, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, cn } from "./admin-ui";

type FirewallTab = "rules" | "forwards";

export function AdminFirewall() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const [nodeId, setNodeId] = useState("");
  const activeNodeId = nodeId || (nodes.length > 0 ? nodes[0].id : "");

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

  const selectedNode = nodes.find((n) => n.id === activeNodeId);

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Firewall"
        sub="Manage firewall rules and port forwards on host nodes."
      />

      <div className="flex flex-wrap items-center gap-4">
        <AdminSelect label="Node" value={nodeId} onChange={setNodeId} placeholder="Auto-select first node" options={Array.isArray(nodes) ? nodes.map((n) => ({ value: n.id, label: n.name })) : []} />
        {statusLoading ? (
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

      <AdminTabs tabs={[{ id: "rules", label: "Firewall Rules" }, { id: "forwards", label: "Port Forwards" }]} active={tab} onChange={(id) => setTab(id as FirewallTab)} />

      {tab === "rules" && (
        <div className="space-y-4">
          <Card>
            <CardHeader
              title="Rules"
              icon={Shield}
              action={
                <Btn size="sm" tone="primary" onClick={() => setShowAddRule(true)}>
                  <Plus size={14} /> Add Rule
                </Btn>
              }
            />
            {rulesQuery.isLoading ? (
              <div className="p-8 text-center text-sm text-slate-500">Loading rules…</div>
            ) : rulesQuery.isError ? (
              <div className="p-4">
                <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                  Could not load rules: {rulesQuery.error instanceof Error ? rulesQuery.error.message : "Unknown error"}
                </div>
              </div>
            ) : !Array.isArray(rules) || rules.length === 0 ? (
              <EmptyState icon={Shield} message="No firewall rules." />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                      <th className="px-4 py-3">Port</th>
                      <th className="px-4 py-3">Protocol</th>
                      <th className="px-4 py-3">Source</th>
                      <th className="px-4 py-3">Action</th>
                      <th className="px-4 py-3">Description</th>
                      <th className="px-4 py-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Array.isArray(rules) && rules.map((rule) => (
                      <RuleRow
                        key={rule.id}
                        rule={rule}
                        nodeId={activeNodeId}
                        onEdit={() => setEditingRule(rule)}
                      />
                    ))}
                  </tbody>
                </table>
              </div>
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
              action={
                <Btn size="sm" tone="primary" onClick={() => setShowAddForward(true)}>
                  <Plus size={14} /> Add Forward
                </Btn>
              }
            />
            {forwardsQuery.isLoading ? (
              <div className="p-8 text-center text-sm text-slate-500">Loading forwards…</div>
            ) : forwardsQuery.isError ? (
              <div className="p-4">
                <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                  Could not load forwards: {forwardsQuery.error instanceof Error ? forwardsQuery.error.message : "Unknown error"}
                </div>
              </div>
            ) : !Array.isArray(forwards) || forwards.length === 0 ? (
              <EmptyState icon={Network} message="No port forwards." />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
                      <th className="px-4 py-3">From Port</th>
                      <th className="px-4 py-3">To Port</th>
                      <th className="px-4 py-3">To IP</th>
                      <th className="px-4 py-3">Protocol</th>
                      <th className="px-4 py-3">Description</th>
                      <th className="px-4 py-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Array.isArray(forwards) && forwards.map((pf) => (
                      <ForwardRow key={pf.id} forward={pf} nodeId={activeNodeId} />
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </div>
      )}

      {showAddRule && (
        <AddRuleModal
          nodeId={activeNodeId}
          onClose={() => setShowAddRule(false)}
        />
      )}

      {editingRule && (
        <EditRuleModal
          rule={editingRule}
          nodeId={activeNodeId}
          onClose={() => setEditingRule(null)}
        />
      )}

      {showAddForward && (
        <AddForwardModal
          nodeId={activeNodeId}
          onClose={() => setShowAddForward(false)}
        />
      )}
    </div>
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

  const actionColor = rule.action === "allow"
    ? "text-emerald-400"
    : rule.action === "deny"
      ? "text-red-400"
      : "text-slate-300";

  return (
    <tr className="border-b border-white/[0.04] transition hover:bg-white/[0.02]">
      <td className="px-4 py-3 font-mono text-xs">{rule.port ?? "—"}</td>
      <td className="px-4 py-3 text-xs uppercase">{rule.protocol ?? "—"}</td>
      <td className="px-4 py-3 font-mono text-xs">{rule.sourceIp ?? "—"}</td>
      <td className={cn("px-4 py-3 text-xs font-semibold", actionColor)}>{rule.action ?? "—"}</td>
      <td className="px-4 py-3 text-xs text-slate-400">{rule.description ?? "—"}</td>
      <td className="px-4 py-3 text-right space-x-2">
        <Btn size="sm" tone="ghost" onClick={onEdit}>Edit</Btn>
        <Btn size="sm" tone="danger" disabled={deleteMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Delete this firewall rule?", description: rule.description ?? `Rule for ${rule.protocol} on port ${rule.port}. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(); })(); }}>
          {deleteMut.isPending ? "…" : <Trash2 size={14} />}
        </Btn>
        {renderConfirm()}
      </td>
    </tr>
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
    <tr className="border-b border-white/[0.04] transition hover:bg-white/[0.02]">
      <td className="px-4 py-3 font-mono text-xs">{forward.fromPort}</td>
      <td className="px-4 py-3 font-mono text-xs">{forward.toPort}</td>
      <td className="px-4 py-3 font-mono text-xs">{forward.toIp}</td>
      <td className="px-4 py-3 text-xs uppercase">{forward.protocol ?? "—"}</td>
      <td className="px-4 py-3 text-xs text-slate-400">{forward.description ?? "—"}</td>
      <td className="px-4 py-3 text-right">
        <Btn size="sm" tone="danger" disabled={deleteMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Delete this port forward?", description: `Forwarding ${forward.fromPort} → ${forward.toIp}:${forward.toPort} will be removed. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(); })(); }}>
          {deleteMut.isPending ? "…" : <Trash2 size={14} />}
        </Btn>
        {renderConfirm()}
      </td>
    </tr>
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
        {validationError && <p className="text-sm text-amber-300">{validationError}</p>}
        {addMut.error && <p className="text-sm text-red-300">{addMut.error instanceof Error ? addMut.error.message : "Failed to add forward."}</p>}
        <ModalFooter onCancel={onClose} onConfirm={() => { if (!validationError) addMut.mutate(); }} confirmLabel={addMut.isPending ? "Adding…" : "Add Forward"} disabled={Boolean(validationError) || addMut.isPending} />
      </form>
    </Modal>
  );
}
