/**
 * Canonical permission helper for the Forge panel.
 *
 * Single source of truth for "can this user do X?" checks across the web app
 * (server views, shared gates, admin surfaces). All other implementations
 * (hasServerPermission, states-permission hasPermission) delegate here so
 * semantics stay uniform.
 *
 * Semantics (fail closed):
 * - owner / admin roles always bypass (root "*" permission also bypasses).
 * - An unverified access object (permissions === null / undefined) is DENIED.
 * - An EMPTY required permission list DENIES. Callers must pass the concrete
 *   permission key(s) an action needs; `permissions: []` is never "everything".
 * - An array is treated as "any of" (OR).
 */
export type AccessLike = {
  isAdmin?: boolean;
  isOwner?: boolean;
  permissions?: string[] | null;
};

export const ROOT_PERMISSION = "*";

/** Default access for an unverified / anonymous context: everything denied. */
export const NO_ACCESS: AccessLike = Object.freeze({
  isAdmin: false,
  isOwner: false,
  permissions: null,
});

export function can(access: AccessLike | null | undefined, permission: string | string[]): boolean {
  if (!access) return false;
  if (access.isAdmin || access.isOwner) return true;
  if (!access.permissions) return false;
  if (access.permissions.includes(ROOT_PERMISSION)) return true;
  const required = Array.isArray(permission) ? permission : [permission];
  if (required.length === 0) return false;
  return required.some((item) => access.permissions!.includes(item));
}
