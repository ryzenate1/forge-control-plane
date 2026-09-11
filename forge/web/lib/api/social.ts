import { fetchJSON, postJSON, deleteJSON } from "./http";

export type SocialIdentity = {
  id: string;
  userId: string;
  provider: string;
  providerId: string;
  providerName?: string;
  email?: string;
  createdAt: string;
};

export type SocialProvider = {
  id: string;
  name: string;
  enabled: boolean;
  clientId?: string;
  authUrl?: string;
};

/**
 * GET /account/social/identities — list linked social identities for current user.
 * Backend: handlers_social_auth.go protected.GET("/account/social/identities")
 */
export async function fetchSocialIdentities(): Promise<SocialIdentity[]> {
  const res = await fetchJSON<SocialIdentity[] | { identities: SocialIdentity[]; data?: SocialIdentity[] }>("/account/social/identities");
  if (Array.isArray(res)) return res;
  const obj = res as { identities?: SocialIdentity[]; data?: SocialIdentity[] };
  return obj.identities ?? obj.data ?? [];
}

/**
 * POST /account/social/:provider/link — initiate link flow (requires provider OAuth).
 * For direct API usage, this endpoint expects the provider callback to have
 * stored state; the browser flow is GET /auth/social/:provider -> callback.
 */
export async function linkSocialIdentity(provider: string, opts?: { redirect?: string }): Promise<{ url?: string; status?: string }> {
  return postJSON<{ url?: string; status?: string }>(`/account/social/${encodeURIComponent(provider)}/link`, opts ?? {});
}

/**
 * DELETE /account/social/:provider/unlink — unlink a provider.
 * Backend: protected.DELETE("/account/social/:provider/unlink")
 */
export async function unlinkSocialIdentity(provider: string): Promise<void> {
  await deleteJSON(`/account/social/${encodeURIComponent(provider)}/unlink`);
}

/**
 * Social login entry points (public, no auth required):
 *   GET /auth/social/:provider           -> 302 to provider authorize URL
 *   GET /auth/social/:provider/callback  -> provider returns -> issues session via code exchange
 * These are browser redirects, not JSON APIs. Helper returns the URL to redirect to.
 */
export function socialLoginUrl(provider: string, opts?: { redirect?: string; link?: boolean }): string {
  const qs = new URLSearchParams();
  if (opts?.redirect) qs.set("redirect", opts.redirect);
  if (opts?.link) qs.set("link", "true");
  const q = qs.toString();
  return `/auth/social/${encodeURIComponent(provider)}${q ? `?${q}` : ""}`;
}

export function socialCallbackUrl(provider: string): string {
  return `/auth/social/${encodeURIComponent(provider)}/callback`;
}
