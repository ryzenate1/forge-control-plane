"use client";

import { useToast } from "@/components/ui/toast";

type ToastAction = "created" | "updated" | "deleted" | "restored" | "deployed" | "started" | "stopped" | "rollback" | "issuing";

const actionLabels: Record<ToastAction, string> = {
  created: "created",
  updated: "updated",
  deleted: "deleted",
  restored: "restored",
  deployed: "deployed",
  started: "started",
  stopped: "stopped",
  rollback: "rolled back",
  issuing: "issuance started",
};

function resourceLabel(resource: string): string {
  return resource.charAt(0).toUpperCase() + resource.slice(1);
}

export function useAppToast() {
  const { toast, dismiss } = useToast();

  function success(resource: string, action: ToastAction) {
    const a = actionLabels[action];
    const title = `${resourceLabel(resource)} ${a} successfully`;
    const id = toast({ tone: "success", title });
    return { id, dismiss: () => dismiss(id) };
  }

  function error(resource: string, message: string) {
    const id = toast({
      tone: "error",
      title: `${resourceLabel(resource)} error`,
      message,
    });
    return { id, dismiss: () => dismiss(id) };
  }

  function info(title: string, message?: string) {
    const id = toast({ tone: "info", title, message });
    return { id, dismiss: () => dismiss(id) };
  }

  function warning(title: string, message?: string) {
    const id = toast({ tone: "warning", title, message });
    return { id, dismiss: () => dismiss(id) };
  }

  function persistent(title: string, message: string) {
    const id = toast({ tone: "loading", title, message });
    return {
      id,
      dismiss: () => dismiss(id),
      update: (nextTitle: string, nextTone: "success" | "error" = "success") => {
        dismiss(id);
        toast({ tone: nextTone, title: nextTitle, message });
      },
    };
  }

  return { toast, dismiss, success, error, info, warning, persistent };
}
