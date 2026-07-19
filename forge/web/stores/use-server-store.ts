"use client";

import { create } from "zustand";

export type ServerTab = "console" | "files" | "databases" | "schedules" | "users" | "backups" | "network" | "startup" | "settings" | "activity" | "mounts";

type ServerStats = {
  cpuPercent: number;
  memoryBytes: number;
  memoryLimit: number;
  diskBytes: number;
};

interface ServerStoreState {
  currentUser: { id: string; email: string; role: string } | null;

  // Navigation
  mode: "server" | "admin";
  activeTab: ServerTab;
  adminTab: string;

  // Server selection
  selectedServerId: string | null;

  // Console
  consoleLines: string[];
  consoleStatus: string;

  // Stats
  liveStats: ServerStats | null;
  cpuHistory: number[];
  memoryHistory: number[];

  // Actions
  setCurrentUser: (user: { id: string; email: string; role: string } | null) => void;
  setMode: (mode: "server" | "admin") => void;
  setActiveTab: (tab: ServerTab) => void;
  setAdminTab: (tab: string) => void;
  setSelectedServerId: (id: string | null) => void;
  addConsoleLine: (line: string) => void;
  addConsoleLines: (lines: string[]) => void;
  clearConsole: () => void;
  setConsoleStatus: (status: string) => void;
  updateStats: (stats: ServerStats) => void;
  reset: () => void;
}

export const useServerStore = create<ServerStoreState>()((set) => ({
  currentUser: null,
  mode: "server" as const,
  activeTab: "console" as ServerTab,
  adminTab: "overview",
  selectedServerId: null,
  consoleLines: [] as string[],
  consoleStatus: "Disconnected",
  liveStats: null,
  cpuHistory: Array(24).fill(0) as number[],
  memoryHistory: Array(24).fill(0) as number[],

  setCurrentUser: (currentUser) => set({ currentUser }),
  setMode: (mode) => set({ mode }),
  setActiveTab: (activeTab) => set({ activeTab }),
  setAdminTab: (adminTab) => set({ adminTab }),
  setSelectedServerId: (selectedServerId) => set({
    selectedServerId,
    consoleLines: [],
    consoleStatus: "Connecting",
    activeTab: "console" as ServerTab,
  }),

  addConsoleLine: (line: string) => set((state) => ({
    consoleLines: [...state.consoleLines.slice(-300), line],
  })),
  addConsoleLines: (lines: string[]) => set((state) => ({
    consoleLines: [...state.consoleLines, ...lines].slice(-300),
  })),
  clearConsole: () => set({ consoleLines: [] }),
  setConsoleStatus: (consoleStatus: string) => set({ consoleStatus }),

  updateStats: (stats: ServerStats) => set((state) => {
    const memPct = stats.memoryLimit > 0
      ? Math.min(100, Math.round((stats.memoryBytes / stats.memoryLimit) * 100))
      : 0;
    const cpuPct = Math.min(300, stats.cpuPercent);
    return {
      liveStats: stats,
      cpuHistory: [...state.cpuHistory.slice(-23), cpuPct],
      memoryHistory: [...state.memoryHistory.slice(-23), memPct],
    };
  }),

  reset: () => set({
    currentUser: null,
    mode: "server" as const,
    activeTab: "console" as ServerTab,
    selectedServerId: null,
    consoleLines: [],
    consoleStatus: "Disconnected",
    liveStats: null,
    cpuHistory: Array(24).fill(0),
    memoryHistory: Array(24).fill(0),
  }),
}));
