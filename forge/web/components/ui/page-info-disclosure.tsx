"use client";

import { useState, type ReactNode } from "react";
import { Info } from "lucide-react";
import { Dialog, Button } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

export interface InfoSection {
  title: string;
  content: ReactNode;
  icon?: React.ComponentType<{ className?: string; size?: number }>;
}

export interface PageInfoDisclosureProps {
  title?: string;
  eyebrow?: string;
  description?: string;
  sections?: InfoSection[];
  children?: ReactNode;
  className?: string;
  triggerClassName?: string;
  triggerLabel?: string;
}

export function PageInfoDisclosure({
  title = "About this page",
  eyebrow = "Control Plane Guide",
  description = "Understand how this dashboard sources and validates operational truth.",
  sections,
  children,
  className,
  triggerClassName,
  triggerLabel = "Learn about this page",
}: PageInfoDisclosureProps) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <>
      <button
        type="button"
        onClick={() => setIsOpen(true)}
        className={cn(
          "inline-flex h-7 w-7 items-center justify-center rounded-lg border border-line bg-overlay-subtle text-text-subtle transition-colors hover:border-line-strong hover:bg-overlay-strong hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color-mix(in_srgb,var(--brand)_50%,transparent)]",
          triggerClassName
        )}
        aria-label={triggerLabel}
        title={triggerLabel}
      >
        <Info size={14} aria-hidden="true" />
      </button>

      <Dialog
        open={isOpen}
        closeAction={() => setIsOpen(false)}
        title={
          <div className="flex items-center gap-2.5">
            <span className="grid h-7 w-7 place-items-center rounded-lg border border-[color-mix(in_srgb,var(--brand)_30%,transparent)] bg-brand-subtle text-brand">
              <Info size={14} />
            </span>
            <div>
              <span className="block text-[11px] font-semibold uppercase tracking-wider text-text-muted">
                {eyebrow}
              </span>
              <span className="text-base font-semibold text-text">{title}</span>
            </div>
          </div>
        }
        description={description}
        className={cn("max-w-xl", className)}
      >
        <div className="space-y-4">
          {sections && sections.length > 0 ? (
            <div className="divide-y divide-line rounded-xl border border-line bg-well">
              {sections.map((section, idx) => {
                const Icon = section.icon;
                return (
                  <div key={idx} className="p-4 space-y-1">
                    <div className="flex items-center gap-2">
                      {Icon ? <Icon size={14} className="text-text-subtle shrink-0" /> : null}
                      <h4 className="text-xs font-semibold uppercase tracking-wider text-text">
                        {section.title}
                      </h4>
                    </div>
                    <div className="text-xs leading-relaxed text-text-subtle pt-0.5">
                      {section.content}
                    </div>
                  </div>
                );
              })}
            </div>
          ) : null}

          {children ? <div className="text-xs text-text-subtle leading-relaxed">{children}</div> : null}

          <div className="flex justify-end pt-2">
            <Button variant="secondary" size="sm" onClick={() => setIsOpen(false)}>
              Got it
            </Button>
          </div>
        </div>
      </Dialog>
    </>
  );
}
