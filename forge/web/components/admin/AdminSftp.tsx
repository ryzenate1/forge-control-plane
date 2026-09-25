"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderLock, RefreshCw, Save, Server, Settings, Shield } from "lucide-react";
import {
  fetchSFTPGlobalConfig,
  updateSFTPGlobalConfig,
  fetchSFTPNodeConfigs,
  fetchSFTPNodeConfig,
  updateSFTPNodeConfig,
  type SFTPGlobalConfig,
  type SFTPNodeConfig,
} from "@/lib/api/sftp";
import { fetchNodes } from "@/lib/api";
import { useT } from "@/components/TranslationProvider";
import { useToast } from "@/components/ui/toast";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageLayout,
  SectionHeader,
  Card,
  CardHeader,
  Btn,
  Pill,
  AdminLoadingState,
  AdminErrorState,
  EmptyState,
  Input,
  Modal,
} from "./admin-ui";

export function AdminSftp() {
  const t = useT();
  const { toast } = useToast();
  const qc = useQueryClient();

  const globalQ = useQuery({ queryKey: ["admin-sftp-settings"], queryFn: fetchSFTPGlobalConfig, retry: false });
  const nodesQ = useQuery({ queryKey: ["admin-sftp-nodes"], queryFn: fetchSFTPNodeConfigs, retry: false });
  const allNodesQ = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, retry: false });

  const [globalForm, setGlobalForm] = useState<SFTPGlobalConfig | null>(null);
  const [editingNodeId, setEditingNodeId] = useState<string | null>(null);

  useEffect(() => {
    if (globalQ.data && !globalForm) setGlobalForm(globalQ.data);
  }, [globalQ.data, globalForm]);

  const saveGlobalMut = useMutation({
    mutationFn: () => {
      if (!globalForm) throw new Error("No form data");
      return updateSFTPGlobalConfig(globalForm);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-sftp-settings"] });
      toast({ tone: "success", title: "Global SFTP settings saved" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  const nodeNameMap = useMemo(() => {
    const m = new Map<string, string>();
    for (const n of allNodesQ.data ?? []) m.set(n.id, n.name);
    return m;
  }, [allNodesQ.data]);

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { void globalQ.refetch(); void nodesQ.refetch(); void allNodesQ.refetch(); }} />
      <SectionHeader
        title="SFTP"
        sub="Global and per-node SFTP configuration — GET+PUT /admin/sftp/settings, GET+PUT /admin/nodes/:nodeId/sftp, GET /admin/sftp/nodes (handlers_sftp.go:9)"
        action={
          <Btn size="sm" tone="ghost" onClick={() => { void globalQ.refetch(); void nodesQ.refetch(); void allNodesQ.refetch(); }}>
            <RefreshCw size={14} /> Refresh
          </Btn>
        }
      />

      {/* Global */}
      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader
          title="Global settings — GET /admin/sftp/settings"
          icon={Settings}
          action={
            <div className="flex items-center gap-2">
              {globalForm ? <Pill tone={globalForm.enabled ? "green" : "red"}>{globalForm.enabled ? "enabled" : "disabled"}</Pill> : null}
              <Btn size="sm" tone="primary" loading={saveGlobalMut.isPending} disabled={!globalForm} onClick={() => saveGlobalMut.mutate()}>
                <Save size={12} /> Save
              </Btn>
            </div>
          }
        />
        {globalQ.isLoading ? (
          <AdminLoadingState label="Loading SFTP settings…" />
        ) : globalQ.isError ? (
          <div className="p-4"><AdminErrorState message={(globalQ.error as Error).message} retry={() => void globalQ.refetch()} /></div>
        ) : globalForm ? (
          <div className="space-y-4 p-4">
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={globalForm.enabled} onChange={(e) => setGlobalForm({ ...globalForm, enabled: e.target.checked })} className="accent-[var(--brand)]" />
              <span className="font-medium">Globally enabled</span>
            </label>

            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Input label="Default port" value={String(globalForm.defaultPort)} onChange={(v) => setGlobalForm({ ...globalForm, defaultPort: Number(v) || 0 })} type="number" mono />
              <Input label="Max connections" value={String(globalForm.defaultMaxConnections)} onChange={(v) => setGlobalForm({ ...globalForm, defaultMaxConnections: Number(v) || 0 })} type="number" mono />
              <Input label="Idle timeout (s)" value={String(globalForm.defaultIdleTimeout)} onChange={(v) => setGlobalForm({ ...globalForm, defaultIdleTimeout: Number(v) || 0 })} type="number" mono />
              <Input label="Rate limit (KB/s, 0=unlimited)" value={String(globalForm.defaultRateLimit)} onChange={(v) => setGlobalForm({ ...globalForm, defaultRateLimit: Number(v) || 0 })} type="number" mono />
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Log level</span>
                <select className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm" value={globalForm.logLevel} onChange={(e) => setGlobalForm({ ...globalForm, logLevel: e.target.value })}>
                  <option value="error">error</option>
                  <option value="warn">warn</option>
                  <option value="info">info</option>
                  <option value="debug">debug</option>
                  <option value="trace">trace</option>
                </select>
              </label>
              <div className="rounded-lg border border-[var(--line)] bg-[var(--canvas)] p-3">
                <div className="text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Crypto</div>
                <div className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">
                  Ciphers: {(globalForm.allowedCiphers ?? []).join(", ") || "default"}<br />
                  MACs: {(globalForm.allowedMACs ?? []).join(", ") || "default"}<br />
                  KEX: {(globalForm.allowedKexAlgos ?? []).join(", ") || "default"}
                </div>
              </div>
            </div>

            <div className="grid gap-3">
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Allowed ciphers (comma-separated, blank = default)</span>
                <input className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm font-mono" value={(globalForm.allowedCiphers ?? []).join(", ")} onChange={(e) => setGlobalForm({ ...globalForm, allowedCiphers: e.target.value ? e.target.value.split(",").map((s) => s.trim()).filter(Boolean) : [] })} placeholder="aes128-ctr, aes256-gcm@openssh.com" />
              </label>
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Allowed MACs</span>
                <input className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm font-mono" value={(globalForm.allowedMACs ?? []).join(", ")} onChange={(e) => setGlobalForm({ ...globalForm, allowedMACs: e.target.value ? e.target.value.split(",").map((s) => s.trim()).filter(Boolean) : [] })} placeholder="hmac-sha2-256, hmac-sha2-512" />
              </label>
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Allowed KEX</span>
                <input className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm font-mono" value={(globalForm.allowedKexAlgos ?? []).join(", ")} onChange={(e) => setGlobalForm({ ...globalForm, allowedKexAlgos: e.target.value ? e.target.value.split(",").map((s) => s.trim()).filter(Boolean) : [] })} placeholder="curve25519-sha256, diffie-hellman-group16-sha512" />
              </label>
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Host key algorithms</span>
                <input className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm font-mono" value={(globalForm.hostKeyAlgorithms ?? []).join(", ")} onChange={(e) => setGlobalForm({ ...globalForm, hostKeyAlgorithms: e.target.value ? e.target.value.split(",").map((s) => s.trim()).filter(Boolean) : [] })} placeholder="ssh-ed25519, rsa-sha2-256" />
              </label>
            </div>

            {saveGlobalMut.isError && <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200">{(saveGlobalMut.error as Error).message}</div>}
            {saveGlobalMut.isSuccess && <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-3 text-sm text-emerald-200">Saved.</div>}
          </div>
        ) : null}
      </Card>

      {/* Per-node */}
      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title="Per-node configs — GET /admin/sftp/nodes" icon={FolderLock} action={<span className="text-xs text-[var(--text-subtle)]">{nodesQ.data?.length ?? 0} nodes</span>} />
        {nodesQ.isLoading ? (
          <AdminLoadingState label="Loading node configs…" />
        ) : nodesQ.isError ? (
          <div className="p-4"><AdminErrorState message={(nodesQ.error as Error).message} retry={() => void nodesQ.refetch()} /></div>
        ) : (nodesQ.data?.length ?? 0) === 0 ? (
          <EmptyState icon={Server} title="No per-node SFTP configs" message="No overrides yet. Each node falls back to the global defaults; configure per-node to override." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--line)] bg-[var(--surface-raised)] text-left text-[10px] uppercase tracking-[0.12em] text-[var(--text-subtle)]">
                  <th className="px-4 py-3">Node</th>
                  <th className="px-4 py-3">Enabled</th>
                  <th className="px-4 py-3">Listen</th>
                  <th className="px-4 py-3">Limits</th>
                  <th className="px-4 py-3">Policy</th>
                  <th className="px-4 py-3">Updated</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {(nodesQ.data ?? []).map((cfg) => (
                  <tr key={cfg.nodeId} className="hover:bg-[var(--surface-hover)]">
                    <td className="px-4 py-3">
                      <div className="font-mono text-xs font-medium text-[var(--text)]">{nodeNameMap.get(cfg.nodeId) ?? cfg.nodeId.slice(0, 12)}</div>
                      <div className="font-mono text-[11px] text-[var(--text-subtle)]">{cfg.nodeId.slice(0, 12)}…</div>
                    </td>
                    <td className="px-4 py-3"><Pill tone={cfg.enabled ? "green" : "red"}>{cfg.enabled ? "enabled" : "disabled"}</Pill></td>
                    <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{cfg.listenIP || "0.0.0.0"}:{cfg.listenPort}</td>
                    <td className="px-4 py-3 text-xs text-[var(--text-subtle)]">
                      {cfg.maxConnections} conn · {cfg.maxAuthAttempts} auth · {cfg.idleTimeout}s · {cfg.rateLimit ? `${cfg.rateLimit} KB/s` : "unlimited"}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        {cfg.readOnly ? <Pill tone="yellow">read-only</Pill> : <Pill tone="neutral">rw</Pill>}
                        <Pill tone="neutral" className="font-mono text-[11px]">{cfg.logLevel}</Pill>
                      </div>
                      {cfg.banner && <div className="mt-1 max-w-[16rem] truncate text-[11px] text-[var(--text-subtle)]" title={cfg.banner}>{cfg.banner}</div>}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{cfg.updatedAt ? new Date(cfg.updatedAt).toLocaleString() : "—"}</td>
                    <td className="px-4 py-3 text-right">
                      <Btn size="sm" tone="ghost" onClick={() => setEditingNodeId(cfg.nodeId)}>
                        <Shield size={12} /> Edit
                      </Btn>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <div className="border-t border-[var(--line)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
          Per-node: <code className="rounded bg-white/[0.06] px-1 font-mono text-[11px]">GET /admin/nodes/:nodeId/sftp</code> ·{" "}
          <code className="font-mono text-[11px]">PUT /admin/nodes/:nodeId/sftp</code> — unconfigured nodes return a sensible default (enabled, :2022, 10 conns) and are persisted on first save.
        </div>
      </Card>

      {/* Also show unconfigured known nodes that have no override row yet */}
      {allNodesQ.data && nodesQ.data ? (
        <UnconfiguredNodesCard
          allNodeIds={allNodesQ.data.map((n) => n.id)}
          configuredIds={new Set((nodesQ.data ?? []).map((c) => c.nodeId))}
          nameMap={nodeNameMap}
          onEdit={setEditingNodeId}
        />
      ) : null}

      {editingNodeId && (
        <SftpNodeEditor nodeId={editingNodeId} nodeName={nodeNameMap.get(editingNodeId) ?? editingNodeId.slice(0, 12)} onClose={() => setEditingNodeId(null)} />
      )}
    </AdminPageLayout>
  );
}

function UnconfiguredNodesCard({
  allNodeIds,
  configuredIds,
  nameMap,
  onEdit,
}: {
  allNodeIds: string[];
  configuredIds: Set<string>;
  nameMap: Map<string, string>;
  onEdit: (id: string) => void;
}) {
  const missing = allNodeIds.filter((id) => !configuredIds.has(id));
  if (missing.length === 0) return null;
  return (
    <Card className="border border-[var(--line)] bg-[var(--surface)]">
      <CardHeader title={`Nodes without override — ${missing.length} using global defaults`} icon={Server} />
      <div className="flex flex-wrap gap-2 p-4">
        {missing.map((id) => (
          <span key={id} className="inline-flex items-center gap-2 rounded-full border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-1 text-xs">
            <span className="font-mono text-[var(--text)]">{nameMap.get(id) ?? id.slice(0, 12)}</span>
            <Btn size="sm" tone="ghost" onClick={() => onEdit(id)}>Configure</Btn>
          </span>
        ))}
      </div>
    </Card>
  );
}

function SftpNodeEditor({ nodeId, nodeName, onClose }: { nodeId: string; nodeName: string; onClose: () => void }) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["admin-sftp-node", nodeId], queryFn: () => fetchSFTPNodeConfig(nodeId), retry: false });
  const [form, setForm] = useState<SFTPNodeConfig | null>(null);

  useEffect(() => { if (q.data && !form) setForm(q.data); }, [q.data, form]);

  const saveMut = useMutation({
    mutationFn: () => {
      if (!form) throw new Error("No form");
      return updateSFTPNodeConfig(nodeId, form);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-sftp-nodes"] });
      void qc.invalidateQueries({ queryKey: ["admin-sftp-node", nodeId] });
      toast({ tone: "success", title: `SFTP config saved for ${nodeName}` });
      onClose();
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  return (
    <Modal title={`SFTP — ${nodeName}`} onClose={onClose} wide>
      {q.isLoading ? <AdminLoadingState label="Loading node config…" /> : q.isError ? <AdminErrorState message={(q.error as Error).message} retry={() => void q.refetch()} /> : form ? (
        <div className="space-y-4">
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} className="accent-[var(--brand)]" />
            <span className="font-medium">Enabled on this node</span>
          </label>

          <div className="grid gap-3 sm:grid-cols-2">
            <Input label="Listen IP" value={form.listenIP} onChange={(v) => setForm({ ...form, listenIP: v })} placeholder="0.0.0.0" mono />
            <Input label="Listen port" value={String(form.listenPort)} onChange={(v) => setForm({ ...form, listenPort: Number(v) || 0 })} type="number" mono />
            <Input label="Max connections" value={String(form.maxConnections)} onChange={(v) => setForm({ ...form, maxConnections: Number(v) || 0 })} type="number" mono />
            <Input label="Max auth attempts" value={String(form.maxAuthAttempts)} onChange={(v) => setForm({ ...form, maxAuthAttempts: Number(v) || 0 })} type="number" mono />
            <Input label="Idle timeout (s)" value={String(form.idleTimeout)} onChange={(v) => setForm({ ...form, idleTimeout: Number(v) || 0 })} type="number" mono />
            <Input label="Rate limit (KB/s)" value={String(form.rateLimit)} onChange={(v) => setForm({ ...form, rateLimit: Number(v) || 0 })} type="number" mono />
          </div>

          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={form.readOnly} onChange={(e) => setForm({ ...form, readOnly: e.target.checked })} className="accent-[var(--brand)]" />
            <span>Read-only</span>
          </label>

          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Log level</span>
            <select className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm" value={form.logLevel} onChange={(e) => setForm({ ...form, logLevel: e.target.value })}>
              <option value="error">error</option>
              <option value="warn">warn</option>
              <option value="info">info</option>
              <option value="debug">debug</option>
            </select>
          </label>

          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Allowed IPs (comma-separated, blank = all)</span>
            <input className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm font-mono" value={(form.allowedIps ?? []).join(", ")} onChange={(e) => setForm({ ...form, allowedIps: e.target.value ? e.target.value.split(",").map((s) => s.trim()).filter(Boolean) : [] })} placeholder="10.0.0.0/8, 192.168.1.10" />
          </label>

          <label className="block text-sm">
            <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Banner</span>
            <textarea className="min-h-[60px] w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] p-3 text-sm" value={form.banner ?? ""} onChange={(e) => setForm({ ...form, banner: e.target.value })} placeholder="Welcome to Forge SFTP" rows={2} />
          </label>

          {saveMut.isError && <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200">{(saveMut.error as Error).message}</div>}

          <div className="flex justify-end gap-2 border-t border-[var(--line)] pt-4">
            <Btn tone="ghost" onClick={onClose}>Cancel</Btn>
            <Btn tone="primary" loading={saveMut.isPending} onClick={() => saveMut.mutate()}>
              <Save size={14} /> Save node config
            </Btn>
          </div>
        </div>
      ) : null}
    </Modal>
  );
}
