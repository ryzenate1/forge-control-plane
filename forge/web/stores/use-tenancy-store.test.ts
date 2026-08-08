import { describe, expect, it, beforeEach, vi } from "vitest";
import { useTenancyStore } from "./use-tenancy-store";

vi.mock("@/lib/api/tenancy", () => ({
  fetchOrganizations: vi.fn(),
  fetchProjects: vi.fn(),
  fetchEnvironments: vi.fn(),
  fetchTeamMembers: vi.fn(),
}));

import {
  fetchOrganizations,
  fetchProjects,
  fetchEnvironments,
  fetchTeamMembers,
  type Environment,
  type Organization,
  type Project,
  type TeamMember,
} from "@/lib/api/tenancy";

const mockFetchOrgs = vi.mocked(fetchOrganizations);
const mockFetchProjects = vi.mocked(fetchProjects);
const mockFetchEnvs = vi.mocked(fetchEnvironments);
const mockFetchMembers = vi.mocked(fetchTeamMembers);

const organization = (id = "o1", name = "Org 1"): Organization => ({
  id,
  name,
  slug: id,
  ownerId: "owner-1",
  ownerName: "Owner",
  createdAt: "2026-01-01T00:00:00Z",
});

const project = (id = "p1", name = "Project"): Project => ({
  id,
  orgId: "o1",
  name,
  slug: id,
  description: "",
  createdAt: "2026-01-01T00:00:00Z",
});

const environment = (id = "e1", name = "Production"): Environment => ({
  id,
  projectId: "p1",
  name,
  color: "#22c55e",
  protected: false,
  createdAt: "2026-01-01T00:00:00Z",
});

const member = (id = "m1"): TeamMember => ({
  id,
  orgId: "o1",
  userId: "user-1",
  email: "user@example.com",
  role: "member",
  createdAt: "2026-01-01T00:00:00Z",
});

beforeEach(() => {
  vi.clearAllMocks();
  useTenancyStore.getState().reset();
});

describe("useTenancyStore", () => {
  describe("initial state", () => {
    it("has expected defaults", () => {
      const state = useTenancyStore.getState();
      expect(state.organizations).toEqual([]);
      expect(state.activeOrg).toBeNull();
      expect(state.projects).toEqual([]);
      expect(state.activeProject).toBeNull();
      expect(state.environments).toEqual([]);
      expect(state.activeEnvironment).toBeNull();
      expect(state.members).toEqual([]);
      expect(state.loading).toBe(false);
      expect(state.error).toBeNull();
    });
  });

  describe("setOrganizations", () => {
    it("sets organizations", () => {
      const orgs = [organization()];
      useTenancyStore.getState().setOrganizations(orgs);
      expect(useTenancyStore.getState().organizations).toEqual(orgs);
    });
  });

  describe("setActiveOrg", () => {
    it("sets active org and resets dependent state", () => {
      useTenancyStore.getState().setProjects([project()]);
      useTenancyStore.getState().setEnvironments([environment()]);
      const org = organization();
      useTenancyStore.getState().setActiveOrg(org);
      const state = useTenancyStore.getState();
      expect(state.activeOrg).toEqual(org);
      expect(state.projects).toEqual([]);
      expect(state.activeProject).toBeNull();
      expect(state.environments).toEqual([]);
      expect(state.activeEnvironment).toBeNull();
    });

    it("clears active org", () => {
      useTenancyStore.getState().setActiveOrg(organization());
      useTenancyStore.getState().setActiveOrg(null);
      expect(useTenancyStore.getState().activeOrg).toBeNull();
    });
  });

  describe("setActiveProject", () => {
    it("sets active project and resets environments", () => {
      useTenancyStore.getState().setEnvironments([environment()]);
      const selectedProject = project("p1", "Proj");
      useTenancyStore.getState().setActiveProject(selectedProject);
      const state = useTenancyStore.getState();
      expect(state.activeProject).toEqual(selectedProject);
      expect(state.environments).toEqual([]);
      expect(state.activeEnvironment).toBeNull();
    });

    it("clears active project", () => {
      useTenancyStore.getState().setActiveProject(project());
      useTenancyStore.getState().setActiveProject(null);
      expect(useTenancyStore.getState().activeProject).toBeNull();
    });
  });

  describe("setActiveEnvironment", () => {
    it("sets and clears environment", () => {
      const env = environment();
      useTenancyStore.getState().setActiveEnvironment(env);
      expect(useTenancyStore.getState().activeEnvironment).toEqual(env);
      useTenancyStore.getState().setActiveEnvironment(null);
      expect(useTenancyStore.getState().activeEnvironment).toBeNull();
    });
  });

  describe("fetchOrgs", () => {
    it("loads organizations on success", async () => {
      const orgs = [organization()];
      mockFetchOrgs.mockResolvedValueOnce(orgs);
      await useTenancyStore.getState().fetchOrgs();
      const state = useTenancyStore.getState();
      expect(state.organizations).toEqual(orgs);
      expect(state.loading).toBe(false);
      expect(state.error).toBeNull();
    });

    it("sets error on failure", async () => {
      mockFetchOrgs.mockRejectedValueOnce(new Error("network error"));
      await useTenancyStore.getState().fetchOrgs();
      const state = useTenancyStore.getState();
      expect(state.error).toBe("network error");
      expect(state.loading).toBe(false);
    });

    it("handles non-Error exceptions", async () => {
      mockFetchOrgs.mockRejectedValueOnce("string error");
      await useTenancyStore.getState().fetchOrgs();
      expect(useTenancyStore.getState().error).toBe("Failed to load organizations");
    });
  });

  describe("selectOrg", () => {
    it("resets dependent state when org is null", async () => {
      await useTenancyStore.getState().selectOrg(null);
      const state = useTenancyStore.getState();
      expect(state.activeOrg).toBeNull();
      expect(state.projects).toEqual([]);
      expect(state.members).toEqual([]);
    });

    it("fetches projects and members on success", async () => {
      const org = organization();
      const projects = [project()];
      const members = [member()];
      mockFetchProjects.mockResolvedValueOnce(projects);
      mockFetchMembers.mockResolvedValueOnce(members);
      await useTenancyStore.getState().selectOrg(org);
      const state = useTenancyStore.getState();
      expect(state.activeOrg).toEqual(org);
      expect(state.projects).toEqual(projects);
      expect(state.members).toEqual(members);
      expect(state.loading).toBe(false);
    });

    it("sets error on failure", async () => {
      mockFetchProjects.mockRejectedValueOnce(new Error("api down"));
      await useTenancyStore.getState().selectOrg(organization());
      expect(useTenancyStore.getState().error).toBe("api down");
      expect(useTenancyStore.getState().loading).toBe(false);
    });
  });

  describe("selectProject", () => {
    it("resets environments when project is null", async () => {
      useTenancyStore.getState().setEnvironments([environment()]);
      await useTenancyStore.getState().selectProject(null);
      expect(useTenancyStore.getState().activeProject).toBeNull();
      expect(useTenancyStore.getState().environments).toEqual([]);
      expect(useTenancyStore.getState().activeEnvironment).toBeNull();
    });

    it("fetches environments on success", async () => {
      const selectedProject = project();
      const envs = [environment("e1", "staging")];
      mockFetchEnvs.mockResolvedValueOnce(envs);
      await useTenancyStore.getState().selectProject(selectedProject);
      const state = useTenancyStore.getState();
      expect(state.activeProject).toEqual(selectedProject);
      expect(state.environments).toEqual(envs);
      expect(state.loading).toBe(false);
    });

    it("sets error on failure", async () => {
      mockFetchEnvs.mockRejectedValueOnce(new Error("failed"));
      await useTenancyStore.getState().selectProject(project());
      expect(useTenancyStore.getState().error).toBe("failed");
    });
  });

  describe("reset", () => {
    it("returns everything to defaults", async () => {
      mockFetchOrgs.mockResolvedValueOnce([organization()]);
      await useTenancyStore.getState().fetchOrgs();
      useTenancyStore.getState().reset();
      const state = useTenancyStore.getState();
      expect(state.organizations).toEqual([]);
      expect(state.activeOrg).toBeNull();
      expect(state.projects).toEqual([]);
      expect(state.activeProject).toBeNull();
      expect(state.environments).toEqual([]);
      expect(state.activeEnvironment).toBeNull();
      expect(state.members).toEqual([]);
      expect(state.error).toBeNull();
    });
  });
});
