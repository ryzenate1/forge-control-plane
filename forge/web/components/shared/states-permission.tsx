"use client";

import { Lock } from "lucide-react";
import type { ReactNode } from "react";

type Access = {
  isAdmin?: boolean;
  isOwner?: boolean;
  permissions?: string[] | null;
};

function hasPermission(access: Access, required: string | string[]): boolean {
  if (access.isAdmin || access.isOwner) return true;
  if (!access.permissions) return false;
  if (access.permissions.includes("*")) return true;
  const list = Array.isArray(required) ? required : [required];
  return list.some((item) => access.permissions!.includes(item));
}

function LockPlaceholder({ message }: { message: string }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-white/10 bg-white/[0.02] px-6 py-10 text-center">
      <div className="grid h-10 w-10 place-items-center rounded-full bg-white/[0.05] text-slate-500">
        <Lock size={18} strokeWidth={1.5} aria-hidden="true" />
      </div>
      <p className="mt-3 text-sm text-slate-500">{message}</p>
    </div>
  );
}

export function PermissionGate({
  access,
  permission,
  children,
  fallback,
}: {
  access: Access | null;
  permission?: string | string[];
  children: ReactNode;
  fallback?: ReactNode;
}) {
  if (!access) return null;
  if (permission && !hasPermission(access, permission)) {
    return (
      fallback ?? <LockPlaceholder message="You do not have permission to view this content." />
    ) as React.ReactElement;
  }
  return children as React.ReactElement;
}

export function RoleGate({
  roles,
  currentRole,
  children,
  fallback,
}: {
  roles: string[];
  currentRole?: string | null;
  children: ReactNode;
  fallback?: ReactNode;
}) {
  if (!currentRole || !roles.includes(currentRole)) {
    return (
      fallback ?? <LockPlaceholder message={`This content is restricted to ${roles.join(" or ")} roles.`} />
    ) as React.ReactElement;
  }
  return children as React.ReactElement;
}

export function ScopeGate({
  requiredScope,
  currentScopes,
  children,
  fallback,
}: {
  requiredScope: string | string[];
  currentScopes?: string[] | null;
  children: ReactNode;
  fallback?: ReactNode;
}) {
  const list = Array.isArray(requiredScope) ? requiredScope : [requiredScope];
  const hasScope = currentScopes?.some((s) => list.includes(s)) ?? false;
  if (!hasScope) {
    return (
      fallback ?? <LockPlaceholder message="This content requires additional permissions." />
    ) as React.ReactElement;
  }
  return children as React.ReactElement;
}
