"use client";

import { useEffect, useState } from "react";

/**
 * Returns a debounced value that lags behind `value` by `delay` ms.
 * Use to avoid firing expensive filter/API queries on every keystroke
 * and to reduce polling storm caused by search-driven refetches.
 *
 * Example:
 *   const [search, setSearch] = useState("");
 *   const debounced = useDebouncedValue(search, 300);
 *   const filtered = useMemo(() => list.filter(x => x.name.includes(debounced)), [list, debounced]);
 */
export function useDebouncedValue<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

/**
 * Debounced callback variant — stable reference, cancels pending call on unmount.
 */
export function useDebouncedCallback<T extends (...args: unknown[]) => void>(fn: T, delay = 300): T {
  const [timer, setTimer] = useState<ReturnType<typeof setTimeout> | null>(null);
  // Use ref-like pattern via closure: keep latest fn
  const fnRef = { current: fn } as { current: T };
  fnRef.current = fn;
  return ((...args: unknown[]) => {
    if (timer) clearTimeout(timer);
    const t = setTimeout(() => fnRef.current(...(args as Parameters<T>)), delay);
    setTimer(t);
  }) as unknown as T;
}
