"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { ChevronDown, ChevronRight, Clock, Pencil, Play, Plus, Terminal, ToggleLeft, ToggleRight, Trash2 } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminPageHeader,
  AdminPageLayout,
  AdminSelect,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Modal,
  ModalFooter,
  Pill,
  Textarea,
  AdminFormSection,
  AdminFormField,
} from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { sourceState, useNodesQuery, useServersQuery } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";
import {
  fetchCronJobs,
  createCronJob,
  updateCronJob,
  deleteCronJob,
  triggerCronJob,
  toggleCronJob,
  fetchCronJobExecutions,
  type CronJob,
  type CreateCronJobInput,
} from "@/lib/api/cron-jobs";
import { deploymentStatusTone } from "@/lib/api/status";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { Switch } from "@/components/ui/primitives";

const JOBS_KEY = ["cron-jobs"];
const EXECUTIONS_KEY = ["cron-executions"];
const JOBS_POLL_MS = 30_000;
const EXECUTIONS_POLL_MS = 10_000;

function statusPill(status: string) {
  // This carried its own table with `running: "yellow"` — a job doing exactly
  // what it was scheduled to do, rendered as a warning — and fell back to
  // `neutral`, so a status we could not read showed as a settled "inactive".
  // deploymentStatusTone knows all four run states and yields `unknown` for
  // anything else.
  return <Pill tone={deploymentStatusTone(status)}>{status}</Pill>;
}

// ------------------------------------------------------------------
// Cron schedule validation
//
// Both layers used to check only "are there five whitespace-separated fields",
// so an out-of-range minute, day-of-week 8, or a zero step saved successfully
// and then never ran: the server's scheduler (robfig/cron, 5-field parser)
// refuses those specs, which leaves the job stored in the database with no
// scheduler entry and therefore no `nextRun`. The form now validates every
// field against the ranges the scheduler uses and previews what the expression
// will actually match.
// ------------------------------------------------------------------

type CronField = {
  name: string;
  min: number;
  max: number;
  names?: Record<string, number>;
};

const CRON_FIELDS: CronField[] = [
  { name: "minute", min: 0, max: 59 },
  { name: "hour", min: 0, max: 23 },
  { name: "day of month", min: 1, max: 31 },
  { name: "month", min: 1, max: 12, names: { jan: 1, feb: 2, mar: 3, apr: 4, may: 5, jun: 6, jul: 7, aug: 8, sep: 9, oct: 10, nov: 11, dec: 12 } },
  { name: "day of week", min: 0, max: 6, names: { sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6 } },
];

type ParsedField = { values: Set<number>; restricted: boolean };

function resolveValue(token: string, field: CronField): number | null {
  const asNumber = Number(token);
  if (Number.isInteger(asNumber)) {
    return asNumber >= field.min && asNumber <= field.max ? asNumber : null;
  }
  if (!field.names) return null;
  const key = token.slice(0, 3).toLowerCase();
  return Object.prototype.hasOwnProperty.call(field.names, key) ? field.names[key] : null;
}

/** Parse one cron field into the set of values it matches, or an error string. */
function parseCronField(raw: string, field: CronField): ParsedField | string {
  const values = new Set<number>();
  let restricted = false;

  for (const part of raw.split(",")) {
    if (part === "") return `${field.name}: empty list item`;
    const [rangePart, stepPart] = part.split("/");
    if (part.split("/").length > 2) return `${field.name}: “${part}” has more than one step`;

    let step = 1;
    if (stepPart !== undefined) {
      step = Number(stepPart);
      if (!Number.isInteger(step) || step <= 0) return `${field.name}: step “${stepPart}” must be a whole number greater than 0`;
    }

    if (rangePart === "*") {
      if (stepPart !== undefined) restricted = true;
      for (let v = field.min; v <= field.max; v += step) values.add(v);
      continue;
    }

    restricted = true;
    const bounds = rangePart.split("-");
    if (bounds.length === 1) {
      const single = resolveValue(bounds[0], field);
      if (single === null) return `${field.name}: “${bounds[0]}” is outside ${field.min}-${field.max}`;
      if (stepPart !== undefined) {
        for (let v = single; v <= field.max; v += step) values.add(v);
      } else {
        values.add(single);
      }
      continue;
    }
    if (bounds.length !== 2) return `${field.name}: “${rangePart}” is not a valid range`;
    const from = resolveValue(bounds[0], field);
    const to = resolveValue(bounds[1], field);
    if (from === null || to === null) return `${field.name}: “${rangePart}” is outside ${field.min}-${field.max}`;
    if (from > to) return `${field.name}: range “${rangePart}” counts backwards`;
    for (let v = from; v <= to; v += step) values.add(v);
  }

  if (values.size === 0) return `${field.name}: matches nothing`;
  return { values, restricted };
}

/** Error message, or null when every field parses. */
function validateCronSchedule(schedule: string): string | null {
  const trimmed = schedule.trim();
  if (!trimmed) return "Cron expression is required";
  const parts = trimmed.split(/\s+/);
  if (parts.length !== 5) {
    return "Cron expression must have exactly 5 fields (minute hour day-of-month month day-of-week)";
  }
  // Mirrors validateCronSchedule in handlers_cronjob.go: a `* *` schedule would
  // fire every minute, which the control plane refuses.
  if (parts[0] === "*" && parts[1] === "*") {
    return "Every-minute schedules are rejected by the control plane — the minimum interval is 1 minute";
  }
  for (let i = 0; i < 5; i += 1) {
    const parsed = parseCronField(parts[i], CRON_FIELDS[i]);
    if (typeof parsed === "string") return parsed;
  }
  return null;
}

function parseSchedule(schedule: string): ParsedField[] | null {
  const parts = schedule.trim().split(/\s+/);
  if (parts.length !== 5) return null;
  const fields: ParsedField[] = [];
  for (let i = 0; i < 5; i += 1) {
    const parsed = parseCronField(parts[i], CRON_FIELDS[i]);
    if (typeof parsed === "string") return null;
    fields.push(parsed);
  }
  return fields;
}

/**
 * The next `count` times this expression will fire, evaluated in the browser's
 * own clock and time zone. Labelled as computed, because the authoritative value
 * is the scheduler's `nextRun`, which only exists once the job is registered.
 */
function computeNextRuns(schedule: string, count = 3, from = new Date()): string[] {
  const fields = parseSchedule(schedule);
  if (!fields) return [];
  const [minute, hour, dayOfMonth, month, dayOfWeek] = fields;
  const domRestricted = dayOfMonth.restricted;
  const dowRestricted = dayOfWeek.restricted;

  const cursor = new Date(from);
  cursor.setSeconds(0, 0);
  cursor.setMinutes(cursor.getMinutes() + 1);

  const runs: string[] = [];
  // Three years of minutes is the practical bound: anything past it is a
  // schedule so rare the preview is not worth the wait.
  const limit = 3 * 366 * 24 * 60;
  for (let i = 0; i < limit && runs.length < count; i += 1) {
    const dayMatches = (() => {
      const domOk = dayOfMonth.values.has(cursor.getDate());
      const dowOk = dayOfWeek.values.has(cursor.getDay());
      if (domRestricted && dowRestricted) return domOk || dowOk; // standard cron OR rule
      if (domRestricted) return domOk;
      if (dowRestricted) return dowOk;
      return true;
    })();

    if (month.values.has(cursor.getMonth() + 1) && dayMatches && hour.values.has(cursor.getHours()) && minute.values.has(cursor.getMinutes())) {
      runs.push(formatDate(cursor));
    }
    cursor.setMinutes(cursor.getMinutes() + 1);
  }
  return runs;
}

const presetSchedules = [
  { label: "Every 5 min", value: "*/5 * * * *" },
  { label: "Hourly", value: "0 * * * *" },
  { label: "Daily", value: "0 0 * * *" },
  { label: "Weekly", value: "0 0 * * 0" },
];

const fieldClass = "ui-input w-full";

function CronForm({ job, onClose }: { job?: CronJob; onClose: () => void }) {
  const [name, setName] = useState(job?.name ?? "");
  const [description, setDescription] = useState(job?.description ?? "");
  const [schedule, setSchedule] = useState(job?.schedule ?? "0 * * * *");
  const [command, setCommand] = useState(job?.command ?? "");
  const [type, setType] = useState(job?.type ?? "shell");
  const [targetType, setTargetType] = useState(job?.targetType ?? "");
  const [targetId, setTargetId] = useState(job?.targetId ?? "");
  const [timeoutSeconds, setTimeoutSeconds] = useState(String(job?.timeoutSeconds ?? 300));
  const [retryCount, setRetryCount] = useState(String(job?.retryCount ?? 0));
  const [enabled, setEnabled] = useState(job?.enabled ?? true);
  const [notifyOnFailure, setNotifyOnFailure] = useState(job?.notifyOnFailure ?? false);

  const [errors, setErrors] = useState<Record<string, string>>({});

  const queryClient = useQueryClient();
  const { toast } = useToast();
  const nodesQuery = useNodesQuery();
  const serversQuery = useServersQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);

  const createMut = useMutation({
    mutationFn: (input: CreateCronJobInput) => createCronJob(input),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: JOBS_KEY }); toast({ tone: "success", title: "Cron job created" }); onClose(); },
    onError: (err) => toast({ tone: "error", title: "Failed to create cron job", message: err instanceof Error ? err.message : "An error occurred" }),
  });
  const updateMut = useMutation({
    mutationFn: (input: CreateCronJobInput) => job ? updateCronJob(job.id, input) : Promise.reject(new Error("No job to update")),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: JOBS_KEY }); toast({ tone: "success", title: "Cron job updated" }); onClose(); },
    onError: (err) => toast({ tone: "error", title: "Failed to update cron job", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  function validate(): Record<string, string> {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required";
    const scheduleErr = validateCronSchedule(schedule);
    if (scheduleErr) errs.schedule = scheduleErr;
    if (!command.trim()) errs.command = "Command is required";
    if (targetType.trim() && !targetId.trim()) errs.targetId = "A target type needs a target ID";
    const timeout = Number.parseInt(timeoutSeconds, 10);
    if (!Number.isFinite(timeout) || timeout < 1 || timeout > 86400) errs.timeoutSeconds = "Must be between 1 and 86400";
    const retry = Number.parseInt(retryCount, 10);
    if (!Number.isFinite(retry) || retry < 0 || retry > 10) errs.retryCount = "Must be between 0 and 10";
    return errs;
  }

  const handleSave = () => {
    const errs = validate();
    setErrors(errs);
    if (Object.keys(errs).length > 0) return;

    const input: CreateCronJobInput = {
      name: name.trim(),
      description: description.trim() || undefined,
      schedule: schedule.trim(),
      command: command.trim(),
      type,
      targetType: targetType.trim() || undefined,
      targetId: targetId.trim() || undefined,
      enabled,
      retryCount: Number.parseInt(retryCount, 10),
      timeoutSeconds: Number.parseInt(timeoutSeconds, 10),
      notifyOnFailure,
    };
    if (job) {
      updateMut.mutate(input);
    } else {
      createMut.mutate(input);
    }
  };

  const scheduleError = errors.schedule;
  // Only preview a schedule that actually parses; an invalid one gets the reason.
  const preview = !scheduleError && schedule.trim() ? computeNextRuns(schedule) : [];

  const targetOptions = targetType === "node"
    ? nodes.map((node) => ({ value: node.id, label: `${node.name} · ${node.id.slice(0, 8)}` }))
    : targetType === "server"
      ? servers.map((server) => ({ value: server.id, label: `${server.name} · ${server.id.slice(0, 8)}` }))
      : [];
  if (targetId && !targetOptions.some((option) => option.value === targetId)) {
    targetOptions.unshift({ value: targetId, label: `${targetId.slice(0, 8)} · not in the current inventory` });
  }

  return (
    <div className="space-y-4">
      <AdminFormSection title="Identity">
        <AdminFormField label="Name" error={errors.name}>
          <input className={fieldClass} onChange={(e) => setName(e.target.value)} placeholder="My cron job" type="text" value={name} />
        </AdminFormField>
        <AdminFormField label="Description">
          <input className={fieldClass} onChange={(e) => setDescription(e.target.value)} placeholder="Optional description" type="text" value={description} />
        </AdminFormField>
        <AdminFormField label="Enabled">
          <Switch checked={enabled} label={enabled ? "Enabled" : "Disabled"} onCheckedChange={setEnabled} />
        </AdminFormField>
      </AdminFormSection>

      <AdminFormSection description="Five fields: minute hour day-of-month month day-of-week. Names (mon, jan) and steps (*/15) are accepted." title="Schedule">
        <div className="flex flex-wrap gap-1.5">
          {presetSchedules.map((preset) => (
            <button
              className={`rounded-lg border px-2.5 py-1 text-meta transition ${schedule === preset.value ? "border-brand-line bg-brand-subtle text-brand" : "border-line bg-overlay-subtle text-text-subtle hover:text-text"}`}
              key={preset.value}
              onClick={() => { setSchedule(preset.value); setErrors((prev) => { const rest = { ...prev }; delete rest.schedule; return rest; }); }}
              type="button"
            >
              {preset.label}
            </button>
          ))}
        </div>
        <AdminFormField error={scheduleError} hint="Standard 5-field cron syntax (min hour day month weekday)" label="Cron expression">
          <input
            aria-label="Cron expression"
            className={`${fieldClass} font-mono text-xs`}
            onChange={(e) => { setSchedule(e.target.value); if (e.target.value) setErrors((prev) => { const rest = { ...prev }; delete rest.schedule; return rest; }); }}
            placeholder="*/5 * * * *"
            type="text"
            value={schedule}
          />
        </AdminFormField>
        {/* The old preview asserted "on a custom schedule" for any 5-field string,
            including unschedulable ones. This states only what was computed. */}
        {preview.length > 0 ? (
          <div className="rounded-lg border border-line bg-overlay-subtle p-2.5 text-xs leading-5">
            <p className="font-semibold text-text">Next runs this expression matches</p>
            <ul className="mt-1 space-y-0.5 font-mono text-text-subtle">
              {preview.map((run) => <li key={run}>{run}</li>)}
            </ul>
            <p className="mt-1 text-text-muted">Computed in this browser&apos;s time zone. Once the job is saved, the row shows the scheduler&apos;s own next run.</p>
          </div>
        ) : null}
      </AdminFormSection>

      <AdminFormSection title="Execution">
        <AdminFormField error={errors.command} label="Command">
          <Textarea value={command} onChange={setCommand} rows={3} placeholder="bash command" />
        </AdminFormField>
        <div className="grid grid-cols-2 gap-4">
          <AdminFormField label="Type">
            <AdminSelect value={type} onChange={setType} options={[{ value: "shell", label: "Shell" }, { value: "script", label: "Script" }]} />
          </AdminFormField>
          <AdminFormField label="Target type">
            <AdminSelect
              value={targetType}
              onChange={(v) => { setTargetType(v); setTargetId(""); }}
              options={[
                { value: "", label: "No specific target" },
                { value: "server", label: "Server" },
                { value: "node", label: "Node" },
              ]}
            />
          </AdminFormField>
        </div>
        {targetType ? (
          <AdminFormField error={errors.targetId} label={`Target ${targetType === "node" ? "node" : "server"}`}>
            <AdminSelect
              mono
              value={targetId}
              onChange={setTargetId}
              options={targetOptions}
              placeholder={
                (targetType === "node" ? nodesQuery.isLoading : serversQuery.isLoading)
                  ? "Loading inventory…"
                  : `Choose a ${targetType === "node" ? "node" : "server"}…`
              }
            />
          </AdminFormField>
        ) : (
          <p className="text-xs leading-5 text-text-muted">
            This job runs wherever the scheduler executes it. Pick a target only when the command must run on
            one specific node or server.
          </p>
        )}
      </AdminFormSection>

      <AdminFormSection title="Reliability">
        <div className="grid grid-cols-2 gap-4">
          <AdminFormField error={errors.timeoutSeconds} label="Timeout (seconds)" hint="Min 1, max 86400 (24h)">
            <input
              className={fieldClass}
              max={86400}
              min={1}
              onChange={(e) => { setTimeoutSeconds(e.target.value); setErrors((prev) => { const rest = { ...prev }; delete rest.timeoutSeconds; return rest; }); }}
              type="number"
              value={timeoutSeconds}
            />
          </AdminFormField>
          <AdminFormField error={errors.retryCount} label="Retry count" hint="0-10 automatic retries on failure">
            <input
              className={fieldClass}
              max={10}
              min={0}
              onChange={(e) => { setRetryCount(e.target.value); setErrors((prev) => { const rest = { ...prev }; delete rest.retryCount; return rest; }); }}
              type="number"
              value={retryCount}
            />
          </AdminFormField>
        </div>
        <AdminFormField label="Notify on failure">
          <Switch checked={notifyOnFailure} label={notifyOnFailure ? "On" : "Off"} onCheckedChange={setNotifyOnFailure} />
        </AdminFormField>
      </AdminFormSection>

      <ModalFooter
        onCancel={onClose}
        onConfirm={handleSave}
        disabled={createMut.isPending || updateMut.isPending}
        confirmLabel={createMut.isPending || updateMut.isPending ? "Saving…" : job ? "Update" : "Create"}
      />
    </div>
  );
}

function ExecutionLog({ jobId }: { jobId: string }) {
  const [expandedOutputs, setExpandedOutputs] = useState<Set<string>>(new Set());

  const executionsQuery = useQuery({
    queryKey: [...EXECUTIONS_KEY, jobId],
    queryFn: () => fetchCronJobExecutions(jobId, 20),
    refetchInterval: EXECUTIONS_POLL_MS,
  });
  const { isLoading, isError, error, refetch } = executionsQuery;

  function toggleOutput(id: string) {
    setExpandedOutputs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  const rows = executionsQuery.data ?? [];
  const latest = rows[0];

  return (
    <Card>
      <CardHeader
        icon={Terminal}
        title="Execution history"
        action={<FreshnessBadge state={sourceState(executionsQuery, EXECUTIONS_POLL_MS)} />}
      />
      {isLoading ? (
        <AdminLoadingRows cols={3} rows={2} label="Loading execution history" />
      ) : isError ? (
        <div className="p-4">
          <AdminErrorState message={`Execution history could not be read: ${error instanceof Error ? error.message : "unknown error"}`} retry={() => void refetch()} />
        </div>
      ) : rows.length === 0 ? (
        <EmptyState icon={Clock} message="This job has no recorded execution yet." title="No executions recorded" />
      ) : (
        <>
          <p className="px-4 pb-2 text-xs leading-5 text-text-subtle">
            {latest.finishedAt
              ? `Last run ${formatDate(latest.finishedAt)} — ${latest.status}${typeof latest.exitCode === "number" ? ` (exit ${latest.exitCode})` : ""}.`
              : `Last run started ${formatDate(latest.startedAt)} and is still ${latest.status}.`}
          </p>
          <div className="divide-y divide-line">
            {rows.map((exec) => {
              const showFull = expandedOutputs.has(exec.id);
              const hasOutput = Boolean(exec.output || exec.error);
              return (
                <div className="px-4 py-3" key={exec.id}>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex flex-wrap items-center gap-2">
                      {statusPill(exec.status)}
                      <span className="text-xs text-text-subtle">started {formatDate(exec.startedAt)}</span>
                      <span className="text-xs text-text-muted">
                        {exec.finishedAt ? `finished ${formatDate(exec.finishedAt)}` : "no finish time reported"}
                      </span>
                      {typeof exec.exitCode === "number" ? (
                        <span className="font-mono text-xs text-text-muted">exit {exec.exitCode}</span>
                      ) : (
                        <span className="text-xs text-text-muted">exit code not reported</span>
                      )}
                    </div>
                    {typeof exec.durationMs === "number" ? (
                      <span className="font-mono text-xs text-text-muted">{exec.durationMs} ms</span>
                    ) : null}
                  </div>
                  {hasOutput ? (
                    <div className="mt-2">
                      <button
                        aria-expanded={showFull}
                        className="inline-flex items-center gap-1 text-xs text-text-subtle hover:text-text"
                        onClick={() => toggleOutput(exec.id)}
                        type="button"
                      >
                        {showFull ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                        {showFull ? "Collapse output" : "Show full output"}
                      </button>
                      {showFull ? (
                        <>
                          {exec.output ? (
                            <pre className="mt-1 max-h-64 overflow-auto rounded-lg border border-line bg-overlay-subtle p-2 text-xs leading-5 text-text-subtle">{exec.output}</pre>
                          ) : null}
                          {exec.error ? (
                            <pre className="mt-1 max-h-64 overflow-auto rounded-lg border border-danger-line bg-danger-subtle p-2 text-xs leading-5 text-danger">{exec.error}</pre>
                          ) : null}
                        </>
                      ) : null}
                    </div>
                  ) : (
                    <p className="mt-1 text-xs text-text-muted">No output or error captured.</p>
                  )}
                </div>
              );
            })}
          </div>
        </>
      )}
    </Card>
  );
}

/** Row actions are icon-only, so each one carries an accessible name. */
function RowButton({ label, onClick, children, tone = "default" }: {
  label: string;
  onClick: () => void;
  children: React.ReactNode;
  tone?: "default" | "danger";
}) {
  return (
    <button
      aria-label={label}
      className={`rounded-lg border border-transparent p-1.5 text-text-subtle transition hover:bg-overlay-subtle hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] ${tone === "danger" ? "hover:text-danger" : ""}`}
      onClick={(event) => { event.stopPropagation(); onClick(); }}
      title={label}
      type="button"
    >
      {children}
    </button>
  );
}

export default function AdminCronJobs() {
  const [confirm, renderConfirm] = useConfirm();
  const [showForm, setShowForm] = useState(false);
  const [editJob, setEditJob] = useState<CronJob | undefined>(undefined);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);

  const queryClient = useQueryClient();
  const { toast } = useToast();

  const jobsQuery = useQuery({
    queryKey: JOBS_KEY,
    queryFn: fetchCronJobs,
    refetchInterval: JOBS_POLL_MS,
  });
  const jobs = useMemo(() => Array.isArray(jobsQuery.data) ? jobsQuery.data : [], [jobsQuery.data]);

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteCronJob(id),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: JOBS_KEY }); toast({ tone: "success", title: "Cron job deleted" }); },
    onError: (err) => toast({ tone: "error", title: "Failed to delete cron job", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const toggleMut = useMutation({
    mutationFn: (id: string) => toggleCronJob(id),
    onSuccess: (job) => {
      queryClient.invalidateQueries({ queryKey: JOBS_KEY });
      toast({ tone: "success", title: job.enabled ? `${job.name} enabled` : `${job.name} disabled`, message: job.enabled ? "The scheduler will pick it up on its schedule." : "It will not run again until re-enabled." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to toggle cron job", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const triggerMut = useMutation({
    mutationFn: (id: string) => triggerCronJob(id),
    onSuccess: (_execution, id) => {
      const job = jobs.find((candidate) => candidate.id === id);
      // Invalidate the list too: the previous copy only refreshed an execution
      // key that matched nothing unless a row happened to be expanded, so
      // "Trigger now" could look like it did nothing at all.
      queryClient.invalidateQueries({ queryKey: JOBS_KEY });
      queryClient.invalidateQueries({ queryKey: EXECUTIONS_KEY });
      toast({ tone: "success", title: `${job?.name ?? "Job"} triggered`, message: "An execution has been started; its history refreshes below." });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to trigger cron job", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  return (
    <AdminPageLayout>
      <AdminPageHeader
        info={adminPageGuides.cronJobs}
        status={<FreshnessBadge state={sourceState(jobsQuery, JOBS_POLL_MS)} />}
        action={
          <Btn onClick={() => { setEditJob(undefined); setShowForm(true); }}>
            <Plus size={14} /> New job
          </Btn>
        }
      />

      {jobsQuery.isLoading ? (
        <Card>
          <CardHeader icon={Clock} title="Cron jobs" />
          <AdminLoadingRows cols={4} rows={4} label="Loading cron jobs" />
        </Card>
      ) : jobsQuery.isError ? (
        <Card>
          <CardHeader icon={Clock} title="Cron jobs" />
          <div className="p-4">
            {/* A failed read is not an empty fleet: this used to render
                "No cron jobs configured", hiding an outage as a clean slate. */}
            <AdminErrorState
              message={`Cron jobs could not be read: ${jobsQuery.error instanceof Error ? jobsQuery.error.message : "unknown error"}`}
              retry={() => void jobsQuery.refetch()}
            />
          </div>
        </Card>
      ) : jobs.length === 0 ? (
        <Card>
          <CardHeader icon={Clock} title="Cron jobs" />
          <EmptyState icon={Clock} message="No cron jobs are configured. Create one to automate a recurring command." title="No cron jobs" />
        </Card>
      ) : (
        <Card>
          <CardHeader
            action={<span className="text-xs text-text-muted">{jobs.length} job{jobs.length === 1 ? "" : "s"} · {jobs.filter((job) => job.enabled).length} enabled</span>}
            icon={Clock}
            title="Cron jobs"
          />
          <div className="divide-y divide-line">
            {jobs.map((job) => {
              const scheduleValid = validateCronSchedule(job.schedule) === null;
              const notScheduled = job.enabled && !job.nextRun;
              return (
                <div key={job.id}>
                  <div
                    className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 transition hover:bg-overlay-subtle"
                  >
                    <button
                      aria-controls={`cron-executions-${job.id}`}
                      aria-expanded={selectedJobId === job.id}
                      className="flex min-w-0 flex-1 items-center gap-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                      onClick={() => setSelectedJobId(selectedJobId === job.id ? null : job.id)}
                      type="button"
                    >
                      <span aria-hidden="true" className={`h-2 w-2 shrink-0 rounded-full ${job.enabled ? "bg-ok" : "bg-text-muted"}`} />
                      <span className="min-w-0">
                        <span className="block truncate text-sm font-medium text-text">{job.name}</span>
                        <span className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-text-muted">
                          <span className="font-mono">{job.schedule}</span>
                          <Pill tone="neutral">{job.type || "shell"}</Pill>
                          {job.targetType ? <span className="font-mono">{job.targetType}{job.targetId ? ` · ${job.targetId.slice(0, 8)}` : ""}</span> : null}
                        </span>
                      </span>
                    </button>
                    <div className="flex flex-wrap items-center gap-2">
                      {/* Next run comes from the scheduler. When it is missing for
                          an enabled job, that is a fault worth naming: either the
                          schedule was rejected or the entry was never registered. */}
                      {job.nextRun ? (
                        <span className="text-xs text-text-subtle">Next: {formatDate(job.nextRun)}</span>
                      ) : job.enabled ? (
                        <span className="max-w-[16rem] text-xs leading-4 text-warn">
                          {scheduleValid
                            ? "No next run — the scheduler has not registered this job"
                            : "Not scheduled — the expression is invalid; edit the job to fix it"}
                        </span>
                      ) : (
                        <span className="text-xs text-text-muted">Disabled — no next run</span>
                      )}
                      <RowButton label={job.enabled ? `Disable ${job.name}` : `Enable ${job.name}`} onClick={() => toggleMut.mutate(job.id)}>
                        {job.enabled ? <ToggleRight size={14} /> : <ToggleLeft size={14} />}
                      </RowButton>
                      <RowButton label={`Run ${job.name} now`} onClick={() => triggerMut.mutate(job.id)}>
                        <Play size={14} />
                      </RowButton>
                      <RowButton label={`Edit ${job.name}`} onClick={() => { setEditJob(job); setShowForm(true); }}>
                        <Pencil size={14} />
                      </RowButton>
                      <RowButton label={`Delete ${job.name}`} tone="danger" onClick={() => {
                        void (async () => {
                          if (await confirm({ title: `Delete cron job ${job.name}?`, description: "The job stops running on its schedule and its definition is removed. Execution history already recorded is kept. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(job.id);
                        })();
                      }}>
                        <Trash2 size={14} />
                      </RowButton>
                    </div>
                  </div>
                  {selectedJobId === job.id ? (
                    <div className="border-t border-line bg-overlay-subtle px-4 py-3" id={`cron-executions-${job.id}`}>
                      <ExecutionLog jobId={job.id} />
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        </Card>
      )}

      {showForm ? (
        <Modal description={editJob ? "Change what this job runs and when." : "Define a recurring command and its schedule."} onClose={() => setShowForm(false)} title={editJob ? "Edit cron job" : "Create cron job"} wide>
          <CronForm job={editJob} onClose={() => setShowForm(false)} />
        </Modal>
      ) : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
