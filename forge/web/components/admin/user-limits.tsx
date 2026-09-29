"use client";

import { useId, useState } from "react";

/** A limit the account record did not carry. `null` is not 0: 0 means "no cap",
 * `null` means "the API sent nothing", and writing 0 back would silently remove
 * a limit the panel never read. */
export type LimitValue = number | null;

export type PasswordPolicy = {
  minLength: number;
  requirements: Array<{ label: string; metBy: (value: string) => boolean }>;
};

/** LimitField is a labelled numeric input for one per-user resource limit. */
function LimitField({ id, label, hint, value, onChange }: { id: string; label: string; hint?: string; value: LimitValue; onChange: (v: LimitValue) => void }) {
  return (
    <div>
      <label className="ui-label mb-1.5 block" htmlFor={id}>{label}</label>
      <input
        id={id}
        type="number"
        min={0}
        step={1}
        inputMode="numeric"
        value={value === null ? "" : value}
        placeholder={value === null ? "Not reported" : undefined}
        aria-describedby={hint ? `${id}-hint` : undefined}
        onChange={(event) => {
          const raw = event.target.value;
          if (raw === "") { onChange(null); return; }
          const parsed = Number.parseInt(raw, 10);
          onChange(Number.isFinite(parsed) ? Math.max(0, parsed) : null);
        }}
        className="ui-input"
      />
      {hint ? <p className="ui-hint mt-1" id={`${id}-hint`}>{hint}</p> : null}
    </div>
  );
}

// UserLimitsGrid renders a 3-column grid of limit fields. The first column is
// used for "count" caps (server/backup/database/allocation/subuser/schedule);
// the second for resource caps (CPU, memory, disk).
export function UserLimitsGrid(props: {
  serverLimit: LimitValue; onServerLimit: (v: LimitValue) => void;
  cpuLimit: LimitValue; onCpuLimit: (v: LimitValue) => void;
  memLimit: LimitValue; onMemLimit: (v: LimitValue) => void;
  diskLimit: LimitValue; onDiskLimit: (v: LimitValue) => void;
  backupLimit: LimitValue; onBackupLimit: (v: LimitValue) => void;
  databaseLimit: LimitValue; onDatabaseLimit: (v: LimitValue) => void;
  allocationLimit: LimitValue; onAllocationLimit: (v: LimitValue) => void;
  subuserLimit: LimitValue; onSubuserLimit: (v: LimitValue) => void;
  scheduleLimit: LimitValue; onScheduleLimit: (v: LimitValue) => void;
}) {
  const [open, setOpen] = useState(false);
  const baseId = useId();
  const panelId = `${baseId}-panel`;
  const values: Array<[string, LimitValue]> = [
    ["Servers", props.serverLimit],
    ["CPU", props.cpuLimit],
    ["Memory", props.memLimit],
    ["Disk", props.diskLimit],
    ["Backups", props.backupLimit],
    ["Databases", props.databaseLimit],
    ["Allocations", props.allocationLimit],
    ["Subusers", props.subuserLimit],
    ["Schedules", props.scheduleLimit],
  ];
  const unread = values.filter(([, v]) => v === null).map(([label]) => label);

  return (
    <div className="rounded-lg border border-line bg-surface">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between gap-3 px-4 py-2.5 text-left text-sm font-medium text-text transition-colors hover:bg-overlay-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--focus)]"
      >
        <span>Resource Limits <span className="t-meta ml-1">(0 = no cap)</span></span>
        <span className="t-meta shrink-0">{unread.length > 0 ? `${unread.length} not reported` : open ? "Collapse" : "Expand"}</span>
      </button>
      {open ? (
        <div className="grid gap-3 border-t border-line p-4 md:grid-cols-3" id={panelId}>
          {unread.length > 0 ? (
            <p className="ui-hint md:col-span-3">
              {unread.join(", ")} {unread.length === 1 ? "was" : "were"} not present on the account record. Leaving {unread.length === 1 ? "it" : "them"} blank keeps whatever is stored; typing 0 sets no cap.
            </p>
          ) : null}
          <LimitField id={`${baseId}-server`} label="Server count" hint="Max servers this user can own" value={props.serverLimit} onChange={props.onServerLimit} />
          <LimitField id={`${baseId}-cpu`} label="CPU %" hint="Aggregate CPU limit across all servers" value={props.cpuLimit} onChange={props.onCpuLimit} />
          <LimitField id={`${baseId}-mem`} label="Memory (MB)" hint="Aggregate memory limit" value={props.memLimit} onChange={props.onMemLimit} />
          <LimitField id={`${baseId}-disk`} label="Disk (MB)" hint="Aggregate disk limit" value={props.diskLimit} onChange={props.onDiskLimit} />
          <LimitField id={`${baseId}-backup`} label="Backups" hint="Max backup count across all servers" value={props.backupLimit} onChange={props.onBackupLimit} />
          <LimitField id={`${baseId}-database`} label="Databases" hint="Max database count across all servers" value={props.databaseLimit} onChange={props.onDatabaseLimit} />
          <LimitField id={`${baseId}-allocation`} label="Allocations" hint="Max additional allocations across all servers" value={props.allocationLimit} onChange={props.onAllocationLimit} />
          <LimitField id={`${baseId}-subuser`} label="Subusers" hint="Max subusers this user can add to their servers" value={props.subuserLimit} onChange={props.onSubuserLimit} />
          <LimitField id={`${baseId}-schedule`} label="Schedules" hint="Max schedule count across all servers" value={props.scheduleLimit} onChange={props.onScheduleLimit} />
        </div>
      ) : null}
    </div>
  );
}
