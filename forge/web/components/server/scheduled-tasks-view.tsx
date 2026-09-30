"use client";

import { useState } from "react";
import { AlarmClock, CircleX, History, ListChecks, Pencil, Play, Plus, Power, Terminal, Trash2, X } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type ScheduledTask, type TaskRun,
  createScheduledTask, deleteScheduledTask, listScheduledTaskRuns, listScheduledTasks, runScheduledTask, updateScheduledTask,
} from "@/lib/api";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage as message, cn } from "@/lib/utils";
import { Skeleton } from "@/components/ui/loading-skeleton";

const inputBase = "ui-input";
const iconClasses = "pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500";

type TaskDraft = { name: string; command: string; schedule: string; enabled: boolean };
const defaultDraft: TaskDraft = { name: "", command: "", schedule: "0 2 * * *", enabled: true };

function draftOf(task: ScheduledTask): TaskDraft {
  return { name: task.name, command: task.command, schedule: task.schedule, enabled: task.enabled };
}

function validateDraft(draft: TaskDraft): string {
  if (!draft.name.trim() || draft.name.trim().length > 128) return "Name is required (max 128 characters).";
  if (!draft.command.trim()) return "Command is required.";
  if (draft.command.length > 4096) return "Command is too long.";
  if (/[\n\r]/.test(draft.command)) return "Command must be a single line.";
  if (draft.schedule.trim().split(/\s+/).filter(Boolean).length !== 5) return "Schedule must be a 5-field cron expression (e.g. 0 2 * * *).";
  return "";
}

function formatTime(value?: string | null): string {
  return value ? new Date(value).toLocaleString() : "—";
}

function EnabledBadge({ enabled }: { enabled: boolean }) {
  return <span className={cn("ui-status-pill", enabled ? "ui-status-pill-success" : "ui-status-pill-neutral")}>{enabled ? "Enabled" : "Disabled"}</span>;
}

function LastRunBadge({ task }: { task: ScheduledTask }) {
  if (!task.lastRunStatus) return <span className="text-xs text-slate-500">never</span>;
  if (task.lastRunStatus === "success") return <span className="ui-status-pill ui-status-pill-success">success</span>;
  if (task.lastRunStatus === "running") return <span className="ui-status-pill ui-status-pill-warning">running</span>;
  return <span className="ui-status-pill ui-status-pill-danger">failed{task.lastRunExitCode !== undefined ? ` (exit ${task.lastRunExitCode})` : ""}</span>;
}

function RunsDrawer({ serverId, task, onClose }: { serverId: string; task: ScheduledTask; onClose: () => void }) {
  const query = useQuery({
    queryKey: ["scheduled-task-runs", serverId, task.id],
    queryFn: () => listScheduledTaskRuns(serverId, task.id, 25),
    refetchInterval: 5_000,
  });
  return (
    <div className="ui-dialog-layer" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <aside aria-label={`Run history for ${task.name}`} className="fixed inset-y-0 right-0 z-50 flex h-full w-full max-w-lg flex-col border-l border-white/[0.08] bg-[var(--surface)] shadow-2xl" role="dialog">
        <div className="flex items-center justify-between border-b border-white/[0.06] px-5 py-4">
          <div className="min-w-0">
            <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-200"><History size={15} className="text-slate-400" /> Run history</h2>
            <p className="mt-0.5 truncate text-xs text-slate-400">{task.name} &middot; <span className="font-mono">{task.schedule}</span></p>
          </div>
          <button aria-label="Close run history" className="ui-icon-button" onClick={onClose} type="button"><X size={16} /></button>
        </div>
        <div className="flex-1 space-y-2 overflow-y-auto p-5">
          {query.isLoading ? <div className="space-y-3">{Array.from({ length: 3 }).map((_, index) => <Skeleton className="h-16 w-full" key={index} />)}</div> : null}
          {query.isError ? <p className="text-sm text-red-300">{message(query.error, "Run history could not be loaded.")}</p> : null}
          {!query.isLoading && !query.isError && !query.data?.length ? <p className="text-sm text-slate-400">No runs recorded yet.</p> : null}
          {(query.data ?? []).map((run: TaskRun) => (
            <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 text-xs" key={run.id}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className={cn("ui-status-pill", run.status === "success" ? "ui-status-pill-success" : run.status === "running" ? "ui-status-pill-warning" : "ui-status-pill-danger")}>{run.status}</span>
                <span className="font-mono text-slate-400">{formatTime(run.startedAt)}{run.exitCode !== undefined ? ` · exit ${run.exitCode}` : ""}</span>
              </div>
              {run.output ? <pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-black/30 p-2 font-mono text-[11px] leading-4 text-slate-300">{run.output}</pre> : null}
            </div>
          ))}
        </div>
      </aside>
    </div>
  );
}

function TaskModal({ serverId, task, pending, error, onClose, onSave }: {
  serverId: string;
  task: ScheduledTask | null;
  pending: boolean;
  error: string;
  onClose: () => void;
  onSave: (draft: TaskDraft) => void;
}) {
  const [draft, setDraft] = useState<TaskDraft>(task ? draftOf(task) : defaultDraft);
  const validation = validateDraft(draft);
  return (
    <div className="ui-dialog-layer" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget && !pending) onClose(); }}>
      <div className="ui-dialog" role="dialog" aria-modal="true" aria-label={task ? "Edit scheduled task" : "Create scheduled task"}>
        <div className="ui-dialog-header">
          <div className="min-w-0">
            <h2 className="ui-dialog-title">{task ? `Edit ${task.name}` : "New Scheduled Task"}</h2>
            <p className="ui-dialog-description">Cron-expression commands run inside this server&apos;s container via Beacon.</p>
          </div>
          <button aria-label="Close dialog" className="ui-icon-button shrink-0" disabled={pending} onClick={onClose} type="button"><X size={16} /></button>
        </div>
        <div className="space-y-4 px-6 py-4">
          <label className="block text-xs font-semibold uppercase tracking-wider text-slate-400">Name
            <input className={cn(inputBase, "mt-1.5 px-3")} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder="Nightly scheduler" maxLength={128} />
          </label>
          <label className="block text-xs font-semibold uppercase tracking-wider text-slate-400">Schedule <span className="font-normal normal-case text-slate-500">(5-field cron)</span>
            <div className="relative mt-1.5">
              <AlarmClock size={14} className={iconClasses} strokeWidth={1.5} />
              <input className={cn(inputBase, "pl-9 font-mono text-xs")} value={draft.schedule} onChange={(e) => setDraft({ ...draft, schedule: e.target.value })} placeholder="0 2 * * *" />
            </div>
            <p className="mt-1 text-[11px] text-slate-500">minute hour day-of-month month day-of-week &mdash; e.g. <span className="font-mono text-slate-400">*/30 * * * *</span></p>
          </label>
          <label className="block text-xs font-semibold uppercase tracking-wider text-slate-400">Command
            <div className="relative mt-1.5">
              <Terminal size={14} className={iconClasses} strokeWidth={1.5} />
              <input className={cn(inputBase, "pl-9 font-mono text-xs")} value={draft.command} onChange={(e) => setDraft({ ...draft, command: e.target.value })} placeholder="php artisan schedule:run" />
            </div>
          </label>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} type="checkbox" className="accent-red-600" /> Enabled</label>
          {validation ? <p className="text-xs text-amber-300">{validation}</p> : null}
          {error ? <p className="text-xs text-red-300">{error}</p> : null}
        </div>
        <div className="flex flex-col-reverse gap-2 border-t border-[var(--line)] bg-white/[0.015] px-6 py-4 sm:flex-row sm:justify-end">
          <button className="ui-button ui-button-ghost" disabled={pending} onClick={onClose} type="button">Cancel</button>
          <button className="ui-button ui-button-primary" disabled={pending || Boolean(validation) || !serverId} onClick={() => onSave({ ...draft, name: draft.name.trim(), command: draft.command.trim(), schedule: draft.schedule.trim() })} type="button">{pending ? "Saving…" : task ? "Save changes" : "Create task"}</button>
        </div>
      </div>
    </div>
  );
}

export function ScheduledTasksView({ server }: { server?: { id: string } }) {
  const serverId = server?.id ?? "";
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canCreate = hasServerPermission(access, "schedule.create");
  const canUpdate = hasServerPermission(access, "schedule.update");
  const canDelete = hasServerPermission(access, "schedule.delete");
  const qc = useQueryClient();
  const [modal, setModal] = useState<{ task: ScheduledTask | null } | null>(null);
  const [historyTask, setHistoryTask] = useState<ScheduledTask | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<ScheduledTask | null>(null);
  const [status, setStatus] = useState("");

  const query = useQuery({
    queryKey: ["scheduled-tasks", serverId],
    queryFn: () => listScheduledTasks(serverId),
    enabled: Boolean(serverId),
    refetchInterval: 10_000,
  });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["scheduled-tasks", serverId] });
    void qc.invalidateQueries({ queryKey: ["scheduled-task-runs", serverId] });
  };

  const saveMut = useMutation({
    mutationFn: ({ task, draft }: { task: ScheduledTask | null; draft: TaskDraft }) => task
      ? updateScheduledTask(serverId, task.id, { name: draft.name, command: draft.command, schedule: draft.schedule, enabled: draft.enabled })
      : createScheduledTask(serverId, draft),
    onSuccess: () => { setModal(null); setStatus(""); refresh(); },
    onError: (error) => setStatus(message(error, "Save failed.")),
  });
  const toggleMut = useMutation({
    mutationFn: (task: ScheduledTask) => updateScheduledTask(serverId, task.id, { enabled: !task.enabled }),
    onSuccess: () => { setStatus(""); refresh(); },
    onError: (error) => setStatus(message(error, "Toggle failed.")),
  });
  const deleteMut = useMutation({
    mutationFn: (task: ScheduledTask) => deleteScheduledTask(serverId, task.id),
    onSuccess: () => { setDeleteConfirm(null); setStatus(""); refresh(); },
    onError: (error) => { setDeleteConfirm(null); setStatus(message(error, "Delete failed.")); },
  });
  const runMut = useMutation({
    mutationFn: (task: ScheduledTask) => runScheduledTask(serverId, task.id),
    onSuccess: (_run, task) => {
      setStatus(`Started “${task.name}”.`);
      refresh();
    },
    onError: (error) => setStatus(message(error, "Run failed.")),
  });

  const tasks = query.data ?? [];
  const modalError = status && !modal ? "" : status;

  return <div className="space-y-6">
    <div className="ui-card flex flex-wrap items-center justify-between gap-3">
      <div>
        <div className="flex items-center gap-2"><ListChecks className="text-slate-400" size={17} strokeWidth={1.5} /><h2 className="text-sm font-semibold text-slate-200">Scheduled Tasks</h2></div>
        <p className="mt-1 text-xs text-slate-400">Cron-style commands executed inside this server&apos;s container (like Dokku&apos;s <span className="font-mono text-slate-300">cron:add</span>).</p>
      </div>
      <button className="ui-button ui-button-primary" disabled={!canCreate || !serverId} onClick={() => { setStatus(""); setModal({ task: null }); }} type="button"><Plus size={14} /> New Task</button>
    </div>

    {status && !modal ? <div className="ui-alert ui-alert-info" role="status"><p className="text-sm">{status}</p></div> : null}

    <div className="ui-card overflow-x-auto">
      {query.isLoading ? <div className="space-y-3">{Array.from({ length: 3 }).map((_, index) => <Skeleton className="h-12 w-full" key={index} />)}</div> : null}
      {query.isError ? <p className="text-sm text-red-300">{message(query.error, "Scheduled tasks could not be loaded. Check the API connection and your permissions, then retry.")}</p> : null}
      {!query.isLoading && !query.isError && !tasks.length ? <div className="ui-empty"><div className="ui-empty-icon"><ListChecks size={18} /></div><h3 className="mt-3 text-sm font-semibold text-slate-200">No scheduled tasks</h3><p className="mt-1 max-w-md text-sm leading-6 text-slate-400">Create a task such as <span className="font-mono text-slate-300">0 2 * * * php artisan schedule:run</span> to run commands on a cron cadence.</p></div> : null}
      {tasks.length ? <table className="w-full min-w-[760px] text-left text-sm">
        <thead>
          <tr className="border-b border-white/[0.06] text-[11px] uppercase tracking-wider text-slate-500">
            <th className="py-2 pr-3 font-semibold">Name</th>
            <th className="py-2 pr-3 font-semibold">Schedule</th>
            <th className="py-2 pr-3 font-semibold">Command</th>
            <th className="py-2 pr-3 font-semibold">Status</th>
            <th className="py-2 pr-3 font-semibold">Last Run</th>
            <th className="py-2 pr-3 font-semibold">Next Run</th>
            <th className="py-2 font-semibold"><span className="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody>
          {tasks.map((task) => (
            <tr className="border-b border-white/[0.04] align-middle" key={task.id}>
              <td className="max-w-[180px] truncate py-2.5 pr-3 font-medium text-slate-100" title={task.name}>{task.name}</td>
              <td className="py-2.5 pr-3 font-mono text-xs text-slate-300" title={task.schedule}>{task.schedule}</td>
              <td className="max-w-[240px] truncate py-2.5 pr-3 font-mono text-xs text-slate-400" title={task.command}>{task.command}</td>
              <td className="py-2.5 pr-3"><EnabledBadge enabled={task.enabled} /></td>
              <td className="py-2.5 pr-3"><LastRunBadge task={task} /></td>
              <td className="whitespace-nowrap py-2.5 pr-3 text-xs text-slate-400">{task.enabled ? formatTime(task.nextRunAt) : "—"}</td>
              <td className="py-2.5">
                <div className="flex flex-wrap justify-end gap-1.5">
                  <button aria-label={`Run ${task.name} now`} className="ui-icon-button" disabled={!canUpdate || runMut.isPending} onClick={() => { setStatus(""); runMut.mutate(task); }} title="Run now" type="button"><Play size={14} /></button>
                  <button aria-label={`Edit ${task.name}`} className="ui-icon-button" disabled={!canUpdate} onClick={() => { setStatus(""); setModal({ task }); }} title="Edit" type="button"><Pencil size={14} /></button>
                  <button aria-label={`Toggle ${task.name}`} className="ui-icon-button" disabled={!canUpdate || toggleMut.isPending} onClick={() => toggleMut.mutate(task)} title={task.enabled ? "Disable" : "Enable"} type="button"><Power size={14} className={task.enabled ? "text-emerald-300" : "text-slate-500"} /></button>
                  <button aria-label={`Show ${task.name} history`} className="ui-icon-button" disabled={!canCreate && !canUpdate} onClick={() => setHistoryTask(task)} title="Run history" type="button"><History size={14} /></button>
                  <button aria-label={`Delete ${task.name}`} className="ui-icon-button ui-button-danger" disabled={!canDelete || deleteMut.isPending} onClick={() => setDeleteConfirm(task)} title="Delete" type="button"><Trash2 size={14} /></button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table> : null}
    </div>

    {modal ? <TaskModal error={modalError} onClose={() => setModal(null)} pending={saveMut.isPending} onSave={(draft) => saveMut.mutate({ draft, task: modal.task })} serverId={serverId} task={modal.task} /> : null}

    {historyTask ? <RunsDrawer onClose={() => setHistoryTask(null)} serverId={serverId} task={historyTask} /> : null}

    {deleteConfirm ? (
      <div className="ui-dialog-layer" role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget && !deleteMut.isPending) setDeleteConfirm(null); }}>
        <div aria-label="Delete scheduled task" aria-modal="true" className="ui-dialog" role="dialog">
          <div className="ui-dialog-header">
            <div className="min-w-0">
              <h2 className="ui-dialog-title">Delete <CircleX size={15} className="inline text-red-300" /> {deleteConfirm.name}?</h2>
              <p className="ui-dialog-description">The task stops running and its history is removed. This cannot be undone.</p>
            </div>
            <button aria-label="Close dialog" className="ui-icon-button shrink-0" disabled={deleteMut.isPending} onClick={() => setDeleteConfirm(null)} type="button"><X size={16} /></button>
          </div>
          <div className="flex flex-col-reverse gap-2 border-t border-[var(--line)] bg-white/[0.015] px-6 py-4 sm:flex-row sm:justify-end">
            <button className="ui-button ui-button-ghost" disabled={deleteMut.isPending} onClick={() => setDeleteConfirm(null)} type="button">Cancel</button>
            <button className="ui-button ui-button-danger" disabled={deleteMut.isPending} onClick={() => deleteMut.mutate(deleteConfirm)} type="button">{deleteMut.isPending ? "Deleting…" : "Delete task"}</button>
          </div>
        </div>
      </div>
    ) : null}
  </div>;
}
