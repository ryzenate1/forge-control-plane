import { requestJSON } from './http';

/** A per-app scheduled task (cron-expression command run in the server's container). */
export interface ScheduledTask {
  id: string;
  serverId: string;
  name: string;
  command: string;
  /** Standard 5-field cron expression, e.g. "0 2 * * *". */
  schedule: string;
  enabled: boolean;
  lastRunAt?: string;
  nextRunAt?: string;
  createdAt: string;
  updatedAt: string;
  createdBy?: string;
  /** Denormalized result of the most recent run ("" when never run). */
  lastRunStatus?: string;
  lastRunExitCode?: number;
  lastRunFinishedAt?: string;
}

/** One recorded execution of a scheduled task. */
export interface TaskRun {
  id: string;
  taskId: string;
  startedAt: string;
  finishedAt?: string;
  exitCode?: number;
  output: string;
  status: 'running' | 'success' | 'failed' | string;
}

export interface CreateScheduledTaskInput {
  name: string;
  command: string;
  schedule: string;
  enabled?: boolean;
}

export interface UpdateScheduledTaskInput {
  name?: string;
  command?: string;
  schedule?: string;
  enabled?: boolean;
}

function tasksPath(serverId: string, taskId?: string, suffix?: string): string {
  let path = `/servers/${encodeURIComponent(serverId)}/tasks`;
  if (taskId) path += `/${encodeURIComponent(taskId)}`;
  if (suffix) path += `/${suffix}`;
  return path;
}

export function listScheduledTasks(serverId: string): Promise<ScheduledTask[]> {
  return requestJSON<ScheduledTask[]>(tasksPath(serverId));
}

export function getScheduledTask(serverId: string, taskId: string): Promise<ScheduledTask> {
  return requestJSON<ScheduledTask>(tasksPath(serverId, taskId));
}

export function createScheduledTask(serverId: string, input: CreateScheduledTaskInput): Promise<ScheduledTask> {
  return requestJSON<ScheduledTask>(tasksPath(serverId), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
}

export function updateScheduledTask(serverId: string, taskId: string, input: UpdateScheduledTaskInput): Promise<ScheduledTask> {
  return requestJSON<ScheduledTask>(tasksPath(serverId, taskId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
}

export function deleteScheduledTask(serverId: string, taskId: string): Promise<void> {
  return requestJSON<void>(tasksPath(serverId, taskId), { method: 'DELETE' });
}

/** Triggers the task immediately without shifting its cron cycle. */
export function runScheduledTask(serverId: string, taskId: string): Promise<TaskRun> {
  return requestJSON<TaskRun>(tasksPath(serverId, taskId, 'run'), { method: 'POST' });
}

export function listScheduledTaskRuns(serverId: string, taskId: string, limit = 50): Promise<TaskRun[]> {
  return requestJSON<TaskRun[]>(`${tasksPath(serverId, taskId, 'runs')}?limit=${encodeURIComponent(String(limit))}`);
}
