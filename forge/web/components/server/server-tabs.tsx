import { Activity, Archive, ArrowLeftRight, Box, Calendar, Cpu, Database, Folder, GitBranch, HardDrive, History, Layers, LayoutDashboard, Network, Rocket, Settings, Terminal, Users, type LucideIcon } from "lucide-react";

export type ServerTab = "overview" | "console" | "files" | "databases" | "schedules" | "users" | "backups" | "builds" | "network" | "startup" | "settings" | "activity" | "mounts" | "processes" | "deployments" | "git" | "transfer" | "lifecycle";

export type ServerTabConfig = {
  id: ServerTab;
  labelKey: string;
  fallback: string;
  icon: LucideIcon;
  permissions: string[];
};

/**
 * Daily-use tabs first (console, files, databases, schedules, backups),
 * then configuration (startup, network, mounts, users, settings),
 * then operations & deployment (deployments, builds, git, processes, activity, lifecycle, transfer).
 * Icons are unique per tab to aid scanning.
 */
export const serverTabs: ServerTabConfig[] = [
  { id: "overview", labelKey: "server.overview", fallback: "Overview", icon: LayoutDashboard, permissions: [] },
  { id: "console", labelKey: "server.console", fallback: "Console", icon: Terminal, permissions: ["websocket.connect"] },
  { id: "files", labelKey: "server.files", fallback: "Files", icon: Folder, permissions: ["file.read"] },
  { id: "databases", labelKey: "server.databases", fallback: "Databases", icon: Database, permissions: ["database.read"] },
  { id: "schedules", labelKey: "server.schedules", fallback: "Schedules", icon: Calendar, permissions: ["schedule.read"] },
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
  { id: "activity", labelKey: "admin.activity", fallback: "Activity", icon: Activity, permissions: ["activity.read"] },
  { id: "lifecycle", labelKey: "server.lifecycle", fallback: "Lifecycle", icon: History, permissions: ["activity.read"] },
  { id: "transfer", labelKey: "server.transfer", fallback: "Transfer", icon: ArrowLeftRight, permissions: ["settings.rename"] },
];

export const serverTabGroups: Array<{ title: string; tabs: ServerTab[] }> = [
  { title: "Daily", tabs: ["overview", "console", "files", "databases", "schedules", "backups"] },
  { title: "Configuration", tabs: ["startup", "network", "mounts", "users", "settings"] },
  { title: "Deploy & Ops", tabs: ["deployments", "builds", "git", "processes", "activity", "lifecycle", "transfer"] },
];

export function serverTabHref(serverId: string, tab: ServerTab): string {
  return tab === "console" ? `/console/servers/${serverId}` : `/console/servers/${serverId}/${tab}`;
}