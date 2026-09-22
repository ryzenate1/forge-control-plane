import { redirect } from "next/navigation";

/**
 * Alias route: the "Deploy" group name became the Deployments release list.
 *
 * `ADMIN_ALIAS_ROUTES` maps `/admin/deploy → /admin/deployments` for nav and
 * breadcrumb resolution only, so the alias *looked* like it worked while any
 * bookmark or docs link to `/admin/deploy` 404ed. `redirect` (307) rather than
 * `permanentRedirect`, matching the other Deploy-group aliases: a 301/308 is
 * cached by the browser, so the target could never be repointed without a hard
 * reload, and it renders nothing at all for the duration of the hop.
 */
export default function DeployAlias() {
  redirect("/admin/deployments");
}
