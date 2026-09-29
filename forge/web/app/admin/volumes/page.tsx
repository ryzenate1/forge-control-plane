import { redirect } from "next/navigation";

/**
 * Vocabulary alias. "Volumes" is ambiguous in this product — mount definitions
 * live here, Docker volumes under /admin/docker — so the alias lands on mounts
 * and the target stays reachable. 307 so it can be repointed.
 */
export default function VolumesAlias() {
  redirect("/admin/mounts");
}
