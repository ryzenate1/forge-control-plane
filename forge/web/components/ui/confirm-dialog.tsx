"use client";

import { useCallback, useRef, useState, type ReactNode } from "react";
import { ForgeConfirmDialog } from "@/components/ui/forge/overlay";

/**
 * ConfirmDialog — the standard confirmation / destructive-action primitive.
 *
 * A thin adapter over {@link ForgeConfirmDialog}, which owns the one focus
 * trap, scroll lock and Escape/backdrop contract in the app. The props below
 * are unchanged because ~25 admin pages pass them through {@link useConfirm}.
 *
 * This file used to carry a second, hand-rolled trap. Collapsing it onto the
 * shared overlay fixed three defects it had accumulated:
 *
 *  - Its focus effect listed `onClose` as a dependency. Every call site passes
 *    an inline arrow, so the trap was rebuilt on each render of the host page,
 *    restoring focus to the trigger and then stealing it back.
 *  - The confirm button rendered `ui-button-secondary` unless `danger` was
 *    set, making it visually identical to Cancel — the operator could not tell
 *    which button was the affirmative one.
 *  - Its doc promised a red confirm button for destructive actions and then
 *    applied the brand-primary class instead.
 *
 * Usage — prefer the {@link useConfirm} hook, which wraps state and promise
 * resolution:
 *
 *   const [confirm, renderConfirm] = useConfirm();
 *   if (await confirm({ title: "Delete server?", description: "…", danger: true, confirmLabel: "Delete" })) {
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
  /** Destructive actions get the danger tone and a red confirm button. */
  danger?: boolean;
  onConfirm: () => void;
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
  return (
    <ForgeConfirmDialog
      cancelLabel={cancelLabel}
      confirmLabel={confirmLabel}
      description={description}
      onClose={onClose}
      onConfirm={onConfirm}
      open={open}
      title={title}
      tone={danger ? "danger" : "info"}
    />
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
    // A second confirm() while one is pending rejects the first: only one
    // pending confirmation is supported per hook instance, and leaving the
    // earlier promise hanging would leak an unresolved await.
    resolverRef.current?.(false);
    resolverRef.current = null;
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
