"use client";

import Link from "next/link";
import type { ReactNode } from "react";
import { AdminPageLayout as AdminFrame, Card, SectionHeader, cn } from "./admin-ui";

/**
 * Compatibility shim over the canonical admin page frame.
 *
 * There used to be two `AdminPageLayout` components: this one (max-width
 * 1280px, its own breadcrumb renderer, raw `text-slate-*` colours and a
 * hardcoded "Live · updated just now" badge) and the one in `admin-ui.tsx`
 * (max-width 1600px, tokenised, breadcrumbs from the admin registry). Pages
 * sat at two different content widths and showed two different breadcrumb
 * trails depending on which import they happened to use, and the badge claimed
 * freshness for data that may never have loaded.
 *
 * This file now delegates so there is one frame, one breadcrumb source and one
 * width. New screens should import from `admin-ui` directly.
 */
export type Breadcrumb = { label: string; href?: string };

export function AdminPageLayout({
  title,
  description,
  breadcrumbs,
  actions,
  children,
}: {
  title: string;
  description?: string;
  breadcrumbs?: Breadcrumb[];
  actions?: ReactNode;
  children: ReactNode;
}) {
  // An explicit trail wins; otherwise `SectionHeader` derives one from the
  // admin registry, which is the same trail the sidebar and shell show.
  const trail =
    breadcrumbs && breadcrumbs.length > 0 ? (
      <div className="flex items-center gap-2">
        {breadcrumbs.map((crumb, index) => {
          const isLast = index === breadcrumbs.length - 1;
          return (
            <span key={`${crumb.label}-${index}`} className="flex items-center gap-2">
              {index > 0 ? (
                <span aria-hidden="true" className="select-none text-text-muted after:content-['/']" />
              ) : null}
              {crumb.href && !isLast ? (
                <Link
                  href={crumb.href}
                  className="rounded text-text-muted transition hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                >
                  {crumb.label}
                </Link>
              ) : (
                <span
                  aria-current={isLast ? "page" : undefined}
                  className={isLast ? "font-semibold text-text" : "text-text-muted"}
                >
                  {crumb.label}
                </span>
              )}
            </span>
          );
        })}
      </div>
    ) : undefined;

  return (
    <AdminFrame>
      <SectionHeader title={title} sub={description} action={actions} breadcrumb={trail} />
      {children}
    </AdminFrame>
  );
}

/**
 * A titled card.
 *
 * Distinct from `admin-ui`'s `AdminCard`, which is the untitled surface: this
 * one takes a title/description header, and the manager screens under
 * `/admin/*` lean on it heavily. It is built on the shared `Card` so both stay
 * on one surface treatment.
 */
export function AdminCard({
  title,
  description,
  actions,
  children,
  className,
}: {
  title?: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={className}>
      {title || description || actions ? (
        // Negative margins matching `.ui-card`'s own padding (p-4 / sm:p-5),
        // the same bleed `admin-ui`'s CardHeader uses.
        <div className="-mx-4 -mt-4 mb-4 flex items-start justify-between gap-4 rounded-t-lg border-b border-line bg-overlay-subtle px-4 py-4 sm:-mx-5 sm:-mt-5 sm:mb-5 sm:px-5">
          <div>
            {title ? (
              <h2 className="text-xs font-semibold uppercase tracking-[0.12em] text-text-subtle">{title}</h2>
            ) : null}
            {description ? <p className="mt-1 text-xs leading-5 text-text-subtle">{description}</p> : null}
          </div>
          {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      {children}
    </Card>
  );
}
