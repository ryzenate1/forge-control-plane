"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { docs, type DocPage } from "@/lib/docs";
import Footer from "@/components/footer";

function CopyCode({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const id = window.setTimeout(() => setCopied(false), 1600);
    return () => window.clearTimeout(id);
  }, [copied]);
  const copy = async () => { try { await navigator.clipboard.writeText(value); setCopied(true); } catch { /* clipboard permission denied or insecure context */ } };
  return <div className="doc-code"><div><span>{label}</span><button type="button" onClick={() => void copy()}>{copied ? "Copied" : "Copy"}</button></div><pre><code>{value}</code></pre></div>;
}

export function DocArticle({ page }: { page: DocPage }) {
  const headings = useMemo(() => page.sections.map((section) => section.heading), [page]);
  const { previous, next } = useMemo(() => {
    const position = docs.findIndex((item) => item.slug === page.slug);
    return { previous: docs[position - 1], next: docs[position + 1] };
  }, [page.slug]);
  return <main className="docs-main"><article className="doc-article">
    <div className="breadcrumbs"><Link href="/docs/introduction">Docs</Link><span>/</span><span>{page.group}</span></div>
    <p className="doc-kicker">{page.group}</p><h1>{page.title}</h1><p className="doc-summary">{page.summary}</p>
    <div className="doc-status">Repository-validated operator guidance <a href="https://github.com/ryzenate1/forge-control-plane">View source ↗</a></div>
    {page.sections.map((section) => { const id = section.heading.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, ""); return <section key={section.heading} id={id}><h2><a href={`#${id}`} aria-label={`Link to ${section.heading}`}>#</a>{section.heading}</h2>{section.body?.map((paragraph) => <p key={paragraph}>{paragraph}</p>)}{section.bullets && <ul>{section.bullets.map((item) => <li key={item}>{item}</li>)}</ul>}{section.table && <div className="table-wrap"><table><caption className="sr-only">{section.heading}</caption><thead><tr>{section.table.headers.map((header) => <th key={header} scope="col">{header}</th>)}</tr></thead><tbody>{section.table.rows.map((row, index) => <tr key={index}>{row.map((cell, cellIndex) => <td key={cellIndex}>{cell}</td>)}</tr>)}</tbody></table></div>}{section.code && <CopyCode {...section.code} />}{section.callout && <div className={`doc-callout ${section.callout.tone}`}><strong>{section.callout.title}</strong><p>{section.callout.body}</p></div>}</section>; })}
    <nav className="doc-pager" aria-label="Documentation pagination">{previous ? <Link href={`/docs/${previous.slug}`}><small>Previous</small>{previous.title}</Link> : <span />}{next ? <Link href={`/docs/${next.slug}`}><small>Next</small>{next.title}</Link> : <span />}</nav>
    <Footer />
  </article><nav className="on-page" aria-label="On this page"><p>On this page</p>{headings.map((heading) => <a key={heading} href={`#${heading.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "")}`}>{heading}</a>)}</nav></main>;
}
