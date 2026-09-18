"use client";

import type { ReactNode } from "react";
import { RefreshCw } from "lucide-react";
import { Btn } from "./admin-ui";

/** Consistent timing, refresh, and page actions for operational dashboards. */
export function AdminPageToolbar({ range, onRefresh, refreshing, refreshLabel = "Refresh data", children }: {
  range?: { value: string; onChange: (value: string) => void; options: readonly { value: string; label: string }[] };
  onRefresh: () => void;
  refreshing: boolean;
  refreshLabel?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {range && (
        <select
          aria-label="Select time range"
          value={range.value}
          onChange={(event) => range.onChange(event.target.value)}
          className="h-8 max-w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-xs font-medium text-text outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
        >
          {range.options.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
        </select>
      )}
      <Btn tone="ghost" size="sm" ariaLabel={refreshLabel} title={refreshLabel} onClick={onRefresh} disabled={refreshing}>
        <RefreshCw size={14} aria-hidden="true" className={refreshing ? "animate-spin" : undefined} />
        Refresh
      </Btn>
      {children}
    </div>
  );
}
