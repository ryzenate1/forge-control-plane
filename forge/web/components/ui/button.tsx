import * as React from "react";
import { cn } from "@/lib/utils";

const buttonVariants = {
  default: "bg-brand text-white hover:bg-brand-hover shadow",
  destructive: "bg-red-600 text-white hover:bg-red-700 shadow",
  outline: "border border-white/[0.12] bg-transparent text-slate-300 hover:bg-white/[0.06] hover:text-slate-100",
  secondary: "bg-surface-card-header text-slate-300 hover:bg-white/[0.08]",
  ghost: "text-slate-400 hover:text-slate-100 hover:bg-white/[0.06]",
  link: "text-brand underline-offset-4 hover:underline",
};

const buttonSizes = {
  default: "h-10 px-4 py-2",
  sm: "h-8 rounded-md px-3 text-xs",
  lg: "h-12 rounded-md px-8",
  icon: "h-10 w-10",
};

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof buttonVariants;
  size?: keyof typeof buttonSizes;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", size = "default", ...props }, ref) => {
    return (
      <button
        className={cn(
          "inline-flex items-center justify-center whitespace-nowrap rounded-lg text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/50 disabled:pointer-events-none disabled:opacity-50",
          buttonVariants[variant],
          buttonSizes[size],
          className
        )}
        ref={ref}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";

export { Button, buttonVariants };
