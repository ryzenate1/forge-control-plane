'use client';

import type { ReactNode } from 'react';
import { useState } from 'react';
import { Sidebar, type SidebarNavItem } from './Sidebar';
import { TopBar } from './TopBar';

interface AdminLayoutProps {
  children: ReactNode;
  title?: string;
  /** Navigation entries rendered by the sidebar. Supplied by the host app. */
  navItems: SidebarNavItem[];
  sidebarTitle?: string;
}

export function AdminLayout({ children, title, navItems, sidebarTitle }: AdminLayoutProps) {
  const [sidebarOpen, setSidebarOpen] = useState(true);

  return (
    <div className="flex h-screen bg-canvas text-text">
      <Sidebar
        open={sidebarOpen}
        onToggle={() => setSidebarOpen(!sidebarOpen)}
        items={navItems}
        title={sidebarTitle}
      />
      <div className="flex flex-1 flex-col overflow-hidden">
        <TopBar title={title} onMenuClick={() => setSidebarOpen(!sidebarOpen)} />
        <main className="flex-1 overflow-y-auto p-6">{children}</main>
      </div>
    </div>
  );
}
