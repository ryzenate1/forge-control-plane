import { fetchJSON } from "./http";

/**
 * Client for the real-time Docker events feed (GET /api/v1/admin/docker/events).
 *
 * The panel stores container lifecycle events reported by every Beacon node, so
 * this is the cluster-wide "what just happened to a container" timeline. Reads
 * are admin-only; the node-facing ingest route is not reachable from the browser
 * and is deliberately not mirrored here.
 */

/** Lifecycle actions Beacon forwards. Mirrors dockerEventActions on the node. */
export const DOCKER_EVENT_TYPES = [
  "start",
  "stop",
  "die",
  "kill",
  "oom",
  "recreate",
  "destroy",
] as const;

export type DockerEventType = (typeof DOCKER_EVENT_TYPES)[number] | string;

export type DockerEvent = {
  id: number;
  nodeId: string;
  /** Joined from the nodes table; absent if the node row was deleted. */
  nodeName?: string;
  eventType: DockerEventType;
  containerId: string;
  containerName: string;
  image: string;
  /** events.Message.Actor.Attributes as reported by the node. */
  actorAttributes?: Record<string, string>;
  /** When Docker observed the event (node clock), not when the panel stored it. */
  timestamp: string;
  ingestedAt: string;
};

export type DockerEventsResponse = {
  events: DockerEvent[];
  total: number;
  limit: number;
  offset: number;
};

export type DockerEventFilter = {
  /** Node uuid. */
  node?: string;
  /** Exact lifecycle action, e.g. "oom". */
  type?: string;
  /** Case-insensitive substring of the container name or id. */
  container?: string;
  /** RFC3339 bounds on the Docker-side event timestamp. */
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
  signal?: AbortSignal;
};

/**
 * MAX_DOCKER_EVENTS is the feed's own ceiling: the panel will serve larger
 * pages, but a live timeline that renders thousands of rows is neither readable
 * nor cheap to re-render every refresh, so the component asks for 200.
 */
export const MAX_DOCKER_EVENTS = 200;

export async function fetchDockerEvents(
  filter: DockerEventFilter = {},
): Promise<DockerEventsResponse> {
  const params = new URLSearchParams();
  if (filter.node) params.set("node", filter.node);
  if (filter.type) params.set("type", filter.type);
  if (filter.container) params.set("container", filter.container);
  if (filter.from) params.set("from", filter.from);
  if (filter.to) params.set("to", filter.to);
  params.set("limit", String(Math.min(filter.limit ?? MAX_DOCKER_EVENTS, MAX_DOCKER_EVENTS)));
  if (filter.offset && filter.offset > 0) params.set("offset", String(filter.offset));

  const query = params.toString();
  const response = await fetchJSON<Partial<DockerEventsResponse> | DockerEvent[]>(
    `/admin/docker/events${query ? `?${query}` : ""}`,
    { signal: filter.signal },
  );

  // The panel answers {events,total,limit,offset}; a bare array is tolerated so a
  // future handler simplification does not blank the feed.
  if (Array.isArray(response)) {
    return { events: response, total: response.length, limit: response.length, offset: 0 };
  }
  return {
    events: response.events ?? [],
    total: response.total ?? (response.events?.length ?? 0),
    limit: response.limit ?? (filter.limit ?? MAX_DOCKER_EVENTS),
    offset: response.offset ?? 0,
  };
}
