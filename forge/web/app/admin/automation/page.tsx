import { redirect } from "next/navigation";

/**
 * Alias route: vocabulary normalization. Scheduled and recurring automation
 * lives at `/admin/cron-jobs`. Kept so an old bookmark resolves rather than
 * 404-ing.
 */
export default function AutomationAlias() {
  redirect("/admin/cron-jobs");
}
