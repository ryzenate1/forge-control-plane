"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { findAdminPageGroup } from "./admin-registry";

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
  const pathname = usePathname() || "";
  const groupMatch = findAdminPageGroup(pathname);

  return (
    <div className="mx-auto w-full max-w-[1280px] space-y-6">
      <div className="flex items-center justify-between text-xs text-slate-400">
        <nav aria-label="Breadcrumb" className="flex items-center gap-2">
          {breadcrumbs && breadcrumbs.length > 0 ? (
            breadcrumbs.map((crumb, idx) => (
              <span key={idx} className="flex items-center gap-2">
                {idx > 0 && <span aria-hidden="true" className="text-slate-600 select-none after:content-['/']" />}
                {crumb.href ? (
                  <Link
                    href={crumb.href}
                    className="text-slate-400 hover:text-slate-200 transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] rounded"
                  >
                    {crumb.label}
                  </Link>
                ) : (
                  <span className="font-semibold text-slate-200">{crumb.label}</span>
                )}
              </span>
            ))
          ) : groupMatch ? (
            <div className="flex items-center gap-2">
              <span>{groupMatch.groupTitle}</span>
              <span aria-hidden="true" className="text-slate-600 select-none after:content-['/']" />
              <span className="font-semibold text-slate-200">{groupMatch.pageLabel}</span>
            </div>
          ) : (
            <div className="flex items-center gap-2">
              <span>Platform</span>
              <span aria-hidden="true" className="text-slate-600 select-none after:content-['/']" />
              <span className="font-semibold text-slate-200">{title}</span>
            </div>
          )}
        </nav>
        <div className="flex items-center gap-1.5 font-mono text-[11px] text-slate-500 shrink-0">
          <span className="relative flex h-1.5 w-1.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-60" />
            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-400" />
          </span>
          <span>Live · updated just now</span>
        </div>
      </div>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-[var(--line)] pb-5">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-100 flex items-center gap-2.5">
            {title}
          </h1>
          {description && (
            <p className="mt-1 text-xs sm:text-sm text-slate-400 leading-relaxed max-w-3xl">
              {description}
            </p>
          )}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2 sm:self-center">{actions}</div>}
      </div>

      {children}
    </div>
  );
}

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
    <div className={`rounded-xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[var(--shadow-card)] motion-safe:transition-colors motion-reduce:transition-none ${className ?? ""}`}>
      {(title || description || actions) && (
        <div className="flex items-start justify-between gap-4 border-b border-[var(--line)] bg-white/[0.018] -m-6 mb-4 px-6 py-4 rounded-t-xl">
          <div>
            {title && <h2 className="text-xs font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]">{title}</h2>}
            {description && <p className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">{description}</p>}
          </div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>
      )}
      {children}
    </div>
  );
}

