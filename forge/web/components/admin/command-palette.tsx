"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Command } from "cmdk";
import {
  Search,
  Server,
  Terminal,
  Activity,
  HeartPulse,
  SlidersHorizontal,
  KeyRound,
  Ticket,
  Box,
  Layers,
  FileCode,
  ArrowRight,
  Sparkles,
} from "lucide-react";
import { adminPageRegistry, type AdminNavEntry } from "./admin-registry";
import { useT } from "@/components/TranslationProvider";

interface CommandPaletteProps {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

export function CommandPalette({ open: controlledOpen, onOpenChange }: CommandPaletteProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const router = useRouter();
  const t = useT();

  const isControlled = controlledOpen !== undefined;
  const isOpen = isControlled ? controlledOpen : internalOpen;

  const setOpen = useCallback(
    (value: boolean) => {
      if (isControlled) {
        onOpenChange?.(value);
      } else {
        setInternalOpen(value);
      }
    },
    [isControlled, onOpenChange]
  );

  // Global Cmd+K / Ctrl+K listener
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen(!isOpen);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, setOpen]);

  const handleSelect = (href: string) => {
    setOpen(false);
    router.push(href);
  };

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-[12vh] bg-black/60 backdrop-blur-sm p-4 animate-in fade-in-0 duration-150"
      onClick={() => setOpen(false)}
      role="dialog"
      aria-modal="true"
      aria-label="Forge Command Palette"
    >
      <div
        className="w-full max-w-2xl overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)] shadow-2xl animate-in zoom-in-95 duration-150"
        onClick={(e) => e.stopPropagation()}
      >
        <Command
          className="flex flex-col w-full text-[var(--text)]"
          loop
          filter={(value, search) => {
            const cleanVal = value.toLowerCase();
            const cleanSearch = search.toLowerCase().trim();
            if (cleanVal.includes(cleanSearch)) return 1;
            return 0;
          }}
        >
          {/* Header & Search Input */}
          <div className="flex items-center gap-3 border-b border-[var(--line)] px-4 py-3 bg-[var(--surface-input)]/50">
            <Search className="h-4 w-4 shrink-0 text-[var(--text-subtle)]" />
            <Command.Input
              autoFocus
              placeholder="Search commands, workloads, nodes, operations… (or type to jump)"
              className="flex-1 bg-transparent text-sm text-[var(--text)] outline-none placeholder:text-[var(--text-muted)]"
            />
            <div className="flex items-center gap-1.5 font-mono text-[10px] text-[var(--text-muted)]">
              <kbd className="rounded border border-[var(--line)] bg-[var(--surface-raised)] px-1.5 py-0.5">ESC</kbd>
              <span>to close</span>
            </div>
          </div>

          {/* Results List */}
          <Command.List className="max-h-[380px] overflow-y-auto p-2 scroll-py-2 focus:outline-none">
            <Command.Empty className="py-8 text-center text-sm text-[var(--text-subtle)]">
              No matching pages or operations found.
            </Command.Empty>

            {/* Quick Operational Actions */}
            <Command.Group heading="OPERATIONAL ACTIONS" className="px-2 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]">
              <Command.Item
                value="action create workload new game server"
                onSelect={() => handleSelect("/admin/servers")}
                className="flex cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-xs font-medium text-[var(--text-subtle)] transition-colors data-[selected=true]:bg-[var(--brand)]/10 data-[selected=true]:text-[var(--brand)]"
              >
                <div className="flex items-center gap-2.5">
                  <Layers className="h-3.5 w-3.5 text-[var(--brand)]" />
                  <span>Manage Workloads & Servers</span>
                </div>
                <span className="font-mono text-[10px] text-[var(--text-muted)]">/admin/servers</span>
              </Command.Item>

              <Command.Item
                value="action fleet health diagnostics anomalies"
                onSelect={() => handleSelect("/admin/monitoring")}
                className="flex cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-xs font-medium text-[var(--text-subtle)] transition-colors data-[selected=true]:bg-[var(--brand)]/10 data-[selected=true]:text-[var(--brand)]"
              >
                <div className="flex items-center gap-2.5">
                  <HeartPulse className="h-3.5 w-3.5 text-emerald-400" />
                  <span>Inspect Fleet Telemetry & Monitoring</span>
                </div>
                <span className="font-mono text-[10px] text-[var(--text-muted)]">/admin/monitoring</span>
              </Command.Item>

              <Command.Item
                value="action onboarding token new host beacon register"
                onSelect={() => handleSelect("/admin/onboarding-tokens")}
                className="flex cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-xs font-medium text-[var(--text-subtle)] transition-colors data-[selected=true]:bg-[var(--brand)]/10 data-[selected=true]:text-[var(--brand)]"
              >
                <div className="flex items-center gap-2.5">
                  <Ticket className="h-3.5 w-3.5 text-amber-400" />
                  <span>Issue Host Onboarding Token</span>
                </div>
                <span className="font-mono text-[10px] text-[var(--text-muted)]">/admin/onboarding-tokens</span>
              </Command.Item>

              <Command.Item
                value="action live operations queue running tasks reconciliation"
                onSelect={() => handleSelect("/admin/operations")}
                className="flex cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-xs font-medium text-[var(--text-subtle)] transition-colors data-[selected=true]:bg-[var(--brand)]/10 data-[selected=true]:text-[var(--brand)]"
              >
                <div className="flex items-center gap-2.5">
                  <SlidersHorizontal className="h-3.5 w-3.5 text-cyan-400" />
                  <span>Review Async Operations Queue</span>
                </div>
                <span className="font-mono text-[10px] text-[var(--text-muted)]">/admin/operations</span>
              </Command.Item>

              <Command.Item
                value="action host terminal remote shell console ssh"
                onSelect={() => handleSelect("/admin/terminal")}
                className="flex cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-xs font-medium text-[var(--text-subtle)] transition-colors data-[selected=true]:bg-[var(--brand)]/10 data-[selected=true]:text-[var(--brand)]"
              >
                <div className="flex items-center gap-2.5">
                  <Terminal className="h-3.5 w-3.5 text-indigo-400" />
                  <span>Open Remote Host Terminal</span>
                </div>
                <span className="font-mono text-[10px] text-[var(--text-muted)]">/admin/terminal</span>
              </Command.Item>
            </Command.Group>

            {/* Navigation Groups */}
            {adminPageRegistry.map((group) => (
              <Command.Group
                key={group.title}
                heading={group.title.toUpperCase()}
                className="px-2 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)]"
              >
                {group.items.map((item) => {
                  const Icon = item.icon;
                  const label = t(item.labelKey) !== item.labelKey ? t(item.labelKey) : item.label;
                  return (
                    <Command.Item
                      key={item.href}
                      value={`${group.title} ${label} ${item.description} ${item.href}`}
                      onSelect={() => handleSelect(item.href)}
                      className="flex cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-xs font-medium text-[var(--text-subtle)] transition-colors data-[selected=true]:bg-[var(--brand)]/10 data-[selected=true]:text-[var(--brand)]"
                    >
                      <div className="flex items-center gap-2.5 truncate">
                        <Icon className="h-3.5 w-3.5 shrink-0" />
                        <span className="truncate">{label}</span>
                        <span className="hidden sm:inline truncate text-[11px] text-[var(--text-muted)]">
                          — {item.description}
                        </span>
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        {item.capability === "metadata-only" && (
                          <span className="rounded border border-[var(--line)] bg-[var(--surface-input)] px-1.5 py-0.2 text-[9px] font-mono text-[var(--text-muted)]">
                            meta
                          </span>
                        )}
                        <span className="font-mono text-[10px] text-[var(--text-muted)]">{item.href}</span>
                      </div>
                    </Command.Item>
                  );
                })}
              </Command.Group>
            ))}
          </Command.List>

          {/* Footer Navigation Bar */}
          <div className="flex items-center justify-between border-t border-[var(--line)] px-4 py-2 bg-[var(--surface-input)]/30 font-mono text-[11px] text-[var(--text-muted)]">
            <div className="flex items-center gap-3">
              <span className="flex items-center gap-1">
                <kbd className="rounded border border-[var(--line)] bg-[var(--surface-raised)] px-1">↑</kbd>
                <kbd className="rounded border border-[var(--line)] bg-[var(--surface-raised)] px-1">↓</kbd>
                <span className="font-sans text-[10px]">Navigate</span>
              </span>
              <span className="flex items-center gap-1">
                <kbd className="rounded border border-[var(--line)] bg-[var(--surface-raised)] px-1">↵</kbd>
                <span className="font-sans text-[10px]">Select</span>
              </span>
            </div>
            <div className="flex items-center gap-1.5">
              <Sparkles className="h-3 w-3 text-[var(--brand)]" />
              <span className="font-sans text-[10px]">Forge Control Plane</span>
            </div>
          </div>
        </Command>
      </div>
    </div>
  );
}
