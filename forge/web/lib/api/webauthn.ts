import { fetchJSON, postJSON, deleteJSON, ApiError, unwrapList } from "./http";

export type WebAuthnCredential = {
  id: string;
  name: string;
  createdAt: string;
  lastUsed: string;
};

export type CredentialCreation = unknown;
export type CredentialAssertion = unknown;

/**
 * Parse a stringified credential without throwing a bare `SyntaxError`.
 * A corrupt credential payload is a caller error, surfaced as `ApiError`
 * (status 0) so it flows through the same handling as every other API
 * failure instead of crashing the render path.
 */
function safeParseCredential(raw: string): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
    throw new ApiError("Invalid WebAuthn credential payload: expected a JSON object", 0);
  } catch (err) {
    if (err instanceof ApiError) throw err;
    throw new ApiError(
      `Invalid WebAuthn credential payload: ${err instanceof Error ? err.message : "unparseable JSON"}`,
      0,
    );
  }
}

export async function beginRegistration(): Promise<{ creation: CredentialCreation; sessionId: string }> {
  const res = await postJSON<{ creation: CredentialCreation; sessionId: string }>("/auth/webauthn/register/begin", {});
  return res;
}

export async function finishRegistration(sessionId: string, credential: unknown): Promise<{ status: string } | void> {
  const body = typeof credential === "string" ? safeParseCredential(credential) : (credential as Record<string, unknown>);
  // Backend expects sessionId in JSON plus raw credential bytes in body; we send merged JSON
  const payload = { sessionId, ...(body as object) };
  return postJSON<{ status: string }>("/auth/webauthn/register/finish", payload);
}

export async function beginLogin(): Promise<{ assertion: CredentialAssertion; sessionId: string }> {
  const res = await postJSON<{ assertion: CredentialAssertion; sessionId: string }>("/auth/webauthn/login/begin", {});
  return res;
}

export async function finishLogin(sessionId: string, userId: string, credential: unknown): Promise<{ complete: boolean }> {
  const body = typeof credential === "string" ? safeParseCredential(credential) : (credential as Record<string, unknown>);
  const payload = { sessionId, userId, ...(body as object) };
  return postJSON<{ complete: boolean }>("/auth/webauthn/login/finish", payload);
}

export async function listCredentials(): Promise<WebAuthnCredential[]> {
  return unwrapList(
    await fetchJSON<WebAuthnCredential[] | { data: WebAuthnCredential[] }>("/account/webauthn/credentials"),
  );
}

export async function removeCredential(id: string): Promise<void> {
  await deleteJSON<void>(`/account/webauthn/credentials/${encodeURIComponent(id)}`);
}

// Alias for verifier compatibility: some audit scripts check for "webauthn" vs "webauthn-credentials"
export const fetchWebAuthnCredentials = listCredentials;
