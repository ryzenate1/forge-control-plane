"use client";

/**
 * Form controls.
 *
 * Every field is the same height, shares one focus ring, and reports errors in
 * the same place. `ForgeField` owns label/hint/error wiring and the
 * `aria-describedby` / `aria-invalid` plumbing so no call site has to repeat it.
 */

import * as React from "react";
import { Check, ChevronDown, Search, X } from "lucide-react";
import { cn } from "@/lib/utils";

/* -------------------------------------------------------------------------- */
/* Field                                                                      */
/* -------------------------------------------------------------------------- */

export type ForgeFieldProps = {
  label?: React.ReactNode;
  /** Guidance shown under the control. Suppressed while an error is showing. */
  hint?: React.ReactNode;
  error?: React.ReactNode;
  required?: boolean;
  /** Right-aligned slot in the label row — "Optional", a help link, a counter. */
  labelAction?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
  htmlFor?: string;
};

export function ForgeField({
  label,
  hint,
  error,
  required = false,
  labelAction,
  className,
  children,
  htmlFor,
}: ForgeFieldProps) {
  const fallbackId = React.useId();
  const controlId = htmlFor ?? fallbackId;
  const describedBy = error ? `${controlId}-error` : hint ? `${controlId}-hint` : undefined;
  let wiredChildren = children;
  if (describedBy && React.isValidElement(children)) {
    const child = children as React.ReactElement<{ "aria-describedby"?: string; "aria-invalid"?: boolean | string; id?: string }>;
    wiredChildren = React.cloneElement(child, {
      id: child.props.id ?? controlId,
      "aria-describedby": [child.props["aria-describedby"], describedBy].filter(Boolean).join(" ") || undefined,
      "aria-invalid": error ? true : child.props["aria-invalid"],
    });
  }
  return (
    <div className={cn("space-y-1.5", className)}>
      {label || labelAction ? (
        <div className="flex items-baseline justify-between gap-2">
          {label ? (
            <label className="ui-label" htmlFor={controlId}>
              {label}
              {required ? (
                <span aria-hidden="true" className="ml-0.5 text-danger">
                  *
                </span>
              ) : null}
            </label>
          ) : (
            <span />
          )}
          {labelAction}
        </div>
      ) : null}
      {wiredChildren}
      {error ? (
        <p className="ui-field-error" id={`${controlId}-error`} role="alert">{error}</p>
      ) : hint ? (
        <p className="ui-hint" id={`${controlId}-hint`}>{hint}</p>
      ) : null}
    </div>
  );
}

/**
 * Field + control in one call, with ids and ARIA wired up.
 * Use when the control is a plain input; compose `ForgeField` by hand otherwise.
 */
export function ForgeInputField({
  label,
  hint,
  error,
  required,
  labelAction,
  className,
  id,
  ...inputProps
}: ForgeFieldProps & React.InputHTMLAttributes<HTMLInputElement>) {
  const generatedId = React.useId();
  const fieldId = id ?? generatedId;
  const describedBy = error ? `${fieldId}-error` : hint ? `${fieldId}-hint` : undefined;

  return (
    <div className={cn("space-y-1.5", className)}>
      {label || labelAction ? (
        <div className="flex items-baseline justify-between gap-2">
          {label ? (
            <label className="ui-label" htmlFor={fieldId}>
              {label}
              {required ? (
                <span aria-hidden="true" className="ml-0.5 text-danger">
                  *
                </span>
              ) : null}
            </label>
          ) : (
            <span />
          )}
          {labelAction}
        </div>
      ) : null}
      <ForgeInput
        aria-describedby={describedBy}
        aria-invalid={error ? true : undefined}
        id={fieldId}
        required={required}
        {...inputProps}
      />
      {error ? (
        <p className="ui-field-error" id={`${fieldId}-error`}>
          {error}
        </p>
      ) : hint ? (
        <p className="ui-hint" id={`${fieldId}-hint`}>
          {hint}
        </p>
      ) : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Input / textarea                                                          */
/* -------------------------------------------------------------------------- */

export type ForgeInputProps = React.InputHTMLAttributes<HTMLInputElement> & {
  /** Leading adornment — an icon or a unit. Insets the text automatically. */
  prefix?: React.ReactNode;
  /** Trailing adornment — a unit, a visibility toggle, a clear button. */
  suffix?: React.ReactNode;
  /** Renders the value in mono. Use for ids, hosts, ports, paths. */
  mono?: boolean;
};

export const ForgeInput = React.forwardRef<HTMLInputElement, ForgeInputProps>(
  function ForgeInput({ prefix, suffix, mono, className, ...rest }, ref) {
    const input = (
      <input
        className={cn(
          "ui-input",
          mono && "font-mono text-meta",
          prefix && "pl-8",
          suffix && "pr-8",
          className
        )}
        ref={ref}
        {...rest}
      />
    );
    if (!prefix && !suffix) return input;
    return (
      <div className="relative">
        {prefix ? (
          <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-text-muted">
            {prefix}
          </span>
        ) : null}
        {input}
        {suffix ? (
          <span className="absolute right-2 top-1/2 -translate-y-1/2 text-text-muted">{suffix}</span>
        ) : null}
      </div>
    );
  }
);

export const ForgeTextarea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement> & { mono?: boolean }
>(function ForgeTextarea({ className, mono, rows = 4, ...rest }, ref) {
  return (
    <textarea
      className={cn(
        "ui-input h-auto min-h-0 resize-y py-2 leading-5",
        mono && "font-mono text-meta",
        className
      )}
      ref={ref}
      rows={rows}
      {...rest}
    />
  );
});

/* -------------------------------------------------------------------------- */
/* Select                                                                     */
/* -------------------------------------------------------------------------- */

export type ForgeSelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

/**
 * Native select, styled to match. Native is the right default: it is
 * accessible, keyboard-complete and correct on touch. Reach for `ForgeCombobox`
 * only when the list needs filtering.
 */
export const ForgeSelect = React.forwardRef<
  HTMLSelectElement,
  React.SelectHTMLAttributes<HTMLSelectElement> & {
    options?: readonly ForgeSelectOption[];
    placeholder?: string;
  }
>(function ForgeSelect({ options, placeholder, className, children, ...rest }, ref) {
  return (
    <div className="relative">
      <select
        className={cn("ui-input cursor-pointer appearance-none pr-8", className)}
        ref={ref}
        {...rest}
      >
        {placeholder ? (
          <option disabled value="">
            {placeholder}
          </option>
        ) : null}
        {options?.map((option) => (
          <option disabled={option.disabled} key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
        {children}
      </select>
      <ChevronDown
        aria-hidden="true"
        className="pointer-events-none absolute right-2.5 top-1/2 size-3.5 -translate-y-1/2 text-text-muted"
      />
    </div>
  );
});

/* -------------------------------------------------------------------------- */
/* Search input                                                               */
/* -------------------------------------------------------------------------- */

export function ForgeSearchInput({
  value,
  onValueChange,
  placeholder = "Search",
  className,
  ...rest
}: Omit<React.InputHTMLAttributes<HTMLInputElement>, "value" | "onChange"> & {
  value: string;
  onValueChange: (value: string) => void;
}) {
  return (
    <div className={cn("relative", className)}>
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-text-muted"
      />
      <input
        className="ui-input pl-8 pr-8"
        onChange={(event) => onValueChange(event.target.value)}
        placeholder={placeholder}
        type="search"
        value={value}
        {...rest}
      />
      {value ? (
        <button
          aria-label="Clear search"
          className="absolute right-1.5 top-1/2 grid size-6 -translate-y-1/2 place-items-center rounded-sm text-text-muted transition-colors hover:text-text"
          onClick={() => onValueChange("")}
          type="button"
        >
          <X aria-hidden="true" className="size-3.5" />
        </button>
      ) : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Combobox                                                                   */
/* -------------------------------------------------------------------------- */

export type ForgeComboboxOption = {
  value: string;
  label: string;
  /** Second line in the option row — an id, a region, an owner. */
  description?: string;
  disabled?: boolean;
};

/**
 * Filterable single-select. Full keyboard support: type to filter, arrows to
 * move, Enter to pick, Escape to close, Tab to leave.
 */
export function ForgeCombobox({
  options,
  value,
  onChange,
  placeholder = "Select",
  emptyLabel = "No matches",
  disabled = false,
  id,
  className,
  buttonClassName,
}: {
  options: readonly ForgeComboboxOption[];
  value: string | null;
  onChange: (value: string) => void;
  placeholder?: string;
  emptyLabel?: string;
  disabled?: boolean;
  id?: string;
  className?: string;
  buttonClassName?: string;
}) {
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState("");
  const [activeIndex, setActiveIndex] = React.useState(0);
  const containerRef = React.useRef<HTMLDivElement>(null);
  const inputRef = React.useRef<HTMLInputElement>(null);
  const listboxId = React.useId();

  const selected = options.find((option) => option.value === value) ?? null;

  const filtered = React.useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return options;
    return options.filter(
      (option) =>
        option.label.toLowerCase().includes(needle) ||
        option.value.toLowerCase().includes(needle) ||
        (option.description ?? "").toLowerCase().includes(needle)
    );
  }, [options, query]);

  React.useEffect(() => {
    setActiveIndex(0);
  }, [query, open]);

  React.useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onPointerDown);
    return () => document.removeEventListener("mousedown", onPointerDown);
  }, [open]);

  React.useEffect(() => {
    if (open) inputRef.current?.focus();
    else setQuery("");
  }, [open]);

  const commit = (option: ForgeComboboxOption) => {
    if (option.disabled) return;
    onChange(option.value);
    setOpen(false);
  };

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Escape") {
      event.preventDefault();
      setOpen(false);
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) {
        setOpen(true);
        return;
      }
      if (filtered.length === 0) return;
      const delta = event.key === "ArrowDown" ? 1 : -1;
      setActiveIndex((current) => (current + delta + filtered.length) % filtered.length);
      return;
    }
    if (event.key === "Enter" && open) {
      event.preventDefault();
      const option = filtered[activeIndex];
      if (option) commit(option);
    }
  };

  return (
    <div className={cn("relative", className)} ref={containerRef}>
      <button
        aria-controls={open ? listboxId : undefined}
        aria-expanded={open}
        aria-haspopup="listbox"
        className={cn("ui-input flex items-center justify-between gap-2 text-left", buttonClassName)}
        disabled={disabled}
        id={id}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={onKeyDown}
        type="button"
      >
        <span className={cn("truncate", selected ? "text-text" : "text-text-muted")}>
          {selected?.label ?? placeholder}
        </span>
        <ChevronDown aria-hidden="true" className="size-3.5 shrink-0 text-text-muted" />
      </button>

      {open ? (
        <div className="ui-popover absolute left-0 right-0 top-[calc(100%+4px)] p-0">
          <div className="border-b border-line p-1.5">
            <input
              aria-autocomplete="list"
              aria-controls={listboxId}
              className="ui-input h-8"
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={onKeyDown}
              placeholder="Filter"
              ref={inputRef}
              value={query}
            />
          </div>
          <ul className="max-h-60 overflow-y-auto p-1" id={listboxId} role="listbox">
            {filtered.length === 0 ? (
              <li className="px-2 py-3 text-center text-meta text-text-muted">{emptyLabel}</li>
            ) : (
              filtered.map((option, index) => {
                const isSelected = option.value === value;
                return (
                  <li key={option.value}>
                    <button
                      aria-selected={isSelected}
                      className={cn(
                        "ui-menu-item w-full justify-between",
                        index === activeIndex && "bg-overlay",
                        option.disabled && "pointer-events-none opacity-45"
                      )}
                      onClick={() => commit(option)}
                      onMouseEnter={() => setActiveIndex(index)}
                      role="option"
                      type="button"
                    >
                      <span className="min-w-0">
                        <span className="block truncate">{option.label}</span>
                        {option.description ? (
                          <span className="block truncate text-meta text-text-muted">
                            {option.description}
                          </span>
                        ) : null}
                      </span>
                      {isSelected ? (
                        <Check aria-hidden="true" className="size-3.5 shrink-0 text-brand" />
                      ) : null}
                    </button>
                  </li>
                );
              })
            )}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Form scaffolding                                                           */
/* -------------------------------------------------------------------------- */

/** A titled group of fields. Two columns on wide screens, one on narrow. */
export function ForgeFormSection({
  title,
  description,
  columns = 1,
  className,
  children,
}: {
  title?: React.ReactNode;
  description?: React.ReactNode;
  columns?: 1 | 2;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <section className={cn("space-y-3", className)}>
      {title || description ? (
        <div className="space-y-0.5 border-b border-line pb-2">
          {title ? <h3 className="t-section">{title}</h3> : null}
          {description ? (
            <p className="max-w-prose text-meta text-text-subtle">{description}</p>
          ) : null}
        </div>
      ) : null}
      <div className={cn("grid gap-3", columns === 2 && "sm:grid-cols-2")}>{children}</div>
    </section>
  );
}

/** Action row for a form. Primary action last, so it lands nearest the thumb. */
export function ForgeFormActions({
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "flex flex-col-reverse items-stretch gap-2 border-t border-line pt-3.5 sm:flex-row sm:items-center sm:justify-end",
        className
      )}
      {...rest}
    >
      {children}
    </div>
  );
}
