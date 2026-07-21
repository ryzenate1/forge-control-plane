"use client";

import * as React from "react";
import { cn } from "@/lib/utils";
import { ChevronDown } from "lucide-react";

interface SelectContextValue {
  value?: string;
  onValueChange?: (value: string) => void;
  disabled?: boolean;
  open: boolean;
  setOpen: (open: boolean) => void;
}

const SelectContext = React.createContext<SelectContextValue | null>(null);

function useSelectContext() {
  const context = React.useContext(SelectContext);
  if (!context) throw new Error("Select components must be used within a Select");
  return context;
}

function Select({ value, onValueChange, disabled, children }: {
  value?: string;
  onValueChange?: (value: string) => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = React.useState(false);
  const containerRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  return (
    <SelectContext.Provider value={{ value, onValueChange, disabled, open, setOpen }}>
      <div ref={containerRef} className="relative">{children}</div>
    </SelectContext.Provider>
  );
}

function SelectTrigger({ className, children }: { className?: string; children?: React.ReactNode }) {
  const { disabled, open, setOpen } = useSelectContext();
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={() => setOpen(!open)}
      className={cn(
        "flex h-10 w-full items-center justify-between rounded-lg border border-white/[0.12] bg-surface-input px-3 py-2 text-sm text-slate-100 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-brand/50 disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
    >
      {children}
      <ChevronDown className="h-4 w-4 text-slate-400 shrink-0" />
    </button>
  );
}

function SelectValue({ placeholder }: { placeholder?: string }) {
  const { value } = useSelectContext();
  return <span className={value ? "text-slate-100" : "text-slate-500"}>{value || placeholder}</span>;
}

function SelectContent({ className, children }: { className?: string; children?: React.ReactNode }) {
  const { open } = useSelectContext();
  if (!open) return null;
  return (
    <div className={cn(
      "absolute z-50 mt-1 w-full rounded-lg border border-white/[0.12] bg-surface-elevated shadow-lg",
      className
    )}>
      <div className="p-1">{children}</div>
    </div>
  );
}

function SelectItem({ value, children, className }: { value: string; children?: React.ReactNode; className?: string }) {
  const { value: selectedValue, onValueChange, setOpen } = useSelectContext();
  const isSelected = selectedValue === value;
  return (
    <div
      role="option"
      aria-selected={isSelected}
      onClick={() => {
        onValueChange?.(value);
        setOpen(false);
      }}
      className={cn(
        "relative flex cursor-default select-none items-center rounded-md px-3 py-2 text-sm text-slate-300 hover:bg-white/[0.06] hover:text-slate-100",
        isSelected && "text-slate-100 bg-white/[0.06]",
        className
      )}
    >
      {children}
    </div>
  );
}

export { Select, SelectTrigger, SelectValue, SelectContent, SelectItem };
