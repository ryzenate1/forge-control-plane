"use client";

import { useState } from "react";
import { Archive, ChevronDown, ChevronUp, History, Pencil, Play, Plus, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type ApiSchedule, type ApiScheduleTask, type ApiServer,
  createServerSchedule, createServerScheduleTask, deleteServerSchedule, deleteServerScheduleTask,
  fetchServerScheduleRuns, fetchServerSchedules, runServerSchedule, updateServerSchedule, updateServerScheduleTask,
} from "@/lib/api";
import { PanelCard } from "@/components/ui/panel-card";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage as message } from "@/lib/utils";

const fieldClass = "mt-1 h-10 w-full rounded border border-[#4b5563] bg-[#161b28] px-3 text-slate-100";
type ScheduleDraft = Pick<ApiSchedule, "name" | "cronMinute" | "cronHour" | "cronDayOfMonth" | "cronMonth" | "cronDayOfWeek" | "onlyWhenOnline" | "enabled">;
type TaskDraft = { action: "command" | "power" | "backup"; value: string; sequence: number; timeOffsetSeconds: number; continueOnFailure: boolean };
const defaultSchedule: ScheduleDraft = { name: "", cronMinute: "0", cronHour: "*/6", cronDayOfMonth: "*", cronMonth: "*", cronDayOfWeek: "*", onlyWhenOnline: false, enabled: true };
const defaultTask: TaskDraft = { action: "command", value: "", sequence: 1, timeOffsetSeconds: 0, continueOnFailure: false };

function scheduleDraft(schedule: ApiSchedule): ScheduleDraft {
  return { name: schedule.name, cronMinute: schedule.cronMinute, cronHour: schedule.cronHour, cronDayOfMonth: schedule.cronDayOfMonth, cronMonth: schedule.cronMonth, cronDayOfWeek: schedule.cronDayOfWeek, onlyWhenOnline: schedule.onlyWhenOnline, enabled: schedule.enabled };
}
function taskDraft(task: ApiScheduleTask): TaskDraft {
  return { action: task.action as TaskDraft["action"], value: String(task.payload.command ?? task.payload.signal ?? ""), sequence: task.sequence ?? 0, timeOffsetSeconds: task.timeOffsetSeconds ?? 0, continueOnFailure: task.continueOnFailure ?? false };
}
function taskPayload(draft: TaskDraft) {
  if (draft.action === "command") return { command: draft.value.trim() };
  if (draft.action === "power") return { signal: draft.value };
  return draft.value.trim() ? { ignoredFiles: draft.value.trim() } : {};
}
function validateSchedule(draft: ScheduleDraft) {
  if (!draft.name.trim()) return "Schedule name is required.";
  if ([draft.cronMinute, draft.cronHour, draft.cronDayOfMonth, draft.cronMonth, draft.cronDayOfWeek].some((value) => !value.trim())) return "Every cron field is required.";
  return "";
}
function validateTask(draft: TaskDraft) {
  if (!Number.isInteger(draft.sequence) || draft.sequence < 0) return "Sequence must be a non-negative integer.";
  if (!Number.isInteger(draft.timeOffsetSeconds) || draft.timeOffsetSeconds < 0 || draft.timeOffsetSeconds > 900) return "Offset must be between 0 and 900 seconds.";
  if (draft.action === "command" && !draft.value.trim()) return "Command is required.";
  if (draft.action === "power" && !["start", "stop", "restart", "kill"].includes(draft.value)) return "Choose a power action.";
  return "";
}

function Runs({ serverId, scheduleId }: { serverId: string; scheduleId: string }) {
  const query = useQuery({ queryKey: ["server-schedule-runs", serverId, scheduleId], queryFn: () => fetchServerScheduleRuns(serverId, scheduleId), refetchInterval: 10_000 });
  if (query.isLoading) return <p className="text-xs text-slate-400">Loading run history…</p>;
  if (query.isError) return <p className="text-xs text-red-300">{message(query.error, "Run history unavailable.")}</p>;
  if (!query.data?.length) return <p className="text-xs text-slate-400">No execution history yet.</p>;
  return <div className="space-y-2">{query.data.slice(0, 10).map((run) => <div className="rounded bg-[#0f1419] p-3 text-xs" key={run.id}><div className="flex flex-wrap justify-between gap-2"><span className="font-semibold uppercase text-slate-100">{run.status}</span><span className="text-slate-400">{new Date(run.startedAt ?? "").toLocaleString()} · {run.trigger}</span></div>{run.error ? <p className="mt-1 text-red-300">{run.error}</p> : null}{run.tasks && run.tasks.length ? <ul className="mt-2 space-y-1 text-slate-400">{(run.tasks ?? []).map((task: any) => <li key={task.id}>{task.status} · {new Date(task.executedAt).toLocaleTimeString()}{task.error ? ` · ${task.error}` : ""}</li>)}</ul> : null}</div>)}</div>;
}

function TaskEditor({ initial, pending, onCancel, onSave }: { initial: TaskDraft; pending: boolean; onCancel: () => void; onSave: (draft: TaskDraft) => void }) {
  const [draft, setDraft] = useState(initial);
  const error = validateTask(draft);
  return <div className="grid gap-3 rounded border border-[#4b5563] bg-[#161b28] p-3 md:grid-cols-3">
    <label className="text-xs text-slate-300">Action<select className={fieldClass} value={draft.action} onChange={(event) => setDraft({ ...draft, action: event.target.value as TaskDraft["action"], value: event.target.value === "power" ? "start" : "" })}><option value="command">Command</option><option value="power">Power</option><option value="backup">Backup</option></select></label>
    {draft.action !== "backup" ? <label className="text-xs text-slate-300">{draft.action === "command" ? "Command" : "Signal"}{draft.action === "command" ? <input className={fieldClass} value={draft.value} onChange={(event) => setDraft({ ...draft, value: event.target.value })} /> : <select className={fieldClass} value={draft.value} onChange={(event) => setDraft({ ...draft, value: event.target.value })}><option value="start">Start</option><option value="stop">Stop</option><option value="restart">Restart</option><option value="kill">Kill</option></select>}</label> : <label className="text-xs text-slate-300">Ignored files (optional)<input className={fieldClass} placeholder="One pattern per line" value={draft.value} onChange={(event) => setDraft({ ...draft, value: event.target.value })} /></label>}
    <label className="text-xs text-slate-300">Sequence<input className={fieldClass} min={0} type="number" value={draft.sequence} onChange={(event) => setDraft({ ...draft, sequence: Number(event.target.value) })} /></label>
    <label className="text-xs text-slate-300">Offset (seconds)<input className={fieldClass} min={0} type="number" value={draft.timeOffsetSeconds} onChange={(event) => setDraft({ ...draft, timeOffsetSeconds: Number(event.target.value) })} /></label>
    <label className="flex items-center gap-2 text-sm text-slate-300 md:self-end md:pb-3"><input checked={draft.continueOnFailure} onChange={(event) => setDraft({ ...draft, continueOnFailure: event.target.checked })} type="checkbox" /> Continue on failure</label>
    <div className="flex items-end justify-end gap-2 md:col-span-3">{error ? <span className="mr-auto text-xs text-red-300">{error}</span> : null}<button className="rounded border border-[#4b5563] px-3 py-2 text-xs font-bold" onClick={onCancel} type="button">Cancel</button><button className="rounded bg-[#dc2626] px-3 py-2 text-xs font-bold text-white disabled:opacity-50" disabled={Boolean(error) || pending} onClick={() => onSave(draft)} type="button">{pending ? "Saving…" : "Save Task"}</button></div>
  </div>;
}

export function SchedulesView({ server }: { server?: ApiServer }) {
  const serverId = server?.id ?? "";
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canCreate = hasServerPermission(access, "schedule.create");
  const canUpdate = hasServerPermission(access, "schedule.update");
  const canDelete = hasServerPermission(access, "schedule.delete");
  const qc = useQueryClient();
  const [createDraft, setCreateDraft] = useState(defaultSchedule);
  const [editingSchedule, setEditingSchedule] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState(defaultSchedule);
  const [taskTarget, setTaskTarget] = useState<{ scheduleId: string; taskId?: string } | null>(null);
  const [taskInitial, setTaskInitial] = useState(defaultTask);
  const [history, setHistory] = useState<string | null>(null);
  const [status, setStatus] = useState("");
  const query = useQuery({ queryKey: ["server-schedules", serverId], queryFn: () => fetchServerSchedules(serverId), enabled: Boolean(serverId) });
  const refresh = () => void qc.invalidateQueries({ queryKey: ["server-schedules", serverId] });
  const createMut = useMutation({ mutationFn: (draft: ScheduleDraft) => createServerSchedule(serverId, draft), onSuccess: () => { setCreateDraft(defaultSchedule); setStatus("Schedule created."); refresh(); }, onError: (error) => setStatus(message(error, "Create failed.")) });
  const updateMut = useMutation({ mutationFn: ({ id, draft }: { id: string; draft: Partial<ScheduleDraft> }) => updateServerSchedule(serverId, id, draft), onSuccess: () => { setEditingSchedule(null); setStatus("Schedule updated."); refresh(); }, onError: (error) => setStatus(message(error, "Update failed.")) });
  const deleteMut = useMutation({ mutationFn: (id: string) => deleteServerSchedule(serverId, id), onSuccess: refresh, onError: (error) => setStatus(message(error, "Delete failed.")) });
  const runMut = useMutation({ mutationFn: (id: string) => runServerSchedule(serverId, id), onSuccess: (_, id) => { setHistory(id); setStatus("Schedule run queued."); void qc.invalidateQueries({ queryKey: ["server-schedule-runs", serverId, id] }); }, onError: (error) => setStatus(message(error, "Run failed.")) });
  const taskMut = useMutation({ mutationFn: ({ target, draft }: { target: { scheduleId: string; taskId?: string }; draft: TaskDraft }) => { if (draft.action === "backup" && server?.backupLimit === 0) throw new Error("Backup tasks are unavailable because this server has no backup slots."); return target.taskId ? updateServerScheduleTask(serverId, target.scheduleId, target.taskId, { ...draft, payload: taskPayload(draft), value: undefined }) : createServerScheduleTask(serverId, target.scheduleId, { ...draft, payload: taskPayload(draft) }); }, onSuccess: () => { setTaskTarget(null); setStatus("Task saved."); refresh(); }, onError: (error) => setStatus(message(error, "Task update failed.")) });
  const removeTaskMut = useMutation({ mutationFn: ({ scheduleId, taskId }: { scheduleId: string; taskId: string }) => deleteServerScheduleTask(serverId, scheduleId, taskId), onSuccess: refresh, onError: (error) => setStatus(message(error, "Task delete failed.")) });
  const reorderMut = useMutation({ mutationFn: async ({ scheduleId, tasks, index, direction }: { scheduleId: string; tasks: ApiScheduleTask[]; index: number; direction: -1 | 1 }) => { const ordered = [...tasks].sort((a, b) => (a.sequence ?? 0) - (b.sequence ?? 0)); const other = ordered[index + direction]; const current = ordered[index]; if (!current || !other) return; await updateServerScheduleTask(serverId, scheduleId, current.id, { sequence: other.sequence }); await updateServerScheduleTask(serverId, scheduleId, other.id, { sequence: current.sequence }); }, onSuccess: refresh, onError: (error) => setStatus(message(error, "Task reorder failed.")) });
  const schedules = query.data ?? [];

  const scheduleFields = (draft: ScheduleDraft, setDraft: (value: ScheduleDraft) => void) => <div className="grid gap-3 md:grid-cols-3">
    <label className="text-sm font-semibold text-[#d2dde8] md:col-span-3">Name<input className={fieldClass} onChange={(e) => setDraft({ ...draft, name: e.target.value })} value={draft.name} /></label>
    {["cronMinute", "cronHour", "cronDayOfMonth", "cronMonth", "cronDayOfWeek"].map((key) => <label className="text-sm font-semibold text-[#d2dde8]" key={key}>{({ cronMinute: "Minute", cronHour: "Hour", cronDayOfMonth: "Day of Month", cronMonth: "Month", cronDayOfWeek: "Day of Week" } as Record<string, string>)[key]}<input className={fieldClass} onChange={(e) => setDraft({ ...draft, [key]: e.target.value })} value={String(draft[key as keyof ScheduleDraft])} /></label>)}
    <div className="flex flex-col justify-end gap-2"><label className="flex items-center gap-2 text-sm text-slate-300"><input checked={draft.onlyWhenOnline} onChange={(e) => setDraft({ ...draft, onlyWhenOnline: e.target.checked })} type="checkbox" /> Only when online</label><label className="flex items-center gap-2 text-sm text-slate-300"><input checked={draft.enabled} onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })} type="checkbox" /> Enabled</label></div>
  </div>;

  return <div className="space-y-6">
    <PanelCard title="Create Schedule" icon={Archive}><div className="space-y-3 bg-[#374151] p-4">{scheduleFields(createDraft, setCreateDraft)}<div className="flex justify-end"><button className="rounded bg-[#dc2626] px-4 py-2 text-sm font-bold uppercase text-white disabled:opacity-50" disabled={!canCreate || !serverId || Boolean(validateSchedule(createDraft)) || createMut.isPending} onClick={() => createMut.mutate(createDraft)} type="button">{createMut.isPending ? "Creating…" : "Create Schedule"}</button></div></div></PanelCard>
    {status ? <div className="rounded border border-white/10 bg-[#1e2536] p-3 text-sm text-slate-200" role="status">{status}</div> : null}
    <PanelCard title="Schedules" icon={Archive}><div className="space-y-3 bg-[#374151] p-4">{query.isLoading ? <p className="text-sm text-slate-300">Loading schedules…</p> : null}{query.isError ? <p className="text-sm text-red-300">{message(query.error, "Schedules unavailable.")}</p> : null}{!query.isLoading && !query.isError && !schedules.length ? <p className="text-sm text-slate-300">No schedules configured.</p> : null}{schedules.map((schedule) => <div className="rounded border border-[#4b5563] bg-[#252d3f] p-4" key={schedule.id}>
      {editingSchedule === schedule.id ? <div className="space-y-3">{scheduleFields(editDraft, setEditDraft)}<div className="flex justify-end gap-2"><button className="rounded border border-[#4b5563] px-3 py-2 text-xs font-bold" onClick={() => setEditingSchedule(null)} type="button">Cancel</button><button className="rounded bg-[#dc2626] px-3 py-2 text-xs font-bold text-white disabled:opacity-50" disabled={!canUpdate || Boolean(validateSchedule(editDraft)) || updateMut.isPending} onClick={() => updateMut.mutate({ id: schedule.id, draft: editDraft })} type="button">Save Schedule</button></div></div> : <><div className="flex flex-wrap items-center justify-between gap-3"><div><h3 className="font-bold text-slate-100">{schedule.name}</h3><p className="font-mono text-xs text-slate-400">{schedule.cronMinute} {schedule.cronHour} {schedule.cronDayOfMonth} {schedule.cronMonth} {schedule.cronDayOfWeek}</p><p className="text-xs text-slate-400">{schedule.enabled ? "Enabled" : "Disabled"} · {schedule.onlyWhenOnline ? "Online only" : "Runs regardless of power state"} · Next: {schedule.nextRunAt ? new Date(schedule.nextRunAt).toLocaleString() : "not scheduled"}</p></div><div className="flex flex-wrap gap-2"><button aria-label={`Edit ${schedule.name}`} disabled={!canUpdate} className="rounded border border-[#4b5563] p-2" onClick={() => { setEditingSchedule(schedule.id); setEditDraft(scheduleDraft(schedule)); }} type="button"><Pencil size={14} /></button><button className="inline-flex items-center gap-1 rounded border border-[#4b5563] px-3 py-2 text-xs font-bold" disabled={!canUpdate || runMut.isPending} onClick={() => runMut.mutate(schedule.id)} type="button"><Play size={14} /> Run</button><button className="inline-flex items-center gap-1 rounded border border-[#4b5563] px-3 py-2 text-xs font-bold disabled:opacity-40" disabled={!canUpdate} onClick={() => { setTaskTarget({ scheduleId: schedule.id }); setTaskInitial({ ...defaultTask, sequence: (schedule.tasks ?? []).length + 1 }); }} type="button"><Plus size={14} /> Task</button><button className="rounded border border-[#4b5563] p-2" onClick={() => setHistory(history === schedule.id ? null : schedule.id)} type="button">{history === schedule.id ? <ChevronUp size={14} /> : <History size={14} />}</button><button aria-label={`Delete ${schedule.name}`} className="rounded border border-red-500/60 p-2 text-red-200" disabled={!canDelete || deleteMut.isPending} onClick={() => { if (confirm(`Delete ${schedule.name}?`)) deleteMut.mutate(schedule.id); }} type="button"><Trash2 size={14} /></button></div></div>
      <div className="mt-3 space-y-2">{[...(schedule.tasks ?? [])].sort((a, b) => (a.sequence ?? 0) - (b.sequence ?? 0)).map((task, index, ordered) => <div className="flex flex-wrap items-center justify-between gap-2 rounded bg-[#161b28] p-3" key={task.id}><div><p className="text-sm font-semibold text-slate-100">#{task.sequence} {task.action}{task.payload.command ? `: ${String(task.payload.command)}` : task.payload.signal ? `: ${String(task.payload.signal)}` : task.payload.ignoredFiles ? ` · ignores ${String(task.payload.ignoredFiles)}` : ""}</p><p className="text-xs text-slate-400">Offset {task.timeOffsetSeconds}s · Continue on failure: {task.continueOnFailure ? "yes" : "no"}</p></div><div className="flex gap-2"><button aria-label={`Move task ${task.sequence} up`} className="rounded border border-[#4b5563] p-2 disabled:opacity-40" disabled={!canUpdate || index === 0 || reorderMut.isPending} onClick={() => reorderMut.mutate({ scheduleId: schedule.id, tasks: ordered, index, direction: -1 })} type="button"><ChevronUp size={13} /></button><button aria-label={`Move task ${task.sequence} down`} className="rounded border border-[#4b5563] p-2 disabled:opacity-40" disabled={!canUpdate || index === ordered.length - 1 || reorderMut.isPending} onClick={() => reorderMut.mutate({ scheduleId: schedule.id, tasks: ordered, index, direction: 1 })} type="button"><ChevronDown size={13} /></button><button aria-label={`Edit task ${task.sequence}`} className="rounded border border-[#4b5563] p-2 disabled:opacity-40" disabled={!canUpdate} onClick={() => { setTaskTarget({ scheduleId: schedule.id, taskId: task.id }); setTaskInitial(taskDraft(task)); }} type="button"><Pencil size={13} /></button><button aria-label={`Delete task ${task.sequence}`} className="rounded border border-red-500/60 p-2 text-red-200 disabled:opacity-40" disabled={!canUpdate || removeTaskMut.isPending} onClick={() => { if (window.confirm(`Delete task ${task.sequence}?`)) removeTaskMut.mutate({ scheduleId: schedule.id, taskId: task.id }); }} type="button"><Trash2 size={13} /></button></div></div>)}{!schedule.tasks?.length ? <p className="text-xs text-slate-400">No tasks.</p> : null}</div>
      {taskTarget?.scheduleId === schedule.id ? <div className="mt-3"><TaskEditor initial={taskInitial} pending={taskMut.isPending} onCancel={() => setTaskTarget(null)} onSave={(draft) => taskMut.mutate({ target: taskTarget, draft })} /></div> : null}
      {history === schedule.id ? <div className="mt-3 border-t border-white/10 pt-3"><Runs serverId={serverId} scheduleId={schedule.id} /></div> : null}</>}
    </div>)}</div></PanelCard>
  </div>;
}
