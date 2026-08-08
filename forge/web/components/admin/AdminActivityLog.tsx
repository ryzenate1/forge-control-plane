"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, ArrowRightLeft, Download, FileText, Filter, RefreshCw, Server, Shield, UserCheck } from "lucide-react";
import { exportAdminActivity, fetchAdminActivity, type AdminActivityFilter, type ApiActivityLog } from "@/lib/api";
import { Btn, Card, EmptyState, Input, SectionHeader, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd, selectStyle, cn } from "./admin-ui";

type ActivityEvent = {
  id: string;
  type: "user_action" | "deployment" | "auth" | "admin" | "node_event" | "system";
  action: string;
  actor: string;
  resource: string;
  detail: string;
  timestamp: string;
};

const PAGE_SIZES = [25, 50, 100] as const;

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

function classifyAuditAction(action: string): ActivityEvent["type"] {
  const normalizedAction = action.toLowerCase();
  const auth = ["login", "logout", "password", "2fa", "totp"];
  const admin = ["role", "permission", "setting", "api_key", "webhook", "oauth"];
  const node = ["node.", "server.", "allocation", "mount", "database_host", "template", "egg", "nest"];
  const deployment = ["deploy", "migration", "evacuation", "recovery"];
  if (auth.some((key) => normalizedAction.includes(key))) return "auth";
  if (admin.some((key) => normalizedAction.includes(key))) return "admin";
  if (deployment.some((key) => normalizedAction.includes(key))) return "deployment";
  if (node.some((key) => normalizedAction.includes(key))) return "node_event";
  if (normalizedAction.startsWith("user.")) return "user_action";
  return "system";
}

function getEventIcon(type: ActivityEvent["type"]) {
  switch (type) {
    case "user_action": return UserCheck;
    case "deployment": return ArrowRightLeft;
    case "auth": return Shield;
    case "admin": return Activity;
    case "node_event": return Server;
    default: return AlertTriangle;
  }
}

function formatDetail(entry: ApiActivityLog): string {
  if (entry.description) return entry.description;
  if (!entry.properties) return "";
  try {
    return JSON.stringify(entry.properties);
  } catch {
    return "Additional activity details are unavailable.";
  }
}

function activityToEvent(entry: ApiActivityLog): ActivityEvent {
  const action = entry.event || entry.action || "activity.unknown";
  return {
    id: entry.id,
    type: classifyAuditAction(action),
    action,
    actor: entry.actorEmail ?? entry.ip ?? "system",
    resource: entry.subjectId ? `${entry.subjectType ?? "resource"}:${entry.subjectId}` : entry.subjectType ?? "panel",
    detail: formatDetail(entry),
    timestamp: entry.timestamp || entry.createdAt || "",
  };
}

function displayTimestamp(timestamp: string) {
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? "Unknown time" : date.toLocaleString();
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
    refetchOnWindowFocus: true,
  });
  const events = useMemo(() => (activityQuery.data?.events ?? []).map(activityToEvent), [activityQuery.data]);
  const total = activityQuery.data?.total ?? 0;
  const currentPage = Math.floor(offset / pageSize) + 1;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

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

  async function exportActivity(format: "csv" | "json") {
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
  const isFetching = activityQuery.isFetching;
  const loadError = activityQuery.isError ? errorMessage(activityQuery.error, "Activity events could not be loaded.") : null;
  const hasActiveFilters = event || actorId || subjectType || subjectId || source || level || from || to;

  const advancedFiltersCount = [actorId, subjectType, subjectId, source, level].filter(Boolean).length;

  const inputBase = "h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15";
  const selectBase = cn(selectStyle, "h-9");

  return (
    <div>
      <SectionHeader
        title="Activity Log"
        sub="Platform-wide audit history. Filters apply to the event count, table, and export."
        action={<Btn tone="ghost" onClick={() => { void activityQuery.refetch(); }} disabled={isFetching}><RefreshCw className={isFetching ? "animate-spin" : ""} size={13} /> Refresh</Btn>}
      />

      <div className="mb-5 flex flex-wrap items-end gap-3">
        <div className="min-w-[240px] flex-1 sm:max-w-sm">
          <label className="block text-xs font-medium text-slate-400 mb-1">Event</label>
          <input
            type="text"
            value={event}
            onChange={(e) => updateFilter(() => setEvent(e.target.value))}
            placeholder="Search by event name..."
            className={inputBase}
          />
        </div>
        <div className="flex items-end gap-2">
          <label className="block text-xs font-medium text-slate-400 mb-1">
            <span className="mb-1 block">From</span>
            <input type="date" value={from} max={to || undefined} onChange={(e) => updateFilter(() => setFrom(e.target.value))} className={inputBase} />
          </label>
          <span className="pb-2 text-slate-600">&rarr;</span>
          <label className="block text-xs font-medium text-slate-400 mb-1">
            <span className="mb-1 block">To</span>
            <input type="date" value={to} min={from || undefined} onChange={(e) => updateFilter(() => setTo(e.target.value))} className={inputBase} />
          </label>
        </div>
        <Btn
          tone="ghost"
          onClick={() => setShowAdvanced(!showAdvanced)}
        >
          <Filter size={13} />
          {showAdvanced ? "Hide filters" : `Filters${advancedFiltersCount > 0 ? ` (${advancedFiltersCount})` : ""}`}
        </Btn>
        <Btn tone="ghost" onClick={clearFilters} disabled={!hasActiveFilters}>Clear</Btn>
      </div>

      {showAdvanced && (
        <div className="mb-5 grid grid-cols-2 gap-3 rounded-xl border border-white/[0.07] bg-white/[0.015] p-4 sm:grid-cols-3 lg:grid-cols-5">
          <Input label="Actor ID" value={actorId} onChange={(value) => updateFilter(() => setActorId(value))} placeholder="Actor ID" />
          <Input label="Resource type" value={subjectType} onChange={(value) => updateFilter(() => setSubjectType(value))} placeholder="Resource type" />
          <Input label="Resource ID" value={subjectId} onChange={(value) => updateFilter(() => setSubjectId(value))} placeholder="Resource ID" />
          <Input label="Source" value={source} onChange={(value) => updateFilter(() => setSource(value))} placeholder="Source" />
          <label className="block text-sm font-medium text-slate-300">
            <span className="mb-1.5 block">Level</span>
            <select className={selectBase} value={level} onChange={(e) => updateFilter(() => setLevel(e.target.value))}>
              <option value="">All levels</option><option value="info">Info</option><option value="warning">Warning</option><option value="error">Error</option>
            </select>
          </label>
        </div>
      )}

      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-xs text-slate-400">Rows
            <select className={cn(selectBase, "w-20")} value={pageSize} onChange={(e) => updateFilter(() => setPageSize(Number(e.target.value) as (typeof PAGE_SIZES)[number]))}>
              {PAGE_SIZES.map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
          </label>
          <span className="text-xs text-slate-500">{total.toLocaleString()} event{total === 1 ? "" : "s"}</span>
        </div>
        <div className="flex items-center gap-2">
          <Btn tone="ghost" onClick={() => { void exportActivity("csv"); }} disabled={exporting !== null}>
            <Download size={13} /> {exporting === "csv" ? "Exporting…" : "Export CSV"}
          </Btn>
          <Btn tone="ghost" onClick={() => { void exportActivity("json"); }} disabled={exporting !== null}>
            <Download size={13} /> {exporting === "json" ? "Exporting…" : "Export JSON"}
          </Btn>
        </div>
      </div>

      {loadError ? <div className="mb-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">{loadError} <button className="ml-2 underline hover:text-white" onClick={() => { void activityQuery.refetch(); }} type="button">Retry</button></div> : null}
      {exportError ? <div className="mb-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">{exportError}</div> : null}

      <Card>
        {isLoading ? <div className="py-10 text-center text-sm text-slate-500">Loading activity events…</div> :
         activityQuery.isError ? <EmptyState icon={FileText} message="Activity events are unavailable." title="Failed to Load" /> :
         events.length === 0 ? (
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
                 <AdminTh>Type</AdminTh>
                 <AdminTh>Action</AdminTh>
                 <AdminTh>Actor</AdminTh>
                 <AdminTh>Resource</AdminTh>
                 <AdminTh>Detail</AdminTh>
               </AdminTHead>
               <AdminTBody>
                 {events.map((entry) => {
                   const Icon = getEventIcon(entry.type);
                   return (
                     <AdminTr key={entry.id}>
                       <AdminTd className="whitespace-nowrap text-slate-500">{displayTimestamp(entry.timestamp)}</AdminTd>
                       <AdminTd>
                         <span className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold text-slate-400">
                           <Icon size={10} />{entry.type.replace("_", " ")}
                         </span>
                       </AdminTd>
                       <AdminTd className="font-semibold text-slate-200">{entry.action}</AdminTd>
                       <AdminTd className="text-slate-300">{entry.actor}</AdminTd>
                       <AdminTd className="font-mono text-slate-500">{entry.resource}</AdminTd>
                       <AdminTd className="max-w-[200px] truncate text-slate-500">
                         <span title={entry.detail}>{entry.detail}</span>
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
            <span>Page {currentPage} of {totalPages}</span>
            <div className="flex gap-2">
              <Btn tone="ghost" disabled={offset === 0 || isFetching} onClick={() => setOffset((current) => Math.max(0, current - pageSize))}>Previous</Btn>
              <Btn tone="ghost" disabled={offset + pageSize >= total || isFetching} onClick={() => setOffset((current) => current + pageSize)}>Next</Btn>
            </div>
          </div>
        ) : null}
      </Card>
    </div>
  );
}
