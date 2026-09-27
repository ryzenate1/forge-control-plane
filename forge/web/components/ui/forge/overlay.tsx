"use client";

/**
 * Overlays: dialog, confirm, drawer, dropdown, tabs, command palette.
 *
 * All of them share one behaviour contract, because inconsistent modals were
 * the single most jarring thing about the old UI:
 *  - Escape closes, backdrop click closes, focus is trapped while open.
 *  - Focus moves into the overlay on open and returns to the trigger on close.
 *  - Body scroll is locked for the duration.
 *  - The primary action sits last in the footer.
 */

import * as React from "react";
import { createPortal } from "react-dom";
import { AlertTriangle, Search, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { ForgeButton } from "./controls";
import { type ForgeTone, toneStyles } from "./status";

const FOCUSABLE =
  'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])';

/** Escape-to-close, focus trap, focus restore and scroll lock for one overlay. */
function useOverlayBehaviour(open: boolean, onClose: () => void) {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const restoreRef = React.useRef<HTMLElement | null>(null);

  React.useEffect(() => {
    if (!open) return;
    restoreRef.current = document.activeElement as HTMLElement | null;

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    const focusFirst = () => {
      const container = containerRef.current;
      if (!container) return;
      const target =
        container.querySelector<HTMLElement>("[data-autofocus]") ??
        container.querySelector<HTMLElement>(FOCUSABLE) ??
        container;
      target.focus();
    };
    const raf = requestAnimationFrame(focusFirst);

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        onClose();
        return;
      }
      if (event.key !== "Tab") return;
      const container = containerRef.current;
      if (!container) return;
      const focusable = Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        (element) => element.offsetParent !== null || element === document.activeElement
      );
      if (focusable.length === 0) {
        event.preventDefault();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown, true);
    return () => {
      cancelAnimationFrame(raf);
      document.removeEventListener("keydown", onKeyDown, true);
      document.body.style.overflow = previousOverflow;
      restoreRef.current?.focus?.();
    };
  }, [open, onClose]);

  return containerRef;
}

/** Renders into document.body once mounted; renders nothing during SSR. */
function OverlayPortal({ children }: { children: React.ReactNode }) {
  const [mounted, setMounted] = React.useState(false);
  React.useEffect(() => setMounted(true), []);
  if (!mounted) return null;
  return createPortal(children, document.body);
}

/* -------------------------------------------------------------------------- */
/* Dialog                                                                     */
/* -------------------------------------------------------------------------- */

export type ForgeDialogSize = "sm" | "md" | "lg" | "xl";

const dialogSizes: Record<ForgeDialogSize, string> = {
  sm: "max-w-sm",
  md: "max-w-lg",
  lg: "max-w-2xl",
  xl: "max-w-4xl",
};

export type ForgeDialogProps = {
  open: boolean;
  onClose: () => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  /** Footer actions. Primary last. */
  footer?: React.ReactNode;
  size?: ForgeDialogSize;
  /** Hides the close button. Only for dialogs that must be resolved. */
  hideClose?: boolean;
  /** Blocks backdrop-click dismissal. Escape still works. */
  static?: boolean;
  className?: string;
  bodyClassName?: string;
  children?: React.ReactNode;
};

export function ForgeDialog({
  open,
  onClose,
  title,
  description,
  footer,
  size = "md",
  hideClose = false,
  static: isStatic = false,
  className,
  bodyClassName,
  children,
}: ForgeDialogProps) {
  const containerRef = useOverlayBehaviour(open, onClose);
  const titleId = React.useId();
  const descriptionId = React.useId();

  if (!open) return null;

  return (
    <OverlayPortal>
      <div
        className="ui-dialog-layer"
        onMouseDown={(event) => {
          if (!isStatic && event.target === event.currentTarget) onClose();
        }}
      >
        <div
          aria-describedby={description ? descriptionId : undefined}
          aria-labelledby={titleId}
          aria-modal="true"
          className={cn("ui-dialog", dialogSizes[size], className)}
          ref={containerRef}
          role="dialog"
          tabIndex={-1}
        >
          <div className="ui-dialog-header">
            <div className="min-w-0">
              <h2 className="ui-dialog-title" id={titleId}>
                {title}
              </h2>
              {description ? (
                <p className="ui-dialog-description" id={descriptionId}>
                  {description}
                </p>
              ) : null}
            </div>
            {hideClose ? null : (
              <button
                aria-label="Close dialog"
                className="ui-icon-button -mr-1.5 -mt-1 size-7"
                onClick={onClose}
                type="button"
              >
                <X aria-hidden="true" className="size-4" />
              </button>
            )}
          </div>
          {children !== undefined && children !== null ? (
            <div className={cn("ui-dialog-body", bodyClassName)}>{children}</div>
          ) : null}
          {footer ? <div className="ui-dialog-footer">{footer}</div> : null}
        </div>
      </div>
    </OverlayPortal>
  );
}

/* -------------------------------------------------------------------------- */
/* Confirm dialog                                                             */
/* -------------------------------------------------------------------------- */

/**
 * Confirmation for a consequential action.
 *
 * The body always renders the description, so the consequence is stated where
 * the operator is looking when they click. For irreversible actions pass
 * `confirmPhrase` — the confirm button stays disabled until it is typed back.
 */
export function ForgeConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  description,
  /** Extra detail: what will be deleted, what will restart. */
  children,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  tone = "danger",
  loading = false,
  confirmPhrase,
}: {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  children?: React.ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: Extract<ForgeTone, "danger" | "warn" | "info">;
  loading?: boolean;
  confirmPhrase?: string;
}) {
  const [typed, setTyped] = React.useState("");
  const phraseId = React.useId();

  React.useEffect(() => {
    if (!open) setTyped("");
  }, [open]);

  const phraseSatisfied = !confirmPhrase || typed.trim() === confirmPhrase;

  return (
    <ForgeDialog
      onClose={onClose}
      open={open}
      size="sm"
      static={loading}
      title={title}
      footer={
        <>
          <ForgeButton block disabled={loading} onClick={onClose} variant="secondary">
            {cancelLabel}
          </ForgeButton>
          <ForgeButton
            block
            disabled={!phraseSatisfied}
            loading={loading}
            onClick={onConfirm}
            variant={tone === "danger" ? "danger" : "primary"}
          >
            {confirmLabel}
          </ForgeButton>
        </>
      }
    >
      <div className="space-y-3">
        <div className="flex items-start gap-2.5">
          <AlertTriangle
            aria-hidden="true"
            className={cn("mt-px size-4 shrink-0", toneStyles[tone].fg)}
          />
          <div className="min-w-0 space-y-2">
            {description ? <p className="text-text">{description}</p> : null}
            {children}
          </div>
        </div>
        {confirmPhrase ? (
          <div className="space-y-1.5">
            <label className="ui-label" htmlFor={phraseId}>
              Type <span className="ui-code-inline">{confirmPhrase}</span> to confirm
            </label>
            <input
              autoComplete="off"
              className="ui-input font-mono"
              data-autofocus
              id={phraseId}
              onChange={(event) => setTyped(event.target.value)}
              value={typed}
            />
          </div>
        ) : null}
      </div>
    </ForgeDialog>
  );
}

/* -------------------------------------------------------------------------- */
/* Drawer                                                                     */
/* -------------------------------------------------------------------------- */

export function ForgeDrawer({
  open,
  onClose,
  title,
  description,
  footer,
  width = "md",
  className,
  bodyClassName,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  footer?: React.ReactNode;
  width?: "sm" | "md" | "lg";
  className?: string;
  bodyClassName?: string;
  children?: React.ReactNode;
}) {
  const containerRef = useOverlayBehaviour(open, onClose);
  const titleId = React.useId();

  if (!open) return null;

  const widthClass = { sm: "max-w-md", md: "max-w-xl", lg: "max-w-3xl" }[width];

  return (
    <OverlayPortal>
      <div
        className="ui-drawer-layer"
        onMouseDown={(event) => {
          if (event.target === event.currentTarget) onClose();
        }}
      >
        <div
          aria-labelledby={titleId}
          aria-modal="true"
          className={cn("ui-drawer", widthClass, className)}
          ref={containerRef}
          role="dialog"
          tabIndex={-1}
        >
          <div className="ui-dialog-header">
            <div className="min-w-0">
              <h2 className="ui-dialog-title" id={titleId}>
                {title}
              </h2>
              {description ? <p className="ui-dialog-description">{description}</p> : null}
            </div>
            <button
              aria-label="Close panel"
              className="ui-icon-button -mr-1.5 -mt-1 size-7"
              onClick={onClose}
              type="button"
            >
              <X aria-hidden="true" className="size-4" />
            </button>
          </div>
          <div className={cn("min-h-0 flex-1 overflow-y-auto p-4 text-xs sm:p-5", bodyClassName)}>
            {children}
          </div>
          {footer ? <div className="ui-dialog-footer">{footer}</div> : null}
        </div>
      </div>
    </OverlayPortal>
  );
}

/* -------------------------------------------------------------------------- */
/* Dropdown menu                                                              */
/* -------------------------------------------------------------------------- */

export type ForgeMenuItem = {
  label: React.ReactNode;
  onSelect: () => void;
  icon?: React.ReactNode;
  tone?: "default" | "danger";
  disabled?: boolean;
};

export function ForgeDropdownMenu({
  trigger,
  items,
  align = "end",
  label,
  className,
}: {
  /** The control that opens the menu. Rendered inside a button wrapper. */
  trigger: React.ReactNode;
  items: readonly ForgeMenuItem[];
  align?: "start" | "end";
  label: string;
  className?: string;
}) {
  const [open, setOpen] = React.useState(false);
  const containerRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  return (
    <div className={cn("relative", className)} ref={containerRef}>
      <button
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label={label}
        className="ui-icon-button"
        onClick={() => setOpen((current) => !current)}
        type="button"
      >
        {trigger}
      </button>
      {open ? (
        <div
          className={cn(
            "ui-popover absolute top-[calc(100%+4px)] min-w-44",
            align === "end" ? "right-0" : "left-0"
          )}
          role="menu"
        >
          {items.map((item, index) => (
            <button
              className={cn("ui-menu-item", item.tone === "danger" && "ui-menu-item-danger")}
              disabled={item.disabled}
              key={index}
              onClick={() => {
                setOpen(false);
                item.onSelect();
              }}
              role="menuitem"
              type="button"
            >
              {item.icon}
              {item.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Tabs                                                                       */
/* -------------------------------------------------------------------------- */

export type ForgeTabItem<T extends string> = {
  value: T;
  label: React.ReactNode;
  icon?: React.ReactNode;
  /** Count or status chip shown after the label. */
  badge?: React.ReactNode;
  disabled?: boolean;
};

/**
 * Underlined tab bar with full arrow-key navigation (Left/Right/Home/End),
 * matching the WAI-ARIA tabs pattern.
 */
export function ForgeTabs<T extends string>({
  items,
  value,
  onChange,
  label,
  className,
}: {
  items: readonly ForgeTabItem<T>[];
  value: T;
  onChange: (value: T) => void;
  label: string;
  className?: string;
}) {
  const onKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const enabled = items.filter((item) => !item.disabled);
    if (enabled.length === 0) return;
    const currentIndex = enabled.findIndex((item) => item.value === value);
    let next: number | null = null;
    if (event.key === "ArrowRight") next = (currentIndex + 1) % enabled.length;
    else if (event.key === "ArrowLeft") next = (currentIndex - 1 + enabled.length) % enabled.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = enabled.length - 1;
    if (next === null) return;
    event.preventDefault();
    onChange(enabled[next].value);
  };

  return (
    <div aria-label={label} className={cn("ui-tablist", className)} onKeyDown={onKeyDown} role="tablist">
      {items.map((item) => {
        const selected = item.value === value;
        return (
          <button
            aria-selected={selected}
            className="ui-tab"
            disabled={item.disabled}
            key={item.value}
            onClick={() => onChange(item.value)}
            role="tab"
            tabIndex={selected ? 0 : -1}
            type="button"
          >
            {item.icon}
            {item.label}
            {item.badge}
          </button>
        );
      })}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Command palette                                                            */
/* -------------------------------------------------------------------------- */

export type ForgeCommandItem = {
  id: string;
  label: string;
  /** Secondary line — a path, a resource id, a description. */
  description?: string;
  group?: string;
  icon?: React.ReactNode;
  keywords?: string;
  onSelect: () => void;
};

/**
 * Global command / search surface. Filters on label, description and
 * keywords; Enter runs the highlighted item.
 */
export function ForgeCommandPalette({
  open,
  onClose,
  items,
  placeholder = "Search commands, servers, pages",
  emptyLabel = "No results",
}: {
  open: boolean;
  onClose: () => void;
  items: readonly ForgeCommandItem[];
  placeholder?: string;
  emptyLabel?: string;
}) {
  const containerRef = useOverlayBehaviour(open, onClose);
  const [query, setQuery] = React.useState("");
  const [activeIndex, setActiveIndex] = React.useState(0);
  const listboxId = React.useId();

  React.useEffect(() => {
    if (!open) {
      setQuery("");
      setActiveIndex(0);
    }
  }, [open]);

  const filtered = React.useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return items;
    return items.filter((item) =>
      `${item.label} ${item.description ?? ""} ${item.keywords ?? ""} ${item.group ?? ""}`
        .toLowerCase()
        .includes(needle)
    );
  }, [items, query]);

  React.useEffect(() => setActiveIndex(0), [query]);

  if (!open) return null;

  const run = (item: ForgeCommandItem) => {
    onClose();
    item.onSelect();
  };

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (filtered.length === 0) return;
      const delta = event.key === "ArrowDown" ? 1 : -1;
      setActiveIndex((current) => (current + delta + filtered.length) % filtered.length);
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      const item = filtered[activeIndex];
      if (item) run(item);
    }
  };

  let lastGroup: string | undefined;

  return (
    <OverlayPortal>
      <div
        className="ui-dialog-layer items-start pt-[10vh] sm:pt-[14vh]"
        onMouseDown={(event) => {
          if (event.target === event.currentTarget) onClose();
        }}
      >
        <div
          aria-label={placeholder}
          aria-modal="true"
          className="ui-dialog max-w-xl"
          ref={containerRef}
          role="dialog"
          tabIndex={-1}
        >
          <div className="flex items-center gap-2.5 border-b border-line px-3.5 py-2.5">
            <Search aria-hidden="true" className="size-4 shrink-0 text-text-muted" />
            <input
              aria-activedescendant={
                filtered[activeIndex] ? `${listboxId}-${filtered[activeIndex].id}` : undefined
              }
              aria-controls={listboxId}
              aria-expanded
              className="h-7 w-full border-0 bg-transparent p-0 text-sm text-text outline-none placeholder:text-text-muted"
              data-autofocus
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={onKeyDown}
              placeholder={placeholder}
              role="combobox"
              value={query}
            />
            <kbd className="ui-kbd">ESC</kbd>
          </div>
          <ul className="max-h-[52vh] min-h-0 overflow-y-auto p-1.5" id={listboxId} role="listbox">
            {filtered.length === 0 ? (
              <li className="px-3 py-8 text-center text-xs text-text-subtle">{emptyLabel}</li>
            ) : (
              filtered.map((item, index) => {
                const showGroup = item.group && item.group !== lastGroup;
                lastGroup = item.group;
                return (
                  <React.Fragment key={item.id}>
                    {showGroup ? (
                      <li className="px-2 pb-1 pt-2.5">
                        <p className="t-eyebrow">{item.group}</p>
                      </li>
                    ) : null}
                    <li
                      aria-selected={index === activeIndex}
                      id={`${listboxId}-${item.id}`}
                      role="option"
                    >
                      <button
                        className={cn("ui-menu-item", index === activeIndex && "bg-overlay")}
                        onClick={() => run(item)}
                        onMouseEnter={() => setActiveIndex(index)}
                        type="button"
                      >
                        {item.icon ? (
                          <span className="shrink-0 text-text-muted">{item.icon}</span>
                        ) : null}
                        <span className="min-w-0 flex-1 text-left">
                          <span className="block truncate">{item.label}</span>
                          {item.description ? (
                            <span className="block truncate text-meta text-text-muted">
                              {item.description}
                            </span>
                          ) : null}
                        </span>
                      </button>
                    </li>
                  </React.Fragment>
                );
              })
            )}
          </ul>
        </div>
      </div>
    </OverlayPortal>
  );
}
