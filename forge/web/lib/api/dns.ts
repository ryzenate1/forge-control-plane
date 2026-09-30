import { fetchJSON, postJSON, deleteJSON, unwrapData } from "./http";

export type DNSProvider = {
  id: string;
  name: string;
  provider: string;
  providerType?: string;
  verificationStatus?: string;
  verified?: boolean;
  isDefault?: boolean;
  createdAt: string;
  updatedAt?: string;
};

export type DNSSupportedProvider = {
  type: string;
  name: string;
  description: string;
  credentialFields: Array<{
    key: string;
    label: string;
    type: string;
    required: boolean;
    description?: string;
  }>;
};

export type CreateDNSProviderInput = {
  name: string;
  providerType: string;
  credentials: Record<string, string>;
};

export function fetchDnsProviders(): Promise<DNSProvider[]> {
  return fetchJSON<DNSProvider[] | { data: DNSProvider[] }>("/dns/providers/configured").then((res) => {
    const data = unwrapData(res as DNSProvider[] | { data: DNSProvider[] });
    return Array.isArray(data) ? data : [];
  });
}

export function fetchDnsConfiguredProviders(): Promise<DNSProvider[]> {
  return fetchDnsProviders();
}

export function fetchSupportedDNSProviders(): Promise<DNSSupportedProvider[]> {
  return fetchJSON<DNSSupportedProvider[] | { data: DNSSupportedProvider[] }>("/dns/providers").then((res) => {
    const data = unwrapData(res as DNSSupportedProvider[] | { data: DNSSupportedProvider[] });
    return Array.isArray(data) ? data : [];
  });
}

export function createDNSProvider(input: CreateDNSProviderInput): Promise<DNSProvider> {
  return postJSON<{ data: DNSProvider }>("/dns/providers", {
    name: input.name,
    providerType: input.providerType,
    credentials: input.credentials,
  }).then((res) => unwrapData(res as { data: DNSProvider }));
}

export function createProvider(name: string, providerType: string, credentials: Record<string, string>): Promise<DNSProvider> {
  return createDNSProvider({ name, providerType, credentials });
}

export function verifyDNSProvider(id: string): Promise<{ ok: boolean; verified: boolean }> {
  return postJSON<{ ok: boolean; verified: boolean }>(`/dns/providers/${encodeURIComponent(id)}/verify`, {});
}

export function verifyProvider(id: string): Promise<{ ok: boolean; verified: boolean }> {
  return verifyDNSProvider(id);
}

export function setDefaultDNSProvider(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/dns/providers/${encodeURIComponent(id)}/set-default`, {});
}

export function setDefault(id: string): Promise<{ ok: boolean }> {
  return setDefaultDNSProvider(id);
}

export function deleteDNSProvider(id: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/dns/providers/${encodeURIComponent(id)}`);
}

export function deleteProvider(id: string): Promise<{ ok: boolean }> {
  return deleteDNSProvider(id);
}
