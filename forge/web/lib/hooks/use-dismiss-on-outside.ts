"use client";

import { useEffect } from "react";
import type { RefObject } from "react";

/**
 * Closes a popover when the user clicks outside it or presses Escape.
 *
 * Shared by every navigation popover (scope switcher, notifications, user
 * menu) so keyboard dismissal behaves identically everywhere — a menu that
 * traps Escape is a menu keyboard users cannot leave.
 */
export function useDismissOnOutside(
  ref: RefObject<HTMLElement | null>,
  isOpen: boolean,
  close: () => void,
) {
  useEffect(() => {
    if (!isOpen) return;
    const onPointerDown = (event: PointerEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) close();
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [ref, isOpen, close]);
}
