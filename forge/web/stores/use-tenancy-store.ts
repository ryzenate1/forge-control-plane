'use client';

import { create } from 'zustand';
import type { Organization, Project, Environment, TeamMember } from '@/lib/api/tenancy';

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
}));
