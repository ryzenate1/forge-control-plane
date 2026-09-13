"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { ChevronDown, Search } from "lucide-react";

export function ScopePicker({ scopes, selected, onChange, disabled, label }: {
  scopes: Record<string, string>;
  selected: string[];
  onChange: (scopes: string[]) => void;
  disabled?: boolean;
  label: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const listboxId = useId();
  const entries = useMemo(() => Object.entries(scopes), [scopes]);
  const filtered = useMemo(() => {
    if (!query.trim()) return entries;
    const q = query.toLowerCase();
    return entries.filter(([key, val]) => key.toLowerCase().includes(q) || val.toLowerCase().includes(q));
  }, [entries, query]);

  const toggle = (scope: string) => {
    onChange(selected.includes(scope) ? selected.filter((s) => s !== scope) : [...selected, scope]);
  };

  const close = () => {
    setOpen(false);
    setQuery("");
    setActiveIndex(0);
  };

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) close();
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        close();
        triggerRef.current?.focus();
      }
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  useEffect(() => setActiveIndex(0), [query]);

  const onListKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActiveIndex((i) => (filtered.length === 0 ? 0 : (i + 1) % filtered.length));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveIndex((i) => (filtered.length === 0 ? 0 : (i - 1 + filtered.length) % filtered.length));
    } else if (e.key === "Home") {
      e.preventDefault();
      setActiveIndex(0);
    } else if (e.key === "End") {
      e.preventDefault();
      setActiveIndex(Math.max(0, filtered.length - 1));
    } else if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      const entry = filtered[activeIndex];
      if (entry) toggle(entry[0]);
    }
  };

  const summary = selected.length === 0
    ? "Select scopes..."
    : selected.length === 1
      ? "1 scope selected"
      : `${selected.length} scopes selected`;

  return (
    <div ref={containerRef} className="relative">
      <button
        ref={triggerRef}
        type="button"
        disabled={disabled}
        aria-expanded={open}
        aria-haspopup="listbox"
        aria-controls={listboxId}
        onClick={() => { if (open) close(); else { setOpen(true); setQuery(""); setActiveIndex(0); setTimeout(() => inputRef.current?.focus(), 0); } }}
        onKeyDown={(e) => {
          if ((e.key === "ArrowDown" || e.key === "Enter" || e.key === " ") && !open) {
            e.preventDefault();
            setOpen(true);
            setQuery("");
            setTimeout(() => inputRef.current?.focus(), 0);
          }
        }}
        className="flex h-10 w-full items-center justify-between rounded-lg border border-line bg-surface-input px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      >
        <span className={selected.length === 0 ? "text-text-muted" : "text-text"}>{summary}</span>
        <ChevronDown className="h-4 w-4 shrink-0 text-text-muted" />
      </button>
      {open && (
        <div className="absolute z-50 mt-1 w-full rounded-lg border border-line bg-surface-raised shadow-card">
          <div className="flex items-center gap-2 border-b border-line px-3 py-2">
            <Search className="h-4 w-4 shrink-0 text-text-muted" />
            <input
              ref={inputRef}
              role="combobox"
              aria-expanded
              aria-controls={listboxId}
              aria-activedescendant={filtered[activeIndex] ? `${listboxId}-${filtered[activeIndex][0]}` : undefined}
              className="min-w-0 flex-1 rounded bg-transparent text-sm text-text outline-none placeholder:text-text-muted focus-visible:ring-2 focus-visible:ring-[color-mix(in_srgb,var(--brand)_50%,transparent)]"
              placeholder={`Search ${label.toLowerCase()}...`}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={onListKeyDown}
            />
          </div>
          <div aria-multiselectable="true" className="max-h-64 overflow-y-auto p-1" role="listbox" id={listboxId} onKeyDown={onListKeyDown}>
            {filtered.length === 0 ? (
              <p className="px-2 py-6 text-center text-xs text-text-muted">No scopes match your search.</p>
            ) : (
              filtered.map(([scope, scopeLabel], index) => {
                const isSelected = selected.includes(scope);
                const isActive = index === activeIndex;
                return (
                  <div
                    key={scope}
                    id={`${listboxId}-${scope}`}
                    role="option"
                    aria-selected={isSelected}
                    tabIndex={-1}
                    onClick={() => toggle(scope)}
                    onMouseEnter={() => setActiveIndex(index)}
                    // No key handler here: Enter/Space are handled once by the
                    // listbox container above. A row-level handler would bubble
                    // to it and toggle twice.
                    className={cn(
                      "flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 hover:bg-overlay focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color-mix(in_srgb,var(--brand)_50%,transparent)]",
                      isActive && "bg-overlay",
                      isSelected && "bg-overlay-subtle",
                    )}
                  >
                    <input
                      checked={isSelected}
                      className="pointer-events-none shrink-0 accent-red-600"
                      readOnly
                      tabIndex={-1}
                      type="checkbox"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm text-text">{scopeLabel}</p>
                      <p className="truncate text-xs text-text-muted">{scope}</p>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
}
