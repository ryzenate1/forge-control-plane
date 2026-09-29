import { redirect } from "next/navigation";

/**
 * Legacy group deep-link → the Endpoints inventory.
 *
 * Networking has no single page of its own, so this alias keeps bookmarks and
 * docs links resolving instead of 404ing. ADMIN_ALIAS_ROUTES declares the
 * mapping, but that map only affects navigation resolution — an alias source is
 * reachable in the browser only if a page exists at it.
 *
 * `redirect` (307) rather than `permanentRedirect`: a 301/308 is cached by the
 * browser, so the alias could never be repointed without a hard reload, and it
 * renders nothing for a paint. Next still shows the nearest `loading.tsx`.
 */
export default function NetworkingAlias() {
  redirect("/admin/endpoints");
}
