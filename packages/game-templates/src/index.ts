import type { ApiTemplate } from '@forge/shared-types';
import type { GameTemplate, GameTemplateRegistryItem, GameTemplateRegistry } from './types';

export * from './types';

export type ApiTemplateTimestamps = {
  /** Real creation time of the template (e.g. from the registry or filesystem), RFC 3339. */
  createdAt: string;
  /** Real last-modified time of the template (e.g. the registry `updatedAt`), RFC 3339. */
  updatedAt: string;
};

export type TemplateToApiOptions = ApiTemplateTimestamps & {
  nestId?: string;
  eggId?: string;
};

/**
 * Convert a `GameTemplate` to the API shape. Timestamps must be real —
 * passed in from the registry entry or the template file's mtime — because a
 * fabricated `new Date()` would misreport template freshness to every caller.
 * Fields without a value are omitted, never set to explicit `undefined`, so
 * serialised output carries no phantom keys.
 */
export function templateToApiTemplate(
  template: GameTemplate,
  opts: TemplateToApiOptions,
): ApiTemplate {
  const environment: Record<string, string> = {};
  for (const item of template.env) {
    environment[item.env_variable] = item.default_value;
  }

  const out: ApiTemplate = {
    id: template.id,
    name: template.name,
    eggId: opts.eggId ?? 'default',
    nestId: opts.nestId ?? 'default',
    createdAt: opts.createdAt,
    updatedAt: opts.updatedAt,
  };
  if (template.description !== undefined) out.description = template.description;
  // `image` is the default image; the full label -> image map stays on the
  // GameTemplate for UIs that offer an image picker.
  if (template.image !== undefined) out.dockerImage = template.image;
  if (template.startup !== undefined) out.startupCommand = template.startup;
  if (Object.keys(environment).length > 0) out.environment = environment;
  return out;
}

/**
 * Convert a registry index entry to the API shape. Registry entries carry no
 * image information, so `dockerImage` is only present when the caller supplies
 * a real image — never an explicit `undefined`.
 */
export function registryItemToApiTemplate(
  item: GameTemplateRegistryItem,
  opts: { nestId?: string; eggId?: string; dockerImage?: string } = {},
): ApiTemplate {
  const out: ApiTemplate = {
    id: item.id,
    name: item.name,
    eggId: opts.eggId ?? 'default',
    nestId: opts.nestId ?? 'default',
    createdAt: item.updatedAt,
    updatedAt: item.updatedAt,
  };
  if (item.description !== undefined) out.description = item.description;
  if (opts.dockerImage !== undefined) out.dockerImage = opts.dockerImage;
  return out;
}
