"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, Play, Plus, RefreshCw, Terminal, ToggleLeft, ToggleRight, Trash2 } from "lucide-react";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea } from "@/components/admin/admin-ui";
import {
  fetchCronJobs,
  createCronJob,
  updateCronJob,
  deleteCronJob,
  triggerCronJob,
  toggleCronJob,
  fetchCronJobExecutions,
  type CronJob,
  type CronJobExecution,
  type CreateCronJobInput,
} from "@/lib/api/cron-jobs";

function statusPill(status: string) {
  const tones: Record<string, "green" | "red" | "yellow" | "neutral"> = {
    running: "yellow",
    success: "green",
    failed: "red",
    cancelled: "neutral",
  };
  return <Pill tone={tones[status] ?? "neutral"}>{status}</Pill>;
}

function CronForm({ job, onClose }: { job?: CronJob; onClose: () => void }) {
  const [name, setName] = useState(job?.name ?? "");
  const [description, setDescription] = useState(job?.description ?? "");
  const [schedule, setSchedule] = useState(job?.schedule ?? "0 * * * *");
  const [command, setCommand] = useState(job?.command ?? "");
  const [type, setType] = useState(job?.type ?? "shell");
  const [retryCount, setRetryCount] = useState(String(job?.retryCount ?? 0));
  const [timeoutSeconds, setTimeoutSeconds] = useState(String(job?.timeoutSeconds ?? 300));
  const [enabled, setEnabled] = useState(job?.enabled ?? true);

  const queryClient = useQueryClient();
  const createMut = useMutation({
    mutationFn: (input: CreateCronJobInput) => createCronJob(input),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["cron-jobs"] }); onClose(); },
  });
  const updateMut = useMutation({
    mutationFn: (input: CreateCronJobInput) => job ? updateCronJob(job.id, input) : Promise.reject(),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["cron-jobs"] }); onClose(); },
  });

  const handleSave = () => {
    const input: CreateCronJobInput = {
      name,
      description,
      schedule,
      command,
      type,
      retryCount: parseInt(retryCount) || 0,
      timeoutSeconds: parseInt(timeoutSeconds) || 300,
      enabled,
    };
    if (job) {
      updateMut.mutate(input);
    } else {
      createMut.mutate(input);
    }
  };

  return (
    <div className="space-y-4">
      <Input label="Name" value={name} onChange={setName} required />
      <Input label="Description" value={description} onChange={setDescription} />
      <Input label="Cron Schedule" value={schedule} onChange={setSchedule} placeholder="*/5 * * * *" />
      <Textarea label="Command" value={command} onChange={setCommand} rows={3} />
      <div className="grid grid-cols-2 gap-4">
        <Input label="Type" value={type} onChange={setType} placeholder="shell" />
        <Input label="Retry Count" value={retryCount} onChange={setRetryCount} />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <Input label="Timeout (seconds)" value={timeoutSeconds} onChange={setTimeoutSeconds} />
        <label className="flex items-center gap-2 pt-6 text-sm text-slate-300">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="rounded" />
          Enabled
        </label>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={handleSave} disabled={!name || !schedule || !command} confirmLabel={job ? "Update" : "Create"} />
    </div>
  );
}

function ExecutionLog({ jobId }: { jobId: string }) {
  const { data: executions, isLoading } = useQuery({
    queryKey: ["cron-executions", jobId],
    queryFn: () => fetchCronJobExecutions(jobId, 20),
    refetchInterval: 10_000,
  });

  return (
    <Card>
      <CardHeader title="Execution History" icon={Terminal} />
      {isLoading ? (
        <div className="p-4 text-sm text-slate-500">Loading...</div>
      ) : !executions || executions.length === 0 ? (
        <div className="p-4 text-sm text-slate-500">No executions yet</div>
      ) : (
        <div className="divide-y divide-white/[0.06]">
          {executions.map((exec) => (
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
              {exec.output && (
                <pre className="mt-2 max-h-24 overflow-auto rounded bg-black/30 p-2 text-xs text-slate-400">
                  {exec.output}
                </pre>
              )}
              {exec.error && (
                <pre className="mt-1 max-h-16 overflow-auto rounded bg-red-900/20 p-2 text-xs text-red-400">
                  {exec.error}
                </pre>
              )}
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

export default function AdminCronJobs() {
  const [showForm, setShowForm] = useState(false);
  const [editJob, setEditJob] = useState<CronJob | undefined>(undefined);
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);

  const queryClient = useQueryClient();

  const { data: jobs, isLoading } = useQuery({
    queryKey: ["cron-jobs"],
    queryFn: fetchCronJobs,
    refetchInterval: 30_000,
  });

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
    <div className="space-y-6">
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
      ) : !jobs || jobs.length === 0 ? (
        <Card>
          <CardHeader title="Cron Jobs" icon={Clock} />
          <EmptyState icon={Clock} message="No cron jobs configured" title="No Jobs" />
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
                    <Pill tone={job.type === "shell" ? "blue" : "neutral"}>{job.type}</Pill>
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
                      className="rounded p-1.5 text-slate-500 hover:bg-white/10 hover:text-blue-400"
                      onClick={(e) => { e.stopPropagation(); setEditJob(job); setShowForm(true); }}
                      title="Edit"
                      type="button"
                    >
                      <RefreshCw size={14} />
                    </button>
                    <button
                      className="rounded p-1.5 text-slate-500 hover:bg-white/10 hover:text-red-400"
                      onClick={(e) => { e.stopPropagation(); if (confirm("Delete this cron job?")) deleteMut.mutate(job.id); }}
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
        <Modal title={editJob ? "Edit Cron Job" : "New Cron Job"} onClose={() => setShowForm(false)} wide>
          <CronForm job={editJob} onClose={() => setShowForm(false)} />
        </Modal>
      )}
    </div>
  );
}
