"use client";

/**
 * Card with a status accent rail. Tones resolve through the canonical status
 * vocabulary, so an unrecognised tone reads grey rather than healthy.
 */

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { resolveTone, toneStyles, type ToneInput } from "@/components/ui/forge/status";

type StatusTone = "ok" | "warning" | "danger" | "neutral";

export function StatusCard({ tone = "neutral", title, sub, icon, children, className }: {
  tone?: StatusTone | ToneInput;
  title: ReactNode;
  sub?: ReactNode;
  icon?: ReactNode;
  children?: ReactNode;
  className?: string;
}) {
  const resolved = resolveTone(tone);
  return (
    <section
      className={cn(
        "ui-surface border-l-[3px] p-4 shadow-card sm:p-5",
        toneStyles[resolved].border,
        className
      )}
    >
      <div className="flex items-start gap-2.5">
        {icon ? (
          <span className={cn("mt-px shrink-0", toneStyles[resolved].fg)}>{icon}</span>
        ) : null}
        <div className="min-w-0 flex-1">
          <h2 className="t-section flex items-center gap-2">{title}</h2>
          {sub ? <p className="mt-1 max-w-prose text-meta text-text-subtle">{sub}</p> : null}
        </div>
      </div>
      {children ? <div className="mt-3.5">{children}</div> : null}
    </section>
  );
}
