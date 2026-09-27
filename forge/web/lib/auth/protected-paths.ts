/**
 * Shared protected-path list for the App Router session guard.
 *
 * Imported by both `middleware.ts` (edge) and `components/providers.tsx`
 * (client) so the two session gates can never disagree about which routes
 * require authentication. Keep this file free of client-only imports — it
 * runs in the middleware edge runtime.
 */
export const PROTECTED_PATH_PREFIXES = [
  "/servers",
  "/server",
  "/account",
  "/admin",
  "/organizations",
  "/console",
] as const;

export function isProtectedPath(pathname: string): boolean {
  return PROTECTED_PATH_PREFIXES.some(
    (prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`),
  );
}
