"use client";

import { CheckCircle2, Info, LoaderCircle, TriangleAlert, X } from "lucide-react";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

type ToastTone = "success" | "error" | "warning" | "info" | "loading";
type Toast = { id: number; title: string; message?: string; tone: ToastTone };
type ToastContextValue = { toast: (input: Omit<Toast, "id">) => number; dismiss: (id: number) => void };

const ToastContext = createContext<ToastContextValue>({ toast: () => 0, dismiss: () => undefined });
let nextToastId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const timeoutsRef = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map());
  const dismiss = useCallback((id: number) => {
    setToasts((items) => items.filter((item) => item.id !== id));
    const tid = timeoutsRef.current.get(id);
    if (tid) { window.clearTimeout(tid); timeoutsRef.current.delete(id); }
  }, []);
  const toast = useCallback((input: Omit<Toast, "id">) => {
    const id = nextToastId++;
    setToasts((items) => [...items, { ...input, id }].slice(-3));
    if (input.tone !== "loading") {
      const tid = window.setTimeout(() => dismiss(id), input.tone === "error" ? 7000 : 4500);
      timeoutsRef.current.set(id, tid);
    }
    return id;
  }, [dismiss]);
  const value = useMemo(() => ({ toast, dismiss }), [dismiss, toast]);
  useEffect(() => () => { timeoutsRef.current.forEach((tid) => window.clearTimeout(tid)); timeoutsRef.current.clear(); }, []);

  return <ToastContext.Provider value={value}>{children}<div aria-atomic="true" aria-label="Notifications" aria-live="polite" className="ui-toast-region">{toasts.map((item) => {
    const Icon = item.tone === "success" ? CheckCircle2 : item.tone === "error" || item.tone === "warning" ? TriangleAlert : item.tone === "loading" ? LoaderCircle : Info;
    return <div className={`ui-toast ui-toast-${item.tone}`} key={item.id} role={item.tone === "error" || item.tone === "warning" ? "alert" : "status"}><Icon aria-hidden="true" className={`mt-0.5 h-5 w-5 shrink-0 ${item.tone === "loading" ? "animate-spin" : ""}`} /><div className="min-w-0 flex-1"><p className="text-sm font-semibold text-slate-100">{item.title}</p>{item.message ? <p className="mt-0.5 text-xs leading-5 text-slate-400">{item.message}</p> : null}</div><button aria-label="Dismiss notification" className="ui-icon-button -mr-1 -mt-1" onClick={() => dismiss(item.id)} type="button"><X className="h-4 w-4" /></button></div>;
  })}</div></ToastContext.Provider>;
}

export function useToast() { return useContext(ToastContext); }
