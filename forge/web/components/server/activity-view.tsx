"use client";

import { useMemo, useState } from "react";
import { Activity, ChevronLeft, ChevronRight, Filter } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { type ApiAuditEvent, type ApiServer, fetchServerActivity } from "@/lib/api";

const PAGE_SIZE = 20;
function metadata(raw: string | Record<string, unknown> | undefined) { if (!raw) return []; const parsed: Record<string, unknown> = typeof raw === "string" ? (() => { try { return JSON.parse(raw) as Record<string, unknown>; } catch { return null; } })() ?? {} : raw; return Object.entries(parsed).map(([key, value]) => ({ key: key.replace(/[_-]/g, " "), value: typeof value === "object" ? JSON.stringify(value) : String(value) })); }
function eventLabel(action: string) { return action.replace(/^server[:.]/, "").replace(/[._:-]+/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase()); }

export function ActivityView({ server }: { server?: ApiServer }) {
  const [filter, setFilter] = useState("");
  const [page, setPage] = useState(1);
  const query = useQuery({ queryKey: ["server-activity", server?.id], queryFn: () => fetchServerActivity(server?.id ?? ""), enabled: Boolean(server?.id) });
  const rows = useMemo(() => [...(query.data?.data ?? [])].sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()), [query.data]);
  const filtered = useMemo(() => { const term = filter.trim().toLowerCase(); if (!term) return rows; return rows.filter((event) => `${event.action} ${event.actorEmail ?? "system"} ${event.targetType} ${event.targetId ?? ""} ${metadata(event.metadata).map((item) => `${item.key} ${item.value}`).join(" ")}`.toLowerCase().includes(term)); }, [filter, rows]);
  const pages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const currentPage = Math.min(page, pages);
  const visible = filtered.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE);

  return <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5"><div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div><h2 className="flex items-center gap-2 font-bold text-white"><Activity size={18} />Activity</h2><p className="mt-1 text-sm text-slate-400">Audit events returned by the server API.</p></div><label className="relative"><Filter className="absolute left-3 top-3 text-slate-500" size={14} /><span className="sr-only">Filter activity</span><input className="h-10 w-full rounded-lg border border-white/10 bg-[#111722] pl-9 pr-3 text-sm text-white outline-none focus:border-red-500 sm:w-72" onChange={(event) => { setFilter(event.target.value); setPage(1); }} placeholder="Filter action, actor, target…" value={filter} /></label></div>
    {query.isLoading ? <p className="mt-6 text-sm text-slate-400">Loading activity…</p> : null}{query.isError ? <div className="mt-5 rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200" role="alert">{query.error instanceof Error ? query.error.message : "Activity could not be loaded."}<button className="ml-3 underline" onClick={() => void query.refetch()} type="button">Retry</button></div> : null}{!query.isLoading && !query.isError && visible.length === 0 ? <p className="mt-6 text-sm text-slate-400">{filter ? "No activity matches this filter." : "No activity has been recorded for this server."}</p> : null}
    <div className="mt-5 space-y-2">{visible.map((event: ApiAuditEvent) => <article className="rounded-lg border border-white/[0.06] bg-[#111722] p-4" key={event.id}><div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between"><div><h3 className="font-semibold text-slate-100">{eventLabel(event.action)}</h3><p className="mt-1 text-xs text-slate-500">{event.actorEmail || "System"}{event.targetType ? ` · ${event.targetType}${event.targetId ? ` ${event.targetId}` : ""}` : ""}</p></div><time className="shrink-0 font-mono text-xs text-slate-500" dateTime={event.createdAt}>{Number.isNaN(new Date(event.createdAt).getTime()) ? event.createdAt : new Date(event.createdAt).toLocaleString()}</time></div>{metadata(event.metadata).length ? <dl className="mt-3 grid gap-2 border-t border-white/[0.06] pt-3 text-xs sm:grid-cols-2">{metadata(event.metadata).map((item) => <div className="min-w-0" key={`${item.key}-${item.value}`}><dt className="capitalize text-slate-500">{item.key}</dt><dd className="mt-0.5 break-words font-mono text-slate-300">{item.value}</dd></div>)}</dl> : null}</article>)}</div>
    {filtered.length > PAGE_SIZE ? <div className="mt-5 flex items-center justify-between border-t border-white/[0.06] pt-4"><p className="text-xs text-slate-500">Page {currentPage} of {pages} · {filtered.length} events (client-side pagination; API pagination is unavailable)</p><div className="flex gap-2"><button aria-label="Previous activity page" className="rounded border border-white/10 p-2 disabled:opacity-40" disabled={currentPage === 1} onClick={() => setPage((value) => Math.max(1, value - 1))} type="button"><ChevronLeft size={15} /></button><button aria-label="Next activity page" className="rounded border border-white/10 p-2 disabled:opacity-40" disabled={currentPage === pages} onClick={() => setPage((value) => Math.min(pages, value + 1))} type="button"><ChevronRight size={15} /></button></div></div> : null}
  </section>;
}
