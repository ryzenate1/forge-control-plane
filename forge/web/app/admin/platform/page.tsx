import { redirect } from "next/navigation";

/**
 * Alias route: legacy group deep-link. Global Forge configuration lives at
 * `/admin/settings`. Kept so an old bookmark resolves rather than 404-ing.
 */
export default function PlatformAlias() {
  redirect("/admin/settings");
}
