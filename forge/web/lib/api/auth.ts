// Authentication and account management API functions
import {
  ApiError,
  deleteJSON,
  fetchJSON,
  patchJSON,
  postJSON,
  putJSON,
  requestJSON,
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
    return await requestJSON<LoginResponse>(
      '/auth/login',
      { method: 'POST', headers: CREDENTIAL_HEADERS, body: JSON.stringify({ email, password }) },
      { suppressSessionExpired: true },
    );
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.status === 401 || err.status === 404) throw new Error('Invalid email or password.');
      if (err.status === 429) throw new Error('Too many login attempts. Please try again later.');
      if (err.status === 0) throw err;
    }
    throw new Error('Unable to sign in. Please try again.');
  }
}

export async function loginCheckpoint(
  confirmationToken: string,
  code?: string,
  recoveryToken?: string,
): Promise<LoginResponse> {
  try {
    return await requestJSON<LoginResponse>(
      '/auth/login/checkpoint',
      {
        method: 'POST',
        headers: CREDENTIAL_HEADERS,
        body: JSON.stringify({ confirmationToken, code, recoveryToken }),
      },
      { suppressSessionExpired: true },
    );
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.status === 400 || err.status === 401) throw new Error('Invalid authentication code.');
      if (err.status === 429) throw new Error('Too many verification attempts. Please try again later.');
      if (err.status === 0) throw err;
    }
    throw new Error('Unable to verify the authentication code.');
  }
}

export async function logout(): Promise<void> {
  try {
    await requestJSON<void>('/auth/logout', { method: 'POST' });
  } catch (err) {
    if (err instanceof ApiError) {
      throw new ApiError(`Logout failed with ${err.status}`, err.status);
    }
    throw err;
  }
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
  try {
    await requestJSON<void>('/auth/session/refresh', { method: 'POST' });
  } catch (err) {
    if (err instanceof ApiError) {
      throw new ApiError(`Session refresh failed with ${err.status}`, err.status);
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
  await deleteJSON('/auth/sessions', {
    exceptSessionId,
    reason,
  });
  return { status: 'revoked' };
}
