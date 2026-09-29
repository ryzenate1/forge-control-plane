import { redirect } from "next/navigation";

/**
 * Alias route: "Game Servers" was renamed to the Servers surface.
 *
 * `ADMIN_ALIAS_ROUTES` maps `/admin/game-servers → /admin/servers` for nav
 * resolution only; this page is what actually serves the old URL so bookmarks
 * and deep links do not 404 behind a correctly-highlighted nav entry.
 */
export default function GameServersAlias() {
  redirect("/admin/servers");
}
