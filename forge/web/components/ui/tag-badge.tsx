"use client";

import * as React from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * Reusable color-coded tag pill.
 *
 * The tag color is *data* (a 6-digit hex string from the backend), never a
 * Tailwind class, so we render it through an inline style after strict
 * sanitization: anything that is not exactly `#rrggbb` is rejected and falls
 * back to neutral theme styling. Text color is derived from the color's
 * relative luminance so the label stays legible on any hue. Everything except
 * the dynamic color uses CSS-variable / utility tokens.
 */

const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/;

/** Returns a normalized `#rrggbb` for safe CSS use, or null when malformed. */
export function sanitizeTagColor(color: string | undefined | null): string | null {
  if (typeof color !== "string") return null;
  const trimmed = color.trim();
  return HEX_COLOR_RE.test(trimmed) ? trimmed.toLowerCase() : null;
}

/** Pick near-black or white text for a background hex using WCAG luminance. */
function readableTextColor(hex: string): string {
  const r = parseInt(hex.slice(1, 3), 16) / 255;
  const g = parseInt(hex.slice(3, 5), 16) / 255;
  const b = parseInt(hex.slice(5, 7), 16) / 255;
  const channel = (c: number) => (c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4));
  const luminance = 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
  return luminance > 0.5 ? "#0b0f19" : "#f8fafc";
}

export interface TagBadgeProps extends Omit<React.HTMLAttributes<HTMLSpanElement>, "color"> {
  name: string;
  color: string;
  /** Optional tooltip / title text (usually the tag description). */
  description?: string;
  size?: "sm" | "md";
  /** Render as a fixed-color dot + neutral text instead of a filled pill. */
  subtle?: boolean;
  /** When provided, a remove (×) affordance is shown and this is called. */
  onRemove?: () => void;
}

export function TagBadge({
  name,
  color,
  description,
  size = "md",
  subtle = false,
  onRemove,
  className,
  title,
  ...props
}: TagBadgeProps) {
  const safe = sanitizeTagColor(color);
  const small = size === "sm";

  const baseStyle: React.CSSProperties = subtle
    ? {}
    : safe
      ? { backgroundColor: safe, color: readableTextColor(safe), borderColor: safe }
      : {};

  return (
    <span
      className={cn(
        "ui-badge max-w-full gap-1.5 tracking-tight",
        small ? "px-2 py-0.5 text-[11px]" : "px-2.5 py-1 text-xs",
        // Neutral fallback when there is no usable color, or for subtle mode.
        (subtle || !safe) && "border-[var(--line)] bg-overlay-subtle text-text-subtle",
        className,
      )}
      style={subtle ? undefined : baseStyle}
      title={title ?? description ?? name}
      {...props}
    >
      {subtle ? (
        <span
          aria-hidden="true"
          className={cn("inline-block shrink-0 rounded-full", small ? "h-1.5 w-1.5" : "h-2 w-2")}
          style={safe ? { backgroundColor: safe } : { backgroundColor: "var(--brand)" }}
        />
      ) : null}
      <span className="truncate">{name}</span>
      {onRemove ? (
        <button
          type="button"
          aria-label={`Remove ${name}`}
          onClick={(event) => {
            event.stopPropagation();
            onRemove();
          }}
          className={cn(
            "-mr-1 inline-flex shrink-0 items-center justify-center rounded-full p-0.5 transition",
            subtle || !safe ? "hover:bg-overlay-strong" : "hover:bg-[color-mix(in_srgb,#000_20%,transparent)]",
          )}
        >
          <X size={small ? 10 : 12} />
        </button>
      ) : null}
    </span>
  );
}

export default TagBadge;
