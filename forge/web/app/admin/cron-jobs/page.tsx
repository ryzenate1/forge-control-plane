"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, Clock, Play, Plus, RefreshCw, Terminal, ToggleLeft, ToggleRight, Trash2 } from "lucide-react";
import { Btn, Card, CardHeader, EmptyState, Modal, ModalFooter, Pill, SectionHeader, Textarea, AdminFormSection, AdminFormField, AdminSelect, AdminPageLayout } from "@/components/admin/admin-ui";
import { Switch } from "@/components/ui/primitives";
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
import { useConfirm } from "@/components/ui/confirm-dialog";

function statusPill(status: string) {
  const tones: Record<string, "green" | "red" | "yellow" | "neutral"> = {
    running: "yellow",
    success: "green",
    failed: "red",
    cancelled: "neutral",
  };
  return <Pill tone={tones[status] ?? "neutral"}>{status}</Pill>;
}

const presetSchedules = [
  { label: "Every 5 min", value: "*/5 * * * *" },
  { label: "Hourly", value: "0 * * * *" },
  { label: "Daily", value: "0 0 * * *" },
  { label: "Weekly", value: "0 0 * * 0" },
];

function parseCronExpression(schedule: string): string | null {
  if (!schedule.trim()) return null;
  const parts = schedule.trim().split(/\s+/);
  if (parts.length !== 5) return "Cron expression must have exactly 5 fields (min hour day month weekday)";
  return null;
}

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
  const createMut = useMutation({
    mutationFn: (input: CreateCronJobInput) => createCronJob(input),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["cron-jobs"] }); onClose(); },
  });
  const updateMut = useMutation({
    mutationFn: (input: CreateCronJobInput) => job ? updateCronJob(job.id, input) : Promise.reject(),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["cron-jobs"] }); onClose(); },
  });

  function validate(): Record<string, string> {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required";
    if (!schedule.trim()) errs.schedule = "Cron expression is required";
    else {
      const cronErr = parseCronExpression(schedule);
      if (cronErr) errs.schedule = cronErr;
    }
    if (!command.trim()) errs.command = "Command is required";
    const timeout = parseInt(timeoutSeconds, 10);
    if (isNaN(timeout) || timeout < 1 || timeout > 86400) errs.timeoutSeconds = "Must be between 1 and 86400";
    const retry = parseInt(retryCount, 10);
    if (isNaN(retry) || retry < 0 || retry > 10) errs.retryCount = "Must be between 0 and 10";
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
      retryCount: parseInt(retryCount, 10),
      timeoutSeconds: parseInt(timeoutSeconds, 10),
      notifyOnFailure,
    };
    if (job) {
      updateMut.mutate(input);
    } else {
      createMut.mutate(input);
    }
  };

  const inputBase = "h-9 w-full rounded-lg border border-white/10 bg-[#0f141f] px-3 text-sm text-slate-100 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15";
  const cronError = errors.schedule;

  return (
    <div className="space-y-6">
      <AdminFormSection title="Identity">
        <AdminFormField label="Name" error={errors.name}>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My Cron Job"
            className={inputBase}
          />
        </AdminFormField>
        <AdminFormField label="Description">
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional description"
            className={inputBase}
          />
        </AdminFormField>
        <AdminFormField label="Enabled">
          <Switch checked={enabled} onCheckedChange={setEnabled} label={enabled ? "Enabled" : "Disabled"} />
        </AdminFormField>
      </AdminFormSection>

      <AdminFormSection title="Schedule">
        <div className="flex flex-wrap gap-1.5">
          {presetSchedules.map((preset) => (
            <button
              key={preset.value}
              type="button"
              onClick={() => { setSchedule(preset.value); setErrors((prev) => { const rest = { ...prev }; delete rest.schedule; return rest; }); }}
              className={`rounded-lg border px-2.5 py-1 text-[11px] font-medium transition ${
                schedule === preset.value
                  ? "border-red-500/40 bg-red-950/30 text-red-300"
                  : "border-white/10 bg-white/[0.03] text-slate-400 hover:border-white/20 hover:text-slate-200"
              }`}
            >
              {preset.label}
            </button>
          ))}
        </div>
        <AdminFormField label="Cron Expression" hint="Standard 5-field cron syntax (min hour day month weekday)" error={cronError}>
          <input
            type="text"
            value={schedule}
            onChange={(e) => { setSchedule(e.target.value); if (e.target.value) setErrors((prev) => { const rest = { ...prev }; delete rest.schedule; return rest; }); }}
            placeholder="*/5 * * * *"
            className={`font-mono text-xs ${inputBase} ${cronError ? "border-red-500/50" : ""}`}
          />
        </AdminFormField>
        {schedule && !cronError && (
          <p className="text-[11px] text-slate-500">
            Runs {schedule === "*/5 * * * *" ? "every 5 minutes" :
                  schedule === "0 * * * *" ? "at the start of every hour" :
                  schedule === "0 0 * * *" ? "once daily at midnight" :
                  schedule === "0 0 * * 0" ? "once weekly on Sunday at midnight" :
                  "on a custom schedule"}
          </p>
        )}
      </AdminFormSection>

      <AdminFormSection title="Execution">
        <AdminFormField label="Command" error={errors.command}>
          <Textarea value={command} onChange={setCommand} rows={3} placeholder="bash command" />
        </AdminFormField>
        <div className="grid grid-cols-2 gap-4">
          <AdminFormField label="Type">
            <AdminSelect value={type} onChange={setType} options={[{ value: "shell", label: "Shell" }, { value: "script", label: "Script" }]} />
          </AdminFormField>
          <AdminFormField label="Target Type">
            <input
              type="text"
              value={targetType}
              onChange={(e) => setTargetType(e.target.value)}
              placeholder="e.g. server, node"
              className={`font-mono text-xs ${inputBase}`}
            />
          </AdminFormField>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <AdminFormField label="Target ID">
            <input
              type="text"
              value={targetId}
              onChange={(e) => setTargetId(e.target.value)}
              placeholder="UUID of target resource"
              className={`font-mono text-xs ${inputBase}`}
            />
          </AdminFormField>
          <AdminFormField label="&nbsp;">
            <div />
          </AdminFormField>
        </div>
      </AdminFormSection>

      <AdminFormSection title="Reliability">
        <div className="grid grid-cols-2 gap-4">
          <AdminFormField label="Timeout (seconds)" hint="Min 1, max 86400 (24h)" error={errors.timeoutSeconds}>
            <input
              type="number"
              min={1}
              max={86400}
              value={timeoutSeconds}
              onChange={(e) => { setTimeoutSeconds(e.target.value); setErrors((prev) => { const rest = { ...prev }; delete rest.timeoutSeconds; return rest; }); }}
              className={inputBase}
            />
          </AdminFormField>
          <AdminFormField label="Retry Count" hint="0-10 automatic retries on failure" error={errors.retryCount}>
            <input
              type="number"
              min={0}
              max={10}
              value={retryCount}
              onChange={(e) => { setRetryCount(e.target.value); setErrors((prev) => { const rest = { ...prev }; delete rest.retryCount; return rest; }); }}
              className={inputBase}
            />
          </AdminFormField>
        </div>
        <AdminFormField label="Notify on Failure">
          <Switch checked={notifyOnFailure} onCheckedChange={setNotifyOnFailure} label={notifyOnFailure ? "On" : "Off"} />
        </AdminFormField>
      </AdminFormSection>

      <ModalFooter
        onCancel={onClose}
        onConfirm={handleSave}
        disabled={createMut.isPending || updateMut.isPending}
        confirmLabel={createMut.isPending || updateMut.isPending ? "Saving..." : job ? "Update" : "Create"}
      />
    </div>
  );
}

function ExecutionLog({ jobId }: { jobId: string }) {
  const [expandedOutputs, setExpandedOutputs] = useState<Set<string>>(new Set());

  const { data: executions, isLoading } = useQuery({
    queryKey: ["cron-executions", jobId],
    queryFn: () => fetchCronJobExecutions(jobId, 20),
    refetchInterval: 10_000,
  });

  function toggleOutput(id: string) {
    setExpandedOutputs((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <Card>
      <CardHeader title="Execution History" icon={Terminal} />
      {isLoading ? (
        <div className="p-4 text-sm text-slate-500">Loading...</div>
      ) : !executions || executions.length === 0 ? (
        <div className="p-4 text-sm text-slate-500">No executions yet</div>
      ) : (
        <div className="divide-y divide-white/[0.06]">
          {executions.map((exec) => {
            const showFull = expandedOutputs.has(exec.id);
            const hasOutput = exec.output || exec.error;
            return (
              <div key={exec.id} className="px-4 py-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    {statusPill(exec.status)}
                    <span className="text-xs text-slate-500">
                      {new Date(exec.startedAt).toLocaleString()}
                    </span>
                  </div>
                  {exec.durationMs != null && (
                    <span className="text-xs text-slate-500">{exec.durationMs}ms</span>
                  )}
                </div>
                {hasOutput && (
                  <div className="mt-2">
                    <button
                      type="button"
                      onClick={() => toggleOutput(exec.id)}
                      className="inline-flex items-center gap-1 text-[11px] text-slate-500 hover:text-slate-300"
                    >
                      {showFull ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                      {showFull ? "Collapse output" : "Show full output"}
                    </button>
                    {showFull && (
                      <>
                        {exec.output && (
                          <pre className="mt-1 max-h-64 overflow-auto rounded bg-black/30 p-2 text-xs text-slate-400">
                            {exec.output}
                          </pre>
                        )}
                        {exec.error && (
                          <pre className="mt-1 max-h-64 overflow-auto rounded bg-red-900/20 p-2 text-xs text-red-400">
                            {exec.error}
                          </pre>
                        )}
                      </>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}

export default function AdminCronJobs() {
  const [confirm, renderConfirm] = useConfirm();
  const [showForm, setShowForm] = useState(false);
  const [editJob, setEditJob] = useState<CronJob | undefined>(undefined);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);

  const queryClient = useQueryClient();

  const { data: jobsData, isLoading } = useQuery({
    queryKey: ["cron-jobs"],
    queryFn: fetchCronJobs,
    refetchInterval: 30_000,
  });
  const jobs = useMemo(() => jobsData ?? [], [jobsData]);

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteCronJob(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["cron-jobs"] }),
  });

  const toggleMut = useMutation({
    mutationFn: (id: string) => toggleCronJob(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["cron-jobs"] }),
  });

  const triggerMut = useMutation({
    mutationFn: (id: string) => triggerCronJob(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["cron-executions", selectedJobId] }),
  });

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Cron Jobs"
        sub="Schedule and manage automated tasks"
        action={
          <div className="flex gap-2">
            <Btn onClick={() => { setEditJob(undefined); setShowForm(true); }}>
              <Plus size={14} /> New Job
            </Btn>
          </div>
        }
      />

      {isLoading ? (
        <div className="text-sm text-slate-500">Loading cron jobs...</div>
      ) : !Array.isArray(jobs) || jobs.length === 0 ? (
        <Card>
          <CardHeader title="Cron Jobs" icon={Clock} />
          <EmptyState icon={Clock} message="No cron jobs configured. Create one to automate tasks." title="No Jobs" />
        </Card>
      ) : (
        <Card>
          <CardHeader title={`Jobs (${jobs.length})`} icon={Clock} />
          <div className="divide-y divide-white/[0.06]">
            {jobs.map((job) => (
              <div key={job.id}>
                <div
                  className="flex cursor-pointer items-center justify-between px-4 py-3 transition hover:bg-white/[0.03]"
                  onClick={() => setSelectedJobId(selectedJobId === job.id ? null : job.id)}
                >
                  <div className="flex items-center gap-3">
                    <div className={`h-2 w-2 rounded-full ${job.enabled ? "bg-emerald-500" : "bg-slate-600"}`} />
                    <div>
                      <span className="text-sm font-medium text-slate-200">{job.name}</span>
                      <span className="ml-2 text-xs text-slate-500">{job.schedule}</span>
                    </div>
                    <Pill tone="neutral">{job.type}</Pill>
                  </div>
                  <div className="flex items-center gap-2">
                    {job.nextRun && (
                      <span className="text-xs text-slate-500">
                        Next: {new Date(job.nextRun).toLocaleString()}
                      </span>
                    )}
                    <button
                      className="rounded p-1.5 text-slate-500 hover:bg-white/10 hover:text-slate-300"
                      onClick={(e) => { e.stopPropagation(); toggleMut.mutate(job.id); }}
                      title={job.enabled ? "Disable" : "Enable"}
                      type="button"
                    >
                      {job.enabled ? <ToggleRight size={14} /> : <ToggleLeft size={14} />}
                    </button>
                    <button
                      className="rounded p-1.5 text-slate-500 hover:bg-white/10 hover:text-emerald-400"
                      onClick={(e) => { e.stopPropagation(); triggerMut.mutate(job.id); }}
                      title="Trigger Now"
                      type="button"
                    >
                      <Play size={14} />
                    </button>
                    <button
                      className="rounded p-1.5 text-slate-500 hover:bg-white/10 hover:text-slate-300"
                      onClick={(e) => { e.stopPropagation(); setEditJob(job); setShowForm(true); }}
                      title="Edit"
                      type="button"
                    >
                      <RefreshCw size={14} />
                    </button>
                    <button
                      className="rounded p-1.5 text-slate-500 hover:bg-white/10 hover:text-red-400"
                      onClick={(e) => { e.stopPropagation(); void (async () => { if (await confirm({ title: `Delete cron job ${job.name}?`, description: "The job will stop running on its schedule. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(job.id); })(); }}
                      title="Delete"
                      type="button"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>
                {selectedJobId === job.id && (
                  <div className="border-t border-white/[0.06] bg-black/10 px-4 py-3">
                    <ExecutionLog jobId={job.id} />
                  </div>
                )}
              </div>
            ))}
          </div>
        </Card>
      )}

      {showForm && (
        <Modal title={editJob ? "Edit Cron Job" : "Create Cron Job"} onClose={() => setShowForm(false)} wide>
          <CronForm job={editJob} onClose={() => setShowForm(false)} />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

