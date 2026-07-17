"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, AlertTriangle, ArrowRightLeft, Download, FileText, RefreshCw, Server, Shield, UserCheck } from "lucide-react";
import { exportAdminActivity, fetchAdminActivity, type AdminActivityFilter, type ApiActivityLog } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, SectionHeader } from "./admin-ui";

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
  if (!entry.properties) return "—";
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

  return (
    <div>
      <SectionHeader
        title="Activity Log"
        sub="Platform-wide audit history. Filters apply to the event count, table, and export."
        action={<Btn tone="ghost" onClick={() => { void activityQuery.refetch(); }} disabled={isFetching}><RefreshCw className={isFetching ? "animate-spin" : ""} size={13} /> Refresh</Btn>}
      />

      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Input label="Event" value={event} onChange={(value) => updateFilter(() => setEvent(value))} placeholder="Exact event name" />
        <Input label="Actor ID" value={actorId} onChange={(value) => updateFilter(() => setActorId(value))} placeholder="Actor ID" />
        <Input label="Resource type" value={subjectType} onChange={(value) => updateFilter(() => setSubjectType(value))} placeholder="Resource type" />
        <Input label="Resource ID" value={subjectId} onChange={(value) => updateFilter(() => setSubjectId(value))} placeholder="Resource ID" />
        <Input label="Source" value={source} onChange={(value) => updateFilter(() => setSource(value))} placeholder="Source" />
        <select aria-label="Activity level" className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none" onChange={(selection) => updateFilter(() => setLevel(selection.target.value))} value={level}>
          <option value="">All levels</option><option value="info">Info</option><option value="warning">Warning</option><option value="error">Error</option>
        </select>
        <label className="text-xs text-slate-400">From<input aria-label="From date" className="mt-1 block h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none" max={to || undefined} onChange={(selection) => updateFilter(() => setFrom(selection.target.value))} type="date" value={from} /></label>
        <label className="text-xs text-slate-400">To<input aria-label="To date" className="mt-1 block h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none" min={from || undefined} onChange={(selection) => updateFilter(() => setTo(selection.target.value))} type="date" value={to} /></label>
      </div>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <Btn tone="ghost" onClick={clearFilters} disabled={!event && !actorId && !subjectType && !subjectId && !source && !level && !from && !to}>Clear filters</Btn>
        <label className="ml-auto flex items-center gap-2 text-xs text-slate-400">Rows<select aria-label="Rows per page" className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none" onChange={(selection) => updateFilter(() => setPageSize(Number(selection.target.value) as (typeof PAGE_SIZES)[number]))} value={pageSize}>{PAGE_SIZES.map((size) => <option key={size} value={size}>{size}</option>)}</select></label>
        <Btn tone="ghost" onClick={() => { void exportActivity("csv"); }} disabled={exporting !== null}><Download size={13} /> {exporting === "csv" ? "Exporting…" : "CSV"}</Btn>
        <Btn tone="ghost" onClick={() => { void exportActivity("json"); }} disabled={exporting !== null}><Download size={13} /> {exporting === "json" ? "Exporting…" : "JSON"}</Btn>
      </div>

      {loadError ? <div className="mb-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">{loadError} <button className="ml-2 underline hover:text-white" onClick={() => { void activityQuery.refetch(); }} type="button">Retry</button></div> : null}
      {exportError ? <div className="mb-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">{exportError}</div> : null}

      <Card>
        <CardHeader title={`${total.toLocaleString()} event${total === 1 ? "" : "s"}`} icon={FileText} />
        {isLoading ? <div className="py-10 text-center text-sm text-slate-500">Loading activity events…</div> : activityQuery.isError ? <EmptyState icon={FileText} message="Activity events are unavailable. Retry the request." /> : events.length === 0 ? <EmptyState icon={FileText} message="No activity events match these filters." /> : <div className="max-h-[680px] overflow-auto"><table className="w-full min-w-[800px] text-left text-xs"><thead className="sticky top-0 z-10 border-b border-white/[0.06] bg-[#161b28] text-slate-500"><tr><th className="px-4 py-3">Time</th><th className="px-4 py-3">Type</th><th className="px-4 py-3">Action</th><th className="px-4 py-3">Actor</th><th className="px-4 py-3">Resource</th><th className="px-4 py-3">Detail</th></tr></thead><tbody className="divide-y divide-white/[0.04]">{events.map((entry) => { const Icon = getEventIcon(entry.type); return <tr key={entry.id} className="hover:bg-white/[0.02]"><td className="whitespace-nowrap px-4 py-3 text-slate-500">{displayTimestamp(entry.timestamp)}</td><td className="px-4 py-3"><span className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold text-slate-400"><Icon size={10} />{entry.type.replace("_", " ")}</span></td><td className="px-4 py-3 font-semibold text-slate-200">{entry.action}</td><td className="px-4 py-3 text-slate-300">{entry.actor}</td><td className="px-4 py-3 font-mono text-slate-500">{entry.resource}</td><td className="max-w-[200px] truncate px-4 py-3 text-slate-500" title={entry.detail}>{entry.detail}</td></tr>; })}</tbody></table></div>}
        {!isLoading && !activityQuery.isError && total > 0 ? <div className="flex flex-wrap items-center justify-between gap-3 border-t border-white/[0.06] px-4 py-3 text-xs text-slate-400"><span>Page {currentPage} of {totalPages}</span><div className="flex gap-2"><Btn tone="ghost" disabled={offset === 0 || isFetching} onClick={() => setOffset((current) => Math.max(0, current - pageSize))}>Previous</Btn><Btn tone="ghost" disabled={offset + pageSize >= total || isFetching} onClick={() => setOffset((current) => current + pageSize)}>Next</Btn></div></div> : null}
      </Card>
    </div>
  );
}
