// Authentication and account management API functions
import {
  ApiError,
  deleteJSON,
  fetchJSON,
  patchJSON,
  postJSON,
  putJSON,
  requestJSON,
  resetSessionExpiredNotification,
} from './http';
import type { ApiUser, ApiUserSession, LoginResponse } from './types';

// Login uses the canonical primitive with the 401 session-expiry signal
// suppressed: here a 401 means "bad credentials", not "your session died", so
// it must not trigger the logout/redirect side effect the app wires to 401s.
const CREDENTIAL_HEADERS: Record<string, string> = {
  'Content-Type': 'application/json',
  Accept: 'application/json',
  'X-Forge-Session-Mode': 'cookie',
};

export async function login(email: string, password: string): Promise<LoginResponse> {
  try {
    const res = await requestJSON<LoginResponse>(
      '/auth/login',
      { method: 'POST', headers: CREDENTIAL_HEADERS, body: JSON.stringify({ email, password }) },
      { suppressSessionExpired: true },
    );
    // A successful login starts a new session lifetime — re-arm the once-per-
    // expiry 401 signal so a future expiry in this page lifetime still fires.
    resetSessionExpiredNotification();
    return res;
  } catch (err) {
    if (err instanceof ApiError) {
      // Friendly message for the UI, original ApiError kept as `cause` so
      // status/details survive for callers that inspect them.
      if (err.status === 401 || err.status === 404)
        throw new Error('Invalid email or password.', { cause: err });
      if (err.status === 429)
        throw new Error('Too many login attempts. Please try again later.', { cause: err });
      if (err.status === 0) throw err;
      throw new Error(err.message ? `Unable to sign in. ${err.message}` : 'Unable to sign in. Please try again.', { cause: err });
    }
    throw err instanceof Error ? err : new Error('Unable to sign in. Please try again.');
  }
}

export async function loginCheckpoint(
  confirmationToken: string,
  code?: string,
  recoveryToken?: string,
): Promise<LoginResponse> {
  try {
    const res = await requestJSON<LoginResponse>(
      '/auth/login/checkpoint',
      {
        method: 'POST',
        headers: CREDENTIAL_HEADERS,
        body: JSON.stringify({ confirmationToken, code, recoveryToken }),
      },
      { suppressSessionExpired: true },
    );
    resetSessionExpiredNotification();
    return res;
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.status === 400 || err.status === 401)
        throw new Error('Invalid authentication code.', { cause: err });
      if (err.status === 429)
        throw new Error('Too many verification attempts. Please try again later.', { cause: err });
      if (err.status === 0) throw err;
      throw new Error(err.message ? `Unable to verify the authentication code. ${err.message}` : 'Unable to verify the authentication code.', { cause: err });
    }
    throw err instanceof Error ? err : new Error('Unable to verify the authentication code.');
  }
}

export async function logout(): Promise<void> {
  try {
    // An explicit logout is never an expired session: a 401 here means the
    // server-side session is already gone, and firing the global
    // session-expired event would race the caller's own logout flow
    // (reset + redirect in `finally`) with a second reset + toast + redirect.
    await requestJSON<void>('/auth/logout', { method: 'POST' }, { suppressSessionExpired: true });
  } catch (err) {
    if (err instanceof ApiError) {
      throw new ApiError(err.message ? `Logout failed: ${err.message}` : `Logout failed with ${err.status}`, err.status, err.details);
    }
    throw err;
  } finally {
    resetSessionExpiredNotification();
  }
}

export async function fetchCurrentUser(): Promise<ApiUser | null> {
  try {
    // A 401 here usually means "not signed in", not "session died mid-use" —
    // suppress the global session-expired signal so a logged-out state reports
    // exactly once via the `null` return (SessionLoader redirects from that).
    return await fetchJSON<ApiUser>('/auth/me', {}, { suppressSessionExpired: true });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) return null;
    throw error;
  }
}

/**
 * Verify a raw API bearer token by calling `/auth/me` with it explicitly.
 * Used by the admin API-key screen right after a key is created, so a 401 here
 * means "that token is not valid" rather than "this browser session expired" —
 * the session-expiry signal is therefore suppressed.
 */
export async function verifyBearerToken(token: string): Promise<ApiUser> {
  try {
    // `credentials: 'omit'` keeps the browser session cookie off this call:
    // the Bearer token alone must authenticate, so a stale/foreign session
    // cookie can neither leak nor confuse the "is this token valid" answer.
    return await requestJSON<ApiUser>(
      '/auth/me',
      { headers: { Authorization: `Bearer ${token}` }, credentials: 'omit' },
      { suppressSessionExpired: true },
    );
  } catch (err) {
    if (err instanceof ApiError)
      throw new Error(err.message ? `Token verification failed: ${err.message}` : `Token verification failed with ${err.status}`, { cause: err });
    throw err;
  }
}

export async function refreshSession(): Promise<void> {
  try {
    await requestJSON<void>('/auth/session/refresh', { method: 'POST' });
  } catch (err) {
    if (err instanceof ApiError) {
      throw new ApiError(err.message ? `Session refresh failed: ${err.message}` : `Session refresh failed with ${err.status}`, err.status, err.details);
    }
    throw err;
  }
}

export async function requestPasswordReset(
  email: string,
): Promise<{ status: string; dev_token?: string; dev_reset_url?: string }> {
  return postJSON<{ status: string; dev_token?: string; dev_reset_url?: string }>(
    '/auth/password/email',
    { email },
  );
}

export async function resetPassword(
  email: string,
  token: string,
  password: string,
): Promise<{ status: string }> {
  return postJSON<{ status: string }>('/auth/password/reset', {
    email,
    token,
    password,
  });
}

export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<{ status: string }> {
  return putJSON<{ status: string }>('/account/password', {
    currentPassword,
    newPassword,
  });
}

export async function changeEmail(
  newEmail: string,
  currentPassword: string,
): Promise<{ status: string }> {
  return patchJSON<{ status: string }>('/account/email', {
    newEmail,
    currentPassword,
  });
}

export async function fetchUserSessions(): Promise<ApiUserSession[]> {
  return fetchJSON<ApiUserSession[]>('/auth/sessions');
}

export async function revokeUserSession(
  sessionId: string,
  reason?: string,
): Promise<{ status: string }> {
  const url = reason
    ? `/auth/sessions/${encodeURIComponent(sessionId)}?reason=${encodeURIComponent(reason)}`
    : `/auth/sessions/${encodeURIComponent(sessionId)}`;
  await deleteJSON(url);
  return { status: 'revoked' };
}

export async function revokeAllUserSessions(
  exceptSessionId?: string,
  reason?: string,
): Promise<{ status: string }> {
  // DELETE bodies are unreliable (proxies/stacks may drop them) and the
  // backend `DELETE /auth/sessions` handler reads no body — pass the selector
  // via the query string like the single-session route (`?reason=`), with an
  // empty body. Unknown is not silently dropped: params absent means "all".
  const params = new URLSearchParams();
  if (exceptSessionId) params.set('except', exceptSessionId);
  if (reason) params.set('reason', reason);
  const qs = params.toString();
  await deleteJSON(`/auth/sessions${qs ? `?${qs}` : ''}`);
  return { status: 'revoked' };
}
