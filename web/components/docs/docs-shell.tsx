"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { docGroups, docs } from "@/lib/docs";

export function DocsShell({ children, slug }: { children: React.ReactNode; slug?: string }) {
  const pathname = usePathname();
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const deferredQuery = useDeferredValue(query);
  const results = useMemo(() => docs.filter((page) => `${page.title} ${page.summary}`.toLowerCase().includes(deferredQuery.toLowerCase())), [deferredQuery]);

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      } else if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        document.getElementById("docs-search")?.focus();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  useEffect(() => {
    if (!open) return;
    const sidebar = document.getElementById("docs-sidebar");
    if (!sidebar) return;
    const focusable = sidebar.querySelectorAll<HTMLElement>("a, button, input, [tabindex]");
    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    function trap(e: KeyboardEvent) {
      if (e.key !== "Tab") return;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last?.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first?.focus();
      }
    }

    first?.focus();
    window.addEventListener("keydown", trap);
    return () => window.removeEventListener("keydown", trap);
  }, [open]);

  return <div className="docs-app">
    <header className="docs-topbar">
      <Link className="docs-brand" href="/"><span>F</span> Forge <i>Docs</i></Link>
      <div className="docs-search"><input id="docs-search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search documentation" aria-label="Search documentation" /> <kbd>⌘ K</kbd>
        {query && <>{results.length > 0 && <div className="docs-results" role="listbox" aria-label="Search results">{results.map((page) => <Link onClick={() => setQuery("")} href={`/docs/${page.slug}`} key={page.slug} role="option">{page.title}<small>{page.group}</small></Link>)}</div>}{results.length === 0 && <div className="docs-results"><p>No matching guide.</p></div>}</>}
      </div>
      <a className="docs-repo" href="https://github.com/ryzenate1/forge-control-plane" target="_blank" rel="noopener noreferrer" aria-label="Repository (opens in new tab)">Repository ↗</a>
      <Link className="docs-features-link" href="/features">Features</Link>
      <Link className="docs-manual-link" href="/manual">User manual</Link>
      <button className="docs-menu" type="button" id="menu-toggle" aria-controls="docs-sidebar" aria-expanded={open} onClick={() => setOpen(!open)}>Menu</button>
    </header>
    <div className="docs-frame" id="main-content">
      <aside id="docs-sidebar" className={`docs-sidebar ${open ? "is-open" : ""}`}>
        <p className="docs-version"><b /> Operator manual <span>current</span></p>
        {docGroups.map((group) => <nav key={group} aria-label={group}><h2>{group}</h2>{docs.filter((page) => page.group === group).map((page) => <Link aria-current={page.slug === slug ? "page" : undefined} onClick={() => setOpen(false)} key={page.slug} href={`/docs/${page.slug}`}>{page.title}</Link>)}</nav>)}
        <div className="docs-sidebar-section">
          <h2>Operations</h2>
          <nav aria-label="Operations">
            <Link href="/health" aria-current={pathname === "/health" ? "page" : undefined} onClick={() => setOpen(false)}>Health</Link>
            <Link href="/system" aria-current={pathname === "/system" ? "page" : undefined} onClick={() => setOpen(false)}>System</Link>
            <Link href="/backups" aria-current={pathname === "/backups" ? "page" : undefined} onClick={() => setOpen(false)}>Backups</Link>
          </nav>
        </div>
        <div className="docs-sidebar-section">
          <h2>Explore</h2>
          <nav aria-label="Explore">
            <Link href="/features" onClick={() => setOpen(false)}>Features</Link>
            <Link href="/manual" onClick={() => setOpen(false)}>User Manual</Link>
          </nav>
        </div>
      </aside>
      {children}
    </div>
  </div>;
}
