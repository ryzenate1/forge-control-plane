import type { FC } from "react";
import type { LucideIcon } from "lucide-react";
import type { IconProps } from "@/components/ui/forge-icons";
import { humanizeSegment, isPathWithin, resolveActive } from "@/lib/nav/active";
import type { NavCrumb } from "@/lib/nav/types";
import {
  OverviewDashboardIcon,
  MonitoringPulseIcon,
  HealthECGIcon,
  ActivityWaveIcon,
  ServerRackIcon,
  ApplicationsCubeIcon,
  DatabaseCylinderIcon,
  GamepadIcon,
  AppStoreIcon,
  TemplateSheetIcon,
  RocketLaunchIcon,
  PipelineFlowIcon,
  GitBranchTreeIcon,
  ComposeSheetsIcon,
  NodeHostIcon,
  GlobeGridIcon,
  LocationPinIcon,
  ContainerShippingIcon,
  EndpointPlugIcon,
  GatewayRouterIcon,
  LoadBalancerSplitIcon,
  NetworkMeshNodesIcon,
  NotificationBellIcon,
  SettingsCogIcon,
  StoragePlattersIcon,
} from "@/components/ui/forge-icons";
import {
  Activity, Archive, ArrowLeftRight, Award, BarChart3, Boxes, Building2, Bug, Cable, Clock, Cloud, Code2, Compass,
  Cpu, CreditCard, Droplets, Eye, FileCode, FileText, Fingerprint, FlaskConical, FolderLock,
  GitPullRequest, Globe, HardDrive, KeyRound, Layers, LifeBuoy, Lock, Mail, Network, Plug,
  ArrowUpCircle, Repeat, Route, Scale, Shield, ShieldAlert, ShieldCheck, SlidersHorizontal,
  Tags, Terminal, Ticket, Trash2, TrendingUp, Users, Webhook, Workflow,
} from "lucide-react";

export type NavIcon = LucideIcon | FC<IconProps>;

export type AdminNavEntry = {
  label: string;
  labelKey: string;
  href: string;
  icon: NavIcon;
  requiredRole: "admin";
  capability: "available" | "metadata-only";
  description: string;
  descriptionKey: string;
  /** Optional: marks items that can show a State-Lanes two-dot pending badge */
  hasPendingGenerations?: boolean;
  /** Secondary items stay searchable + routable but collapse under "More" in the sidebar. */
  secondary?: boolean;
  /**
   * Registered for active-state and breadcrumb resolution but never rendered as
   * a nav row. Use for real pages that belong *underneath* another page rather
   * than beside it (e.g. /admin/health is a view of Monitoring, not a sibling).
   */
  hidden?: boolean;
  /** href of the entry this page sits under. Drives the breadcrumb chain. */
  parent?: string;
  /** Extra terms matched by sidebar filter and command palette. */
  keywords?: string[];
};

export type AdminNavGroup = { title: string; titleKey: string; items: AdminNavEntry[] };

/**
 * Forge control-plane information architecture.
 *
 * The groups read as the operator's own narrative, in order:
 *
 *   OVERVIEW      — what is happening right now
 *   WORKLOADS     — the things Forge runs, and the definitions they come from
 *   DEPLOY        — how change reaches those workloads
 *   INFRASTRUCTURE— the machines and runtimes underneath
 *   NETWORKING    — how traffic reaches a workload
 *   OPERATIONS    — keeping a running fleet healthy (day 2)
 *   ACCESS        — who may do any of it (orgs → projects → environments → users)
 *   PLATFORM      — Forge itself
 *
 * Progressive disclosure: each group leads with the handful of destinations a
 * new operator needs, and everything specialised carries `secondary: true` so
 * it collapses under "More" while staying in search, the command palette and
 * breadcrumbs. Group membership — not alphabetical order — is the teaching
 * tool; a reader who has never seen the codebase should be able to guess which
 * group holds a capability.
 *
 * Invariants enforced by test/route-integrity.test.ts:
 *  - every `href` resolves to a real page under app/ (or is an alias source)
 *  - no href appears twice
 *  - exactly one entry is active for any pathname (longest match wins)
 *  - every `parent` points at a registered href
 *
 * Deliberate structural decisions, so they are not "fixed" back:
 *  - `/admin/health` (AdminHealth) is a *view* of Monitoring, registered hidden
 *    with `parent: "/admin/monitoring"`. It was previously a second Overview
 *    row pointing at the same component reachable via /admin/monitoring/[section].
 *  - Database Services is a tab of /admin/databases, not a nav row.
 *  - "Backup Engines" (/admin/backups/engines) is a secondary child of Backups.
 *    The classic pipeline (policies, jobs, artifacts) and the Restic/Kopia
 *    engine surface are different subsystems with different APIs, so they are
 *    two pages rather than one page with a mode switch.
 *  - Security Headers, Private CA & mTLS and Vault moved from Networking to
 *    Access — they are credential/authorization surfaces, not traffic surfaces.
 *  - Detail routes (/admin/nodes/<id>, /admin/apps/<id>/git, …) are NOT listed.
 *    They resolve by prefix and get their trailing crumbs from
 *    ADMIN_SEGMENT_LABELS + humanizeSegment.
 */
export const adminPageRegistry: AdminNavGroup[] = [
  { title: "Overview", titleKey: "admin.navGroup.overview", items: [
    { label: "Overview", labelKey: "admin.nav.overview", href: "/admin/overview", icon: OverviewDashboardIcon, requiredRole: "admin", capability: "available", description: "Live control-plane summary and fleet health at a glance", descriptionKey: "admin.navDesc.overview", keywords: ["home", "dashboard", "start"] },
    { label: "Monitoring", labelKey: "admin.nav.monitoring", href: "/admin/monitoring", icon: MonitoringPulseIcon, requiredRole: "admin", capability: "available", description: "Platform, node and workload health dashboards", descriptionKey: "admin.navDesc.monitoring", keywords: ["metrics", "observability", "graphs", "health"] },
    { label: "Diagnostics", labelKey: "admin.nav.health", href: "/admin/health", icon: HealthECGIcon, requiredRole: "admin", capability: "available", description: "Dependency and service diagnostics for the control plane", descriptionKey: "admin.navDesc.health", hidden: true, parent: "/admin/monitoring", keywords: ["health", "checks", "dependencies"] },
    { label: "Activity", labelKey: "admin.nav.activity", href: "/admin/activity", icon: ActivityWaveIcon, requiredRole: "admin", capability: "available", description: "Human-readable audit and activity history", descriptionKey: "admin.navDesc.activity", keywords: ["audit", "logs", "history", "events"] },
  ]},

  { title: "Workloads", titleKey: "admin.navGroup.workloads", items: [
    { label: "Game Servers", labelKey: "admin.nav.servers", href: "/admin/servers", icon: GamepadIcon, requiredRole: "admin", capability: "available", description: "Game server instances and their lifecycle", descriptionKey: "admin.navDesc.servers", keywords: ["instances", "workloads", "minecraft"] },
    { label: "Applications", labelKey: "admin.nav.apps", href: "/admin/apps", icon: ApplicationsCubeIcon, requiredRole: "admin", capability: "available", description: "Container applications built from images or Git", descriptionKey: "admin.navDesc.apps", keywords: ["containers", "services", "web"] },
    { label: "Databases", labelKey: "admin.nav.databaseHosts", href: "/admin/databases", icon: DatabaseCylinderIcon, requiredRole: "admin", capability: "available", description: "Database inventory: hosts, managed instances and linked services", descriptionKey: "admin.navDesc.databaseHosts", keywords: ["postgres", "mysql", "redis", "database services", "hosts"] },

    { label: "Service Catalog", labelKey: "admin.nav.catalog", href: "/admin/catalog", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "Provision managed services (databases, caches, queues) from the catalog", descriptionKey: "admin.navDesc.catalog", secondary: true, keywords: ["provision", "managed", "one-click"] },
    { label: "App Store", labelKey: "admin.nav.appStore", href: "/admin/app-store", icon: AppStoreIcon, requiredRole: "admin", capability: "available", description: "Browse and install pre-built applications", descriptionKey: "admin.navDesc.appStore", secondary: true, keywords: ["marketplace", "install", "one-click"] },
    { label: "Service Definitions", labelKey: "admin.nav.nestsEggs", href: "/admin/nests", icon: Layers, requiredRole: "admin", capability: "available", description: "Reusable game-server blueprints (nests and eggs)", descriptionKey: "admin.navDesc.nestsEggs", secondary: true, keywords: ["nests", "eggs", "blueprints", "variables"] },
    { label: "App Templates", labelKey: "admin.nav.appTemplates", href: "/admin/app-templates", icon: FileCode, requiredRole: "admin", capability: "available", description: "Application deployment blueprints", descriptionKey: "admin.navDesc.appTemplates", secondary: true, keywords: ["blueprints"] },
    { label: "Legacy Templates", labelKey: "admin.nav.compatibilityTemplates", href: "/admin/templates", icon: Archive, requiredRole: "admin", capability: "available", description: "Legacy compatibility templates kept for imported installs", descriptionKey: "admin.navDesc.compatibilityTemplates", secondary: true },
    { label: "Image Registries", labelKey: "admin.nav.registries", href: "/admin/registries", icon: Boxes, requiredRole: "admin", capability: "available", description: "Private Docker registry credentials for image pull and push", descriptionKey: "admin.navDesc.registries", secondary: true, keywords: ["docker hub", "ghcr", "credentials"] },
    { label: "Forgefile", labelKey: "admin.nav.forgefile", href: "/admin/forgefile", icon: FileText, requiredRole: "admin", capability: "available", description: "Environment-as-code: validate and apply a Forgefile", descriptionKey: "admin.navDesc.forgefile", secondary: true, keywords: ["as code", "iac", "declarative"] },
    { label: "App Storage", labelKey: "admin.nav.appMounts", href: "/admin/app-mounts", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Per-application persistent storage: volumes, binds, tmpfs and seed files", descriptionKey: "admin.navDesc.appMounts", secondary: true, keywords: ["volumes", "mounts", "persistence"] },
    { label: "Tags", labelKey: "admin.nav.tags", href: "/admin/tags", icon: Tags, requiredRole: "admin", capability: "available", description: "Colour-coded labels for organising resources", descriptionKey: "admin.navDesc.tags", secondary: true, keywords: ["labels"] },
  ]},

  { title: "Deploy", titleKey: "admin.navGroup.delivery", items: [
    { label: "Deployments", labelKey: "admin.nav.deployments", href: "/admin/deployments", icon: RocketLaunchIcon, requiredRole: "admin", capability: "available", description: "Rolling and blue-green deployments, revisions and history", descriptionKey: "admin.navDesc.deployments", keywords: ["release", "rollout", "revisions", "history"] },
    { label: "Compose Stacks", labelKey: "admin.nav.compose", href: "/admin/compose", icon: ComposeSheetsIcon, requiredRole: "admin", capability: "available", description: "Docker Compose stacks and imports", descriptionKey: "admin.navDesc.compose", keywords: ["docker-compose", "stack", "yaml"] },
    { label: "Pipelines", labelKey: "admin.nav.pipelines", href: "/admin/pipelines", icon: PipelineFlowIcon, requiredRole: "admin", capability: "available", description: "CI/CD pipelines and delivery workflows", descriptionKey: "admin.navDesc.pipelines", keywords: ["ci", "cd", "build"] },
    { label: "Git Integrations", labelKey: "admin.nav.gitConnections", href: "/admin/git", icon: GitBranchTreeIcon, requiredRole: "admin", capability: "available", description: "Git providers, credentials and connected sources", descriptionKey: "admin.navDesc.gitConnections", keywords: ["github", "gitlab", "providers", "webhooks"] },

    { label: "Stack Templates", labelKey: "admin.nav.stackTemplates", href: "/admin/compose-templates", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "Reusable, parameterised compose stack templates", descriptionKey: "admin.navDesc.stackTemplates", secondary: true },
    { label: "Preview Environments", labelKey: "admin.nav.previewEnvironments", href: "/admin/preview-environments", icon: FlaskConical, requiredRole: "admin", capability: "available", description: "Ephemeral per-branch environments scoped to a project", descriptionKey: "admin.navDesc.previewEnvironments", secondary: true, keywords: ["branch", "ephemeral"] },
    { label: "Preview Deployments", labelKey: "admin.nav.previewDeployments", href: "/admin/preview-deployments", icon: Eye, requiredRole: "admin", capability: "available", description: "Pull-request preview deployments", descriptionKey: "admin.navDesc.previewDeployments", secondary: true, keywords: ["pull request", "pr"] },
    { label: "Source Deployments", labelKey: "admin.nav.sourceDeployments", href: "/admin/source-deployments", icon: GitPullRequest, requiredRole: "admin", capability: "available", description: "Build and deploy directly from Git sources", descriptionKey: "admin.navDesc.sourceDeployments", secondary: true, keywords: ["buildpack", "from source"] },
    { label: "Zero-Downtime Releases", labelKey: "admin.nav.zeroDowntime", href: "/admin/zerodowntime", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Zero-downtime release strategy and health gates", descriptionKey: "admin.navDesc.zeroDowntime", secondary: true, keywords: ["drain", "health gate"] },
  ]},

  { title: "Infrastructure", titleKey: "admin.navGroup.infrastructure", items: [
    { label: "Nodes", labelKey: "admin.nav.nodes", href: "/admin/nodes", icon: NodeHostIcon, requiredRole: "admin", capability: "available", description: "Beacon hosts, heartbeat status and capacity", descriptionKey: "admin.navDesc.nodes", keywords: ["beacon", "hosts", "daemon", "machines"] },
    { label: "Regions", labelKey: "admin.nav.regions", href: "/admin/regions", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "Cluster regions and placement zones", descriptionKey: "admin.navDesc.regions", keywords: ["zones", "placement"] },
    { label: "Containers", labelKey: "admin.nav.docker", href: "/admin/docker", icon: ContainerShippingIcon, requiredRole: "admin", capability: "available", description: "Docker containers, images, networks and volumes per node", descriptionKey: "admin.navDesc.docker", keywords: ["docker", "images", "runtime"] },
    { label: "Storage Mounts", labelKey: "admin.nav.mounts", href: "/admin/mounts", icon: StoragePlattersIcon, requiredRole: "admin", capability: "available", description: "Shared storage mounts available to workloads", descriptionKey: "admin.navDesc.mounts", keywords: ["nfs", "volumes", "storage"] },

    { label: "Locations", labelKey: "admin.nav.locations", href: "/admin/locations", icon: LocationPinIcon, requiredRole: "admin", capability: "available", description: "Physical and logical node locations", descriptionKey: "admin.navDesc.locations", secondary: true, keywords: ["datacenter"] },
    { label: "Node Capabilities", labelKey: "admin.nav.capabilities", href: "/admin/capabilities", icon: Cpu, requiredRole: "admin", capability: "available", description: "Per-node capability inventory and drift", descriptionKey: "admin.navDesc.capabilities", secondary: true, keywords: ["features", "drift"] },
    { label: "Onboarding Tokens", labelKey: "admin.nav.onboardingTokens", href: "/admin/onboarding-tokens", icon: Ticket, requiredRole: "admin", capability: "available", description: "Issue, approve and revoke node onboarding tokens", descriptionKey: "admin.navDesc.onboardingTokens", secondary: true, keywords: ["enroll", "join", "register node"] },
    { label: "Host Inspector", labelKey: "admin.nav.host", href: "/admin/host", icon: ServerRackIcon, requiredRole: "admin", capability: "available", description: "Per-node system, disk, memory and process inspection", descriptionKey: "admin.navDesc.host", secondary: true, keywords: ["processes", "top", "system"] },
    { label: "Host Files", labelKey: "admin.nav.files", href: "/admin/files", icon: FileText, requiredRole: "admin", capability: "available", description: "Browse and manage files on a node", descriptionKey: "admin.navDesc.files", secondary: true, keywords: ["filesystem", "browse"] },
    { label: "Host Terminal", labelKey: "admin.nav.terminal", href: "/admin/terminal", icon: Terminal, requiredRole: "admin", capability: "available", description: "Secure host shell and console access", descriptionKey: "admin.navDesc.terminal", secondary: true, keywords: ["shell", "ssh", "console"] },
    { label: "SFTP", labelKey: "admin.nav.sftp", href: "/admin/sftp", icon: FolderLock, requiredRole: "admin", capability: "available", description: "Global and per-node SFTP configuration", descriptionKey: "admin.navDesc.sftp", secondary: true },
    { label: "Image & Cache Cleanup", labelKey: "admin.nav.dockerCleanup", href: "/admin/docker-cleanup", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Per-node disk usage and retention-aware image, cache and volume pruning", descriptionKey: "admin.navDesc.dockerCleanup", secondary: true, keywords: ["prune", "disk", "reclaim"] },
    { label: "Cloud Instances", labelKey: "admin.nav.cloud", href: "/admin/cloud", icon: Cloud, requiredRole: "admin", capability: "available", description: "Cloud provider instances and provisioning", descriptionKey: "admin.navDesc.cloud", secondary: true, keywords: ["aws", "hetzner", "provider"] },
    { label: "Kubernetes", labelKey: "admin.nav.kubernetes", href: "/admin/kubernetes", icon: Boxes, requiredRole: "admin", capability: "available", description: "Kubernetes pods, deployments and services", descriptionKey: "admin.navDesc.kubernetes", secondary: true, keywords: ["k8s", "runtime"] },
    { label: "Incus", labelKey: "admin.nav.incus", href: "/admin/incus", icon: Boxes, requiredRole: "admin", capability: "available", description: "Incus system containers and virtual machines", descriptionKey: "admin.navDesc.incus", secondary: true, keywords: ["lxc", "lxd", "runtime", "vm"] },
    { label: "Nomad", labelKey: "admin.nav.nomad", href: "/admin/nomad", icon: Workflow, requiredRole: "admin", capability: "available", description: "Nomad jobs, allocations, nodes and deployments", descriptionKey: "admin.navDesc.nomad", secondary: true, keywords: ["runtime"] },
    { label: "NetBird VPN", labelKey: "admin.nav.netbird", href: "/admin/netbird", icon: Network, requiredRole: "admin", capability: "available", description: "WireGuard mesh VPN control plane", descriptionKey: "admin.navDesc.netbird", secondary: true, keywords: ["wireguard", "mesh", "vpn"] },
  ]},

  { title: "Networking", titleKey: "admin.navGroup.networking", items: [
    { label: "Domains", labelKey: "admin.nav.domains", href: "/admin/domains", icon: Globe, requiredRole: "admin", capability: "available", description: "Custom domains with DNS and TLS status", descriptionKey: "admin.navDesc.domains", keywords: ["dns", "hostname", "tls"] },
    { label: "Certificates", labelKey: "admin.nav.certificates", href: "/admin/certificates", icon: Award, requiredRole: "admin", capability: "available", description: "Public TLS certificates and automated issuance", descriptionKey: "admin.navDesc.certificates", keywords: ["tls", "ssl", "letsencrypt"] },
    { label: "Gateways", labelKey: "admin.nav.gateways", href: "/admin/gateways", icon: GatewayRouterIcon, requiredRole: "admin", capability: "available", description: "Edge gateway routers, services and middlewares", descriptionKey: "admin.navDesc.gateways", keywords: ["ingress", "proxy", "caddy", "traefik"] },
    { label: "Firewall", labelKey: "admin.nav.firewall", href: "/admin/firewall", icon: Shield, requiredRole: "admin", capability: "available", description: "Firewall rules and port forwarding", descriptionKey: "admin.navDesc.firewall", keywords: ["ports", "iptables", "nftables"] },

    { label: "DNS Providers", labelKey: "admin.nav.dnsProviders", href: "/admin/dns", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "DNS providers used for DNS-01 challenges", descriptionKey: "admin.navDesc.dnsProviders", secondary: true, keywords: ["cloudflare", "route53", "dns-01"] },
    { label: "ACME Accounts", labelKey: "admin.nav.acme", href: "/admin/acme", icon: KeyRound, requiredRole: "admin", capability: "available", description: "ACME accounts for automatic certificate issuance", descriptionKey: "admin.navDesc.acme", secondary: true, keywords: ["letsencrypt", "zerossl"] },
    { label: "Endpoints", labelKey: "admin.nav.endpoints", href: "/admin/endpoints", icon: EndpointPlugIcon, requiredRole: "admin", capability: "available", description: "Public endpoint inventory across the fleet", descriptionKey: "admin.navDesc.endpoints", secondary: true },
    { label: "IP Allocations", labelKey: "admin.nav.allocations", href: "/admin/allocations", icon: Cable, requiredRole: "admin", capability: "available", description: "Network ports and IP bindings assigned to workloads", descriptionKey: "admin.navDesc.allocations", secondary: true, keywords: ["ports", "ip"] },
    { label: "Load Balancer", labelKey: "admin.nav.loadBalancer", href: "/admin/load-balancer", icon: LoadBalancerSplitIcon, requiredRole: "admin", capability: "available", description: "Target groups and traffic distribution", descriptionKey: "admin.navDesc.loadBalancer", secondary: true, keywords: ["lb", "upstream"] },
    { label: "Traffic Policies", labelKey: "admin.nav.traffic", href: "/admin/traffic", icon: Route, requiredRole: "admin", capability: "available", description: "Route rules, rate limits and traffic shaping", descriptionKey: "admin.navDesc.traffic", secondary: true, keywords: ["rate limit", "routing"] },
    { label: "Service Discovery", labelKey: "admin.nav.discovery", href: "/admin/discovery", icon: NetworkMeshNodesIcon, requiredRole: "admin", capability: "available", description: "Service discovery records and network policy", descriptionKey: "admin.navDesc.discovery", secondary: true },
    { label: "Cross-Node Routing", labelKey: "admin.nav.crossnode", href: "/admin/crossnode", icon: Repeat, requiredRole: "admin", capability: "available", description: "Cross-node resolver cache and ingress synchronisation", descriptionKey: "admin.navDesc.crossnode", secondary: true },
  ]},

  { title: "Operations", titleKey: "admin.navGroup.operations", items: [
    { label: "Operations", labelKey: "admin.nav.operations", href: "/admin/operations", icon: SlidersHorizontal, requiredRole: "admin", capability: "available", description: "Control-plane operation history and manual controls", descriptionKey: "admin.navDesc.operations", keywords: ["jobs", "tasks", "queue"] },
    { label: "Backups", labelKey: "admin.nav.backups", href: "/admin/backups", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Backup policies, jobs, artifacts and restores", descriptionKey: "admin.navDesc.backups", keywords: ["restore", "snapshot", "retention"] },
    { label: "Backup Engines", labelKey: "admin.nav.backupEngines", href: "/admin/backups/engines", icon: Archive, requiredRole: "admin", capability: "available", description: "Restic and Kopia repositories, snapshots, verification and restores", descriptionKey: "admin.navDesc.backupEngines", secondary: true, parent: "/admin/backups", keywords: ["restic", "kopia", "repository", "prune", "snapshot"] },
    { label: "Migrations", labelKey: "admin.nav.migrations", href: "/admin/migrations", icon: ArrowLeftRight, requiredRole: "admin", capability: "available", description: "Live workload migration jobs and recovery plans", descriptionKey: "admin.navDesc.migrations", keywords: ["move", "transfer", "evacuate"] },
    { label: "Cron Jobs", labelKey: "admin.nav.cronJobs", href: "/admin/cron-jobs", icon: Clock, requiredRole: "admin", capability: "available", description: "Scheduled and recurring automation", descriptionKey: "admin.navDesc.cronJobs", keywords: ["schedule", "automation", "recurring"] },

    { label: "Docker Events", labelKey: "admin.nav.dockerEvents", href: "/admin/docker-events", icon: Activity, requiredRole: "admin", capability: "available", description: "Live container lifecycle events streamed from every Beacon node", descriptionKey: "admin.navDesc.dockerEvents", secondary: true, keywords: ["stream", "lifecycle"] },
    { label: "Reconciliation", labelKey: "admin.nav.reconciliation", href: "/admin/reconciliation", icon: FlaskConical, requiredRole: "admin", capability: "available", description: "Drift detection and desired-state reconciliation", descriptionKey: "admin.navDesc.reconciliation", secondary: true, keywords: ["drift", "desired state"] },
    { label: "Orphaned Resources", labelKey: "admin.nav.orphans", href: "/admin/orphans", icon: Bug, requiredRole: "admin", capability: "available", description: "Servers and databases with no owning record", descriptionKey: "admin.navDesc.orphans", secondary: true, keywords: ["cleanup", "dangling"] },
    { label: "Node Drain", labelKey: "admin.nav.drain", href: "/admin/drain", icon: Droplets, requiredRole: "admin", capability: "available", description: "Node-drain lifecycle and evacuation progress", descriptionKey: "admin.navDesc.drain", secondary: true, keywords: ["maintenance", "evacuate", "cordon"] },
    { label: "Cleanup", labelKey: "admin.nav.cleanup", href: "/admin/cleanup", icon: Trash2, requiredRole: "admin", capability: "available", description: "Garbage collection for stale platform resources", descriptionKey: "admin.navDesc.cleanup", secondary: true, keywords: ["gc", "prune"] },
    { label: "Failover", labelKey: "admin.nav.failover", href: "/admin/failover", icon: LifeBuoy, requiredRole: "admin", capability: "available", description: "Automatic failover and recovery behaviour", descriptionKey: "admin.navDesc.failover", secondary: true, keywords: ["ha", "recovery"] },
    { label: "Procedures", labelKey: "admin.nav.procedures", href: "/admin/procedures", icon: Workflow, requiredRole: "admin", capability: "available", description: "Workflow procedures, schedules and executions", descriptionKey: "admin.navDesc.procedures", secondary: true, keywords: ["runbook", "workflow"] },
    { label: "Scheduler", labelKey: "admin.nav.scheduler", href: "/admin/scheduler", icon: BarChart3, requiredRole: "admin", capability: "available", description: "Placement scoring, strategies and explanations", descriptionKey: "admin.navDesc.scheduler", secondary: true, keywords: ["placement", "bin packing", "explain"] },
    { label: "Workload Autoscaling", labelKey: "admin.nav.autoScaler", href: "/admin/autoscaler", icon: Scale, requiredRole: "admin", capability: "available", description: "Automatic scaling policies for workloads", descriptionKey: "admin.navDesc.autoScaler", secondary: true, keywords: ["hpa", "scale", "policy"] },
    { label: "Node Autoscaling", labelKey: "admin.nav.nodeAutoscaler", href: "/admin/node-autoscaler", icon: TrendingUp, requiredRole: "admin", capability: "available", description: "Cloud node autoscale policies and events", descriptionKey: "admin.navDesc.nodeAutoscaler", secondary: true, keywords: ["scale out", "capacity"] },
    { label: "Placement Affinity", labelKey: "admin.nav.envAffinity", href: "/admin/env-affinity", icon: Network, requiredRole: "admin", capability: "available", description: "Placement constraints, affinity rules and explanations", descriptionKey: "admin.navDesc.envAffinity", secondary: true, keywords: ["affinity", "anti-affinity", "constraints"] },
  ]},

  { title: "Access", titleKey: "admin.navGroup.access", items: [
    { label: "Organizations", labelKey: "admin.nav.organizations", href: "/admin/organizations", icon: Building2, requiredRole: "admin", capability: "available", description: "Tenants: the top of the organization → project → environment hierarchy", descriptionKey: "admin.navDesc.organizations", keywords: ["tenancy", "tenants", "members", "invitations"] },
    { label: "Projects", labelKey: "admin.nav.projects", href: "/admin/projects", icon: Layers, requiredRole: "admin", capability: "available", description: "Projects grouping workloads inside an organization", descriptionKey: "admin.navDesc.projects", keywords: ["tenancy"] },
    { label: "Environments", labelKey: "admin.nav.environments", href: "/admin/environments", icon: Globe, requiredRole: "admin", capability: "available", description: "Environment stages within a project, and their variables", descriptionKey: "admin.navDesc.environments", keywords: ["staging", "production", "env vars", "secrets"] },
    { label: "Users", labelKey: "admin.nav.users", href: "/admin/users", icon: Users, requiredRole: "admin", capability: "available", description: "Accounts, limits and status", descriptionKey: "admin.navDesc.users", keywords: ["accounts", "people", "access"] },
    { label: "Roles & Permissions", labelKey: "admin.nav.roles", href: "/admin/roles", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Role definitions, scopes and permission sets", descriptionKey: "admin.navDesc.roles", keywords: ["rbac", "scopes", "permissions"] },

    { label: "Single Sign-On", labelKey: "admin.nav.socialLogin", href: "/admin/social", icon: Fingerprint, requiredRole: "admin", capability: "available", description: "Social and enterprise SSO providers", descriptionKey: "admin.navDesc.socialLogin", secondary: true, keywords: ["oidc", "saml", "oauth login", "google", "github"] },
    { label: "OAuth Clients", labelKey: "admin.nav.oauthClients", href: "/admin/oauth-clients", icon: KeyRound, requiredRole: "admin", capability: "available", description: "OAuth clients authorised against the Forge API", descriptionKey: "admin.navDesc.oauthClients", secondary: true },
    { label: "API Keys", labelKey: "admin.nav.apiKeys", href: "/admin/api", icon: Code2, requiredRole: "admin", capability: "available", description: "API keys and programmatic access", descriptionKey: "admin.navDesc.apiKeys", secondary: true, keywords: ["tokens", "programmatic"] },
    { label: "Security Headers", labelKey: "admin.nav.security", href: "/admin/security", icon: ShieldAlert, requiredRole: "admin", capability: "available", description: "Security headers and per-domain policies", descriptionKey: "admin.navDesc.security", secondary: true, keywords: ["csp", "hsts", "headers"] },
    { label: "Private CA & mTLS", labelKey: "admin.nav.mtlsCertificates", href: "/admin/mtls", icon: Lock, requiredRole: "admin", capability: "available", description: "Private certificate authority and mutual TLS identities", descriptionKey: "admin.navDesc.mtlsCertificates", secondary: true, keywords: ["ca", "client certs"] },
    { label: "Vault", labelKey: "admin.nav.vault", href: "/admin/vault", icon: KeyRound, requiredRole: "admin", capability: "available", description: "HashiCorp Vault connections for live secret references", descriptionKey: "admin.navDesc.vault", secondary: true, keywords: ["secrets", "hashicorp"] },
  ]},

  { title: "Platform", titleKey: "admin.navGroup.platform", items: [
    { label: "Platform Settings", labelKey: "admin.nav.settings", href: "/admin/settings", icon: SettingsCogIcon, requiredRole: "admin", capability: "available", description: "Global Forge configuration", descriptionKey: "admin.navDesc.settings", keywords: ["config", "preferences", "branding"] },
    { label: "Billing & Quotas", labelKey: "admin.nav.billing", href: "/admin/billing", icon: CreditCard, requiredRole: "admin", capability: "available", description: "Billing plans, quotas and usage", descriptionKey: "admin.navDesc.billing", keywords: ["plans", "usage", "limits"] },

    { label: "Notifications", labelKey: "admin.nav.notifications", href: "/admin/notifications", icon: NotificationBellIcon, requiredRole: "admin", capability: "available", description: "Notification channels, event subscriptions and delivery logs", descriptionKey: "admin.navDesc.notifications", secondary: true, keywords: ["alerts", "slack", "discord"] },
    { label: "Mail", labelKey: "admin.nav.mail", href: "/admin/mail", icon: Mail, requiredRole: "admin", capability: "available", description: "Outbound mail settings and triggers", descriptionKey: "admin.navDesc.mail", secondary: true, keywords: ["smtp", "email"] },
    { label: "Webhooks", labelKey: "admin.nav.webhooks", href: "/admin/webhooks", icon: Webhook, requiredRole: "admin", capability: "available", description: "Outbound event delivery endpoints", descriptionKey: "admin.navDesc.webhooks", secondary: true },
    { label: "Plugins", labelKey: "admin.nav.plugins", href: "/admin/plugins", icon: Plug, requiredRole: "admin", capability: "metadata-only", description: "Extensions and marketplace plugins", descriptionKey: "admin.navDesc.plugins", secondary: true },
    { label: "Guided Setup", labelKey: "admin.nav.onboarding", href: "/admin/onboarding", icon: Compass, requiredRole: "admin", capability: "available", description: "Guided first-run setup: connect a node and deploy", descriptionKey: "admin.navDesc.onboarding", secondary: true, keywords: ["getting started", "wizard", "first run"] },
    { label: "Platform Upgrade", labelKey: "admin.nav.upgrade", href: "/admin/upgrade", icon: ArrowUpCircle, requiredRole: "admin", capability: "available", description: "Self-upgrade the control plane with backup and rollback", descriptionKey: "admin.navDesc.upgrade", secondary: true, keywords: ["update", "version", "release"] },
    { label: "State Components", labelKey: "admin.nav.devStates", href: "/admin/dev/states", icon: FlaskConical, requiredRole: "admin", capability: "available", description: "Development-only gallery of loading, empty and error states", descriptionKey: "admin.navDesc.devStates", hidden: true, keywords: ["dev", "gallery", "storybook"] },
  ]},
];

/**
 * Legacy and vocabulary-alias paths. These affect *navigation resolution only*
 * (active item, breadcrumbs, palette) so a stale deep link or bookmark still
 * resolves to the right nav position. Routing itself is unchanged: the few
 * aliases that also need to move the browser have their own redirect stub page
 * (e.g. app/admin/containers/page.tsx).
 */
export const ADMIN_ALIAS_ROUTES: Record<string, string> = {
  "/admin/containers": "/admin/docker",
  "/admin/logs": "/admin/activity",
  "/admin/git-providers": "/admin/git",
  "/admin/database-services": "/admin/databases",
  // Vocabulary normalization (Node ↔ Beacon)
  "/admin/beacons": "/admin/nodes",
  "/admin/beacon": "/admin/nodes",
  // Legacy group deep-links
  "/admin/workloads": "/admin/servers",
  "/admin/game-servers": "/admin/servers",
  "/admin/networking": "/admin/endpoints",
  "/admin/storage": "/admin/mounts",
  "/admin/volumes": "/admin/mounts",
  "/admin/database-hosts": "/admin/databases",
  "/admin/build": "/admin/catalog",
  "/admin/deploy": "/admin/deployments",
  "/admin/operations/advanced": "/admin/operations",
  "/admin/infra": "/admin/nodes",
  "/admin/infra/beacons": "/admin/nodes",
  "/admin/infra/networking": "/admin/endpoints",
  "/admin/infra/storage": "/admin/mounts",
  "/admin/access": "/admin/users",
  "/admin/platform": "/admin/settings",
  "/admin/command": "/admin/overview",
  "/admin/network": "/admin/domains",
  "/admin/data": "/admin/databases",
  "/admin/automation": "/admin/cron-jobs",
  "/admin/tenancy": "/admin/organizations",
  "/admin/security-headers": "/admin/security",
  "/admin/mtls-ca": "/admin/mtls",
  "/admin/traffic-policies": "/admin/traffic",
  "/admin/dns-providers": "/admin/dns",
  "/admin/acme-accounts": "/admin/acme",
};

/**
 * Labels for known trailing URL segments on detail routes. Anything not listed
 * falls through to `humanizeSegment`, which returns opaque ids verbatim rather
 * than inventing a name for them.
 */
export const ADMIN_SEGMENT_LABELS: Record<string, string> = {
  new: "New",
  edit: "Edit",
  history: "History",
  revisions: "Revisions",
  deployments: "Deployments",
  compose: "Compose",
  git: "Git",
  eggs: "Eggs",
  variables: "Variables",
  policy: "Policy",
  services: "Services",
  states: "States",
  dev: "Development",
};

function resolveAlias(pathname: string): string {
  return ADMIN_ALIAS_ROUTES[pathname] ?? pathname;
}

const allEntries: AdminNavEntry[] = adminPageRegistry.flatMap((group) => group.items);
const entriesByHref = new Map(allEntries.map((entry) => [entry.href, entry]));
const groupTitleByHref = new Map(
  adminPageRegistry.flatMap((group) => group.items.map((item) => [item.href, group.title] as const)),
);

/** Every registered entry, including hidden ones. Search and palette use this. */
export function adminNavEntries(): AdminNavEntry[] {
  return allEntries;
}

export type AdminSidebarGroup = AdminNavGroup & { secondaryItems: AdminNavEntry[] };

export function adminPagesForRole(role?: string): AdminNavGroup[] {
  return adminPageRegistry
    .map((group) => ({ ...group, items: group.items.filter((item) => role === item.requiredRole && !item.hidden) }))
    .filter((group) => group.items.length > 0);
}

/**
 * Sidebar view: primaries always visible, secondaries collapsed under "More".
 * Hidden entries never appear here — they exist for breadcrumbs and active
 * state only.
 */
export function adminSidebarGroups(role?: string): AdminSidebarGroup[] {
  return adminPageRegistry
    .map((group) => {
      const items = group.items.filter((item) => role === item.requiredRole && !item.hidden);
      return {
        ...group,
        items: items.filter((item) => !item.secondary),
        secondaryItems: items.filter((item) => item.secondary),
      };
    })
    .filter((group) => group.items.length > 0 || group.secondaryItems.length > 0);
}

/**
 * The registry entry a pathname belongs to — the most specific match, including
 * hidden entries. `/admin/nodes/abc123` resolves to Nodes; `/admin/health`
 * resolves to Diagnostics.
 */
export function findAdminPage(pathname: string): AdminNavEntry | undefined {
  return resolveActive(resolveAlias(pathname), allEntries);
}

/**
 * The entry the sidebar should highlight: the nearest *visible* ancestor of the
 * matched entry. Exactly one row is ever highlighted, and a hidden page
 * (`/admin/health`) highlights its parent (Monitoring) rather than nothing.
 */
export function adminActiveEntry(pathname: string): AdminNavEntry | undefined {
  let entry = findAdminPage(pathname);
  const seen = new Set<string>();
  while (entry?.hidden && entry.parent && !seen.has(entry.href)) {
    seen.add(entry.href);
    entry = entriesByHref.get(entry.parent);
  }
  return entry?.hidden ? undefined : entry;
}

/** href of the single sidebar row to mark `aria-current="page"`. */
export function adminActiveHref(pathname: string): string | undefined {
  return adminActiveEntry(pathname)?.href;
}

export function findAdminPageGroup(pathname: string): { groupTitle: string; pageLabel: string } | undefined {
  const match = findAdminPage(pathname);
  if (!match) return undefined;
  const groupTitle = groupTitleByHref.get(match.href);
  if (!groupTitle) return undefined;
  return { groupTitle, pageLabel: match.label };
}

/**
 * Full breadcrumb chain for an admin pathname:
 *
 *   Workloads / Applications / 8f3a / Git
 *   Overview / Monitoring / Diagnostics
 *   Deploy / Deployments / History
 *
 * The first crumb is the group (no link — groups are not pages). Ancestor
 * crumbs link; the final crumb never does. Unregistered paths return a
 * two-crumb fallback rather than pretending to know the hierarchy.
 */
export function adminBreadcrumbTrail(pathname: string): NavCrumb[] {
  const resolved = resolveAlias(pathname);
  const match = findAdminPage(resolved);
  if (!match) {
    const last = resolved.split("/").filter(Boolean).at(-1);
    return last && last !== "admin"
      ? [{ label: "Admin" }, { label: ADMIN_SEGMENT_LABELS[last] ?? humanizeSegment(last) }]
      : [{ label: "Admin" }];
  }

  // Walk up `parent` links so hidden pages sit under their real owner.
  const chain: AdminNavEntry[] = [match];
  const seen = new Set<string>([match.href]);
  let cursor = match.parent ? entriesByHref.get(match.parent) : undefined;
  while (cursor && !seen.has(cursor.href)) {
    seen.add(cursor.href);
    chain.unshift(cursor);
    cursor = cursor.parent ? entriesByHref.get(cursor.parent) : undefined;
  }

  const groupTitle = groupTitleByHref.get(chain[0].href);
  const crumbs: NavCrumb[] = groupTitle ? [{ label: groupTitle }] : [];
  for (const entry of chain) crumbs.push({ label: entry.label, href: entry.href });

  // Anything below the matched entry becomes a trailing crumb chain, so detail
  // routes read as a hierarchy instead of collapsing onto their list page.
  if (isPathWithin(resolved, match.href) && resolved !== match.href) {
    const rest = resolved.slice(match.href.length).split("/").filter(Boolean);
    let href = match.href;
    rest.forEach((segment, index) => {
      href = `${href}/${segment}`;
      crumbs.push({
        label: ADMIN_SEGMENT_LABELS[segment] ?? humanizeSegment(segment),
        href: index === rest.length - 1 ? undefined : href,
      });
    });
  }

  const last = crumbs.at(-1);
  if (last) delete last.href;
  return crumbs;
}

/** Case-insensitive match over label, description and keywords. */
export function adminEntryMatches(entry: AdminNavEntry, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  if (entry.label.toLowerCase().includes(needle)) return true;
  if (entry.description.toLowerCase().includes(needle)) return true;
  if (entry.href.toLowerCase().includes(needle)) return true;
  return (entry.keywords ?? []).some((keyword) => keyword.toLowerCase().includes(needle));
}
