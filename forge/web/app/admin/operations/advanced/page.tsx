import { redirect } from "next/navigation";

/**
 * Alias route: legacy deep-link into the operations history. The control-plane
 * operation history and manual controls live at `/admin/operations`. Kept so
 * an old bookmark resolves rather than 404-ing.
 */
export default function OperationsAdvancedAlias() {
  redirect("/admin/operations");
}
