"use client";

/**
 * Base progress track.
 *
 * A non-finite or absent `value` is an unknown reading, not zero: the track
 * renders empty and dashed with `aria-valuenow` omitted. New code should use
 * `ForgeProgress` from `@/components/ui/forge`.
 */

import * as React from "react";
import { cn } from "@/lib/utils";

interface ProgressProps extends React.HTMLAttributes<HTMLDivElement> {
  value?: number | null;
}

const Progress = React.forwardRef<HTMLDivElement, ProgressProps>(
  ({ className, value, ...props }, ref) => {
    const known = typeof value === "number" && Number.isFinite(value);
    const clamped = known ? Math.min(100, Math.max(0, value)) : null;

    if (clamped === null) {
      return (
        <div
          aria-valuemax={100}
          aria-valuemin={0}
          aria-valuetext="Unknown"
          className={cn(
            "h-1.5 w-full rounded-full border border-dashed border-unknown-line bg-unknown-subtle",
            className
          )}
          ref={ref}
          role="progressbar"
          title="No reading available"
          {...props}
        />
      );
    }

    return (
      <div
        aria-valuemax={100}
        aria-valuemin={0}
        aria-valuenow={Math.round(clamped)}
        className={cn("h-1.5 w-full overflow-hidden rounded-full bg-overlay-strong", className)}
        ref={ref}
        role="progressbar"
        {...props}
      >
        <div
          className="h-full rounded-full bg-brand transition-[width] duration-300"
          style={{ width: `${clamped}%` }}
        />
      </div>
    );
  }
);
Progress.displayName = "Progress";

export { Progress };
