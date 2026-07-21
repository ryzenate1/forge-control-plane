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
  const setMembers = useTenancyStore((s) => s.setMembers);
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

    if (orgs.length > 0 && !useTenancyStore.getState().activeOrg) {
      setActiveOrg(orgs[0]);
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
    if (projects.length > 0 && !useTenancyStore.getState().activeProject) {
      setActiveProject(projects[0]);
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
    if (envs.length > 0 && !useTenancyStore.getState().activeEnvironment) {
      setActiveEnvironment(envs[0]);
    }
  }, [envsQuery.data, activeProject, setEnvironments, setActiveEnvironment]);

  return null;
}
