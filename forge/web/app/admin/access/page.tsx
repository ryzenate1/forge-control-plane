import { redirect } from "next/navigation";

/**
 * Alias route: the Access group's accounts screen is `/admin/users`.
 *
 * `ADMIN_ALIAS_ROUTES` resolves `/admin/access` for the sidebar and the
 * breadcrumb, which made the entry look mounted while nothing served the URL —
 * a bookmarked operator got a 404 behind a correctly-highlighted nav row.
 *
 * `redirect` (307) rather than `permanentRedirect`: a permanent redirect is
 * cached by the browser, so the alias could never be repointed without a hard
 * reload, and it renders nothing for a paint. Next still shows the nearest
 * `loading.tsx` for the hop. Same shape as `app/admin/containers/page.tsx`.
 */
export default function AccessAlias() {
  redirect("/admin/users");
}
