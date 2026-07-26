'use client';

import { create } from 'zustand';
import type { Organization, Project, Environment, TeamMember } from '@/lib/api/tenancy';
import {
  fetchOrganizations,
  fetchProjects as apiFetchProjects,
  fetchEnvironments as apiFetchEnvironments,
  fetchTeamMembers,
} from '@/lib/api/tenancy';

interface TenancyState {
  organizations: Organization[];
  activeOrg: Organization | null;
  projects: Project[];
  activeProject: Project | null;
  environments: Environment[];
  activeEnvironment: Environment | null;
  members: TeamMember[];
  loading: boolean;
  error: string | null;

  setOrganizations: (orgs: Organization[]) => void;
  setActiveOrg: (org: Organization | null) => void;
  setProjects: (projects: Project[]) => void;
  setActiveProject: (project: Project | null) => void;
  setEnvironments: (envs: Environment[]) => void;
  setActiveEnvironment: (env: Environment | null) => void;
  setMembers: (members: TeamMember[]) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;

  fetchOrgs: () => Promise<void>;
  selectOrg: (org: Organization | null) => Promise<void>;
  selectProject: (project: Project | null) => Promise<void>;
  selectEnvironment: (env: Environment | null) => void;
}

export const useTenancyStore = create<TenancyState>((set) => ({
  organizations: [],
  activeOrg: null,
  projects: [],
  activeProject: null,
  environments: [],
  activeEnvironment: null,
  members: [],
  loading: false,
  error: null,

  setOrganizations: (organizations) => set({ organizations }),
  setActiveOrg: (activeOrg) => set({ activeOrg, projects: [], activeProject: null, environments: [], activeEnvironment: null }),
  setProjects: (projects) => set({ projects }),
  setActiveProject: (activeProject) => set({ activeProject, environments: [], activeEnvironment: null }),
  setEnvironments: (environments) => set({ environments }),
  setActiveEnvironment: (activeEnvironment) => set({ activeEnvironment }),
  setMembers: (members) => set({ members }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set({ organizations: [], activeOrg: null, projects: [], activeProject: null, environments: [], activeEnvironment: null, members: [], error: null }),

  fetchOrgs: async () => {
    set({ loading: true, error: null });
    try {
      const orgs = await fetchOrganizations();
      set({ organizations: orgs, loading: false });
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to load organizations', loading: false });
    }
  },

  selectOrg: async (org) => {
    set({ activeOrg: org, projects: [], activeProject: null, environments: [], activeEnvironment: null, members: [] });
    if (!org) return;
    set({ loading: true, error: null });
    try {
      const results = await Promise.allSettled([
        apiFetchProjects(org.id),
        fetchTeamMembers(org.id),
      ]);
      const projects = results[0].status === 'fulfilled' ? results[0].value : [];
      const members = results[1].status === 'fulfilled' ? results[1].value : [];
      const errors = results.filter((r) => r.status === 'rejected').map((r) => (r as PromiseRejectedResult).reason);
      set({ projects, members, loading: false, error: errors.length > 0 ? (errors[0] instanceof Error ? errors[0].message : 'Failed to load org details') : null });
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to load org details', loading: false });
    }
  },

  selectProject: async (project) => {
    set({ activeProject: project, environments: [], activeEnvironment: null });
    if (!project) return;
    set({ loading: true, error: null });
    try {
      const environments = await apiFetchEnvironments(project.id);
      set({ environments, loading: false });
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to load environments', loading: false });
    }
  },

  selectEnvironment: (env) => set({ activeEnvironment: env }),
}));
