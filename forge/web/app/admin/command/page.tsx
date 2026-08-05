import { redirect } from "next/navigation";

/**
 * Alias route: legacy group deep-link. The live control-plane summary lives
 * at `/admin/overview`. Kept so an old bookmark resolves rather than 404-ing.
 */
export default function CommandAlias() {
  redirect("/admin/overview");
}
