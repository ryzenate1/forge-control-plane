"use client";

/**
 * Page scaffolding — the outer shape every Forge page shares.
 *
 * One page width, one header rhythm, one section rhythm. If a page needs a
 * different gutter or a bigger title, that is a change here, not a local
 * override at the call site.
 */

import * as React from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

/* -------------------------------------------------------------------------- */
/* Page                                                                       */
/* -------------------------------------------------------------------------- */

export type PageWidth = "default" | "wide" | "narrow" | "full";

const pageWidths: Record<PageWidth, string> = {
  narrow: "max-w-3xl",
  default: "max-w-[1280px]",
  wide: "max-w-page",
  full: "max-w-none",
};

export function ForgePage({
  width = "wide",
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLDivElement> & { width?: PageWidth }) {
  return (
    <div
      className={cn("mx-auto w-full space-y-5 px-4 py-6 sm:px-6", pageWidths[width], className)}
      {...rest}
    >
      {children}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Breadcrumbs                                                                */
/* -------------------------------------------------------------------------- */

export type ForgeCrumb = { label: string; href?: string };

export function ForgeBreadcrumbs({
  items,
  className,
}: {
  items: readonly ForgeCrumb[];
  className?: string;
}) {
  if (items.length === 0) return null;
  return (
    <nav aria-label="Breadcrumb" className={cn("min-w-0", className)}>
      <ol className="flex flex-wrap items-center gap-1 text-meta text-text-muted">
        {items.map((crumb, index) => {
          const isLast = index === items.length - 1;
          return (
            <li className="flex min-w-0 items-center gap-1" key={`${crumb.label}-${index}`}>
              {index > 0 ? (
                <ChevronRight aria-hidden="true" className="size-3 shrink-0 opacity-60" />
              ) : null}
              {crumb.href && !isLast ? (
                <Link
                  className="truncate rounded-sm transition-colors hover:text-text"
                  href={crumb.href}
                >
                  {crumb.label}
                </Link>
              ) : (
                <span aria-current={isLast ? "page" : undefined} className="truncate text-text-subtle">
                  {crumb.label}
                </span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

/* -------------------------------------------------------------------------- */
/* Page header                                                                */
/* -------------------------------------------------------------------------- */

export type ForgePageHeaderProps = {
  title: React.ReactNode;
  /** One sentence. What this page is for, not what it contains. */
  description?: React.ReactNode;
  /** Small uppercase label above the title — section or category. */
  eyebrow?: React.ReactNode;
  breadcrumbs?: readonly ForgeCrumb[];
  /** Status chip or live indicator rendered beside the title. */
  meta?: React.ReactNode;
  /** Primary and secondary actions, right-aligned. */
  actions?: React.ReactNode;
  /** Back affordance rendered above the breadcrumbs. */
  back?: React.ReactNode;
  className?: string;
};

export function ForgePageHeader({
  title,
  description,
  eyebrow,
  breadcrumbs,
  meta,
  actions,
  back,
  className,
}: ForgePageHeaderProps) {
  return (
    <header className={cn("flex flex-col gap-3", className)}>
      {back ? <div className="flex items-center">{back}</div> : null}
      {breadcrumbs && breadcrumbs.length > 0 ? <ForgeBreadcrumbs items={breadcrumbs} /> : null}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          {eyebrow ? <p className="t-eyebrow">{eyebrow}</p> : null}
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="t-page min-w-0 break-words">{title}</h1>
            {meta}
          </div>
          {description ? (
            <p className="max-w-prose text-xs text-text-subtle">{description}</p>
          ) : null}
        </div>
        {actions ? (
          <div className="flex shrink-0 flex-wrap items-center gap-2 sm:justify-end">{actions}</div>
        ) : null}
      </div>
    </header>
  );
}

/* -------------------------------------------------------------------------- */
/* Section                                                                    */
/* -------------------------------------------------------------------------- */

export type ForgeSectionProps = {
  title?: React.ReactNode;
  description?: React.ReactNode;
  eyebrow?: React.ReactNode;
  actions?: React.ReactNode;
  /** Renders a hairline under the heading row. Use on long, scannable pages. */
  divided?: boolean;
  className?: string;
  bodyClassName?: string;
  children?: React.ReactNode;
  id?: string;
};

export function ForgeSection({
  title,
  description,
  eyebrow,
  actions,
  divided = false,
  className,
  bodyClassName,
  children,
  id,
}: ForgeSectionProps) {
  const hasHeading = Boolean(title || description || eyebrow || actions);
  return (
    <section className={cn("space-y-3", className)} id={id}>
      {hasHeading ? (
        <div
          className={cn(
            "flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between",
            divided && "border-b border-line pb-2.5"
          )}
        >
          <div className="min-w-0 space-y-0.5">
            {eyebrow ? <p className="t-eyebrow">{eyebrow}</p> : null}
            {title ? <h2 className="t-title">{title}</h2> : null}
            {description ? (
              <p className="max-w-prose text-meta text-text-subtle">{description}</p>
            ) : null}
          </div>
          {actions ? (
            <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>
          ) : null}
        </div>
      ) : null}
      {children ? <div className={bodyClassName}>{children}</div> : null}
    </section>
  );
}

/* -------------------------------------------------------------------------- */
/* Toolbar / separator                                                        */
/* -------------------------------------------------------------------------- */

/** Filter + search + action strip that sits above a table or grid. */
export function ForgeToolbar({
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("ui-toolbar", className)} {...rest}>
      {children}
    </div>
  );
}

/** Pushes whatever follows it to the far end of a toolbar or header row. */
export function ForgeSpacer() {
  return <div aria-hidden="true" className="hidden flex-1 sm:block" />;
}

export function ForgeSeparator({
  orientation = "horizontal",
  className,
}: {
  orientation?: "horizontal" | "vertical";
  className?: string;
}) {
  return (
    <div
      aria-orientation={orientation}
      className={cn(
        orientation === "horizontal" ? "ui-separator" : "h-full w-px shrink-0 bg-line",
        className
      )}
      role="separator"
    />
  );
}

/** Responsive grid for stat tiles and panel clusters. */
export function ForgeGrid({
  cols = 3,
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLDivElement> & { cols?: 2 | 3 | 4 | 6 }) {
  const colClass = {
    2: "sm:grid-cols-2",
    3: "sm:grid-cols-2 lg:grid-cols-3",
    4: "sm:grid-cols-2 lg:grid-cols-4",
    6: "grid-cols-2 sm:grid-cols-3 lg:grid-cols-6",
  }[cols];
  return (
    <div className={cn("grid grid-cols-1 gap-3", colClass, className)} {...rest}>
      {children}
    </div>
  );
}
