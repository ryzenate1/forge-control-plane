import { fetchJSON, postJSON, deleteJSON } from "./http";

export interface ServerDomain {
  id: string;
  serverId: string;
  domain: string;
  verified: boolean;
  verificationToken?: string;
  dnsConfigured?: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface VerifyDomainResult {
  verified: boolean;
  message?: string;
}

export interface CheckDNSResult {
  configured: boolean;
  currentIp?: string;
  expectedIp?: string;
  message?: string;
}

export async function fetchServerDomains(serverId: string): Promise<ServerDomain[]> {
  const res = await fetchJSON<{ data: ServerDomain[] }>(`/servers/${encodeURIComponent(serverId)}/domains`);
  return res.data;
}

export async function addServerDomain(serverId: string, domain: string): Promise<ServerDomain> {
  const res = await postJSON<{ data: ServerDomain }>(`/servers/${encodeURIComponent(serverId)}/domains`, { domain });
  return res.data;
}

export async function removeServerDomain(serverId: string, domainId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/domains/${encodeURIComponent(domainId)}`);
}

export async function verifyDomain(id: string): Promise<VerifyDomainResult> {
  const res = await postJSON<{ data: VerifyDomainResult }>("/domains/verify", { id });
  return res.data;
}

export async function checkDNS(domain: string, expectedIp?: string): Promise<CheckDNSResult> {
  const res = await postJSON<{ data: CheckDNSResult }>("/domains/check-dns", { domain, expectedIp });
  return res.data;
}
