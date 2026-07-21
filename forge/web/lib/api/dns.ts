import { fetchJSON } from "./http";

export type DNSProvider = {
  id: string;
  name: string;
  provider: string;
  verificationStatus?: string;
  createdAt: string;
};

export function fetchDnsProviders(): Promise<DNSProvider[]> {
  return fetchJSON<DNSProvider[]>("/dns/providers");
}
