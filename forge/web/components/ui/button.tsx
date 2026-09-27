"use client";

/**
 * Base button element.
 *
 * Geometry, motion and colour all come from the canonical `.ui-button` family
 * in `app/globals.css` — this file only maps the historical variant/size names
 * onto it so the ~200 existing call sites keep working. New code should use
 * `ForgeButton` from `@/components/ui/forge`.
 */

import * as React from "react";
import { LoaderCircle } from "lucide-react";
import { cn } from "@/lib/utils";

const buttonVariants = {
  default: "ui-button-primary",
  destructive: "ui-button-danger",
  outline: "ui-button-outline",
  secondary: "ui-button-secondary",
  ghost: "ui-button-ghost",
  link: "border-transparent bg-transparent px-0 text-brand underline-offset-4 hover:underline",
};

const buttonSizes = {
  default: "",
  sm: "h-8 min-h-8 px-2.5",
  lg: "h-10 min-h-10 px-5 text-sm",
  icon: "h-9 w-9 px-0",
};

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof buttonVariants;
  size?: keyof typeof buttonSizes;
  loading?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", size = "default", loading = false, children, ...props }, ref) => {
    return (
      <button
        className={cn("ui-button", buttonVariants[variant], buttonSizes[size], className)}
        ref={ref}
        disabled={props.disabled || loading}
        aria-busy={loading || undefined}
        {...props}
      >
        {loading ? <LoaderCircle aria-hidden="true" className="size-3.5 animate-spin" /> : null}
        {children}
      </button>
    );
  }
);
Button.displayName = "Button";

export { Button, buttonVariants };
