"use client";

import { useRef, useState } from "react";
import { CheckCircle2, CircleSlash, Sparkles, RefreshCw, Wand2, Map as MapIcon } from "lucide-react";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminErrorState,
  AdminPageHeader,
  AdminPageLayout,
  Btn,
  Card,
  CardHeader,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { NodeSelect } from "@/components/admin/node-select";
import { useConfirm } from "@/components/ui/confirm-dialog";
import * as api from "@/lib/api/envaffinity";
import { sanitizeError } from "@/lib/sanitize";
import { cn } from "@/lib/utils";

function ConstraintChips({ items }: { items?: api.PlacementConstraint[] }) {
  if (!items || items.length === 0) {
    return <p className="text-meta text-text-subtle">No constraints reported — this workload is not pinned to an environment group.</p>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {items.map((c, i) => (
        <span
          key={i}
          className={cn(
            "inline-flex items-center gap-1 rounded-md border px-2 py-0.5 font-mono text-eyebrow",
            c.required ? "border-warn-line bg-warn-subtle text-warn" : "border-line bg-overlay-subtle text-text-subtle",
          )}
        >
          {/* The requirement word is rendered, not hidden in a `title` tooltip: a
              required and a preferred constraint otherwise look identical when the
              colour is not perceived. */}
          <span className="font-semibold">{c.required ? "required" : "preferred"}</span>
          {c.key} {c.operator} [{(c.values ?? []).join(", ") || "no values"}]
        </span>
      ))}
    </div>
  );
}

export function EnvAffinityManager() {
  const [confirm, renderConfirm] = useConfirm();
  const [nodeId, setNodeId] = useState("");
  // The viewer and the preview are two independent questions — "why would this node
  // host X?" and "what would X be pinned to?" — and used to share one `serverId`
  // state, so typing in one box silently changed the other.
  const [viewerServerId, setViewerServerId] = useState("");
  const [previewServerId, setPreviewServerId] = useState("");
  const [explain, setExplain] = useState<api.ExplainResult | null>(null);
  const [enriched, setEnriched] = useState<api.EnrichedPlacement | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [patched, setPatched] = useState<api.PatchResult | null>(null);
  const [busy, setBusy] = useState<"explain" | "enrich" | "patch" | null>(null);
  /**
   * The banner's Retry has to retry something real. This page loads nothing on
   * mount, so the only honest retry is "run the last action again" — recorded here
   * instead of leaving `onRetry` as an empty closure.
   */
  const lastAction = useRef<"explain" | "enrich" | "patch" | null>(null);

  async function run(kind: "explain" | "enrich" | "patch", fn: () => Promise<void>) {
    setError(null);
    setBusy(kind);
    try {
      await fn();
      lastAction.current = kind;
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Request failed"));
    } finally {
      setBusy(null);
    }
  }

  const handleExplain = () =>
    run("explain", async () => {
      if (!nodeId.trim()) throw new Error("Choose a node first — the page will not pick one for you.");
      setExplain(await api.explainPlacement(nodeId.trim(), { serverId: viewerServerId.trim() || undefined }));
    });

  const handleEnrich = () =>
    run("enrich", async () => {
      setEnriched(await api.enrichPlacement({ serverId: previewServerId.trim() || undefined }));
    });

  const confirmAndPatch = () =>
    void (async () => {
      const ok = await confirm({
        title: "Rebuild scheduler affinity rules for every server?",
        description:
          "This rewrites the fleet's placement constraints in one pass: every server's pinned environment is re-resolved and the resulting affinity rules replace what the scheduler holds. It affects where all future workloads land, not just one server.",
        confirmLabel: "Rebuild rules",
        danger: true,
      });
      if (!ok) return;
      await run("patch", async () => {
        setPatched(await api.patchConstraints());
      });
    })();

  const retryLast = () => {
    const which = lastAction.current;
    if (!which) {
      setError("Nothing to retry yet — no action has been run on this page.");
      return;
    }
    if (which === "explain") void handleExplain();
    else if (which === "enrich") void handleEnrich();
    else void confirmAndPatch();
  };

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={retryLast} />
      <AdminPageHeader
        description="Explain why a node would or would not host a server, preview the constraints a pinned environment adds, and rebuild scheduler affinity rules. Rule editing itself lives on the Scheduler page."
        status={error ? <span className="font-mono text-eyebrow text-danger">Last action failed</span> : null}
      />

      {error && (
        <AdminErrorState message={error} retry={lastAction.current ? retryLast : undefined} />
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Affinity viewer" icon={MapIcon} />
          <div className="space-y-3">
            <div>
              <NodeSelect label="Node to evaluate" onChange={setNodeId} value={nodeId} />
            </div>
            <Input label="Server (optional)" value={viewerServerId} onChange={setViewerServerId} placeholder="server whose environment pinning is applied" mono />
            <Btn tone="primary" loading={busy === "explain"} disabled={!nodeId.trim()} onClick={() => void handleExplain()}>
              <Sparkles size={14} /> Explain placement
            </Btn>

            {explain && (
              <div className="mt-2 space-y-3 rounded-xl border border-line bg-[var(--surface-raised)] p-4">
                <div className="flex flex-wrap items-center gap-2">
                  {explain.isCandidate ? (
                    <Pill tone="green"><CheckCircle2 aria-hidden="true" size={12} /> eligible for this placement</Pill>
                  ) : (
                    <Pill tone="red"><CircleSlash aria-hidden="true" size={12} /> not a candidate</Pill>
                  )}
                  <span className="font-mono text-xs text-text-subtle">{explain.nodeId}</span>
                  {explain.requestedEnv ? <Pill tone="blue">env: {explain.requestedEnv}</Pill> : <Pill tone="neutral">no environment requested</Pill>}
                </div>

                {(explain.nodeEnvGroups?.length || 0) > 0 && (
                  <div className="flex flex-wrap items-center gap-1.5 text-xs">
                    <span className="text-text-subtle">Node advertises:</span>
                    {explain.nodeEnvGroups.map((g) => (
                      <span key={g} className="rounded bg-overlay-subtle px-1.5 py-0.5 font-mono text-eyebrow text-text">{g}</span>
                    ))}
                  </div>
                )}

                {explain.matchedLabels?.length ? (
                  <p className="text-meta text-ok">Matched: {explain.matchedLabels.join("  ·  ")}</p>
                ) : null}
                {explain.missingLabels?.length ? (
                  <p className="text-meta text-danger">Missing: {explain.missingLabels.join("  ·  ")}</p>
                ) : null}
                {!explain.matchedLabels?.length && !explain.missingLabels?.length ? (
                  <p className="text-meta text-text-muted">The scorer reported neither matched nor missing labels for this pairing.</p>
                ) : null}

                <div>
                  <p className="t-eyebrow mb-1">Applied constraints</p>
                  <ConstraintChips items={explain.constraints} />
                </div>

                {explain.ranking?.length ? (
                  <div>
                    <p className="t-eyebrow mb-1">Fleet ranking (top {Math.min(explain.ranking.length, 6)} of {explain.ranking.length})</p>
                    <ol className="space-y-1">
                      {explain.ranking.slice(0, 6).map((r, i) => (
                        <li key={r.nodeId} className="flex items-center gap-2 text-xs">
                          <span className="w-4 text-right text-text-subtle">{i + 1}</span>
                          <span className={cn("font-mono", r.nodeId === explain.nodeId ? "text-info" : "text-text")}>
                            {r.nodeId}
                            {r.nodeId === explain.nodeId ? " (this node)" : ""}
                          </span>
                          <span className="text-text-subtle">{Number.isFinite(r.score) ? r.score.toFixed(0) : "no score"}</span>
                          {r.env ? <Pill tone="blue">{r.env}</Pill> : null}
                        </li>
                      ))}
                    </ol>
                  </div>
                ) : (
                  <p className="text-meta text-text-muted">No fleet ranking was returned, so this node cannot be compared with the rest.</p>
                )}
              </div>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title="Preview & resync" icon={Wand2} />
          <div className="space-y-4">
            <div className="flex items-end gap-2">
              <div className="flex-1">
                <Input label="Server to preview" value={previewServerId} onChange={setPreviewServerId} placeholder="server ID" mono />
              </div>
              <Btn tone="ghost" loading={busy === "enrich"} disabled={!previewServerId.trim()} onClick={() => void handleEnrich()}>
                <Sparkles size={14} /> Preview
              </Btn>
            </div>

            {enriched && (
              <div className="rounded-xl border border-line bg-[var(--surface-raised)] p-4 text-sm">
                <div className="mb-2 flex items-center gap-2">
                  <span className="t-eyebrow">Resolved env group</span>
                  {enriched.envGroup ? <Pill tone="blue">{enriched.envGroup}</Pill> : <Pill tone="neutral">no env group resolved</Pill>}
                </div>
                <ConstraintChips items={enriched.constraints as api.PlacementConstraint[]} />
              </div>
            )}

            <div className="border-t border-line pt-4">
              <p className="mb-1 text-sm font-semibold text-text">Rebuild affinity rules fleet-wide</p>
              <p className="mb-2 text-meta leading-5 text-text-subtle">
                Re-resolves every server&rsquo;s pinned environment and the least-loaded node in each env group, then replaces the
                scheduler&rsquo;s affinity rules with the result. Idempotent, but it is a fleet-wide write and is confirmed first.
              </p>
              <Btn tone="danger" loading={busy === "patch"} onClick={confirmAndPatch}>
                <RefreshCw size={14} /> Rebuild placement constraints
              </Btn>
              {patched && (
                <div className="ui-alert ui-alert-info mt-3">
                  <span>
                    <strong className="font-semibold">{Number.isFinite(patched.serversPinned) ? patched.serversPinned : "an unknown number of"}</strong> servers pinned ·
                    {" "}<strong className="font-semibold">{Number.isFinite(patched.rulesRegistered) ? patched.rulesRegistered : "an unknown number of"}</strong> rules registered
                    {patched.envsMapped?.length ? ` · env groups: ${patched.envsMapped.join(", ")}` : " · no env groups reported"}
                  </span>
                </div>
              )}
            </div>
          </div>
        </Card>
      </div>
      {renderConfirm()}
    </AdminPageLayout>
  );
}
