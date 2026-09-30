import { redirect } from "next/navigation";

/**
 * Legacy alias: `/admin/network` pointed at the domain list before the group
 * was named Networking. Declared in ADMIN_ALIAS_ROUTES; without this page the
 * URL 404s even though the sidebar resolves it.
 *
 * `redirect` (307) so the alias stays repointable — see app/admin/containers.
 */
export default function NetworkAlias() {
  redirect("/admin/domains");
}
