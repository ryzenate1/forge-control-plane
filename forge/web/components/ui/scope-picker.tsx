"use client";

import { useMemo, useRef, useState } from "react";
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
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const entries = useMemo(() => Object.entries(scopes), [scopes]);
  const filtered = useMemo(() => {
    if (!query.trim()) return entries;
    const q = query.toLowerCase();
    return entries.filter(([key, val]) => key.toLowerCase().includes(q) || val.toLowerCase().includes(q));
  }, [entries, query]);

  const toggle = (scope: string) => {
    onChange(selected.includes(scope) ? selected.filter((s) => s !== scope) : [...selected, scope]);
  };

  const summary = selected.length === 0
    ? "Select scopes..."
    : selected.length === 1
      ? "1 scope selected"
      : `${selected.length} scopes selected`;

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        disabled={disabled}
        aria-expanded={open}
        aria-haspopup="listbox"
        onClick={() => { setOpen((v) => !v); setQuery(""); setTimeout(() => inputRef.current?.focus(), 0); }}
        className="flex h-10 w-full items-center justify-between rounded-lg border border-white/[0.12] bg-surface-input px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      >
        <span className={selected.length === 0 ? "text-slate-500" : "text-slate-100"}>{summary}</span>
        <ChevronDown className="h-4 w-4 shrink-0 text-slate-400" />
      </button>
      {open && (
        <div className="absolute z-50 mt-1 w-full rounded-lg border border-white/[0.12] bg-surface-elevated shadow-card">
          <div className="flex items-center gap-2 border-b border-white/[0.06] px-3 py-2">
            <Search className="h-4 w-4 shrink-0 text-slate-500" />
            <input
              ref={inputRef}
              className="min-w-0 flex-1 rounded bg-transparent text-sm text-slate-100 outline-none placeholder:text-slate-500 focus-visible:ring-2 focus-visible:ring-brand/50"
              placeholder={`Search ${label.toLowerCase()}...`}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <div aria-multiselectable="true" className="max-h-64 overflow-y-auto p-1" role="listbox">
            {filtered.length === 0 ? (
              <p className="px-2 py-6 text-center text-xs text-slate-500">No scopes match your search.</p>
            ) : (
              filtered.map(([scope, scopeLabel]) => {
                const isSelected = selected.includes(scope);
                return (
                  <div
                    key={scope}
                    role="option"
                    aria-selected={isSelected}
                    onClick={() => toggle(scope)}
                    className={cn(
                      "flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 hover:bg-white/[0.06]",
                      isSelected && "bg-white/[0.04]",
                    )}
                  >
                    <input
                      checked={isSelected}
                      className="pointer-events-none shrink-0 accent-red-600"
                      readOnly
                      type="checkbox"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm text-slate-200">{scopeLabel}</p>
                      <p className="truncate text-xs text-slate-500">{scope}</p>
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
