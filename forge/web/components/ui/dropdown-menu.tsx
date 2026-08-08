"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

interface DropdownMenuContextValue {
  open: boolean;
  setOpen: (open: boolean) => void;
  menuId: string;
  containerRef: React.RefObject<HTMLDivElement | null>;
}

const DropdownMenuContext = React.createContext<DropdownMenuContextValue | null>(null);

function useDropdownMenuContext() {
  const context = React.useContext(DropdownMenuContext);
  if (!context) throw new Error("DropdownMenu components must be used within a DropdownMenu");
  return context;
}

function getMenuItems(menuId: string): HTMLElement[] {
  const menu = document.getElementById(menuId);
  if (!menu) return [];
  return Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]'));
}

function focusMenuItem(menuId: string, index: number) {
  const items = getMenuItems(menuId);
  const target = items[index];
  if (target) target.focus();
  else document.getElementById(menuId)?.focus();
}

function focusAdjacentItem(menuId: string, direction: 1 | -1) {
  const items = getMenuItems(menuId);
  if (items.length === 0) return;
  const currentIndex = items.indexOf(document.activeElement as HTMLElement);
  const nextIndex = Math.min(Math.max(currentIndex + direction, 0), items.length - 1);
  items[nextIndex]?.focus();
}

function openMenu(setOpen: (open: boolean) => void, menuId: string, focusLast = false) {
  setOpen(true);
  window.setTimeout(() => {
    const items = getMenuItems(menuId);
    if (focusLast) items[items.length - 1]?.focus();
    else items[0]?.focus();
  }, 0);
}

function DropdownMenu({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false);
  const containerRef = React.useRef<HTMLDivElement>(null);
  const menuId = React.useId();

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
    <DropdownMenuContext.Provider value={{ open, setOpen, menuId, containerRef }}>
      <div ref={containerRef} className="relative inline-block">{children}</div>
    </DropdownMenuContext.Provider>
  );
}

function DropdownMenuTrigger({ children, asChild }: {
  children: React.ReactNode;
  asChild?: boolean;
}) {
  const { open, setOpen, menuId } = useDropdownMenuContext();

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown" || e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      if (open) focusMenuItem(menuId, 0);
      else openMenu(setOpen, menuId);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (open) focusMenuItem(menuId, -1);
      else openMenu(setOpen, menuId, true);
    }
  };

  if (asChild && React.isValidElement(children)) {
    const child = children as React.ReactElement<{ onClick?: (e: React.MouseEvent) => void }>;
    return React.cloneElement(child as React.ReactElement<Record<string, unknown>>, {
      "aria-controls": menuId,
      "aria-expanded": open,
      "aria-haspopup": "menu",
      onKeyDown: handleKeyDown,
      onClick: (e: React.MouseEvent) => {
        setOpen(!open);
        child.props.onClick?.(e);
      },
    });
  }
  return (
    <button type="button" aria-controls={menuId} aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen(!open)} onKeyDown={handleKeyDown}>
      {children}
    </button>
  );
}

function DropdownMenuContent({ className, children, align = "start", ...props }: {
  className?: string;
  children?: React.ReactNode;
  align?: "start" | "end";
}) {
  const { open, setOpen, menuId, containerRef } = useDropdownMenuContext();
  if (!open) return null;

  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        focusAdjacentItem(menuId, 1);
        break;
      case "ArrowUp":
        e.preventDefault();
        focusAdjacentItem(menuId, -1);
        break;
      case "Home":
        e.preventDefault();
        focusMenuItem(menuId, 0);
        break;
      case "End":
        e.preventDefault();
        focusMenuItem(menuId, -1);
        break;
      case "Escape":
        e.preventDefault();
        setOpen(false);
        containerRef.current?.querySelector<HTMLElement>('[aria-haspopup="menu"]')?.focus();
        break;
      case "Tab":
        setOpen(false);
        break;
    }
  };

  return (
    <div
      id={menuId}
      role="menu"
      aria-orientation="vertical"
      onKeyDown={handleKeyDown}
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
  return <div role="separator" className={cn("-mx-1 my-1 h-px bg-white/[0.06]", className)} {...props} />;
}

function DropdownMenuItem({ className, children, onClick, disabled, ...props }: React.HTMLAttributes<HTMLDivElement> & { onClick?: () => void; disabled?: boolean }) {
  const { setOpen } = useDropdownMenuContext();
  return (
    <div
      aria-disabled={disabled || undefined}
      data-disabled={disabled || undefined}
      className={cn(
        "relative flex cursor-default select-none items-center rounded-md px-2 py-1.5 text-sm text-slate-300 outline-none hover:bg-white/[0.06] hover:text-slate-100 focus:bg-white/[0.06] focus:text-slate-100 focus-visible:ring-2 focus-visible:ring-brand/50 data-[disabled]:pointer-events-none data-[disabled]:opacity-50",
        className
      )}
      onClick={() => { if (disabled) return; onClick?.(); setOpen(false); }}
      onKeyDown={(e) => {
        if (disabled) return;
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick?.();
          setOpen(false);
        }
      }}
      role="menuitem"
      tabIndex={-1}
      {...props}
    >
      {children}
    </div>
  );
}

DropdownMenu.displayName = "DropdownMenu";
DropdownMenuTrigger.displayName = "DropdownMenuTrigger";
DropdownMenuContent.displayName = "DropdownMenuContent";
DropdownMenuLabel.displayName = "DropdownMenuLabel";
DropdownMenuSeparator.displayName = "DropdownMenuSeparator";
DropdownMenuItem.displayName = "DropdownMenuItem";
export { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuItem };
