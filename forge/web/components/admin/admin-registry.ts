import type { FC } from "react";
import type { LucideIcon } from "lucide-react";
import type { IconProps } from "@/components/ui/forge-icons";
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
  StoragePlattersIcon,
} from "@/components/ui/forge-icons";
import {
  Activity, Archive, ArrowLeftRight, Award, BarChart3, Bell, Boxes, Building2, Bug, Cable, Clock, Cloud, Code2, Compass,
  Cpu, CreditCard, Droplets, Eye, FileCode, FileText, Fingerprint, FlaskConical, FolderLock,
  GitPullRequest, Globe, HardDrive, KeyRound, Layers, LifeBuoy, Lock, Mail, Network, Plug,
  ArrowUpCircle, Repeat, Route, Scale, Settings, Shield, ShieldAlert, ShieldCheck, SlidersHorizontal,
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
};

export type AdminNavGroup = { title: string; titleKey: string; items: AdminNavEntry[] };

/**
 * Consolidated IA — 7 goal-oriented groups (was 11).
 *
 * OVERVIEW (4) · WORKLOADS (12) · DELIVERY (9) · INFRASTRUCTURE (17)
 * NETWORK & SECURITY (14) · OPERATIONS (16) · ORGANIZATION & SETTINGS (16) = 88 hrefs.
 * Counts are a current census, not an invariant — recount the `href:` entries per
 * group before trusting them after a nav change.
 *
 * All hrefs retained for compat; legacy paths map via ADMIN_ALIAS_ROUTES.
 * Database Services is intentionally NOT a second nav item — it lives as a
 * tab inside the unified Databases page (/admin/databases?tab=services).
 * Sidebar shows primary items; `secondary: true` items collapse under "More"
 * per group but remain in search, command palette, and breadcrumbs.
 * Hidden routes (/admin/containers, /admin/logs, /admin/dev/states, .../new,
 * .../history) are NOT nav entries — they resolve via alias or parent prefix
 * match in findAdminPage and must not be added here.
 */
export const adminPageRegistry: AdminNavGroup[] = [
  { title: "Overview", titleKey: "admin.navGroup.overview", items: [
    { label: "Overview", labelKey: "admin.nav.overview", href: "/admin/overview", icon: OverviewDashboardIcon, requiredRole: "admin", capability: "available", description: "Live control-plane summary and fleet health at a glance", descriptionKey: "admin.navDesc.overview" },
    { label: "Monitoring", labelKey: "admin.nav.monitoring", href: "/admin/monitoring", icon: MonitoringPulseIcon, requiredRole: "admin", capability: "available", description: "Platform and node health dashboards", descriptionKey: "admin.navDesc.monitoring" },
    { label: "Health", labelKey: "admin.nav.health", href: "/admin/health", icon: HealthECGIcon, requiredRole: "admin", capability: "available", description: "Dependency and service diagnostics", descriptionKey: "admin.navDesc.health" },
    { label: "Activity", labelKey: "admin.nav.activity", href: "/admin/activity", icon: ActivityWaveIcon, requiredRole: "admin", capability: "available", description: "Human-readable audit and activity history", descriptionKey: "admin.navDesc.activity" },
  ]},
  { title: "Workloads", titleKey: "admin.navGroup.workloads", items: [
    { label: "Servers", labelKey: "admin.nav.servers", href: "/admin/servers", icon: GamepadIcon, requiredRole: "admin", capability: "available", description: "Game server instances and lifecycle", descriptionKey: "admin.navDesc.servers" },
    { label: "Apps", labelKey: "admin.nav.apps", href: "/admin/apps", icon: ApplicationsCubeIcon, requiredRole: "admin", capability: "available", description: "Container apps, Git repos, and stacks", descriptionKey: "admin.navDesc.apps" },
    { label: "Databases", labelKey: "admin.nav.databaseHosts", href: "/admin/databases", icon: DatabaseCylinderIcon, requiredRole: "admin", capability: "available", description: "Databases — unified inventory, hosts, containers, managed DBs, services, catalog, server DBs", descriptionKey: "admin.navDesc.databaseHosts" },
    { label: "Catalog", labelKey: "admin.nav.catalog", href: "/admin/catalog", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "One-click service catalog and provisioning", descriptionKey: "admin.navDesc.catalog" },
    { label: "App Store", labelKey: "admin.nav.appStore", href: "/admin/app-store", icon: AppStoreIcon, requiredRole: "admin", capability: "available", description: "Curated one-click service catalog", descriptionKey: "admin.navDesc.appStore", secondary: true },
    { label: "Registries", labelKey: "admin.nav.registries", href: "/admin/registries", icon: Boxes, requiredRole: "admin", capability: "available", description: "Private Docker registry credentials for image pull/push", descriptionKey: "admin.navDesc.registries", secondary: true },
    { label: "Service Definitions", labelKey: "admin.nav.nestsEggs", href: "/admin/nests", icon: Layers, requiredRole: "admin", capability: "available", description: "Reusable server blueprints (nests and eggs)", descriptionKey: "admin.navDesc.nestsEggs", secondary: true },
    { label: "App Templates", labelKey: "admin.nav.appTemplates", href: "/admin/app-templates", icon: FileCode, requiredRole: "admin", capability: "available", description: "Application deployment blueprints", descriptionKey: "admin.navDesc.appTemplates", secondary: true },
    { label: "Legacy Templates", labelKey: "admin.nav.compatibilityTemplates", href: "/admin/templates", icon: Archive, requiredRole: "admin", capability: "available", description: "Legacy compatibility templates", descriptionKey: "admin.navDesc.compatibilityTemplates", secondary: true },
    { label: "Forgefile", labelKey: "admin.nav.forgefile", href: "/admin/forgefile", icon: FileText, requiredRole: "admin", capability: "available", description: "Environment-as-code: validate and apply a Forgefile", descriptionKey: "admin.navDesc.forgefile", secondary: true },
    { label: "App Mounts", labelKey: "admin.nav.appMounts", href: "/admin/app-mounts", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Per-application persistent storage (volumes, binds, tmpfs, seed files)", descriptionKey: "admin.navDesc.appMounts", secondary: true },
    { label: "Tags", labelKey: "admin.nav.tags", href: "/admin/tags", icon: Tags, requiredRole: "admin", capability: "available", description: "Color-coded labels for organizing resources", descriptionKey: "admin.navDesc.tags", secondary: true },
  ]},
  { title: "Delivery", titleKey: "admin.navGroup.delivery", items: [
    { label: "Deployments", labelKey: "admin.nav.deployments", href: "/admin/deployments", icon: RocketLaunchIcon, requiredRole: "admin", capability: "available", description: "Blue-green and rolling deployments", descriptionKey: "admin.navDesc.deployments" },
    { label: "Compose Stacks", labelKey: "admin.nav.compose", href: "/admin/compose", icon: ComposeSheetsIcon, requiredRole: "admin", capability: "available", description: "Docker Compose stacks and imports", descriptionKey: "admin.navDesc.compose" },
    { label: "Stack Templates", labelKey: "admin.nav.stackTemplates", href: "/admin/compose-templates", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "Reusable parameterized compose stack templates", descriptionKey: "admin.navDesc.stackTemplates" },
    { label: "Pipelines", labelKey: "admin.nav.pipelines", href: "/admin/pipelines", icon: PipelineFlowIcon, requiredRole: "admin", capability: "available", description: "CI/CD pipelines and delivery workflows", descriptionKey: "admin.navDesc.pipelines" },
    { label: "Git Integrations", labelKey: "admin.nav.gitConnections", href: "/admin/git", icon: GitBranchTreeIcon, requiredRole: "admin", capability: "available", description: "Git credentials, providers, and sources", descriptionKey: "admin.navDesc.gitConnections" },
    { label: "Preview Deployments", labelKey: "admin.nav.previewDeployments", href: "/admin/preview-deployments", icon: Eye, requiredRole: "admin", capability: "available", description: "Pull-request preview environments", descriptionKey: "admin.navDesc.previewDeployments", secondary: true },
    { label: "Preview Environments", labelKey: "admin.nav.previewEnvironments", href: "/admin/preview-environments", icon: FlaskConical, requiredRole: "admin", capability: "available", description: "Ephemeral per-branch environments scoped to a project", descriptionKey: "admin.navDesc.previewEnvironments" },
    { label: "Source Deployments", labelKey: "admin.nav.sourceDeployments", href: "/admin/source-deployments", icon: GitPullRequest, requiredRole: "admin", capability: "available", description: "Build and deploy from Git sources", descriptionKey: "admin.navDesc.sourceDeployments", secondary: true },
    { label: "Zero Downtime", labelKey: "admin.nav.zeroDowntime", href: "/admin/zerodowntime", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Zero-downtime releases and health gates", descriptionKey: "admin.navDesc.zeroDowntime", secondary: true },
  ]},
  { title: "Infrastructure", titleKey: "admin.navGroup.infrastructure", items: [
    { label: "Nodes", labelKey: "admin.nav.nodes", href: "/admin/nodes", icon: NodeHostIcon, requiredRole: "admin", capability: "available", description: "Daemon hosts and heartbeat status", descriptionKey: "admin.navDesc.nodes" },
    { label: "Regions", labelKey: "admin.nav.regions", href: "/admin/regions", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "Cluster regions and placement zones", descriptionKey: "admin.navDesc.regions" },
    { label: "Docker", labelKey: "admin.nav.docker", href: "/admin/docker", icon: ContainerShippingIcon, requiredRole: "admin", capability: "available", description: "Container, image, network and volume management", descriptionKey: "admin.navDesc.docker" },
    { label: "Docker Cleanup", labelKey: "admin.nav.dockerCleanup", href: "/admin/docker-cleanup", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Per-node disk usage and scheduled, retention-aware image/cache/volume pruning", descriptionKey: "admin.navDesc.dockerCleanup", secondary: true },
    { label: "Storage Mounts", labelKey: "admin.nav.mounts", href: "/admin/mounts", icon: StoragePlattersIcon, requiredRole: "admin", capability: "available", description: "Shared storage mounts and volumes", descriptionKey: "admin.navDesc.mounts" },
    { label: "Locations", labelKey: "admin.nav.locations", href: "/admin/locations", icon: LocationPinIcon, requiredRole: "admin", capability: "available", description: "Physical and logical node locations", descriptionKey: "admin.navDesc.locations", secondary: true },
    { label: "Capabilities", labelKey: "admin.nav.capabilities", href: "/admin/capabilities", icon: Cpu, requiredRole: "admin", capability: "available", description: "Node capability inventory and drift", descriptionKey: "admin.navDesc.capabilities", secondary: true },
    { label: "Onboarding Tokens", labelKey: "admin.nav.onboardingTokens", href: "/admin/onboarding-tokens", icon: Ticket, requiredRole: "admin", capability: "available", description: "Node onboarding tokens: issue, approve, revoke", descriptionKey: "admin.navDesc.onboardingTokens", secondary: true },
    { label: "Host Detail", labelKey: "admin.nav.host", href: "/admin/host", icon: ServerRackIcon, requiredRole: "admin", capability: "available", description: "Per-node system, disk, memory and processes", descriptionKey: "admin.navDesc.host", secondary: true },
    { label: "Kubernetes", labelKey: "admin.nav.kubernetes", href: "/admin/kubernetes", icon: Boxes, requiredRole: "admin", capability: "available", description: "Kubernetes pods, deployments and services", descriptionKey: "admin.navDesc.kubernetes", secondary: true },
    { label: "Incus", labelKey: "admin.nav.incus", href: "/admin/incus", icon: Boxes, requiredRole: "admin", capability: "available", description: "Incus system containers and virtual machines", descriptionKey: "admin.navDesc.incus", secondary: true },
    { label: "Nomad", labelKey: "admin.nav.nomad", href: "/admin/nomad", icon: Workflow, requiredRole: "admin", capability: "available", description: "Nomad jobs, allocations, nodes and deployments", descriptionKey: "admin.navDesc.nomad", secondary: true },
    { label: "NetBird VPN", labelKey: "admin.nav.netbird", href: "/admin/netbird", icon: Network, requiredRole: "admin", capability: "available", description: "WireGuard mesh VPN control plane", descriptionKey: "admin.navDesc.netbird", secondary: true },
    { label: "Cloud Instances", labelKey: "admin.nav.cloud", href: "/admin/cloud", icon: Cloud, requiredRole: "admin", capability: "available", description: "Cloud provider instances and provisioning", descriptionKey: "admin.navDesc.cloud", secondary: true },
    { label: "Host Files", labelKey: "admin.nav.files", href: "/admin/files", icon: FileText, requiredRole: "admin", capability: "available", description: "Browse and manage host files", descriptionKey: "admin.navDesc.files", secondary: true },
    { label: "Terminal", labelKey: "admin.nav.terminal", href: "/admin/terminal", icon: Terminal, requiredRole: "admin", capability: "available", description: "Secure host shell and console access", descriptionKey: "admin.navDesc.terminal", secondary: true },
    { label: "SFTP", labelKey: "admin.nav.sftp", href: "/admin/sftp", icon: FolderLock, requiredRole: "admin", capability: "available", description: "Global and per-node SFTP configuration", descriptionKey: "admin.navDesc.sftp", secondary: true },
  ]},
  { title: "Network & Security", titleKey: "admin.navGroup.networkSecurity", items: [
    { label: "Domains", labelKey: "admin.nav.domains", href: "/admin/domains", icon: Globe, requiredRole: "admin", capability: "available", description: "Custom domains with DNS and TLS status", descriptionKey: "admin.navDesc.domains" },
    { label: "Certificates", labelKey: "admin.nav.certificates", href: "/admin/certificates", icon: Award, requiredRole: "admin", capability: "available", description: "Public TLS certificates and automated issuance", descriptionKey: "admin.navDesc.certificates" },
    { label: "Gateways", labelKey: "admin.nav.gateways", href: "/admin/gateways", icon: GatewayRouterIcon, requiredRole: "admin", capability: "available", description: "Edge gateway routers, services and middlewares", descriptionKey: "admin.navDesc.gateways" },
    { label: "Firewall", labelKey: "admin.nav.firewall", href: "/admin/firewall", icon: Shield, requiredRole: "admin", capability: "available", description: "Firewall rules and port forwarding", descriptionKey: "admin.navDesc.firewall" },
    { label: "DNS Providers", labelKey: "admin.nav.dnsProviders", href: "/admin/dns", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "DNS providers for DNS-01 challenges", descriptionKey: "admin.navDesc.dnsProviders", secondary: true },
    { label: "ACME Accounts", labelKey: "admin.nav.acme", href: "/admin/acme", icon: KeyRound, requiredRole: "admin", capability: "available", description: "ACME accounts for automatic issuance", descriptionKey: "admin.navDesc.acme", secondary: true },
    { label: "Private CA & mTLS", labelKey: "admin.nav.mtlsCertificates", href: "/admin/mtls", icon: Lock, requiredRole: "admin", capability: "available", description: "Private certificate authority and mutual TLS", descriptionKey: "admin.navDesc.mtlsCertificates", secondary: true },
    { label: "Security Headers", labelKey: "admin.nav.security", href: "/admin/security", icon: ShieldAlert, requiredRole: "admin", capability: "available", description: "Security headers and per-domain policies", descriptionKey: "admin.navDesc.security", secondary: true },
    { label: "Traffic Policies", labelKey: "admin.nav.traffic", href: "/admin/traffic", icon: Route, requiredRole: "admin", capability: "available", description: "Route rules and traffic shaping", descriptionKey: "admin.navDesc.traffic", secondary: true },
    { label: "Load Balancer", labelKey: "admin.nav.loadBalancer", href: "/admin/load-balancer", icon: LoadBalancerSplitIcon, requiredRole: "admin", capability: "available", description: "Target groups and traffic routing", descriptionKey: "admin.navDesc.loadBalancer", secondary: true },
    { label: "Endpoints", labelKey: "admin.nav.endpoints", href: "/admin/endpoints", icon: EndpointPlugIcon, requiredRole: "admin", capability: "available", description: "Public endpoint inventory", descriptionKey: "admin.navDesc.endpoints", secondary: true },
    { label: "IP Allocations", labelKey: "admin.nav.allocations", href: "/admin/allocations", icon: Cable, requiredRole: "admin", capability: "available", description: "Network ports and IP bindings", descriptionKey: "admin.navDesc.allocations", secondary: true },
    { label: "Service Discovery", labelKey: "admin.nav.discovery", href: "/admin/discovery", icon: NetworkMeshNodesIcon, requiredRole: "admin", capability: "available", description: "Service discovery and network policy", descriptionKey: "admin.navDesc.discovery", secondary: true },
    { label: "Cross-Node", labelKey: "admin.nav.crossnode", href: "/admin/crossnode", icon: Repeat, requiredRole: "admin", capability: "available", description: "Cross-node resolver cache and ingress sync", descriptionKey: "admin.navDesc.crossnode", secondary: true },
  ]},
  { title: "Operations", titleKey: "admin.navGroup.operations", items: [
    { label: "Operations", labelKey: "admin.nav.operations", href: "/admin/operations", icon: SlidersHorizontal, requiredRole: "admin", capability: "available", description: "Control-plane operation history and controls", descriptionKey: "admin.navDesc.operations" },
    { label: "Docker Events", labelKey: "admin.nav.dockerEvents", href: "/admin/docker-events", icon: Activity, requiredRole: "admin", capability: "available", description: "Live container lifecycle events streamed from every Beacon node", descriptionKey: "admin.navDesc.dockerEvents" },
    { label: "Backups", labelKey: "admin.nav.backups", href: "/admin/backups", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Backups, artifacts and restore operations", descriptionKey: "admin.navDesc.backups" },
    { label: "Backup Engines", labelKey: "admin.nav.backupEngines", href: "/admin/backups/engines", icon: Archive, requiredRole: "admin", capability: "available", description: "Restic and Kopia repositories, snapshots and restores", descriptionKey: "admin.navDesc.backupEngines", secondary: true },
    { label: "Migrations", labelKey: "admin.nav.migrations", href: "/admin/migrations", icon: ArrowLeftRight, requiredRole: "admin", capability: "available", description: "Live migration jobs and recovery plans", descriptionKey: "admin.navDesc.migrations" },
    { label: "Cron Jobs", labelKey: "admin.nav.cronJobs", href: "/admin/cron-jobs", icon: Clock, requiredRole: "admin", capability: "available", description: "Scheduled and recurring automation", descriptionKey: "admin.navDesc.cronJobs" },
    { label: "Reconciliation", labelKey: "admin.nav.reconciliation", href: "/admin/reconciliation", icon: FlaskConical, requiredRole: "admin", capability: "available", description: "Drift detection and state reconciliation", descriptionKey: "admin.navDesc.reconciliation", secondary: true },
    { label: "Orphans", labelKey: "admin.nav.orphans", href: "/admin/orphans", icon: Bug, requiredRole: "admin", capability: "available", description: "Orphaned servers and databases needing cleanup", descriptionKey: "admin.navDesc.orphans", secondary: true },
    { label: "Drain", labelKey: "admin.nav.drain", href: "/admin/drain", icon: Droplets, requiredRole: "admin", capability: "available", description: "Node-drain lifecycle and evacuation progress", descriptionKey: "admin.navDesc.drain", secondary: true },
    { label: "Cleanup", labelKey: "admin.nav.cleanup", href: "/admin/cleanup", icon: Trash2, requiredRole: "admin", capability: "available", description: "Garbage collection for stale resources", descriptionKey: "admin.navDesc.cleanup", secondary: true },
    { label: "Failover", labelKey: "admin.nav.failover", href: "/admin/failover", icon: LifeBuoy, requiredRole: "admin", capability: "available", description: "Automatic failover and recovery", descriptionKey: "admin.navDesc.failover", secondary: true },
    { label: "Procedures", labelKey: "admin.nav.procedures", href: "/admin/procedures", icon: Workflow, requiredRole: "admin", capability: "available", description: "Workflow procedures, schedules and executions", descriptionKey: "admin.navDesc.procedures", secondary: true },
    { label: "Scheduler", labelKey: "admin.nav.scheduler", href: "/admin/scheduler", icon: BarChart3, requiredRole: "admin", capability: "available", description: "Placement scoring and affinity rules", descriptionKey: "admin.navDesc.scheduler", secondary: true },
    { label: "Auto Scaling", labelKey: "admin.nav.autoScaler", href: "/admin/autoscaler", icon: Scale, requiredRole: "admin", capability: "available", description: "Automatic scaling policies", descriptionKey: "admin.navDesc.autoScaler", secondary: true },
    { label: "Env Affinity", labelKey: "admin.nav.envAffinity", href: "/admin/env-affinity", icon: Network, requiredRole: "admin", capability: "available", description: "Placement constraints and explanations", descriptionKey: "admin.navDesc.envAffinity", secondary: true },
    { label: "Node Autoscaler", labelKey: "admin.nav.nodeAutoscaler", href: "/admin/node-autoscaler", icon: TrendingUp, requiredRole: "admin", capability: "available", description: "Cloud node autoscale policies and events", descriptionKey: "admin.navDesc.nodeAutoscaler", secondary: true },
  ]},
  { title: "Organization & Settings", titleKey: "admin.navGroup.organization", items: [
    { label: "Users", labelKey: "admin.nav.users", href: "/admin/users", icon: Users, requiredRole: "admin", capability: "available", description: "Accounts, limits and status", descriptionKey: "admin.navDesc.users" },
    { label: "Organizations", labelKey: "admin.nav.organizations", href: "/admin/organizations", icon: Building2, requiredRole: "admin", capability: "available", description: "Multi-tenant organizations", descriptionKey: "admin.navDesc.organizations" },
    { label: "Projects", labelKey: "admin.nav.projects", href: "/admin/projects", icon: Layers, requiredRole: "admin", capability: "available", description: "Projects within organizations", descriptionKey: "admin.navDesc.projects" },
    { label: "Platform Settings", labelKey: "admin.nav.settings", href: "/admin/settings", icon: Settings, requiredRole: "admin", capability: "available", description: "Global platform configuration", descriptionKey: "admin.navDesc.settings" },
    { label: "Roles", labelKey: "admin.nav.roles", href: "/admin/roles", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Role definitions and permissions", descriptionKey: "admin.navDesc.roles", secondary: true },
    { label: "OAuth Clients", labelKey: "admin.nav.oauthClients", href: "/admin/oauth-clients", icon: KeyRound, requiredRole: "admin", capability: "available", description: "OAuth clients and API access", descriptionKey: "admin.navDesc.oauthClients", secondary: true },
    { label: "Single Sign-On", labelKey: "admin.nav.socialLogin", href: "/admin/social", icon: Fingerprint, requiredRole: "admin", capability: "available", description: "Social and enterprise SSO providers", descriptionKey: "admin.navDesc.socialLogin", secondary: true },
    { label: "Environments", labelKey: "admin.nav.environments", href: "/admin/environments", icon: Globe, requiredRole: "admin", capability: "available", description: "Environment stages and variables", descriptionKey: "admin.navDesc.environments", secondary: true },
    { label: "Billing", labelKey: "admin.nav.billing", href: "/admin/billing", icon: CreditCard, requiredRole: "admin", capability: "available", description: "Billing plans, quotas and usage", descriptionKey: "admin.navDesc.billing", secondary: true },
    { label: "Notifications", labelKey: "admin.nav.notifications", href: "/admin/notifications", icon: Bell, requiredRole: "admin", capability: "available", description: "Notification engine channels, event subscriptions and delivery logs", descriptionKey: "admin.navDesc.notifications", secondary: true },
    { label: "Mail", labelKey: "admin.nav.mail", href: "/admin/mail", icon: Mail, requiredRole: "admin", capability: "available", description: "Mail settings and triggers", descriptionKey: "admin.navDesc.mail", secondary: true },
    { label: "Webhooks", labelKey: "admin.nav.webhooks", href: "/admin/webhooks", icon: Webhook, requiredRole: "admin", capability: "available", description: "Event delivery and webhook endpoints", descriptionKey: "admin.navDesc.webhooks", secondary: true },
    { label: "Plugins", labelKey: "admin.nav.plugins", href: "/admin/plugins", icon: Plug, requiredRole: "admin", capability: "metadata-only", description: "Extensions and marketplace plugins", descriptionKey: "admin.navDesc.plugins", secondary: true },
    { label: "API Keys", labelKey: "admin.nav.apiKeys", href: "/admin/api", icon: Code2, requiredRole: "admin", capability: "available", description: "API keys and programmatic access", descriptionKey: "admin.navDesc.apiKeys", secondary: true },
    { label: "Onboarding", labelKey: "admin.nav.onboarding", href: "/admin/onboarding", icon: Compass, requiredRole: "admin", capability: "available", description: "Guided setup: connect and deploy", descriptionKey: "admin.navDesc.onboarding", secondary: true },
    { label: "Platform Upgrade", labelKey: "admin.nav.upgrade", href: "/admin/upgrade", icon: ArrowUpCircle, requiredRole: "admin", capability: "available", description: "Self-upgrade the control plane with backup + rollback", descriptionKey: "admin.navDesc.upgrade", secondary: true },
  ]},
];

/** Invisible/demo alias routes that redirect but should not appear as duplicate nav items. */
export const ADMIN_ALIAS_ROUTES: Record<string, string> = {
  "/admin/containers": "/admin/docker",
  "/admin/logs": "/admin/activity",
  // Vocabulary normalization — target-ia §2 (Node → Beacon)
  "/admin/beacons": "/admin/nodes",
  "/admin/beacon": "/admin/nodes",
  // Consolidated IA compat — legacy group deep-links redirect to new homes
  "/admin/workloads": "/admin/servers",
  "/admin/game-servers": "/admin/servers",
  "/admin/networking": "/admin/endpoints",
  "/admin/storage": "/admin/mounts",
  "/admin/volumes": "/admin/mounts",
  "/admin/database-hosts": "/admin/databases",
  "/admin/database-services": "/admin/databases",
  "/admin/build": "/admin/catalog",
  "/admin/deploy": "/admin/deployments",
  "/admin/operations/advanced": "/admin/operations",
  "/admin/infra": "/admin/nodes",
  "/admin/infra/beacons": "/admin/nodes",
  "/admin/infra/networking": "/admin/endpoints",
  "/admin/infra/storage": "/admin/mounts",
  "/admin/access": "/admin/users",
  "/admin/platform": "/admin/settings",
  // Legacy group names from the previous 11-group IA
  "/admin/command": "/admin/overview",
  "/admin/network": "/admin/domains",
  "/admin/data": "/admin/databases",
  "/admin/automation": "/admin/cron-jobs",
  "/admin/tenancy": "/admin/organizations",
  // Legacy shims for old group deep-links
  "/admin/security-headers": "/admin/security",
  "/admin/mtls-ca": "/admin/mtls",
  "/admin/traffic-policies": "/admin/traffic",
  "/admin/dns-providers": "/admin/dns",
  "/admin/acme-accounts": "/admin/acme",
};

function resolveAlias(pathname: string): string {
  return ADMIN_ALIAS_ROUTES[pathname] ?? pathname;
}

export type AdminSidebarGroup = AdminNavGroup & { secondaryItems: AdminNavEntry[] };

export function adminPagesForRole(role?: string): AdminNavGroup[] {
  return adminPageRegistry.map((group) => ({ ...group, items: group.items.filter((item) => role === item.requiredRole) })).filter((group) => group.items.length > 0);
}

/**
 * Sidebar view: primaries always visible, secondaries collapsed under "More".
 * Search, palette, and breadcrumbs keep using adminPagesForRole (full list).
 */
export function adminSidebarGroups(role?: string): AdminSidebarGroup[] {
  return adminPageRegistry
    .map((group) => {
      const items = group.items.filter((item) => role === item.requiredRole);
      return {
        ...group,
        items: items.filter((item) => !item.secondary),
        secondaryItems: items.filter((item) => item.secondary),
      };
    })
    .filter((group) => group.items.length > 0 || group.secondaryItems.length > 0);
}

export function findAdminPage(pathname: string): AdminNavEntry | undefined {
  const resolved = resolveAlias(pathname);
  return adminPageRegistry.flatMap((group) => group.items)
    .filter((item) => resolved === item.href || resolved.startsWith(`${item.href}/`))
    .sort((left, right) => right.href.length - left.href.length)[0];
}

export function findAdminPageGroup(pathname: string): { groupTitle: string; pageLabel: string } | undefined {
  const resolved = resolveAlias(pathname);
  for (const group of adminPageRegistry) {
    const match = group.items
      .filter((item) => resolved === item.href || resolved.startsWith(`${item.href}/`))
      .sort((left, right) => right.href.length - left.href.length)[0];
    if (match) {
      return { groupTitle: group.title, pageLabel: match.label };
    }
  }
  return undefined;
}
