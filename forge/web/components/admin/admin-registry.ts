import type { LucideIcon } from "lucide-react";
import {
  Activity, ArrowLeftRight, Award, BarChart3, Bell, Box, Bug, Cable, Clock, Cloud, Container, Database, GitBranch,
  FileText, FlaskConical, GanttChart, Globe, HardDrive, HeartPulse, KeyRound, Layers, Layout,
  Map, MapPin, Network, Plug, Scale, Server, ShieldCheck, SlidersHorizontal,
  RotateCcw, Terminal, Users, Shield,
} from "lucide-react";

export type AdminNavEntry = {
  label: string;
  labelKey: string;
  href: string;
  icon: LucideIcon;
  requiredRole: "admin";
  capability: "available" | "metadata-only";
  description: string;
  descriptionKey: string;
};

export type AdminNavGroup = { title: string; titleKey: string; items: AdminNavEntry[] };

export const adminPageRegistry: AdminNavGroup[] = [
  { title: "Operations", titleKey: "admin.navGroup.operations", items: [
    { label: "Overview", labelKey: "admin.nav.overview", href: "/admin/overview", icon: Server, requiredRole: "admin", capability: "available", description: "Live control-plane summary", descriptionKey: "admin.navDesc.overview" },
    { label: "Monitoring", labelKey: "admin.nav.monitoring", href: "/admin/monitoring", icon: HeartPulse, requiredRole: "admin", capability: "available", description: "Platform and node health", descriptionKey: "admin.navDesc.monitoring" },
    { label: "Health", labelKey: "admin.nav.health", href: "/admin/health", icon: HeartPulse, requiredRole: "admin", capability: "available", description: "Dependency and service diagnostics", descriptionKey: "admin.navDesc.health" },
    { label: "Activity", labelKey: "admin.nav.activity", href: "/admin/activity", icon: Activity, requiredRole: "admin", capability: "available", description: "Human-readable audit history", descriptionKey: "admin.navDesc.activity" },
    { label: "Cron Jobs", labelKey: "admin.nav.cronJobs", href: "/admin/cron-jobs", icon: Clock, requiredRole: "admin", capability: "available", description: "Schedule and manage automated tasks", descriptionKey: "admin.navDesc.cronJobs" },
    { label: "Host", labelKey: "admin.nav.host", href: "/admin/host", icon: Server, requiredRole: "admin", capability: "available", description: "Per-node system info, disk, memory, network and processes", descriptionKey: "admin.navDesc.host" },
    { label: "Operations", labelKey: "admin.nav.operations", href: "/admin/operations", icon: SlidersHorizontal, requiredRole: "admin", capability: "available", description: "Control-plane operation history and controls", descriptionKey: "admin.navDesc.operations" },
    { label: "Migrations", labelKey: "admin.nav.migrations", href: "/admin/migrations", icon: ArrowLeftRight, requiredRole: "admin", capability: "available", description: "Migration jobs and recovery plans", descriptionKey: "admin.navDesc.migrations" },
    { label: "Recovery", labelKey: "admin.nav.recovery", href: "/admin/migrations", icon: RotateCcw, requiredRole: "admin", capability: "available", description: "Recovery plan management", descriptionKey: "admin.navDesc.recovery" },
    { label: "Reconciliation", labelKey: "admin.nav.reconciliation", href: "/admin/reconciliation", icon: FlaskConical, requiredRole: "admin", capability: "available", description: "Drift detection and state reconciliation", descriptionKey: "admin.navDesc.reconciliation" },
  ]},
  { title: "Infrastructure", titleKey: "admin.navGroup.infrastructure", items: [
    { label: "Regions", labelKey: "admin.nav.regions", href: "/admin/regions", icon: Map, requiredRole: "admin", capability: "available", description: "Cluster regions", descriptionKey: "admin.navDesc.regions" },
    { label: "Locations", labelKey: "admin.nav.locations", href: "/admin/locations", icon: MapPin, requiredRole: "admin", capability: "available", description: "Node locations", descriptionKey: "admin.navDesc.locations" },
    { label: "Nodes", labelKey: "admin.nav.nodes", href: "/admin/nodes", icon: Network, requiredRole: "admin", capability: "available", description: "Daemon hosts", descriptionKey: "admin.navDesc.nodes" },
    { label: "Allocations", labelKey: "admin.nav.allocations", href: "/admin/allocations", icon: Cable, requiredRole: "admin", capability: "available", description: "Network allocations", descriptionKey: "admin.navDesc.allocations" },
    { label: "Database Hosts", labelKey: "admin.nav.databaseHosts", href: "/admin/databases", icon: Database, requiredRole: "admin", capability: "available", description: "Provisioning hosts", descriptionKey: "admin.navDesc.databaseHosts" },
    { label: "Mounts", labelKey: "admin.nav.mounts", href: "/admin/mounts", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Shared storage mounts", descriptionKey: "admin.navDesc.mounts" },
    { label: "Files", labelKey: "admin.nav.files", href: "/admin/files", icon: FileText, requiredRole: "admin", capability: "available", description: "Host file manager", descriptionKey: "admin.navDesc.files" },
    { label: "Terminal", labelKey: "admin.nav.terminal", href: "/admin/terminal", icon: Terminal, requiredRole: "admin", capability: "available", description: "Host shell terminal", descriptionKey: "admin.navDesc.terminal" },
  ]},
  { title: "Management", titleKey: "admin.navGroup.management", items: [
    { label: "Servers", labelKey: "admin.nav.servers", href: "/admin/servers", icon: Layers, requiredRole: "admin", capability: "available", description: "Game server instances", descriptionKey: "admin.navDesc.servers" },
    { label: "Apps", labelKey: "admin.nav.apps", href: "/admin/apps", icon: Box, requiredRole: "admin", capability: "available", description: "Container apps, Git repos, and Compose stacks", descriptionKey: "admin.navDesc.apps" },
    { label: "Users", labelKey: "admin.nav.users", href: "/admin/users", icon: Users, requiredRole: "admin", capability: "available", description: "Accounts and limits", descriptionKey: "admin.navDesc.users" },
    { label: "Roles", labelKey: "admin.nav.roles", href: "/admin/roles", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Additional role assignments", descriptionKey: "admin.navDesc.roles" },
    { label: "OAuth Clients", labelKey: "admin.nav.oauthClients", href: "/admin/oauth-clients", icon: KeyRound, requiredRole: "admin", capability: "available", description: "User-owned OAuth clients", descriptionKey: "admin.navDesc.oauthClients" },
    { label: "Organizations", labelKey: "admin.nav.organizations", href: "/admin/organizations", icon: Users, requiredRole: "admin", capability: "available", description: "Multi-tenant organizations", descriptionKey: "admin.navDesc.organizations" },
    { label: "Projects", labelKey: "admin.nav.projects", href: "/admin/projects", icon: Layers, requiredRole: "admin", capability: "available", description: "Projects within organizations", descriptionKey: "admin.navDesc.projects" },
    { label: "Environments", labelKey: "admin.nav.environments", href: "/admin/environments", icon: Globe, requiredRole: "admin", capability: "available", description: "Deployment environments and variables", descriptionKey: "admin.navDesc.environments" },
  ]},
  { title: "Services", titleKey: "admin.navGroup.services", items: [
    { label: "Nests & Eggs", labelKey: "admin.nav.nestsEggs", href: "/admin/nests", icon: Box, requiredRole: "admin", capability: "available", description: "Canonical service definitions", descriptionKey: "admin.navDesc.nestsEggs" },
    { label: "App Templates", labelKey: "admin.nav.appTemplates", href: "/admin/app-templates", icon: Layout, requiredRole: "admin", capability: "available", description: "Application deployment templates", descriptionKey: "admin.navDesc.appTemplates" },
    { label: "Compatibility Templates", labelKey: "admin.nav.compatibilityTemplates", href: "/admin/templates", icon: Box, requiredRole: "admin", capability: "available", description: "Legacy compatibility templates", descriptionKey: "admin.navDesc.compatibilityTemplates" },
    { label: "Webhooks", labelKey: "admin.nav.webhooks", href: "/admin/webhooks", icon: Globe, requiredRole: "admin", capability: "available", description: "Event delivery", descriptionKey: "admin.navDesc.webhooks" },
    { label: "Plugins", labelKey: "admin.nav.plugins", href: "/admin/plugins", icon: Plug, requiredRole: "admin", capability: "metadata-only", description: "Manifest registry; runtime unavailable", descriptionKey: "admin.navDesc.plugins" },
    { label: "API Keys", labelKey: "admin.nav.apiKeys", href: "/admin/api", icon: KeyRound, requiredRole: "admin", capability: "available", description: "Application API credentials", descriptionKey: "admin.navDesc.apiKeys" },
    { label: "Settings", labelKey: "admin.nav.settings", href: "/admin/settings", icon: SlidersHorizontal, requiredRole: "admin", capability: "available", description: "Panel configuration", descriptionKey: "admin.navDesc.settings" },
    { label: "Notifications", labelKey: "admin.nav.notifications", href: "/admin/notifications", icon: Bell, requiredRole: "admin", capability: "available", description: "Channels, alerts, and delivery logs", descriptionKey: "admin.navDesc.notifications" },
  ]},
  { title: "Advanced", titleKey: "admin.navGroup.advanced", items: [
    { label: "Docker", labelKey: "admin.nav.docker", href: "/admin/docker", icon: Container, requiredRole: "admin", capability: "available", description: "Container, image, network, and volume management", descriptionKey: "admin.navDesc.docker" },
    { label: "Scheduler", labelKey: "admin.nav.scheduler", href: "/admin/scheduler", icon: BarChart3, requiredRole: "admin", capability: "available", description: "Scoring, affinity rules, and placement constraints", descriptionKey: "admin.navDesc.scheduler" },
    { label: "Auto-Scaler", labelKey: "admin.nav.autoScaler", href: "/admin/autoscaler", icon: Scale, requiredRole: "admin", capability: "available", description: "Automatic resource scaling policies", descriptionKey: "admin.navDesc.autoScaler" },
    { label: "Deployments", labelKey: "admin.nav.deployments", href: "/admin/deployments", icon: ArrowLeftRight, requiredRole: "admin", capability: "available", description: "Blue-green and rolling deployments", descriptionKey: "admin.navDesc.deployments" },
    { label: "Preview Deployments", labelKey: "admin.nav.previewDeployments", href: "/admin/preview-deployments", icon: Layout, requiredRole: "admin", capability: "available", description: "PR-based preview deployments", descriptionKey: "admin.navDesc.previewDeployments" },
    { label: "Failover", labelKey: "admin.nav.failover", href: "/admin/failover", icon: Bug, requiredRole: "admin", capability: "available", description: "Automatic failover policies and crash simulation", descriptionKey: "admin.navDesc.failover" },
    { label: "Load Balancer", labelKey: "admin.nav.loadBalancer", href: "/admin/load-balancer", icon: GanttChart, requiredRole: "admin", capability: "available", description: "Target groups and traffic routing", descriptionKey: "admin.navDesc.loadBalancer" },
    { label: "Traffic", labelKey: "admin.nav.traffic", href: "/admin/traffic", icon: Globe, requiredRole: "admin", capability: "available", description: "Route rules and traffic policies", descriptionKey: "admin.navDesc.traffic" },
    { label: "Domains", labelKey: "admin.nav.domains", href: "/admin/domains", icon: Globe, requiredRole: "admin", capability: "available", description: "Custom domain management", descriptionKey: "admin.navDesc.domains" },
    { label: "Certificates", labelKey: "admin.nav.certificates", href: "/admin/certificates", icon: Award, requiredRole: "admin", capability: "available", description: "TLS/SSL certificate management", descriptionKey: "admin.navDesc.certificates" },
    { label: "Security", labelKey: "admin.nav.security", href: "/admin/security", icon: Shield, requiredRole: "admin", capability: "available", description: "HTTP security headers and per-domain overrides", descriptionKey: "admin.navDesc.security" },
    { label: "Cloud", labelKey: "admin.nav.cloud", href: "/admin/cloud", icon: Cloud, requiredRole: "admin", capability: "available", description: "Cloud provider integrations and instance provisioning", descriptionKey: "admin.navDesc.cloud" },
    { label: "Compose", labelKey: "admin.nav.compose", href: "/admin/compose", icon: Container, requiredRole: "admin", capability: "available", description: "Docker Compose file import and management", descriptionKey: "admin.navDesc.compose" },
    { label: "Social Login", labelKey: "admin.nav.socialLogin", href: "/admin/social", icon: Users, requiredRole: "admin", capability: "available", description: "OAuth social login providers", descriptionKey: "admin.navDesc.socialLogin" },
    { label: "mTLS Certificates", labelKey: "admin.nav.mtlsCertificates", href: "/admin/mtls", icon: Shield, requiredRole: "admin", capability: "available", description: "Mutual TLS certificate management", descriptionKey: "admin.navDesc.mtlsCertificates" },
    { label: "Backups", labelKey: "admin.nav.backups", href: "/admin/backups", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Backup, artifact, and restore operations", descriptionKey: "admin.navDesc.backups" },
    { label: "Database Services", labelKey: "admin.nav.databaseServices", href: "/admin/database-services", icon: Database, requiredRole: "admin", capability: "available", description: "Managed application databases", descriptionKey: "admin.navDesc.databaseServices" },
    { label: "App Store", labelKey: "admin.nav.appStore", href: "/admin/app-store", icon: Box, requiredRole: "admin", capability: "available", description: "Install packaged services", descriptionKey: "admin.navDesc.appStore" },
    { label: "Endpoints", labelKey: "admin.nav.endpoints", href: "/admin/endpoints", icon: Globe, requiredRole: "admin", capability: "available", description: "Public endpoint inventory", descriptionKey: "admin.navDesc.endpoints" },
    { label: "Firewall", labelKey: "admin.nav.firewall", href: "/admin/firewall", icon: Shield, requiredRole: "admin", capability: "available", description: "Firewall rules and port forwards", descriptionKey: "admin.navDesc.firewall" },
    { label: "Git Connections", labelKey: "admin.nav.gitConnections", href: "/admin/git", icon: Plug, requiredRole: "admin", capability: "available", description: "Credentials, providers, and sources", descriptionKey: "admin.navDesc.gitConnections" },
    { label: "Git Providers", labelKey: "admin.nav.gitProviders", href: "/admin/git-providers", icon: GitBranch, requiredRole: "admin", capability: "available", description: "Provider accounts", descriptionKey: "admin.navDesc.gitProviders" },
    { label: "Source Deployments", labelKey: "admin.nav.sourceDeployments", href: "/admin/source-deployments", icon: GitBranch, requiredRole: "admin", capability: "available", description: "Build and deploy repositories", descriptionKey: "admin.navDesc.sourceDeployments" },
  ]},
];

export function adminPagesForRole(role?: string): AdminNavGroup[] {
  return adminPageRegistry.map((group) => ({ ...group, items: group.items.filter((item) => role === item.requiredRole) })).filter((group) => group.items.length > 0);
}

export function findAdminPage(pathname: string): AdminNavEntry | undefined {
  return adminPageRegistry.flatMap((group) => group.items)
    .filter((item) => pathname === item.href || pathname.startsWith(`${item.href}/`))
    .sort((left, right) => right.href.length - left.href.length)[0];
}
