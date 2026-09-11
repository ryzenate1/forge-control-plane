"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTenancyStore } from "@/stores/use-tenancy-store";
import { ApiError } from "./http";
import { queryKeys } from "./query-keys";
import { fetchOrganizations, fetchProjects, fetchEnvironments } from "./tenancy";

const TENANCY_STALE_TIME = 30_000;

function isForbidden(err: unknown): boolean {
  return err instanceof ApiError && err.status === 403;
}

export function TenancyHydrator() {
  const queryClient = useQueryClient();
  const setOrganizations = useTenancyStore((s) => s.setOrganizations);
  const setActiveOrg = useTenancyStore((s) => s.setActiveOrg);
  const setProjects = useTenancyStore((s) => s.setProjects);
  const setActiveProject = useTenancyStore((s) => s.setActiveProject);
  const setEnvironments = useTenancyStore((s) => s.setEnvironments);
  const setActiveEnvironment = useTenancyStore((s) => s.setActiveEnvironment);
  const setLoading = useTenancyStore((s) => s.setLoading);
  const setError = useTenancyStore((s) => s.setError);

  const orgsQuery = useQuery({
    queryKey: queryKeys.tenancy.organizations(),
    queryFn: fetchOrganizations,
    staleTime: TENANCY_STALE_TIME,
    retry: false,
  });

  useEffect(() => {
    if (orgsQuery.isPending) {
      setLoading(true);
      return;
    }
    setLoading(false);

    if (orgsQuery.error) {
      // Membership revoked (or never granted): drop every cached tenancy
      // subtree for the stale scope and clear the selection explicitly so
      // the UI cannot keep acting on an org the user can no longer access.
      // removeQueries (not invalidateQueries): invalidation would refetch,
      // hit 403 again and loop through this effect.
      if (isForbidden(orgsQuery.error)) {
        void queryClient.removeQueries({ queryKey: queryKeys.tenancy.all });
        setActiveOrg(null);
      }
      setError(orgsQuery.error instanceof Error ? orgsQuery.error.message : String(orgsQuery.error));
      return;
    }

    const orgs = orgsQuery.data ?? [];
    setOrganizations(orgs);

    // Explicit selection: the hydrator only picks a default when there is
    // exactly one candidate (no ambiguity). With several orgs and no valid
    // selection, the choice is left to the user via the scope switcher —
    // silently acting on orgs[0] would resolve an ambiguous target.
    // An existing valid selection is never overridden (refetch flicker).
    const currentActiveOrg = useTenancyStore.getState().activeOrg;
    if (orgs.length === 1) {
      if (!currentActiveOrg || currentActiveOrg.id !== orgs[0].id) {
        setActiveOrg(orgs[0]);
      }
    } else if (orgs.length > 1) {
      if (currentActiveOrg && !orgs.some((o) => o.id === currentActiveOrg.id)) {
        setActiveOrg(null);
      }
    } else if (currentActiveOrg) {
      setActiveOrg(null);
    }
  }, [orgsQuery.data, orgsQuery.error, orgsQuery.isPending, setOrganizations, setActiveOrg, setLoading, setError, queryClient]);

  const activeOrg = useTenancyStore((s) => s.activeOrg);

  const projectsQuery = useQuery({
    queryKey: queryKeys.tenancy.projects(activeOrg?.id ?? "none"),
    queryFn: () => fetchProjects(activeOrg!.id),
    enabled: Boolean(activeOrg?.id),
    staleTime: TENANCY_STALE_TIME,
    retry: false,
  });

  useEffect(() => {
    if (!activeOrg) return;
    if (projectsQuery.error) {
      if (isForbidden(projectsQuery.error)) {
        void queryClient.removeQueries({ queryKey: queryKeys.tenancy.all });
        setActiveOrg(null);
      }
      setError(projectsQuery.error instanceof Error ? projectsQuery.error.message : String(projectsQuery.error));
      return;
    }
    if (projectsQuery.isPending && projectsQuery.data === undefined) return;
    const projects = projectsQuery.data ?? [];
    setProjects(projects);
    const currentActiveProject = useTenancyStore.getState().activeProject;
    if (projects.length === 1) {
      if (!currentActiveProject || currentActiveProject.id !== projects[0].id) {
        setActiveProject(projects[0]);
      }
    } else if (projects.length > 1) {
      if (currentActiveProject && !projects.some((p) => p.id === currentActiveProject.id)) {
        setActiveProject(null);
      }
    } else if (currentActiveProject) {
      setActiveProject(null);
    }
  }, [projectsQuery.data, projectsQuery.error, projectsQuery.isPending, activeOrg, setProjects, setActiveProject, setActiveOrg, setError, queryClient]);

  const activeProject = useTenancyStore((s) => s.activeProject);

  const envsQuery = useQuery({
    queryKey: queryKeys.tenancy.environments(activeProject?.id ?? "none"),
    queryFn: () => fetchEnvironments(activeProject!.id),
    enabled: Boolean(activeProject?.id),
    staleTime: TENANCY_STALE_TIME,
    retry: false,
  });

  useEffect(() => {
    if (!activeProject) return;
    if (envsQuery.error) {
      if (isForbidden(envsQuery.error)) {
        void queryClient.removeQueries({ queryKey: queryKeys.tenancy.all });
        setActiveProject(null);
      }
      setError(envsQuery.error instanceof Error ? envsQuery.error.message : String(envsQuery.error));
      return;
    }
    if (envsQuery.isPending && envsQuery.data === undefined) return;
    const envs = envsQuery.data ?? [];
    setEnvironments(envs);
    const currentActiveEnv = useTenancyStore.getState().activeEnvironment;
    if (envs.length === 1) {
      if (!currentActiveEnv || currentActiveEnv.id !== envs[0].id) {
        setActiveEnvironment(envs[0]);
      }
    } else if (envs.length > 1) {
      if (currentActiveEnv && !envs.some((e) => e.id === currentActiveEnv.id)) {
        setActiveEnvironment(null);
      }
    } else if (currentActiveEnv) {
      setActiveEnvironment(null);
    }
  }, [envsQuery.data, envsQuery.error, envsQuery.isPending, activeProject, setEnvironments, setActiveEnvironment, setActiveProject, setError, queryClient]);

  return null;
}
