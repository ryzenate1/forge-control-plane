"use client";

/**
 * Buttons and toggles.
 *
 * One geometry, one motion curve, one disabled treatment. Variants carry
 * colour only — nothing here changes height or radius per call site, which is
 * what made the old buttons look like three different products.
 */

import * as React from "react";
import Link from "next/link";
import { LoaderCircle } from "lucide-react";
import { cn } from "@/lib/utils";

/* -------------------------------------------------------------------------- */
/* Button                                                                     */
/* -------------------------------------------------------------------------- */

export type ForgeButtonVariant =
  | "primary"
  | "secondary"
  | "outline"
  | "ghost"
  | "danger"
  | "link"
  // Legacy aliases kept so existing call sites keep compiling.
  | "default"
  | "destructive";

export type ForgeButtonSize = "xs" | "sm" | "md" | "lg" | "icon";

const variantClass: Record<ForgeButtonVariant, string> = {
  primary: "ui-button-primary",
  default: "ui-button-primary",
  secondary: "ui-button-secondary",
  outline: "ui-button-outline",
  ghost: "ui-button-ghost",
  danger: "ui-button-danger",
  destructive: "ui-button-danger",
  link: "border-transparent bg-transparent px-0 text-brand underline-offset-4 hover:underline",
};

const sizeClass: Record<ForgeButtonSize, string> = {
  xs: "h-7 min-h-7 gap-1 px-2 text-meta",
  sm: "h-8 min-h-8 px-2.5",
  md: "",
  lg: "h-10 min-h-10 px-5 text-sm",
  icon: "h-9 w-9 px-0",
};

export type ForgeButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ForgeButtonVariant;
  size?: ForgeButtonSize;
  /** Shows a spinner and disables the button. Use for in-flight mutations. */
  loading?: boolean;
  /** Leading icon. Pass the element, not the component. */
  icon?: React.ReactNode;
  /** Trailing icon — chevrons, external-link marks. */
  trailingIcon?: React.ReactNode;
  /** Stretches to the container width. Used in dialog footers on mobile. */
  block?: boolean;
};

export const ForgeButton = React.forwardRef<HTMLButtonElement, ForgeButtonProps>(
  function ForgeButton(
    {
      variant = "secondary",
      size = "md",
      loading = false,
      icon,
      trailingIcon,
      block = false,
      className,
      children,
      disabled,
      type = "button",
      ...rest
    },
    ref
  ) {
    return (
      <button
        aria-busy={loading || undefined}
        className={cn(
          "ui-button",
          variantClass[variant],
          sizeClass[size],
          block && "w-full",
          className
        )}
        disabled={disabled || loading}
        ref={ref}
        type={type}
        {...rest}
      >
        {loading ? (
          <LoaderCircle aria-hidden="true" className="size-3.5 shrink-0 animate-spin" />
        ) : (
          icon
        )}
        {children}
        {trailingIcon}
      </button>
    );
  }
);

/** Button-shaped link. Same geometry as ForgeButton so rows stay aligned. */
export function ForgeButtonLink({
  href,
  variant = "secondary",
  size = "md",
  icon,
  trailingIcon,
  block = false,
  external = false,
  className,
  children,
  ...rest
}: Omit<React.AnchorHTMLAttributes<HTMLAnchorElement>, "href"> & {
  href: string;
  variant?: ForgeButtonVariant;
  size?: ForgeButtonSize;
  icon?: React.ReactNode;
  trailingIcon?: React.ReactNode;
  block?: boolean;
  external?: boolean;
}) {
  const classes = cn(
    "ui-button",
    variantClass[variant],
    sizeClass[size],
    block && "w-full",
    className
  );
  const content = (
    <>
      {icon}
      {children}
      {trailingIcon}
    </>
  );
  if (external) {
    return (
      <a className={classes} href={href} rel="noreferrer noopener" target="_blank" {...rest}>
        {content}
      </a>
    );
  }
  return (
    <Link className={classes} href={href} {...rest}>
      {content}
    </Link>
  );
}

/* -------------------------------------------------------------------------- */
/* Icon button                                                                */
/* -------------------------------------------------------------------------- */

export type ForgeIconButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  /** Required: an icon-only control with no accessible name is unusable. */
  label: string;
  icon?: React.ReactNode;
  size?: "sm" | "md";
  tone?: "default" | "danger";
  loading?: boolean;
};

export const ForgeIconButton = React.forwardRef<HTMLButtonElement, ForgeIconButtonProps>(
  function ForgeIconButton(
    { label, icon, size = "md", tone = "default", loading = false, className, children, disabled, type = "button", ...rest },
    ref
  ) {
    return (
      <button
        aria-busy={loading || undefined}
        aria-label={label}
        className={cn(
          "ui-icon-button",
          size === "sm" && "h-7 w-7",
          tone === "danger" && "text-danger hover:bg-danger-subtle hover:text-danger",
          className
        )}
        disabled={disabled || loading}
        ref={ref}
        title={label}
        type={type}
        {...rest}
      >
        {loading ? (
          <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
        ) : (
          (icon ?? children)
        )}
      </button>
    );
  }
);

/* -------------------------------------------------------------------------- */
/* Button group / segmented control                                           */
/* -------------------------------------------------------------------------- */

/** Joins buttons into a single control. Used for time-window pickers. */
export function ForgeButtonGroup({
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "inline-flex items-center gap-0.5 rounded-lg border border-line bg-overlay-subtle p-0.5",
        className
      )}
      role="group"
      {...rest}
    >
      {children}
    </div>
  );
}

export type ForgeSegmentOption<T extends string> = {
  value: T;
  label: React.ReactNode;
  icon?: React.ReactNode;
  disabled?: boolean;
};

/** Segmented control. Exactly one option is selected at all times. */
export function ForgeSegmented<T extends string>({
  options,
  value,
  onChange,
  label,
  className,
}: {
  options: readonly ForgeSegmentOption<T>[];
  value: T;
  onChange: (value: T) => void;
  label: string;
  className?: string;
}) {
  return (
    <ForgeButtonGroup aria-label={label} className={className}>
      {options.map((option) => {
        const selected = option.value === value;
        return (
          <button
            aria-pressed={selected}
            className={cn(
              "inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-meta font-semibold",
              "transition-colors duration-150 disabled:pointer-events-none disabled:opacity-40",
              selected
                ? "bg-surface-raised text-text shadow-flat"
                : "text-text-subtle hover:text-text"
            )}
            disabled={option.disabled}
            key={option.value}
            onClick={() => onChange(option.value)}
            type="button"
          >
            {option.icon}
            {option.label}
          </button>
        );
      })}
    </ForgeButtonGroup>
  );
}

/* -------------------------------------------------------------------------- */
/* Switch                                                                     */
/* -------------------------------------------------------------------------- */

export function ForgeSwitch({
  checked,
  onChange,
  label,
  description,
  disabled = false,
  className,
  id,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  /** Visible label. Omit only when an adjacent label already names the control. */
  label?: React.ReactNode;
  description?: React.ReactNode;
  disabled?: boolean;
  className?: string;
  id?: string;
}) {
  const switchId = React.useId();
  const labelId = `${switchId}-label`;
  const toggle = (
    <button
      aria-checked={checked}
      aria-labelledby={label ? labelId : undefined}
      className={cn(
        "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors duration-150",
        "disabled:pointer-events-none disabled:opacity-45",
        checked ? "border-transparent bg-brand" : "border-line bg-overlay-strong"
      )}
      disabled={disabled}
      id={id ?? switchId}
      onClick={(event) => {
        // The labelled wrapper below also toggles on click, so a click on the
        // switch itself must not bubble — otherwise onChange fires twice and
        // the switch appears stuck.
        event.stopPropagation();
        onChange(!checked);
      }}
      role="switch"
      type="button"
    >
      <span
        className={cn(
          "pointer-events-none ml-0.5 size-4 rounded-full bg-white shadow-flat transition-transform duration-150",
          checked ? "translate-x-4" : "translate-x-0"
        )}
      />
    </button>
  );

  if (!label && !description) {
    return <span className={className}>{toggle}</span>;
  }

  return (
    <div className={cn("flex cursor-pointer items-start gap-2.5", className)} onClick={disabled ? undefined : () => onChange(!checked)} role="presentation">
      {toggle}
      <span className="min-w-0 space-y-0.5" id={label ? labelId : undefined}>
        {label ? <span className="block text-xs font-semibold text-text">{label}</span> : null}
        {description ? <span className="ui-hint block">{description}</span> : null}
      </span>
    </div>
  );
}
