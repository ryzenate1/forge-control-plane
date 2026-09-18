"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  ArrowRightLeft,
  Calendar,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Download,
  FileText,
  Filter,
  Search,
  Server,
  Shield,
  UserCheck,
  X,
} from "lucide-react";
import {
  exportAdminActivity,
  fetchActivityStats,
  fetchAdminActivity,
  type AdminActivityFilter,
  type ApiActivityLog,
} from "@/lib/api";
import { AdminPageToolbar } from "./admin-page-toolbar";
import {
  AdminPageLayout,
  SectionHeader,
  AdminDrawer,
  Btn,
  Card,
  EmptyState,
  Input,
  Pill,
  AdminTable,
  AdminTHead,
  AdminTh,
  AdminTBody,
  AdminTr,
  AdminTd,
  selectStyle,
  cn,
} from "./admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";

type ActivityKind = "user_action" | "deployment" | "auth" | "admin" | "node_event" | "system";

type FullActivityEvent = ApiActivityLog & {
  actorType?: string;
  subjectName?: string;
  userAgent?: string;
};

const PAGE_SIZES = [25, 50, 100] as const;

const LEVELS = [
  { value: "", label: "All levels" },
  { value: "info", label: "Info" },
  { value: "warning", label: "Warning" },
  { value: "error", label: "Error" },
  { value: "critical", label: "Critical" },
] as const;

function downloadBlob(filename: string, blob: Blob) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function timeAgo(value: number | string): string {
  const t = typeof value === "number" ? value : new Date(value).getTime();
  if (!Number.isFinite(t)) return "—";
  const mins = Math.max(0, Math.round((Date.now() - t) / 60000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function displayTimestamp(timestamp: string) {
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? "Unknown time" : date.toLocaleString();
}

function classifyAuditAction(action: string): ActivityKind {
  const normalizedAction = action.toLowerCase();
  const auth = ["login", "logout", "password", "2fa", "totp", "webauthn", "sso", "oauth"];
  const admin = ["role", "permission", "setting", "api_key", "webhook"];
  const node = ["node.", "server.", "allocation", "mount", "database_host", "template", "egg", "nest"];
  const deployment = ["deploy", "migration", "evacuation", "recovery"];
  if (auth.some((key) => normalizedAction.includes(key))) return "auth";
  if (admin.some((key) => normalizedAction.includes(key))) return "admin";
  if (deployment.some((key) => normalizedAction.includes(key))) return "deployment";
  if (node.some((key) => normalizedAction.includes(key))) return "node_event";
  if (normalizedAction.startsWith("user.")) return "user_action";
  return "system";
}

function getEventIcon(type: ActivityKind) {
  switch (type) {
    case "user_action": return UserCheck;
    case "deployment": return ArrowRightLeft;
    case "auth": return Shield;
    case "admin": return Activity;
    case "node_event": return Server;
    default: return AlertTriangle;
  }
}

function levelTone(level?: string): "blue" | "yellow" | "red" | "neutral" {
  switch ((level ?? "").toLowerCase()) {
    case "info": return "blue";
    case "warning": return "yellow";
    case "error":
    case "critical": return "red";
    default: return "neutral";
  }
}

function actorLabel(entry: FullActivityEvent): string {
  return entry.actorEmail ?? entry.userId ?? entry.ip ?? "system";
}

function resourceLabel(entry: FullActivityEvent): string {
  if (entry.subjectName) return entry.subjectName;
  if (entry.subjectId) return `${entry.subjectType ?? "resource"}:${entry.subjectId}`;
  return entry.subjectType ?? "panel";
}

function dayBoundary(value: string, endOfDay: boolean) {
  if (!value) return undefined;
  return new Date(`${value}T${endOfDay ? "23:59:59.999" : "00:00:00.000"}`).toISOString();
}

export function AdminActivityLog() {
  const [event, setEvent] = useState("");
  const [actorId, setActorId] = useState("");
  const [subjectType, setSubjectType] = useState("");
  const [subjectId, setSubjectId] = useState("");
  const [source, setSource] = useState("");
  const [level, setLevel] = useState("");
  const [timePreset, setTimePreset] = useState("all");
  const [presetBounds, setPresetBounds] = useState<{ from: string; to: string } | null>(null);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [pageSize, setPageSize] = useState<(typeof PAGE_SIZES)[number]>(50);
  const [offset, setOffset] = useState(0);
  const [exporting, setExporting] = useState<"csv" | "json" | null>(null);
  const [exportError, setExportError] = useState<string | null>(null);
  const [exportOpen, setExportOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const filter = useMemo<AdminActivityFilter>(() => ({
    actorId: actorId.trim() || undefined,
    subjectType: subjectType.trim() || undefined,
    subjectId: subjectId.trim() || undefined,
    event: event.trim() || undefined,
    level: level || undefined,
    source: source.trim() || undefined,
    from: presetBounds?.from ?? dayBoundary(from, false),
    to: presetBounds?.to ?? dayBoundary(to, true),
    limit: pageSize,
    offset,
  }), [actorId, event, from, level, offset, pageSize, source, subjectId, subjectType, to, presetBounds]);

  const activityQuery = useQuery({
    queryKey: ["admin-activity", filter],
    queryFn: () => fetchAdminActivity(filter),
    refetchInterval: 15_000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: true,
  });
  const statsQuery = useQuery({
    queryKey: ["admin-activity-stats"],
    queryFn: fetchActivityStats,
    refetchInterval: 30_000,
    refetchIntervalInBackground: false,
    retry: 1,
  });

  const events = useMemo(() => (activityQuery.data?.events ?? []) as FullActivityEvent[], [activityQuery.data]);
  const total = activityQuery.data?.total ?? 0;
  const currentPage = Math.floor(offset / pageSize) + 1;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const selected = useMemo(() => events.find((e) => e.id === selectedId) ?? null, [events, selectedId]);
  const stats = statsQuery.data;
  const byLevel = stats?.byLevel ?? {};
  const levelTotal = ["info", "warning", "error", "critical"].reduce((s, l) => s + (byLevel[l] ?? 0), 0);

  function updateFilter(update: () => void) {
    setOffset(0);
    update();
  }

  function selectTimePreset(value: string) {
    const durations: Record<string, number> = { "1h": 3600000, "24h": 86400000, "7d": 604800000 };
    const now = Date.now();
    setTimePreset(value);
    setPresetBounds(durations[value] ? { from: new Date(now - durations[value]).toISOString(), to: new Date(now).toISOString() } : null);
    setFrom("");
    setTo("");
    setOffset(0);
  }

  function clearFilters() {
    setTimePreset("all");
    setPresetBounds(null);
    setEvent("");
    setActorId("");
    setSubjectType("");
    setSubjectId("");
    setSource("");
    setLevel("");
    setFrom("");
    setTo("");
    setOffset(0);
  }

  async function handleRefresh() {
    setIsRefreshing(true);
    await Promise.allSettled([activityQuery.refetch(), statsQuery.refetch()]);
    setTimeout(() => setIsRefreshing(false), 500);
  }

  async function exportActivity(format: "csv" | "json") {
    setExportOpen(false);
    setExporting(format);
    setExportError(null);
    try {
      const exportFilter = { ...filter };
      delete exportFilter.limit;
      delete exportFilter.offset;
      const blob = await exportAdminActivity(format, exportFilter);
      downloadBlob(`activity-log-${new Date().toISOString().slice(0, 10)}.${format}`, blob);
    } catch (error) {
      setExportError(errorMessage(error, "Activity export failed."));
    } finally {
      setExporting(null);
    }
  }

  const isLoading = activityQuery.isLoading;
  const loadError = activityQuery.isError ? errorMessage(activityQuery.error, "Activity events could not be loaded.") : null;
  const hasActiveFilters = Boolean(event || actorId || subjectType || subjectId || source || level || from || to || presetBounds);
  const advancedFiltersCount = [actorId, subjectType, subjectId, source].filter(Boolean).length;

  const kpis = [
    { label: "Total events", value: stats?.totalEvents, icon: FileText, color: "text-sky-400", sub: "Recorded in audit store" },
    { label: "Events today", value: stats?.eventsToday, icon: Calendar, color: "text-emerald-400", sub: "Since 00:00 UTC" },
    { label: "This hour", value: stats?.eventsThisHour, icon: Activity, color: "text-amber-400", sub: "Rolling 60m window" },
    { label: "Unique actors", value: stats?.uniqueActors, icon: UserCheck, color: "text-purple-400", sub: "Active identities" },
  ];

  return (
    <AdminPageLayout>
      {/*
        No breadcrumb here: `AdminShell` renders the registry-derived trail for
        every /admin page, and this file's hand-written "Command / Activity"
        both duplicated it and named a group that no longer exists.

        The badge is derived from the query's own `dataUpdatedAt`. It replaces a
        pulsing "Live · updated just now" that was pure markup — it claimed
        freshness before the first fetch and while the fetch was failing.
      */}
      <div className="flex items-center justify-end text-xs">
        <FreshnessBadge state={sourceState(activityQuery, 15_000)} />
      </div>

      <SectionHeader
        title="Activity"
        info={{
          title: "Audit trail",
          triggerLabel: "About Activity",
          eyebrow: "Architecture & Semantics",
          description: "Review recorded control-plane actions, including their actor, target, severity, and time. Filters apply to the event count, table, and export. Audit gaps are possible during outages.",
          sections: [
                {
                  title: "Query & export",
                  icon: FileText,
                  content:
                    "GET /admin/activity supports actor, resource, event, level, source and time-range filters with limit/offset pagination. Export streams the same filtered dataset as CSV or JSON.",
                },
                {
                  title: "Health vs Activity",
                  icon: Activity,
                  content:
                    "Health shows what is wrong right now. Activity shows who did what and when — the forensic trail behind every state change.",
                },
              ],
        }}
        sub="Platform-wide audit history. Filters apply to the event count, table, and export."
        action={<AdminPageToolbar range={{ value: timePreset, onChange: selectTimePreset, options: [
            { value: "all", label: "All time" }, { value: "1h", label: "Last 1 hour" },
            { value: "24h", label: "Last 24 hours" }, { value: "7d", label: "Last 7 days" },
            ...(timePreset === "custom" ? [{ value: "custom", label: "Custom dates" }] : []),
          ] }} onRefresh={handleRefresh} refreshing={isRefreshing} refreshLabel="Refresh activity">
            <div className="relative">
            <button
              type="button"
              onClick={() => setExportOpen((v) => !v)}
              disabled={exporting !== null}
              className="inline-flex h-8 items-center gap-1.5 rounded-md bg-[var(--brand)] px-3 text-xs font-semibold text-white shadow-sm transition hover:bg-[var(--brand-hover)] focus:outline-none focus:ring-1 focus:ring-[var(--brand)] disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Download size={14} />
              <span>{exporting ? "Exporting…" : "Export"}</span>
              <ChevronDown size={13} />
            </button>
            {exportOpen && (
              <>
                <button type="button" aria-label="Close export menu" className="fixed inset-0 z-10 cursor-default" onClick={() => setExportOpen(false)} />
                <div className="absolute right-0 z-20 mt-1 w-44 overflow-hidden rounded-lg border border-white/10 bg-[var(--surface-raised)] shadow-xl">
                  {(["csv", "json"] as const).map((format) => (
                    <button
                      key={format}
                      type="button"
                      onClick={() => { void exportActivity(format); }}
                      className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-slate-200 transition hover:bg-white/[0.06]"
                    >
                      <Download size={12} className="text-slate-400" />
                      Export {format.toUpperCase()}
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>
          </AdminPageToolbar>}
      />

      <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
        {kpis.map((kpi) => {
          const Icon = kpi.icon;
          return (
            <div key={kpi.label} className="rounded-xl border border-white/[0.08] bg-[var(--surface)] p-4 shadow-sm">
              <div className="flex items-center justify-between text-xs font-semibold text-slate-200">
                <span className="flex items-center gap-2">
                  <Icon size={14} className={kpi.color} />
                  <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">{kpi.label}</span>
                </span>
              </div>
              <p className="mt-2 font-mono text-2xl font-bold tracking-tight text-slate-100">
                {statsQuery.isLoading ? "…" : typeof kpi.value === "number" ? kpi.value.toLocaleString() : "—"}
              </p>
              <p className="mt-1 text-[11px] text-slate-500">{kpi.sub}</p>
            </div>
          );
        })}
      </div>
      {!statsQuery.isLoading && !statsQuery.isError && levelTotal > 0 && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-xl border border-white/[0.07] bg-white/[0.015] px-4 py-2.5 text-[11px]">
          <span className="font-semibold uppercase tracking-wider text-slate-500">By level</span>
          {[
            { key: "info", label: "Info", color: "bg-sky-400", text: "text-sky-300" },
            { key: "warning", label: "Warning", color: "bg-amber-400", text: "text-amber-300" },
            { key: "error", label: "Errors", color: "bg-red-400", text: "text-red-300" },
            { key: "critical", label: "Critical", color: "bg-red-500", text: "text-red-300" },
          ].map((l) => (
            <span key={l.key} className="flex items-center gap-1.5 font-mono text-slate-300">
              <span className={cn("h-1.5 w-1.5 rounded-full", l.color)} />
              <span>{l.label}</span>
              <span className={cn("font-bold", l.text)}>{(byLevel[l.key] ?? 0).toLocaleString()}</span>
            </span>
          ))}
          <div className="flex h-1.5 min-w-40 flex-1 overflow-hidden rounded-full bg-white/[0.06]">
            {[
              { key: "info", color: "bg-sky-400" },
              { key: "warning", color: "bg-amber-400" },
              { key: "error", color: "bg-red-400" },
              { key: "critical", color: "bg-red-500" },
            ].map((l) => (
              <div key={l.key} className={cn("h-full", l.color)} style={{ width: `${((byLevel[l.key] ?? 0) / levelTotal) * 100}%` }} />
            ))}
          </div>
        </div>
      )}

      <div className="flex flex-col gap-2 rounded-xl border border-white/[0.07] bg-white/[0.015] p-3 xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5">
          <Search size={13} className="shrink-0 text-slate-500" />
          <input
            type="text"
            value={event}
            onChange={(e) => updateFilter(() => setEvent(e.target.value))}
            placeholder="Search by event name…"
            aria-label="Search by event name"
            className="w-full bg-transparent text-xs text-slate-200 outline-none placeholder:text-slate-600"
          />
          {event && (
            <button type="button" aria-label="Clear event search" onClick={() => updateFilter(() => setEvent(""))} className="text-slate-500 hover:text-white">
              <X size={13} />
            </button>
          )}
        </label>
        <div className="flex flex-wrap gap-1 rounded-lg border border-white/[0.07] bg-black/20 p-0.5">
          {LEVELS.map((l) => (
            <button
              key={l.value || "all"}
              type="button"
              onClick={() => updateFilter(() => setLevel(l.value))}
              className={cn(
                "rounded-md px-3 py-1 text-[11px] font-semibold transition",
                level === l.value ? "bg-[var(--brand)] text-white" : "text-slate-400 hover:text-slate-200",
              )}
            >
              {l.label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5 text-[11px] text-slate-400">
            <Calendar size={12} />
            <input type="date" value={from} max={to || undefined} onChange={(e) => updateFilter(() => { setFrom(e.target.value); setTimePreset("custom"); setPresetBounds(null); })} aria-label="From date" className="bg-transparent text-slate-200 outline-none" />
          </label>
          <span className="text-slate-600">→</span>
          <label className="flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-black/20 px-2.5 py-1.5 text-[11px] text-slate-400">
            <Calendar size={12} />
            <input type="date" value={to} min={from || undefined} onChange={(e) => updateFilter(() => { setTo(e.target.value); setTimePreset("custom"); setPresetBounds(null); })} aria-label="To date" className="bg-transparent text-slate-200 outline-none" />
          </label>
        </div>
        <div className="flex items-center gap-2 xl:ml-auto">
          <Btn tone="ghost" onClick={() => setShowAdvanced(!showAdvanced)}>
            <Filter size={13} />
            {showAdvanced ? "Hide filters" : `Filters${advancedFiltersCount > 0 ? ` (${advancedFiltersCount})` : ""}`}
          </Btn>
          <Btn tone="ghost" onClick={clearFilters} disabled={!hasActiveFilters}>Clear</Btn>
        </div>
      </div>

      {showAdvanced && (
        <div className="grid grid-cols-2 gap-3 rounded-xl border border-white/[0.07] bg-white/[0.015] p-4 sm:grid-cols-2 lg:grid-cols-4">
          <Input label="Actor" value={actorId} onChange={(value) => updateFilter(() => setActorId(value))} placeholder="User ID or email" />
          <Input label="Resource type" value={subjectType} onChange={(value) => updateFilter(() => setSubjectType(value))} placeholder="e.g. server" />
          <Input label="Resource ID" value={subjectId} onChange={(value) => updateFilter(() => setSubjectId(value))} placeholder="Resource UUID" />
          <Input label="Source" value={source} onChange={(value) => updateFilter(() => setSource(value))} placeholder="e.g. api" />
        </div>
      )}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-xs text-slate-400">Rows
            <select className={cn(selectStyle, "h-8 w-20")} value={pageSize} onChange={(e) => updateFilter(() => setPageSize(Number(e.target.value) as (typeof PAGE_SIZES)[number]))} aria-label="Rows per page">
              {PAGE_SIZES.map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
          </label>
          <span className="text-xs text-slate-500">{total.toLocaleString()} event{total === 1 ? "" : "s"}{hasActiveFilters ? " (filtered)" : ""}</span>
        </div>
        <span className="font-mono text-[11px] text-slate-600">Page {currentPage} of {totalPages}</span>
      </div>

      {loadError ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-950/20 p-4 text-sm text-red-100" role="alert">
          <span>{loadError}</span>
          <Btn size="sm" tone="ghost" onClick={() => { void activityQuery.refetch(); }}>Retry</Btn>
        </div>
      ) : null}
      {exportError ? <div className="rounded-xl border border-red-500/25 bg-red-950/20 p-3 text-xs text-red-200" role="alert">{exportError}</div> : null}

      <Card>
        {isLoading ? (
          <div className="space-y-0 divide-y divide-white/[0.04]" role="status" aria-label="Loading activity">
            {Array.from({ length: 6 }, (_, i) => (
              <div key={i} className="flex gap-4 px-4 py-3">
                <div className="h-4 w-32 animate-pulse rounded bg-white/[0.06]" />
                <div className="h-4 w-24 animate-pulse rounded bg-white/[0.06]" />
                <div className="h-4 flex-1 animate-pulse rounded bg-white/[0.06]" />
              </div>
            ))}
          </div>
        ) : activityQuery.isError ? (
          <EmptyState icon={FileText} message="Activity events are unavailable." title="Failed to Load" />
        ) : events.length === 0 ? (
          <EmptyState
            icon={FileText}
            message={hasActiveFilters ? "No activity events match these filters. Try adjusting your search." : "No activity events recorded yet. Events will appear here as users interact with the platform."}
            title={hasActiveFilters ? "No Results" : "No Events"}
          />
        ) : (
          <div className="max-h-[680px] overflow-auto">
            <AdminTable label="Activity events">
              <AdminTHead>
                <AdminTh>Time</AdminTh>
                <AdminTh>Event</AdminTh>
                <AdminTh>Action</AdminTh>
                <AdminTh>Actor</AdminTh>
                <AdminTh>Resource</AdminTh>
                <AdminTh>Level</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {events.map((entry) => {
                  const kind = classifyAuditAction(entry.event || entry.action || "");
                  const Icon = getEventIcon(kind);
                  return (
                    <AdminTr key={entry.id} onClick={() => setSelectedId(entry.id)} className={cn(selectedId === entry.id && "bg-sky-500/[0.05]")}>
                      <AdminTd className="whitespace-nowrap font-mono text-[11px] text-slate-500">
                        <span title={displayTimestamp(entry.timestamp || entry.createdAt || "")}>{timeAgo(entry.timestamp || entry.createdAt || "")}</span>
                      </AdminTd>
                      <AdminTd>
                        <span className="inline-flex items-center gap-1.5 text-xs text-slate-300">
                          <span className="grid h-6 w-6 place-items-center rounded-md border border-white/[0.08] bg-white/[0.03] text-slate-400">
                            <Icon size={12} />
                          </span>
                          <span className="capitalize">{kind.replace("_", " ")}</span>
                        </span>
                      </AdminTd>
                      <AdminTd className="max-w-56 truncate font-mono text-[11px] text-slate-200">
                        <span title={entry.event || entry.action}>{entry.event || entry.action || "—"}</span>
                      </AdminTd>
                      <AdminTd className="max-w-44 truncate text-xs text-slate-300">
                        <span title={entry.ip ? `${actorLabel(entry)} · ${entry.ip}` : actorLabel(entry)}>{actorLabel(entry)}</span>
                      </AdminTd>
                      <AdminTd className="max-w-44 truncate font-mono text-[11px] text-slate-500">
                        <span title={resourceLabel(entry)}>{resourceLabel(entry)}</span>
                      </AdminTd>
                      <AdminTd>
                        <Pill tone={levelTone(entry.level)}>{entry.level || "info"}</Pill>
                      </AdminTd>
                    </AdminTr>
                  );
                })}
              </AdminTBody>
            </AdminTable>
          </div>
        )}
        {!isLoading && !activityQuery.isError && total > pageSize ? (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.06] px-4 py-3 text-xs text-slate-400">
            <span>Page {currentPage} of {totalPages} · {total.toLocaleString()} events</span>
            <div className="flex gap-2">
              <Btn size="sm" tone="ghost" disabled={offset === 0 || activityQuery.isFetching} onClick={() => setOffset(0)} ariaLabel="First page">
                <ChevronLeft size={13} /><ChevronLeft size={13} className="-ml-2.5" />
              </Btn>
              <Btn size="sm" tone="ghost" disabled={offset === 0 || activityQuery.isFetching} onClick={() => setOffset((current) => Math.max(0, current - pageSize))}>Previous</Btn>
              <Btn size="sm" tone="ghost" disabled={offset + pageSize >= total || activityQuery.isFetching} onClick={() => setOffset((current) => current + pageSize)}>Next</Btn>
              <Btn size="sm" tone="ghost" disabled={offset + pageSize >= total || activityQuery.isFetching} onClick={() => setOffset((totalPages - 1) * pageSize)} ariaLabel="Last page">
                <ChevronRight size={13} /><ChevronRight size={13} className="-ml-2.5" />
              </Btn>
            </div>
          </div>
        ) : null}
      </Card>

      {selected && (
        <AdminDrawer title="Event detail" onClose={() => setSelectedId(null)}>
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <Pill tone={levelTone(selected.level)}>{selected.level || "info"}</Pill>
              <span className="font-mono text-xs text-slate-400">{selected.source || "api"}</span>
              <span className="ml-auto font-mono text-[11px] text-slate-500">{displayTimestamp(selected.timestamp || selected.createdAt || "")}</span>
            </div>
            <div>
              <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">Event</p>
              <p className="mt-1 font-mono text-sm text-slate-100">{selected.event || selected.action || "—"}</p>
              {selected.description && <p className="mt-1 text-sm leading-6 text-slate-300">{selected.description}</p>}
            </div>
            <dl className="grid grid-cols-1 gap-3 rounded-xl border border-white/[0.07] bg-black/20 p-4 text-xs sm:grid-cols-2">
              {[
                ["Actor", actorLabel(selected)],
                ["Actor type", selected.actorType ?? "—"],
                ["IP", selected.ip ?? "—"],
                ["Resource", resourceLabel(selected)],
                ["Subject type", selected.subjectType ?? "—"],
                ["Subject ID", selected.subjectId ?? "—"],
                ["Event ID", selected.id],
              ].map(([label, value]) => (
                <div key={label} className="min-w-0">
                  <dt className="font-semibold uppercase tracking-wider text-slate-500 text-[10px]">{label}</dt>
                  <dd className="mt-0.5 break-all font-mono text-slate-200">{value}</dd>
                </div>
              ))}
              {selected.userAgent && (
                <div className="min-w-0 sm:col-span-2">
                  <dt className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">User agent</dt>
                  <dd className="mt-0.5 break-all font-mono text-slate-200">{selected.userAgent}</dd>
                </div>
              )}
            </dl>
            <div>
              <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">Properties</p>
              <pre className="mt-1 max-h-80 overflow-auto rounded-xl border border-white/[0.07] bg-black/30 p-3 font-mono text-[11px] leading-5 text-slate-300">
                {(() => {
                  try {
                    const raw = selected.properties as unknown;
                    const parsed = typeof raw === "string" ? JSON.parse(raw) : (raw ?? {});
                    return JSON.stringify(parsed, null, 2);
                  } catch {
                    return "Properties are not valid JSON.";
                  }
                })()}
              </pre>
            </div>
          </div>
        </AdminDrawer>
      )}

      <p className="text-[11px] text-slate-600">
        Audit writes are best-effort and retained per server policy — gaps during outages are possible. For point-in-time failures see{" "}
        <Link href="/admin/health" className="underline hover:text-text">Health</Link>; for trends see <Link href="/admin/monitoring" className="underline hover:text-text">Monitoring</Link>.
      </p>
    </AdminPageLayout>
  );
}
