import { describe, expect, it } from 'vitest';
import {
  assignMountToServer,
  assignServerToMount,
  fetchMountServers,
  unassignMountFromServer,
  unassignServerFromMount,
} from './mounts';
import { jsonResponse, mockFetch, requestJSON } from '@/test/fetch-mock';

describe('mount API contracts', () => {
  it('exposes runtime synchronization after server-scoped assignment and removal', async () => {
    const response = { ok: true, runtimeSynchronized: true };
    const { calls } = mockFetch(jsonResponse(response), jsonResponse(response));

    await expect(assignMountToServer('server/id', { mountId: 'mount/id' })).resolves.toEqual(response);
    await expect(unassignMountFromServer('server/id', 'mount/id')).resolves.toEqual(response);

    expect(calls[0].url).toContain('/servers/server%2Fid/mounts');
    expect(requestJSON(calls[0])).toEqual({ mountId: 'mount/id' });
    expect(calls[1].url).toContain('/servers/server%2Fid/mounts/mount%2Fid');
    expect(calls[1].init?.method).toBe('DELETE');
  });

  it('uses the mount-scoped server routes and returns server records', async () => {
    const server = { id: 'server-1', name: 'Game server', status: 'offline' };
    const response = { ok: true, runtimeSynchronized: true };
    const { calls } = mockFetch(jsonResponse([server]), jsonResponse(response), jsonResponse(response));

    await expect(fetchMountServers('mount/id')).resolves.toEqual([server]);
    await expect(assignServerToMount('mount/id', 'server/id')).resolves.toEqual(response);
    await expect(unassignServerFromMount('mount/id', 'server/id')).resolves.toEqual(response);

    expect(calls[0].url).toContain('/mounts/mount%2Fid/servers');
    expect(calls[1].url).toContain('/mounts/mount%2Fid/servers');
    expect(requestJSON(calls[1])).toEqual({ serverId: 'server/id' });
    expect(calls[2].url).toContain('/mounts/mount%2Fid/servers/server%2Fid');
    expect(calls[2].init?.method).toBe('DELETE');
  });
});
