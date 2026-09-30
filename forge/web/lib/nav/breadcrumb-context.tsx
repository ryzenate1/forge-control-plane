"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";

/**
 * Page-supplied labels for the shell's breadcrumb trail.
 *
 * The registry can name every static route, but not a resource: for
 * `/admin/apps/8f1c…/compose` it can only humanise the id segment. Detail pages
 * used to solve that by rendering a *second* breadcrumb of their own inside the
 * page header, so every such screen showed two trails a few pixels apart that
 * disagreed with each other — the shell's `Apps / 8f1c… / Compose` above the
 * page's `Build / Apps / My API / Compose`.
 *
 * Now there is one trail. A detail page calls `useBreadcrumbLabel(id, name)`
 * and the shell substitutes the real name in the crumb for that segment. A page
 * whose resource has not loaded passes `null` and the segment keeps its
 * humanised form rather than flickering through a placeholder.
 */
type Overrides = Readonly<Record<string, string>>;

type BreadcrumbContextValue = {
  overrides: Overrides;
  /** Register a label for a path segment. Returns nothing; unset on unmount. */
  setOverride: (segment: string, label: string | null) => void;
};

const BreadcrumbContext = createContext<BreadcrumbContextValue | null>(null);

export function BreadcrumbProvider({ children }: { children: ReactNode }) {
  const [overrides, setOverrides] = useState<Overrides>({});

  const setOverride = useCallback((segment: string, label: string | null) => {
    setOverrides((current) => {
      if (label === null) {
        if (!(segment in current)) return current;
        const next = { ...current };
        delete next[segment];
        return next;
      }
      if (current[segment] === label) return current;
      return { ...current, [segment]: label };
    });
  }, []);

  const value = useMemo(() => ({ overrides, setOverride }), [overrides, setOverride]);
  return <BreadcrumbContext.Provider value={value}>{children}</BreadcrumbContext.Provider>;
}

/** The registered overrides. Empty outside a provider, never throws. */
export function useBreadcrumbOverrides(): Overrides {
  return useContext(BreadcrumbContext)?.overrides ?? {};
}

/**
 * Name the crumb for `segment` while this component is mounted.
 *
 * Both arguments may be null while data loads — that registers nothing, so the
 * trail shows the raw segment rather than "undefined" or an empty crumb.
 */
export function useBreadcrumbLabel(segment: string | null | undefined, label: string | null | undefined) {
  const context = useContext(BreadcrumbContext);
  const setOverride = context?.setOverride;

  useEffect(() => {
    if (!setOverride || !segment || !label) return;
    setOverride(segment, label);
    return () => setOverride(segment, null);
  }, [setOverride, segment, label]);
}

/** Apply overrides to a trail, matching on the crumb's own label. */
export function applyBreadcrumbOverrides<T extends { label: string }>(crumbs: T[], overrides: Overrides): T[] {
  if (Object.keys(overrides).length === 0) return crumbs;
  return crumbs.map((crumb) => {
    const override = overrides[crumb.label];
    return override ? { ...crumb, label: override } : crumb;
  });
}
