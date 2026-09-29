"use client";
import { sourceState, useNodesQuery } from "@/lib/admin/telemetry";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Container, HardDrive, Info, Layers, PlayCircle, Plus, RefreshCw, Trash2, Archive } from "lucide-react";

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
  type DockerPruneResult,
} from "@/lib/api/docker-cleanup";
import { errorMessage, formatBytes, formatDate } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { chart } from "@/lib/design-tokens";
import { FreshnessBadge } from "./telemetry-ui";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
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
} from "./admin-ui";

// Admin "Image & Cache Cleanup" panel. Per-node Docker disk accounting
// (images / containers / volumes / build cache), an unused-image list that
// honours a retention floor, prune actions with confirmation, and cron-based
// cleanup policies. The heavy lifting (Beacon dispatch, the scheduler loop,
// retention logic) lives in the control plane; this view renders what the API
// actually reports and says so where it does not.

const NOT_REPORTED = "Not reported";

/** Runtimes whose engine this panel can prune. An unset provider is Docker, the deployed default. */
const PRUNABLE_RUNTIMES = ["", "docker", "podman", "containerd"];

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
  const tags = (img.tags ?? []).filter((tag) => tag && tag !== "<none>:<none>" && !tag.includes("<none>"));
  if (tags.length > 0) return tags[0];
  const dangling = (img.tags ?? []).find((tag) => tag && tag !== "<none>:<none>");
  return dangling ?? `${shortId(img.id)} (dangling)`;
}

function bytes(value: number | null | undefined): string {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? formatBytes(value) : NOT_REPORTED;
}

/**
 * A prune result is only as good as the engine's report. Beacon sets
 * `reclaimedBytesKnown:false` when it could not measure freed space, but the
 * control-plane wire type (`dockerleanup.PruneResult`) has no field for it, so
 * an unmeasured prune arrives here as `reclaimedBytes: 0`. Zero objects removed
 * is a real result; objects removed with a 0 byte report is not a measurement,
 * and must never be toasted as "0 B reclaimed" (AGENTS.md: not-reported is not
 * zero).
 */
function pruneSummary(res: DockerPruneResult | undefined): { message: string; measured: boolean } {
  if (!res) return { message: "The node returned no prune report.", measured: false };
  const removed = typeof res.removedCount === "number" ? res.removedCount : null;
  const reclaimed = typeof res.reclaimedBytes === "number" && Number.isFinite(res.reclaimedBytes) ? res.reclaimedBytes : null;
  const removedText = removed === null ? "Removal count not reported" : `${removed} removed`;
  if (removed !== null && removed === 0) {
    return { message: `${removedText} · nothing was deleted`, measured: true };
  }
  if (removed !== null && removed > 0 && (reclaimed === null || reclaimed === 0)) {
    return { message: `${removedText} · reclaimed space not measured by the engine`, measured: false };
  }
  return { message: `${removedText} · ${bytes(reclaimed)} reclaimed`, measured: true };
}

const STATUS_TONE: Record<string, "green" | "red" | "yellow" | "neutral"> = {
  success: "green",
  failed: "red",
  running: "yellow",
};

type PolicyForm = {
  nodeId: string;
  schedule: string;
  mostRecentLimit: string;
  enabled: boolean;
  pruneBuildCache: boolean;
  pruneVolumes: boolean;
};

const EMPTY_FORM: PolicyForm = {
  nodeId: "",
  schedule: "0 3 * * *",
  mostRecentLimit: "1",
  enabled: true,
  pruneBuildCache: true,
  pruneVolumes: false,
};

export function DockerCleanupManager() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  // Nothing is read and nothing is pruned until the operator names a node. The
  // old `nodes[0]` fallback aimed destructive prunes at whichever node sorted
  // first, and the audit endpoints take the node in the path, so the backend's
  // ambiguity guard never saw the request.
  const [nodeId, setNodeId] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [limitText, setLimitText] = useState("1");
  const [policyModal, setPolicyModal] = useState<{ mode: "create" | "edit"; policy?: DockerCleanupPolicy } | null>(null);

  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const nodeOptions = useMemo(
    () => nodes.map((node) => ({ value: node.id, label: node.status === "active" ? node.name : `${node.name} · ${node.status}` })),
    [nodes],
  );

  const limit = useMemo(() => {
    const value = Number(limitText.trim());
    return Number.isInteger(value) && value >= 0 ? value : null;
  }, [limitText]);

  const selectedNode = nodes.find((node) => node.id === nodeId);
  const runtimeProvider = (selectedNode?.runtimeProvider ?? "").toLowerCase();
  const canPrune = PRUNABLE_RUNTIMES.includes(runtimeProvider);

  const usageQuery = useQuery({
    queryKey: ["admin", "docker-cleanup", "disk-usage", nodeId],
    enabled: Boolean(nodeId),
    queryFn: () => getDiskUsage(nodeId),
  });
  const usage = usageQuery.data;

  const unusedQuery = useQuery({
    queryKey: ["admin", "docker-cleanup", "unused", nodeId, limit],
    enabled: Boolean(nodeId) && limit !== null,
    queryFn: () => listUnusedImages(nodeId, limit ?? 1),
  });
  const unused = useMemo(() => unusedQuery.data ?? [], [unusedQuery.data]);

  // An image the engine never sized contributes nothing to a reclaim estimate,
  // so a total over a partly-unsized list is a lower bound, not a measurement.
  const sized = unused.filter((img) => typeof img.size === "number" && Number.isFinite(img.size) && img.size >= 0);
  const unsizedCount = unused.length - sized.length;
  const reclaimable = sized.reduce((sum, img) => sum + img.size, 0);

  const policiesQuery = useQuery({ queryKey: ["admin", "docker-cleanup", "policies"], queryFn: listPolicies });
  const policies = useMemo(() => policiesQuery.data ?? [], [policiesQuery.data]);

  const nodeName = selectedNode?.name ?? (nodeId ? `${nodeId.slice(0, 12)}…` : NOT_REPORTED);

  const refreshAll = () => {
    void qc.invalidateQueries({ queryKey: ["admin", "docker-cleanup"] });
  };

  const afterPrune = (label: string) => (res: DockerPruneResult) => {
    const summary = pruneSummary(res);
    toast({
      tone: summary.measured ? "success" : "warning",
      title: summary.measured ? label : `${label} — result not fully measured`,
      message: `${summary.message} on ${nodeName}`,
    });
    setSelected(new Set());
    refreshAll();
  };

  const pruneImagesMut = useMutation({
    mutationFn: (ids: string[]) => pruneImages(nodeId, ids),
    onSuccess: afterPrune("Unused images pruned"),
    onError: (err) => toast({ tone: "error", title: "Prune failed", message: errorMessage(err) }),
  });
  const pruneCacheMut = useMutation({
    mutationFn: () => pruneBuildCache(nodeId),
    onSuccess: afterPrune("Build cache pruned"),
    onError: (err) => toast({ tone: "error", title: "Build cache prune failed", message: errorMessage(err) }),
  });
  const pruneVolumesMut = useMutation({
    mutationFn: () => pruneVolumes(nodeId),
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
      title: `Prune ${ids.length} image${ids.length === 1 ? "" : "s"} on ${nodeName}?`,
      description: `The selected images will be removed from ${nodeName}. Images still referenced by a container are skipped by the engine. Retention floor in effect: ${limit === 0 ? "0, which preserves nothing" : `${limit} most recent unused image(s)`}. Freed space may be reported as unmeasured if the engine cannot size the removals.`,
      danger: true,
      confirmLabel: "Prune images",
    });
    if (ok) pruneImagesMut.mutate(ids);
  };

  const handlePruneCache = async () => {
    const ok = await confirm({
      title: `Prune build cache on ${nodeName}?`,
      description: `Reclaims unused BuildKit layers on ${nodeName}. The last read measured ${bytes(usage?.buildCacheBytes)} of build cache; this prune removes that whole category, not a selection, and in-progress builds are not affected. Freed space may be reported as unmeasured.`,
      danger: true,
      confirmLabel: "Prune cache",
    });
    if (ok) pruneCacheMut.mutate();
  };

  const handlePruneVolumes = async () => {
    const ok = await confirm({
      title: `Prune dangling volumes on ${nodeName}?`,
      description: `Volumes attached to any container are left alone by the engine, but this panel cannot preview which volumes qualify: the last read measured ${bytes(usage?.volumesBytes)} in volumes and reports no per-volume list. Freed space and skipped volumes may be unreported.`,
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
        message: tone === "error"
          ? p.lastError || "The policy reported no reason."
          : p.nextRunAt ? `Next run ${formatDate(p.nextRunAt, "Not scheduled")}` : "Next run not scheduled yet",
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
    const scope = policy.nodeId ? nodes.find((node) => node.id === policy.nodeId)?.name ?? policy.nodeId : "every reachable node";
    const ok = await confirm({
      title: "Delete cleanup policy?",
      description: `The "${policy.schedule}" policy for ${scope} will stop running. Existing images are not affected by deleting the policy itself.`,
      danger: true,
      confirmLabel: "Delete policy",
    });
    if (ok) deletePolicyMut.mutate(policy.id);
  };

  return (
    <AdminPageLayout>
      <SectionHeader
        info={{
          description: "Reclaims node disk by pruning unused images, build cache and dangling volumes, guarded by a retention floor and scheduled policies.",
          eyebrow: "Architecture & Semantics",
          sections: [
            {
              content:
                "Prune actions target one named node; pick it below and nothing is read or deleted until you do. The panel no longer defaults to the first node in the list.",
              icon: HardDrive,
              title: "One named node",
            },
            {
              content:
                "The unused-image list always preserves the newest N images by creation date so the deployed version is never stranded — unless N is 0, which preserves nothing and makes every not-in-use image a candidate. Prunes skip images still referenced by a container.",
              icon: Archive,
              title: "Retention floor",
            },
            {
              content:
                "Totals here are the four Docker categories the engine reported, summed by the panel — not the filesystem size, and not free space. Per-mount totals live in Host Inspector. Freed space is reported as unmeasured when the engine cannot size it, never as 0 B.",
              icon: Info,
              title: "What \"total\" means",
            },
            {
              content:
                "Policies run on a cron schedule across all nodes or one node, optionally including build cache and volumes. Run-now executes a policy immediately, outside its schedule.",
              icon: PlayCircle,
              title: "Scheduled policies",
            },
          ],
          title: "Image and cache cleanup",
          triggerLabel: "About Image & Cache Cleanup",
        }}
        status={nodeId ? <FreshnessBadge state={sourceState(usageQuery)} /> : null}
        action={
          <Btn disabled={!nodeId} loading={usageQuery.isFetching || unusedQuery.isFetching} onClick={refreshAll} size="sm" tone="ghost">
            <RefreshCw size={14} className="mr-1.5" /> Refresh
          </Btn>
        }
      />

      <Card>
        <CardHeader icon={HardDrive} title="Node" />
        <div className="grid gap-3 p-4 sm:max-w-md">
          <AdminSelect
            disabled={nodesQuery.isPending || nodeOptions.length === 0}
            label="Report on node"
            onChange={(value) => {
              setNodeId(value);
              setSelected(new Set());
            }}
            options={nodeOptions}
            placeholder={nodesQuery.isPending ? "Loading nodes…" : nodeOptions.length === 0 ? "No nodes registered" : "Select a node…"}
            value={nodeId}
          />
          {nodesQuery.isError ? (
            <AdminErrorState message={errorMessage(nodesQuery.error, "The node list could not be loaded.")} retry={() => void nodesQuery.refetch()} />
          ) : null}
          {nodeId && !canPrune ? (
            <p className="text-xs text-warn">
              {selectedNode?.name} reports the {runtimeProvider} runtime. Image, cache and volume pruning drives a
              Docker-family engine through Beacon, so the actions below are disabled for this node.
            </p>
          ) : null}
        </div>
      </Card>

      {!nodeId ? (
        <EmptyState
          icon={HardDrive}
          message="Disk usage, unused images and prune actions all belong to one machine. Pick it above — the panel does not choose a node for you, and pruning is not reversible."
          title="No node selected"
        />
      ) : (
        <>
          {usageQuery.isError ? (
            <AdminErrorState message={errorMessage(usageQuery.error, "Disk usage could not be loaded.")} retry={() => void usageQuery.refetch()} />
          ) : null}

          {usageQuery.isPending ? <AdminLoadingState label="Querying node disk usage…" /> : null}

          {usage ? (
            <Card>
              <CardHeader title={`Docker categories on ${usage.nodeName || nodeName}`} />
              <div className="p-4">
                <p className="mb-4 text-xs leading-5 text-text-subtle">
                  These four categories are what the engine reported for {nodeName}, summed by the panel. It is not
                  the size of the disk and says nothing about free space — per-mount totals and percentages live in
                  Host Inspector → Disk.
                </p>
                <StatsRow
                  items={[
                    { label: "Measured", value: bytes(usage.totalBytes), icon: HardDrive, tone: "neutral" },
                    { label: "Images", value: bytes(usage.imagesBytes), icon: Container, tone: "neutral" },
                    { label: "Containers", value: bytes(usage.containersBytes), icon: Layers, tone: "neutral" },
                    { label: "Build cache", value: bytes(usage.buildCacheBytes), icon: Archive, tone: "neutral" },
                  ]}
                />
                <UsageBar
                  total={usage.totalBytes}
                  segments={[
                    { label: "Images", bytes: usage.imagesBytes, color: chart.blue },
                    { label: "Containers", bytes: usage.containersBytes, color: chart.violet },
                    { label: "Volumes", bytes: usage.volumesBytes, color: chart.lightCyan },
                    { label: "Build cache", bytes: usage.buildCacheBytes, color: chart.disk },
                  ]}
                />
              </div>
            </Card>
          ) : null}

          {usage ? (
            <Card>
              <CardHeader
                title="Unused images"
                action={
                  unusedQuery.isPending ? (
                    <Badge>Analyzing images…</Badge>
                  ) : unusedQuery.isError ? (
                    <Badge>Reclaim estimate unavailable</Badge>
                  ) : (
                    <Badge>
                      {formatBytes(reclaimable)} in {sized.length} sized image{unsizedCount ? ` + ${unsizedCount} unsized` : ""}
                    </Badge>
                  )
                }
              />
              <div className="flex flex-wrap items-end gap-3 border-b border-line p-4">
                <div className="sm:w-40">
                  <Input
                    label="Keep newest N"
                    onChange={(value) => setLimitText(value)}
                    type="number"
                    value={limitText}
                  />
                </div>
                <p className="min-w-48 flex-1 text-xs leading-5 text-text-subtle">
                  {limit === null ? (
                    <span className="text-warn">Enter a whole number of 0 or more — the list is not re-read until the value is valid.</span>
                  ) : limit === 0 ? (
                    <span className="text-warn">
                      Retention floor 0 preserves nothing: every image not in use becomes a prune candidate,
                      including the version a workload is running.
                    </span>
                  ) : (
                    <>
                      Retention floor: the {limit === 1 ? "1 most recent" : `${limit} most recent`} unused image(s) by
                      creation date are always preserved so the deployed version is never stranded.
                    </>
                  )}
                </p>
                <Btn
                  disabled={selected.size === 0 || !canPrune}
                  loading={pruneImagesMut.isPending}
                  onClick={handlePruneSelected}
                  size="sm"
                  tone="danger"
                >
                  <Trash2 size={14} className="mr-1.5" /> Prune selected ({selected.size})
                </Btn>
                <Btn disabled={!canPrune} loading={pruneCacheMut.isPending} onClick={handlePruneCache} size="sm" tone="danger">
                  Prune build cache
                </Btn>
                <Btn disabled={!canPrune} loading={pruneVolumesMut.isPending} onClick={handlePruneVolumes} size="sm" tone="danger">
                  Prune volumes
                </Btn>
              </div>
              {!canPrune ? (
                <p className="px-4 pt-3 text-xs text-warn">Pruning is disabled because this node does not report a Docker-family runtime.</p>
              ) : null}

              {unusedQuery.isPending ? (
                <div className="p-4"><AdminLoadingState label="Analyzing images…" /></div>
              ) : unusedQuery.isError ? (
                <div className="p-4"><AdminErrorState message={errorMessage(unusedQuery.error, "Unused images could not be loaded.")} retry={() => void unusedQuery.refetch()} /></div>
              ) : unused.length === 0 ? (
                <EmptyState
                  icon={Container}
                  message={`No unused image was reported beyond the retention floor (${limit === 0 ? "floor 0 — nothing preserved" : `floor ${limit}`}).`}
                  title="No prune candidates"
                />
              ) : (
                <AdminTable label={`Unused images on ${nodeName}`}>
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
                            aria-label={`Select ${imageLabel(img)}`}
                            checked={selected.has(img.id)}
                            className="h-4 w-4 accent-[var(--brand)]"
                            onChange={() => toggleSelected(img.id)}
                            type="checkbox"
                          />
                        </AdminTd>
                        <AdminTd>
                          <div className="font-medium text-text">{imageLabel(img)}</div>
                          <div className="font-mono text-xs text-text-muted">{shortId(img.id)}</div>
                        </AdminTd>
                        <AdminTd className="whitespace-nowrap font-mono">{bytes(img.size)}</AdminTd>
                        <AdminTd className="whitespace-nowrap text-text-subtle" title={formatDate(img.createdAt, NOT_REPORTED)}>
                          {formatDate(img.createdAt, NOT_REPORTED)}
                          <span className="ml-1 text-text-muted">({img.createdAt ? formatAge(img.createdAt) : NOT_REPORTED})</span>
                        </AdminTd>
                      </AdminTr>
                    ))}
                  </AdminTBody>
                </AdminTable>
              )}
              {unsizedCount > 0 ? (
                <p className="px-4 pb-4 pt-2 text-xs leading-5 text-warn">
                  {unsizedCount} image{unsizedCount === 1 ? "" : "s"} in this list were reported without a size, so the
                  reclaim estimate is a lower bound. The engine's accounting gaps are not carried to this panel.
                </p>
              ) : null}
            </Card>
          ) : null}
        </>
      )}

      <Card>
        <CardHeader
          action={
            <Btn onClick={() => setPolicyModal({ mode: "create" })} size="sm">
              <Plus size={14} className="mr-1.5" /> New policy
            </Btn>
          }
          icon={PlayCircle}
          title="Cleanup policies"
        />
        {policiesQuery.isPending ? (
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
              {policies.map((policy) => {
                const scope = policy.nodeId ? nodes.find((node) => node.id === policy.nodeId)?.name ?? `${policy.nodeId.slice(0, 8)}…` : "All nodes";
                const runningThis = runNowMut.isPending && runNowMut.variables === policy.id;
                const togglingThis = togglePolicyMut.isPending && togglePolicyMut.variables?.id === policy.id;
                return (
                  <AdminTr key={policy.id}>
                    <AdminTd>
                      <div className="font-medium text-text">{scope}</div>
                      <div className="flex gap-1.5 pt-1">
                        {policy.pruneBuildCache ? <Badge>cache</Badge> : null}
                        {policy.pruneVolumes ? <Badge>volumes</Badge> : null}
                      </div>
                    </AdminTd>
                    <AdminTd><span className="font-mono text-xs text-text-subtle">{policy.schedule}</span></AdminTd>
                    <AdminTd>
                      {policy.mostRecentLimit === 0
                        ? <span className="text-warn">keep 0 (nothing preserved)</span>
                        : `keep ${policy.mostRecentLimit}`}
                    </AdminTd>
                    <AdminTd className="whitespace-nowrap text-text-subtle">
                      {policy.enabled
                        ? formatDate(policy.nextRunAt, "Not scheduled")
                        : <span className="text-text-muted">disabled</span>}
                    </AdminTd>
                    <AdminTd>
                      <Pill tone={STATUS_TONE[policy.lastStatus ?? ""] ?? "unknown"}>
                        {policy.lastStatus || "not run"}
                      </Pill>
                      <div className="pt-1 text-xs text-text-muted">
                        {policy.lastRunAt ? formatDate(policy.lastRunAt, NOT_REPORTED) : "Never run"}
                        {policy.lastStatus === "failed" && policy.lastError ? ` — ${policy.lastError}` : ""}
                      </div>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex items-center justify-end gap-1.5">
                        <Btn
                          ariaLabel={policy.enabled ? `Disable the ${policy.schedule} policy` : `Enable the ${policy.schedule} policy`}
                          loading={togglingThis}
                          onClick={() => togglePolicyMut.mutate({ id: policy.id, enabled: !policy.enabled })}
                          size="sm"
                          tone="ghost"
                        >
                          {policy.enabled ? "Disable" : "Enable"}
                        </Btn>
                        <Btn
                          ariaLabel={`Run the ${scope} ${policy.schedule} policy now`}
                          disabled={runNowMut.isPending}
                          loading={runningThis}
                          onClick={() => runNowMut.mutate(policy.id)}
                          size="sm"
                          tone="subtle"
                          title="Run now"
                        >
                          <PlayCircle size={14} />
                        </Btn>
                        <Btn ariaLabel={`Edit the ${scope} policy`} onClick={() => setPolicyModal({ mode: "edit", policy })} size="sm" tone="ghost">Edit</Btn>
                        <Btn
                          ariaLabel={`Delete the ${scope} ${policy.schedule} policy`}
                          onClick={() => void handleDeletePolicy(policy)}
                          size="sm"
                          tone="danger"
                          title="Delete policy"
                        >
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
          defaultNodeId={nodeId}
          nodeOptions={nodeOptions}
          onClose={() => setPolicyModal(null)}
          onDone={() => {
            setPolicyModal(null);
            refreshAll();
          }}
        />
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );
}

// Chart ramp, not status: segment hues distinguish disk categories at a glance
// and come from the categorical `chart` tokens rather than a page-local palette.

function UsageBar({ total, segments }: { total: number; segments: Array<{ label: string; bytes: number; color: string }> }) {
  const measured = typeof total === "number" && Number.isFinite(total) && total > 0;
  const sum = segments.reduce((acc, segment) => acc + (Number.isFinite(segment.bytes) ? segment.bytes : 0), 0);
  const safeTotal = measured ? total : sum;
  if (!safeTotal) {
    return (
      <p className="rounded-lg border border-dashed border-unknown-line bg-unknown-subtle px-3 py-2 text-xs text-unknown" role="status">
        No byte counts were reported for this node, so there is nothing to chart. This is an unmeasured read, not an empty disk.
      </p>
    );
  }
  return (
    <div className="space-y-3">
      <div className="flex h-3 w-full overflow-hidden rounded-full bg-overlay-strong" aria-hidden="true">
        {segments.map((segment) => {
          const pct = Math.max(0, Math.min(100, (segment.bytes / safeTotal) * 100));
          if (pct <= 0) return null;
          return <div key={segment.label} className="h-full" style={{ backgroundColor: segment.color, width: `${pct}%` }} title={`${segment.label}: ${formatBytes(segment.bytes)}`} />;
        })}
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {segments.map((segment) => {
          const pct = Math.round((segment.bytes / safeTotal) * 100);
          return (
            <div key={segment.label} className="flex items-center gap-2 text-xs">
              <span aria-hidden="true" className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: segment.color }} />
              <span className="text-text-subtle">{segment.label}</span>
              <span className="ml-auto font-mono text-text">{formatBytes(segment.bytes)}</span>
              <span className="w-12 text-right font-mono text-text-muted">{pct}%</span>
            </div>
          );
        })}
      </div>
      <p className="text-xs text-text-muted">Percentages are each category's share of the {formatBytes(safeTotal)} measured, not of the disk.</p>
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
          mostRecentLimit: String(initial.mostRecentLimit),
          enabled: initial.enabled,
          pruneBuildCache: initial.pruneBuildCache,
          pruneVolumes: initial.pruneVolumes,
        }
      : { ...EMPTY_FORM, nodeId: defaultNodeId },
  );
  const [errors, setErrors] = useState<string[]>([]);

  // A policy with no node is global (types.go: "NodeID == '' means the policy is
  // global"), so "All nodes" has to be an option the operator can actually pick
  // — including when editing a policy that already is global.
  const scopeOptions = useMemo(
    () => [{ value: "", label: `All nodes (global)${nodeOptions.length ? ` — ${nodeOptions.length} registered nodes` : ""}` }, ...nodeOptions],
    [nodeOptions],
  );

  const limitValue = useMemo(() => {
    const value = Number(form.mostRecentLimit.trim());
    return Number.isInteger(value) && value >= 0 ? value : null;
  }, [form.mostRecentLimit]);

  const saveMut = useMutation({
    mutationFn: async () => {
      const nextErrors: string[] = [];
      if (!form.schedule.trim()) nextErrors.push("A cron schedule is required (5 fields, e.g. 0 3 * * *).");
      if (limitValue === null) nextErrors.push("The retention floor must be a whole number of 0 or more.");
      if (nextErrors.length) {
        setErrors(nextErrors);
        throw new Error(nextErrors[0]);
      }
      setErrors([]);
      return mode === "create"
        ? createPolicy({
            nodeId: form.nodeId || undefined,
            schedule: form.schedule.trim(),
            mostRecentLimit: limitValue ?? 1,
            enabled: form.enabled,
            pruneBuildCache: form.pruneBuildCache,
            pruneVolumes: form.pruneVolumes,
          })
        : updatePolicy(initial!.id, {
            nodeId: form.nodeId,
            schedule: form.schedule.trim(),
            mostRecentLimit: limitValue ?? 1,
            enabled: form.enabled,
            pruneBuildCache: form.pruneBuildCache,
            pruneVolumes: form.pruneVolumes,
          });
    },
    onSuccess: () => {
      toast({ tone: "success", title: mode === "create" ? "Policy created" : "Policy updated" });
      onDone();
    },
    onError: (err) => toast({ tone: "error", title: "Could not save policy", message: errorMessage(err) }),
  });

  const set = <K extends keyof PolicyForm,>(key: K, value: PolicyForm[K]) => setForm((prev) => ({ ...prev, [key]: value }));
  const scopeLabel = form.nodeId ? nodeOptions.find((option) => option.value === form.nodeId)?.label ?? `${form.nodeId.slice(0, 8)}…` : "all nodes";

  return (
    <Modal
      description="A cron schedule with a retention floor. Runs across all nodes or a single node; the floor decides which images are never pruned."
      onClose={onClose}
      title={mode === "create" ? "New cleanup policy" : "Edit cleanup policy"}
    >
      <div className="space-y-4">
        {errors.length ? <AdminErrorState message={`Fix before saving — ${errors.join(" ")}`} /> : null}
        <AdminSelect
          label="Scope"
          onChange={(value) => set("nodeId", value)}
          options={scopeOptions}
          placeholder="All nodes (global)"
          value={form.nodeId}
        />
        <Input label="Cron schedule" mono onChange={(value) => set("schedule", value)} placeholder="0 3 * * *" required value={form.schedule} />
        <Input label="Most recent to keep" onChange={(value) => set("mostRecentLimit", value)} type="number" value={form.mostRecentLimit} />
        <p className="text-xs leading-5 text-text-subtle">
          {limitValue === null
            ? "Enter a whole number of 0 or more."
            : limitValue === 0
              ? "0 preserves nothing: a scheduled run will prune every not-in-use image, including the version a workload is running."
              : `The ${limitValue} most recent unused image${limitValue === 1 ? "" : "s"} by creation date are always preserved on ${scopeLabel}.`}
        </p>
        <div className="space-y-2 rounded-lg border border-line p-3">
          <ToggleRow checked={form.enabled} label="Enabled" onChange={(value) => set("enabled", value)} />
          <ToggleRow checked={form.pruneBuildCache} label="Also prune build cache" onChange={(value) => set("pruneBuildCache", value)} />
          <ToggleRow checked={form.pruneVolumes} label="Also prune dangling volumes" onChange={(value) => set("pruneVolumes", value)} />
        </div>
      </div>
      <ModalFooter
        confirmLabel={mode === "create" ? "Create policy" : "Save changes"}
        destructive={limitValue === 0}
        disabled={saveMut.isPending}
        onCancel={onClose}
        onConfirm={() => saveMut.mutate()}
      />
    </Modal>
  );
}

function ToggleRow({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <label className="flex items-center justify-between gap-3 text-sm text-text">
      <span>{label}</span>
      <input checked={checked} className="h-4 w-4 accent-[var(--brand)]" onChange={(event) => onChange(event.target.checked)} type="checkbox" />
    </label>
  );
}
