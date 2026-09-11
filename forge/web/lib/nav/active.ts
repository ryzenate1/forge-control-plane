/**
 * Shared active-route resolution for every Forge navigation surface.
 *
 * There is exactly one rule, used by the admin sidebar, the console sidebar and
 * the resource-scoped server nav: **longest matching href wins, and only one
 * item is ever active.** The previous per-surface `pathname.startsWith(href)`
 * checks highlighted every ancestor at once (`/admin/backups` *and*
 * `/admin/backups/engines`), and the console's exact-match variant highlighted
 * nothing on deep sub-paths.
 *
 * Matching is segment-aware: `/admin/node` must not match `/admin/nodes`.
 */

/** True when `pathname` is `href` itself or a descendant path of it. */
export function isPathWithin(pathname: string, href: string): boolean {
  if (pathname === href) return true;
  if (href === "/") return pathname.startsWith("/");
  return pathname.startsWith(`${href}/`);
}

/**
 * Pick the single most specific candidate for `pathname`.
 * Returns `undefined` when nothing matches — callers must not fall back to
 * "first item", which would silently mislabel the page.
 */
export function resolveActive<T extends { href: string }>(
  pathname: string,
  candidates: readonly T[],
): T | undefined {
  let best: T | undefined;
  for (const candidate of candidates) {
    if (!isPathWithin(pathname, candidate.href)) continue;
    if (!best || candidate.href.length > best.href.length) best = candidate;
  }
  return best;
}

/** Convenience wrapper for callers that only need the winning href. */
export function resolveActiveHref(pathname: string, hrefs: readonly string[]): string | undefined {
  return resolveActive(
    pathname,
    hrefs.map((href) => ({ href })),
  )?.href;
}

/**
 * Turn an unknown URL segment into something a human can read.
 * Used for the trailing crumb of detail routes (`/admin/nodes/<id>`), where the
 * registry cannot know the label ahead of time. Opaque ids are returned
 * verbatim rather than guessed at — a wrong-but-confident label is worse than
 * the raw value.
 */
export function humanizeSegment(segment: string): string {
  const decoded = safeDecode(segment);
  if (looksLikeOpaqueId(decoded)) return decoded;
  return decoded
    .split(/[-_]/)
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function looksLikeOpaqueId(value: string): boolean {
  if (/^\d+$/.test(value)) return true;
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)) return true;
  // Long unhyphenated hex/base-ish tokens are ids, not words.
  return value.length >= 12 && !/[-_ ]/.test(value) && /\d/.test(value);
}
