/**
 * Console page registry — single source of truth for the /console shell.
 *
 * Mirrors the adminPageRegistry pattern but scoped to the customer-facing
 * product surface. Every `href` here must resolve to a real page file;
 * `test/route-integrity.test.ts` walks `app/` and fails the build if one does
 * not, so this registry cannot drift into advertising a 404 again.
 *
 * Namespacing note: game servers are **not** under `/console`. The canonical
 * customer surfaces are `/servers` (the list) and `/server/[id]/*` (20 detail
 * tabs, each a real page with its own layout). The console shell links out to
 * them rather than re-hosting them under `/console/servers/*`, which would mean
 * a second copy of every workload route.
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
  /** Marks workload routes that live outside the /console namespace. */
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
  | "database-services"
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
  { id: "database-services", labelKey: "server.databaseServices", fallback: "Managed Services", icon: Boxes, permissions: ["database.read"] },
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
  { title: "Daily", tabs: ["overview", "terminal", "files", "databases", "database-services", "schedules", "tasks", "backups"] },
  { title: "Configuration", tabs: ["startup", "network", "mounts", "users", "resource-limits", "settings"] },
  { title: "Deploy & Ops", tabs: ["deployments", "builds", "git", "processes", "activity", "transfer"] },
];

/**
 * Tab ids whose route path differs from the id. Values may contain `/`.
 *
 * The terminal tab's page is `app/server/[id]/console/page.tsx` — the directory
 * predates the "terminal" label — so the id alone would produce
 * `/server/<id>/terminal`, which has no page.
 *
 * `database-services` nests under the databases section: it used to live at
 * `/server/<id>/database`, one character away from `/server/<id>/databases` and
 * a different feature, so the sidebar offered "Database" and "Databases" as
 * sibling rows. It is now a child of Databases, and the old path redirects.
 */
const WORKLOAD_TAB_SEGMENTS: Partial<Record<WorkloadTabId, string>> = {
  terminal: "console",
  "database-services": "databases/services",
};

/** Tabs rendered one level in, under the tab named here. */
export const WORKLOAD_TAB_PARENTS: Partial<Record<WorkloadTabId, WorkloadTabId>> = {
  "database-services": "databases",
};

/**
 * Href for a workload tab.
 *
 * These resolve to `app/server/[id]/*`, which is where every one of these tabs
 * actually exists. The previous form returned `/console/servers/<id>/<tab>` —
 * a namespace with no pages in it at all, so every tab in the console sidebar
 * led to a 404.
 */
export function workloadTabHref(serverId: string, tab: WorkloadTabId): string {
  if (tab === "overview") return `/server/${serverId}`;
  return `/server/${serverId}/${WORKLOAD_TAB_SEGMENTS[tab] ?? tab}`;
}

/** The canonical customer server list, linked from the console shell. */
export const SERVERS_LIST_HREF = "/servers";

// ─── Top-level console navigation groups ─────────────────────────────────────

/**
 * The landing pages of the customer console.
 *
 * "Backups" used to sit in Operations pointing at `/console/backups`. There is
 * no such page and no aggregate backups API behind it — backups are per-server
 * (`fetchBackups(serverId)`, surfaced at `/server/[id]/backups`). Advertising a
 * platform-wide backups section implied a capability the product does not have,
 * so the entry is gone rather than pointed at a stub.
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
      { href: SERVERS_LIST_HREF, label: "Game Servers", labelKey: "console.servers", icon: Server, audience: "customer", workload: true },
      { href: "/console/apps", label: "Applications", labelKey: "console.apps", icon: Boxes, audience: "customer", workload: true },
      { href: "/console/databases", label: "Databases", labelKey: "console.databases", icon: Database, audience: "customer" },
    ],
  },
  {
    title: "Operations",
    items: [
      { href: "/console/domains", label: "Domains", labelKey: "console.domains", icon: Network, audience: "customer" },
    ],
  },
];
