import { describe, expect, it } from 'vitest';
import { registryItemToApiTemplate, templateToApiTemplate } from '../index';
import type { GameTemplate, GameTemplateRegistryItem } from '../types';

const template: GameTemplate = {
  id: 'test-game',
  name: 'Test Game',
  description: 'A test template',
  version: '1.0.0',
  game: 'Test',
  image: 'example.com/test:latest',
  images: { Latest: 'example.com/test:latest' },
  startup: './run.sh -port {{SERVER_PORT}} -name "{{SERVER_NAME}}"',
  config: { startup: { done: 'ready' }, stop: 'stop', logs: {} },
  ports: [{ port: 25565, protocol: 'tcp', public: true }],
  env: [
    {
      name: 'Name',
      env_variable: 'SERVER_NAME',
      default_value: 'Test',
      user_viewable: true,
      user_editable: true,
      rules: 'required|string|max:64',
    },
  ],
  resources: { cpu: 100, memory_mb: 1024, disk_mb: 5120 },
  install_script: { container: 'alpine', entrypoint: 'sh', script: 'echo hi' },
  supported_platforms: ['docker'],
  categories: ['survival'],
};

describe('templateToApiTemplate', () => {
  it('uses caller-supplied real timestamps instead of inventing them', () => {
    const out = templateToApiTemplate(template, {
      nestId: 'n1',
      eggId: 'e1',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-06-01T00:00:00Z',
    });
    expect(out.createdAt).toBe('2026-01-01T00:00:00Z');
    expect(out.updatedAt).toBe('2026-06-01T00:00:00Z');
    expect(out.dockerImage).toBe('example.com/test:latest');
    expect(out.environment).toEqual({ SERVER_NAME: 'Test' });
  });

  it('omits fields without values instead of setting undefined', () => {
    const minimal = { ...template, description: undefined as unknown as string };
    const out = templateToApiTemplate(minimal, {
      nestId: 'n1',
      eggId: 'e1',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    });
    expect('description' in out).toBe(false);
    expect(JSON.stringify(out)).not.toContain('undefined');
  });
});

describe('registryItemToApiTemplate', () => {
  const item: GameTemplateRegistryItem = {
    id: 'test-game',
    name: 'Test Game',
    description: 'A test template',
    game: 'Test',
    version: '1.0.0',
    categories: ['survival'],
    tags: [],
    source: 'https://example.com',
    author: 'Test',
    updatedAt: '2026-06-01T00:00:00Z',
  };

  it('uses the caller-supplied real createdAt instead of reusing updatedAt', () => {
    const out = registryItemToApiTemplate(item, {
      nestId: 'n1',
      eggId: 'e1',
      createdAt: '2026-01-01T00:00:00Z',
    });
    expect(out.createdAt).toBe('2026-01-01T00:00:00Z');
    expect(out.updatedAt).toBe('2026-06-01T00:00:00Z');
    expect(out.nestId).toBe('n1');
    expect(out.eggId).toBe('e1');
  });

  it('omits dockerImage when no real image is supplied', () => {
    const opts = { nestId: 'n1', eggId: 'e1', createdAt: '2026-06-01T00:00:00Z' };
    expect('dockerImage' in registryItemToApiTemplate(item, opts)).toBe(false);
    expect(
      registryItemToApiTemplate(item, { ...opts, dockerImage: 'example.com/test:latest' })
        .dockerImage,
    ).toBe('example.com/test:latest');
  });
});
