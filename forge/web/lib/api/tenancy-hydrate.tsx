"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTenancyStore } from "@/stores/use-tenancy-store";
import { fetchOrganizations, fetchProjects, fetchEnvironments } from "./tenancy";

export function TenancyHydrator() {
  const setOrganizations = useTenancyStore((s) => s.setOrganizations);
  const setActiveOrg = useTenancyStore((s) => s.setActiveOrg);
  const setProjects = useTenancyStore((s) => s.setProjects);
  const setActiveProject = useTenancyStore((s) => s.setActiveProject);
  const setEnvironments = useTenancyStore((s) => s.setEnvironments);
  const setActiveEnvironment = useTenancyStore((s) => s.setActiveEnvironment);
  const setLoading = useTenancyStore((s) => s.setLoading);
  const setError = useTenancyStore((s) => s.setError);

  const orgsQuery = useQuery({
    queryKey: ["organizations"],
    queryFn: fetchOrganizations,
    staleTime: 60_000,
  });

  useEffect(() => {
    if (orgsQuery.isPending) {
      setLoading(true);
      return;
    }
    setLoading(false);

    if (orgsQuery.error) {
      setError(String(orgsQuery.error));
      return;
    }

    const orgs = orgsQuery.data ?? [];
    setOrganizations(orgs);

    const currentActiveOrg = useTenancyStore.getState().activeOrg;
    if (orgs.length > 0) {
      if (!currentActiveOrg || !orgs.some((o) => o.id === currentActiveOrg.id)) {
        setActiveOrg(orgs[0]);
      }
    } else if (currentActiveOrg) {
      setActiveOrg(null);
    }
  }, [orgsQuery.data, orgsQuery.error, orgsQuery.isPending, setOrganizations, setActiveOrg, setLoading, setError]);

  const activeOrg = useTenancyStore((s) => s.activeOrg);

  const projectsQuery = useQuery({
    queryKey: ["projects", activeOrg?.id],
    queryFn: () => fetchProjects(activeOrg!.id),
    enabled: Boolean(activeOrg),
    staleTime: 60_000,
  });

  useEffect(() => {
    if (!activeOrg) return;
    const projects = projectsQuery.data ?? [];
    setProjects(projects);
    const currentActiveProject = useTenancyStore.getState().activeProject;
    if (projects.length > 0) {
      if (!currentActiveProject || !projects.some((p) => p.id === currentActiveProject.id)) {
        setActiveProject(projects[0]);
      }
    } else if (currentActiveProject) {
      setActiveProject(null);
    }
  }, [projectsQuery.data, activeOrg, setProjects, setActiveProject]);

  const activeProject = useTenancyStore((s) => s.activeProject);

  const envsQuery = useQuery({
    queryKey: ["environments", activeProject?.id],
    queryFn: () => fetchEnvironments(activeProject!.id),
    enabled: Boolean(activeProject),
    staleTime: 60_000,
  });

  useEffect(() => {
    if (!activeProject) return;
    const envs = envsQuery.data ?? [];
    setEnvironments(envs);
    const currentActiveEnv = useTenancyStore.getState().activeEnvironment;
    if (envs.length > 0) {
      if (!currentActiveEnv || !envs.some((e) => e.id === currentActiveEnv.id)) {
        setActiveEnvironment(envs[0]);
      }
    } else if (currentActiveEnv) {
      setActiveEnvironment(null);
    }
  }, [envsQuery.data, activeProject, setEnvironments, setActiveEnvironment]);

  return null;
}
