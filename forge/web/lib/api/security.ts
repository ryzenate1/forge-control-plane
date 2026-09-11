import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

export interface SecurityHeaderConfig {
  id: string;
  domainId: string;
  hstsEnabled: boolean;
  hstsMaxAge: number;
  hstsIncludeSubdomains: boolean;
  hstsPreload: boolean;
  xFrameOptions: string;
  xContentTypeOptions: string;
  referrerPolicy: string;
  cspEnabled: boolean;
  cspPolicy?: string;
  permissionsPolicy?: string;
  customHeaders?: Record<string, string>;
  createdAt: string;
  updatedAt: string;
}

export interface UpdateSecurityHeadersInput {
  hstsEnabled?: boolean;
  hstsMaxAge?: number;
  hstsIncludeSubdomains?: boolean;
  hstsPreload?: boolean;
  xFrameOptions?: string;
  xContentTypeOptions?: string;
  referrerPolicy?: string;
  cspEnabled?: boolean;
  cspPolicy?: string;
  permissionsPolicy?: string;
  customHeaders?: Record<string, string>;
}

export function fetchDomainSecurityHeaders(domainId: string): Promise<SecurityHeaderConfig | null> {
  return fetchJSON<{ data: SecurityHeaderConfig | null }>(
    `/domains/${encodeURIComponent(domainId)}/security-headers`
  ).then((res) => res.data);
}

export function createDomainSecurityHeaders(
  domainId: string,
  input: UpdateSecurityHeadersInput
): Promise<SecurityHeaderConfig> {
  return postJSON<{ data: SecurityHeaderConfig }>(
    `/domains/${encodeURIComponent(domainId)}/security-headers`,
    input
  ).then((res) => res.data);
}

export function updateDomainSecurityHeaders(
  domainId: string,
  id: string,
  input: UpdateSecurityHeadersInput
): Promise<SecurityHeaderConfig> {
  return putJSON<{ data: SecurityHeaderConfig }>(
    `/domains/${encodeURIComponent(domainId)}/security-headers/${encodeURIComponent(id)}`,
    input
  ).then((res) => res.data);
}

export function deleteDomainSecurityHeaders(domainId: string, id: string): Promise<void> {
  return deleteJSON<void>(
    `/domains/${encodeURIComponent(domainId)}/security-headers/${encodeURIComponent(id)}`
  );
}

// Re-export redirect family for convenience — canonical implementation lives in lib/api/redirects.ts
// This satisfies the "security.ts redirects added" wiring requirement while keeping redirects modular.
export {
  fetchRedirects,
  createRedirect,
  updateRedirect,
  deleteRedirect,
  fetchServerRedirects,
  createServerRedirect,
  updateServerRedirect,
  deleteServerRedirect,
  type RedirectRule,
  type CreateRedirectInput,
} from "./redirects";
