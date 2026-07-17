import type { LucideIcon } from "lucide-react";
import {
  Activity, ArrowLeftRight, BarChart3, Box, Bug, Cable, Cloud, Database,
  FileText, GanttChart, Globe, HardDrive, HeartPulse, KeyRound, Layers, Layout,
  Map, MapPin, Network, Plug, Scale, Server, ShieldCheck, SlidersHorizontal,
  Users, Workflow,
} from "lucide-react";

export type AdminNavEntry = {
  label: string;
  href: string;
  icon: LucideIcon;
  requiredRole: "admin";
  capability: "available" | "metadata-only";
  description: string;
};

export type AdminNavGroup = { title: string; items: AdminNavEntry[] };

export const adminPageRegistry: AdminNavGroup[] = [
  { title: "Operations", items: [
    { label: "Overview", href: "/admin/overview", icon: Server, requiredRole: "admin", capability: "available", description: "Live control-plane summary" },
    { label: "Monitoring", href: "/admin/monitoring", icon: HeartPulse, requiredRole: "admin", capability: "available", description: "Platform and node health" },
    { label: "Activity", href: "/admin/activity", icon: Activity, requiredRole: "admin", capability: "available", description: "Human-readable audit history" },
    { label: "Migrations & Recovery", href: "/admin/operations", icon: Workflow, requiredRole: "admin", capability: "metadata-only", description: "Planning only; no workload executor available" },
  ]},
  { title: "Infrastructure", items: [
    { label: "Regions", href: "/admin/regions", icon: Map, requiredRole: "admin", capability: "available", description: "Cluster regions" },
    { label: "Locations", href: "/admin/locations", icon: MapPin, requiredRole: "admin", capability: "available", description: "Node locations" },
    { label: "Nodes", href: "/admin/nodes", icon: Network, requiredRole: "admin", capability: "available", description: "Daemon hosts" },
    { label: "Allocations", href: "/admin/allocations", icon: Cable, requiredRole: "admin", capability: "available", description: "Network allocations" },
    { label: "Database Hosts", href: "/admin/databases", icon: Database, requiredRole: "admin", capability: "available", description: "Provisioning hosts" },
    { label: "Mounts", href: "/admin/mounts", icon: HardDrive, requiredRole: "admin", capability: "available", description: "Shared storage mounts" },
  ]},
  { title: "Management", items: [
    { label: "Servers", href: "/admin/servers", icon: Layers, requiredRole: "admin", capability: "available", description: "Game server instances" },
    { label: "Users", href: "/admin/users", icon: Users, requiredRole: "admin", capability: "available", description: "Accounts and limits" },
    { label: "Roles", href: "/admin/roles", icon: ShieldCheck, requiredRole: "admin", capability: "available", description: "Additional role assignments" },
    { label: "OAuth Clients", href: "/admin/oauth-clients", icon: KeyRound, requiredRole: "admin", capability: "available", description: "User-owned OAuth clients" },
  ]},
  { title: "Services", items: [
    { label: "Nests & Eggs", href: "/admin/nests", icon: Box, requiredRole: "admin", capability: "available", description: "Canonical service definitions" },
    { label: "Compatibility Templates", href: "/admin/templates", icon: Layout, requiredRole: "admin", capability: "available", description: "Legacy compatibility templates" },
    { label: "Webhooks", href: "/admin/webhooks", icon: Globe, requiredRole: "admin", capability: "available", description: "Event delivery" },
    { label: "Plugins", href: "/admin/plugins", icon: Plug, requiredRole: "admin", capability: "metadata-only", description: "Manifest registry; runtime unavailable" },
    { label: "API Keys", href: "/admin/api", icon: KeyRound, requiredRole: "admin", capability: "available", description: "Application API credentials" },
    { label: "Settings", href: "/admin/settings", icon: SlidersHorizontal, requiredRole: "admin", capability: "available", description: "Panel configuration" },
  ]},
  { title: "Advanced", items: [
    { label: "Scheduler", href: "/admin/scheduler", icon: BarChart3, requiredRole: "admin", capability: "available", description: "Scoring, affinity rules, and placement constraints" },
    { label: "Auto-Scaler", href: "/admin/autoscaler", icon: Scale, requiredRole: "admin", capability: "available", description: "Automatic resource scaling policies" },
    { label: "Deployments", href: "/admin/deployments", icon: ArrowLeftRight, requiredRole: "admin", capability: "available", description: "Blue-green and rolling deployments" },
    { label: "Failover", href: "/admin/failover", icon: Bug, requiredRole: "admin", capability: "available", description: "Automatic failover policies and crash simulation" },
    { label: "Load Balancer", href: "/admin/load-balancer", icon: GanttChart, requiredRole: "admin", capability: "available", description: "Target groups and traffic routing" },
    { label: "Traffic", href: "/admin/traffic", icon: Globe, requiredRole: "admin", capability: "available", description: "Route rules and traffic policies" },
    { label: "Cloud", href: "/admin/cloud", icon: Cloud, requiredRole: "admin", capability: "available", description: "Cloud provider integrations and instance provisioning" },
    { label: "Social Login", href: "/admin/social", icon: Users, requiredRole: "admin", capability: "available", description: "OAuth social login providers" },
  ]},
];

export function adminPagesForRole(role?: string): AdminNavGroup[] {
  return adminPageRegistry.map((group) => ({ ...group, items: group.items.filter((item) => role === item.requiredRole) })).filter((group) => group.items.length > 0);
}
