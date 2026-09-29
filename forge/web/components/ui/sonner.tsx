"use client";

import { pushToast } from "./toast";

type ToastInput = string | { title: string; message?: string };

function toInput(input: ToastInput): { title: string; message?: string } {
  return typeof input === "string" ? { title: input } : input;
}

/**
 * Module-level toast API — same store as `useToast().toast`, safe to call
 * outside React (loaders, query callbacks). `pushToast` queues until the
 * provider mounts, so early calls are flushed, not dropped.
 *
 * NOTE: there is intentionally no `<Toaster/>` component here. The toast
 * region is rendered once by `ToastProvider` (`components/ui/toast.tsx`),
 * which owns the toast state — a second region component would render empty.
 * Mount `<ToastProvider>` (see `components/providers.tsx`) and call `toast.*`.
 */

export const toast = {
  success: (message: ToastInput) => pushToast({ ...toInput(message), tone: "success" }),
  error: (message: ToastInput) => pushToast({ ...toInput(message), tone: "error" }),
  info: (message: ToastInput) => pushToast({ ...toInput(message), tone: "info" }),
  warning: (message: ToastInput) => pushToast({ ...toInput(message), tone: "warning" }),
  loading: (message: ToastInput) => pushToast({ ...toInput(message), tone: "loading" }),
};
