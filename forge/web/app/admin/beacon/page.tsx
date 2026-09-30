import { redirect } from "next/navigation";

/**
 * Alias route: vocabulary normalization (Node ↔ Beacon). The fleet inventory
 * lives at `/admin/nodes`. Kept so an old bookmark resolves rather than
 * 404-ing.
 */
export default function BeaconAlias() {
  redirect("/admin/nodes");
}
