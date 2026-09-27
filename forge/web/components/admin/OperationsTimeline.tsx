"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, ArrowRightLeft, Clock, Database, HardDrive, Search, ShieldAlert, Workflow } from "lucide-react";
import { cn } from "@/lib/utils";
import { GenerationFencedDots, StateLanesBadge } from "@/components/shared/generation-fenced-dot";
import { fetchOperationsTimeline, type OperationsTimelineItem } from "@/lib/api/operations";
import { statusTone as centralStatusTone } from "@/lib/api/status";
import { toneStyles } from "@/components/ui/forge/status";

function kindIcon(kind: string) {
  switch (kind) {
    case "job":
      return Workflow;
    case "operation":
      return Activity;
    case "drain":
      return HardDrive;
    case "transfer":
      return ArrowRightLeft;
    case "orphan":
      return ShieldAlert;
    default:
      return Clock;
  }
}

function kindTone(kind: string) {
  switch (kind) {
    case "job":
      return "border-sky-500/30 bg-sky-500/10 text-sky-300";
    case "operation":
      return "border-violet-500/30 bg-violet-500/10 text-violet-300";
    case "drain":
      return "border-amber-500/30 bg-amber-500/10 text-amber-300";
    case "transfer":
      return "border-violet-500/30 bg-violet-500/10 text-violet-300";
    case "orphan":
      return "border-red-500/30 bg-red-500/10 text-red-300";
    default:
      return "border-[var(--line)] bg-white/[0.03] text-[var(--text-subtle)]";
  }
}

function statusTone(status: string) {
  return toneStyles[centralStatusTone(status, "deployment")].chip;
}

function formatTime(iso: string) {
  try {
    const d = new Date(iso);
    return d.toLocaleString(undefined, { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" });
  } catch {
    return iso;
  }
}

export function OperationsTimeline() {
  const [kind, setKind] = useState("");
  const [fencedOnly, setFencedOnly] = useState(false);
  const [search, setSearch] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const q = useQuery({
    queryKey: ["operations-timeline", kind, fencedOnly],
    queryFn: ({ signal }) => fetchOperationsTimeline({ kind: kind || undefined, fenced: fencedOnly || undefined, limit: 120, signal }),
    refetchInterval: 10_000,
    placeholderData: (prev) => prev,
  });

  const filtered = useMemo(() => {
    const rows = q.data?.data ?? [];
    if (!search.trim()) return rows;
    const needle = search.toLowerCase();
    return rows.filter((r) => `${r.id} ${r.type} ${r.status} ${r.serverId ?? ""} ${r.nodeId ?? ""} ${r.error ?? ""}`.toLowerCase().includes(needle));
  }, [q.data, search]);

  const lanes = useMemo(() => {
    const m = new Map<string, { id: string; kind: string; count: number; fenced: number }>();
    for (const r of filtered) {
      const lane = r.serverId || r.nodeId || r.resourceId || r.id;
      const key = `${r.kind}:${lane.slice(0, 8)}`;
      const cur = m.get(key) ?? { id: lane, kind: r.kind, count: 0, fenced: 0 };
      cur.count += 1;
      if (r.isFenced) cur.fenced += 1;
      m.set(key, cur);
    }
    return Array.from(m.entries()).slice(0, 12);
  }, [filtered]);

  const selected = useMemo(() => filtered.find((r) => r.id === selectedId) ?? null, [filtered, selectedId]);

  return (
    <div className="w-full">
      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2 border-y border-[var(--line)] bg-white/[0.01] px-1 py-3">
        <div className="flex items-center gap-1.5">
          <label className="text-[11px] font-semibold uppercase tracking-wider text-[var(--text-subtle)]">Kind</label>
          <select value={kind} onChange={(e) => setKind(e.target.value)} className="h-7 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-2 text-xs">
            <option value="">All</option>
            <option value="job">Jobs (queue)</option>
            <option value="operation">Ops (operation)</option>
            <option value="drain">Drains</option>
            <option value="transfer">Transfers</option>
            <option value="orphan">Orphans</option>
          </select>
        </div>
        <label className="inline-flex items-center gap-1.5 rounded-full border border-[var(--line)] bg-white/[0.03] px-2.5 py-1 text-xs">
          <input type="checkbox" checked={fencedOnly} onChange={(e) => setFencedOnly(e.target.checked)} className="h-3 w-3 rounded" />
          Fenced only
        </label>
        <div className="relative ml-auto flex min-w-[220px] max-w-sm flex-1 items-center">
          <Search size={12} className="pointer-events-none absolute left-2.5 text-[var(--text-subtle)]" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search server, node, status, error…"
            className="h-7 w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-7 pr-2 text-xs"
          />
        </div>
        <span className="text-xs text-[var(--text-subtle)]">
          {q.isLoading ? "Loading…" : `${filtered.length} items`}
          {q.data?.meta?.total != null ? ` · ${q.data.meta.total} total` : ""}
          <span className="ml-1 inline-flex items-center gap-1">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-500" /> live 10s
          </span>
        </span>
      </div>

      <div className="mt-4 grid grid-cols-12 gap-6">
        {/* Lane list (sticky) */}
        <aside className="col-span-12 lg:col-span-3">
          <div className="sticky top-4 space-y-3">
            <div className="rounded-xl border border-[var(--line)] bg-white/[0.02] p-3">
              <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Lanes</div>
              <div className="mt-2 space-y-1.5">
                {lanes.length === 0 ? <div className="text-xs text-[var(--text-subtle)]">No lanes — all quiet.</div> : lanes.map(([key, info]) => (
                  <div key={key} className="flex items-center justify-between rounded-lg border border-[var(--line)] bg-white/[0.02] px-2.5 py-1.5 text-xs">
                    <span className={cn("rounded-full border px-1.5 py-0.5 text-[10px] font-semibold uppercase", kindTone(info.kind))}>{info.kind}</span>
                    <span className="font-mono text-[11px]">{info.id.slice(0, 8)}</span>
                    <span className="text-[11px] text-[var(--text-subtle)]">
                      {info.count} {info.fenced ? <span className="text-red-300">· {info.fenced} fenced</span> : null}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mt-3 rounded-lg border border-dashed border-[var(--line)] bg-white/[0.02] p-2 text-[11px] leading-4 text-[var(--text-subtle)]">
                <div className="font-semibold text-[var(--text)]">Legend</div>
                <div className="mt-1 flex items-center gap-2"><GenerationFencedDots desired="running" actual="running" size={7} /> <span>top=desired · bottom=actual</span></div>
                <div className="flex items-center gap-2"><GenerationFencedDots desired="running" actual="running" generation={13} fenceGeneration={14} isFenced size={7} /> <span>ring = fenced (gen &lt; fence)</span></div>
                <div className="flex items-center gap-2"><span className="h-px w-6 bg-[var(--line)]" /> solid · <span className="h-px w-6 border-t border-dashed border-red-500/50" /> dashed = fenced segment</div>
              </div>
            </div>
            {q.isError ? <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-300">Timeline failed: {(q.error as Error)?.message ?? "unknown"}</div> : null}
          </div>
        </aside>

        {/* Timeline vertical */}
        <section className="col-span-12 lg:col-span-6">
          <div className="relative">
            {/* vertical line */}
            <div className="pointer-events-none absolute left-[11px] top-2 h-[calc(100%-8px)] w-px bg-[var(--line)]" aria-hidden />
            <div className="space-y-2">
              {filtered.length === 0 ? (
                <div className="rounded-xl border border-dashed border-[var(--line)] bg-white/[0.02] p-6 text-center">
                  <Clock size={16} className="mx-auto text-[var(--text-subtle)]" />
                  <div className="mt-2 text-sm font-medium">No operations</div>
                  <div className="text-xs text-[var(--text-subtle)]">Jobs, drains, transfers and orphans will appear here once recorded.</div>
                </div>
              ) : (
                filtered.map((item) => {
                  const Icon = kindIcon(item.kind);
                  const fenced = item.isFenced;
                  const isSelected = selectedId === item.id;
                  return (
                    <button
                      key={`${item.kind}:${item.id}:${item.updatedAt}`}
                      onClick={() => setSelectedId((prev) => (prev === item.id ? null : item.id))}
                      className={cn(
                        "group relative flex w-full gap-3 rounded-xl border bg-white/[0.02] px-3 py-3 text-left transition",
                        isSelected ? "border-violet-500/40 bg-violet-500/[0.06]" : "border-[var(--line)] hover:bg-white/[0.04]",
                        fenced && !isSelected && "border-red-500/20",
                      )}
                    >
                      {/* dot rail */}
                      <div className="relative flex shrink-0 flex-col items-center">
                        <GenerationFencedDots
                          desired={item.desiredState}
                          actual={item.actualState}
                          generation={item.generation}
                          fenceGeneration={item.fenceGeneration}
                          isFenced={fenced}
                          size={8}
                        />
                        {fenced ? <div className="absolute inset-x-1/2 top-[18px] h-6 w-px -translate-x-1/2 border-l border-dashed border-red-500/40" aria-hidden /> : null}
                      </div>

                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <span className={cn("inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider", kindTone(item.kind))}>
                            <Icon size={10} /> {item.kind}
                          </span>
                          <span className={cn("rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider", statusTone(item.status))}>{item.status}</span>
                          <span className="font-mono text-[11px] text-[var(--text-subtle)]">{item.type}</span>
                          {item.serverId ? <span className="font-mono text-[11px] text-[var(--text-subtle)]">srv:{item.serverId.slice(0, 6)}</span> : null}
                          {item.nodeId ? <span className="font-mono text-[11px] text-[var(--text-subtle)]">node:{item.nodeId.slice(0, 6)}</span> : null}
                          <span className="ml-auto font-mono text-[10px] text-[var(--text-subtle)]">{formatTime(item.createdAt)}</span>
                        </div>

                        {item.progress != null ? (
                          <div className="mt-2">
                            <div className="flex items-center justify-between text-[10px] text-[var(--text-subtle)]">
                              <span>Progress</span>
                              <span className="font-mono">{Math.round(item.progress)}%</span>
                            </div>
                            <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-white/10">
                              <div
                                className={cn("h-full rounded-full transition-all", fenced ? "bg-red-500" : item.status === "failed" ? "bg-red-500" : "bg-emerald-500")}
                                style={{ width: `${Math.min(100, Math.max(0, item.progress))}%` }}
                              />
                            </div>
                          </div>
                        ) : null}

                        {item.error ? (
                          <div className={cn("mt-2 flex gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs", fenced ? "border-red-500/30 bg-red-500/10 text-red-200" : "border-amber-500/30 bg-amber-500/10 text-amber-200")}>
                            <AlertTriangle size={12} className="mt-0.5 shrink-0" />
                            <span className="line-clamp-2 break-all">{item.error.slice(0, 220)}</span>
                          </div>
                        ) : null}

                        <div className="mt-1.5 flex flex-wrap gap-1">
                          <StateLanesBadge desired={item.desiredState} actual={item.actualState} generation={item.generation} fenceGeneration={item.fenceGeneration} isFenced={fenced} />
                          {item.correlationId ? <span className="rounded-full border border-[var(--line)] bg-white/[0.03] px-2 py-0.5 font-mono text-[10px] text-[var(--text-subtle)]">corr:{item.correlationId.slice(0, 6)}</span> : null}
                        </div>
                      </div>
                    </button>
                  );
                })
              )}
            </div>
          </div>
        </section>

        {/* Drawer */}
        <aside className="col-span-12 lg:col-span-3">
          <div className="sticky top-4">
            {selected ? <SelectedDrawer item={selected} onClose={() => setSelectedId(null)} /> : (
              <div className="rounded-xl border border-[var(--line)] bg-white/[0.02] p-4">
                <div className="text-sm font-semibold">Details</div>
                <p className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">Click a tick to inspect fencing, progress and error. The two dots encode Desired vs Actual; the ring means the generation is fenced and the operation will be rejected by Beacon.</p>
                <div className="mt-3 rounded-lg border border-[var(--line)] bg-white/[0.02] p-2.5 text-xs">
                  <div className="font-mono text-[11px] font-semibold">Generation fencing (AF-3)</div>
                  <p className="mt-1 text-[11px] leading-4 text-[var(--text-subtle)]">Every server carries a monotonic <code className="rounded bg-white/10 px-1">generation</code>. Evacuation, recovery and fencing bump it with CAS (<code className="rounded bg-white/10 px-1">WHERE generation=$old</code>). Beacon rejects power/create with a stale <code className="rounded bg-white/10 px-1">X-Forge-Generation</code> header.</p>
                </div>
              </div>
            )}
          </div>
        </aside>
      </div>
    </div>
  );
}

function SelectedDrawer({ item, onClose }: { item: OperationsTimelineItem; onClose: () => void }) {
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4 shadow-xl">
      <div className="flex items-center justify-between">
        <div className="text-sm font-semibold">Tick detail</div>
        <button onClick={onClose} className="rounded-lg border border-[var(--line)] px-2 py-1 text-xs hover:bg-white/[0.04]">Close</button>
      </div>
      <div className="mt-3 space-y-3">
        <div className="flex items-center gap-2">
          <GenerationFencedDots desired={item.desiredState} actual={item.actualState} generation={item.generation} fenceGeneration={item.fenceGeneration} isFenced={item.isFenced} size={10} showLabel />
          <div className="min-w-0 flex-1">
            <div className="font-mono text-xs font-semibold">{item.type}</div>
            <div className="text-[11px] text-[var(--text-subtle)]">{item.kind} · {item.status} {item.isFenced ? "· fenced" : ""}</div>
          </div>
        </div>

        <dl className="grid grid-cols-2 gap-2 text-xs">
          <div className="rounded-lg border border-[var(--line)] bg-white/[0.02] p-2"><dt className="text-[11px] text-[var(--text-subtle)]">Generation</dt><dd className="font-mono">{item.generation ?? "—"} {item.fenceGeneration != null && item.fenceGeneration !== item.generation ? `→ ${item.fenceGeneration}` : ""}</dd></div>
          <div className="rounded-lg border border-[var(--line)] bg-white/[0.02] p-2"><dt className="text-[11px] text-[var(--text-subtle)]">Fenced</dt><dd className={cn("font-semibold", item.isFenced ? "text-red-300" : "text-emerald-300")}>{item.isFenced ? "Yes — will be rejected" : "No"}</dd></div>
          <div className="rounded-lg border border-[var(--line)] bg-white/[0.02] p-2"><dt className="text-[11px] text-[var(--text-subtle)]">Server</dt><dd className="font-mono truncate">{item.serverId ?? "—"}</dd></div>
          <div className="rounded-lg border border-[var(--line)] bg-white/[0.02] p-2"><dt className="text-[11px] text-[var(--text-subtle)]">Node</dt><dd className="font-mono truncate">{item.nodeId ?? "—"}</dd></div>
          <div className="col-span-2 rounded-lg border border-[var(--line)] bg-white/[0.02] p-2"><dt className="text-[11px] text-[var(--text-subtle)]">Created</dt><dd className="font-mono text-[11px]">{formatTime(item.createdAt)}</dd></div>
          <div className="col-span-2 rounded-lg border border-[var(--line)] bg-white/[0.02] p-2"><dt className="text-[11px] text-[var(--text-subtle)]">Updated</dt><dd className="font-mono text-[11px]">{formatTime(item.updatedAt)}</dd></div>
        </dl>

        {item.progress != null ? (
          <div>
            <div className="text-xs font-medium">Progress</div>
            <div className="mt-1 h-2 overflow-hidden rounded-full bg-white/10"><div className="h-full bg-emerald-500" style={{ width: `${Math.min(100, Math.max(0, item.progress))}%` }} /></div>
            <div className="mt-1 text-right font-mono text-xs text-[var(--text-subtle)]">{item.progress}%</div>
          </div>
        ) : null}

        <div>
          <div className="text-xs font-medium">State lanes</div>
          <div className="mt-1"><StateLanesBadge desired={item.desiredState} actual={item.actualState} generation={item.generation} fenceGeneration={item.fenceGeneration} isFenced={item.isFenced} /></div>
        </div>

        {item.error ? (
          <div>
            <div className="text-xs font-medium">Error / detail</div>
            <pre className="mt-1 max-h-32 overflow-auto rounded-lg border border-[var(--line)] bg-black/20 p-2 font-mono text-[11px] leading-4 whitespace-pre-wrap break-all">{item.error}</pre>
          </div>
        ) : null}

        <div className="rounded-lg border border-dashed border-[var(--line)] bg-white/[0.02] p-2 text-[11px] leading-4 text-[var(--text-subtle)]">
          Correlation grouping uses the envelope CorrelationID (events/event.go). Jobs and ops that share a CorrelationID belong to one lane (one server).
        </div>
      </div>
    </div>
  );
}
