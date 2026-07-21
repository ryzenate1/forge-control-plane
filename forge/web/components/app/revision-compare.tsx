"use client";

import { useState } from "react";
import { ArrowLeftRight, ArrowRight, GitCompare, RotateCcw } from "lucide-react";
import { cn } from "@/lib/utils";
import { compareRevisions, rollbackToRevision, fetchDeploymentRevisions } from "@/lib/api/deployments";
import type { Revision, RevisionDiff } from "@/lib/api/deployments";
import { Btn, Card, CardHeader } from "@/components/admin/admin-ui";

interface RevisionCompareProps {
  deploymentId: string;
  onRollback?: () => void;
}

export function RevisionCompare({ deploymentId, onRollback }: RevisionCompareProps) {
  const [revisions, setRevisions] = useState<Revision[]>([]);
  const [loading, setLoading] = useState(false);
  const [diff, setDiff] = useState<RevisionDiff | null>(null);
  const [fromRev, setFromRev] = useState<string>("");
  const [toRev, setToRev] = useState<string>("");
  const [rollingBack, setRollingBack] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  const loadRevisions = async () => {
    setLoading(true);
    try {
      const data = await fetchDeploymentRevisions(deploymentId);
      setRevisions(data);
      setLoaded(true);
      if (data.length >= 2) {
        setFromRev(data[data.length - 2].id);
        setToRev(data[data.length - 1].id);
      } else if (data.length === 1) {
        setFromRev(data[0].id);
        setToRev(data[0].id);
      }
    } catch {
      // ignore
    }
    setLoading(false);
  };

  const handleCompare = async () => {
    if (!fromRev || !toRev || fromRev === toRev) return;
    setLoading(true);
    try {
      const data = await compareRevisions(deploymentId, fromRev, toRev);
      setDiff(data);
    } catch {
      setDiff(null);
    }
    setLoading(false);
  };

  const handleRollback = async (revisionId: string) => {
    setRollingBack(revisionId);
    try {
      await rollbackToRevision(deploymentId, revisionId);
      onRollback?.();
    } catch {
      // ignore
    }
    setRollingBack(null);
  };

  const getRevNumber = (id: string) => revisions.find((r) => r.id === id)?.revisionNumber ?? "?";

  return (
    <Card>
      <CardHeader
        title="Revision Compare"
        icon={GitCompare}
        action={
          !loaded ? (
            <Btn size="sm" tone="ghost" onClick={loadRevisions} disabled={loading}>
              Load Revisions
            </Btn>
          ) : null
        }
      />
      <div className="space-y-4 p-4">
        {!loaded ? (
          <div className="py-4 text-center text-sm text-slate-500">
            <Btn size="sm" tone="primary" onClick={loadRevisions} disabled={loading}>
              {loading ? "Loading..." : "Load Revisions"}
            </Btn>
          </div>
        ) : revisions.length < 2 ? (
          <div className="py-4 text-center text-sm text-slate-500">
            {revisions.length === 0
              ? "No revisions available."
              : "At least two revisions are needed for comparison."}
          </div>
        ) : (
          <>
            <div className="flex items-end gap-3">
              <div className="flex-1">
                <label className="mb-1 block text-xs font-medium text-slate-400">From Revision</label>
                <select
                  value={fromRev}
                  onChange={(e) => setFromRev(e.target.value)}
                  className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white outline-none"
                >
                  {revisions.map((r) => (
                    <option key={r.id} value={r.id}>
                      #{r.revisionNumber} - {r.imageRef.slice(0, 30)} ({r.status})
                    </option>
                  ))}
                </select>
              </div>
              <div className="pb-2">
                <ArrowRight className="h-4 w-4 text-slate-500" />
              </div>
              <div className="flex-1">
                <label className="mb-1 block text-xs font-medium text-slate-400">To Revision</label>
                <select
                  value={toRev}
                  onChange={(e) => setToRev(e.target.value)}
                  className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white outline-none"
                >
                  {revisions.map((r) => (
                    <option key={r.id} value={r.id}>
                      #{r.revisionNumber} - {r.imageRef.slice(0, 30)} ({r.status})
                    </option>
                  ))}
                </select>
              </div>
              <Btn size="sm" onClick={handleCompare} disabled={loading || fromRev === toRev}>
                <ArrowLeftRight className="mr-1 h-3 w-3" />
                Compare
              </Btn>
            </div>

            {diff && (
              <div className="space-y-3">
                <div className="rounded-lg border border-white/10 bg-white/[0.02] p-3">
                  <p className="mb-2 text-xs font-semibold text-slate-400">
                    Comparing Revision #{diff.fromRevisionId} → #{diff.toRevisionId}
                  </p>
                  {diff.changes.length === 0 ? (
                    <p className="text-sm text-slate-500">No changes between these revisions.</p>
                  ) : (
                    <div className="space-y-2">
                      {diff.changes.map((change, i) => (
                        <div key={i} className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-2">
                          <p className="mb-1 text-xs font-medium text-slate-400">{change.field}</p>
                          <div className="grid grid-cols-2 gap-2 font-mono text-xs">
                            <div className="rounded bg-red-950/20 p-1.5 text-red-300 line-through">
                              {change.oldValue || "(empty)"}
                            </div>
                            <div className="rounded bg-emerald-950/20 p-1.5 text-emerald-300">
                              {change.newValue || "(empty)"}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div className="space-y-2">
                  <p className="text-xs font-semibold text-slate-400">Quick Rollback</p>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {revisions
                      .filter((r) => r.status === "active" || r.status === "superseded")
                      .slice(-4)
                      .map((rev) => (
                        <div
                          key={rev.id}
                          className={cn(
                            "flex items-center justify-between rounded-lg border border-white/10 bg-white/[0.02] p-3",
                            rev.status === "active" && "border-emerald-500/30",
                          )}
                        >
                          <div>
                            <p className="text-sm font-medium text-slate-200">#{rev.revisionNumber}</p>
                            <p className="font-mono text-xs text-slate-500">{rev.imageRef.slice(0, 35)}</p>
                            <p className="text-xs text-slate-500">{rev.configHash}</p>
                          </div>
                          {rev.status !== "active" && (
                            <Btn
                              size="sm"
                              tone="warning"
                              onClick={() => handleRollback(rev.id)}
                              disabled={rollingBack === rev.id}
                            >
                              <RotateCcw className={cn("mr-1 h-3 w-3", rollingBack === rev.id && "animate-spin")} />
                              Rollback
                            </Btn>
                          )}
                        </div>
                      ))}
                  </div>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </Card>
  );
}
