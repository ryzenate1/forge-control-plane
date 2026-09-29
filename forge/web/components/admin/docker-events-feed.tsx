"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  Ban,
  Clock,
  Container as ContainerIcon,
  Pause,
  Play,
  RefreshCw,
  RotateCcw,
  Search,
  Server,
  Skull,
  Square,
  Trash2,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import {
  DOCKER_EVENT_TYPES,
  MAX_DOCKER_EVENTS,
  fetchDockerEvents,
  type DockerEvent,
} from "@/lib/api/docker-events";

/**
 * Container-lifecycle timeline.
 *
 * Reads the cluster-wide feed that every Beacon node fills via
 * POST /api/remote/docker/events. There is no socket: the panel *polls*
 * GET /admin/docker/events every 5 seconds, so the worst-case delay from a
 * container dying on node "helix-2" to a row appearing here is one beacon flush
 * plus one poll. Rows are newest first and capped at MAX_DOCKER_EVENTS by
 * design: the feed is a "what just happened" view, and the admin activity log is
 * the place for history.
 */

const REFRESH_INTERVAL_MS = 5_000;
const RELATIVE_TICK_MS = 15_000;

type EventTone = {
  icon: typeof Activity;
  /** Badge classes: subtle background + solid token text. */
  badge: string;
  /** Left accent rail, solid so it stays visible on both themes. */
  accent: string;
};

// Green for a container that came up, red for the ones an operator acts on,
// amber for the reversible middle ground, grey for gone-and-not-coming-back.
function eventTone(type: string): EventTone {
  switch (type) {
    case "start":
      return {
        icon: Play,
        badge: "bg-[var(--success-subtle)] text-[var(--success)] border-[var(--success-subtle)]",
        accent: "border-l-[var(--success)]",
      };
    case "die":
      return {
        icon: Skull,
        badge: "bg-[var(--danger-subtle)] text-[var(--danger)] border-[var(--danger-subtle)]",
        accent: "border-l-[var(--danger)]",
      };
    case "kill":
      return {
        icon: Ban,
        badge: "bg-[var(--danger-subtle)] text-[var(--danger)] border-[var(--danger-subtle)]",
        accent: "border-l-[var(--danger)]",
      };
    case "oom":
      return {
        icon: AlertTriangle,
        badge: "bg-[var(--danger-subtle)] text-[var(--danger)] border-[var(--danger-subtle)]",
        accent: "border-l-[var(--danger)]",
      };
    case "stop":
      return {
        icon: Square,
        badge: "bg-[var(--warning-subtle)] text-[var(--warning)] border-[var(--warning-subtle)]",
        accent: "border-l-[var(--warning)]",
      };
    case "recreate":
      return {
        icon: RotateCcw,
        badge: "bg-[var(--warning-subtle)] text-[var(--warning)] border-[var(--warning-subtle)]",
        accent: "border-l-[var(--warning)]",
      };
    case "destroy":
      return {
        icon: Trash2,
        badge: "bg-[var(--surface-raised)] text-[var(--text-subtle)] border-[var(--line)]",
        accent: "border-l-[var(--text-muted)]",
      };
    default:
      return {
        icon: Activity,
        badge: "bg-[var(--surface-raised)] text-[var(--text-subtle)] border-[var(--line)]",
        accent: "border-l-[var(--line-strong)]",
      };
  }
}

function clockText(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString(undefined, {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// toISOString() throws on an unparseable date, and the tooltip is rendered for
// every row, so the guard has to live here rather than in the caller.
function isoText(iso: string): string {
  const stamp = new Date(iso).getTime();
  return Number.isNaN(stamp) ? iso : new Date(stamp).toISOString();
}

// Relative time is derived from a client-held clock rather than Date.now() at
// render time: the latter guarantees a hydration mismatch on every row.
function relativeText(iso: string, now: number | null): string {
  if (now == null) return "";
  const stamp = new Date(iso).getTime();
  if (Number.isNaN(stamp)) return "";
  const seconds = Math.max(0, Math.floor((now - stamp) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 2592000) return `${Math.floor(seconds / 86400)}d ago`;
  return clockText(iso);
}

function displayName(event: DockerEvent): string {
  return event.containerName || event.image || `${event.containerId.slice(0, 12)} (unnamed)`;
}

function exitCodeOf(event: DockerEvent): string | null {
  const raw = event.actorAttributes?.exitCode ?? event.actorAttributes?.exitCodeString;
  if (raw == null) return null;
  const value = String(raw).trim();
  return value.length > 0 ? value : null;
}

/** Attributes already shown on the row itself, so the expansion omits them. */
const HIDDEN_ATTRS = new Set(["name", "image", "id"]);

const PARAM_KEYS = { node: "node", type: "type", search: "q", paused: "paused" } as const;

/** Read the feed's filters out of the URL after mount (never during SSR, so the
 *  first paint cannot mismatch the hydrated tree). */
function readFilterParams(): { node: string; type: string; search: string; paused: boolean } {
  if (typeof window === "undefined") return { node: "", type: "", search: "", paused: false };
  const params = new URLSearchParams(window.location.search);
  return {
    node: params.get(PARAM_KEYS.node) ?? "",
    type: params.get(PARAM_KEYS.type) ?? "",
    search: params.get(PARAM_KEYS.search) ?? "",
    paused: params.get(PARAM_KEYS.paused) === "1",
  };
}

export function DockerEventsFeed({ className }: { className?: string }) {
  const [node, setNode] = useState("");
  const [type, setType] = useState("");
  const [search, setSearch] = useState("");
  const [paused, setPaused] = useState(false);
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [now, setNow] = useState<number | null>(null);

  // Filters live in the URL, so a feed showing one node's OOMs can be pasted
  // into an incident channel and the state survives a reload.
  const [restoredFilters] = useState(readFilterParams);
  useEffect(() => {
    setNode(restoredFilters.node);
    setType(restoredFilters.type);
    setSearch(restoredFilters.search);
    setPaused(restoredFilters.paused);
  }, [restoredFilters]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const params = new URLSearchParams(window.location.search);
    const write = (key: string, value: string) => {
      if (value) params.set(key, value);
      else params.delete(key);
    };
    write(PARAM_KEYS.node, node);
    write(PARAM_KEYS.type, type);
    write(PARAM_KEYS.search, search.trim() ? search : "");
    write(PARAM_KEYS.paused, paused ? "1" : "");
    const query = params.toString();
    window.history.replaceState(null, "", query ? `${window.location.pathname}?${query}` : window.location.pathname);
  }, [node, type, search, paused]);

  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const nodeNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const item of nodes) map.set(item.id, item.name);
    return map;
  }, [nodes]);

  const query = useQuery({
    queryKey: ["docker-events", node || null, type || null],
    queryFn: ({ signal }) =>
      fetchDockerEvents({
        node: node || undefined,
        type: type || undefined,
        limit: MAX_DOCKER_EVENTS,
        signal,
      }),
    refetchInterval: paused ? false : REFRESH_INTERVAL_MS,
    // Keep the previous page on screen while the next one is in flight, so the
    // feed ticks instead of flashing a spinner every 5 seconds.
    placeholderData: (prev) => prev,
  });

  useEffect(() => {
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), RELATIVE_TICK_MS);
    return () => clearInterval(timer);
  }, []);

  // Memoised on the query result, not on the array literal: `?? []` would create
  // a new identity every render and defeat the filters below.
  const events = useMemo(() => query.data?.events ?? [], [query.data]);

  const feedState = sourceState(query, paused ? undefined : REFRESH_INTERVAL_MS);

  // Nodes that are not reporting cannot contribute rows, so "the feed is quiet"
  // and "half the fleet is down" have to be distinguishable.
  const inactiveNodes = useMemo(() => nodes.filter((item) => item.status !== "active"), [nodes]);

  const visible = useMemo(() => {
    if (!search.trim()) return events;
    const needle = search.toLowerCase();
    return events.filter((event) =>
      `${event.containerName} ${event.containerId} ${event.image} ${event.nodeId} ${
        event.nodeName ?? ""
      } ${event.eventType}`
        .toLowerCase()
        .includes(needle),
    );
  }, [events, search]);

  const counts = useMemo(() => {
    let failing = 0;
    let starting = 0;
    for (const event of visible) {
      if (event.eventType === "die" || event.eventType === "kill" || event.eventType === "oom") failing += 1;
      else if (event.eventType === "start") starting += 1;
    }
    return { starting, failing };
  }, [visible]);

  return (
    <div className={cn("w-full space-y-3", className)}>
      <div className="flex flex-wrap items-center gap-2 rounded-xl border border-[var(--line)] bg-white/[0.015] px-3 py-2.5">
        <div className="flex items-center gap-1.5">
          <label
            htmlFor="docker-events-node"
            className="text-meta font-semibold uppercase tracking-wider text-[var(--text-subtle)]"
          >
            Node
          </label>
          <select
            id="docker-events-node"
            value={node}
            onChange={(event) => setNode(event.target.value)}
            className="h-7 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-2 text-xs text-[var(--text)]"
          >
            <option value="">All nodes</option>
            {nodes.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
                {item.status !== "active" ? ` (${item.status})` : ""}
              </option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-1.5">
          <label
            htmlFor="docker-events-type"
            className="text-meta font-semibold uppercase tracking-wider text-[var(--text-subtle)]"
          >
            Event
          </label>
          <select
            id="docker-events-type"
            value={type}
            onChange={(event) => setType(event.target.value)}
            className="h-7 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-2 text-xs text-[var(--text)]"
          >
            <option value="">All events</option>
            {DOCKER_EVENT_TYPES.map((item) => (
              <option key={item} value={item}>
                {item}
              </option>
            ))}
          </select>
        </div>

        <div className="relative ml-auto flex min-w-[200px] max-w-xs flex-1 items-center">
          <Search size={12} className="pointer-events-none absolute left-2.5 text-[var(--text-subtle)]" />
          <input
            aria-label="Filter events by container, image or node"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter this page…"
            className="h-7 w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-7 pr-7 text-xs text-[var(--text)]"
          />
          {search ? (
            <button
              type="button"
              aria-label="Clear filter"
              onClick={() => setSearch("")}
              className="absolute right-2 text-[var(--text-subtle)] hover:text-[var(--text)]"
            >
              <X size={12} />
            </button>
          ) : null}
        </div>

        <button
          type="button"
          onClick={() => setPaused((prev) => !prev)}
          className="inline-flex h-7 items-center gap-1.5 rounded-lg border border-[var(--line)] bg-white/[0.03] px-2.5 text-xs text-[var(--text)] transition hover:bg-[var(--surface-hover)]"
        >
          {paused ? <Play size={12} /> : <Pause size={12} />}
          {paused ? "Resume" : "Pause"}
        </button>
        <button
          type="button"
          onClick={() => void query.refetch()}
          className="inline-flex h-7 items-center gap-1.5 rounded-lg border border-[var(--line)] bg-white/[0.03] px-2.5 text-xs text-[var(--text)] transition hover:bg-[var(--surface-hover)]"
        >
          <RefreshCw size={12} className={query.isFetching ? "animate-spin" : undefined} />
          Refresh
        </button>
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--text-subtle)]">
        <span>
          {query.isLoading
            ? "Loading…"
            : `${visible.length} shown of ${events.length} loaded · ${query.data ? `${query.data.total} matched on the server` : "server count not read"}`}
        </span>
        {counts.failing > 0 ? <span className="text-[var(--danger)]">{counts.failing} failed/killed</span> : null}
        {counts.starting > 0 ? <span className="text-[var(--success)]">{counts.starting} started</span> : null}
        {/* Connection vocabulary, derived from the query itself. The badge used to
            pulse green and read "live 5s" while sitting next to its own error
            banner, because it was keyed off `paused` and nothing else. */}
        <span className="inline-flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className={cn(
              "h-1.5 w-1.5 rounded-full",
              feedState.status === "error"
                ? "bg-[var(--danger)]"
                : feedState.status === "restricted"
                  ? "bg-[var(--warning)]"
                  : paused
                    ? "bg-[var(--text-muted)]"
                    : feedState.stale
                      ? "bg-[var(--warning)]"
                      : "bg-[var(--success)]",
            )}
          />
          {feedState.status === "error"
            ? "Offline — the feed is not reading"
            : feedState.status === "restricted"
              ? "Permission restricted"
              : paused
                ? "Paused — not polling"
                : feedState.status === "loading"
                  ? "Connecting — first read pending"
                  : feedState.stale
                    ? "Stale — last poll did not return"
                    : `Polling every ${REFRESH_INTERVAL_MS / 1000}s`}
        </span>
        {paused ? null : <FreshnessBadge state={feedState} />}
        {nodesQuery.isError ? (
          <span className="text-[var(--warning)]">Node inventory unavailable — this feed cannot say which nodes are reporting.</span>
        ) : nodesQuery.isSuccess && inactiveNodes.length > 0 ? (
          <span className="text-[var(--warning)]">
            {inactiveNodes.length} of {nodes.length} {nodes.length === 1 ? "node is" : "nodes are"} not active
            {" "}({inactiveNodes.map((item) => item.name).join(", ")}) and reporting no events
            {node ? "" : " to this feed"}.
          </span>
        ) : null}
      </div>

      {query.isError ? (
        <div
          role="alert"
          className="rounded-xl border border-[var(--danger-subtle)] bg-[var(--danger-subtle)] px-4 py-3 text-sm text-[var(--danger)]"
        >
          Could not load the events feed: {(query.error as Error)?.message ?? "unknown error"}
        </div>
      ) : null}

      <div className="max-h-[62vh] overflow-y-auto rounded-xl border border-[var(--line)] bg-[var(--surface)] p-2">
        {query.isLoading ? (
          <ul aria-busy="true" aria-label="Loading container events" className="space-y-2" role="status">
            {[0, 1, 2, 3].map((row) => (
              <li aria-hidden="true" className="h-[46px] animate-pulse rounded-lg bg-white/[0.03]" key={row} />
            ))}
          </ul>
        ) : visible.length === 0 ? (
          query.isError ? null : (
            <div className="rounded-lg border border-dashed border-[var(--line)] bg-white/[0.02] px-6 py-10 text-center">
              <Clock size={16} className="mx-auto text-[var(--text-subtle)]" />
              <div className="mt-2 text-sm font-medium text-[var(--text)]">No container events</div>
              <div className="text-xs text-[var(--text-subtle)]">
                {search
                  ? "Nothing on this page matches the filter."
                  : "Beacon reports start, stop, die, kill, oom, recreate and destroy as they happen."}
              </div>
            </div>
          )
        ) : (
          <ol className="space-y-1.5">
            {visible.map((event) => (
              <DockerEventRow
                key={event.id}
                event={event}
                now={now}
                nodeName={event.nodeName || nodeNameById.get(event.nodeId) || `${event.nodeId.slice(0, 8)} (unknown node)`}
                expanded={expandedId === event.id}
                onToggle={() => setExpandedId((prev) => (prev === event.id ? null : event.id))}
              />
            ))}
          </ol>
        )}
      </div>

      <p className="text-meta leading-4 text-[var(--text-muted)]">
        Events are timestamped by the node that observed them, so a beacon whose clock drifts will appear out
        of order. This view holds the newest {MAX_DOCKER_EVENTS} matching rows; older events stay in the
        panel&apos;s store, whose retention window is enforced by the control plane and is not reported to this
        page.
      </p>
    </div>
  );
}

function DockerEventRow({
  event,
  nodeName,
  now,
  expanded,
  onToggle,
}: {
  event: DockerEvent;
  nodeName: string;
  now: number | null;
  expanded: boolean;
  onToggle: () => void;
}) {
  const tone = eventTone(event.eventType);
  const Icon = tone.icon;
  const exitCode = exitCodeOf(event);
  const attributes = useMemo(
    () => Object.entries(event.actorAttributes ?? {}).filter(([key]) => !HIDDEN_ATTRS.has(key)),
    [event.actorAttributes],
  );

  return (
    <li>
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        className={cn(
          "flex w-full items-start gap-3 rounded-lg border border-l-2 border-[var(--line)] bg-white/[0.02] px-3 py-2 text-left transition hover:bg-[var(--surface-hover)]",
          tone.accent,
        )}
      >
        <span
          className={cn(
            "mt-0.5 inline-flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-eyebrow font-semibold uppercase tracking-wider",
            tone.badge,
          )}
        >
          <Icon size={10} />
          {event.eventType}
        </span>

        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
            <span className="truncate text-sm font-medium text-[var(--text)]">{displayName(event)}</span>
            {event.image ? (
              <span className="truncate font-mono text-meta text-[var(--text-subtle)]">{event.image}</span>
            ) : null}
            {exitCode ? (
              <span
                className={cn(
                  "rounded border px-1 font-mono text-eyebrow",
                  exitCode === "0"
                    ? "border-[var(--line)] text-[var(--text-subtle)]"
                    : "border-[var(--danger-subtle)] bg-[var(--danger-subtle)] text-[var(--danger)]",
                )}
              >
                exit {exitCode}
              </span>
            ) : null}
          </span>
          <span className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-meta text-[var(--text-subtle)]">
            <span className="inline-flex items-center gap-1">
              <Server size={10} />
              {nodeName}
            </span>
            <span className="inline-flex items-center gap-1">
              <ContainerIcon size={10} />
              <span className="font-mono">{event.containerId.slice(0, 12)}</span>
            </span>
          </span>
        </span>

        <span
          className="shrink-0 whitespace-nowrap font-mono text-meta text-[var(--text-subtle)]"
          title={isoText(event.timestamp)}
        >
          {relativeText(event.timestamp, now) || clockText(event.timestamp)}
        </span>
      </button>

      {expanded ? (
        <div className="mt-1 rounded-lg border border-dashed border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2">
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-meta text-[var(--text-subtle)]">
            <span>
              ingested <span className="font-mono text-[var(--text)]">{clockText(event.ingestedAt)}</span>
            </span>
            <span>
              node <span className="font-mono text-[var(--text)]">{event.nodeId}</span>
            </span>
          </div>
          {attributes.length === 0 ? (
            <div className="mt-1 text-meta text-[var(--text-muted)]">No actor attributes reported.</div>
          ) : (
            <dl className="mt-2 grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2">
              {attributes.map(([key, value]) => (
                <div key={key} className="min-w-0">
                  <dt className="truncate font-semibold uppercase tracking-wider text-[var(--text-muted)] text-eyebrow">
                    {key}
                  </dt>
                  <dd className="break-all font-mono text-meta text-[var(--text)]">{value || "—"}</dd>
                </div>
              ))}
            </dl>
          )}
        </div>
      ) : null}
    </li>
  );
}

export default DockerEventsFeed;
