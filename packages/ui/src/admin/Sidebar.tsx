'use client';

import type { ReactNode } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '../lib/utils';

export interface SidebarNavItem {
  label: string;
  href: string;
  /** Rendered in the icon slot. Navigation entries are supplied by the host
   * app (e.g. the web admin registry) — this package ships no hardcoded nav. */
  icon?: ReactNode;
}

interface SidebarProps {
  open: boolean;
  onToggle: () => void;
  /** Navigation entries. Required: the sidebar renders exactly what it is given. */
  items: SidebarNavItem[];
  title?: string;
}

export function Sidebar({ open, onToggle, items, title = 'Admin Panel' }: SidebarProps) {
  const pathname = usePathname();

  return (
    <aside
      className={cn(
        'border-r border-line bg-surface transition-all duration-300',
        open ? 'w-64' : 'w-16',
      )}
    >
      <div className="flex items-center justify-between border-b border-line p-4">
        {open && <span className="text-lg font-bold text-text">{title}</span>}
        <button
          onClick={onToggle}
          className="rounded p-1 text-text-subtle hover:bg-overlay hover:text-text"
          aria-label={open ? 'Collapse sidebar' : 'Expand sidebar'}
        >
          <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d={open ? 'M11 19l-7-7 7-7' : 'M13 5l7 7-7 7'}
            />
          </svg>
        </button>
      </div>
      <nav className="space-y-1 p-2">
        {items.map((item) => {
          const isActive = pathname === item.href || pathname?.startsWith(item.href + '/');
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                'flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors',
                isActive
                  ? 'bg-brand-subtle text-brand'
                  : 'text-text-subtle hover:bg-overlay hover:text-text',
              )}
              title={!open ? item.label : undefined}
            >
              <span className="flex h-5 w-5 shrink-0 items-center justify-center">{item.icon}</span>
              {open && <span>{item.label}</span>}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
