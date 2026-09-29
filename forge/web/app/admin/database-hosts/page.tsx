import { redirect } from "next/navigation";

/**
 * Alias route: Database Hosts now lives inside the Databases tabs.
 *
 * `ADMIN_ALIAS_ROUTES` maps `/admin/database-hosts → /admin/databases` for nav
 * resolution only; this page serves the old URL. The Databases frame parses
 * `?tab=`, so the forward lands on the Hosts tab instead of Overview.
 */
export default function DatabaseHostsAlias() {
  redirect("/admin/databases?tab=hosts");
}
