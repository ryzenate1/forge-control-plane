"use client";

import { useState } from "react";
import { RotateCcw, AlertTriangle, Camera } from "lucide-react";
import { Btn } from "@/components/admin/admin-ui";
import { cn } from "@/lib/utils";

export interface RollbackChange {
  field: string;
  oldValue: string;
  newValue: string;
}

interface RollbackConfirmProps {
  revisionNumber: number;
  changes: RollbackChange[];
  estimatedDowntime: string;
  onConfirm: (createSnapshot: boolean) => void;
  onCancel: () => void;
  loading?: boolean;
}

export function RollbackConfirm({
  revisionNumber,
  changes,
  estimatedDowntime,
  onConfirm,
  onCancel,
  loading = false,
}: RollbackConfirmProps) {
  const [createSnapshot, setCreateSnapshot] = useState(true);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-xl border border-white/10 bg-slate-900 p-6 shadow-2xl">
        <div className="mb-4 flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-500/10">
            <RotateCcw className="h-5 w-5 text-amber-400" />
          </div>
          <div>
            <h2 className="text-lg font-bold text-white">Rollback to Revision #{revisionNumber}</h2>
            <p className="text-sm text-slate-400">Review the changes before rolling back</p>
          </div>
        </div>

        <div className="mb-4 rounded-lg border border-amber-500/20 bg-amber-950/10 p-3">
          <div className="flex items-start gap-2">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
            <div>
              <p className="text-sm font-medium text-amber-300">Expected Downtime: {estimatedDowntime}</p>
              <p className="text-xs text-amber-400/60">
                The service may be briefly unavailable during the rollback process.
              </p>
            </div>
          </div>
        </div>

        {changes.length > 0 && (
          <div className="mb-4">
            <p className="mb-2 text-xs font-semibold text-slate-400">Changes that will be applied:</p>
            <div className="space-y-2">
              {changes.map((change, i) => (
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
          </div>
        )}

        <label className="mb-6 flex items-center gap-3 rounded-lg border border-white/10 bg-white/[0.02] p-3">
          <input
            type="checkbox"
            checked={createSnapshot}
            onChange={(e) => setCreateSnapshot(e.target.checked)}
            className="h-4 w-4 rounded border-white/20 bg-[#161b28] accent-[#dc2626]"
          />
          <div>
            <div className="flex items-center gap-1.5 text-sm font-medium text-slate-200">
              <Camera className="h-3.5 w-3.5" />
              Create pre-rollback snapshot
            </div>
            <p className="text-xs text-slate-500">
              Take a backup before applying changes so you can restore if needed.
            </p>
          </div>
        </label>

        <div className="flex justify-end gap-3">
          <Btn tone="ghost" onClick={onCancel} disabled={loading}>
            Cancel
          </Btn>
          <Btn
            tone="warning"
            onClick={() => onConfirm(createSnapshot)}
            disabled={loading}
          >
            <RotateCcw className={cn("mr-1.5 h-4 w-4", loading && "animate-spin")} />
            {loading ? "Rolling Back..." : `Rollback to #${revisionNumber}`}
          </Btn>
        </div>
      </div>
    </div>
  );
}
