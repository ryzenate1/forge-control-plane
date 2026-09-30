import { redirect } from "next/navigation";

/**
 * Alias route: the old "Workloads" group name became Game Servers.
 *
 * `ADMIN_ALIAS_ROUTES` maps `/admin/workloads → /admin/servers` for nav
 * resolution only — the sidebar highlights the right row even when nothing
 * serves the URL. This page is what actually answers old bookmarks and deep
 * links. `redirect` emits a 307 so the alias can be repointed later without a
 * permanently cached hop.
 */
export default function WorkloadsAlias() {
  redirect("/admin/servers");
}
