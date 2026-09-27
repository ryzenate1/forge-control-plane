"use client";

/**
 * Base badge element, mapped onto the canonical `.ui-badge` family.
 *
 * Forge badges are square-shouldered mono chips, not rounded pills — a badge
 * carries a machine-readable value (a state, a count, a version), so it reads
 * like one. New code should use `ForgeBadge` from `@/components/ui/forge`.
 */

import * as React from "react";
import { cn } from "@/lib/utils";

const badgeVariants = {
  default: "ui-badge-brand",
  secondary: "ui-badge-neutral",
  destructive: "ui-badge-danger",
  outline: "ui-badge-neutral",
};

interface BadgeProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: keyof typeof badgeVariants;
}

function Badge({ className, variant = "default", ...props }: BadgeProps) {
  return <div className={cn("ui-badge", badgeVariants[variant], className)} {...props} />;
}

Badge.displayName = "Badge";
export { Badge, badgeVariants };
