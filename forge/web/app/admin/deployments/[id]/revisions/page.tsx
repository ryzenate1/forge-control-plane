"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import { useState } from "react";
import {
  ArrowLeft, ArrowRightLeft, GitCommit, History, Layers,
  Package, RefreshCw, RotateCcw, Search, ShieldAlert,
} from "lucide-react";
import { fetchJSON, postJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";

type Revision = {
  id: string;
  deploymentId: string;
  revisionNumber: number;
  imageRef: string;
  composeManifestRef: string;
  gitCommitSha: string;
  configHash: string;
  status: string;
  deployedAt?: string;
  description: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
};

type RevisionDiff = {
  fromRevisionId: number;
  toRevisionId: number;
  changes: { field: string; oldValue: string; newValue: string }[];
};

const statusConfig: Record<string, { tone: "green" | "yellow" | "neutral" | "red" }> = {
  active: { tone: "green" },
  pending: { tone: "yellow" },
  superseded: { tone: "neutral" },
  failed: { tone: "red" },
};

export default function DeploymentRevisionsPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const id = params.id as string;
  const [showDiff, setShowDiff] = useState(false);
  const [diffFrom, setDiffFrom] = useState("");
  const [diffTo, setDiffTo] = useState("");
  const [diffData, setDiffData] = useState<RevisionDiff | null>(null);

  const revsQuery = useQuery({
    queryKey: ["admin", "deployments", id, "revisions"],
    queryFn: () => fetchJSON<Revision[]>(`/admin/deployments/${id}/revisions`),
  });

  const compareQuery = useQuery({
    queryKey: ["admin", "deployments", id, "compare", diffFrom, diffTo],
    queryFn: () =>
      fetchJSON<RevisionDiff>(`/admin/deployments/${id}/compare?from=${diffFrom}&to=${diffTo}`),
    enabled: false,
  });

  const rollbackMutation = useMutation({
    mutationFn: (revisionId: string) =>
      postJSON(`/admin/deployments/${id}/revisions/${revisionId}/rollback`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id, "revisions"] });
      queryClient.invalidateQueries({ queryKey: ["admin", "deployments", id] });
    },
  });

  const revisions = revsQuery.data ?? [];
  const activeRevision = revisions.find((r) => r.status === "active");

  const handleCompare = async (from: string, to: string) => {
    try {
      const data = await fetchJSON<RevisionDiff>(
        `/admin/deployments/${id}/compare?from=${from}&to=${to}`
      );
      setDiffData(data);
      setShowDiff(true);
    } catch {}
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Btn tone="ghost" onClick={() => router.push(`/admin/deployments/${id}`)}>
          <ArrowLeft size={14} /> Back to Deployment
        </Btn>
        <SectionHeader title="Revision History" sub="View, compare, and roll back deployment revisions." />
      </div>

      {activeRevision && (
        <Card className="p-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-xs uppercase tracking-wider text-slate-500 mb-1">Active Revision</p>
              <div className="flex items-center gap-3">
                <span className="font-mono text-lg font-semibold text-emerald-300">
                  Rev #{activeRevision.revisionNumber}
                </span>
                <Pill tone="green">active</Pill>
              </div>
              <p className="text-xs text-slate-400 mt-1">{activeRevision.imageRef}</p>
            </div>
            <div className="flex gap-2">
              {revisions.length > 1 && (
                <Btn
                  tone="warning"
                  onClick={() => {
                    const activeIdx = revisions.findIndex((r) => r.id === activeRevision.id);
                    if (activeIdx < revisions.length - 1) {
                      rollbackMutation.mutate(revisions[activeIdx + 1].id);
                    }
                  }}
                  disabled={rollbackMutation.isPending}
                >
                  <RotateCcw size={14} /> Rollback to Previous
                </Btn>
              )}
            </div>
          </div>
        </Card>
      )}

      <Card>
        <CardHeader
          title="Revisions"
          icon={History}
          action={
            revisions.length >= 2 && (
              <Btn tone="ghost" size="sm" onClick={() => setShowDiff(!showDiff)}>
                <ArrowRightLeft size={14} /> {showDiff ? "Hide Diff" : "Compare"}
              </Btn>
            )
          }
        />

        {showDiff && revisions.length >= 2 && (
          <div className="border-b border-white/[0.06] p-4">
            <div className="flex items-center gap-3 mb-3">
              <select
                className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
                value={diffFrom}
                onChange={(e) => setDiffFrom(e.target.value)}
              >
                <option value="">Select from revision</option>
                {revisions.map((r) => (
                  <option key={r.id} value={r.id}>
                    Rev #{r.revisionNumber} — {r.description}
                  </option>
                ))}
              </select>
              <ArrowRightLeft size={14} className="text-slate-500" />
              <select
                className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-xs text-slate-300 outline-none"
                value={diffTo}
                onChange={(e) => setDiffTo(e.target.value)}
              >
                <option value="">Select to revision</option>
                {revisions.map((r) => (
                  <option key={r.id} value={r.id}>
                    Rev #{r.revisionNumber} — {r.description}
                  </option>
                ))}
              </select>
              <Btn
                tone="primary"
                size="sm"
                disabled={!diffFrom || !diffTo}
                onClick={() => handleCompare(diffFrom, diffTo)}
              >
                <Search size={14} /> Compare
              </Btn>
            </div>
            {diffData && (
              <div className="space-y-2">
                <p className="text-xs font-medium text-slate-400">
                  Changes from Rev #{diffData.fromRevisionId} to Rev #{diffData.toRevisionId}
                </p>
                {diffData.changes.length === 0 ? (
                  <p className="text-xs text-slate-500">No changes detected.</p>
                ) : (
                  <div className="space-y-1">
                    {diffData.changes.map((ch, i) => (
                      <div key={i} className="flex items-start gap-3 rounded bg-white/[0.03] p-3 text-xs">
                        <span className="shrink-0 font-mono font-semibold text-indigo-300">{ch.field}</span>
                        <div className="space-y-0.5">
                          <p className="text-red-400 line-through">{ch.oldValue || "(empty)"}</p>
                          <p className="text-emerald-400">{ch.newValue || "(empty)"}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {revsQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading revisions...</div>
        ) : revisions.length === 0 ? (
          <EmptyState icon={GitCommit} message="No revisions recorded yet." />
        ) : (
          <div className="relative pl-8 pr-4 py-4">
            <div className="absolute left-4 top-0 bottom-0 w-px bg-white/[0.06]" />
            {revisions.map((rev, idx) => {
              const cfg = statusConfig[rev.status] ?? statusConfig.pending;
              const isActive = rev.status === "active";
              const canRollback = !isActive && idx > 0;
              return (
                <div key={rev.id} className="relative pb-5 last:pb-0">
                  <div
                    className={cn(
                      "absolute -left-[19px] mt-1 h-3 w-3 rounded-full border-2",
                      isActive ? "border-emerald-500 bg-emerald-500/20" : "border-slate-600 bg-[#1e2536]"
                    )}
                  />
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-slate-200">
                          Rev #{rev.revisionNumber}
                        </span>
                        <Pill tone={cfg.tone}>{rev.status}</Pill>
                        {rev.gitCommitSha && (
                          <span className="flex items-center gap-1 text-[10px] font-mono text-slate-500">
                            <GitCommit size={10} />
                            {rev.gitCommitSha.slice(0, 7)}
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-slate-400 mt-0.5">{rev.description}</p>
                      <div className="flex flex-wrap items-center gap-2 mt-1">
                        {rev.imageRef && (
                          <span className="flex items-center gap-1 text-[10px] text-slate-500">
                            <Package size={10} />
                            {rev.imageRef}
                          </span>
                        )}
                        <span className="flex items-center gap-1 text-[10px] font-mono text-slate-600">
                          <Layers size={10} />
                          {rev.configHash}
                        </span>
                      </div>
                      <p className="text-[10px] text-slate-600 mt-1">
                        {new Date(rev.createdAt).toLocaleString()}
                      </p>
                    </div>
                    <div className="flex gap-2 shrink-0">
                      {canRollback && (
                        <Btn
                          size="sm"
                          tone="warning"
                          onClick={() => rollbackMutation.mutate(rev.id)}
                          disabled={rollbackMutation.isPending}
                        >
                          <RotateCcw size={12} /> Rollback
                        </Btn>
                      )}
                      {revisions.length >= 2 && idx < revisions.length - 1 && (
                        <Btn
                          size="sm"
                          tone="ghost"
                          onClick={() => handleCompare(revisions[idx + 1].id, rev.id)}
                        >
                          <ArrowRightLeft size={12} /> Diff
                        </Btn>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Card>

      {rollbackMutation.isPending && (
        <div className="flex items-center gap-2 p-3 rounded-lg border border-amber-700/30 bg-amber-900/10 text-sm text-amber-300">
          <ShieldAlert size={14} />
          <span>Rolling back...</span>
          <RefreshCw size={14} className="animate-spin" />
        </div>
      )}
    </div>
  );
}
