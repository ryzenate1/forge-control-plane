import { requestJSON } from './http';
import type { ApiSetupRequest, ApiSetupStatus } from './types';

/**
 * First-run (setup wizard) client.
 *
 * These routes run before any session exists, so the 401 → session-expired
 * signal is suppressed: there is no session to expire, and firing the event
 * would bounce the operator off the wizard. Transport, CSRF and error shaping
 * still come from the canonical primitive in `lib/api/http.ts`.
 */
export function fetchSetupStatus(): Promise<ApiSetupStatus> {
  return requestJSON<ApiSetupStatus>(
    '/setup/status',
    { signal: AbortSignal.timeout(10_000) },
    { suppressSessionExpired: true },
  );
}

export function runSetup(
  req: ApiSetupRequest,
): Promise<{ ok: boolean; userId: string; email: string }> {
  return requestJSON<{ ok: boolean; userId: string; email: string }>(
    '/setup',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
    { suppressSessionExpired: true },
  );
}
