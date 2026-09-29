"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { AdminLoadingState, AdminPageLayout, SectionHeader } from "@/components/admin/admin-ui";

/**
 * Legacy route — Database Services is a tab inside `/admin/databases`, not a nav
 * row (`admin-registry.ts` deliberately omits it and lists the alias in
 * `ADMIN_ALIAS_ROUTES`). The alias only resolves the *nav* position, so this
 * filesystem route is what answers the old URL and has to do the navigating.
 *
 * It used to render full product chrome while it did so: its own `<h1>`
 * "Database Services" plus a bespoke description contradicting the registry, a
 * hand-styled brand button, and a `useEffect` redirect that fired after paint —
 * so every visit flashed a second page title before jumping, and the target
 * `?tab=services` was ignored by the destination, which silently landed the
 * operator on Overview.
 *
 * Now: the header copy is the registry's (no hand-passed title, no invented
 * description), the body is a loading state rather than a page, and
 * `/admin/databases` reads `?tab=` so Services is actually reached. The redirect
 * is still `router.replace`, so the stale URL never enters history.
 */
export default function AdminDatabaseServicesRedirect() {
  const router = useRouter();

  useEffect(() => {
    router.replace("/admin/databases?tab=services");
  }, [router]);

  return (
    <AdminPageLayout>
      <SectionHeader />
      <AdminLoadingState label="Opening Database Services inside Databases…" />
    </AdminPageLayout>
  );
}
