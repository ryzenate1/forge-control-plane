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

/**
 * Runtime view of a registered plugin, as serialised by the plugin *service*.
 *
 * The panel reads two projections of the same `plugins` rows because the two
 * Go stores disagree about which columns they return:
 *
 *   - `GET /admin/plugins` is served by the metadata store and returns
 *     `description`, `kind`, `version`, `installPath`, `installed` and
 *     `enabled`. It never returns `state`.
 *   - `/enable` and `/disable` are served by the plugin service, which writes
 *     the `state` column (`internal/services/plugins/store.go`) and does not
 *     touch `enabled`.
 *
 * So the only way to verify a lifecycle change from the UI is the service's
 * own list projection, which `GET /admin/plugins/marketplace` happens to
 * return — see {@link fetchPluginRuntimeRecords}.
 */
export type PluginRuntimeRecord = {
  id: string;
  name: string;
  state?: string;
  source?: string;
  error?: string;
  installedAt?: string;
  updatedAt?: string;
  manifest?: unknown;
  settings?: unknown;
};

/** @deprecated Kept for the `lib/api.ts` barrel. See {@link PluginRuntimeRecord}. */
export type PluginMarketplaceItem = PluginRuntimeRecord;
/** `GET /admin/plugins/discover` items report the on-disk path the scanner found. */
export type PluginDiscoverItem = PluginRuntimeRecord & { path?: string };

export type PluginInstallInput = {
  name: string;
  source: string;
  manifest: string;
};

/**
 * Accept the bare-list and the `{ [key]: [] }` envelope, but never turn a
 * payload that holds no list into "no plugins". An unrecognised body is a
 * contract failure and has to surface as one — the same rule
 * `components/admin/AdminWebhooks.tsx` applies.
 */
function asList<T>(body: unknown, key: string): T[] {
  if (Array.isArray(body)) return body as T[];
  if (body && typeof body === "object") {
    const value = (body as Record<string, unknown>)[key];
    if (Array.isArray(value)) return value as T[];
    if (Array.isArray((body as Record<string, unknown>).data)) return (body as { data: T[] }).data;
  }
  throw new Error(`The plugin response did not contain a \`${key}\` list.`);
}

export function fetchPlugins(): Promise<ApiPlugin[]> {
  return fetchJSON<ApiPlugin[]>('/admin/plugins');
}

/**
 * The plugin service's projection of every **registered** plugin.
 *
 * The route is named `marketplace` and the handler returns `pluginSvc.List()`
 * — the stored rows, not a catalogue (`internal/http/handlers_plugins_extended.go`).
 * Forge has no third-party plugin source, so this is used for what only this
 * projection carries: the lifecycle `state` and the last `error`.
 */
export async function fetchPluginRuntimeRecords(): Promise<PluginRuntimeRecord[]> {
  const body = await fetchJSON<PluginRuntimeRecord[] | { marketplace?: PluginRuntimeRecord[] }>('/admin/plugins/marketplace');
  return asList<PluginRuntimeRecord>(body, 'marketplace');
}

/** @deprecated Use {@link fetchPluginRuntimeRecords}. Returns registered plugins. */
export async function fetchMarketplacePlugins(): Promise<PluginMarketplaceItem[]> {
  return fetchPluginRuntimeRecords();
}

/**
 * `GET /admin/plugins/discover` is **not** a read-only scan: the handler walks
 * the server's plugin directory and calls `CreatePlugin` for every manifest it
 * has not seen, so by the time the response arrives every item in it is
 * already registered — and `POST /install` then rejects them as duplicates.
 * Nothing in the UI offers an Install action against it any more.
 */
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

/**
 * Move a plugin to the requested state.
 *
 * The argument is the **target** action. The previous signature took
 * `enabled: boolean` meaning "*currently* enabled" and inverted it, so a caller
 * could not ask to enable a plugin whose state it did not know — which is
 * exactly the case `GET /admin/plugins` puts every plugin in, because that
 * endpoint never reports the `state` these two routes write.
 */
export function togglePluginLifecycle(id: string, action: 'enable' | 'disable'): Promise<{ status?: string }> {
  return postJSON<{ status?: string }>(`/admin/plugins/${encodeURIComponent(id)}/${action}`, {});
}

export function updatePlugin(id: string, data: Record<string, unknown>): Promise<ApiPlugin> {
  return patchJSON<ApiPlugin>(`/admin/plugins/${encodeURIComponent(id)}`, data);
}

/**
 * Replace a plugin's settings document.
 *
 * Real endpoint, and it overwrites whatever is stored. No admin surface offers
 * it, because the list endpoints the page reads do not return the current
 * settings, so an editor could only open blind — see the `settings` note in
 * `components/admin/AdminPlugins.tsx`.
 */
export function updatePluginSettings(id: string, settings: Record<string, unknown>): Promise<ApiPlugin> {
  return putJSON<ApiPlugin>(`/admin/plugins/${encodeURIComponent(id)}/settings`, settings);
}

export function deletePlugin(id: string): Promise<void> {
  return deleteJSON<void>(`/admin/plugins/${encodeURIComponent(id)}`);
}
