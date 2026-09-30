"use client";

/**
 * Tables and list data.
 *
 * One density model with a single `compact` step, sticky headers by default,
 * and horizontal overflow handled by the wrapper rather than by each page. A
 * table that has no rows renders `ForgeTableEmpty` inside the table body so the
 * header and column widths stay put.
 */

import * as React from "react";
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight, ChevronsUpDown } from "lucide-react";
import { cn } from "@/lib/utils";

/* -------------------------------------------------------------------------- */
/* Table                                                                      */
/* -------------------------------------------------------------------------- */

export type ForgeTableDensity = "default" | "compact";

export function ForgeTable({
  density = "default",
  /** Caps the body height and keeps the header pinned while the body scrolls. */
  maxHeight,
  className,
  wrapperClassName,
  children,
  ...rest
}: React.TableHTMLAttributes<HTMLTableElement> & {
  density?: ForgeTableDensity;
  maxHeight?: number | string;
  wrapperClassName?: string;
}) {
  return (
    <div
      className={cn("w-full overflow-x-auto", maxHeight && "overflow-y-auto", wrapperClassName)}
      style={maxHeight ? { maxHeight } : undefined}
    >
      <table
        className={cn("ui-table", density === "compact" && "ui-table-compact", className)}
        {...rest}
      >
        {children}
      </table>
    </div>
  );
}

export function ForgeTHead({ children, ...rest }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <thead {...rest}>{children}</thead>;
}

export function ForgeTBody({ children, ...rest }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody {...rest}>{children}</tbody>;
}

export type ForgeSortDirection = "asc" | "desc";

export function ForgeTh({
  align = "left",
  /** Set to make the header a sort control. */
  sortable = false,
  sortDirection,
  onSort,
  numeric = false,
  className,
  children,
  ...rest
}: React.ThHTMLAttributes<HTMLTableCellElement> & {
  align?: "left" | "right" | "center";
  sortable?: boolean;
  sortDirection?: ForgeSortDirection | null;
  onSort?: () => void;
  numeric?: boolean;
}) {
  const alignClass = { left: "text-left", right: "text-right", center: "text-center" }[align];
  const SortIcon = sortDirection === "asc" ? ArrowUp : sortDirection === "desc" ? ArrowDown : ChevronsUpDown;

  return (
    <th
      aria-sort={
        sortable
          ? sortDirection === "asc"
            ? "ascending"
            : sortDirection === "desc"
              ? "descending"
              : "none"
          : undefined
      }
      className={cn("ui-th", alignClass, (numeric || align === "right") && "text-right", className)}
      scope="col"
      {...rest}
    >
      {sortable ? (
        <button
          className={cn(
            "inline-flex items-center gap-1 rounded-sm transition-colors hover:text-text",
            align === "right" && "flex-row-reverse"
          )}
          onClick={onSort}
          type="button"
        >
          {children}
          <SortIcon
            aria-hidden="true"
            className={cn("size-3", sortDirection ? "text-text" : "opacity-50")}
          />
        </button>
      ) : (
        children
      )}
    </th>
  );
}

export function ForgeTr({
  /** Makes the whole row activatable. Requires a keyboard handler too. */
  interactive = false,
  selected = false,
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLTableRowElement> & { interactive?: boolean; selected?: boolean }) {
  return (
    <tr
      className={cn(
        interactive && "ui-tr-interactive",
        selected && "bg-brand-subtle",
        className
      )}
      data-selected={selected || undefined}
      tabIndex={interactive ? 0 : undefined}
      {...rest}
    >
      {children}
    </tr>
  );
}

export function ForgeTd({
  align = "left",
  /** Right-aligns and applies tabular mono so digits line up down the column. */
  numeric = false,
  mono = false,
  /** Dims the cell — use for derived or secondary values, never for errors. */
  muted = false,
  truncate = false,
  className,
  children,
  ...rest
}: React.TdHTMLAttributes<HTMLTableCellElement> & {
  align?: "left" | "right" | "center";
  numeric?: boolean;
  mono?: boolean;
  muted?: boolean;
  truncate?: boolean;
}) {
  const alignClass = { left: "text-left", right: "text-right", center: "text-center" }[align];
  return (
    <td
      className={cn(
        "ui-td",
        alignClass,
        numeric && "text-right font-mono tabular-nums",
        mono && "font-mono text-meta",
        muted && "text-text-subtle",
        truncate && "max-w-0 truncate",
        className
      )}
      {...rest}
    >
      {children}
    </td>
  );
}

/** Full-width message row. Keeps the header and column widths in place. */
export function ForgeTableEmpty({
  colSpan,
  children,
  className,
}: {
  colSpan: number;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <tr>
      <td className={cn("px-3 py-10 text-center text-xs text-text-subtle", className)} colSpan={colSpan}>
        {children}
      </td>
    </tr>
  );
}

/* -------------------------------------------------------------------------- */
/* Pagination                                                                 */
/* -------------------------------------------------------------------------- */

export function ForgePagination({
  page,
  pageCount,
  onPageChange,
  /** Total row count, when known. Omit rather than passing 0 for "unknown". */
  total,
  className,
}: {
  page: number;
  pageCount: number;
  onPageChange: (page: number) => void;
  total?: number;
  className?: string;
}) {
  if (pageCount <= 1) return null;
  return (
    <nav
      aria-label="Pagination"
      className={cn(
        "flex items-center justify-between gap-3 border-t border-line px-3 py-2",
        className
      )}
    >
      <p className="text-meta text-text-subtle">
        Page <span className="font-mono text-text tabular-nums">{page}</span> of{" "}
        <span className="font-mono text-text tabular-nums">{pageCount}</span>
        {typeof total === "number" ? (
          <>
            {" · "}
            <span className="font-mono tabular-nums">{total.toLocaleString()}</span> total
          </>
        ) : null}
      </p>
      <div className="flex items-center gap-1">
        <button
          aria-label="Previous page"
          className="ui-button ui-button-secondary h-7 px-2"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
          type="button"
        >
          <ChevronLeft aria-hidden="true" className="size-3.5" />
          Prev
        </button>
        <button
          aria-label="Next page"
          className="ui-button ui-button-secondary h-7 px-2"
          disabled={page >= pageCount}
          onClick={() => onPageChange(page + 1)}
          type="button"
        >
          Next
          <ChevronRight aria-hidden="true" className="size-3.5" />
        </button>
      </div>
    </nav>
  );
}

/* -------------------------------------------------------------------------- */
/* Data list                                                                  */
/* -------------------------------------------------------------------------- */

/**
 * Stacked list rows. The right shape when a record has two or three fields and
 * a table would be mostly whitespace.
 */
export function ForgeDataList({
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLUListElement>) {
  return (
    <ul className={cn("divide-y divide-line", className)} {...rest}>
      {children}
    </ul>
  );
}

export function ForgeDataListRow({
  title,
  description,
  meta,
  actions,
  icon,
  className,
  ...rest
}: React.LiHTMLAttributes<HTMLLIElement> & {
  title: React.ReactNode;
  description?: React.ReactNode;
  /** Right-aligned status or timestamp, before the actions. */
  meta?: React.ReactNode;
  actions?: React.ReactNode;
  icon?: React.ReactNode;
}) {
  return (
    <li
      className={cn("flex items-center gap-3 px-3 py-2.5 transition-colors hover:bg-overlay-subtle", className)}
      {...rest}
    >
      {icon ? <span className="shrink-0 text-text-subtle">{icon}</span> : null}
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs font-semibold text-text">{title}</p>
        {description ? (
          <p className="truncate text-meta text-text-subtle">{description}</p>
        ) : null}
      </div>
      {meta ? <div className="shrink-0 text-meta text-text-subtle">{meta}</div> : null}
      {actions ? <div className="flex shrink-0 items-center gap-1">{actions}</div> : null}
    </li>
  );
}
