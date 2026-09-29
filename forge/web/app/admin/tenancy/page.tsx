import { redirect } from "next/navigation";

/**
 * Alias route: the tenancy root of the hierarchy is `/admin/organizations`.
 *
 * `ADMIN_ALIAS_ROUTES` lists `/admin/tenancy`, and the nav layer resolves it —
 * so the sidebar highlighted Organizations while the URL itself 404ed. This stub
 * is the only thing that makes the bookmark actually load.
 *
 * `redirect` (307) rather than `permanentRedirect`, matching
 * `app/admin/containers/page.tsx`: a permanent redirect is cached by the browser
 * and could never be repointed without a hard reload.
 */
export default function TenancyAlias() {
  redirect("/admin/organizations");
}
