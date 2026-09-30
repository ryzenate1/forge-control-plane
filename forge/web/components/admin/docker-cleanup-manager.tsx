"use client";
import { useNodesQuery, sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "./telemetry-ui";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Container, HardDrive, Layers, PlayCircle, Plus, RefreshCw, Trash2, Archive } from "lucide-react";

import {
  createPolicy,
  deletePolicy,
  getDiskUsage,
  listPolicies,
  listUnusedImages,
  pruneBuildCache,
  pruneImages,
  pruneVolumes,
  runPolicyNow,
  updatePolicy,
  type DockerCleanupPolicy,
  type DockerImageInfo,
} from "@/lib/api/docker-cleanup";
import { errorMessage, formatBytes, formatDate } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Badge,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  StatsRow,
  cn,
} from "./admin-ui";

// Admin "Docker Disk Usage & Automated Cleanup" panel. Per-node disk
// accounting (images / containers / volumes / build cache), an unused-image
// list that honours a retention floor, one-click prune actions with
// confirmation, and cron-based cleanup policies. The heavy lifting (Beacon
// dispatch, the scheduler loop, retention logic) lives in the control plane;
// this view is a thin, honest renderer over the admin API.

function formatAge(iso: string): string {
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  const days = Math.floor((Date.now() - t) / 86_400_000);
  if (days <= 0) return "today";
  if (days === 1) return "1 day ago";
  if (days < 30) return `${days} days ago`;
  const months = Math.floor(days / 30);
  return months <= 1 ? "~1 month ago" : `~${months} months ago`;
}

function shortId(id: string): string {
  return id.startsWith("sha256:") ? id.slice(7, 19) : id.slice(0, 12);
}

function imageLabel(img: DockerImageInfo): string {
  const tags = (img.tags ?? []).filter((t) => t && t !== "<none>:<none>" && !t.includes("<none>"));
  if (tags.length > 0) return tags[0];
  const dangling = (img.tags ?? []).find((t) => t && t !== "<none>:<none>");
  return dangling ?? `${shortId(img.id)} (dangling)`;
}

const STATUS_TONE: Record<string, "green" | "red" | "yellow" | "neutral"> = {
  success: "green",
  failed: "red",
  running: "yellow",
};

type PolicyForm = {
  nodeId: string;
  schedule: string;
  mostRecentLimit: number;
  enabled: boolean;
  pruneBuildCache: boolean;
  pruneVolumes: boolean;
};

const EMPTY_FORM: PolicyForm = {
  nodeId: "",
  schedule: "0 3 * * *",
  mostRecentLimit: 1,
  enabled: true,
  pruneBuildCache: true,
  pruneVolumes: false,
};

export function DockerCleanupManager() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  const [nodeId, setNodeId] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [limit, setLimit] = useState(1);
  const [policyModal, setPolicyModal] = useState<{ mode: "create" | "edit"; policy?: DockerCleanupPolicy } | null>(null);

  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const nodeOptions = useMemo(
    () => nodes.map((n) => ({ value: n.id, label: n.name })),
    [nodes],
  );

  // Default to the first node once the list arrives so the panel is useful
  // immediately without a manual selection.
  const activeNodeId = nodeId || (nodes[0]?.id ?? "");

  const usageQuery = useQuery({
    queryKey: ["admin", "docker-cleanup", "disk-usage", activeNodeId],
    enabled: Boolean(activeNodeId),
    queryFn: () => getDiskUsage(activeNodeId),
  });
  const usage = usageQuery.data;

  const unusedQuery = useQuery({
    queryKey: ["admin", "docker-cleanup", "unused", activeNodeId, limit],
    enabled: Boolean(activeNodeId),
    queryFn: () => listUnusedImages(activeNodeId, limit),
  });
  const unused = useMemo(() => unusedQuery.data ?? [], [unusedQuery.data]);
  const reclaimable = useMemo(() => unused.reduce((sum, img) => sum + (img.size || 0), 0), [unused]);

  const policiesQuery = useQuery({ queryKey: ["admin", "docker-cleanup", "policies"], queryFn: listPolicies });
  const policies = useMemo(() => policiesQuery.data ?? [], [policiesQuery.data]);

  const nodeName = (nodes.find((n) => n.id === activeNodeId)?.name ?? activeNodeId) || "—";

  const refreshAll = () => {
    void qc.invalidateQueries({ queryKey: ["admin", "docker-cleanup"] });
  };

  const afterPrune = (label: string) => (res: { reclaimedBytes: number; removedCount: number }) => {
    // Beacon answers `reclaimedBytesKnown: false` with `reclaimedBytes: 0`
    // when it could not size the removals; the control-plane type drops that
    // flag, so 0-with-removals is "not measured", never "0 B reclaimed".
    const reclaimed = res.removedCount > 0 && !(res.reclaimedBytes > 0)
      ? "reclaimed amount not measured"
      : `${formatBytes(res.reclaimedBytes)} reclaimed`;
    toast({
      tone: "success",
      title: label,
      message: `${res.removedCount} removed · ${reclaimed}`,
    });
    setSelected(new Set());
    refreshAll();
  };

  const pruneImagesMut = useMutation({
    mutationFn: (ids: string[]) => pruneImages(activeNodeId, ids),
    onSuccess: afterPrune("Unused images pruned"),
    onError: (err) => toast({ tone: "error", title: "Prune failed", message: errorMessage(err) }),
  });
  const pruneCacheMut = useMutation({
    mutationFn: () => pruneBuildCache(activeNodeId),
    onSuccess: afterPrune("Build cache pruned"),
    onError: (err) => toast({ tone: "error", title: "Build cache prune failed", message: errorMessage(err) }),
  });
  const pruneVolumesMut = useMutation({
    mutationFn: () => pruneVolumes(activeNodeId),
    onSuccess: afterPrune("Dangling volumes pruned"),
    onError: (err) => toast({ tone: "error", title: "Volume prune failed", message: errorMessage(err) }),
  });

  const toggleSelected = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handlePruneSelected = async () => {
    const ids = [...selected];
    if (ids.length === 0) return;
    const ok = await confirm({
      title: `Prune ${ids.length} image${ids.length === 1 ? "" : "s"}?`,
      description: "The selected images will be removed from this node. Images still referenced by a container are skipped by the engine.",
      danger: true,
      confirmLabel: "Prune images",
    });
    if (ok) pruneImagesMut.mutate(ids);
  };

  const handlePruneCache = async () => {
    const ok = await confirm({
      title: "Prune build cache?",
      description: "Reclaims unused BuildKit layers on this node. In-progress builds are not affected.",
      danger: true,
      confirmLabel: "Prune cache",
    });
    if (ok) pruneCacheMut.mutate();
  };

  const handlePruneVolumes = async () => {
    const ok = await confirm({
      title: "Prune dangling volumes?",
      description: "Removes only volumes not attached to any container. Volumes still in use are preserved.",
      danger: true,
      confirmLabel: "Prune volumes",
    });
    if (ok) pruneVolumesMut.mutate();
  };

  const runNowMut = useMutation({
    mutationFn: (id: string) => runPolicyNow(id),
    onSuccess: (p) => {
      const tone = p.lastStatus === "failed" ? "error" : "success";
      toast({
        tone,
        title: tone === "error" ? "Policy run failed" : "Policy run finished",
        message: tone === "error" ? p.lastError ?? "See policy for details." : `Next run ${formatDate(p.nextRunAt)}`,
      });
      refreshAll();
    },
    onError: (err) => toast({ tone: "error", title: "Policy run failed", message: errorMessage(err) }),
  });

  const deletePolicyMut = useMutation({
    mutationFn: (id: string) => deletePolicy(id),
    onSuccess: () => {
      toast({ tone: "success", title: "Policy deleted" });
      refreshAll();
    },
    onError: (err) => toast({ tone: "error", title: "Delete failed", message: errorMessage(err) }),
  });

  const togglePolicyMut = useMutation({
    mutationFn: (vars: { id: string; enabled: boolean }) => updatePolicy(vars.id, { enabled: vars.enabled }),
    onSuccess: () => refreshAll(),
    onError: (err) => toast({ tone: "error", title: "Update failed", message: errorMessage(err) }),
  });

  const handleDeletePolicy = async (policy: DockerCleanupPolicy) => {
    const ok = await confirm({
      title: "Delete cleanup policy?",
      description: `Schedule "${policy.schedule}" will stop running. Existing images are not affected.`,
      danger: true,
      confirmLabel: "Delete policy",
    });
    if (ok) deletePolicyMut.mutate(policy.id);
  };

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Docker Cleanup"
        sub="Per-node disk usage and automated, retention-aware cleanup of unused images, build cache and dangling volumes."
        status={<FreshnessBadge state={sourceState(usageQuery, 30_000)} />}
        action={
          <Btn tone="ghost" size="sm" onClick={refreshAll} loading={usageQuery.isFetching}>
            <RefreshCw size={14} className="mr-1.5" /> Refresh
          </Btn>
        }
      />

      <Card>
        <CardHeader title="Node" icon={Layers} />
        <div className="grid gap-3 p-4 sm:max-w-md">
          <AdminSelect
            label="Report on node"
            value={activeNodeId}
            onChange={(v) => {
              setNodeId(v);
              setSelected(new Set());
            }}
            options={nodeOptions}
            placeholder={nodesQuery.isLoading ? "Loading nodes…" : "Select a node"}
            disabled={nodesQuery.isLoading || nodeOptions.length === 0}
          />
          {nodeOptions.length === 0 && !nodesQuery.isLoading ? (
            <p className="text-xs text-text-muted">No nodes are registered yet.</p>
          ) : null}
        </div>
      </Card>

      {usageQuery.isError ? (
        <AdminErrorState message={errorMessage(usageQuery.error, "Disk usage could not be loaded.")} retry={() => void usageQuery.refetch()} />
      ) : null}

      {activeNodeId && usageQuery.isLoading ? <AdminLoadingState label="Querying node disk usage…" /> : null}

      {usage ? (
        <Card>
          <CardHeader title={`Disk usage — ${usage.nodeName || nodeName}`} icon={HardDrive} />
          <div className="p-4">
            <StatsRow
              items={[
                { label: "Total", value: formatBytes(usage.totalBytes), icon: HardDrive, tone: "neutral" },
                { label: "Images", value: formatBytes(usage.imagesBytes), icon: Container, tone: "blue" },
                { label: "Containers", value: formatBytes(usage.containersBytes), icon: Layers, tone: "yellow" },
                { label: "Build cache", value: formatBytes(usage.buildCacheBytes), icon: Archive, tone: "green" },
              ]}
            />
            <UsageBar
              total={usage.totalBytes}
              segments={[
                { label: "Images", bytes: usage.imagesBytes, color: "bg-sky-500" },
                { label: "Containers", bytes: usage.containersBytes, color: "bg-violet-500" },
                { label: "Volumes", bytes: usage.volumesBytes, color: "bg-ok" },
                { label: "Build cache", bytes: usage.buildCacheBytes, color: "bg-warn" },
              ]}
            />
          </div>
        </Card>
      ) : null}

      {usage ? (
        <Card>
          <CardHeader
            title="Unused images"
            icon={Container}
            action={<Badge className="bg-info-subtle text-info">{formatBytes(reclaimable)} reclaimable</Badge>}
          />
          <div className="flex flex-wrap items-end gap-3 border-b border-[var(--line)] p-4">
            <div className="sm:w-40">
              <Input label="Keep newest N" value={String(limit)} onChange={(v) => setLimit(Math.max(0, Number(v) || 0))} type="number" />
            </div>
            <p className="flex-1 text-xs text-text-muted">Retention floor: the {limit === 1 ? "1 most recent" : `${limit} most recent`} unused image(s) by creation date are always preserved so the deployed version is never stranded.</p>
            <Btn
              tone="danger"
              size="sm"
              disabled={selected.size === 0}
              loading={pruneImagesMut.isPending}
              onClick={handlePruneSelected}
            >
              <Trash2 size={14} className="mr-1.5" /> Prune selected ({selected.size})
            </Btn>
            <Btn tone="subtle" size="sm" disabled={!activeNodeId} loading={pruneCacheMut.isPending} onClick={handlePruneCache}>
              Prune build cache
            </Btn>
            <Btn tone="subtle" size="sm" disabled={!activeNodeId} loading={pruneVolumesMut.isPending} onClick={handlePruneVolumes}>
              Prune volumes
            </Btn>
          </div>
          {unusedQuery.isLoading ? (
            <div className="p-4"><AdminLoadingState label="Analyzing images…" /></div>
          ) : unusedQuery.isError ? (
            <div className="p-4"><AdminErrorState message={errorMessage(unusedQuery.error, "Unused images could not be loaded.")} retry={() => void unusedQuery.refetch()} /></div>
          ) : unused.length === 0 ? (
            <EmptyState icon={Container} title="Nothing to prune" sub="No unused images beyond the retention floor on this node." />
          ) : (
            <AdminTable label="Unused images">
              <AdminTHead>
                <AdminTh className="w-10"><span className="sr-only">Select</span></AdminTh>
                <AdminTh>Image</AdminTh>
                <AdminTh>Size</AdminTh>
                <AdminTh>Created</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {unused.map((img) => (
                  <AdminTr key={img.id}>
                    <AdminTd>
                      <input
                        type="checkbox"
                        aria-label={`Select ${imageLabel(img)}`}
                        className="h-4 w-4 accent-[var(--brand)]"
                        checked={selected.has(img.id)}
                        onChange={() => toggleSelected(img.id)}
                      />
                    </AdminTd>
                    <AdminTd>
                      <div className="font-medium text-text">{imageLabel(img)}</div>
                      <div className="font-mono text-xs text-text-muted">{shortId(img.id)}</div>
                    </AdminTd>
                    <AdminTd className="whitespace-nowrap font-mono">{formatBytes(img.size)}</AdminTd>
                    <AdminTd className="whitespace-nowrap text-text-subtle">{formatDate(img.createdAt)} <span className="text-text-muted">({formatAge(img.createdAt)})</span></AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      ) : null}

      <Card>
        <CardHeader
          title="Cleanup policies"
          icon={PlayCircle}
          action={
            <Btn size="sm" onClick={() => setPolicyModal({ mode: "create" })} disabled={!activeNodeId}>
              <Plus size={14} className="mr-1.5" /> New policy
            </Btn>
          }
        />
        {policiesQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading policies…" /></div>
        ) : policiesQuery.isError ? (
          <div className="p-4"><AdminErrorState message={errorMessage(policiesQuery.error, "Policies could not be loaded.")} retry={() => void policiesQuery.refetch()} /></div>
        ) : policies.length === 0 ? (
          <EmptyState icon={PlayCircle} title="No policies" sub="Create a cron-based policy to prune unused images automatically on a schedule." />
        ) : (
          <AdminTable label="Cleanup policies">
            <AdminTHead>
              <AdminTh>Scope</AdminTh>
              <AdminTh>Schedule</AdminTh>
              <AdminTh>Retention</AdminTh>
              <AdminTh>Next run</AdminTh>
              <AdminTh>Last run</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {policies.map((p) => {
                const scope = p.nodeId ? nodes.find((n) => n.id === p.nodeId)?.name ?? p.nodeId.slice(0, 8) : "All nodes";
                return (
                  <AdminTr key={p.id}>
                    <AdminTd>
                      <div className="font-medium text-text">{scope}</div>
                      <div className="flex gap-1.5 pt-1">
                        {p.pruneBuildCache ? <Badge className="bg-warn-subtle text-warn">cache</Badge> : null}
                        {p.pruneVolumes ? <Badge className="bg-ok-subtle text-ok">volumes</Badge> : null}
                      </div>
                    </AdminTd>
                    <AdminTd><span className="font-mono text-xs text-text">{p.schedule}</span></AdminTd>
                    <AdminTd>keep {p.mostRecentLimit}</AdminTd>
                    <AdminTd className="whitespace-nowrap text-text-subtle">{p.enabled ? formatDate(p.nextRunAt) : <span className="text-text-muted">disabled</span>}</AdminTd>
                    <AdminTd>
                      <Pill tone={STATUS_TONE[p.lastStatus ?? ""] ?? "neutral"}>{p.lastStatus ?? "never"}</Pill>
                      <div className="pt-1 text-xs text-text-muted">{formatDate(p.lastRunAt)}</div>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex items-center justify-end gap-1.5">
                        <Btn size="sm" tone="ghost" onClick={() => togglePolicyMut.mutate({ id: p.id, enabled: !p.enabled })}>
                          {p.enabled ? "Disable" : "Enable"}
                        </Btn>
                        <Btn size="sm" tone="subtle" disabled={runNowMut.isPending} onClick={() => runNowMut.mutate(p.id)} title="Run now">
                          <PlayCircle size={14} />
                        </Btn>
                        <Btn size="sm" tone="ghost" onClick={() => setPolicyModal({ mode: "edit", policy: p })}>Edit</Btn>
                        <Btn size="sm" tone="danger" onClick={() => handleDeletePolicy(p)} title="Delete policy">
                          <Trash2 size={14} />
                        </Btn>
                      </div>
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {policyModal ? (
        <PolicyModal
          initial={policyModal.mode === "edit" && policyModal.policy ? policyModal.policy : undefined}
          mode={policyModal.mode}
          defaultNodeId={activeNodeId}
          nodeOptions={nodeOptions}
          onClose={() => setPolicyModal(null)}
          onDone={() => {
            setPolicyModal(null);
            refreshAll();
          }}
        />
      ) : null}

      {renderConfirm()}
    </div>
  );
}

function UsageBar({ total, segments }: { total: number; segments: Array<{ label: string; bytes: number; color: string }> }) {
  const safeTotal = total > 0 ? total : segments.reduce((sum, s) => sum + s.bytes, 0) || 1;
  return (
    <div className="space-y-3">
      <div className="flex h-3 w-full overflow-hidden rounded-full bg-overlay">
        {segments.map((s) => {
          const pct = Math.max(0, Math.min(100, (s.bytes / safeTotal) * 100));
          if (pct <= 0) return null;
          return <div key={s.label} className={cn(s.color, "h-full")} style={{ width: `${pct}%` }} title={`${s.label}: ${formatBytes(s.bytes)}`} />;
        })}
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {segments.map((s) => {
          const pct = total > 0 ? Math.round((s.bytes / total) * 100) : 0;
          return (
            <div key={s.label} className="flex items-center gap-2 text-xs">
              <span className={cn("h-2.5 w-2.5 rounded-full", s.color)} aria-hidden="true" />
              <span className="text-text-subtle">{s.label}</span>
              <span className="ml-auto font-mono text-text">{formatBytes(s.bytes)}</span>
              <span className="w-9 text-right text-text-muted">{pct}%</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function PolicyModal({
  mode,
  initial,
  defaultNodeId,
  nodeOptions,
  onClose,
  onDone,
}: {
  mode: "create" | "edit";
  initial?: DockerCleanupPolicy;
  defaultNodeId: string;
  nodeOptions: Array<{ value: string; label: string }>;
  onClose: () => void;
  onDone: () => void;
}) {
  const { toast } = useToast();
  const [form, setForm] = useState<PolicyForm>(() =>
    initial
      ? {
          nodeId: initial.nodeId ?? "",
          schedule: initial.schedule,
          mostRecentLimit: initial.mostRecentLimit,
          enabled: initial.enabled,
          pruneBuildCache: initial.pruneBuildCache,
          pruneVolumes: initial.pruneVolumes,
        }
      : { ...EMPTY_FORM, nodeId: defaultNodeId },
  );

  const saveMut = useMutation({
    mutationFn: () =>
      mode === "create"
        ? createPolicy({
            nodeId: form.nodeId || undefined,
            schedule: form.schedule,
            mostRecentLimit: form.mostRecentLimit,
            enabled: form.enabled,
            pruneBuildCache: form.pruneBuildCache,
            pruneVolumes: form.pruneVolumes,
          })
        : updatePolicy(initial!.id, {
            nodeId: form.nodeId,
            schedule: form.schedule,
            mostRecentLimit: form.mostRecentLimit,
            enabled: form.enabled,
            pruneBuildCache: form.pruneBuildCache,
            pruneVolumes: form.pruneVolumes,
          }),
    onSuccess: () => {
      toast({ tone: "success", title: mode === "create" ? "Policy created" : "Policy updated" });
      onDone();
    },
    onError: (err) => toast({ tone: "error", title: "Could not save policy", message: errorMessage(err) }),
  });

  const set = <K extends keyof PolicyForm,>(key: K, value: PolicyForm[K]) => setForm((prev) => ({ ...prev, [key]: value }));
  const scheduleValid = form.schedule.trim().length > 0;

  return (
    <Modal open title={mode === "create" ? "New cleanup policy" : "Edit cleanup policy"} onClose={onClose} description="A cron schedule with a retention floor; runs across all nodes or a single node.">
      <div className="space-y-4">
        <AdminSelect
          label="Scope"
          value={form.nodeId}
          onChange={(v) => set("nodeId", v)}
          options={nodeOptions}
          placeholder="All nodes (global)"
        />
        <Input label="Cron schedule" value={form.schedule} onChange={(v) => set("schedule", v)} mono placeholder="0 3 * * *" required />
        {!scheduleValid ? <p className="text-xs text-danger">A schedule is required (5-field cron, e.g. <span className="font-mono">0 3 * * *</span>).</p> : null}
        <Input label="Most recent to keep" value={String(form.mostRecentLimit)} onChange={(v) => set("mostRecentLimit", Math.max(0, Number(v) || 0))} type="number" />
        <div className="space-y-2 rounded-lg border border-[var(--line)] p-3">
          <ToggleRow label="Enabled" checked={form.enabled} onChange={(v) => set("enabled", v)} />
          <ToggleRow label="Also prune build cache" checked={form.pruneBuildCache} onChange={(v) => set("pruneBuildCache", v)} />
          <ToggleRow label="Also prune dangling volumes" checked={form.pruneVolumes} onChange={(v) => set("pruneVolumes", v)} />
        </div>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => saveMut.mutate()} confirmLabel={mode === "create" ? "Create policy" : "Save changes"} disabled={!scheduleValid || saveMut.isPending} />
    </Modal>
  );
}

function ToggleRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex items-center justify-between gap-3 text-sm text-text">
      <span>{label}</span>
      <input type="checkbox" className="h-4 w-4 accent-[var(--brand)]" checked={checked} onChange={(e) => onChange(e.target.checked)} />
    </label>
  );
}
