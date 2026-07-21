export interface Organization {
  id: string;
  name: string;
  slug: string;
  ownerId: string;
  ownerName: string;
  role?: string;
  memberCount?: number;
  createdAt: string;
}

export interface Project {
  id: string;
  orgId: string;
  name: string;
  slug: string;
  description: string;
  createdAt: string;
}

export interface Environment {
  id: string;
  projectId: string;
  name: string;
  color: string;
  protected: boolean;
  createdAt: string;
}

export interface TeamMember {
  id: string;
  orgId: string;
  userId: string;
  email: string;
  role: 'owner' | 'admin' | 'member' | 'viewer';
  createdAt: string;
}

export interface OrgInvitation {
  id: string;
  orgId: string;
  email: string;
  role: string;
  token: string;
  invitedBy?: string;
  acceptedAt?: string;
  expiresAt: string;
  revokedAt?: string;
  createdAt: string;
}

export interface GranularPermissions {
  canCreateProjects: boolean;
  canDeleteProjects: boolean;
  canCreateEnvironments: boolean;
  canDeleteEnvironments: boolean;
  canCreateServices: boolean;
  canDeleteServices: boolean;
  canManageMembers: boolean;
  canManageEnvVars: boolean;
  canManageBackups: boolean;
  canViewSensitiveEnv: boolean;
}

export interface EnvironmentVariable {
  id: string;
  orgId?: string;
  projectId?: string;
  environmentId?: string;
  serviceId?: string;
  scope: string;
  key: string;
  isSensitive: boolean;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface EnvVarRevision {
  id: string;
  variableId: string;
  version: number;
  createdBy?: string;
  createdAt: string;
}

import { fetchJSON, postJSON, putJSON, patchJSON, deleteJSON } from "./http";

// ---- Organizations ----

export function fetchOrganizations(): Promise<Organization[]> {
  return fetchJSON<Organization[]>("/organizations");
}

export function fetchOrganization(identifier: string): Promise<Organization> {
  return fetchJSON<Organization>(`/organizations/${encodeURIComponent(identifier)}`);
}

export function createOrganization(name: string, slug?: string): Promise<Organization> {
  return postJSON<Organization>("/organizations", { name, slug });
}

export function updateOrganization(id: string, name?: string, slug?: string): Promise<void> {
  return patchJSON<void>(`/organizations/${encodeURIComponent(id)}`, { name, slug });
}

export function deleteOrganization(id: string): Promise<void> {
  return deleteJSON<void>(`/organizations/${encodeURIComponent(id)}`);
}

// ---- Projects ----

export function fetchProjects(orgId: string): Promise<Project[]> {
  return fetchJSON<Project[]>(`/organizations/${encodeURIComponent(orgId)}/projects`);
}

export function createProject(orgId: string, name: string, slug?: string, description?: string): Promise<Project> {
  return postJSON<Project>(`/organizations/${encodeURIComponent(orgId)}/projects`, { name, slug, description });
}

export function updateProject(id: string, name: string, slug?: string, description?: string): Promise<Project> {
  return putJSON<Project>(`/projects/${encodeURIComponent(id)}`, { name, slug, description });
}

export function deleteProject(id: string): Promise<void> {
  return deleteJSON<void>(`/projects/${encodeURIComponent(id)}`);
}

// ---- Environments ----

export function fetchEnvironments(projectId: string): Promise<Environment[]> {
  return fetchJSON<Environment[]>(`/projects/${encodeURIComponent(projectId)}/envs`);
}

export function createEnvironment(projectId: string, name: string, color?: string, protected_?: boolean): Promise<Environment> {
  return postJSON<Environment>(`/projects/${encodeURIComponent(projectId)}/envs`, { name, color, protected: protected_ });
}

export function updateEnvironment(id: string, name: string, color?: string, protected_?: boolean): Promise<Environment> {
  return putJSON<Environment>(`/envs/${encodeURIComponent(id)}`, { name, color, protected: protected_ });
}

export function deleteEnvironment(id: string): Promise<void> {
  return deleteJSON<void>(`/envs/${encodeURIComponent(id)}`);
}

// ---- Team Members ----

export function fetchTeamMembers(orgId: string): Promise<TeamMember[]> {
  return fetchJSON<TeamMember[]>(`/organizations/${encodeURIComponent(orgId)}/members`);
}

export function addTeamMember(orgId: string, userId: string, role: string): Promise<TeamMember> {
  return postJSON<TeamMember>(`/organizations/${encodeURIComponent(orgId)}/members`, { userId, role });
}

export function updateTeamMember(orgId: string, userId: string, role: string): Promise<void> {
  return putJSON<void>(`/organizations/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}`, { role });
}

export function removeTeamMember(orgId: string, userId: string): Promise<void> {
  return deleteJSON<void>(`/organizations/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}`);
}

// ---- Invitations ----

export function fetchInvitations(orgId: string): Promise<OrgInvitation[]> {
  return fetchJSON<OrgInvitation[]>(`/organizations/${encodeURIComponent(orgId)}/invitations`);
}

export function createInvitation(orgId: string, email: string, role: string): Promise<OrgInvitation> {
  return postJSON<OrgInvitation>(`/organizations/${encodeURIComponent(orgId)}/invitations`, { email, role });
}

export function acceptInvitation(token: string): Promise<void> {
  return postJSON<void>("/invitations/accept", { token });
}

export function revokeInvitation(orgId: string, invId: string): Promise<void> {
  return deleteJSON<void>(`/organizations/${encodeURIComponent(orgId)}/invitations/${encodeURIComponent(invId)}`);
}

// ---- Granular Permissions ----

export function fetchMemberPermissions(orgId: string, userId: string): Promise<GranularPermissions> {
  return fetchJSON<GranularPermissions>(`/organizations/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}/permissions`);
}

export function updateMemberPermissions(orgId: string, userId: string, permissions: GranularPermissions): Promise<void> {
  return putJSON<void>(`/organizations/${encodeURIComponent(orgId)}/members/${encodeURIComponent(userId)}/permissions`, permissions);
}

// ---- Environment Variables ----

export function fetchEnvVars(envId: string): Promise<EnvironmentVariable[]> {
  return fetchJSON<EnvironmentVariable[]>(`/environments/${encodeURIComponent(envId)}/env-vars`);
}

export function createEnvVar(envId: string, key: string, value: string, isSensitive: boolean): Promise<EnvironmentVariable> {
  return postJSON<EnvironmentVariable>(`/environments/${encodeURIComponent(envId)}/env-vars`, { key, value, isSensitive });
}

export function updateEnvVar(id: string, value: string, isSensitive: boolean): Promise<EnvironmentVariable> {
  return putJSON<EnvironmentVariable>(`/env-vars/${encodeURIComponent(id)}`, { value, isSensitive });
}

export function deleteEnvVar(id: string): Promise<void> {
  return deleteJSON<void>(`/env-vars/${encodeURIComponent(id)}`);
}

export function resolveEnvVars(envId: string): Promise<Record<string, string>> {
  return fetchJSON<Record<string, string>>(`/environments/${encodeURIComponent(envId)}/env-vars/resolved`);
}

export function fetchEnvVarRevisions(varId: string): Promise<EnvVarRevision[]> {
  return fetchJSON<EnvVarRevision[]>(`/env-vars/${encodeURIComponent(varId)}/revisions`);
}

export function fetchProjectEnvVars(projectId: string): Promise<EnvironmentVariable[]> {
  return fetchJSON<EnvironmentVariable[]>(`/projects/${encodeURIComponent(projectId)}/env-vars`);
}

export function createProjectEnvVar(projectId: string, key: string, value: string, isSensitive: boolean): Promise<EnvironmentVariable> {
  return postJSON<EnvironmentVariable>(`/projects/${encodeURIComponent(projectId)}/env-vars`, { key, value, isSensitive });
}
