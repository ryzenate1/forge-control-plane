import { deleteJSON, fetchJSON, postJSON, putJSON, requestJSON, unwrapList, unwrapData } from "./http";

export type RedirectRule = {
  id: string;
  domainId: string;
  sourcePath: string;
  targetUrl: string;
  statusCode: number;
  regex?: boolean;
  preservePath?: boolean;
  enabled?: boolean;
  priority?: number;
  createdAt?: string;
  updatedAt?: string;
};

export type CreateRedirectInput = {
  sourcePath: string;
  targetUrl: string;
  statusCode: number;
  regex?: boolean;
  preservePath?: boolean;
  enabled?: boolean;
  priority?: number;
};

// Admin scoped redirects (handlers_proxy_domains.go /domains/:domainId/redirects)
export function fetchRedirects(domainId: string): Promise<RedirectRule[]> {
  return fetchJSON<{ data: RedirectRule[] } | RedirectRule[]>(`/domains/${encodeURIComponent(domainId)}/redirects`).then(unwrapList);
}

export function createRedirect(domainId: string, input: CreateRedirectInput): Promise<RedirectRule> {
  return postJSON<{ data: RedirectRule } | RedirectRule>(`/domains/${encodeURIComponent(domainId)}/redirects`, { ...input, domainId }).then(unwrapData);
}

export function updateRedirect(domainId: string, redirectId: string, input: Partial<CreateRedirectInput>): Promise<RedirectRule> {
  return putJSON<{ data: RedirectRule } | RedirectRule>(`/domains/${encodeURIComponent(domainId)}/redirects/${encodeURIComponent(redirectId)}`, input).then(unwrapData);
}

export function deleteRedirect(domainId: string, redirectId: string): Promise<void> {
  return deleteJSON<void>(`/domains/${encodeURIComponent(domainId)}/redirects/${encodeURIComponent(redirectId)}`);
}

// Server scoped redirects (handlers_user_web.go /servers/:id/proxy-domains/:domainId/redirects)
export function fetchServerRedirects(serverId: string, domainId: string): Promise<RedirectRule[]> {
  return fetchJSON<RedirectRule[]>(`/servers/${encodeURIComponent(serverId)}/proxy-domains/${encodeURIComponent(domainId)}/redirects`);
}

export function createServerRedirect(serverId: string, domainId: string, input: CreateRedirectInput): Promise<RedirectRule> {
  return postJSON<RedirectRule>(`/servers/${encodeURIComponent(serverId)}/proxy-domains/${encodeURIComponent(domainId)}/redirects`, { ...input, domainId });
}

export function updateServerRedirect(serverId: string, domainId: string, redirectId: string, input: Partial<CreateRedirectInput>): Promise<RedirectRule> {
  return putJSON<RedirectRule>(`/servers/${encodeURIComponent(serverId)}/proxy-domains/${encodeURIComponent(domainId)}/redirects/${encodeURIComponent(redirectId)}`, input);
}

export function deleteServerRedirect(serverId: string, domainId: string, redirectId: string): Promise<void> {
  return deleteJSON<void>(`/servers/${encodeURIComponent(serverId)}/proxy-domains/${encodeURIComponent(domainId)}/redirects/${encodeURIComponent(redirectId)}`);
}

// ---------------------------------------------------------------------------
// Application-scoped domain redirects & forwards
// (handlers_redirects.go -> /api/v1/apps/:appId/redirects, backed by the
// internal/services/redirects package and the `domain_redirects` table)
//
// These are a different feature from the admin `RedirectRule` above: those rows
// belong to a proxy domain and are written by an administrator, these belong to
// an application and are edited by the people who own it. The two share a word
// and nothing else, so the types are named apart rather than overloaded.
// ---------------------------------------------------------------------------

/** How a rule was produced. Mirrors redirects.PresetType. */
export type DomainRedirectPreset = "http-to-https" | "www-apex" | "apex-www" | "custom";

/** The only status codes the gateway is asked to emit. Mirrors redirects.ValidStatusCode. */
export type DomainRedirectStatusCode = 301 | 302 | 307 | 308;

export const DOMAIN_REDIRECT_STATUS_CODES: DomainRedirectStatusCode[] = [301, 302, 307, 308];

/**
 * Mirrors redirects.MaxRulesPerApplication. Kept as a client-side constant (and
 * not read from the list response) so the form can disable itself before a
 * request that the API would refuse with 422 anyway.
 */
export const MAX_DOMAIN_REDIRECT_RULES = 64;

export type DomainRedirect = {
  id: string;
  applicationId: string;
  sourceDomain: string;
  targetDomain: string;
  sourcePath: string;
  /**
   * Absent (null/undefined) means "preserve the incoming request path", which is
   * what a host canonicalisation wants; "" means "send everything to the target
   * root". The distinction is real, so do not coalesce one into the other.
   */
  targetPath?: string | null;
  /** Constrained to {@link DOMAIN_REDIRECT_STATUS_CODES} by the API; typed as a number so a row written before a future code was added still renders. */
  statusCode: number;
  presetType?: DomainRedirectPreset;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type CreateDomainRedirectInput = {
  sourceDomain: string;
  targetDomain: string;
  sourcePath?: string;
  targetPath?: string | null;
  statusCode?: number;
  presetType?: DomainRedirectPreset;
  enabled?: boolean;
};

/**
 * Patch semantics: an omitted field keeps its stored value. `targetPath: null`
 * or `clearTargetPath: true` restores "preserve the request path".
 */
export type UpdateDomainRedirectInput = {
  sourceDomain?: string;
  targetDomain?: string;
  sourcePath?: string;
  targetPath?: string | null;
  clearTargetPath?: boolean;
  statusCode?: number;
  enabled?: boolean;
  presetType?: DomainRedirectPreset;
};

export type DomainRedirectPresetSkip = {
  preset: DomainRedirectPreset;
  domain?: string;
  reason: string;
};

export type DomainRedirectPresetResult = {
  applicationId: string;
  created: DomainRedirect[];
  skipped: DomainRedirectPresetSkip[];
};

/** One rule rendered for the gateway, as returned by POST .../apply. */
export type GeneratedDomainRedirect = {
  id: string;
  applicationId: string;
  name: string;
  kind: "scheme-upgrade" | "host" | "path";
  sourceHost: string;
  sourcePath: string;
  targetUrl: string;
  statusCode: number;
  permanent: boolean;
  preservePath: boolean;
  /** False when the source host is not bound to this application: the rule is valid but can never fire. */
  matchesAppDomain: boolean;
  traefik: {
    middlewareName: string;
    routerRule: string;
    redirectRegex?: { regex: string; replacement: string; permanent: boolean };
    redirectScheme?: { scheme: string; port?: string; permanent: boolean };
  };
  caddy: {
    routeId: string;
    hosts: string[];
    paths?: string[];
    location: string;
    statusCode: number;
  };
};

export type ApplyDomainRedirectsResult = {
  applicationId: string;
  rules: GeneratedDomainRedirect[];
  ruleCount: number;
  adapter?: string;
  gatewaySynced: boolean;
  syncDetail?: string;
};

/**
 * Every call goes through `requestJSON`, the app's single HTTP primitive, so CSRF
 * signing, cookie credentials, the 401 session signal and error shaping are the
 * same as everywhere else in `lib/api`.
 */
function appScopedJSON<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  return requestJSON<T>(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

const appRedirectsPath = (appId: string) => `/apps/${encodeURIComponent(appId)}/redirects`;

export async function fetchDomainRedirects(appId: string): Promise<DomainRedirect[]> {
  const response = await appScopedJSON<{ data?: DomainRedirect[] } | DomainRedirect[]>("GET", appRedirectsPath(appId));
  return unwrapList(response);
}

export function createDomainRedirect(appId: string, input: CreateDomainRedirectInput): Promise<DomainRedirect> {
  return appScopedJSON<{ data: DomainRedirect } | DomainRedirect>("POST", appRedirectsPath(appId), input).then(unwrapData);
}

export function updateDomainRedirect(
  appId: string,
  redirectId: string,
  input: UpdateDomainRedirectInput,
): Promise<DomainRedirect> {
  return appScopedJSON<{ data: DomainRedirect } | DomainRedirect>(
    "PATCH",
    `${appRedirectsPath(appId)}/${encodeURIComponent(redirectId)}`,
    input,
  ).then(unwrapData);
}

/** Resolves once the row is gone; the API answers 404 rather than 204 when nothing matched. */
export function deleteDomainRedirect(appId: string, redirectId: string): Promise<void> {
  return appScopedJSON<void>("DELETE", `${appRedirectsPath(appId)}/${encodeURIComponent(redirectId)}`);
}

/**
 * Bulk-creates the common patterns from the hosts the application is bound to.
 * A preset that could not be applied is reported in `skipped[]` with the reason
 * (the host is not bound, the reverse rule already exists…), so `created.length`
 * is the only honest measure of "did anything change".
 */
export function applyDomainRedirectPresets(
  appId: string,
  presets: DomainRedirectPreset[],
): Promise<DomainRedirectPresetResult> {
  return appScopedJSON<DomainRedirectPresetResult>("POST", `${appRedirectsPath(appId)}/presets`, { presets });
}

/**
 * Recomputes the gateway configuration for this application's enabled rules and
 * triggers the traffic sync. `gatewaySynced` reports the sync only — the
 * generated rules are returned whether or not the gateway was reachable, and
 * `syncDetail` explains a sync that could not be attempted.
 */
export function applyDomainRedirectGateway(appId: string): Promise<ApplyDomainRedirectsResult> {
  return appScopedJSON<ApplyDomainRedirectsResult>("POST", `${appRedirectsPath(appId)}/apply`);
}
