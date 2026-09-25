/**
 * Console page registry — single source of truth for the /console shell.
 *
 * Mirrors the adminPageRegistry pattern but scoped to the customer-facing
 * product surface. Every entry here MUST have a corresponding page under
 * app/console/ or be marked `workload: true` (dynamic server/app routes).
 * The console-nav component filters out entries without pages at build time.
 */
import {
  Archive,
  ArrowLeftRight,
  Box,
  Boxes,
  Calendar,
  Cpu,
  Database,
  Folder,
  Gauge,
  GitBranch,
  HardDrive,
  HeartPulse,
  History,
  Layers,
  LayoutDashboard,
  ListChecks,
  Network,
  Rocket,
  Server,
  Settings,
  Terminal,
  Users,
  type LucideIcon,
} from "lucide-react";

// ─── Types ────────────────────────────────────────────────────────────────────

export type Audience = "customer" | "admin" | "both";

export type ConsoleNavItem = {
  href: string;
  label: string;
  labelKey?: string;
  icon: LucideIcon;
  badge?: string;
  audience: Audience;
  /** Marks dynamic workload detail routes (e.g. /console/servers/[id]) */
  workload?: boolean;
};

export type ConsoleNavGroup = {
  title: string;
  items: ConsoleNavItem[];
};

// ─── Workload tab definitions ─────────────────────────────────────────────────

export type WorkloadTabId =
  | "overview"
  | "terminal"
  | "files"
  | "databases"
  | "schedules"
  | "tasks"
  | "backups"
  | "startup"
  | "network"
  | "mounts"
  | "users"
  | "resource-limits"
  | "settings"
  | "deployments"
  | "builds"
  | "git"
  | "processes"
  | "activity"
  | "transfer";

export type WorkloadTabConfig = {
  id: WorkloadTabId;
  labelKey: string;
  fallback: string;
  icon: LucideIcon;
  permissions: string[];
};

/**
 * Unified workload tabs — replaces both server-tabs.tsx and server-nav.tsx
 * hardcoded arrays. Rendered by both the /console sidebar and the /server/[id]
 * layout depending on audience.
 */
export const workloadTabs: WorkloadTabConfig[] = [
  { id: "overview", labelKey: "server.overview", fallback: "Overview", icon: LayoutDashboard, permissions: [] },
  { id: "terminal", labelKey: "server.terminal", fallback: "Terminal", icon: Terminal, permissions: ["websocket.connect", "control.console"] },
  { id: "files", labelKey: "server.files", fallback: "Files", icon: Folder, permissions: ["file.read"] },
  { id: "databases", labelKey: "server.databases", fallback: "Databases", icon: Database, permissions: ["database.read"] },
  { id: "schedules", labelKey: "server.schedules", fallback: "Schedules", icon: Calendar, permissions: ["schedule.read"] },
  { id: "tasks", labelKey: "server.scheduledTasks", fallback: "Scheduled Tasks", icon: ListChecks, permissions: ["schedule.read"] },
  { id: "backups", labelKey: "server.backups", fallback: "Backups", icon: Archive, permissions: ["backup.read"] },
  { id: "startup", labelKey: "server.startup", fallback: "Startup", icon: Rocket, permissions: ["startup.read"] },
  { id: "network", labelKey: "server.network", fallback: "Network", icon: Network, permissions: ["allocation.read"] },
  { id: "mounts", labelKey: "server.mounts", fallback: "Mounts", icon: HardDrive, permissions: ["mount.read"] },
  { id: "users", labelKey: "admin.users", fallback: "Users", icon: Users, permissions: ["user.read"] },
  { id: "resource-limits", labelKey: "server.resourceLimits", fallback: "Resources", icon: Gauge, permissions: ["settings.rename"] },
  { id: "settings", labelKey: "server.settings", fallback: "Settings", icon: Settings, permissions: ["settings.rename", "settings.reinstall", "file.sftp"] },
  { id: "deployments", labelKey: "server.deployments", fallback: "Deployments", icon: Layers, permissions: [] },
  { id: "builds", labelKey: "server.builds", fallback: "Builds", icon: Box, permissions: [] },
  { id: "git", labelKey: "server.git", fallback: "Git", icon: GitBranch, permissions: [] },
  { id: "processes", labelKey: "server.processes", fallback: "Processes", icon: Cpu, permissions: ["control.start"] },
  { id: "activity", labelKey: "admin.activity", fallback: "Activity", icon: History, permissions: ["activity.read"] },
  { id: "transfer", labelKey: "server.transfer", fallback: "Transfer", icon: ArrowLeftRight, permissions: ["settings.rename"] },
];

export const workloadTabGroups: Array<{ title: string; tabs: WorkloadTabId[] }> = [
  { title: "Daily", tabs: ["overview", "terminal", "files", "databases", "schedules", "tasks", "backups"] },
  { title: "Configuration", tabs: ["startup", "network", "mounts", "users", "resource-limits", "settings"] },
  { title: "Deploy & Ops", tabs: ["deployments", "builds", "git", "processes", "activity", "transfer"] },
];

/** Generate the href for a workload tab within the /console namespace. */
export function workloadTabHref(serverId: string, tab: WorkloadTabId): string {
  return tab === "overview" ? `/console/servers/${serverId}` : `/console/servers/${serverId}/${tab}`;
}

// ─── Top-level console navigation groups ─────────────────────────────────────

/**
 * These are the landing pages of the customer console. Each MUST have a
 * corresponding page.tsx under app/console/ or the nav will filter it out.
 */
export const consoleNavGroups: ConsoleNavGroup[] = [
  {
    title: "Overview",
    items: [
      { href: "/console", label: "Dashboard", labelKey: "console.dashboard", icon: LayoutDashboard, audience: "customer" },
      { href: "/console/health", label: "Platform Health", labelKey: "console.health", icon: HeartPulse, audience: "customer" },
    ],
  },
  {
    title: "Workloads",
    items: [
      { href: "/console/servers", label: "Game Servers", labelKey: "console.servers", icon: Server, audience: "customer", workload: true },
      { href: "/console/apps", label: "Applications", labelKey: "console.apps", icon: Boxes, audience: "customer", workload: true },
      { href: "/console/databases", label: "Databases", labelKey: "console.databases", icon: Database, audience: "customer" },
    ],
  },
  {
    title: "Operations",
    items: [
      { href: "/console/backups", label: "Backups", labelKey: "console.backups", icon: Archive, audience: "customer" },
      { href: "/console/domains", label: "Domains", labelKey: "console.domains", icon: Network, audience: "customer" },
    ],
  },
];
