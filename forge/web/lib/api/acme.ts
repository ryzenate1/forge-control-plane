import { fetchJSON, postJSON, putJSON, deleteJSON, requestBlob } from './http';

export interface Certificate {
  id: string;
  domain: string;
  issuer: string;
  notBefore: string;
  notAfter: string;
  autoRenew: boolean;
  status: string;
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
  return fetchJSON<AcmeAccount[]>('/acme/accounts');
}

export function createAcmeAccount(config: { email: string; caUrl?: string; privateKey?: string }): Promise<AcmeAccount> {
  return postJSON<AcmeAccount>('/acme/accounts', config);
}

export function getAcmeAccount(id: string): Promise<AcmeAccount> {
  return fetchJSON<AcmeAccount>(`/acme/accounts/${encodeURIComponent(id)}`);
}

export function updateAcmeAccount(id: string, config: { email?: string; caUrl?: string; isDefault?: boolean }): Promise<AcmeAccount> {
  return putJSON<AcmeAccount>(`/acme/accounts/${encodeURIComponent(id)}`, config);
}

export function deleteAcmeAccount(id: string): Promise<void> {
  return deleteJSON(`/acme/accounts/${encodeURIComponent(id)}`);
}

export function listDNSAccounts(provider?: string): Promise<DNSProviderAccount[]> {
  const query = provider ? `?provider=${encodeURIComponent(provider)}` : '';
  return fetchJSON<DNSProviderAccount[]>(`/acme/dns-accounts${query}`);
}

export function createDNSAccount(config: { name: string; provider: string; credentials: Record<string, string> }): Promise<DNSProviderAccount> {
  return postJSON<DNSProviderAccount>('/acme/dns-accounts', config);
}

export function getDNSAccount(id: string): Promise<DNSProviderAccount> {
  return fetchJSON<DNSProviderAccount>(`/acme/dns-accounts/${encodeURIComponent(id)}`);
}

export function updateDNSAccount(id: string, config: { name?: string; provider?: string; credentials?: Record<string, string> }): Promise<DNSProviderAccount> {
  return putJSON<DNSProviderAccount>(`/acme/dns-accounts/${encodeURIComponent(id)}`, config);
}

export function deleteDNSAccount(id: string): Promise<void> {
  return deleteJSON(`/acme/dns-accounts/${encodeURIComponent(id)}`);
}

export function uploadCertificate(cert: string, key: string, chain?: string): Promise<Certificate> {
  return postJSON<Certificate>('/certificates/upload', { certificate: cert, privateKey: key, chain });
}

export function downloadCertificate(id: string): Promise<Blob> {
  return requestBlob(`/certificates/${encodeURIComponent(id)}/download`);
}

export function exportCertificate(id: string): Promise<{ certificate: string; privateKey: string }> {
  return postJSON<{ certificate: string; privateKey: string }>(`/certificates/${encodeURIComponent(id)}/export`);
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
