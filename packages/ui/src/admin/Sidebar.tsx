'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { cn } from '../../lib/utils';

interface SidebarProps {
  open: boolean;
  onToggle: () => void;
}

interface NavItem {
  label: string;
  href: string;
  icon: string;
  children?: NavItem[];
}

const navItems: NavItem[] = [
  { label: 'Dashboard', href: '/admin', icon: 'LayoutDashboard' },
  { label: 'Nodes', href: '/admin/nodes', icon: 'Server' },
  { label: 'Servers', href: '/admin/servers', icon: 'Gamepad2' },
  { label: 'Users', href: '/admin/users', icon: 'Users' },
  { label: 'Locations', href: '/admin/locations', icon: 'MapPin' },
  { label: 'Nests', href: '/admin/nests', icon: 'Hierarchy' },
  { label: 'Eggs', href: '/admin/eggs', icon: 'Egg' },
  { label: 'Database Hosts', href: '/admin/database-hosts', icon: 'Database' },
  { label: 'Mounts', href: '/admin/mounts', icon: 'FolderKanban' },
  { label: 'Plugins', href: '/admin/plugins', icon: 'Puzzle' },
  { label: 'Social Login', href: '/admin/social', icon: 'LogIn' },
  { label: 'Activity Log', href: '/admin/activity', icon: 'Activity' },
  { label: 'Settings', href: '/admin/settings', icon: 'Settings' },
];

export function Sidebar({ open, onToggle }: SidebarProps) {
  const pathname = usePathname();

  return (
    <aside className={cn(
      'bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 transition-all duration-300',
      open ? 'w-64' : 'w-16'
    )}>
      <div className="flex items-center justify-between p-4 border-b border-gray-200 dark:border-gray-700">
        {open && <span className="font-bold text-lg">Admin Panel</span>}
        <button onClick={onToggle} className="p-1 rounded hover:bg-gray-100 dark:hover:bg-gray-700">
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={open ? "M11 19l-7-7 7-7" : "M13 5l7 7-7 7"} />
          </svg>
        </button>
      </div>
      <nav className="p-2 space-y-1">
        {navItems.map((item) => {
          const isActive = pathname === item.href || pathname?.startsWith(item.href + '/');
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                'flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors',
                isActive
                  ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300'
                  : 'text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-700'
              )}
              title={!open ? item.label : undefined}
            >
              <span className="w-5 h-5 flex-shrink-0">{/* Icon placeholder */}</span>
              {open && <span>{item.label}</span>}
            </Link>
          );
        })}
      </nav>
    </aside>
  );
}
