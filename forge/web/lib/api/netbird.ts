// Typed client for the NetBird mesh-VPN admin API (/admin/netbird/*).
// Mirrors the style of ./crossnode: every response is unwrapped from the
// `{ data: ... }` envelope the Go handlers emit.
import { fetchJSON, postJSON, patchJSON, deleteJSON } from "./http";

export type NetBirdGroupRef = {
  id: string;
  name: string;
  peers_count?: number;
};

export type NetBirdPeer = {
  id: string;
  name: string;
  hostname?: string;
  ip: string;
  connected: boolean;
  os?: string;
  version?: string;
  user_id?: string;
  dns_label?: string;
  groups?: NetBirdGroupRef[];
  last_seen?: string;
  created_at?: string;
  approval_required?: boolean;
  pending_approval?: boolean;
  ephemeral?: boolean;
  ssh_enabled?: boolean;
  login_expired?: boolean;
  country_code?: string;
  city_name?: string;
};

export type NetBirdNetwork = {
  id: string;
  name: string;
  description?: string;
  policies?: string[];
  resources?: string[];
  routers?: string[];
  routing_peers_count?: number;
};

export type NetBirdGroup = {
  id: string;
  name: string;
  peers?: string[];
  peers_count?: number;
};

export type NetBirdRoute = {
  id: string;
  description?: string;
  network_id?: string;
  network?: string;
  network_type?: string;
  domains?: string[];
  peer?: string;
  peer_groups?: string[];
  groups?: string[];
  access_control_groups?: string[];
  enabled: boolean;
  metric?: number;
  masquerade?: boolean;
  keep_route?: boolean;
};

export type NetBirdACLSubRule = {
  id?: string;
  name?: string;
  description?: string;
  enabled: boolean;
  action?: string;
  protocol?: string;
  bidirectional?: boolean;
  sources?: NetBirdGroupRef[];
  destinations?: NetBirdGroupRef[];
  ports?: string[];
  port_ranges?: { from: number; to: number }[];
};

export type NetBirdACLRule = {
  id?: string;
  name: string;
  description?: string;
  enabled: boolean;
  rules?: NetBirdACLSubRule[];
  revision?: number;
};

export type NetBirdDNSConfig = {
  disabled_management_groups: string[];
};

export type NetBirdSetupKey = {
  id: string;
  key?: string;
  name: string;
  type: string;
  description?: string;
  valid: boolean;
  revoked: boolean;
  state?: string;
  expires?: string;
  last_used?: string;
  used_times?: number;
  usage_limit?: number;
  ephemeral?: boolean;
  auto_groups?: string[];
  allow_custom_name?: boolean;
};

export type NetBirdSetupKeyInput = {
  name: string;
  description?: string;
  type: "one-off" | "reusable";
  expires_in: number;
  used_times?: number;
  allow_custom_name?: boolean;
  auto_groups?: string[];
  ephemeral?: boolean;
};

function unwrap<T>(res: { data: T } | T): T {
  return (res as { data: T }).data ?? (res as T);
}

// --- Peers ---

export async function fetchNetBirdPeers(): Promise<NetBirdPeer[]> {
  const res = await fetchJSON<{ data: NetBirdPeer[] } | NetBirdPeer[]>("/admin/netbird/peers");
  return unwrap<NetBirdPeer[]>(res) ?? [];
}

export async function fetchNetBirdPeer(id: string): Promise<NetBirdPeer | null> {
  const res = await fetchJSON<{ data: NetBirdPeer | null } | NetBirdPeer | null>(`/admin/netbird/peers/${encodeURIComponent(id)}`);
  return unwrap<NetBirdPeer | null>(res) ?? null;
}

export async function approveNetBirdPeer(id: string): Promise<NetBirdPeer> {
  const res = await postJSON<{ data: NetBirdPeer } | NetBirdPeer>(`/admin/netbird/peers/${encodeURIComponent(id)}/approve`, {});
  return unwrap<NetBirdPeer>(res);
}

export async function denyNetBirdPeer(id: string): Promise<NetBirdPeer> {
  const res = await postJSON<{ data: NetBirdPeer } | NetBirdPeer>(`/admin/netbird/peers/${encodeURIComponent(id)}/deny`, {});
  return unwrap<NetBirdPeer>(res);
}

export async function deleteNetBirdPeer(id: string): Promise<void> {
  await deleteJSON(`/admin/netbird/peers/${encodeURIComponent(id)}`);
}

// --- Networks ---

export async function fetchNetBirdNetworks(): Promise<NetBirdNetwork[]> {
  const res = await fetchJSON<{ data: NetBirdNetwork[] } | NetBirdNetwork[]>("/admin/netbird/networks");
  return unwrap<NetBirdNetwork[]>(res) ?? [];
}

export async function createNetBirdNetwork(input: { name: string; description?: string }): Promise<NetBirdNetwork> {
  const res = await postJSON<{ data: NetBirdNetwork } | NetBirdNetwork>("/admin/netbird/networks", input);
  return unwrap<NetBirdNetwork>(res);
}

export async function updateNetBirdNetwork(id: string, input: { name: string; description?: string }): Promise<NetBirdNetwork> {
  const res = await patchJSON<{ data: NetBirdNetwork } | NetBirdNetwork>(`/admin/netbird/networks/${encodeURIComponent(id)}`, input);
  return unwrap<NetBirdNetwork>(res);
}

export async function deleteNetBirdNetwork(id: string): Promise<void> {
  await deleteJSON(`/admin/netbird/networks/${encodeURIComponent(id)}`);
}

// --- Groups ---

export async function fetchNetBirdGroups(): Promise<NetBirdGroup[]> {
  const res = await fetchJSON<{ data: NetBirdGroup[] } | NetBirdGroup[]>("/admin/netbird/groups");
  return unwrap<NetBirdGroup[]>(res) ?? [];
}

export async function createNetBirdGroup(input: { name: string; peers?: string[] }): Promise<NetBirdGroup> {
  const res = await postJSON<{ data: NetBirdGroup } | NetBirdGroup>("/admin/netbird/groups", input);
  return unwrap<NetBirdGroup>(res);
}

export async function deleteNetBirdGroup(id: string): Promise<void> {
  await deleteJSON(`/admin/netbird/groups/${encodeURIComponent(id)}`);
}

// --- Routes ---

export async function fetchNetBirdRoutes(): Promise<NetBirdRoute[]> {
  const res = await fetchJSON<{ data: NetBirdRoute[] } | NetBirdRoute[]>("/admin/netbird/routes");
  return unwrap<NetBirdRoute[]>(res) ?? [];
}

export async function createNetBirdRoute(input: Partial<NetBirdRoute> & { description: string }): Promise<NetBirdRoute> {
  const res = await postJSON<{ data: NetBirdRoute } | NetBirdRoute>("/admin/netbird/routes", input);
  return unwrap<NetBirdRoute>(res);
}

export async function deleteNetBirdRoute(id: string): Promise<void> {
  await deleteJSON(`/admin/netbird/routes/${encodeURIComponent(id)}`);
}

// --- ACLs ---

export async function fetchNetBirdACLs(): Promise<NetBirdACLRule[]> {
  const res = await fetchJSON<{ data: NetBirdACLRule[] } | NetBirdACLRule[]>("/admin/netbird/acls");
  return unwrap<NetBirdACLRule[]>(res) ?? [];
}

export async function createNetBirdACL(input: NetBirdACLRule): Promise<NetBirdACLRule> {
  const res = await postJSON<{ data: NetBirdACLRule } | NetBirdACLRule>("/admin/netbird/acls", input);
  return unwrap<NetBirdACLRule>(res);
}

export async function deleteNetBirdACL(id: string): Promise<void> {
  await deleteJSON(`/admin/netbird/acls/${encodeURIComponent(id)}`);
}

// --- DNS ---

export async function fetchNetBirdDNSSettings(): Promise<NetBirdDNSConfig | null> {
  const res = await fetchJSON<{ data: NetBirdDNSConfig | null } | NetBirdDNSConfig | null>("/admin/netbird/dns");
  return unwrap<NetBirdDNSConfig | null>(res) ?? null;
}

export async function updateNetBirdDNSSettings(input: NetBirdDNSConfig): Promise<NetBirdDNSConfig> {
  const res = await patchJSON<{ data: NetBirdDNSConfig } | NetBirdDNSConfig>("/admin/netbird/dns", input);
  return unwrap<NetBirdDNSConfig>(res);
}

// --- Setup keys ---

export async function createNetBirdSetupKey(input: NetBirdSetupKeyInput): Promise<NetBirdSetupKey> {
  const res = await postJSON<{ data: NetBirdSetupKey } | NetBirdSetupKey>("/admin/netbird/setup-keys", input);
  return unwrap<NetBirdSetupKey>(res);
}

export async function revokeNetBirdSetupKey(id: string): Promise<void> {
  await postJSON(`/admin/netbird/setup-keys/${encodeURIComponent(id)}/revoke`, {});
}
