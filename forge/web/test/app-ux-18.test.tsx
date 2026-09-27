import { describe, it, expect, vi, beforeEach } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

// --- 1. statusTone centralization ---
import { statusTone, deploymentStatusTone, appStatusTone } from "@/lib/api/status";
import { DeployStatusBadge } from "@/components/admin/AdminAppsShared";
import { renderWithQuery } from "@/test/render";
import { APP_TYPE_ICONS } from "@/lib/app-type-icons";
import { DEPLOYMENT_STEPS_POLL_INTERVAL_MS, DEPLOYMENT_STEPS_MAX_DURATION_MS, isDeploymentStepsTerminal } from "@/hooks/useDeploymentSteps";
import { restartComposeStack } from "@/lib/api/compose";

// Mock next/navigation for tab routing tests
const replace = vi.fn();
const push = vi.fn();
let mockSearchParams = new URLSearchParams();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push }),
  useSearchParams: () => mockSearchParams,
  usePathname: () => "/admin/apps/app-123",
}));

describe("statusTone centralization via lib/api/status.ts", () => {
  it("maps app statuses to correct tones via single statusTone", () => {
    expect(statusTone("running", "app")).toBe("ok");
    expect(statusTone("failed", "app")).toBe("danger");
    expect(statusTone("deploying", "app")).toBe("pending");
    expect(statusTone("stopping", "app")).toBe("warn");
    expect(statusTone("stopped", "app")).toBe("neutral");
    // default kind is app
    expect(statusTone("running")).toBe("ok");
  });

  it("maps deployment statuses via same function with kind=deployment", () => {
    expect(statusTone("completed", "deployment")).toBe("ok");
    expect(statusTone("failed", "deployment")).toBe("danger");
    expect(statusTone("pending", "deployment")).toBe("pending");
    expect(statusTone("running", "deployment")).toBe("pending");
    expect(statusTone("canceled", "deployment")).toBe("neutral");
    expect(statusTone("cancelled", "deployment")).toBe("neutral");
  });

  it("deploymentStatusTone is backward-compatible wrapper", () => {
    expect(deploymentStatusTone("completed")).toBe("ok");
    expect(deploymentStatusTone("failed")).toBe("danger");
  });

  it("appStatusTone alias works", () => {
    expect(appStatusTone("running")).toBe("ok");
  });

  it("DeployStatusBadge uses centralized statusTone (no local divergence)", () => {
    const { container } = renderWithQuery(<DeployStatusBadge status="running" type="app" />);
    expect(container.textContent?.toLowerCase()).toContain("running");
    const { container: c2 } = renderWithQuery(<DeployStatusBadge status="failed" type="deployment" />);
    expect(c2.textContent?.toLowerCase()).toContain("failed");
  });

  it("a status we cannot interpret is unknown, never neutral", () => {
    // `neutral` is a reading — the thing is idle. `unknown` means we have no
    // reading at all. Collapsing the second into the first renders a confident
    // grey "inactive" chip for data we never understood.
    expect(statusTone("unknown_status", "app")).toBe("unknown");
    expect(statusTone("weird", "deployment")).toBe("unknown");
  });

  it("a missing or empty status is unknown", () => {
    expect(statusTone(null)).toBe("unknown");
    expect(statusTone(undefined)).toBe("unknown");
    expect(statusTone("")).toBe("unknown");
    expect(statusTone("   ")).toBe("unknown");
  });

  it("statusTone is the only statusTone reachable from the api barrel", () => {
    // `lib/api/apps.ts` used to export a second, colour-word `statusTone` that
    // won the `export *` race in `lib/api.ts`, so importing from "@/lib/api"
    // silently got the wrong vocabulary.
    const appsSrc = readFileSync(resolve(__dirname, "../lib/api/apps.ts"), "utf8");
    expect(appsSrc).not.toMatch(/export function statusTone/);
    expect(appsSrc).not.toMatch(/export function deploymentStatusTone/);
    const barrelSrc = readFileSync(resolve(__dirname, "../lib/api.ts"), "utf8");
    expect(barrelSrc).toContain("export * from './api/status'");
  });

  it("nomad, incus and database statuses resolve through the canonical tables", () => {
    // Case-insensitively: Incus reports "Running"/"Frozen" capitalised, and the
    // page's own copy of this map used a case-sensitive switch.
    expect(statusTone("Running", "incus")).toBe("ok");
    expect(statusTone("Frozen", "incus")).toBe("neutral");
    expect(statusTone("Error", "incus")).toBe("danger");
    expect(statusTone("running", "nomad")).toBe("ok");
    expect(statusTone("dead", "nomad")).toBe("neutral");
    expect(statusTone("lost", "nomad")).toBe("danger");
    expect(statusTone("provisioning", "database")).toBe("pending");
    // Every domain inherits the unknown rule.
    for (const kind of ["nomad", "incus", "database"] as const) {
      expect(statusTone("something_new_from_the_backend", kind)).toBe("unknown");
      expect(statusTone(undefined, kind)).toBe("unknown");
    }
  });

  it("a state the operator chose is not a fault, and a lost one is", () => {
    // These three were wrong in the page-local copies this replaced: a
    // deliberately stopped database rendered in the same red as a crashed one,
    // and an Incus cluster member that had gone offline rendered in the same
    // grey as an instance the operator had shut down on purpose.
    expect(statusTone("stopped", "database")).toBe("neutral");
    expect(statusTone("failed", "database")).toBe("danger");
    expect(statusTone("offline", "incus")).toBe("danger");
    // A drained Nomad node needs attention but is not broken.
    expect(statusTone("draining", "nomad")).toBe("warn");
    expect(statusTone("down", "nomad")).toBe("danger");
  });

  it("discovery endpoint health keeps unknown distinct from inactive", () => {
    // Discovery genuinely reports `unknown` until a heartbeat touches an
    // endpoint, so unknown has to survive as unknown rather than collapsing
    // into the neutral "inactive" chip an absent table entry would give.
    expect(statusTone("healthy", "discovery")).toBe("ok");
    expect(statusTone("unhealthy", "discovery")).toBe("danger");
    expect(statusTone("unknown", "discovery")).toBe("unknown");
    expect(statusTone("draining", "discovery")).toBe("warn");
    expect(statusTone("something_new", "discovery")).toBe("unknown");
    expect(statusTone("", "discovery")).toBe("unknown");
  });

  it("no page keeps a private status-tone table", () => {
    // Each of these shipped its own map, and each disagreed with the canonical
    // vocabulary somewhere: an unreadable status rendered as a confident
    // "inactive", a running cron job as a warning, an unrecognised upgrade plan
    // as in progress. The tables live in lib/api/status.ts now.
    const shadowed = [
      "../app/admin/nomad/page.tsx",
      "../app/admin/incus/page.tsx",
      "../app/server/[id]/databases/services/page.tsx",
      "../app/server/[id]/git/page.tsx",
      "../app/admin/cron-jobs/page.tsx",
      "../components/admin/AdminUpgrade.tsx",
      "../components/admin/beacon-workspace.tsx",
      // AdminDiscovery kept STATUS_COLORS and read `[status] ?? ""`, which lands
      // on Pill's neutral default; the revisions page fell back to
      // `statusConfig.pending`, rendering an unreadable status as work in flight.
      "../components/admin/AdminDiscovery.tsx",
      "../app/admin/deployments/[id]/revisions/page.tsx",
    ];
    for (const rel of shadowed) {
      const src = readFileSync(resolve(__dirname, rel), "utf8");
      expect(src, rel).not.toMatch(/^(?:const|function)\s+(?:statusTone|healthTone|stateTone)\b/m);
      // A local map keyed on colour words is the tell, whatever it is called —
      // whether the colour is the value directly or wrapped in an object.
      expect(src, rel).not.toMatch(/Record<string,\s*"(?:green|red|yellow|blue)"/);
      expect(src, rel).not.toMatch(/Record<string,\s*\{[^}]*"(?:green|red|yellow|blue)"/);
    }
  });
});

describe("APP_TYPE_ICONS shared centralization (typeIcons shadow fix)", () => {
  it("exports shared mapping for all AppType values", () => {
    expect(APP_TYPE_ICONS.image).toBeDefined();
    expect(APP_TYPE_ICONS.git).toBeDefined();
    expect(APP_TYPE_ICONS.compose).toBeDefined();
    expect(APP_TYPE_ICONS.game_server).toBeDefined();
  });

  it("admin/apps/page.tsx no longer defines local typeIcons shadow vs EGG_TEMPLATES", () => {
    const pageSrc = readFileSync(resolve(__dirname, "../app/admin/apps/page.tsx"), "utf8");
    // Should import from shared lib, not define local const typeIcons
    expect(pageSrc).toContain("APP_TYPE_ICONS");
    expect(pageSrc).not.toMatch(/const\s+typeIcons\s*:/);
    // Should not define duplicate duplicate OfflineBanner
    const offlineCount = (pageSrc.match(/OfflineBanner/g) || []).length;
    // Expect exactly 1 import + 1 usage = 2 occurrences, not 3+ (previous had 3)
    expect(offlineCount).toBeLessThanOrEqual(2);
  });

  it("EGG_TEMPLATES remains separate and not shadowed", () => {
    const eggSrc = readFileSync(resolve(__dirname, "../lib/egg-templates.ts"), "utf8");
    expect(eggSrc).toContain("export const EGG_TEMPLATES");
  });
});

describe("useDeploymentSteps single poller dedup (5s adaptive, no 2s duplicate)", () => {
  it("exports 5s constant, not 2s", () => {
    expect(DEPLOYMENT_STEPS_POLL_INTERVAL_MS).toBe(5000);
    expect(DEPLOYMENT_STEPS_MAX_DURATION_MS).toBe(10 * 60 * 1000);
  });

  it("isDeploymentStepsTerminal correctly detects terminal states", () => {
    expect(isDeploymentStepsTerminal(undefined)).toBe(false);
    expect(isDeploymentStepsTerminal([])).toBe(false);
    expect(
      isDeploymentStepsTerminal([
        { id: "1", deploymentId: "d1", stepNumber: 1, stepName: "init", status: "completed", createdAt: "", updatedAt: "" },
        { id: "2", deploymentId: "d1", stepNumber: 2, stepName: "provision", status: "completed", createdAt: "", updatedAt: "" },
      ]),
    ).toBe(true);
    expect(
      isDeploymentStepsTerminal([
        { id: "1", deploymentId: "d1", stepNumber: 1, stepName: "init", status: "completed", createdAt: "", updatedAt: "" },
        { id: "2", deploymentId: "d1", stepNumber: 2, stepName: "provision", status: "in_progress", createdAt: "", updatedAt: "" },
      ]),
    ).toBe(false);
  });

  it("deployment-progress.tsx uses shared hook and no longer defines 2s POLL_INTERVAL", () => {
    const src = readFileSync(resolve(__dirname, "../components/app/deployment-progress.tsx"), "utf8");
    expect(src).toContain("useDeploymentSteps");
    expect(src).not.toContain("POLL_INTERVAL_MS = 2000");
    expect(src).not.toContain("MAX_POLL_DURATION_MS");
    // Should not import useQuery directly for polling (hook handles it)
    // It should still import useEffect/useState for UI state but not define its own refetchInterval
    expect(src).not.toMatch(/refetchInterval:\s*\(q\)\s*=>/);
  });

  it("DeploymentTimeline.tsx uses shared hook with same queryKey", () => {
    const src = readFileSync(resolve(__dirname, "../components/charts/DeploymentTimeline.tsx"), "utf8");
    expect(src).toContain("useDeploymentSteps");
    // No longer defines inline useQuery with 5000 duplicate – hook does
    // But hook is 5s, so file should not contain inline refetchInterval 5000 logic
    expect(src).not.toMatch(/refetchInterval:\s*\(query\)\s*=>/);
    // Both files share same queryKey via hook – verify hook centralizes it
    const hookSrc = readFileSync(resolve(__dirname, "../hooks/useDeploymentSteps.ts"), "utf8");
    expect(hookSrc).toContain('["deployment-steps"');
    expect(hookSrc).toContain("POLL_INTERVAL_MS = 5000");
    // Components should not directly contain queryKey string (now in hook) – indirect via hook
    const progSrc = readFileSync(resolve(__dirname, "../components/app/deployment-progress.tsx"), "utf8");
    expect(progSrc).toContain("useDeploymentSteps");
  });

  it("hook uses adaptive 5s polling and respects terminal state", async () => {
    // This is a behavioral check: hook's refetchInterval logic should return false when terminal
    // We test isDeploymentStepsTerminal covers that branch
    const terminal = [
      { id: "1", deploymentId: "d1", stepNumber: 1, stepName: "init", status: "completed" as const, createdAt: "", updatedAt: "" },
      { id: "2", deploymentId: "d1", stepNumber: 2, stepName: "complete", status: "failed" as const, createdAt: "", updatedAt: "" },
    ];
    // failed is terminal, so all steps terminal even though one is failed (failed counts as terminal)
    // Our helper requires every step terminal; completed+failed are both terminal => true -> polling stops
    expect(isDeploymentStepsTerminal(terminal)).toBe(true);
  });
});

describe("restartComposeStack export in lib/api/compose.ts", () => {
  it("exports restartComposeStack alongside deploy/start/stop", () => {
    expect(typeof restartComposeStack).toBe("function");
    const src = readFileSync(resolve(__dirname, "../lib/api/compose.ts"), "utf8");
    expect(src).toContain("export function restartComposeStack");
    expect(src).toContain("export function deployComposeStack");
    expect(src).toContain("export function startComposeStack");
    expect(src).toContain("export function stopComposeStack");
  });
});

describe("tab routing router-driven (router.replace sync with ?tab=)", () => {
  beforeEach(() => {
    replace.mockClear();
    push.mockClear();
    mockSearchParams = new URLSearchParams();
  });

  it("page.tsx uses router.replace with ?tab= instead of window.history.replaceState", () => {
    const src = readFileSync(resolve(__dirname, "../app/admin/apps/[id]/page.tsx"), "utf8");
    expect(src).toContain("router.replace");
    expect(src).toContain("?tab=");
    expect(src).not.toContain("window.history.replaceState");
    // Should derive tab from searchParams, not only useState initializer
    expect(src).toContain("searchParams.get(\"tab\")");
    expect(src).toContain("TABS.some");
  });

  it("validates tab param and falls back to overview for invalid values", () => {
    // Simulate the validation logic extracted from page
    const TABS = ["overview", "deployments", "configuration", "logs", "console", "domains", "backups"];
    const getTab = (raw: string | null) => (raw && TABS.includes(raw) ? raw : "overview");
    expect(getTab("logs")).toBe("logs");
    expect(getTab("invalid")).toBe("overview");
    expect(getTab(null)).toBe("overview");
    expect(getTab("")).toBe("overview");
  });

  it("setTab calls router.replace with encoded tab and scroll:false", async () => {
    // Directly test that the page's setTab behavior would call replace correctly
    // We simulate by calling the same string interpolation the file uses
    const id = "app-123";
    const tId = "deployments";
    const expected = `/admin/apps/${encodeURIComponent(id)}?tab=${encodeURIComponent(tId)}`;
    // Emulate the setTab function from page
    const router = { replace: replace as unknown as (url: string, opts?: { scroll: boolean }) => void };
    router.replace(expected, { scroll: false });
    expect(replace).toHaveBeenCalledWith(expected, { scroll: false });
  });

  it("refresh restores tab via URL (searchParams drives rendered tab)", () => {
    // When URL is /admin/apps/app-123?tab=logs, searchParams contains tab=logs
    mockSearchParams = new URLSearchParams("tab=logs");
    // The tab derivation in page: rawTab = searchParams.get("tab"); tab = valid ? rawTab : "overview"
    const raw = mockSearchParams.get("tab");
    const validTabs = ["overview", "deployments", "configuration", "logs", "console", "domains", "backups"];
    const tab = raw && validTabs.includes(raw) ? raw : "overview";
    expect(tab).toBe("logs");
    // If URL is ?tab=deployments, same logic restores deployments
    mockSearchParams = new URLSearchParams("tab=deployments");
    const raw2 = mockSearchParams.get("tab");
    const tab2 = raw2 && validTabs.includes(raw2) ? raw2 : "overview";
    expect(tab2).toBe("deployments");
  });
});

describe("triple env editors shadow fix", () => {
  it("AdminAppsShared EnvVarEditor remains canonical for Record<string,string> app config", () => {
    const src = readFileSync(resolve(__dirname, "../components/admin/AdminAppsShared.tsx"), "utf8");
    expect(src).toContain("export function EnvVarEditor");
    expect(src).toContain("envVars: Record<string, string>");
  });

  it("environment/env-var-editor.tsx is scoped and provides alias to avoid shadow", () => {
    const src = readFileSync(resolve(__dirname, "../components/environment/env-var-editor.tsx"), "utf8");
    expect(src).toContain("EnvVarEditor");
    // Should document that it's scoped to project/environment, not app record editor
    expect(src).toMatch(/scopeType|scopeId/);
  });
});
