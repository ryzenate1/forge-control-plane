import { readFileSync, existsSync, readdirSync, statSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  SERVERS_LIST_HREF,
  consoleNavGroups,
  workloadTabGroups,
  workloadTabHref,
  workloadTabs,
  type WorkloadTabId,
} from "@/components/console/console-registry";
import {
  ADMIN_ALIAS_ROUTES,
  adminNavEntries,
  adminPageRegistry,
} from "@/components/admin/admin-registry";

/**
 * Every href the console shell advertises must resolve to a real page.
 *
 * This replaces the previous guard in components/console/console-registry.test.ts,
 * which resolved hrefs by pasting them under `app/console/` regardless of what
 * they actually pointed at. That assumption is what let the registry ship
 * `/console/servers`, `/console/backups` and 19 `/console/servers/[id]/<tab>`
 * hrefs — none of which had a page anywhere — while the registry's own doc
 * comment claimed a 404 was structurally impossible.
 *
 * The invariant asserted here is the real one: walk `app/` the way the Next.js
 * App Router does, honouring dynamic (`[id]`) and group (`(marketing)`)
 * segments, and require a `page.tsx` at the end.
 */

const APP_DIR = path.resolve(__dirname, "../app");
const PLACEHOLDER_ID = "route-integrity-placeholder";

function childDirs(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir).filter((entry) => statSync(path.join(dir, entry)).isDirectory());
}

/** Directories the router traverses without consuming a URL segment. */
function transparentDirs(dir: string): string[] {
  return childDirs(dir).filter((entry) => entry.startsWith("(") && entry.endsWith(")"));
}

function matchSegment(dir: string, segment: string): string[] {
  const candidates: string[] = [];
  const entries = childDirs(dir);
  if (entries.includes(segment)) candidates.push(path.join(dir, segment));
  for (const entry of entries) {
    // [id], [slug], [...rest], [[...rest]] all match a single concrete segment.
    if (entry.startsWith("[") && entry.endsWith("]")) candidates.push(path.join(dir, entry));
  }
  return candidates;
}

/**
 * Resolve a URL path to the directories that could serve it, mirroring the
 * App Router: literal match first, then a dynamic segment, descending through
 * route groups at every level.
 */
function resolveRoute(href: string): boolean {
  const segments = href.split("/").filter(Boolean);
  let frontier = [APP_DIR, ...transparentDirs(APP_DIR)];

  for (const segment of segments) {
    const next: string[] = [];
    for (const dir of frontier) {
      for (const match of matchSegment(dir, segment)) {
        next.push(match, ...transparentDirs(match));
      }
    }
    if (next.length === 0) return false;
    frontier = next;
  }

  return frontier.some((dir) => existsSync(path.join(dir, "page.tsx")));
}

describe("resolveRoute mirrors the App Router", () => {
  it("accepts a literal route that exists", () => {
    expect(resolveRoute("/console")).toBe(true);
  });

  it("accepts a dynamic segment", () => {
    expect(resolveRoute(`/server/${PLACEHOLDER_ID}/files`)).toBe(true);
  });

  it("rejects a route with no page", () => {
    expect(resolveRoute("/console/definitely-not-a-route")).toBe(false);
  });

  it("rejects a directory that exists but has no page.tsx", () => {
    // /server/<id> is a real page; /server on its own is not.
    expect(resolveRoute("/server")).toBe(false);
  });
});

describe("console navigation advertises only real routes", () => {
  it.each(consoleNavGroups.flatMap((group) => group.items).map((item) => [item.href] as const))(
    "top-level nav entry %s resolves to a page",
    (href) => {
      expect(resolveRoute(href), `${href} does not resolve to a page under app/`).toBe(true);
    },
  );

  it("the servers list href resolves", () => {
    expect(resolveRoute(SERVERS_LIST_HREF), `${SERVERS_LIST_HREF} does not resolve`).toBe(true);
  });

  it.each(workloadTabs.map((tab) => [tab.id] as const))("workload tab %s resolves to a page", (id) => {
    const href = workloadTabHref(PLACEHOLDER_ID, id);
    expect(resolveRoute(href), `tab "${id}" links to ${href}, which has no page`).toBe(true);
  });

  it("every tab in a group is a declared tab", () => {
    const declared = new Set<WorkloadTabId>(workloadTabs.map((tab) => tab.id));
    const orphaned = workloadTabGroups.flatMap((group) => group.tabs).filter((id) => !declared.has(id));
    expect(orphaned, `group references unknown tab ids: ${orphaned.join(", ")}`).toEqual([]);
  });

  it("every declared tab is reachable from a group", () => {
    // A tab absent from every group is rendered by nothing — dead config that
    // still claims a permission set.
    const grouped = new Set<WorkloadTabId>(workloadTabGroups.flatMap((group) => group.tabs));
    const unreachable = workloadTabs.map((tab) => tab.id).filter((id) => !grouped.has(id));
    expect(unreachable, `tabs declared but in no group: ${unreachable.join(", ")}`).toEqual([]);
  });

  it("nav labelKeys live under the console. namespace and that namespace is populated", () => {
    const en = JSON.parse(
      readFileSync(path.resolve(__dirname, "../../../lang/en.json"), "utf8"),
    ) as Record<string, unknown>;
    const consoleBundle = (en.console ?? {}) as Record<string, string>;
    const misplaced = consoleNavGroups
      .flatMap((group) => group.items)
      .filter((item) => item.labelKey && !item.labelKey.startsWith("console."))
      .map((item) => item.href);
    expect(misplaced, "nav labelKeys must live under the console. namespace").toEqual([]);
    // An empty lang block silently degrades every label to its hardcoded fallback.
    expect(Object.keys(consoleBundle).length).toBeGreaterThan(0);
  });
});

/**
 * The same invariant for the admin shell.
 *
 * admin-registry.ts's doc comment claimed these were "enforced by
 * test/navigation.test.ts". That file has never existed, so the ~88-entry admin
 * registry has been unguarded the whole time — which is how it came to advertise
 * `/admin/backups/engines`, a nav row with no page behind it. Asserted here,
 * against the same App Router resolver the console uses.
 */
describe("admin navigation advertises only real routes", () => {
  const entries = adminNavEntries();

  it("the registry is non-empty (a bad import would vacuously pass every case below)", () => {
    expect(entries.length).toBeGreaterThan(50);
  });

  it.each(entries.map((entry) => [entry.href, entry.label] as const))(
    "admin entry %s (%s) resolves to a page",
    (href) => {
      expect(resolveRoute(href), `${href} does not resolve to a page under app/`).toBe(true);
    },
  );

  it("no href is registered twice", () => {
    const seen = new Map<string, number>();
    for (const entry of entries) seen.set(entry.href, (seen.get(entry.href) ?? 0) + 1);
    const duplicated = [...seen.entries()].filter(([, count]) => count > 1).map(([href]) => href);
    expect(duplicated, `hrefs registered more than once: ${duplicated.join(", ")}`).toEqual([]);
  });

  it("every parent points at a registered href", () => {
    const registered = new Set(entries.map((entry) => entry.href));
    const dangling = entries
      .filter((entry) => entry.parent && !registered.has(entry.parent))
      .map((entry) => `${entry.href} -> ${entry.parent}`);
    expect(dangling, `parent hrefs that are not registered: ${dangling.join(", ")}`).toEqual([]);
  });

  it("no entry is its own parent", () => {
    const selfParented = entries.filter((entry) => entry.parent === entry.href).map((e) => e.href);
    expect(selfParented).toEqual([]);
  });

  it.each(Object.entries(ADMIN_ALIAS_ROUTES))(
    "alias %s targets a registered, resolvable href",
    (alias, target) => {
      const registered = new Set(entries.map((entry) => entry.href));
      expect(registered.has(target), `alias ${alias} targets ${target}, which is not registered`).toBe(true);
      expect(resolveRoute(target), `alias ${alias} targets ${target}, which has no page`).toBe(true);
    },
  );

  it("no alias shadows a real registered route", () => {
    // An alias for an href that is itself registered would redirect a working
    // page somewhere else.
    const registered = new Set(entries.map((entry) => entry.href));
    const shadowing = Object.keys(ADMIN_ALIAS_ROUTES).filter((alias) => registered.has(alias));
    expect(shadowing, `aliases shadowing real routes: ${shadowing.join(", ")}`).toEqual([]);
  });

  it("every alias source is served by a real page, so no bookmark is a 404", () => {
    // ADMIN_ALIAS_ROUTES is consulted by active-state and breadcrumb resolution
    // only, so an alias *looks* like it works — the sidebar highlights the right
    // row — even when nothing serves the URL. That gap let most of the map ship
    // as dead links: a renamed section turned every old bookmark and deep link
    // into a 404 behind a correctly-highlighted nav entry. Routing is not
    // changed by the map, so the only thing that makes an alias load is a real
    // redirect page under app/.
    const dead = Object.keys(ADMIN_ALIAS_ROUTES).filter((alias) => !resolveRoute(alias));
    expect(dead, `alias paths with no page: ${dead.join(", ")}`).toEqual([]);
  });

  it("every group has a title and at least one item", () => {
    const empty = adminPageRegistry
      .filter((group) => !group.title || group.items.length === 0)
      .map((group) => group.title || "(untitled)");
    expect(empty, `empty or untitled nav groups: ${empty.join(", ")}`).toEqual([]);
  });
});
