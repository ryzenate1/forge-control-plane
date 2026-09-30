import { describe, expect, it } from 'vitest';
import {
  createApp,
  fetchApps,
  mapApplication,
  mapApplicationDetail,
  type BackendApplication,
} from './apps';
import { jsonResponse, mockFetch, requestJSON } from '@/test/fetch-mock';

const baseRow: BackendApplication = {
  id: 'app-1',
  name: 'Web',
  sourceType: 'DOCKER_IMAGE',
  sourceConfig: { image: 'nginx:1.25', nodeId: 'node-1' },
  desiredState: 'running',
  observedStatus: 'running',
  createdAt: '2026-09-25T00:00:00Z',
};

describe('app contract mapping (store.Application -> ApiApp)', () => {
  it('maps image apps with tag split', () => {
    const app = mapApplication(baseRow);
    expect(app.type).toBe('image');
    expect(app.status).toBe('running');
    expect(app.image).toBe('nginx:1.25');
    expect(app.version).toBe('1.25');
    expect(app.node).toBe('node-1');
  });

  it('maps git and compose source types', () => {
    expect(mapApplication({ ...baseRow, sourceType: 'GIT' }).type).toBe('git');
    expect(mapApplication({ ...baseRow, sourceType: 'COMPOSE' }).type).toBe('compose');
  });

  it('parses stringified source configs and tolerates garbage', () => {
    const fromString = mapApplication({ ...baseRow, sourceConfig: '{"image":"redis:7"}' });
    expect(fromString.image).toBe('redis:7');
    expect(fromString.version).toBe('7');

    const broken = mapApplication({ ...baseRow, sourceConfig: '{nope' });
    expect(broken.image).toBeUndefined();
    expect(broken.ports).toEqual([]);
    expect(broken.envVars).toEqual({});
  });

  it('handles registry ports in image refs', () => {
    const app = mapApplication({ ...baseRow, sourceConfig: { image: 'registry:5000/img:tag' } });
    expect(app.image).toBe('registry:5000/img:tag');
    expect(app.version).toBe('tag');
  });

  it('fills detail fields from source config', () => {
    const detail = mapApplicationDetail({
      ...baseRow,
      sourceConfig: { image: 'nginx:1.25', envVars: { FOO: 'bar' }, memoryLimit: '2048' },
    });
    expect(detail.gitRepo).toBeUndefined();
    expect(detail.envVars).toEqual({ FOO: 'bar' });
    expect(detail.resourceLimits?.memory).toBe('2048');
  });

  it('unwraps list responses and maps every row', async () => {
    mockFetch(jsonResponse({ data: [baseRow] }));
    const apps = await fetchApps();
    expect(apps).toHaveLength(1);
    expect(apps[0].type).toBe('image');
    expect(apps[0].image).toBe('nginx:1.25');
  });

  it('createApp sends sourceType plus sourceConfig', async () => {
    const { calls } = mockFetch(jsonResponse({ ...baseRow, sourceType: 'GIT', sourceConfig: {} }));
    await createApp({
      name: 'Api',
      type: 'git',
      gitUrl: 'https://github.com/acme/api.git',
      gitBranch: 'main',
      ports: [],
      envVars: {},
      volumes: [],
      domains: [],
      enableTls: false,
    });
    const body = requestJSON(calls[0]) as Record<string, unknown>;
    expect(body.sourceType).toBe('GIT');
    expect(body.sourceConfig).toMatchObject({ gitUrl: 'https://github.com/acme/api.git', gitBranch: 'main' });
  });
});
