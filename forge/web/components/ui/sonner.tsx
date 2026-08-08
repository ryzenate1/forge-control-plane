"use client";

import { pushToast } from "./toast";

export function Toaster() {
  return null;
}

export const toast = {
  success: (message: string) => pushToast({ title: message, tone: "success" }),
  error: (message: string) => pushToast({ title: message, tone: "error" }),
  info: (message: string) => pushToast({ title: message, tone: "info" }),
  warning: (message: string) => pushToast({ title: message, tone: "warning" }),
};
