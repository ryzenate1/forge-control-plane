import { fetchJSON } from './http';
import type { ApiPanelSettings, ApiPublicPanelSettings } from './types';

/**
 * Panel settings client.
 *
 * The public endpoint is an unauthenticated, pre-login read (branding on the
 * sign-in screen), so the 401 session-expiry signal is suppressed — a failure
 * there must never be interpreted as "your session died" and redirect the user.
 */
export function fetchPublicPanelSettings(): Promise<ApiPublicPanelSettings> {
  return fetchJSON<ApiPublicPanelSettings>('/panel/settings/public', undefined, { suppressSessionExpired: true });
}

export function fetchPanelSettings(): Promise<ApiPanelSettings> {
  return fetchJSON<ApiPanelSettings>('/admin/settings');
}

export function savePanelSettings(input: ApiPanelSettings): Promise<ApiPanelSettings> {
  return fetchJSON<ApiPanelSettings>('/admin/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
}
