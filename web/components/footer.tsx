import Link from "next/link";

export default function Footer() {
  return (
    <footer className="border-t border-line bg-paper px-6 py-4">
      <div className="mx-auto flex max-w-4xl items-center justify-between text-xs text-muted">
        <span>Made by Riyaz with love</span>
        <div className="flex gap-4">
          <Link href="/docs/introduction" className="px-2 py-1 hover:text-ink transition-colors">Docs</Link>
          <Link href="/features" className="px-2 py-1 hover:text-ink transition-colors">Features</Link>
          <Link href="/manual" className="px-2 py-1 hover:text-ink transition-colors">Manual</Link>
          <a href="https://github.com/ryzenate1/forge-control-plane" target="_blank" rel="noopener noreferrer" className="px-2 py-1 hover:text-ink transition-colors" aria-label="GitHub (opens in new tab)">GitHub ↗</a>
        </div>
      </div>
    </footer>
  );
}
