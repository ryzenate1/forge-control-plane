// Authentication and account management API functions
import { API_BASE_URL, ApiError, LEGACY_TOKEN_KEY, deleteJSON, fetchJSON, getCSRFToken, patchJSON, postJSON, putJSON } from './http';
import type { ApiUser, ApiUserSession, LoginResponse } from './types';

export async function login(
  email: string,
  password: string,
): Promise<LoginResponse> {
  const response = await fetch(`${API_BASE_URL}/auth/login`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
      'X-Forge-Session-Mode': 'cookie',
    },
    body: JSON.stringify({ email, password }),
    credentials: 'include',
  });

  if (!response.ok) {
    if (response.status === 401 || response.status === 404) throw new Error('Invalid email or password.');
    if (response.status === 429) throw new Error('Too many login attempts. Please try again later.');
    throw new Error('Unable to sign in. Please try again.');
  }

  const payload = await response.json() as LoginResponse;
  return payload;
}

export async function loginCheckpoint(
  confirmationToken: string,
  code?: string,
  recoveryToken?: string,
): Promise<LoginResponse> {
  const response = await fetch(`${API_BASE_URL}/auth/login/checkpoint`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
      'X-Forge-Session-Mode': 'cookie',
    },
    body: JSON.stringify({ confirmationToken, code, recoveryToken }),
    credentials: 'include',
  });

  if (!response.ok) {
    if (response.status === 400 || response.status === 401) throw new Error('Invalid authentication code.');
    if (response.status === 429) throw new Error('Too many verification attempts. Please try again later.');
    throw new Error('Unable to verify the authentication code.');
  }

  const payload = await response.json() as LoginResponse;
  return payload;
}

export async function logout(): Promise<void> {
  const csrfToken = getCSRFToken();
  const response = await fetch(`${API_BASE_URL}/auth/logout`, {
    method: 'POST',
    headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : undefined,
    credentials: 'include',
  });
  if (!response.ok) throw new ApiError(`Logout failed with ${response.status}`, response.status);
}

export async function fetchCurrentUser(): Promise<ApiUser | null> {
  try {
    return await fetchJSON<ApiUser>('/auth/me');
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) return null;
    throw error;
  }
}

export async function refreshSession(): Promise<void> {
  const csrfToken = getCSRFToken();
  const response = await fetch(`${API_BASE_URL}/auth/session/refresh`, {
    method: 'POST',
    headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : undefined,
    credentials: 'include',
  });
  if (!response.ok) throw new ApiError(`Session refresh failed with ${response.status}`, response.status);
}

export async function migrateToCookieSession(): Promise<boolean> {
  if (typeof window === 'undefined') return false;
  const legacyToken = window.localStorage.getItem(LEGACY_TOKEN_KEY);
  if (!legacyToken) return false;

  try {
    const response = await fetch(`${API_BASE_URL}/auth/session/migrate`, {
      method: 'POST',
      headers: { Accept: 'application/json', Authorization: `Bearer ${legacyToken}` },
      credentials: 'include',
    });
    if (response.ok) {
      window.localStorage.removeItem(LEGACY_TOKEN_KEY);
      return true;
    }
    if (response.status === 400 || response.status === 401 || response.status === 403) {
      window.localStorage.removeItem(LEGACY_TOKEN_KEY);
    }
  } catch {
    // Preserve a valid legacy token while the API is temporarily unavailable.
  }
  return false;
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
  return patchJSON<{ status: string }>('/auth/email/change', {
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
  await deleteJSON('/auth/sessions', {
    exceptSessionId,
    reason,
  });
  return { status: 'revoked' };
}
