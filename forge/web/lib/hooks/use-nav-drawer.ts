"use client";

import { useEffect } from "react";
import type { RefObject } from "react";

const FOCUSABLE = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(", ");

/**
 * Modal behaviour for a mobile navigation drawer: lock background scroll, move
 * focus into the drawer, close on Escape, and keep Tab inside it.
 *
 * Every Forge nav surface (admin, console, server) opens the same kind of
 * off-canvas drawer, and each had grown its own partial version — the admin
 * one trapped focus, the console and server ones did nothing, so a keyboard
 * user tabbed straight into the page behind the overlay with no way to close
 * it. One hook, one behaviour.
 */
export function useNavDrawer({
  open,
  close,
  containerRef,
  initialFocusRef,
}: {
  open: boolean;
  close: () => void;
  containerRef: RefObject<HTMLElement | null>;
  /** Element to focus when the drawer opens. Defaults to the first focusable child. */
  initialFocusRef?: RefObject<HTMLElement | null>;
}) {
  useEffect(() => {
    if (!open) {
      document.body.style.overflow = "";
      return;
    }

    const previouslyFocused = document.activeElement as HTMLElement | null;
    document.body.style.overflow = "hidden";

    const target =
      initialFocusRef?.current ?? containerRef.current?.querySelector<HTMLElement>(FOCUSABLE) ?? null;
    target?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        close();
        return;
      }
      if (event.key !== "Tab" || !containerRef.current) return;
      const focusable = Array.from(containerRef.current.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
        (element) => element.offsetParent !== null || element === document.activeElement,
      );
      if (focusable.length === 0) return;
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

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.body.style.overflow = "";
      previouslyFocused?.focus?.();
    };
  }, [open, close, containerRef, initialFocusRef]);
}

/**
 * Roving-tabindex arrow-key traversal for a vertical nav list.
 *
 * Attach the returned handler to the list container's `onKeyDown`. Items must
 * carry `data-nav-item`. Sidebars previously required one Tab press per row —
 * up to 40 of them to reach the bottom of the admin nav.
 */
export function navListKeyDown(event: React.KeyboardEvent<HTMLElement>) {
  const keys = ["ArrowDown", "ArrowUp", "Home", "End"];
  if (!keys.includes(event.key)) return;

  const container = event.currentTarget;
  const items = Array.from(container.querySelectorAll<HTMLElement>("[data-nav-item]")).filter(
    (element) => element.offsetParent !== null,
  );
  if (items.length === 0) return;

  const index = items.indexOf(document.activeElement as HTMLElement);
  let next: number;
  switch (event.key) {
    case "ArrowDown":
      next = index < 0 ? 0 : (index + 1) % items.length;
      break;
    case "ArrowUp":
      next = index < 0 ? items.length - 1 : (index - 1 + items.length) % items.length;
      break;
    case "Home":
      next = 0;
      break;
    default:
      next = items.length - 1;
  }
  event.preventDefault();
  items[next]?.focus();
}
