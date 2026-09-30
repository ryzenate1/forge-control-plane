"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
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
import {
  AdminDrawer,
  AdminErrorState,
  AdminLoadingRows,
  AdminPageLayout,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  EmptyState,
  Input,
  Pill,
  SectionHeader,
  StatsRow,
  cn,
  selectStyle,
} from "./admin-ui";
import { AdminPageToolbar } from "./admin-page-toolbar";
import { FreshnessBadge } from "./telemetry-ui";
import { relativeTime, sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";

type ActivityKind = "user_action" | "deployment" | "auth" | "admin" | "node_event" | "system";

type FullActivityEvent = ApiActivityLog & {
  actorType?: string;
  subjectName?: string;
  userAgent?: string;
};

const PAGE_SIZES = [25, 50, 100] as const;

/** `custom` is a real option because a hand-picked date range is a real state;
 * leaving it out made the select silently display "All time" while the query was
 * filtered. */
const RANGE_OPTIONS = [
  { value: "all", label: "All time" },
  { value: "1h", label: "Last 1 hour" },
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
  { value: "custom", label: "Custom range" },
] as const;

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

function levelTone(level?: string): "info" | "warn" | "danger" | "neutral" {
  switch ((level ?? "").toLowerCase()) {
    case "info": return "info";
    case "warning": return "warn";
    case "error":
    case "critical": return "danger";
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
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [pageSize, setPageSize] = useState<(typeof PAGE_SIZES)[number]>(50);
  const [offset, setOffset] = useState(0);
  const [exporting, setExporting] = useState<"csv" | "json" | null>(null);
  const [exportError, setExportError] = useState<string | null>(null);
  const [exportOpen, setExportOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const filter = useMemo<AdminActivityFilter>(() => ({
    actorId: actorId.trim() || undefined,
    subjectType: subjectType.trim() || undefined,
    subjectId: subjectId.trim() || undefined,
    event: event.trim() || undefined,
    level: level || undefined,
    source: source.trim() || undefined,
    from: dayBoundary(from, false),
    to: dayBoundary(to, true),
    limit: pageSize,
    offset,
  }), [actorId, event, from, level, offset, pageSize, source, subjectId, subjectType, to]);

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
  const total = activityQuery.data?.total;
  const currentPage = Math.floor(offset / pageSize) + 1;
  const totalPages = Math.max(1, Math.ceil((total ?? 0) / pageSize));
  const selected = useMemo(() => events.find((e) => e.id === selectedId) ?? null, [events, selectedId]);
  const stats = statsQuery.data;
  const byLevel = stats?.byLevel ?? {};
  const levelTotal = ["info", "warning", "error", "critical"].reduce((s, l) => s + (byLevel[l] ?? 0), 0);

  /** `GET /admin/activity` answers `{events:[],total:0}` and `…/stats` answers `{}`
   * when no activity service is wired (`handlers_activity.go:39,64`), so an empty
   * list cannot be told apart from an absent feature by the response alone. The
   * stats payload is the distinguisher: no counters means "not enabled", which is
   * not "no events". */
  const serviceNotReporting = statsQuery.isSuccess && typeof stats?.totalEvents !== "number";

  function updateFilter(update: () => void) {
    setOffset(0);
    update();
  }

  function clearFilters() {
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
    await Promise.allSettled([activityQuery.refetch(), statsQuery.refetch()]);
  }

  function applyRange(value: string) {
    if (value === "all") { setFrom(""); setTo(""); }
    if (value === "1h" || value === "24h" || value === "7d") {
      const ms = value === "1h" ? 3_600_000 : value === "24h" ? 86_400_000 : 604_800_000;
      setFrom(new Date(Date.now() - ms).toISOString().slice(0, 10));
      setTo(new Date().toISOString().slice(0, 10));
    }
    // "custom" is the state the date inputs already own; selecting it changes
    // nothing rather than wiping a range the operator typed.
    updateFilter(() => {});
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

  const isLoading = activityQuery.isPending;
  const hasActiveFilters = Boolean(event || actorId || subjectType || subjectId || source || level || from || to);
  const advancedFiltersCount = [actorId, subjectType, subjectId, source].filter(Boolean).length;
  const rangeValue = from ? "custom" : "all";

  const kpi = (value: number | undefined) => (statsQuery.isSuccess && typeof value === "number" ? value.toLocaleString() : "Not reported");

  return (
    <AdminPageLayout>
      {/* One frame, one h1, no second breadcrumb row: `AdminShell` already renders
          the registry trail, and the hand-written "Command / Activity" this file
          used to draw named a group that does not exist in the registry. */}
      <SectionHeader
        title="Activity"
        sub="Human-readable audit and activity history."
        status={<FreshnessBadge state={sourceState(activityQuery, 15_000)} />}
        info={{
          title: "Audit trail",
          eyebrow: "Architecture & Semantics",
          description: "Every mutation in the control plane is logged with actor identity, target resource, level and timestamps. Filters apply to the event count, table and export.",
          sections: [
            {
              title: "Query & export",
              icon: FileText,
              content: "GET /admin/activity supports actor, resource, event, level, source and time-range filters with limit/offset pagination. Export streams the same filtered dataset as CSV or JSON.",
            },
            {
              title: "Health vs Activity",
              icon: Activity,
              content: "Health shows what is wrong right now. Activity shows who did what and when — the forensic trail behind every state change.",
            },
          ],
        }}
        action={
          <AdminPageToolbar
            range={{ value: rangeValue, onChange: applyRange, options: RANGE_OPTIONS }}
            onRefresh={() => void handleRefresh()}
            refreshing={activityQuery.isFetching || statsQuery.isFetching}
            refreshLabel="Refresh activity"
          >
            <div className="relative">
              <Btn tone="ghost" size="sm" onClick={() => setExportOpen((v) => !v)} disabled={exporting !== null} ariaLabel="Export activity">
                <Download size={14} aria-hidden="true" /> {exporting ? "Exporting…" : "Export"} <ChevronDown size={12} aria-hidden="true" />
              </Btn>
              {exportOpen ? (
                <>
                  <button type="button" aria-label="Close export menu" className="fixed inset-0 z-10 cursor-default" onClick={() => setExportOpen(false)} />
                  <div className="ui-popover absolute right-0 z-20 mt-1 w-44">
                    {(["csv", "json"] as const).map((format) => (
                      <button
                        key={format}
                        type="button"
                        onClick={() => { void exportActivity(format); }}
                        className="ui-menu-item"
                      >
                        <Download size={12} aria-hidden="true" className="text-text-subtle" />
                        Export {format.toUpperCase()}
                      </button>
                    ))}
                  </div>
                </>
              ) : null}
            </div>
          </AdminPageToolbar>
        }
      />

      <StatsRow items={[
        { label: "Total events", value: kpi(stats?.totalEvents), icon: FileText },
        { label: "Events today", value: kpi(stats?.eventsToday), icon: Calendar },
        { label: "This hour", value: kpi(stats?.eventsThisHour), icon: Activity },
        { label: "Unique actors", value: kpi(stats?.uniqueActors), icon: UserCheck },
      ]} />
      <p className="ui-hint">
        “Events today” counts records from 00:00 UTC; “this hour” is a rolling 60-minute window; “unique actors” counts
        distinct actor IDs. Counters come from <code className="ui-code-inline">/admin/activity/stats</code> and are not filtered by the controls below.
      </p>

      {statsQuery.isSuccess && levelTotal > 0 ? (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-line bg-overlay-subtle px-4 py-2.5">
          <span className="t-eyebrow">By level</span>
          {([
            { key: "info", label: "Info", tone: "info" },
            { key: "warning", label: "Warning", tone: "warn" },
            { key: "error", label: "Errors", tone: "danger" },
            { key: "critical", label: "Critical", tone: "danger" },
          ] as const).map((l) => (
            <span key={l.key} className="flex items-center gap-1.5 font-mono text-xs text-text">
              <span aria-hidden="true" className={cn("h-1.5 w-1.5 rounded-full", l.tone === "info" ? "bg-info" : l.tone === "warn" ? "bg-warn" : "bg-danger")} />
              <span>{l.label}</span>
              <span className="font-semibold">{(byLevel[l.key] ?? 0).toLocaleString()}</span>
            </span>
          ))}
          <div className="flex h-1.5 min-w-40 flex-1 overflow-hidden rounded-full bg-overlay-strong" aria-hidden="true">
            {([
              { key: "info", cls: "bg-info" },
              { key: "warning", cls: "bg-warn" },
              { key: "error", cls: "bg-danger" },
              { key: "critical", cls: "bg-danger" },
            ] as const).map((l) => (
              <div key={l.key} className={cn("h-full", l.cls)} style={{ width: `${((byLevel[l.key] ?? 0) / levelTotal) * 100}%` }} />
            ))}
          </div>
          {typeof stats?.totalEvents === "number" && stats.totalEvents > levelTotal ? (
            <span className="t-meta">
              {stats.totalEvents.toLocaleString()} total includes levels outside these four
            </span>
          ) : null}
        </div>
      ) : null}

      <div className="ui-toolbar xl:flex-row xl:items-center">
        <label className="flex min-w-52 flex-1 items-center gap-2 rounded-lg border border-line bg-surface-input px-2.5 py-1.5">
          <Search size={13} aria-hidden="true" className="shrink-0 text-text-muted" />
          <input
            type="text"
            value={event}
            onChange={(e) => updateFilter(() => setEvent(e.target.value))}
            placeholder="Search by event name…"
            aria-label="Search by event name"
            className="w-full bg-transparent text-xs text-text outline-none placeholder:text-text-muted"
          />
          {event ? (
            <button type="button" aria-label="Clear event search" onClick={() => updateFilter(() => setEvent(""))} className="text-text-muted hover:text-text">
              <X size={13} aria-hidden="true" />
            </button>
          ) : null}
        </label>
        <div className="flex flex-wrap gap-1 rounded-lg border border-line bg-surface-input p-0.5" role="group" aria-label="Event level filter">
          {LEVELS.map((l) => (
            <button
              key={l.value || "all"}
              type="button"
              aria-pressed={level === l.value}
              onClick={() => updateFilter(() => setLevel(l.value))}
              className={cn(
                "rounded-md px-3 py-1 text-xs font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]",
                level === l.value ? "bg-brand text-text" : "text-text-subtle hover:text-text",
              )}
            >
              {l.label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-1.5 rounded-lg border border-line bg-surface-input px-2.5 py-1.5 text-xs text-text-subtle">
            From
            <input type="date" value={from} max={to || undefined} onChange={(e) => updateFilter(() => setFrom(e.target.value))} aria-label="From date" className="bg-transparent text-text outline-none" />
          </label>
          <label className="flex items-center gap-1.5 rounded-lg border border-line bg-surface-input px-2.5 py-1.5 text-xs text-text-subtle">
            To
            <input type="date" value={to} min={from || undefined} onChange={(e) => updateFilter(() => setTo(e.target.value))} aria-label="To date" className="bg-transparent text-text outline-none" />
          </label>
        </div>
        <div className="flex items-center gap-2 xl:ml-auto">
          {/* Native button rather than the frozen `Btn`, because the disclosure
              needs `aria-expanded`/`aria-controls` and Btn does not forward them. */}
          <button
            type="button"
            className="ui-button ui-button-ghost"
            aria-expanded={showAdvanced}
            aria-controls="activity-advanced-filters"
            onClick={() => setShowAdvanced((v) => !v)}
          >
            <Filter size={13} aria-hidden="true" />
            {showAdvanced ? "Hide filters" : `Filters${advancedFiltersCount > 0 ? ` (${advancedFiltersCount})` : ""}`}
          </button>
          <Btn tone="ghost" onClick={clearFilters} disabled={!hasActiveFilters}>Clear</Btn>
        </div>
      </div>

      {showAdvanced ? (
        <div className="grid grid-cols-2 gap-3 rounded-lg border border-line bg-overlay-subtle p-4 sm:grid-cols-2 lg:grid-cols-4" id="activity-advanced-filters">
          <Input label="Actor" value={actorId} onChange={(value) => updateFilter(() => setActorId(value))} placeholder="User ID or email" />
          <Input label="Resource type" value={subjectType} onChange={(value) => updateFilter(() => setSubjectType(value))} placeholder="e.g. server" />
          <Input label="Resource ID" value={subjectId} onChange={(value) => updateFilter(() => setSubjectId(value))} placeholder="Resource UUID" />
          <Input label="Source" value={source} onChange={(value) => updateFilter(() => setSource(value))} placeholder="e.g. api" />
        </div>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-xs text-text-subtle">Rows
            <select className={cn(selectStyle, "h-8 w-20")} value={pageSize} onChange={(e) => updateFilter(() => setPageSize(Number(e.target.value) as (typeof PAGE_SIZES)[number]))} aria-label="Rows per page">
              {PAGE_SIZES.map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
          </label>
          <span className="t-meta">
            {activityQuery.isSuccess ? `${(total ?? 0).toLocaleString()} event${total === 1 ? "" : "s"}${hasActiveFilters ? " matching these filters" : ""}` : "Event count not loaded"}
          </span>
        </div>
        <span className="t-meta">{activityQuery.isSuccess ? `Page ${currentPage} of ${totalPages}` : "Page —"}</span>
      </div>

      {exportError ? <div className="ui-alert ui-alert-danger" role="alert">{exportError}</div> : null}

      <Card>
        {isLoading ? (
          <AdminLoadingRows rows={6} cols={4} label="Loading activity events" />
        ) : activityQuery.isError ? (
          // A failed read is an error, never an empty list.
          <div className="p-4"><AdminErrorState message={errorMessage(activityQuery.error, "Activity events could not be loaded.")} retry={() => void activityQuery.refetch()} /></div>
        ) : serviceNotReporting ? (
          <div className="ui-alert ui-alert-warning flex-wrap items-center justify-between gap-3">
            <span className="flex items-center gap-2">
              <Pill tone="unknown">Not reporting</Pill>
              The control plane&apos;s activity service returned no counters, so this page cannot tell “no events recorded” from “audit is not enabled”. Check the server configuration rather than reading this as an empty history.
            </span>
            <Btn size="sm" tone="ghost" onClick={() => void statsQuery.refetch()}>Retry stats</Btn>
          </div>
        ) : events.length === 0 ? (
          <EmptyState
            icon={FileText}
            message={hasActiveFilters ? "No activity events match these filters. Try widening the time range or clearing the level." : "The event list loaded successfully and contains no records."}
            title={hasActiveFilters ? "No results" : "No events recorded"}
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
                  const stamp = entry.timestamp || entry.createdAt || "";
                  return (
                    <AdminTr key={entry.id} onClick={() => setSelectedId(entry.id)} className={cn(selectedId === entry.id && "bg-overlay-subtle")}>
                      <AdminTd className="whitespace-nowrap font-mono text-xs text-text-subtle" title={formatDate(stamp, "Unknown time")}>
                        {relativeTime(stamp) ?? "age unknown"}
                      </AdminTd>
                      <AdminTd>
                        <span className="inline-flex items-center gap-1.5 text-xs text-text">
                          <span aria-hidden="true" className="grid h-6 w-6 place-items-center rounded-md border border-line bg-overlay text-text-subtle">
                            <Icon size={12} />
                          </span>
                          <span className="capitalize">{kind.replace("_", " ")}</span>
                        </span>
                      </AdminTd>
                      <AdminTd className="max-w-56 truncate font-mono text-xs text-text" title={entry.event || entry.action}>
                        {entry.event || entry.action || "—"}
                      </AdminTd>
                      <AdminTd className="max-w-44 truncate text-xs text-text" title={entry.ip ? `${actorLabel(entry)} · ${entry.ip}` : actorLabel(entry)}>
                        {actorLabel(entry)}
                      </AdminTd>
                      <AdminTd className="max-w-44 truncate font-mono text-xs text-text-subtle" title={resourceLabel(entry)}>
                        {resourceLabel(entry)}
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
        {activityQuery.isSuccess && !activityQuery.isError && (total ?? 0) > pageSize ? (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-4 py-3 text-xs text-text-subtle">
            <span>Page {currentPage} of {totalPages} · {(total ?? 0).toLocaleString()} events</span>
            <div className="flex gap-2">
              <Btn size="sm" tone="ghost" disabled={offset === 0 || activityQuery.isFetching} onClick={() => setOffset(0)} ariaLabel="First page">
                <ChevronLeft size={13} aria-hidden="true" /><ChevronLeft size={13} aria-hidden="true" className="-ml-2.5" />
              </Btn>
              <Btn size="sm" tone="ghost" disabled={offset === 0 || activityQuery.isFetching} onClick={() => setOffset((current) => Math.max(0, current - pageSize))}>Previous</Btn>
              <Btn size="sm" tone="ghost" disabled={offset + pageSize >= (total ?? 0) || activityQuery.isFetching} onClick={() => setOffset((current) => current + pageSize)}>Next</Btn>
              <Btn size="sm" tone="ghost" disabled={offset + pageSize >= (total ?? 0) || activityQuery.isFetching} onClick={() => setOffset((totalPages - 1) * pageSize)} ariaLabel="Last page">
                <ChevronRight size={13} aria-hidden="true" /><ChevronRight size={13} aria-hidden="true" className="-ml-2.5" />
              </Btn>
            </div>
          </div>
        ) : null}
      </Card>

      {selected ? (
        <AdminDrawer title="Event detail" onClose={() => setSelectedId(null)}>
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <Pill tone={levelTone(selected.level)}>{selected.level || "info"}</Pill>
              <span className="font-mono text-xs text-text-subtle">{selected.source || "source not recorded"}</span>
              <span className="ml-auto font-mono text-xs text-text-subtle">{formatDate(selected.timestamp || selected.createdAt || "", "Unknown time")}</span>
            </div>
            <div>
              <p className="t-eyebrow">Event</p>
              <p className="mt-1 font-mono text-sm text-text">{selected.event || selected.action || "—"}</p>
              {selected.description ? <p className="mt-1 text-sm leading-6 text-text-subtle">{selected.description}</p> : null}
            </div>
            <dl className="grid grid-cols-1 gap-3 rounded-lg border border-line bg-surface-input p-4 text-xs sm:grid-cols-2">
              {[
                ["Actor", actorLabel(selected)],
                ["Actor type", selected.actorType ?? "Not reported"],
                ["IP", selected.ip ?? "Not reported"],
                ["Resource", resourceLabel(selected)],
                ["Subject type", selected.subjectType ?? "Not reported"],
                ["Subject ID", selected.subjectId ?? "Not reported"],
                ["Event ID", selected.id],
              ].map(([label, value]) => (
                <div key={label} className="min-w-0">
                  <dt className="t-eyebrow">{label}</dt>
                  <dd className="mt-0.5 break-all font-mono text-text">{value}</dd>
                </div>
              ))}
              {selected.userAgent ? (
                <div className="min-w-0 sm:col-span-2">
                  <dt className="t-eyebrow">User agent</dt>
                  <dd className="mt-0.5 break-all font-mono text-text">{selected.userAgent}</dd>
                </div>
              ) : null}
            </dl>
            <div>
              <p className="t-eyebrow">Properties</p>
              <pre className="ui-code-block mt-1 max-h-80 overflow-auto whitespace-pre-wrap">
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
      ) : null}

      <p className="ui-hint">
        Audit writes are best-effort and retained per server policy, so gaps during outages are possible. For point-in-time
        failures see <Link className="text-brand underline underline-offset-2" href="/admin/health">Health</Link>; for trends see{" "}
        <Link className="text-brand underline underline-offset-2" href="/admin/monitoring">Monitoring</Link>.
      </p>
    </AdminPageLayout>
  );
}
