"use client";

import { cn } from "@/lib/utils";
import type { LucideIcon } from "lucide-react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useState } from "react";

/**
 * Canonical Section primitive — universalizes the 4 exemplar pages
 * (overview, health, monitoring, host) design philosophy:
 *   - container: mx-auto w-full max-w-[1280px]
 *   - tokens: var(--line) for dividers, var(--surface) for cards
 *   - eyebrow: 11px uppercase tracking [0.12em] (or [0.08em] for card labels)
 *   - title: 30–32px tracking [-0.03em] leading-none, font [600–650]
 *   - card spacing: p-4 (16px = space md) + gap-4 (16px)
 *   - typography: Manrope for body (var(--font-sans) after beautify),
 *                 Space Grotesk for numbers (var(--font-display)),
 *                 JetBrains Mono for code/mono fallback
 *
 * Two variants:
 *   - PageHeader: eyebrow + title + description + divider (top of page)
 *   - Section:    card or bordered block with optional collapsible header
 */

type PageHeaderProps = {
  eyebrow: string;
  title: string;
  description?: string;
  meta?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
};

export function PageHeader({ eyebrow, title, description, meta, action, className }: PageHeaderProps) {
  return (
    <div className={cn("border-b border-[var(--line)] pb-5", className)}>
      <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]">{eyebrow}</div>
      <div className="mt-2 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-[30px] font-[650] tracking-[-0.03em] leading-none">{title}</h1>
          {description ? <p className="mt-2 max-w-[65ch] text-sm leading-5 text-[var(--text-subtle)]">{description}</p> : null}
          {meta ? <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">{meta}</div> : null}
        </div>
        {action ? <div className="flex shrink-0 flex-wrap gap-2">{action}</div> : null}
      </div>
    </div>
  );
}

type SectionProps = {
  eyebrow?: string;
  title?: string;
  description?: string;
  icon?: LucideIcon;
  count?: string;
  defaultOpen?: boolean;
  collapsible?: boolean;
  children: React.ReactNode;
  className?: string;
  headerClassName?: string;
  contentClassName?: string;
  /** when true, wraps in rounded-xl border bg-[var(--surface)] — default true for card sections */
  card?: boolean;
  action?: React.ReactNode;
};

export function Section({
  eyebrow,
  title,
  description,
  icon: Icon,
  count,
  defaultOpen = true,
  collapsible = false,
  children,
  className,
  headerClassName,
  contentClassName,
  card = true,
  action,
}: SectionProps) {
  const [open, setOpen] = useState(defaultOpen);

  // Non-collapsible simple section: eyebrow + title + divider + children with 16px rhythm
  if (!collapsible) {
    return (
      <section className={cn(card ? "rounded-xl border border-[var(--line)] bg-[var(--surface)] overflow-hidden" : "", className)}>
        {title || eyebrow ? (
          <div className={cn("px-4 py-3", card ? "border-b border-[var(--line)] bg-overlay-subtle" : "border-b border-[var(--line)] pb-3", headerClassName)}>
            <div className="flex items-start justify-between gap-3">
              <div>
                {eyebrow ? <div className="text-[11px] font-semibold uppercase tracking-[0.08em] text-[var(--text-subtle)] flex items-center gap-1.5">{Icon ? <Icon size={12} /> : null}{eyebrow}</div> : null}
                {title ? (
                  <h2 className={cn("font-semibold tracking-[-0.01em]", eyebrow ? "mt-1 text-sm" : "text-sm flex items-center gap-2", headerClassName)}>
                    {Icon && !eyebrow ? <Icon size={14} className="text-[var(--text-subtle)]" /> : null}
                    {title}
                    {count ? <span className="ml-2 rounded-full bg-overlay-strong px-2 py-0.5 text-[11px] font-medium text-[var(--text-subtle)]">{count}</span> : null}
                  </h2>
                ) : null}
                {description ? <p className="mt-1 text-xs leading-5 text-[var(--text-subtle)] max-w-[65ch]">{description}</p> : null}
              </div>
              {action ? <div className="shrink-0">{action}</div> : null}
            </div>
          </div>
        ) : null}
        <div className={cn(card ? "p-4" : "pt-4", "space-y-4", contentClassName)}>{children}</div>
      </section>
    );
  }

  // Collapsible variant — mirrors AdminHealth Section pattern, now tokenized
  return (
    <div className={cn("rounded-xl border border-[var(--line)] bg-[var(--surface)] overflow-hidden", className)}>
      <button onClick={() => setOpen(!open)} className={cn("flex w-full items-center justify-between px-5 py-4 text-left hover:bg-overlay-subtle transition", headerClassName)} type="button">
        <span className="flex items-center gap-2 text-sm font-semibold">
          {Icon ? <Icon size={16} className="text-[var(--text-subtle)]" /> : null}
          {title}
          {count ? <span className="ml-2 rounded-full bg-overlay-strong px-2 py-0.5 text-[11px] font-medium text-[var(--text-subtle)]">{count}</span> : null}
        </span>
        {open ? <ChevronDown size={14} className="text-[var(--text-subtle)]" /> : <ChevronRight size={14} className="text-[var(--text-subtle)]" />}
      </button>
      {open ? <div className={cn("border-t border-[var(--line)] p-4 space-y-4", contentClassName)}>{children}</div> : null}
    </div>
  );
}

/**
 * Page container — enforces 1280px max-width used by all 4 exemplars.
 * Use to wrap page-level content so every admin page shares the same frame.
 */
export function PageContainer({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("mx-auto w-full max-w-[1280px] space-y-6", className)}>{children}</div>;
}

/**
 * Number — Space Grotesk token for tabular numbers (exemplar: 28px mono with Grotesk tracking)
 * Body text remains Plex Sans / Manrope fallback.
 */
export function NumberValue({ children, className, size = "base" }: { children: React.ReactNode; className?: string; size?: "sm" | "base" | "lg" | "xl" }) {
  const sizes = {
    sm: "text-sm font-medium tracking-[-0.01em]",
    base: "text-base font-semibold tracking-[-0.015em]",
    lg: "text-[20px] font-[600] tracking-[-0.02em] leading-none",
    xl: "text-[28px] font-[650] tracking-[-0.02em] leading-none",
  } as const;
  return <span className={cn("font-mono tabular-nums", sizes[size], className)} style={{ fontFamily: "var(--font-display, var(--font-mono)), ui-monospace, monospace" }}>{children}</span>;
}
