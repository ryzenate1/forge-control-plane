'use client';

import { create } from 'zustand';
import type { Organization, Project, Environment, TeamMember } from '@/lib/api/tenancy';
import {
  fetchOrganizations,
  fetchProjects as apiFetchProjects,
  fetchEnvironments as apiFetchEnvironments,
  fetchTeamMembers,
} from '@/lib/api/tenancy';

let fetchOrgsGeneration = 0;
let selectOrgGeneration = 0;
let projectGeneration = 0;

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
  reset: () => set({ organizations: [], activeOrg: null, projects: [], activeProject: null, environments: [], activeEnvironment: null, members: [], loading: false, error: null }),

  fetchOrgs: async () => {
    const generation = ++fetchOrgsGeneration;
    set({ loading: true, error: null });
    try {
      const orgs = await fetchOrganizations();
      if (generation !== fetchOrgsGeneration) return;
      set({ organizations: orgs, loading: false });
    } catch (err) {
      if (generation !== fetchOrgsGeneration) return;
      set({ error: err instanceof Error ? err.message : 'Failed to load organizations', loading: false });
    }
  },

  selectOrg: async (org) => {
    const generation = ++selectOrgGeneration;
    set({ activeOrg: org, projects: [], activeProject: null, environments: [], activeEnvironment: null, members: [] });
    if (!org) return;
    set({ loading: true, error: null });
    try {
      const results = await Promise.allSettled([
        apiFetchProjects(org.id),
        fetchTeamMembers(org.id),
      ]);
      if (generation !== selectOrgGeneration) return;
      // A rejected branch is reported, not mistaken for "this org has none":
      // the failed slice keeps its previous (here: reset) value and the error
      // names which source failed so the UI can offer a retry.
      const sources = ['projects', 'members'] as const;
      const failures = results
        .map((r, i) => ({ r, source: sources[i] }))
        .filter((x): x is { r: PromiseRejectedResult; source: (typeof sources)[number] } => x.r.status === 'rejected');
      const projects = results[0].status === 'fulfilled' ? results[0].value : [];
      const members = results[1].status === 'fulfilled' ? results[1].value : [];
      const error = failures.length > 0
        ? `Failed to load org details (${failures.map((f) => `${f.source}: ${f.r.reason instanceof Error ? f.r.reason.message : 'unknown error'}`).join('; ')})`
        : null;
      set({ projects, members, loading: false, error });
    } catch (err) {
      if (generation !== selectOrgGeneration) return;
      set({ error: err instanceof Error ? err.message : 'Failed to load org details', loading: false });
    }
  },

  selectProject: async (project) => {
    const generation = ++projectGeneration;
    set({ activeProject: project, environments: [], activeEnvironment: null });
    if (!project) return;
    set({ loading: true, error: null });
    try {
      const environments = await apiFetchEnvironments(project.id);
      if (generation !== projectGeneration) return;
      set({ environments, loading: false });
    } catch (err) {
      if (generation !== projectGeneration) return;
      set({ error: err instanceof Error ? err.message : 'Failed to load environments', loading: false });
    }
  },

  selectEnvironment: (env) => set({ activeEnvironment: env }),
}));
