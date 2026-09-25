import { deleteJSON, fetchJSON, patchJSON, postJSON, postMultipartJSON, putJSON } from './http';
import type { ApiPlugin } from './types';

/**
 * Plugin (platform integrations) client.
 *
 * Every call goes through the canonical primitives in `lib/api/http.ts`, so
 * CSRF signing, cookie credentials, the 401 session-expiry signal and
 * `ApiError` shaping stay in one place — including the multipart file import,
 * which used to be an ad-hoc `fetch` inside the admin component.
 */

/** `GET /admin/plugins/marketplace` items carry install metadata beyond the stored manifest. */
export type PluginMarketplaceItem = ApiPlugin & { source?: string; manifest?: string; author?: string; description?: string };
/** `GET /admin/plugins/discover` items report the on-disk path the scanner found. */
export type PluginDiscoverItem = ApiPlugin & { source?: string; path?: string };

export type PluginInstallInput = {
  name: string;
  source: string;
  manifest: string;
};

/** Some deployments return the bare list, others an envelope — accept both. */
function asList<T>(body: unknown, key: string): T[] {
  if (Array.isArray(body)) return body as T[];
  if (body && typeof body === 'object') {
    const value = (body as Record<string, unknown>)[key];
    if (Array.isArray(value)) return value as T[];
  }
  return [];
}

export function fetchPlugins(): Promise<ApiPlugin[]> {
  return fetchJSON<ApiPlugin[]>('/admin/plugins');
}

export async function fetchMarketplacePlugins(): Promise<PluginMarketplaceItem[]> {
  const body = await fetchJSON<PluginMarketplaceItem[] | { marketplace?: PluginMarketplaceItem[] }>('/admin/plugins/marketplace');
  return asList<PluginMarketplaceItem>(body, 'marketplace');
}

export async function fetchDiscoveredPlugins(): Promise<PluginDiscoverItem[]> {
  const body = await fetchJSON<PluginDiscoverItem[] | { plugins?: PluginDiscoverItem[] }>('/admin/plugins/discover');
  return asList<PluginDiscoverItem>(body, 'plugins');
}

export async function fetchPluginHooks(id: string): Promise<unknown[]> {
  const body = await fetchJSON<unknown[] | { hooks?: unknown[] }>(`/admin/plugins/${encodeURIComponent(id)}/hooks`);
  return asList<unknown>(body, 'hooks');
}

export function importPluginFromURL(url: string): Promise<ApiPlugin> {
  return postJSON<ApiPlugin>('/admin/plugins/import/url', { url });
}

/** Upload a manifest file as `multipart/form-data` under the `file` field. */
export function importPluginFile(file: File): Promise<ApiPlugin> {
  const form = new FormData();
  form.append('file', file);
  return postMultipartJSON<ApiPlugin>('/admin/plugins/import/file', form);
}

export function installPlugin(input: PluginInstallInput): Promise<ApiPlugin> {
  return postJSON<ApiPlugin>('/admin/plugins/install', input);
}

export function setPluginEnabled(id: string, enabled: boolean): Promise<unknown> {
  return postJSON(`/admin/plugins/${encodeURIComponent(id)}/${enabled ? 'disable' : 'enable'}`, {});
}

export function updatePlugin(id: string, data: Record<string, unknown>): Promise<ApiPlugin> {
  return patchJSON<ApiPlugin>(`/admin/plugins/${encodeURIComponent(id)}`, data);
}

export function updatePluginSettings(id: string, settings: Record<string, unknown>): Promise<ApiPlugin> {
  return putJSON<ApiPlugin>(`/admin/plugins/${encodeURIComponent(id)}/settings`, settings);
}

export function deletePlugin(id: string): Promise<void> {
  return deleteJSON<void>(`/admin/plugins/${encodeURIComponent(id)}`);
}
