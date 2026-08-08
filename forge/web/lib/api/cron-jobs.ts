import { fetchJSON, postJSON, putJSON, deleteJSON } from './http';

export interface CronJob {
  id: string;
  name: string;
  description: string;
  schedule: string;
  command: string;
  type: string;
  targetType: string;
  targetId: string;
  enabled: boolean;
  retryCount: number;
  timeoutSeconds: number;
  notifyOnFailure: boolean;
  createdAt: string;
  updatedAt: string;
  nextRun?: string;
}

export interface CronJobExecution {
  id: string;
  cronJobId: string;
  startedAt: string;
  finishedAt?: string;
  status: string;
  exitCode?: number;
  output: string;
  error: string;
  durationMs?: number;
}

export interface CreateCronJobInput {
  name: string;
  description?: string;
  schedule: string;
  command: string;
  type?: string;
  targetType?: string;
  targetId?: string;
  enabled?: boolean;
  retryCount?: number;
  timeoutSeconds?: number;
  notifyOnFailure?: boolean;
}

export interface UpdateCronJobInput {
  name?: string;
  description?: string;
  schedule?: string;
  command?: string;
  type?: string;
  targetType?: string;
  targetId?: string;
  enabled?: boolean;
  retryCount?: number;
  timeoutSeconds?: number;
  notifyOnFailure?: boolean;
}

export function fetchCronJobs(): Promise<CronJob[]> {
  return fetchJSON<CronJob[]>('/cron-jobs');
}

export function fetchCronJob(id: string): Promise<CronJob> {
  return fetchJSON<CronJob>(`/cron-jobs/${encodeURIComponent(id)}`);
}

export function createCronJob(input: CreateCronJobInput): Promise<CronJob> {
  return postJSON<CronJob>('/cron-jobs', input);
}

export function updateCronJob(id: string, input: UpdateCronJobInput): Promise<CronJob> {
  return putJSON<CronJob>(`/cron-jobs/${encodeURIComponent(id)}`, input);
}

export function deleteCronJob(id: string): Promise<void> {
  return deleteJSON(`/cron-jobs/${encodeURIComponent(id)}`);
}

export function triggerCronJob(id: string): Promise<CronJobExecution> {
  return postJSON<CronJobExecution>(`/cron-jobs/${encodeURIComponent(id)}/execute`);
}

export function toggleCronJob(id: string): Promise<CronJob> {
  return postJSON<CronJob>(`/cron-jobs/${encodeURIComponent(id)}/toggle`);
}

export function fetchCronJobExecutions(id: string, limit?: number): Promise<CronJobExecution[]> {
  const query = limit ? `?limit=${limit}` : '';
  return fetchJSON<CronJobExecution[]>(`/cron-jobs/${encodeURIComponent(id)}/executions${query}`);
}
