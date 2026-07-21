import { fetchJSON, postJSON, putJSON, deleteJSON } from './http';

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

export async function downloadCertificate(id: string): Promise<Blob> {
  const response = await fetch(`/api/v1/certificates/${encodeURIComponent(id)}/download`, {
    credentials: 'include',
  });
  if (!response.ok) {
    throw new Error(`download failed: ${response.status}`);
  }
  return response.blob();
}

export function exportCertificate(id: string): Promise<{ certificate: string; privateKey: string }> {
  return postJSON<{ certificate: string; privateKey: string }>(`/certificates/${encodeURIComponent(id)}/export`);
}
