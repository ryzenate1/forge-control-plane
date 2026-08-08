"use client";

import { useState } from "react";

// LimitField is a labelled numeric input that lets the user set a per-user
// resource limit (0 = unlimited).
function LimitField({ label, hint, value, onChange }: { label: string; hint?: string; value: number; onChange: (v: number) => void }) {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-slate-300">{label}</label>
      <input
        type="number"
        min={0}
        value={value}
        onChange={(e) => onChange(Math.max(0, parseInt(e.target.value || "0", 10)))}
        className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 focus:border-[#dc2626] focus:outline-none"
      />
      {hint && <p className="mt-1 text-[10px] text-slate-500">{hint}</p>}
    </div>
  );
}

// UserLimitsGrid renders a 3-column grid of limit fields. The first column is
// used for "count" caps (server/backup/database/allocation/subuser/schedule);
// the second for resource caps (CPU, memory, disk).
export function UserLimitsGrid(props: {
  serverLimit: number; onServerLimit: (v: number) => void;
  cpuLimit: number; onCpuLimit: (v: number) => void;
  memLimit: number; onMemLimit: (v: number) => void;
  diskLimit: number; onDiskLimit: (v: number) => void;
  backupLimit: number; onBackupLimit: (v: number) => void;
  databaseLimit: number; onDatabaseLimit: (v: number) => void;
  allocationLimit: number; onAllocationLimit: (v: number) => void;
  subuserLimit: number; onSubuserLimit: (v: number) => void;
  scheduleLimit: number; onScheduleLimit: (v: number) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="rounded-lg border border-white/[0.06] bg-[#0f1419]">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between px-4 py-2.5 text-left text-sm font-medium text-slate-200 hover:bg-white/[0.02]"
      >
        <span>Resource Limits <span className="ml-1 text-xs text-slate-500">(0 = unlimited)</span></span>
        <span className="text-xs text-slate-500">{open ? "▾" : "▸"}</span>
      </button>
      {open && (
        <div className="grid gap-3 border-t border-white/[0.06] p-4 md:grid-cols-3">
          <LimitField label="Server count" hint="Max servers this user can own" value={props.serverLimit} onChange={props.onServerLimit} />
          <LimitField label="CPU %" hint="Aggregate CPU limit across all servers" value={props.cpuLimit} onChange={props.onCpuLimit} />
          <LimitField label="Memory (MB)" hint="Aggregate memory limit" value={props.memLimit} onChange={props.onMemLimit} />
          <LimitField label="Disk (MB)" hint="Aggregate disk limit" value={props.diskLimit} onChange={props.onDiskLimit} />
          <LimitField label="Backups" hint="Max backup count across all servers" value={props.backupLimit} onChange={props.onBackupLimit} />
          <LimitField label="Databases" hint="Max database count across all servers" value={props.databaseLimit} onChange={props.onDatabaseLimit} />
          <LimitField label="Allocations" hint="Max additional allocations across all servers" value={props.allocationLimit} onChange={props.onAllocationLimit} />
          <LimitField label="Subusers" hint="Max subusers this user can add to their servers" value={props.subuserLimit} onChange={props.onSubuserLimit} />
          <LimitField label="Schedules" hint="Max schedule count across all servers" value={props.scheduleLimit} onChange={props.onScheduleLimit} />
        </div>
      )}
    </div>
  );
}