import type { ApiTemplate } from '@forge/shared-types';
import type { GameTemplate, GameTemplateRegistryItem, GameTemplateRegistry } from './types';

export * from './types';

export function templateToApiTemplate(template: GameTemplate, nestId = 'default', eggId = 'default'): ApiTemplate {
  const environment: Record<string, string> = {};
  for (const item of template.env) {
    environment[item.env_variable] = item.default_value;
  }

  return {
    id: template.id,
    name: template.name,
    description: template.description,
    eggId,
    nestId,
    dockerImage: template.image,
    startupCommand: template.startup,
    environment,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  };
}

export function registryItemToApiTemplate(item: GameTemplateRegistryItem, nestId = 'default', eggId = 'default'): ApiTemplate {
  return {
    id: item.id,
    name: item.name,
    description: item.description,
    eggId,
    nestId,
    dockerImage: undefined,
    createdAt: item.updatedAt,
    updatedAt: item.updatedAt,
  };
}
