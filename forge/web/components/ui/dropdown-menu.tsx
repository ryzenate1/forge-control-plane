"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

interface DropdownMenuContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
}

const DropdownMenuContext = React.createContext<DropdownMenuContextValue | null>(null);

function useDropdownMenuContext() {
  const context = React.useContext(DropdownMenuContext);
  if (!context) throw new Error("DropdownMenu components must be used within a DropdownMenu");
  return context;
}

function DropdownMenu({ children }: { children: React.ReactNode }) {
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
    <DropdownMenuContext.Provider value={{ open, setOpen }}>
      <div ref={containerRef} className="relative inline-block">{children}</div>
    </DropdownMenuContext.Provider>
  );
}

function DropdownMenuTrigger({ children, asChild }: {
  children: React.ReactNode;
  asChild?: boolean;
}) {
  const { open, setOpen } = useDropdownMenuContext();
  if (asChild && React.isValidElement(children)) {
    const child = children as React.ReactElement<{ onClick?: (e: React.MouseEvent) => void }>;
    return React.cloneElement(child, {
      onClick: (e: React.MouseEvent) => {
        setOpen(!open);
        child.props.onClick?.(e);
      },
    });
  }
  return (
    <button type="button" onClick={() => setOpen(!open)}>
      {children}
    </button>
  );
}

function DropdownMenuContent({ className, children, align = "start", ...props }: {
  className?: string;
  children?: React.ReactNode;
  align?: "start" | "end";
}) {
  const { open } = useDropdownMenuContext();
  if (!open) return null;
  return (
    <div
      className={cn(
        "absolute z-50 min-w-[8rem] overflow-hidden rounded-lg border border-white/[0.12] bg-surface-elevated shadow-card p-1",
        align === "end" ? "right-0" : "left-0",
        "mt-1",
        className
      )}
      {...props}
    >
      {children}
    </div>
  );
}

function DropdownMenuLabel({ className, children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("px-2 py-1.5 text-xs font-semibold text-slate-400", className)} {...props}>
      {children}
    </div>
  );
}

function DropdownMenuSeparator({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("-mx-1 my-1 h-px bg-white/[0.06]", className)} {...props} />;
}

function DropdownMenuItem({ className, children, onClick, ...props }: React.HTMLAttributes<HTMLDivElement> & { onClick?: () => void }) {
  const { setOpen } = useDropdownMenuContext();
  return (
    <div
      className={cn(
        "relative flex cursor-default select-none items-center rounded-md px-2 py-1.5 text-sm text-slate-300 outline-none hover:bg-white/[0.06] hover:text-slate-100 focus:bg-white/[0.06] focus:text-slate-100 data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
        className
      )}
      onClick={() => { onClick?.(); setOpen(false); }}
      role="menuitem"
      {...props}
    >
      {children}
    </div>
  );
}

export { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuItem };
