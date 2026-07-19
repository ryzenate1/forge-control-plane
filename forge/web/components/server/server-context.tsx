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

export function hasServerPermission(access: ServerAccess, permission: string | string[]) {
  if (access.isAdmin || access.isOwner) return true;
  if (!access.permissions) return false;
  if (access.permissions.includes("*")) return true;
  const required = Array.isArray(permission) ? permission : [permission];
  return required.some((item) => access.permissions!.includes(item));
}
