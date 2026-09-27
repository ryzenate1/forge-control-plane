"use client";

/**
 * Base input element. One height, one focus ring — both from `.ui-input`.
 * New code should use `ForgeInput` from `@/components/ui/forge`.
 */

import * as React from "react";
import { cn } from "@/lib/utils";

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => {
    return <input className={cn("ui-input", className)} ref={ref} type={type} {...props} />;
  }
);
Input.displayName = "Input";

export { Input };
