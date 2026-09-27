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
 */
export function Toaster() {
  return null;
}

export const toast = {
  success: (message: ToastInput) => pushToast({ ...toInput(message), tone: "success" }),
  error: (message: ToastInput) => pushToast({ ...toInput(message), tone: "error" }),
  info: (message: ToastInput) => pushToast({ ...toInput(message), tone: "info" }),
  warning: (message: ToastInput) => pushToast({ ...toInput(message), tone: "warning" }),
  loading: (message: ToastInput) => pushToast({ ...toInput(message), tone: "loading" }),
};
