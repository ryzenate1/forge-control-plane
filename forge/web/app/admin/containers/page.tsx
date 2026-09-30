import { redirect } from "next/navigation";

/**
 * Alias route: the Containers inventory lives inside the Docker manager tabs.
 *
 * This used to be `permanentRedirect`, which the browser caches permanently —
 * the alias could then never be repointed without a hard reload — and it
 * rendered nothing at all for a paint. `redirect` emits a 307 and Next still
 * shows the nearest `loading.tsx` for the hop.
 */
export default function ContainersAlias() {
  redirect("/admin/docker");
}
