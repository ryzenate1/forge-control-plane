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
  ArrowLeftRight, Award, BarChart3, Bell, Bug, Cable, Clock, Cloud, Cpu, CreditCard, FileText, FlaskConical, FolderLock,
  Globe, HardDrive, KeyRound, Layers, Mail, Network, Plug, Scale, Shield, ShieldCheck, SlidersHorizontal,
  Terminal, Ticket, Trash2, Users, Workflow,
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
};

export type AdminNavGroup = { title: string; titleKey: string; items: AdminNavEntry[] };

/**
 * Consolidated IA — 7 goal-oriented groups per docs/architecture/target-ia.md §3.
 * COMMAND (4) · BUILD (8) · DEPLOY (8) · INFRA (30) · OPERATIONS (13) · ACCESS (7) · PLATFORM (7) = 77 hrefs.
 * Primary ~32 (COMMAND 4 + BUILD 4 + DEPLOY 4 + INFRA 4 top + OPERATIONS 4 + ACCESS 3 + PLATFORM 3) surface as
 * top-level nav; remaining 77 become contextual tabs/drawers within those + Advanced sections (SUB_GROUPS).
 * All 77 hrefs retained for compat; legacy paths map via ADMIN_ALIAS_ROUTES.
 */
export const adminPageRegistry: AdminNavGroup[] = [
  { title: "Command", titleKey: "admin.navGroup.command", items: [
    { label: "Overview", labelKey: "admin.nav.overview", href: "/admin/overview", icon: OverviewDashboardIcon, requiredRole: "admin", capability: "available", description: "Live control-plane summary and fleet health at a glance", descriptionKey: "admin.navDesc.overview" },
    { label: "Monitoring", labelKey: "admin.nav.monitoring", href: "/admin/monitoring", icon: MonitoringPulseIcon, requiredRole: "admin", capability: "available", description: "Platform and node health dashboards", descriptionKey: "admin.navDesc.monitoring" },
    { label: "Health", labelKey: "admin.nav.health", href: "/admin/health", icon: HealthECGIcon, requiredRole: "admin", capability: "available", description: "Dependency and service diagnostics", descriptionKey: "admin.navDesc.health" },
    { label: "Activity", labelKey: "admin.nav.activity", href: "/admin/activity", icon: ActivityWaveIcon, requiredRole: "admin", capability: "available", description: "Human-readable audit and activity history", descriptionKey: "admin.navDesc.activity" },
  ]},
  { title: "Build", titleKey: "admin.navGroup.build", items: [
    { label: "Servers", labelKey: "admin.nav.servers", href: "/admin/servers", icon: GamepadIcon, requiredRole: "admin", capability: "available", description: "Game server instances and lifecycle", descriptionKey: "admin.navDesc.servers" },
    { label: "Apps", labelKey: "admin.nav.apps", href: "/admin/apps", icon: ApplicationsCubeIcon, requiredRole: "admin", capability: "available", description: "Container apps, Git repos, and stacks", descriptionKey: "admin.navDesc.apps" },
    { label: "App Store", labelKey: "admin.nav.appStore", href: "/admin/app-store", icon: AppStoreIcon, requiredRole: "admin", capability: "available", description: "Curated one-click service catalog", descriptionKey: "admin.navDesc.appStore" },
    { label: "Catalog", labelKey: "admin.nav.catalog", href: "/admin/catalog", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "One-click service catalog and provisioning", descriptionKey: "admin.navDesc.catalog" },
    { label: "Service Definitions", labelKey: "admin.nav.nestsEggs", href: "/admin/nests", icon: ApplicationsCubeIcon, requiredRole: "admin", capability: "available", description: "Reusable server blueprints and templates", descriptionKey: "admin.navDesc.nestsEggs" },
    { label: "App Blueprints", labelKey: "admin.nav.appTemplates", href: "/admin/app-templates", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "Application deployment blueprints", descriptionKey: "admin.navDesc.appTemplates" },
    { label: "Legacy Templates", labelKey: "admin.nav.compatibilityTemplates", href: "/admin/templates", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "Legacy compatibility templates", descriptionKey: "admin.navDesc.compatibilityTemplates" },
    { label: "Forgefile", labelKey: "admin.nav.forgefile", href: "/admin/forgefile", icon: FileText, requiredRole: "admin", capability: "available", description: "Env-as-code forgefile validate & apply", descriptionKey: "admin.navDesc.forgefile" },
  ]},
  { title: "Deploy", titleKey: "admin.navGroup.deploy", items: [
    { label: "Deployments", labelKey: "admin.nav.deployments", href: "/admin/deployments", icon: RocketLaunchIcon, requiredRole: "admin", capability: "available", description: "Blue-green and rolling deployments", descriptionKey: "admin.navDesc.deployments" },
    { label: "Preview Deployments", labelKey: "admin.nav.previewDeployments", href: "/admin/preview-deployments", icon: TemplateSheetIcon, requiredRole: "admin", capability: "available", description: "Pull-request preview environments", descriptionKey: "admin.navDesc.previewDeployments" },
    { label: "Source Deployments", labelKey: "admin.nav.sourceDeployments", href: "/admin/source-deployments", icon: GitBranchTreeIcon, requiredRole: "admin", capability: "available", description: "Build and deploy from Git sources", descriptionKey: "admin.navDesc.sourceDeployments" },
    { label: "Compose Stacks", labelKey: "admin.nav.compose", href: "/admin/compose", icon: ComposeSheetsIcon, requiredRole: "admin", capability: "available", description: "Docker Compose stacks and imports", descriptionKey: "admin.navDesc.compose" },
    { label: "Git Integrations", labelKey: "admin.nav.gitConnections", href: "/admin/git", icon: GitBranchTreeIcon, requiredRole: "admin", capability: "available", description: "Git credentials, providers, and sources", descriptionKey: "admin.navDesc.gitConnections" },
    { label: "Git Providers", labelKey: "admin.nav.gitProviders", href: "/admin/git-providers", icon: GitBranchTreeIcon, requiredRole: "admin", capability: "available", description: "Connected Git provider accounts", descriptionKey: "admin.navDesc.gitProviders" },
    { label: "Pipelines", labelKey: "admin.nav.pipelines", href: "/admin/pipelines", icon: PipelineFlowIcon, requiredRole: "admin", capability: "available", description: "CI/CD pipelines and delivery workflows", descriptionKey: "admin.navDesc.pipelines" },
    { label: "Zero Downtime", labelKey: "admin.nav.zeroDowntime", href: "/admin/zerodowntime", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Zero-downtime releases & health gates", descriptionKey: "admin.navDesc.zeroDowntime" },
  ]},
  { title: "Infrastructure", titleKey: "admin.navGroup.infrastructure", items: [
    { label: "Nodes", labelKey: "admin.nav.nodes", href: "/admin/nodes", icon: NodeHostIcon, requiredRole: "admin", capability: "available", description: "Daemon hosts and heartbeat status", descriptionKey: "admin.navDesc.nodes" },
    { label: "Regions", labelKey: "admin.nav.regions", href: "/admin/regions", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "Cluster regions and placement zones", descriptionKey: "admin.navDesc.regions" },
    { label: "Locations", labelKey: "admin.nav.locations", href: "/admin/locations", icon: LocationPinIcon, requiredRole: "admin", capability: "available", description: "Physical and logical node locations", descriptionKey: "admin.navDesc.locations" },
    { label: "Capabilities", labelKey: "admin.nav.capabilities", href: "/admin/capabilities", icon: Cpu, requiredRole: "admin", capability: "available", description: "Global node capability inventory and drift delta", descriptionKey: "admin.navDesc.capabilities" },
    { label: "Onboarding Tokens", labelKey: "admin.nav.onboardingTokens", href: "/admin/onboarding-tokens", icon: Ticket, requiredRole: "admin", capability: "available", description: "Node onboarding token lifecycle — issue, approve, revoke", descriptionKey: "admin.navDesc.onboardingTokens" },
    { label: "Host", labelKey: "admin.nav.host", href: "/admin/host", icon: ServerRackIcon, requiredRole: "admin", capability: "available", description: "Per-node system, disk, memory and processes", descriptionKey: "admin.navDesc.host" },
    { label: "Kubernetes", labelKey: "admin.nav.kubernetes", href: "/admin/kubernetes", icon: ComposeSheetsIcon, requiredRole: "admin", capability: "available", description: "Kubernetes pods, deployments and services", descriptionKey: "admin.navDesc.kubernetes" },
    { label: "Docker", labelKey: "admin.nav.docker", href: "/admin/docker", icon: ContainerShippingIcon, requiredRole: "admin", capability: "available", description: "Container, image, network and volume management", descriptionKey: "admin.navDesc.docker" },
    { label: "Cloud Instances", labelKey: "admin.nav.cloud", href: "/admin/cloud", icon: Cloud, requiredRole: "admin", capability: "available", description: "Cloud provider instances and provisioning", descriptionKey: "admin.navDesc.cloud" },
    { label: "Host Files", labelKey: "admin.nav.files", href: "/admin/files", icon: FileText, requiredRole: "admin", capability: "available", description: "Browse and manage host files", descriptionKey: "admin.navDesc.files" },
    { label: "Terminal", labelKey: "admin.nav.terminal", href: "/admin/terminal", icon: Terminal, requiredRole: "admin", capability: "available", description: "Secure host shell and console access", descriptionKey: "admin.navDesc.terminal" },
    { label: "Storage Mounts", labelKey: "admin.nav.mounts", href: "/admin/mounts", icon: StoragePlattersIcon, requiredRole: "admin", capability: "available", description: "Shared storage mounts and volumes", descriptionKey: "admin.navDesc.mounts" },
    { label: "SFTP", labelKey: "admin.nav.sftp", href: "/admin/sftp", icon: FolderLock, requiredRole: "admin", capability: "available", description: "Global and per-node SFTP configuration", descriptionKey: "admin.navDesc.sftp" },
  ]},
  { title: "Network", titleKey: "admin.navGroup.network", items: [
    { label: "IP Allocations", labelKey: "admin.nav.allocations", href: "/admin/allocations", icon: Cable, requiredRole: "admin", capability: "available", description: "Network ports and IP bindings", descriptionKey: "admin.navDesc.allocations" },
    { label: "Endpoints", labelKey: "admin.nav.endpoints", href: "/admin/endpoints", icon: EndpointPlugIcon, requiredRole: "admin", capability: "available", description: "Public endpoint inventory", descriptionKey: "admin.navDesc.endpoints" },
    { label: "Service Discovery", labelKey: "admin.nav.discovery", href: "/admin/discovery", icon: NetworkMeshNodesIcon, requiredRole: "admin", capability: "available", description: "Service discovery and network policy", descriptionKey: "admin.navDesc.discovery" },
    { label: "Firewall", labelKey: "admin.nav.firewall", href: "/admin/firewall", icon: Shield, requiredRole: "admin", capability: "available", description: "Firewall rules and port forwarding", descriptionKey: "admin.navDesc.firewall" },
    { label: "Load Balancer", labelKey: "admin.nav.loadBalancer", href: "/admin/load-balancer", icon: LoadBalancerSplitIcon, requiredRole: "admin", capability: "available", description: "Target groups and traffic routing", descriptionKey: "admin.navDesc.loadBalancer" },
    { label: "Traffic Policies", labelKey: "admin.nav.traffic", href: "/admin/traffic", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "Route rules and traffic shaping", descriptionKey: "admin.navDesc.traffic" },
    { label: "Domains", labelKey: "admin.nav.domains", href: "/admin/domains", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "Custom domain management and verification", descriptionKey: "admin.navDesc.domains" },
    { label: "DNS Providers", labelKey: "admin.nav.dnsProviders", href: "/admin/dns", icon: GlobeGridIcon, requiredRole: "admin", capability: "available", description: "ACME DNS-01 providers", descriptionKey: "admin.navDesc.dnsProviders" },
    { label: "Gateways", labelKey: "admin.nav.gateways", href: "/admin/gateways", icon: GatewayRouterIcon, requiredRole: "admin", capability: "available", description: "Edge gateway routers, services and middlewares", descriptionKey: "admin.navDesc.gateways" },
    { label: "Cross-Node", labelKey: "admin.nav.crossnode", href: "/admin/crossnode", icon: NetworkMeshNodesIcon, requiredRole: "admin", capability: "available", description: "Cross-node resolver cache and ingress sync controls", descriptionKey: "admin.navDesc.crossnode" },
  ]},
  { title: "Data", titleKey: "admin.navGroup.data", items: [
    { label: "Database Hosts", labelKey: "admin.nav.databaseHosts", href: "/admin/databases", icon: DatabaseCylinderIcon, requiredRole: "admin", capability: "available", description: "Managed database hosts", descriptionKey: "admin.navDesc.databaseHosts" },
    { label: "Database Services", labelKey: "admin.nav.databaseServices", href: "/admin/database-services", icon: DatabaseCylinderIcon, requiredRole: "admin", capability: "available", description: "Managed application databases", descriptionKey: "admin.navDesc.databaseServices" },
  ]},
  { title: "Operations", titleKey: "admin.navGroup.operations", items: [
    { label: "Operations", labelKey: "admin.nav.operations", href: "/admin/operations", icon: SlidersHorizontal, requiredRole: "admin", capability: "available", description: "Control-plane operation history and controls", descriptionKey: "admin.navDesc.operations" },
    { label: "Migrations", labelKey: "admin.nav.migrations", href: "/admin/migrations", icon: ArrowLeftRight, requiredRole: "admin", capability: "available", description: "Live migration jobs and recovery plans", descriptionKey: "admin.navDesc.migrations" },
    { label: "Reconciliation Center", labelKey: "admin.nav.reconciliation", href: "/admin/reconciliation", icon: FlaskConical, requiredRole: "admin", capability: "available", description: "Drift detection and state reconciliation", descriptionKey: "admin.navDesc.reconciliation" },
    { label: "Orphan Remediation", labelKey: "admin.nav.orphans", href: "/admin/orphans", icon: Bug, requiredRole: "admin", capability: "available", description: "Orphaned servers and databases requiring manual cleanup", descriptionKey: "admin.navDesc.orphans" },
    { label: "Cleanup", labelKey: "admin.nav.cleanup", href: "/admin/cleanup", icon: Trash2, requiredRole: "admin", capability: "available", description: "Garbage collection, Inspect & Run cleanup for stale resources", descriptionKey: "admin.navDesc.cleanup" },
    { label: "Backups", labelKey: "admin.nav.backups", href: "/admin/backups", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Backups, artifacts and restore operations", descriptionKey: "admin.navDesc.backups" },
    { label: "Failover", labelKey: "admin.nav.failover", href: "/admin/failover", icon: Bug, requiredRole: "admin", capability: "available", description: "Automatic failover and recovery", descriptionKey: "admin.navDesc.failover" },
  ]},
  { title: "Automation", titleKey: "admin.navGroup.automation", items: [
    { label: "Cron Jobs", labelKey: "admin.nav.cronJobs", href: "/admin/cron-jobs", icon: Clock, requiredRole: "admin", capability: "available", description: "Scheduled and recurring automation", descriptionKey: "admin.navDesc.cronJobs" },
    { label: "Procedures", labelKey: "admin.nav.procedures", href: "/admin/procedures", icon: Workflow, requiredRole: "admin", capability: "available", description: "Workflow procedures, schedules & executions", descriptionKey: "admin.navDesc.procedures" },
    { label: "Placement Scheduler", labelKey: "admin.nav.scheduler", href: "/admin/scheduler", icon: BarChart3, requiredRole: "admin", capability: "available", description: "Placement scoring and affinity rules", descriptionKey: "admin.navDesc.scheduler" },
    { label: "Auto Scaling", labelKey: "admin.nav.autoScaler", href: "/admin/autoscaler", icon: Scale, requiredRole: "admin", capability: "available", description: "Automatic scaling policies", descriptionKey: "admin.navDesc.autoScaler" },
    { label: "Env Affinity", labelKey: "admin.nav.envAffinity", href: "/admin/env-affinity", icon: Network, requiredRole: "admin", capability: "available", description: "Placement explain / enrich & constraints", descriptionKey: "admin.navDesc.envAffinity" },
    { label: "Node Autoscaler", labelKey: "admin.nav.nodeAutoscaler", href: "/admin/node-autoscaler", icon: Scale, requiredRole: "admin", capability: "available", description: "Cloud node autoscale policies & events", descriptionKey: "admin.navDesc.nodeAutoscaler" },
  ]},
  { title: "Access", titleKey: "admin.navGroup.access", items: [
    { label: "Users", labelKey: "admin.nav.users", href: "/admin/users", icon: Users, requiredRole: "admin", capability: "available", description: "Accounts, limits and status", descriptionKey: "admin.navDesc.users" },
    { label: "Roles", labelKey: "admin.nav.roles", href: "/admin/roles", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Role definitions and permissions", descriptionKey: "admin.navDesc.roles" },
    { label: "OAuth Clients", labelKey: "admin.nav.oauthClients", href: "/admin/oauth-clients", icon: KeyRound, requiredRole: "admin", capability: "available", description: "OAuth clients and API access", descriptionKey: "admin.navDesc.oauthClients" },
    { label: "Single Sign-On", labelKey: "admin.nav.socialLogin", href: "/admin/social", icon: Users, requiredRole: "admin", capability: "available", description: "Social and enterprise SSO providers", descriptionKey: "admin.navDesc.socialLogin" },
  ]},
  { title: "Tenancy", titleKey: "admin.navGroup.tenancy", items: [
    { label: "Organizations", labelKey: "admin.nav.organizations", href: "/admin/organizations", icon: Users, requiredRole: "admin", capability: "available", description: "Multi-tenant organizations", descriptionKey: "admin.navDesc.organizations" },
    { label: "Projects", labelKey: "admin.nav.projects", href: "/admin/projects", icon: Layers, requiredRole: "admin", capability: "available", description: "Projects within organizations", descriptionKey: "admin.navDesc.projects" },
    { label: "Environments", labelKey: "admin.nav.environments", href: "/admin/environments", icon: Globe, requiredRole: "admin", capability: "available", description: "Environment-scoped variables and stages", descriptionKey: "admin.navDesc.environments" },
  ]},
  { title: "Platform", titleKey: "admin.navGroup.platform", items: [
    { label: "Certificates", labelKey: "admin.nav.certificates", href: "/admin/certificates", icon: Award, requiredRole: "admin", capability: "available", description: "TLS certificates and automated issuance", descriptionKey: "admin.navDesc.certificates" },
    { label: "mTLS & Private CA", labelKey: "admin.nav.mtlsCertificates", href: "/admin/mtls", icon: Shield, requiredRole: "admin", capability: "available", description: "Mutual TLS and private certificates", descriptionKey: "admin.navDesc.mtlsCertificates" },
    { label: "Security Headers", labelKey: "admin.nav.security", href: "/admin/security", icon: Shield, requiredRole: "admin", capability: "available", description: "Security headers and per-domain policies", descriptionKey: "admin.navDesc.security" },
    { label: "Webhooks", labelKey: "admin.nav.webhooks", href: "/admin/webhooks", icon: Globe, requiredRole: "admin", capability: "available", description: "Event delivery and webhook endpoints", descriptionKey: "admin.navDesc.webhooks" },
    { label: "ACME Accounts", labelKey: "admin.nav.acme", href: "/admin/acme", icon: Award, requiredRole: "admin", capability: "available", description: "ACME accounts and DNS credentials", descriptionKey: "admin.navDesc.acme" },
    { label: "Plugins", labelKey: "admin.nav.plugins", href: "/admin/plugins", icon: Plug, requiredRole: "admin", capability: "metadata-only", description: "Extensions and marketplace plugins", descriptionKey: "admin.navDesc.plugins" },
    { label: "API Keys", labelKey: "admin.nav.apiKeys", href: "/admin/api", icon: KeyRound, requiredRole: "admin", capability: "available", description: "API keys and programmatic access", descriptionKey: "admin.navDesc.apiKeys" },
    { label: "Platform Settings", labelKey: "admin.nav.settings", href: "/admin/settings", icon: SlidersHorizontal, requiredRole: "admin", capability: "available", description: "Global platform configuration", descriptionKey: "admin.navDesc.settings" },
    { label: "Mail", labelKey: "admin.nav.mail", href: "/admin/mail", icon: Mail, requiredRole: "admin", capability: "available", description: "SMTP mail settings & triggers", descriptionKey: "admin.navDesc.mail" },
    { label: "Notifications", labelKey: "admin.nav.notifications", href: "/admin/notifications", icon: Bell, requiredRole: "admin", capability: "available", description: "Channels, alerts and delivery logs", descriptionKey: "admin.navDesc.notifications" },
    { label: "Billing", labelKey: "admin.nav.billing", href: "/admin/billing", icon: CreditCard, requiredRole: "admin", capability: "available", description: "Billing plans, org quotas and usage metering", descriptionKey: "admin.navDesc.billing" },
    { label: "Onboarding", labelKey: "admin.nav.onboarding", href: "/admin/onboarding", icon: Workflow, requiredRole: "admin", capability: "available", description: "Guided onboarding — repos, connect & deploy", descriptionKey: "admin.navDesc.onboarding" },
  ]},
];

/** Invisible/demo alias routes that redirect but should not appear as duplicate nav items. */
export const ADMIN_ALIAS_ROUTES: Record<string, string> = {
  "/admin/containers": "/admin/docker",
  "/admin/logs": "/admin/activity",
  // Vocabulary normalization — target-ia §2 (Node → Beacon)
  "/admin/beacons": "/admin/nodes",
  "/admin/beacon": "/admin/nodes",
  // Consolidated IA compat — new 32 top-level redirect to contextual tabs (all 77 retained)
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

export function adminPagesForRole(role?: string): AdminNavGroup[] {
  return adminPageRegistry.map((group) => ({ ...group, items: group.items.filter((item) => role === item.requiredRole) })).filter((group) => group.items.length > 0);
}

export function findAdminPage(pathname: string): AdminNavEntry | undefined {
  const resolved = resolveAlias(pathname);
  return adminPageRegistry.flatMap((group) => group.items)
    .filter((item) => resolved === item.href || resolved.startsWith(`${item.href}/`))
    .sort((left, right) => right.href.length - left.href.length)[0];
}
