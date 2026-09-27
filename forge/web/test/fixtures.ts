import type {
  ApiServer,
  ApiNode,
  ApiUser,
  ApiBackup,
  ApiDatabase,
  ApiSchedule,
  ApiScheduleTask,
} from '@forge/shared-types';
import type { ServerAccess } from '@/components/server/server-context';

export const fixtureUser: ApiUser = {
  id: 'u-admin',
  email: 'admin@example.com',
  role: 'admin',
  username: '',
  nameFirst: '',
  nameLast: '',
  useTotp: false,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
};

export const fixtureServer: ApiServer = {
  id: 's1',
  name: 'Game',
  ownerId: 'u-admin',
  owner: 'admin@example.com',
  template: 'Minecraft Java',
  node: 'Node A',
  nodeId: 'n1',
  status: 'offline',
  memoryMb: 2048,
  diskMb: 10240,
  cpuShares: 1024,
  databaseLimit: 2,
  backupLimit: 2,
  allocationLimit: 2,
  generation: 0,
};

export function makeServerAccess(overrides: Partial<ServerAccess> = {}): ServerAccess {
  return { user: fixtureUser, permissions: null, isOwner: true, isAdmin: true, ...overrides };
}

export function makeServer(overrides: Partial<ApiServer> = {}): ApiServer {
  return { ...fixtureServer, ...overrides };
}

export const fixtureNode: ApiNode = {
  id: 'n1',
  name: 'Node A',
  region: 'eu',
  status: 'online',
  fqdn: 'node.example.com',
  daemonSftp: 2022,
  lastHeartbeatAt: '2026-01-01T00:00:00Z',
  memoryMb: 8192,
  diskMb: 51200,
};

export function makeBackup(overrides: Partial<ApiBackup> = {}): ApiBackup {
  return {
    uuid: 'b1',
    id: 'b1',
    serverId: 's1',
    name: 'backup-01',
    status: 'completed',
    isLocked: false,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    completedAt: '2026-01-01T01:00:00Z',
    size: 12,
    ...overrides,
  };
}

export function makeDatabase(overrides: Partial<ApiDatabase> = {}): ApiDatabase {
  return {
    id: 'db1',
    serverId: 's1',
    name: 'world',
    username: 'u1',
    remote: '%',
    engine: 'mysql',
    host: 'db.local',
    port: 3306,
    maxConnections: 10,
    provisioningState: 'ready',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

export function makeSchedule(
  overrides: Partial<ApiSchedule> = {},
  tasks: ApiScheduleTask[] = [],
): ApiSchedule {
  return {
    id: 'sch1',
    serverId: 's1',
    name: 'Daily backup',
    cronMinute: '0',
    cronHour: '2',
    cronDayOfMonth: '*',
    cronMonth: '*',
    cronDayOfWeek: '*',
    onlyWhenOnline: false,
    enabled: true,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    tasks,
    ...overrides,
  };
}

export function makeTask(overrides: Partial<ApiScheduleTask> = {}): ApiScheduleTask {
  return {
    id: 't1',
    scheduleId: 'sch1',
    action: 'backup',
    payload: {},
    continueOnFailure: false,
    sequence: 1,
    ...overrides,
  };
}

/** Wraps data in the paginated envelope the API returns for list endpoints. */
export function apiPage<T>(data: T[], total = data.length) {
  return {
    data,
    meta: {
      pagination: {
        count: data.length,
        current: 1,
        per_page: 15,
        total: Math.ceil(total / 15),
        total_records: total,
      },
    },
  };
}
