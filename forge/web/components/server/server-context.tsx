"use client";

import { createContext, useContext } from "react";
import type { ApiServer, ApiUser } from "@/lib/api";

export type ServerAccess = {
  user: ApiUser | null;
  permissions: string[] | null;
  isOwner: boolean;
  isAdmin: boolean;
};

type ServerContextValue = {
  server: ApiServer;
  access: ServerAccess;
  refreshServer: () => Promise<void>;
};

const Context = createContext<ServerContextValue | null>(null);

export function ServerProvider({ value, children }: { value: ServerContextValue; children: React.ReactNode }) {
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useServerContext() {
  const value = useContext(Context);
  if (!value) throw new Error("useServerContext must be used inside ServerProvider");
  return value;
}

export function useOptionalServerContext() {
  return useContext(Context);
}

import { can } from "@/lib/permissions";

export function hasServerPermission(access: ServerAccess, permission: string | string[]) {
  const required = Array.isArray(permission) ? permission : [permission];
  // An empty required list means "any authenticated server member": tabs that
  // have no backend permission gate (e.g. builds, deployments) stay visible to
  // every verified member while remaining hidden from unverified contexts.
  if (required.length === 0) {
    return access.isAdmin || access.isOwner || access.permissions !== null;
  }
  return can(access, permission);
}
