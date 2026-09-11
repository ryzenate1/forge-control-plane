// Tags/labels API client.
//
// A tag is a color-coded label (hex color stored as data, never as a CSS class)
// that can be attached to applications, servers, and environments. These calls
// back the /api/v1/tags catalog, the per-resource assignment routes, and the
// tag-driven bulk operations. All requests go through the canonical HTTP
// primitives in ./http so CSRF signing, cookie credentials and the 401
// session-expiry signal are handled in exactly one place.

import { deleteJSON, fetchJSON, patchJSON, postJSON, unwrapList } from './http';

/** Kinds of objects a tag can be attached to. Mirrors the backend enum. */
export type TagResourceType = 'application' | 'server' | 'environment';

export interface Tag {
  id: string;
  name: string;
  /** 6-digit hex color, e.g. "#1a2b3c". Rendered via a sanitized inline style. */
  color: string;
  description: string;
  createdAt: string;
}

/** One resource carrying a tag; the mixed list backs bulk operations. */
export interface TaggedResource {
  resourceType: string;
  resourceId: string;
  name: string;
}

export interface CreateTagInput {
  name: string;
  /** Optional; the backend applies a default when omitted. */
  color?: string;
  description?: string;
}

export interface UpdateTagInput {
  name?: string;
  color?: string;
  description?: string;
}

export type TagBulkAction = 'start' | 'stop' | 'restart' | 'deploy';

export interface TagBulkResult {
  resourceType: string;
  resourceId: string;
  name: string;
  ok: boolean;
  error?: string;
}

export interface TagBulkResponse {
  data: TagBulkResult[];
  total: number;
}

/** The backend wraps list payloads in `{ data: [...] }`; the canonical
 * {@link unwrapList} also throws on an unexpected shape instead of rendering
 * it as an empty list. */
type ListEnvelope<T> = { data?: T[] } | T[];

/** URL segment per resource type (the assignment routes are registered per
 * concrete kind: /applications/:id/tags, /servers/:id/tags, ...). */
const RESOURCE_SEGMENTS: Record<TagResourceType, string> = {
  application: 'applications',
  server: 'servers',
  environment: 'environments',
};

function assignmentPath(resourceType: TagResourceType, resourceId: string): string {
  const segment = RESOURCE_SEGMENTS[resourceType];
  if (!segment) {
    // Fail closed: an unknown kind would build a malformed URL.
    throw new Error(`unsupported tag resource type: ${resourceType}`);
  }
  return `/${segment}/${encodeURIComponent(resourceId)}/tags`;
}

// ---- Catalog --------------------------------------------------------------

export async function fetchTags(): Promise<Tag[]> {
  return unwrapList(await fetchJSON<ListEnvelope<Tag>>('/tags'));
}

export async function fetchTag(tagId: string): Promise<Tag> {
  return fetchJSON<Tag>(`/tags/${encodeURIComponent(tagId)}`);
}

export async function createTag(input: CreateTagInput): Promise<Tag> {
  return postJSON<Tag>('/tags', input);
}

export async function updateTag(tagId: string, input: UpdateTagInput): Promise<Tag> {
  return patchJSON<Tag>(`/tags/${encodeURIComponent(tagId)}`, input);
}

export async function deleteTag(tagId: string): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(`/tags/${encodeURIComponent(tagId)}`);
}

// ---- Tag → resources ------------------------------------------------------

/** Every resource carrying a tag, optionally narrowed to a single kind. */
export async function fetchTagResources(
  tagId: string,
  resourceType?: TagResourceType,
): Promise<TaggedResource[]> {
  const query = resourceType ? `?resourceType=${encodeURIComponent(resourceType)}` : '';
  const body = await fetchJSON<ListEnvelope<TaggedResource>>(
    `/tags/${encodeURIComponent(tagId)}/resources${query}`,
  );
  return unwrapList(body);
}

/** Dispatch a lifecycle action against every resource tagged (admin-only). */
export async function bulkActionByTag(
  tagId: string,
  action: TagBulkAction,
  resourceType?: TagResourceType,
): Promise<TagBulkResponse> {
  return postJSON<TagBulkResponse>(`/tags/${encodeURIComponent(tagId)}/bulk`, {
    action,
    resourceType: resourceType ?? '',
  });
}

// ---- Resource → tags (assignment) -----------------------------------------

export async function fetchResourceTags(
  resourceType: TagResourceType,
  resourceId: string,
): Promise<Tag[]> {
  const body = await fetchJSON<ListEnvelope<Tag>>(assignmentPath(resourceType, resourceId));
  return unwrapList(body);
}

export async function assignTag(
  resourceType: TagResourceType,
  resourceId: string,
  tagId: string,
): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(assignmentPath(resourceType, resourceId), { tagId });
}

export async function unassignTag(
  resourceType: TagResourceType,
  resourceId: string,
  tagId: string,
): Promise<{ ok: boolean }> {
  return deleteJSON<{ ok: boolean }>(
    `${assignmentPath(resourceType, resourceId)}/${encodeURIComponent(tagId)}`,
  );
}
