import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

export interface FirewallStatus {
  enabled: boolean;
  active: boolean;
}

export interface FirewallRule {
  id: string;
  port?: number;
  protocol?: string;
  sourceIp?: string;
  action?: string;
  description?: string;
}

export interface PortForward {
  id: string;
  fromPort: number;
  toPort: number;
  toIp: string;
  protocol: string;
  description?: string;
}

export interface AddRuleInput {
  port?: number;
  protocol?: string;
  sourceIp?: string;
  action?: string;
  description?: string;
}

export interface UpdateRuleInput {
  port?: number;
  protocol?: string;
  sourceIp?: string;
  action?: string;
  description?: string;
}

export interface AddForwardInput {
  fromPort: number;
  toPort: number;
  toIp: string;
  protocol: string;
  description?: string;
}

function nodeParam(nodeId?: string): string {
  return nodeId ? `?nodeId=${encodeURIComponent(nodeId)}` : "";
}

export function fetchFirewallStatus(nodeId?: string): Promise<FirewallStatus> {
  return fetchJSON<FirewallStatus>(`/host/firewall/status${nodeParam(nodeId)}`);
}

export function enableFirewall(nodeId?: string): Promise<FirewallStatus> {
  return postJSON<FirewallStatus>(`/host/firewall/enable${nodeParam(nodeId)}`);
}

export function disableFirewall(nodeId?: string): Promise<FirewallStatus> {
  return postJSON<FirewallStatus>(`/host/firewall/disable${nodeParam(nodeId)}`);
}

export function fetchFirewallRules(nodeId?: string): Promise<FirewallRule[]> {
  return fetchJSON<FirewallRule[]>(`/host/firewall/rules${nodeParam(nodeId)}`);
}

export function addFirewallRule(input: AddRuleInput, nodeId?: string): Promise<FirewallRule> {
  return postJSON<FirewallRule>(`/host/firewall/rules${nodeParam(nodeId)}`, input);
}

export function updateFirewallRule(id: string, input: UpdateRuleInput, nodeId?: string): Promise<FirewallRule> {
  return putJSON<FirewallRule>(`/host/firewall/rules/${encodeURIComponent(id)}${nodeParam(nodeId)}`, input);
}

export function deleteFirewallRule(id: string, nodeId?: string): Promise<void> {
  return deleteJSON<void>(`/host/firewall/rules/${encodeURIComponent(id)}${nodeParam(nodeId)}`);
}

export function fetchPortForwards(nodeId?: string): Promise<PortForward[]> {
  return fetchJSON<PortForward[]>(`/host/firewall/forward${nodeParam(nodeId)}`);
}

export function addPortForward(input: AddForwardInput, nodeId?: string): Promise<PortForward> {
  return postJSON<PortForward>(`/host/firewall/forward${nodeParam(nodeId)}`, input);
}

export function deletePortForward(id: string, nodeId?: string): Promise<void> {
  return deleteJSON<void>(`/host/firewall/forward/${encodeURIComponent(id)}${nodeParam(nodeId)}`);
}

export function openFirewallPort(body: Record<string, unknown>, nodeId?: string): Promise<unknown> {
  return postJSON<unknown>(`/host/firewall/port${nodeParam(nodeId)}`, body);
}
