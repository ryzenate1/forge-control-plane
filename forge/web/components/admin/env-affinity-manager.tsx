"use client";

import { useState } from "react";
import { CheckCircle2, CircleSlash, Sparkles, RefreshCw, Wand2, Map as MapIcon } from "lucide-react";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageHeader,
  AdminPageLayout,
  Btn,
  Card,
  CardHeader,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import * as api from "@/lib/api/envaffinity";
import { sanitizeError } from "@/lib/sanitize";
import { cn } from "@/lib/utils";

function ConstraintChips({ items }: { items?: api.PlacementConstraint[] }) {
  if (!items || items.length === 0) {
    return <p className="text-xs text-[var(--text-subtle)]">No constraints — workload is unconstrained by environment.</p>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {items.map((c, i) => (
        <span
          key={i}
          className={cn(
            "inline-flex items-center gap-1 rounded-md border px-2 py-0.5 font-mono text-[11px]",
            c.required ? "border-amber-500/30 bg-amber-500/10 text-amber-200" : "border-white/10 bg-white/[0.03] text-slate-300",
          )}
          title={c.required ? "required" : "preferred"}
        >
          {c.key} {c.operator} [{(c.values ?? []).join(", ")}]
        </span>
      ))}
    </div>
  );
}

export function EnvAffinityManager() {
  const [nodeId, setNodeId] = useState("");
  const [serverId, setServerId] = useState("");
  const [explain, setExplain] = useState<api.ExplainResult | null>(null);
  const [enriched, setEnriched] = useState<api.EnrichedPlacement | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [patched, setPatched] = useState<api.PatchResult | null>(null);
  const [busy, setBusy] = useState<"explain" | "enrich" | "patch" | null>(null);

  async function run(kind: "explain" | "enrich" | "patch", fn: () => Promise<void>) {
    setError(null);
    setBusy(kind);
    try {
      await fn();
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Request failed"));
    } finally {
      setBusy(null);
    }
  }

  const handleExplain = () =>
    run("explain", async () => {
      if (!nodeId.trim()) throw new Error("node id is required");
      setExplain(await api.explainPlacement(nodeId.trim(), { serverId: serverId.trim() || undefined }));
    });

  const handleEnrich = () =>
    run("enrich", async () => {
      setEnriched(await api.enrichPlacement({ serverId: serverId.trim() || undefined }));
    });

  const handlePatch = () =>
    run("patch", async () => {
      setPatched(await api.patchConstraints());
    });

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => { /* per-action retry */ }} />
      <AdminPageHeader
        title="Placement Affinity"
        description="Debug how environments pin workloads to nodes. Explain why a node would or would not host a server, preview the constraints an env-affinity adds, and re-sync affinity rules into the scheduler."
      />

      {error && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-500/[0.09] p-4 text-sm text-red-200">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06]">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Affinity viewer" icon={MapIcon} />
          <div className="space-y-3 p-4">
            <Input label="Node" value={nodeId} onChange={setNodeId} placeholder="node id to evaluate" mono />
            <Input label="Server (optional)" value={serverId} onChange={setServerId} placeholder="server id whose env affinity is applied" mono />
            <Btn tone="primary" loading={busy === "explain"} onClick={() => void handleExplain()}>
              <Sparkles size={14} /> Explain placement
            </Btn>

            {explain && (
              <div className="mt-2 space-y-3 rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-4">
                <div className="flex items-center gap-2">
                  {explain.isCandidate ? (
                    <Pill tone="green"><CheckCircle2 size={12} /> eligible node</Pill>
                  ) : (
                    <Pill tone="red"><CircleSlash size={12} /> not a candidate</Pill>
                  )}
                  <span className="font-mono text-xs text-[var(--text-subtle)]">{explain.nodeId}</span>
                  {explain.requestedEnv ? <Pill tone="blue">env: {explain.requestedEnv}</Pill> : null}
                </div>

                {(explain.nodeEnvGroups?.length || 0) > 0 && (
                  <div className="flex flex-wrap items-center gap-1.5 text-xs">
                    <span className="text-[var(--text-subtle)]">Node advertises:</span>
                    {explain.nodeEnvGroups.map((g) => (
                      <span key={g} className="rounded bg-white/[0.06] px-1.5 py-0.5 font-mono text-[11px] text-slate-200">{g}</span>
                    ))}
                  </div>
                )}

                {explain.matchedLabels?.length ? (
                  <div className="text-xs text-emerald-300">✓ {explain.matchedLabels.join("  ·  ")}</div>
                ) : null}
                {explain.missingLabels?.length ? (
                  <div className="text-xs text-red-300">✗ {explain.missingLabels.join("  ·  ")}</div>
                ) : null}

                <div>
                  <div className="mb-1 text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Applied constraints</div>
                  <ConstraintChips items={explain.constraints} />
                </div>

                {explain.ranking?.length ? (
                  <div>
                    <div className="mb-1 text-[11px] uppercase tracking-widest text-[var(--text-subtle)]">Fleet ranking (top {Math.min(explain.ranking.length, 6)})</div>
                    <ol className="space-y-1">
                      {explain.ranking.slice(0, 6).map((r, i) => (
                        <li key={r.nodeId} className="flex items-center gap-2 text-xs">
                          <span className="w-4 text-right text-[var(--text-subtle)]">{i + 1}</span>
                          <span className={cn("font-mono", r.nodeId === explain.nodeId ? "text-blue-300" : "text-slate-300")}>{r.nodeId}</span>
                          <span className="text-[var(--text-subtle)]">{r.score.toFixed(0)}</span>
                          {r.env ? <Pill tone="blue">{r.env}</Pill> : null}
                        </li>
                      ))}
                    </ol>
                  </div>
                ) : null}
              </div>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title="Enrich & resync" icon={Wand2} />
          <div className="space-y-4 p-4">
            <div className="flex items-end gap-2">
              <div className="flex-1">
                <Input label="Server" value={serverId} onChange={setServerId} placeholder="server id to preview" mono />
              </div>
              <Btn tone="ghost" loading={busy === "enrich"} onClick={() => void handleEnrich()}>
                <Sparkles size={14} /> Preview
              </Btn>
            </div>

            {enriched && (
              <div className="rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] p-4 text-sm">
                <div className="mb-2 flex items-center gap-2">
                  <span className="text-xs uppercase tracking-widest text-[var(--text-subtle)]">Resolved env group</span>
                  {enriched.envGroup ? <Pill tone="blue">{enriched.envGroup}</Pill> : <Pill tone="neutral">none</Pill>}
                </div>
                <ConstraintChips items={enriched.constraints as api.PlacementConstraint[]} />
              </div>
            )}

            <div className="border-t border-[var(--line)] pt-4">
              <div className="mb-1 text-sm font-semibold text-[var(--text)]">Resync affinity rules</div>
              <p className="mb-2 text-xs text-[var(--text-subtle)]">
                Rebuilds scheduler affinity rules from every server&rsquo;s pinned environment and the least-loaded node in each env group. Safe to re-run.
              </p>
              <Btn tone="primary" loading={busy === "patch"} onClick={() => void handlePatch()}>
                <RefreshCw size={14} /> Patch placement constraints
              </Btn>
              {patched && (
                <div className="mt-3 flex flex-wrap gap-3 rounded-xl border border-emerald-500/20 bg-emerald-500/[0.08] p-3 text-xs text-emerald-200">
                  <span><strong>{patched.serversPinned}</strong> servers pinned</span>
                  <span><strong>{patched.rulesRegistered}</strong> rules registered</span>
                  {patched.envsMapped?.length ? <span>envs: {patched.envsMapped.join(", ")}</span> : null}
                </div>
              )}
            </div>
          </div>
        </Card>
      </div>
    </AdminPageLayout>
  );
}
