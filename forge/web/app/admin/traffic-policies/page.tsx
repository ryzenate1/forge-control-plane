import { redirect } from "next/navigation";

/**
 * Legacy plural form of the Traffic Policies route (ADMIN_ALIAS_ROUTES).
 * `redirect` (307) so the alias stays repointable — see app/admin/containers.
 */
export default function TrafficPoliciesAlias() {
  redirect("/admin/traffic");
}
