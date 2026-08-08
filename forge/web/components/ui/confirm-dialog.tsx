"use client";

import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * ConfirmDialog — the standard confirmation / destructive-action primitive.
 *
 * Accessibility: role="dialog" + aria-modal, dynamic ids via useId (never a
 * hardcoded "dialog-title"), labelled by the title text, described by the
 * optional description. Focus is moved to the cancel button on open, Tab is
 * trapped between the dialog's focusables, and focus is restored to the
 * previously focused element on close.
 *
 * Dismissal: Escape key, backdrop click, or the cancel button. Clicks inside
 * the panel call stopPropagation so they never reach the backdrop handler.
 *
 * Props:
 * - open: visibility flag (render is no-op when false)
 * - onClose: called when the dialog is dismissed without confirming
 * - title: dialog heading (used as the aria-labelledby target)
 * - description?: supporting text (aria-describedby)
 * - confirmLabel?: confirm button label (default "Confirm")
 * - cancelLabel?: cancel button label (default "Cancel")
 * - danger?: when true the confirm button is solid brand red and the dialog
 *   reads as destructive; when false the confirm button is neutral
 * - onConfirm: called when the user confirms (dialog closes itself first)
 *
 * Buttons use the .ui-button primitives (min-h-10) and all colors are tokens.
 *
 * Prefer the useConfirm() hook below — it wraps state + promise resolution:
 *
 *   const [confirm, renderConfirm] = useConfirm();
 *   if (await confirm({ title: "Delete server?", description: "...", danger: true, confirmLabel: "Delete" })) {
 *     deleteServer();
 *   }
 *   return <>{renderConfirm()}</>;
 */

interface ConfirmDialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
}

function getFocusable(root: HTMLElement | null): HTMLElement[] {
  if (!root) return [];
  return Array.from(
    root.querySelectorAll<HTMLElement>(
      'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  );
}

export function ConfirmDialog({
  open,
  onClose,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger = false,
  onConfirm,
}: ConfirmDialogProps) {
  const titleId = useId();
  const descriptionId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;

    previouslyFocusedRef.current = document.activeElement as HTMLElement | null;
    document.body.style.overflow = "hidden";

    const panel = panelRef.current;
    const focusables = getFocusable(panel);
    (focusables[0] ?? panel)?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.stopPropagation();
        onClose();
        return;
      }
      if (event.key !== "Tab") return;
      const elements = getFocusable(panelRef.current);
      if (elements.length === 0) {
        event.preventDefault();
        return;
      }
      const first = elements[0];
      const last = elements[elements.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.body.style.overflow = "";
      document.removeEventListener("keydown", handleKeyDown);
      previouslyFocusedRef.current?.focus?.();
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="ui-dialog-layer"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descriptionId : undefined}
        className="ui-dialog"
        onClick={(event) => event.stopPropagation()}
      >
        <h2 id={titleId} className="text-lg font-semibold text-[var(--text)]">
          {title}
        </h2>
        {description ? (
          <p id={descriptionId} className="mt-1.5 text-sm leading-6 text-[var(--text-subtle)]">
            {description}
          </p>
        ) : null}
        <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <button type="button" className="ui-button ui-button-secondary" onClick={onClose}>
            {cancelLabel}
          </button>
          <button
            type="button"
            className={cn("ui-button", danger ? "ui-button-primary" : "ui-button-secondary")}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

export type { ConfirmDialogProps };

type ConfirmOptions = {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
};

/**
 * useConfirm — promise-based confirmation helper.
 *
 * Returns [confirm, render]:
 * - confirm(options): opens the dialog and resolves to true if the user
 *   confirms, false if they cancel / dismiss. Call it from an event handler.
 * - render(): render the dialog once in your tree (a single slot per hook
 *   instance). It returns null until a confirm() is in flight.
 *
 * Only one pending confirmation is supported per hook instance.
 */
export function useConfirm(): [(options: ConfirmOptions) => Promise<boolean>, () => ReactNode] {
  const [options, setOptions] = useState<ConfirmOptions | null>(null);
  const resolverRef = useRef<((confirmed: boolean) => void) | null>(null);

  const confirm = useCallback((next: ConfirmOptions) => {
    setOptions(next);
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
    });
  }, []);

  const settle = useCallback((confirmed: boolean) => {
    resolverRef.current?.(confirmed);
    resolverRef.current = null;
    setOptions(null);
  }, []);

  const render = useCallback(() => {
    if (!options) return null;
    return (
      <ConfirmDialog
        open
        onClose={() => settle(false)}
        onConfirm={() => settle(true)}
        {...options}
      />
    );
  }, [options, settle]);

  return [confirm, render];
}
