import { readFileSync, existsSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  consoleNavGroups,
  workloadTabGroups,
  workloadTabs,
  type WorkloadTabId,
} from "./console-registry";

/**
 * Structural guards for the console registry.
 *
 * The registry's own doc comment claims it is "structurally impossible for the
 * nav to advertise a route that 404s". These tests make that claim true rather
 * than aspirational: the console sidebar renders both the top-level groups and
 * the per-workload tabs, so an entry without a page is a user-visible dead link.
 *
 * This exists because `git` and `processes` were listed as tabs with no page
 * under app/console/servers/[id]/, and nothing caught it.
 */

const APP_DIR = path.resolve(__dirname, "../../app");

function consolePageExists(href: string): boolean {
  // /console            -> app/console/page.tsx
  // /console/servers    -> app/console/servers/page.tsx
  // /console/apps/[id]  -> app/console/apps/[id]/page.tsx
  const relative = href.replace(/^\/console/, "") || "";
  const asIndex = path.join(APP_DIR, "console", relative, "page.tsx");
  const asFile = path.join(APP_DIR, "console", `${relative}.tsx`);
  return existsSync(asIndex) || existsSync(asFile);
}

function consoleLayoutExists(dirRelative: string): boolean {
  return existsSync(path.join(APP_DIR, "console", dirRelative, "layout.tsx"));
}

describe("console registry routes are actually wired", () => {
  it.each(
    consoleNavGroups.flatMap((group) => group.items).map((item) => [item.href, item] as const),
  )("top-level nav entry %s has a page", (_href, item) => {
    expect(consolePageExists(item.href), `${item.href} has no page.tsx under app/console/`).toBe(true);
  });

  it("every workload tab has a page under app/console/servers/[id]/", () => {
    const implemented = new Set<string>();
    for (const id of allTabIds()) {
      const dir = id === "overview" ? "" : `/${id}`;
      const pagePath = path.join(APP_DIR, "console", "servers", "[id]", dir, "page.tsx");
      if (existsSync(pagePath)) implemented.add(id);
    }
    const missing = allTabIds().filter((id) => !implemented.has(id));
    expect(missing, `tabs rendered by ConsoleNav but with no page: ${missing.join(", ")}`).toEqual([]);
  });

  it("every tab in a group is a declared tab", () => {
    const declared = new Set(workloadTabs.map((tab) => tab.id));
    const orphaned = workloadTabGroups
      .flatMap((group) => group.tabs)
      .filter((id) => !declared.has(id));
    expect(orphaned, `group references unknown tab ids: ${orphaned.join(", ")}`).toEqual([]);
  });

  it("the server-detail segment owns a layout that provides the shell", () => {
    // Without this layout every tab page mounts its own ServerConsoleLayout,
    // producing a second sidebar inside the console shell.
    expect(
      consoleLayoutExists(path.join("servers", "[id]")),
      "app/console/servers/[id]/layout.tsx is missing — tabs would render duplicate navigation",
    ).toBe(true);
  });

  it("nav labels are translated or fall back to a readable default", () => {
    const en = JSON.parse(readFileSync(path.resolve(__dirname, "../../../../lang/en.json"), "utf8")) as Record<string, unknown>;
    const consoleBundle = (en.console ?? {}) as Record<string, string>;
    const missing = consoleNavGroups
      .flatMap((group) => group.items)
      .filter((item) => item.labelKey && !item.labelKey.startsWith("console."))
      .map((item) => item.href);
    expect(missing, "nav labelKeys must live under the console. namespace").toEqual([]);
    // The namespace itself must not be empty — an empty lang block silently
    // degrades every label to its hardcoded fallback.
    expect(Object.keys(consoleBundle).length).toBeGreaterThan(0);
  });
});

function allTabIds(): WorkloadTabId[] {
  return workloadTabGroups.flatMap((group) => group.tabs);
}
