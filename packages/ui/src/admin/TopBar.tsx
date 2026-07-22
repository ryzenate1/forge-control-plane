'use client';

interface TopBarProps {
  title?: string;
  onMenuClick: () => void;
}

export function TopBar({ title, onMenuClick }: TopBarProps) {
  return (
    <header className="border-b border-line bg-surface px-6 py-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <button
            onClick={onMenuClick}
            className="rounded p-1 text-text-subtle hover:bg-overlay hover:text-text lg:hidden"
            aria-label="Toggle menu"
          >
            <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M4 6h16M4 12h16M4 18h16"
              />
            </svg>
          </button>
          {title && <h1 className="t-section text-text">{title}</h1>}
        </div>
        <div className="flex items-center gap-3">
          {/* Search, notifications, user menu placeholders */}
        </div>
      </div>
    </header>
  );
}
