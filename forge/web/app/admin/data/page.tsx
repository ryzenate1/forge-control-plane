import { redirect } from "next/navigation";

/**
 * Alias route: the old "Data" section became Databases.
 *
 * `ADMIN_ALIAS_ROUTES` maps `/admin/data → /admin/databases` for nav resolution
 * only; this page serves the old URL. The Databases frame reads `?tab=`, so
 * tab-targeting aliases land on the right tab rather than Overview.
 */
export default function DataAlias() {
  redirect("/admin/databases");
}
