import { redirect } from "next/navigation";

/**
 * Alias route: "Beacons" was a second name for the same entity the sidebar
 * calls Nodes. `ADMIN_ALIAS_ROUTES` maps this href for navigation resolution
 * only, so without a page here a stale deep link or bookmark was a 404.
 *
 * `redirect` (307) rather than `permanentRedirect`: a permanent redirect is
 * cached by the browser, so the alias could never be repointed without a hard
 * reload.
 */
export default function BeaconsAlias() {
  redirect("/admin/nodes");
}
