import { fetchJSON, postJSON, putJSON, deleteJSON, requestBlob, unwrapList, unwrapData } from './http';

// Backend wraps single objects and lists in a `{ data: ... }` envelope
// (see forge/api/internal/http/handlers_acme_accounts.go,
// handlers_certificates.go, handlers_certificates_ext.go). Unwrap both the
// envelope and the legacy bare shape so the admin UI works against the real
// API instead of rendering an empty list.
type Envelope<T> = { data: T } | T;
type ListEnvelope<T> = { data: T[] } | T[];

export interface Certificate {
  id: string;
  domains: string[];
  issuer: string;
  certificate?: string;
  expiresAt: string;
  autoRenew: boolean;
  provider: string;
  challengeType: string;
  wildcard: boolean;
  createdAt: string;
  updatedAt: string;
  // Legacy aliases kept for callers written against the pre-envelope shape.
  /** @deprecated use `domains[0]` */
  domain?: string;
  /** @deprecated use `expiresAt` */
  notAfter?: string;
  /** @deprecated certificates carry no issued-at; kept for compat */
  notBefore?: string;
  /** @deprecated use `provider` */
  status?: string;
}

export interface AcmeAccount {
  id: string;
  email: string;
  caUrl: string;
  isDefault: boolean;
  createdAt: string;
}

export interface DNSProviderAccount {
  id: string;
  name: string;
  provider: string;
  createdAt: string;
}

export function listAcmeAccounts(): Promise<AcmeAccount[]> {
  return fetchJSON<ListEnvelope<AcmeAccount>>('/acme/accounts').then(unwrapList);
}

export function createAcmeAccount(config: { email: string; caUrl?: string; privateKey?: string }): Promise<AcmeAccount> {
  return postJSON<Envelope<AcmeAccount>>('/acme/accounts', config).then(unwrapData);
}

export function getAcmeAccount(id: string): Promise<AcmeAccount> {
  return fetchJSON<Envelope<AcmeAccount>>(`/acme/accounts/${encodeURIComponent(id)}`).then(unwrapData);
}

export function updateAcmeAccount(id: string, config: { email?: string; caUrl?: string; isDefault?: boolean }): Promise<AcmeAccount> {
  return putJSON<Envelope<AcmeAccount>>(`/acme/accounts/${encodeURIComponent(id)}`, config).then(unwrapData);
}

export function deleteAcmeAccount(id: string): Promise<void> {
  return deleteJSON(`/acme/accounts/${encodeURIComponent(id)}`);
}

export function setDefaultAcmeAccount(id: string): Promise<AcmeAccount> {
  return updateAcmeAccount(id, { isDefault: true });
}

export function listDNSAccounts(provider?: string): Promise<DNSProviderAccount[]> {
  const query = provider ? `?provider=${encodeURIComponent(provider)}` : '';
  return fetchJSON<ListEnvelope<DNSProviderAccount>>(`/acme/dns-accounts${query}`).then(unwrapList);
}

export function createDNSAccount(config: { name: string; provider: string; credentials: Record<string, string> }): Promise<DNSProviderAccount> {
  return postJSON<Envelope<DNSProviderAccount>>('/acme/dns-accounts', config).then(unwrapData);
}

export function getDNSAccount(id: string): Promise<DNSProviderAccount> {
  return fetchJSON<Envelope<DNSProviderAccount>>(`/acme/dns-accounts/${encodeURIComponent(id)}`).then(unwrapData);
}

export function updateDNSAccount(id: string, config: { name?: string; provider?: string; credentials?: Record<string, string> }): Promise<DNSProviderAccount> {
  return putJSON<Envelope<DNSProviderAccount>>(`/acme/dns-accounts/${encodeURIComponent(id)}`, config).then(unwrapData);
}

export function deleteDNSAccount(id: string): Promise<void> {
  return deleteJSON(`/acme/dns-accounts/${encodeURIComponent(id)}`);
}

export function uploadCertificate(cert: string, key: string, chain?: string): Promise<Certificate> {
  return postJSON<Envelope<Certificate>>('/certificates/upload', { certificate: cert, privateKey: key, chain }).then(unwrapData);
}

export function downloadCertificate(id: string): Promise<Blob> {
  return requestBlob(`/certificates/${encodeURIComponent(id)}/download`);
}

export function exportCertificate(id: string): Promise<{ certificate: string; privateKey: string }> {
  return postJSON<Envelope<{ certificate: string; privateKey: string }>>(`/certificates/${encodeURIComponent(id)}/export`).then(unwrapData);
}

// ---- Certificate inventory + ACME issuance (POST /certificates/issue, etc.) ----

export type IssueCertificateRequest = {
  domains: string[];
  provider?: string;
  email?: string;
  challengeType?: "http-01" | "dns-01" | "tls-alpn-01";
  dnsProvider?: string;
  dnsCredentials?: Record<string, string>;
  autoRenew?: boolean;
};

export function listCertificates(params?: {
  provider?: string;
  status?: string;
  wildcard?: boolean;
  limit?: number;
  offset?: number;
}): Promise<Certificate[]> {
  const query = new URLSearchParams();
  if (params?.provider) query.set("provider", params.provider);
  if (params?.status) query.set("status", params.status);
  if (params?.wildcard !== undefined) query.set("wildcard", String(params.wildcard));
  if (params?.limit !== undefined) query.set("limit", String(params.limit));
  if (params?.offset !== undefined) query.set("offset", String(params.offset));
  const qs = query.toString();
  return fetchJSON<{ data: Certificate[] }>(`/certificates${qs ? `?${qs}` : ""}`).then((r) => r.data ?? []);
}

export function getCertificate(id: string): Promise<Certificate> {
  return fetchJSON<{ data: Certificate }>(`/certificates/${encodeURIComponent(id)}`).then((r) => r.data);
}

export function issueCertificate(req: IssueCertificateRequest): Promise<Certificate> {
  return postJSON<{ data: Certificate }>("/certificates/issue", req).then((r) => r.data);
}

export function deleteCertificate(id: string): Promise<void> {
  return deleteJSON<void>(`/certificates/${encodeURIComponent(id)}`);
}

export function renewCertificate(id: string): Promise<Certificate> {
  return postJSON<{ data: Certificate }>(`/certificates/${encodeURIComponent(id)}/renew`, {}).then((r) => r.data);
}

// ---- Domain-bound import + attach (POST /certificates with domainId,
// PUT /domains/:id) ----

export type ImportDomainCertificateRequest = {
  domainId: string;
  domains?: string[];
  certificate: string;
  privateKey: string;
  issuer?: string;
  autoRenew?: boolean;
};

export function importDomainCertificate(req: ImportDomainCertificateRequest): Promise<Certificate> {
  return postJSON<Envelope<Certificate>>("/certificates", req).then(unwrapData);
}

export type AttachLetsEncryptRequest = {
  certType: "letsencrypt" | "custom" | "none";
  certData?: string;
  certKey?: string;
  autoRenew?: boolean;
  https?: boolean;
};

/** Attach (or detach with certType "none") TLS on a proxy domain. PUT /domains/:id */
export function attachCertificateToDomain(domainId: string, req: AttachLetsEncryptRequest): Promise<unknown> {
  return putJSON<Envelope<unknown>>(`/domains/${encodeURIComponent(domainId)}`, req).then(unwrapData);
}
