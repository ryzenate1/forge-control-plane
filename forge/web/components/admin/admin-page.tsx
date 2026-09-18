"use client";

/**
 * The canonical admin page frame.
 *
 * Every admin route renders through this. Its job is to make page language a
 * property of the *route* rather than of whichever call site happened to write
 * the header, because the admin registry already holds the canonical label and
 * description for all of them — the sidebar, the command palette and the
 * breadcrumb trail have been reading them for a while, while page headers
 * restated them by hand and drifted.
 *
 * Consequences of deriving instead of restating:
 *   - The sidebar row, the breadcrumb tail and the `<h1>` cannot disagree.
 *   - A page cannot ship with no description, or with one that contradicts the
 *     nav entry the operator clicked to get there.
 *   - Renaming a section is one registry edit, not a grep.
 *
 * Overrides exist for the cases where the route genuinely does not know its own
 * title: detail routes (`/admin/nodes/<id>`) pass the resource name, and wizards
 * pass a step title. Passing `title` on a *list* route is a smell — change the
 * registry instead.
 *
 * Freshness is deliberately not rendered here. This frame has no way to know
 * whether the page's data loaded, so it will not claim that it did; a page with
 * live sources passes `status={<FreshnessBadge state={…} />}`, which is derived
 * from a real `dataUpdatedAt`. An earlier version of the admin frame rendered a
 * hardcoded "Live · updated just now" badge on every page, including pages
 * whose queries had failed.
 */

import * as React from "react";
import { usePathname } from "next/navigation";

import {
  ForgePage,
  ForgePageHeader,
  type ForgeCrumb,
  type PageWidth,
} from "@/components/ui/forge";
import {
  adminBreadcrumbTrail,
  findAdminPage,
  findAdminPageGroup,
} from "./admin-registry";

export type AdminPageProps = {
  /**
   * Overrides the registry label. Use for detail routes and wizard steps, where
   * the title is a property of the resource rather than of the route.
   */
  title?: React.ReactNode;
  /** Overrides the registry description. One sentence: what the page is for. */
  description?: React.ReactNode;
  /**
   * Small uppercase label above the title. Defaults to the registry group
   * ("Infrastructure", "Deploy", …) so a page always states where it sits.
   */
  eyebrow?: React.ReactNode;
  /** Explicit trail. Defaults to the registry trail for the current pathname. */
  breadcrumbs?: readonly ForgeCrumb[];
  /**
   * Freshness or live-state indicator beside the title. Pass a `FreshnessBadge`
   * bound to a real query state — never a static "live" label.
   */
  status?: React.ReactNode;
  /** Primary and secondary actions, right-aligned in the header. */
  actions?: React.ReactNode;
  /** Back affordance above the breadcrumbs, for detail and wizard routes. */
  back?: React.ReactNode;
  width?: PageWidth;
  className?: string;
  children: React.ReactNode;
};

export function AdminPage({
  title,
  description,
  eyebrow,
  breadcrumbs,
  status,
  actions,
  back,
  width = "wide",
  className,
  children,
}: AdminPageProps) {
  const pathname = usePathname() ?? "";

  const entry = React.useMemo(() => findAdminPage(pathname), [pathname]);
  const group = React.useMemo(() => findAdminPageGroup(pathname), [pathname]);
  const trail = React.useMemo<readonly ForgeCrumb[]>(
    () => breadcrumbs ?? adminBreadcrumbTrail(pathname),
    [breadcrumbs, pathname],
  );

  // An unregistered route still renders, but it gets no invented title: the
  // fallback is the last URL segment via the registry's own trail, and the
  // missing registry entry is the thing to fix.
  const resolvedTitle = title ?? entry?.label ?? trail.at(-1)?.label ?? "Admin";
  const resolvedDescription = description ?? entry?.description;
  const resolvedEyebrow = eyebrow ?? group?.groupTitle;

  return (
    <ForgePage className={className} width={width}>
      <ForgePageHeader
        actions={actions}
        back={back}
        breadcrumbs={trail}
        description={resolvedDescription}
        eyebrow={resolvedEyebrow}
        meta={status}
        title={resolvedTitle}
      />
      {children}
    </ForgePage>
  );
}
