"use client";

/**
 * Compositional alert parts, mapped onto the canonical `.ui-alert` family.
 * New code should use `ForgeAlert` from `@/components/ui/forge`, which also
 * carries the tone icon and the action slot.
 */

import * as React from "react";
import { cn } from "@/lib/utils";

interface AlertProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: "default" | "destructive";
}

const Alert = React.forwardRef<HTMLDivElement, AlertProps>(
  ({ className, variant = "default", ...props }, ref) => (
    <div
      className={cn(
        "ui-alert flex-col items-stretch",
        variant === "destructive" ? "ui-alert-danger" : "border-line bg-overlay-subtle text-text",
        className
      )}
      ref={ref}
      role="alert"
      {...props}
    />
  )
);
Alert.displayName = "Alert";

const AlertTitle = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLHeadingElement>>(
  ({ className, ...props }, ref) => (
    <h5 className={cn("text-xs font-semibold text-current", className)} ref={ref} {...props} />
  )
);
AlertTitle.displayName = "AlertTitle";

const AlertDescription = React.forwardRef<HTMLParagraphElement, React.HTMLAttributes<HTMLParagraphElement>>(
  ({ className, ...props }, ref) => (
    <div className={cn("text-xs leading-5 [&_p]:leading-5", className)} ref={ref} {...props} />
  )
);
AlertDescription.displayName = "AlertDescription";

export { Alert, AlertTitle, AlertDescription };
