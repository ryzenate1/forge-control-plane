/**
 * @deprecated Use workloadTabs / workloadTabHref from console-registry.ts directly.
 * This file exists for backward compatibility with components/server/* that still
 * import from here. It re-exports the canonical definitions.
 */
import {
  Activity, Archive, ArrowLeftRight, Box, Calendar, Cpu, Database, Folder,
  Gauge, GitBranch, HardDrive, History, Layers, LayoutDashboard, ListChecks,
  Network, Rocket, Settings, Terminal, Users, type LucideIcon,
} from "lucide-react";

export type ServerTab = "overview" | "console" | "files" | "databases" | "schedules" | "tasks" | "users" | "backups" | "builds" | "network" | "startup" | "settings" | "activity" | "mounts" | "processes" | "resource-limits" | "deployments" | "git" | "transfer" | "lifecycle";

export type ServerTabConfig = {
  id: ServerTab;
  labelKey: string;
  fallback: string;
  icon: LucideIcon;
  permissions: string[];
};

/**
 * Legacy tab list — kept in sync with console-registry workloadTabs.
 * The "console" tab is now labeled "Terminal" in user-facing text.
 */
export const serverTabs: ServerTabConfig[] = [
  { id: "overview", labelKey: "server.overview", fallback: "Overview", icon: LayoutDashboard, permissions: [] },
  { id: "console", labelKey: "server.terminal", fallback: "Terminal", icon: Terminal, permissions: ["websocket.connect"] },
  { id: "files", labelKey: "server.files", fallback: "Files", icon: Folder, permissions: ["file.read"] },
  { id: "databases", labelKey: "server.databases", fallback: "Databases", icon: Database, permissions: ["database.read"] },
  { id: "schedules", labelKey: "server.schedules", fallback: "Schedules", icon: Calendar, permissions: ["schedule.read"] },
  { id: "tasks", labelKey: "server.scheduledTasks", fallback: "Scheduled Tasks", icon: ListChecks, permissions: ["schedule.read"] },
  { id: "backups", labelKey: "server.backups", fallback: "Backups", icon: Archive, permissions: ["backup.read"] },
  { id: "startup", labelKey: "server.startup", fallback: "Startup", icon: Rocket, permissions: ["startup.read"] },
  { id: "network", labelKey: "server.network", fallback: "Network", icon: Network, permissions: ["allocation.read"] },
  { id: "mounts", labelKey: "server.mounts", fallback: "Mounts", icon: HardDrive, permissions: ["mount.read"] },
  { id: "users", labelKey: "admin.users", fallback: "Users", icon: Users, permissions: ["user.read"] },
  { id: "settings", labelKey: "server.settings", fallback: "Settings", icon: Settings, permissions: ["settings.rename", "settings.reinstall", "file.sftp"] },
  { id: "deployments", labelKey: "server.deployments", fallback: "Deployments", icon: Layers, permissions: [] },
  { id: "builds", labelKey: "server.builds", fallback: "Builds", icon: Box, permissions: [] },
  { id: "git", labelKey: "server.git", fallback: "Git", icon: GitBranch, permissions: [] },
  { id: "processes", labelKey: "server.processes", fallback: "Processes", icon: Cpu, permissions: ["control.start"] },
  { id: "resource-limits", labelKey: "server.resourceLimits", fallback: "Resources", icon: Gauge, permissions: ["settings.rename"] },
  { id: "activity", labelKey: "admin.activity", fallback: "Activity", icon: Activity, permissions: ["activity.read"] },
  { id: "lifecycle", labelKey: "server.lifecycle", fallback: "Lifecycle", icon: History, permissions: ["activity.read"] },
  { id: "transfer", labelKey: "server.transfer", fallback: "Transfer", icon: ArrowLeftRight, permissions: ["settings.rename"] },
];

export const serverTabGroups: Array<{ title: string; tabs: ServerTab[] }> = [
  { title: "Daily", tabs: ["overview", "console", "files", "databases", "schedules", "tasks", "backups"] },
  { title: "Configuration", tabs: ["startup", "network", "mounts", "users", "resource-limits", "settings"] },
  { title: "Deploy & Ops", tabs: ["deployments", "builds", "git", "processes", "activity", "lifecycle", "transfer"] },
];

/** Href for legacy /server/[id] route tree (not /console). */
export function serverTabHref(serverId: string, tab: ServerTab): string {
  return tab === "overview" ? `/server/${serverId}` : `/server/${serverId}/${tab}`;
}
