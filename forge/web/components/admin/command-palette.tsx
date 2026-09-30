"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Terminal,
  HeartPulse,
  SlidersHorizontal,
  Ticket,
  Layers,
} from "lucide-react";
import { ForgeCommandPalette, type ForgeCommandItem } from "@/components/ui/forge";
import { adminPageRegistry } from "./admin-registry";
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

  // Global Cmd+K / Ctrl+K to toggle, Escape to close.
  //
  // The footer has always advertised "ESC to close" while nothing listened for
  // it, so the only way out of the palette was a mouse click on the backdrop.
  // (The palette overlay itself also closes on Escape; both paths converge on
  // setOpen(false), which is idempotent.)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen(!isOpen);
        return;
      }
      if (e.key === "Escape" && isOpen) {
        e.preventDefault();
        setOpen(false);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, setOpen]);

  const handleSelect = (href: string) => {
    setOpen(false);
    router.push(href);
  };

  // Operational shortcuts first, then one group per admin registry section —
  // the same order the cmdk palette used, so muscle memory survives the move
  // onto the hand-rolled ForgeCommandPalette (the single command surface;
  // there is no second primitive set).
  const items: ForgeCommandItem[] = [
    {
      id: "action-workloads",
      label: "Manage Workloads & Servers",
      description: "/admin/servers",
      group: "OPERATIONAL ACTIONS",
      icon: <Layers className="h-3.5 w-3.5 shrink-0" />,
      keywords: "action create workload new game server",
      onSelect: () => handleSelect("/admin/servers"),
    },
    {
      id: "action-monitoring",
      label: "Inspect Fleet Telemetry & Monitoring",
      description: "/admin/monitoring",
      group: "OPERATIONAL ACTIONS",
      icon: <HeartPulse className="h-3.5 w-3.5 shrink-0" />,
      keywords: "action fleet health diagnostics anomalies",
      onSelect: () => handleSelect("/admin/monitoring"),
    },
    {
      id: "action-onboarding",
      label: "Issue Host Onboarding Token",
      description: "/admin/onboarding-tokens",
      group: "OPERATIONAL ACTIONS",
      icon: <Ticket className="h-3.5 w-3.5 shrink-0" />,
      keywords: "action onboarding token new host beacon register",
      onSelect: () => handleSelect("/admin/onboarding-tokens"),
    },
    {
      id: "action-operations",
      label: "Review Async Operations Queue",
      description: "/admin/operations",
      group: "OPERATIONAL ACTIONS",
      icon: <SlidersHorizontal className="h-3.5 w-3.5 shrink-0" />,
      keywords: "action live operations queue running tasks reconciliation",
      onSelect: () => handleSelect("/admin/operations"),
    },
    {
      id: "action-terminal",
      label: "Open Remote Host Terminal",
      description: "/admin/terminal",
      group: "OPERATIONAL ACTIONS",
      icon: <Terminal className="h-3.5 w-3.5 shrink-0" />,
      keywords: "action host terminal remote shell console ssh",
      onSelect: () => handleSelect("/admin/terminal"),
    },
    ...adminPageRegistry.flatMap((group) =>
      group.items.map((item) => {
        const Icon = item.icon;
        const label = t(item.labelKey) !== item.labelKey ? t(item.labelKey) : item.label;
        // Keywords are the whole point of the registry's synonym list:
        // without them "docker" cannot find Containers and "postgres"
        // cannot find Databases, which is how people actually search.
        // The href is searchable too, as it was in the cmdk value string.
        const keywords = [...(item.keywords ?? []), item.href].join(" ");
        return {
          id: item.href,
          label,
          // The cmdk palette showed a "meta" badge for metadata-only pages;
          // this surface has no badge slot, so the capability rides along in
          // the description line instead of being dropped.
          description:
            item.capability === "metadata-only"
              ? `${item.description} · meta · ${item.href}`
              : `${item.description} · ${item.href}`,
          group: group.title.toUpperCase(),
          icon: <Icon className="h-3.5 w-3.5 shrink-0" />,
          keywords,
          onSelect: () => handleSelect(item.href),
        } satisfies ForgeCommandItem;
      })
    ),
  ];

  return (
    <ForgeCommandPalette
      open={isOpen}
      onClose={() => setOpen(false)}
      items={items}
      placeholder="Search commands, workloads, nodes, operations… (or type to jump)"
      emptyLabel="No matching pages or operations found."
    />
  );
}
