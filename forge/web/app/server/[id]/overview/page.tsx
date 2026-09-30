import { permanentRedirect } from "next/navigation";

/**
 * Compat alias for the duplicate overview route.
 *
 * `/server/<id>` and `/server/<id>/overview` rendered the same `OverviewView`
 * through the same layout — two URLs for one page, which meant the sidebar's
 * Overview row highlighted on one of them and not the other depending on how
 * the user arrived. `/server/<id>` is the canonical landing page.
 */
export default async function ServerOverviewAlias({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<never> {
  const { id } = await params;
  return permanentRedirect(`/server/${id}`);
}
