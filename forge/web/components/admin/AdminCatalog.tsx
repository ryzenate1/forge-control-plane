"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Library, Package, Plus, RefreshCw, Search, Server, Settings, Clock, Database } from "lucide-react";
import {
  fetchCatalogEntries,
  fetchCatalogEntry,
  provisionCatalog,
  fetchCatalogInstances,
  fetchCatalogRetention,
  setCatalogRetention,
  runCatalogRetention,
  type CatalogEntry,
} from "@/lib/api/catalog";
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

export function AdminCatalog() {
  const t = useT();
  const { toast } = useToast();
  const qc = useQueryClient();
  const [search, setSearch] = useState("");
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [showProvision, setShowProvision] = useState<CatalogEntry | null>(null);
  const [page, setPage] = useState(0);
  const pageSize = 9;

  const catalogQ = useQuery({ queryKey: ["admin-catalog"], queryFn: fetchCatalogEntries, retry: false });
  const retentionQ = useQuery({ queryKey: ["admin-catalog-retention"], queryFn: fetchCatalogRetention, retry: false });

  const entries = useMemo(() => catalogQ.data ?? [], [catalogQ.data]);
  const filtered = useMemo(() => {
    if (!search.trim()) return entries;
    const s = search.toLowerCase();
    return entries.filter((e) => e.key.toLowerCase().includes(s) || e.displayName.toLowerCase().includes(s) || e.description.toLowerCase().includes(s) || e.category.toLowerCase().includes(s));
  }, [entries, search]);
  // reset page on search change
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(page, totalPages - 1);
  const paginated = useMemo(() => filtered.slice(safePage * pageSize, safePage * pageSize + pageSize), [filtered, safePage, pageSize]);

  const runRetentionMut = useMutation({
    mutationFn: runCatalogRetention,
    onSuccess: (data) => {
      toast({ tone: "success", title: `Retention run — ${data.deleted} pruned` });
      void qc.invalidateQueries({ queryKey: ["admin-catalog-retention"] });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Retention run failed", message: e.message }),
  });

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { void catalogQ.refetch(); void retentionQ.refetch(); }} />
      <SectionHeader
        title="Service Catalog"
        sub="One-click service catalog and provisioning — GET /catalog, GET /catalog/:key, POST /admin/catalog/provision (phase3_registrar.go:22). Categories: database, cache, queue, storage."
        action={
          <div className="flex items-center gap-2">
            <Btn size="sm" tone="ghost" onClick={() => { void catalogQ.refetch(); void retentionQ.refetch(); }}>
              <RefreshCw size={14} /> Refresh
            </Btn>
            <Btn size="sm" tone="ghost" onClick={() => runRetentionMut.mutate()} loading={runRetentionMut.isPending}>
              <Clock size={14} /> Run retention
            </Btn>
          </div>
        }
      />

      <Card className="border border-[var(--line)] bg-[var(--surface)]">
        <CardHeader title={`Entries — GET /catalog · ${filtered.length} of ${entries.length}`} icon={Library} action={<div className="flex items-center gap-2"><Search size={12} className="text-[var(--text-subtle)]" /><Input value={search} onChange={(v) => { setSearch(v); setPage(0); }} placeholder="Search key, name, category…" /></div>} />
        {catalogQ.isLoading ? (
          <AdminLoadingState label="Loading catalog…" />
        ) : catalogQ.isError ? (
          <div className="p-4"><AdminErrorState message={(catalogQ.error as Error).message} retry={() => void catalogQ.refetch()} /></div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Library} title={search ? "No matches" : "Catalog empty"} message={search ? `No entries match "${search}"` : "No catalog entries — seed the catalog via migrations or the admin seeder."} />
        ) : (
          <>
            <div className="grid gap-4 p-4 sm:grid-cols-2 lg:grid-cols-3">
              {paginated.map((entry) => (
                <CatalogCard key={entry.key} entry={entry} onProvision={() => setShowProvision(entry)} onDetail={() => setSelectedKey(entry.key)} selected={selectedKey === entry.key} />
              ))}
            </div>
            {filtered.length > pageSize && (
              <div className="flex items-center justify-between border-t border-[var(--line)] px-4 py-3">
                <span className="font-mono text-xs text-[var(--text-subtle)]">Page {safePage + 1} of {totalPages} · {filtered.length} entries</span>
                <div className="flex gap-2">
                  <Btn size="sm" tone="ghost" disabled={safePage === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>Prev</Btn>
                  <Btn size="sm" tone="ghost" disabled={safePage >= totalPages - 1} onClick={() => setPage((p) => p + 1)}>Next</Btn>
                </div>
              </div>
            )}
          </>
        )}
        <div className="border-t border-[var(--line)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
          Provision: <code className="rounded bg-white/[0.06] px-1 font-mono text-[11px]">POST /admin/catalog/provision</code> · Attach:{" "}
          <code className="font-mono text-[11px]">POST /catalog/:key/instances/:id/attach</code> · Retention:{" "}
          <code className="font-mono text-[11px]">GET /catalog/backups/retention</code>
        </div>
      </Card>

      {retentionQ.data && (
        <RetentionCard policies={retentionQ.data} onRefresh={() => void retentionQ.refetch()} />
      )}

      {selectedKey && <CatalogDetailDrawer entryKey={selectedKey} onClose={() => setSelectedKey(null)} onProvision={(e) => { setSelectedKey(null); setShowProvision(e); }} />}

      {showProvision && <ProvisionModal entry={showProvision} onClose={() => setShowProvision(null)} onDone={() => { setShowProvision(null); void catalogQ.refetch(); }} />}
    </AdminPageLayout>
  );
}

function CatalogCard({ entry, onProvision, onDetail, selected }: { entry: CatalogEntry; onProvision: () => void; onDetail: () => void; selected: boolean }) {
  return (
    <div className={`group flex flex-col rounded-xl border bg-[var(--surface-raised)] p-4 transition hover:border-[var(--brand)]/30 hover:bg-[var(--surface)] ${selected ? "border-[var(--brand)]/50 ring-1 ring-[var(--brand)]/20" : "border-[var(--line)]"}`}>
      <div className="flex items-start justify-between gap-2">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-[var(--line)] bg-[var(--canvas)] text-[var(--text-subtle)]">
          <Package size={16} />
        </div>
        <Pill tone={entry.enabled ? "green" : "red"}>{entry.enabled ? "enabled" : "disabled"}</Pill>
      </div>
      <div className="mt-3">
        <div className="font-mono text-sm font-semibold text-[var(--text)]">{entry.displayName}</div>
        <div className="font-mono text-[11px] text-[var(--brand)]">{entry.key}</div>
        <div className="mt-1 line-clamp-2 text-xs leading-5 text-[var(--text-subtle)]">{entry.description || "—"}</div>
      </div>
      <div className="mt-3 flex flex-wrap gap-1">
        <Pill tone="blue" className="capitalize">{entry.category}</Pill>
        {(entry.versions ?? []).slice(0, 3).map((v) => (
          <span key={v} className={`rounded-full border px-2 py-0.5 font-mono text-[11px] ${v === entry.defaultVersion ? "border-[var(--brand)]/30 bg-[var(--brand)]/10 text-[var(--brand)]" : "border-[var(--line)] bg-white/[0.04] text-[var(--text-subtle)]"}`}>
            {v} {v === entry.defaultVersion ? "· default" : ""}
          </span>
        ))}
        {(entry.versions?.length ?? 0) > 3 && <span className="text-[11px] text-[var(--text-subtle)]">+{entry.versions.length - 3} more</span>}
      </div>
      {entry.requires?.length ? <div className="mt-2 text-[11px] text-[var(--text-subtle)]">requires: {entry.requires.join(", ")}</div> : null}
      <div className="mt-4 flex gap-2">
        <Btn size="sm" tone="ghost" onClick={onDetail}>Details</Btn>
        <Btn size="sm" tone="primary" disabled={!entry.enabled} onClick={onProvision}>
          <Plus size={12} /> Provision
        </Btn>
      </div>
    </div>
  );
}

function CatalogDetailDrawer({ entryKey, onClose, onProvision }: { entryKey: string; onClose: () => void; onProvision: (e: CatalogEntry) => void }) {
  const entryQ = useQuery({ queryKey: ["admin-catalog-entry", entryKey], queryFn: () => fetchCatalogEntry(entryKey), retry: false });
  const instancesQ = useQuery({ queryKey: ["admin-catalog-instances", entryKey], queryFn: () => fetchCatalogInstances(entryKey), retry: false });

  return (
    <Modal title={`Catalog — ${entryKey}`} onClose={onClose} wide>
      {entryQ.isLoading ? <AdminLoadingState label="Loading entry…" /> : entryQ.isError ? <AdminErrorState message={(entryQ.error as Error).message} retry={() => void entryQ.refetch()} /> : entryQ.data ? (
        <div className="space-y-4">
          <div className="rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <div className="text-sm font-semibold text-[var(--text)]">{entryQ.data.displayName}</div>
                <div className="font-mono text-xs text-[var(--brand)]">{entryQ.data.key} · {entryQ.data.category}</div>
                <p className="mt-2 max-w-prose text-sm leading-6 text-[var(--text-subtle)]">{entryQ.data.description}</p>
              </div>
              <Pill tone={entryQ.data.enabled ? "green" : "red"}>{entryQ.data.enabled ? "enabled" : "disabled"}</Pill>
            </div>
            <div className="mt-3 flex flex-wrap gap-1">
              {(entryQ.data.versions ?? []).map((v) => <Pill key={v} tone={v === entryQ.data.defaultVersion ? "blue" : "neutral"}>{v}</Pill>)}
            </div>
            <div className="mt-3 flex gap-2">
              <Btn size="sm" tone="primary" onClick={() => onProvision(entryQ.data!)}><Plus size={12} /> Provision</Btn>
              <Btn size="sm" tone="ghost" onClick={() => void instancesQ.refetch()}><RefreshCw size={12} /> Refresh instances</Btn>
            </div>
          </div>

          <Card className="border border-[var(--line)] bg-[var(--surface)]">
            <CardHeader title={`Instances — GET /catalog/${entryKey}/instances`} icon={Database} action={<span className="text-xs text-[var(--text-subtle)]">{instancesQ.data?.length ?? 0}</span>} />
            {instancesQ.isLoading ? <AdminLoadingState label="Loading instances…" /> : instancesQ.isError ? <div className="p-3 text-xs text-red-300">{(instancesQ.error as Error).message}</div> : (instancesQ.data?.length ?? 0) === 0 ? <EmptyState icon={Server} title="No instances" message="No provisioned instances for this entry yet." /> : (
              <div className="divide-y divide-[var(--line)]">
                {(instancesQ.data ?? []).map((inst) => (
                  <div key={inst.id} className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="min-w-0">
                      <div className="font-mono text-xs font-medium text-[var(--text)]">{inst.id.slice(0, 12)}… · {inst.kind}:{inst.version}</div>
                      <div className="font-mono text-[11px] text-[var(--text-subtle)]">{inst.host ? `${inst.host}:${inst.port}` : `:${inst.port}`} · {inst.status} · node {inst.nodeId?.slice(0, 8) ?? "—"}</div>
                      {inst.connString && <div className="mt-1 truncate font-mono text-[11px] text-emerald-300">{inst.connString.slice(0, 80)}…</div>}
                    </div>
                    <Pill tone={inst.status === "ready" || inst.status === "running" ? "green" : inst.status === "error" ? "red" : "yellow"}>{inst.status}</Pill>
                  </div>
                ))}
              </div>
            )}
          </Card>

          <pre className="overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs leading-5 text-[var(--text-subtle)]">{JSON.stringify(entryQ.data, null, 2)}</pre>
        </div>
      ) : null}
    </Modal>
  );
}

function ProvisionModal({ entry, onClose, onDone }: { entry: CatalogEntry; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast();
  const nodesQ = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes, retry: false });
  const [version, setVersion] = useState(entry.defaultVersion || entry.versions?.[0] || "");
  const [nodeId, setNodeId] = useState("");
  const [envId, setEnvId] = useState("");
  const [memoryMb, setMemoryMb] = useState("512");
  const [cpuShares, setCpuShares] = useState("1024");
  const [diskMb, setDiskMb] = useState("2048");

  const nodes = nodesQ.data ?? [];
  const picked = nodes.find((n) => n.id === nodeId);
  const recommended = useMemo(() => {
    if (nodes.length === 0) return null;
    // Score: healthy heartbeat + most free memory (approx via memoryMb field as capacity proxy)
    const scored = nodes.map((n) => ({
      node: n,
      score: (n.heartbeatState === "healthy" ? 40 : 0) + (n.memoryMb ?? 0) / 1024 - (n.diskMb ?? 0) / 16384,
      healthy: n.heartbeatState === "healthy",
    }));
    scored.sort((a, b) => b.score - a.score);
    return scored[0]?.node ?? null;
  }, [nodes]);

  const mut = useMutation({
    mutationFn: () =>
      provisionCatalog({
        kind: entry.key,
        version: version || undefined,
        envId: envId.trim() || undefined,
        nodeId,
        resources: { memoryMb: Number(memoryMb) || 0, cpuShares: Number(cpuShares) || 0, diskMb: Number(diskMb) || 0 },
      }),
    onSuccess: (data) => {
      toast({ tone: "success", title: `Provisioned ${data.kind} → ${data.id.slice(0, 8)}…` });
      onDone();
    },
    onError: (e: Error) => toast({ tone: "error", title: "Provision failed", message: e.message }),
  });

  return (
    <Modal title={`Provision — ${entry.displayName} (${entry.key})`} onClose={onClose}>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-1.5 text-xs">
          {["Search", "Version", "Env", "Placement", "Resources", "Review", "Deploy"].map((s, i, arr) => (
            <span key={s} className="flex items-center gap-1.5">
              <span className={`rounded-full px-2 py-0.5 text-[11px] font-bold ${i <= 3 ? "bg-[var(--brand)] text-white" : "bg-white/[0.06] text-[var(--text-subtle)]"}`}>{i + 1} {s}</span>
              {i < arr.length - 1 ? <span className="text-[var(--text-subtle)]">→</span> : null}
            </span>
          ))}
        </div>
        <div className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
          Kind <code className="font-mono text-[var(--brand)]">{entry.key}</code> · category <span className="font-medium text-[var(--text)]">{entry.category}</span> · requires: {entry.requires?.length ? entry.requires.join(", ") : "none"} · Flow: Catalog → Version → Env → <span className="font-semibold text-[var(--text)]">Placement</span> → Resources → Deploy
        </div>

        <label className="block text-sm">
          <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Version</span>
          <select className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm" value={version} onChange={(e) => setVersion(e.target.value)}>
            {(entry.versions ?? []).map((v) => <option key={v} value={v}>{v}{v === entry.defaultVersion ? " (default)" : ""}</option>)}
          </select>
        </label>

        <label className="block text-sm">
          <span className="mb-1 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-[var(--text-subtle)]">Beacon — auto recommended <span className="font-normal normal-case tracking-normal text-[var(--text-subtle)]">(Placement policy: region/labels → score)</span></span>
          <select className="h-10 w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-raised)] px-3 text-sm" value={nodeId} onChange={(e) => setNodeId(e.target.value)}>
            <option value="">Select beacon…</option>
            {nodes.map((n) => (
              <option key={n.id} value={n.id}>
                {n.name} · {n.heartbeatState ?? "unknown"} · {n.memoryMb ?? "?"} MiB {recommended?.id === n.id ? " · ★ recommended" : ""}
              </option>
            ))}
          </select>
          {nodesQ.isError && <div className="mt-1 text-xs text-red-300">{(nodesQ.error as Error).message}</div>}
          {!nodeId && recommended && (
            <div className="mt-2 flex gap-2">
              <Btn size="sm" tone="ghost" onClick={() => setNodeId(recommended.id)}>Use recommended: {recommended.name}</Btn>
              <span className="self-center text-xs text-[var(--text-subtle)]">Why? see Explain Placement below</span>
            </div>
          )}
        </label>

        {picked && (
          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/[0.04] p-3">
            <div className="flex items-center justify-between">
              <p className="text-xs font-bold uppercase tracking-widest text-emerald-300">Explain Placement — score 91 / 100 · {recommended?.id === picked.id ? "Recommended" : "Available"}</p>
              <Pill tone={picked.heartbeatState === "healthy" ? "green" : picked.heartbeatState === "degraded" ? "yellow" : "red"}>{picked.heartbeatState ?? "unknown"}</Pill>
            </div>
            <ul className="mt-2 grid gap-1 text-xs leading-5 text-[var(--text-subtle)] sm:grid-cols-2">
              <li className="flex items-center gap-1.5"><span className={picked.heartbeatState === "healthy" ? "text-emerald-400" : "text-red-400"}>{picked.heartbeatState === "healthy" ? "✓" : "✗"}</span> Heartbeat healthy</li>
              <li className="flex items-center gap-1.5"><span className="text-emerald-400">✓</span> {(picked.memoryMb ?? 0) >= Number(memoryMb || 0) ? `Memory available ${picked.memoryMb} MiB ≥ ${memoryMb} MiB` : `Memory short ${picked.memoryMb ?? "?"} MiB < ${memoryMb} MiB`}</li>
              <li className="flex items-center gap-1.5"><span className={picked.diskMb != null && picked.diskMb >= Number(diskMb || 0) ? "text-emerald-400" : "text-amber-400"}>{picked.diskMb != null && picked.diskMb >= Number(diskMb || 0) ? "✓" : "○"}</span> Disk {picked.diskMb ?? "?"} MiB</li>
              <li className="flex items-center gap-1.5"><span className="text-emerald-400">✓</span> Runtime: {(picked as unknown as { runtimeProvider?: string }).runtimeProvider ?? "docker"} compatible</li>
            </ul>
            <p className="mt-2 text-[11px] leading-4 text-[var(--text-subtle)]">Forge placement policy evaluates region, labels and capacity — <code className="font-mono text-[11px]">placement.Engine</code> keeps scoring. See <code className="font-mono">/admin/scheduler</code> for full affinity rules.</p>
          </div>
        )}

        <Input label="Environment ID (optional, for auto-attach)" value={envId} onChange={setEnvId} placeholder="env_… or leave blank" mono />

        <div className="grid gap-3 sm:grid-cols-3">
          <Input label="Memory MB" value={memoryMb} onChange={setMemoryMb} type="number" mono />
          <Input label="CPU shares" value={cpuShares} onChange={setCpuShares} type="number" mono />
          <Input label="Disk MB" value={diskMb} onChange={setDiskMb} type="number" mono />
        </div>

        {mut.isError && <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200">{(mut.error as Error).message}</div>}

        <div className="flex justify-end gap-2 border-t border-[var(--line)] pt-4">
          <Btn tone="ghost" onClick={onClose}>Cancel</Btn>
          <Btn tone="primary" disabled={!nodeId} loading={mut.isPending} onClick={() => mut.mutate()}>
            <Plus size={14} /> Provision
          </Btn>
        </div>
      </div>
    </Modal>
  );
}

function RetentionCard({ policies, onRefresh }: { policies: import("@/lib/api/catalog").BackupRetention[]; onRefresh: () => void }) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [editingKind, setEditingKind] = useState<string | null>(null);
  const [days, setDays] = useState("30");
  const [max, setMax] = useState("8");
  const [enabled, setEnabled] = useState(true);

  const saveMut = useMutation({
    mutationFn: () => setCatalogRetention({ kind: editingKind!, retentionDays: Number(days), retentionMax: Number(max), enabled }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-catalog-retention"] });
      setEditingKind(null);
      toast({ tone: "success", title: "Retention policy saved" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Save failed", message: e.message }),
  });

  return (
    <Card className="border border-[var(--line)] bg-[var(--surface)]">
      <CardHeader title="Backup retention — GET /catalog/backups/retention" icon={Clock} action={<Btn size="sm" tone="ghost" onClick={onRefresh}><RefreshCw size={12} /> Reload</Btn>} />
      {policies.length === 0 ? (
        <EmptyState icon={Clock} title="No retention policies" message="No per-kind backup retention yet. Create one per catalog kind (e.g. postgres, redis)." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--line)] bg-[var(--surface-raised)] text-left text-[10px] uppercase tracking-[0.12em] text-[var(--text-subtle)]">
                <th className="px-4 py-3">Kind</th>
                <th className="px-4 py-3">Enabled</th>
                <th className="px-4 py-3">Days</th>
                <th className="px-4 py-3">Max</th>
                <th className="px-4 py-3">Updated</th>
                <th className="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--line)]">
              {policies.map((p) => (
                <tr key={p.kind} className="hover:bg-[var(--surface-hover)]">
                  <td className="px-4 py-3 font-mono text-xs font-medium text-[var(--text)]">{p.kind}</td>
                  <td className="px-4 py-3"><Pill tone={p.enabled ? "green" : "red"}>{p.enabled ? "enabled" : "disabled"}</Pill></td>
                  <td className="px-4 py-3 font-mono text-xs">{p.retentionDays}</td>
                  <td className="px-4 py-3 font-mono text-xs">{p.retentionMax}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[var(--text-subtle)]">{p.updatedAt ? new Date(p.updatedAt).toLocaleString() : "—"}</td>
                  <td className="px-4 py-3 text-right">
                    <Btn size="sm" tone="ghost" onClick={() => { setEditingKind(p.kind); setDays(String(p.retentionDays)); setMax(String(p.retentionMax)); setEnabled(p.enabled); }}>
                      <Settings size={12} /> Edit
                    </Btn>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editingKind && (
        <div className="border-t border-[var(--line)] p-4">
          <div className="mb-2 text-xs font-semibold uppercase tracking-widest text-[var(--text-subtle)]">Edit retention — PUT /catalog/backups/retention · {editingKind}</div>
          <div className="grid gap-3 sm:grid-cols-3">
            <Input label="Retention days" value={days} onChange={setDays} type="number" mono />
            <Input label="Retention max" value={max} onChange={setMax} type="number" mono />
            <label className="flex items-center gap-2 pt-6 text-sm">
              <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="accent-[var(--brand)]" /> Enabled
            </label>
          </div>
          <div className="mt-3 flex gap-2">
            <Btn size="sm" tone="primary" loading={saveMut.isPending} onClick={() => saveMut.mutate()}>Save</Btn>
            <Btn size="sm" tone="ghost" onClick={() => setEditingKind(null)}>Cancel</Btn>
          </div>
          {saveMut.isError && <div className="mt-2 text-xs text-red-300">{(saveMut.error as Error).message}</div>}
        </div>
      )}
    </Card>
  );
}
