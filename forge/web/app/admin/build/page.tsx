import { redirect } from "next/navigation";

/**
 * Alias route: "Build" vocabulary became the Service Catalog.
 *
 * `ADMIN_ALIAS_ROUTES` maps `/admin/build → /admin/catalog` for nav resolution
 * only; this page serves the old URL so bookmarks do not 404.
 */
export default function BuildAlias() {
  redirect("/admin/catalog");
}
