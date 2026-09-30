import { describe, expect, it } from "vitest";
import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { workloadTabGroups, workloadTabHref, workloadTabs } from "@/components/console/console-registry";

const ID = "00000000-0000-0000-0000-000000000000";

describe("domains tab wiring", () => {
  it("declares the domains tab as admin-only", () => {
    const tab = workloadTabs.find((t) => t.id === "domains");
    expect(tab).toBeDefined();
    expect(tab?.adminOnly).toBe(true);
  });
  it("resolves to a real page file", () => {
    const href = workloadTabHref(ID, "domains");
    expect(href).toBe(`/server/${ID}/domains`);
    expect(existsSync(resolve(__dirname, `../app/server/[id]/domains/page.tsx`))).toBe(true);
  });
  it("is reachable from a group and every group tab is declared", () => {
    const declared = new Set(workloadTabs.map((t) => t.id));
    const grouped = workloadTabGroups.flatMap((g) => g.tabs);
    expect(grouped).toContain("domains");
    expect(grouped.filter((id) => !declared.has(id))).toEqual([]);
  });
});
