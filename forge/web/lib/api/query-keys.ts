import type { QueryClient } from "@tanstack/react-query";

/**
 * Central queryKeyFactory for stale query keys.
 * Ensures consistent keys for servers, nodes, backups, domains
 * so invalidation covers all variants (e.g. ["nodes"] and ["nodes","all"]).
 */
export const queryKeys = {
  servers: {
    all: ["servers"] as const,
    lists: () => [...queryKeys.servers.all] as const,
    allLists: () => [...queryKeys.servers.all, "all"] as const,
    detail: (id: string) => [...queryKeys.servers.all, "detail", id] as const,
    page: (page: number, perPage: number) => [...queryKeys.servers.all, "page", page, perPage] as const,
  },
  nodes: {
    all: ["nodes"] as const,
    /** Base list key ["nodes"] */
    lists: () => [...queryKeys.nodes.all] as const,
    /** Aggregated all-pages key ["nodes","all"] used by fetchAllNodes */
    allLists: () => [...queryKeys.nodes.all, "all"] as const,
    detail: (id: string) => [...queryKeys.nodes.all, "detail", id] as const,
    allocations: (nodeId: string) => [...queryKeys.nodes.all, "allocations", nodeId] as const,
    lifecycle: (nodeId: string) => [...queryKeys.nodes.all, "lifecycle", nodeId] as const,
    servers: (nodeId: string) => [...queryKeys.nodes.all, "servers", nodeId] as const,
  },
  backups: {
    all: ["backups"] as const,
    lists: () => [...queryKeys.backups.all] as const,
    byServer: (serverId: string) => [...queryKeys.backups.all, "server", serverId] as const,
    admin: {
      all: ["admin", "backups"] as const,
      configs: () => [...queryKeys.backups.admin.all, "configs"] as const,
      jobs: () => [...queryKeys.backups.admin.all, "jobs"] as const,
      artifacts: () => [...queryKeys.backups.admin.all, "artifacts"] as const,
      restores: () => [...queryKeys.backups.admin.all, "restores"] as const,
      status: () => [...queryKeys.backups.admin.all, "status"] as const,
      /**
       * Restic / Kopia engine surface (/admin/backup-engines). Nested under the
       * admin backup prefix so `invalidateBackups` reaches it too.
       *
       * Snapshot keys are per-repository because listing snapshots invokes the
       * engine CLI against one repository — there is no fleet-wide snapshot
       * read to cache under a single key.
       */
      engines: {
        all: ["admin", "backups", "engines"] as const,
        repositories: () => [...queryKeys.backups.admin.engines.all, "repositories"] as const,
        snapshots: (repoId: string) => [...queryKeys.backups.admin.engines.all, "snapshots", repoId] as const,
        restores: (repoId: string) => [...queryKeys.backups.admin.engines.all, "restores", repoId] as const,
      },
    },
  },
  domains: {
    all: ["domains"] as const,
    lists: () => [...queryKeys.domains.all] as const,
    byServer: (serverId: string | "all") => [...queryKeys.domains.all, serverId] as const,
    detail: (id: string) => [...queryKeys.domains.all, "detail", id] as const,
  },
  /**
   * Operational telemetry read by the admin Overview / Monitoring / Health
   * experiences. These three pages read the same sources, so they must agree on
   * the keys: a disagreement means one page shows a cached reading the others
   * have already refreshed, which is a stale-data-presented-as-live defect.
   */
  health: {
    all: ["health"] as const,
    /** Control-plane diagnostic report — GET /health */
    report: () => [...queryKeys.health.all, "report"] as const,
    /** Live health-check runner targets — GET /target-health/targets */
    targets: () => [...queryKeys.health.all, "targets"] as const,
  },
  monitoring: {
    all: ["monitoring"] as const,
    /** Latest reported row per node — GET /monitoring/nodes/metrics (no nodeId) */
    latest: () => [...queryKeys.monitoring.all, "latest"] as const,
    /** Observability summary — GET /monitoring/summary */
    summary: () => [...queryKeys.monitoring.all, "summary"] as const,
    /**
     * Metric history — GET /monitoring/nodes/metrics?nodeId=…
     *
     * The `"fleet"` segment is a *shape* discriminator, not decoration. This key
     * caches a `PartialFleet<NodeMetrics>` (`{rows, failedNodeIds, skippedNodeIds}`)
     * because fleet-wide history is a client fan-out that must report which
     * nodes it could not read. A second reader once cached a bare
     * `NodeMetrics[]` under the same `(scope, period)` pair; whichever query
     * resolved first won the cache entry and the other consumer read the wrong
     * shape — `.rows` of an array is `undefined`, `.map` of an object throws.
     * Keeping the discriminator here means a differently-shaped reader cannot
     * silently occupy this entry.
     */
    history: (scope: string, period: string) =>
      [...queryKeys.monitoring.all, "history", "fleet", scope, period] as const,
  },
  activity: {
    all: ["activity"] as const,
    admin: (limit: number) => [...queryKeys.activity.all, "admin", limit] as const,
    audit: () => [...queryKeys.activity.all, "audit"] as const,
  },
  reservations: { all: ["reservations"] as const },
  recovery: { all: ["recovery"] as const },
  users: { all: ["users"] as const },
  apps: {
    all: ["apps"] as const,
    lists: () => [...queryKeys.apps.all] as const,
    detail: (id: string) => [...queryKeys.apps.all, "detail", id] as const,
    store: (category: string, search: string) => [...queryKeys.apps.all, "store", category, search] as const,
  },
  session: {
    all: ["session"] as const,
    currentUser: () => [...queryKeys.session.all, "currentUser"] as const,
  },
  tenancy: {
    all: ["tenancy"] as const,
    /** Organization list — read by TenancyHydrator and the scope switcher. */
    organizations: () => [...queryKeys.tenancy.all, "organizations"] as const,
    /** Projects scoped to one org. */
    projects: (orgId: string) => [...queryKeys.tenancy.all, "projects", orgId] as const,
    /** Environments scoped to one project. */
    environments: (projectId: string) => [...queryKeys.tenancy.all, "environments", projectId] as const,
  },
  envVars: {
    all: ["env-vars"] as const,
    /** Variables scoped to a project or environment scope id. */
    byScope: (scope: "project" | "environment", scopeId: string) =>
      [...queryKeys.envVars.all, scope, scopeId] as const,
    byEnv: (envId: string) => [...queryKeys.envVars.all, "environment", envId] as const,
  },
  files: {
    all: ["files"] as const,
    byServer: (serverId: string, path?: string): readonly string[] =>
      path === undefined
        ? [...queryKeys.files.all, "server", serverId]
        : [...queryKeys.files.all, "server", serverId, path],
  },
  deployments: {
    all: ["deployments"] as const,
    byApp: (appId: string) => [...queryKeys.deployments.all, "app", appId] as const,
  },
} as const;

/**
 * Invalidate all node-related queries. Covers both ["nodes"] and ["nodes","all"]
 * because some views use fetchNodes (["nodes"]) and others use fetchAllNodes (["nodes","all"]).
 * A prefix invalidation on ["nodes"] already matches ["nodes","all"] unless exact:true,
 * but we invalidate both explicitly to avoid stale data when callers use exact:true.
 */
export function invalidateNodes(queryClient: QueryClient): void {
  void queryClient.invalidateQueries({ queryKey: queryKeys.nodes.all });
  void queryClient.invalidateQueries({ queryKey: queryKeys.nodes.allLists() });
}

export function invalidateServers(queryClient: QueryClient): void {
  void queryClient.invalidateQueries({ queryKey: queryKeys.servers.all });
  void queryClient.invalidateQueries({ queryKey: queryKeys.servers.allLists() });
}

export function invalidateBackups(queryClient: QueryClient, serverId?: string): void {
  if (serverId) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.backups.byServer(serverId) });
  }
  void queryClient.invalidateQueries({ queryKey: queryKeys.backups.all });
  void queryClient.invalidateQueries({ queryKey: queryKeys.backups.admin.all });
}

export function invalidateDomains(queryClient: QueryClient, serverId?: string): void {
  if (serverId) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.domains.byServer(serverId) });
  }
  void queryClient.invalidateQueries({ queryKey: queryKeys.domains.all });
}

/**
 * Invalidate cached env-var reads for a scope. Prefix-scoped so both the
 * `byScope` and legacy `byEnv` keys for the same environment are covered.
 */
export function invalidateEnvVars(
  queryClient: QueryClient,
  scope?: { type: "project" | "environment"; id: string } | { envId: string },
): void {
  if (!scope) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.envVars.all });
    return;
  }
  if ("envId" in scope) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.envVars.byEnv(scope.envId) });
    return;
  }
  void queryClient.invalidateQueries({ queryKey: queryKeys.envVars.byScope(scope.type, scope.id) });
}

export function invalidateFiles(queryClient: QueryClient, serverId?: string): void {
  if (serverId) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.files.byServer(serverId) });
    return;
  }
  void queryClient.invalidateQueries({ queryKey: queryKeys.files.all });
}

export function invalidateDeployments(queryClient: QueryClient, appId?: string): void {
  if (appId) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.deployments.byApp(appId) });
  }
  void queryClient.invalidateQueries({ queryKey: queryKeys.deployments.all });
}
