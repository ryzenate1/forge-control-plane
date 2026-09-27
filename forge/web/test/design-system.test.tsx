/**
 * Design system & shared components — Phase 08 Agent 15
 *
 * Covers:
 * - Tokens exist (brand/canvas/line/text etc.)
 * - No hardcoded bg-[#...] remaining (except blurple)
 * - Space Grotesk / IBM Plex Sans / JetBrains Mono loaded via next/font
 * - generation-fenced-dot renders two-dot badge correctly
 *
 * Canonical sources:
 * - forge/web/lib/design-tokens.ts:1
 * - forge/web/app/globals.css:5
 * - forge/web/tailwind.config.ts
 * - forge/web/components/shared/* (states-empty, states-loading, generation-fenced-dot)
 */

import { readFileSync, existsSync, readdirSync, statSync } from "node:fs";
import { resolve, join, relative } from "node:path";
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  brand,
  canvas,
  line,
  text,
  status,
  shadow,
  radius,
  tokens,
  colors,
  type as typeTokens,
  space,
  motion,
} from "@/lib/design-tokens";
import { forgeStatusTones, toneStyles } from "@/components/ui/forge/status";
import { GenerationFencedDots, StateLanesBadge, ServerStateLaneBadge, NodeStateLaneBadge } from "@/components/shared/generation-fenced-dot";

// ---------------------------------------------------------------------------
// Helpers: filesystem scan for hardcoded bg-[#...]
// ---------------------------------------------------------------------------
function collectSourceFiles(root: string, exts = new Set([".ts", ".tsx"])): string[] {
  const out: string[] = [];
  function walk(dir: string) {
    if (!existsSync(dir)) return;
    for (const entry of readdirSync(dir)) {
      // skip internals
      if (entry === ".next" || entry === "node_modules" || entry === ".git" || entry === "coverage") continue;
      const full = join(dir, entry);
      const st = statSync(full);
      if (st.isDirectory()) walk(full);
      else if (exts.has(entry.slice(entry.lastIndexOf("."))) ) out.push(full);
    }
  }
  walk(root);
  return out;
}

const BLURPLE = "#5865f2"; // Discord brand — intentionally exempt per DESIGN_TOKENS.md
const BG_HARDCODE_RE = /bg-\[#([0-9a-fA-F]{3,8})\]/g;

// ---------------------------------------------------------------------------
// 1. Tokens exist (brand/canvas/line/text etc.)
// ---------------------------------------------------------------------------
describe("design tokens: semantic groups exist", () => {
  it("exports brand with var(--*) and hex mirrors", () => {
    expect(brand.DEFAULT).toBe("var(--brand)");
    expect(brand.hover).toBe("var(--brand-hover)");
    expect(brand.dark).toBe("var(--brand-dark)");
    expect(brand.subtle).toBe("var(--brand-subtle)");
    expect(brand.hex).toBe("#dc2626");
    expect(brand.hexHover).toBe("#ef4444");
    expect(brand.hexDark).toBe("#991b1b");
    expect(brand.hexSubtle).toMatch(/rgba/);
  });

  it("exports canvas surfaces with var(--*) and hex mirrors", () => {
    expect(canvas.DEFAULT).toBe("var(--canvas)");
    expect(canvas.surface).toBe("var(--surface)");
    expect(canvas.raised).toBe("var(--surface-raised)");
    expect(canvas.input).toBe("var(--surface-input)");
    expect(canvas.hover).toBe("var(--surface-hover)");
    expect(canvas.nav).toBe("var(--nav)");
    expect(canvas.hexDark).toBe("#0a0e16");
    expect(canvas.hexSurface).toBe("#111722");
    expect(canvas.hexRaised).toBe("#171f2d");
    expect(canvas.hexInput).toBe("#0d131d");
    expect(canvas.hexHover).toBe("#1a2233");
    expect(canvas.hexNav).toBe("#0f141f");
  });

  it("exports line/border tokens", () => {
    expect(line.DEFAULT).toBe("var(--line)");
    expect(line.strong).toBe("var(--line-strong)");
    expect(line.border).toBe("var(--border)");
    expect(line.borderStrong).toBe("var(--border-strong)");
    expect(line.hexLine).toMatch(/rgba/);
    expect(line.hexLineStrong).toMatch(/rgba/);
  });

  it("exports text tokens", () => {
    expect(text.DEFAULT).toBe("var(--text)");
    expect(text.subtle).toBe("var(--text-subtle)");
    expect(text.focus).toBe("var(--focus)");
  });

  it("exports status tokens (success/warning/danger)", () => {
    expect(status.success).toBe("var(--success)");
    expect(status.successSubtle).toBe("var(--success-subtle)");
    expect(status.warning).toBe("var(--warning)");
    expect(status.warningSubtle).toBe("var(--warning-subtle)");
    expect(status.danger).toBe("var(--danger)");
    expect(status.dangerSubtle).toBe("var(--danger-subtle)");
  });

  it("exports shadow and radius tokens", () => {
    expect(shadow.card).toBe("var(--shadow-card)");
    expect(shadow.elevated).toBe("var(--shadow-elevated)");
    expect(shadow.dialog).toBe("var(--shadow-dialog)");
    expect(radius.sm).toBe("var(--radius-sm)");
    expect(radius.DEFAULT).toBe("var(--radius)");
    expect(radius.lg).toBe("var(--radius-lg)");
    expect(radius.full).toBe("var(--radius-full)");
  });

  it("tokens aggregate contains all semantic groups", () => {
    expect(tokens.brand).toBe(brand);
    expect(tokens.canvas).toBe(canvas);
    expect(tokens.line).toBe(line);
    expect(tokens.text).toBe(text);
    expect(tokens.status).toBe(status);
    expect(tokens.shadow).toBe(shadow);
    expect(tokens.radius).toBe(radius);
  });

  it("legacy colors retains Industrial Terminal spec hex", () => {
    expect(colors.ink).toBe("#0B1118");
    expect(colors.steel).toBe("#1B2636");
    expect(colors.concrete).toBe("#8A9BA8");
    expect(colors.paper).toBe("#E6EDF3");
    expect(colors.phosphor).toBe("#FFB000");
    expect(colors.fault).toBe("#E63E2A");
    expect(colors.brand).toBe("#dc2626");
    expect(colors.canvas).toBe("#0a0e16");
  });

  it("the status vocabulary is the canonical seven, and every one has styles", () => {
    expect(forgeStatusTones).toEqual([
      "ok",
      "warn",
      "danger",
      "info",
      "pending",
      "neutral",
      "unknown",
    ]);
    for (const tone of forgeStatusTones) {
      expect(toneStyles[tone]).toBeDefined();
    }
  });

  it("unknown is visually distinct from every other tone", () => {
    // A reading we do not have must not look like one we do. The dashed edge
    // is what separates "no reading" from the solid chips around it.
    expect(toneStyles.unknown.chip).toContain("border-dashed");
    for (const tone of forgeStatusTones.filter((t) => t !== "unknown")) {
      expect(toneStyles[tone].chip).not.toContain("border-dashed");
    }
  });

  it("spacing uses 4pt grid", () => {
    expect(space.xs).toBe(4);
    expect(space.sm).toBe(8);
    expect(space.md).toBe(16);
    expect(space.lg).toBe(24);
    expect(space.xl).toBe(32);
  });

  it("motion and type tokens exist", () => {
    expect(motion.duration).toBe(180);
    expect(motion.easing).toBe("cubic-bezier(0.2,0,0,1)");
    expect(typeTokens.display).toBe("Space Grotesk");
    expect(typeTokens.body).toBe("IBM Plex Sans");
    expect(typeTokens.mono).toBe("JetBrains Mono");
  });

  it("all var(--*) tokens resolve to declared CSS vars in globals.css:5", () => {
    const css = readFileSync(resolve(__dirname, "../app/globals.css"), "utf8");
    // every semantic var referenced in design-tokens should be declared in :root
    const expectedVars = [
      "--brand", "--brand-hover", "--brand-dark", "--brand-subtle",
      "--canvas", "--surface", "--surface-raised", "--surface-input", "--surface-hover", "--nav",
      "--line", "--line-strong", "--border", "--border-strong",
      "--text", "--text-subtle", "--focus",
      "--success", "--success-subtle", "--warning", "--warning-subtle", "--danger", "--danger-subtle",
      "--shadow-card", "--shadow-elevated", "--shadow-dialog",
      "--radius-sm", "--radius", "--radius-lg", "--radius-full",
      "--ink", "--steel", "--concrete", "--paper", "--phosphor", "--fault",
    ];
    for (const v of expectedVars) {
      expect(css, `missing CSS var ${v} in globals.css`).toContain(`${v}:`);
    }
    // light theme block exists
    expect(css).toContain('[data-theme="light"]');
  });

  it("tailwind.config.ts maps every semantic color to var(--*) (no hardcoded hex)", () => {
    const tw = readFileSync(resolve(__dirname, "../tailwind.config.ts"), "utf8");
    // brand/canvas/line/text/success/warning/danger must map via var(--*)
    expect(tw).toContain('canvas: "var(--canvas)"');
    expect(tw).toContain('nav: "var(--nav)"');
    expect(tw).toContain('DEFAULT: "var(--surface)"');
    expect(tw).toContain('DEFAULT: "var(--border)"');
    expect(tw).toContain('DEFAULT: "var(--line)"');
    expect(tw).toContain('DEFAULT: "var(--text)"');
    expect(tw).toContain('DEFAULT: "var(--brand)"');
    expect(tw).toContain('DEFAULT: "var(--success)"');
    expect(tw).toContain('DEFAULT: "var(--warning)"');
    expect(tw).toContain('DEFAULT: "var(--danger)"');
    // legacy palette
    expect(tw).toContain('ink: "var(--ink)"');
    expect(tw).toContain('phosphor: "var(--phosphor)"');
    expect(tw).toContain('fault: "var(--fault)"');
    // shadow/radius
    expect(tw).toContain('card: "var(--shadow-card)"');
    expect(tw).toContain('sm: "var(--radius-sm)"');
    // ensure no raw bg hex slipped into tailwind config outside comments
    const hardcodedInTw = tw.match(/"#[0-9a-fA-F]{3,8}"/g) ?? [];
    // No quoted hex outside comments — tokens are all var(--*)
    expect(hardcodedInTw.length, `unexpected hardcoded hex in tailwind.config.ts: ${hardcodedInTw.join(", ")}`).toBe(0);
  });

  it("shared primitives use var(--*) not hardcoded hex for card/surface (spot-check)", () => {
    const emptySrc = readFileSync(resolve(__dirname, "../components/shared/states-empty.tsx"), "utf8");
    expect(emptySrc).toContain("border-[var(--line)]");
    expect(emptySrc).toContain("bg-[var(--surface)]");
    expect(emptySrc).toContain("bg-[var(--surface-raised)]");
    expect(emptySrc).not.toMatch(/bg-\[#(?!5865f2)/);

    const loadingSrc = readFileSync(resolve(__dirname, "../components/shared/states-loading.tsx"), "utf8");
    expect(loadingSrc).toContain("bg-[var(--surface-raised)]");
    expect(loadingSrc).toContain("border-[var(--line)]");
    expect(loadingSrc).not.toMatch(/bg-\[#(?!5865f2)/);

    const badgeSrc = readFileSync(resolve(__dirname, "../components/shared/generation-fenced-dot.tsx"), "utf8");
    expect(badgeSrc).toContain("var(--success)");
    expect(badgeSrc).toContain("var(--warning)");
    expect(badgeSrc).toContain("var(--danger)");
    expect(badgeSrc).toContain("var(--text-subtle)");
  });
});

// ---------------------------------------------------------------------------
// 2. No hardcoded bg-[#...] remaining (except blurple)
// ---------------------------------------------------------------------------
describe("no hardcoded bg-[#...] remaining (except blurple)", () => {
  it("finds zero hardcoded bg-[#...] files outside allowed exception", () => {
    const roots = [
      resolve(__dirname, "../app"),
      resolve(__dirname, "../components"),
      resolve(__dirname, "../lib"),
      resolve(__dirname, "../stores"),
      resolve(__dirname, "../hooks"),
    ];
    const violations: string[] = [];
    for (const root of roots) {
      for (const file of collectSourceFiles(root)) {
        const src = readFileSync(file, "utf8");
        // Reset regex per file
        let m: RegExpExecArray | null;
        const re = new RegExp(BG_HARDCODE_RE.source, "g");
        while ((m = re.exec(src)) !== null) {
          const full = m[0]; // e.g. bg-[#161b28]
          const hex = m[1].toLowerCase();
          if (hex === BLURPLE.replace("#", "").toLowerCase()) continue; // allowed
          // Allow globals.css and design-tokens.ts hex declarations (not bg-[#...] anyway)
          violations.push(`${relative(resolve(__dirname, ".."), file)}: ${full}`);
        }
      }
    }
    expect(violations, `hardcoded bg-[#...] found (only bg-[#${BLURPLE}] allowed):\n${violations.join("\n")}`).toEqual([]);
  });

  it("only allowed bg-[#5865f2] exists and is Discord-isolated", () => {
    const roots = [
      resolve(__dirname, "../app"),
      resolve(__dirname, "../components"),
      resolve(__dirname, "../lib"),
    ];
    const all = roots.flatMap((r) => collectSourceFiles(r));
    const hits: string[] = [];
    for (const file of all) {
      const src = readFileSync(file, "utf8");
      if (src.includes(`bg-[#${BLURPLE.slice(1)}]`) || src.includes(`bg-[${BLURPLE}]`)) {
        hits.push(relative(resolve(__dirname, ".."), file));
      }
    }
    // Expect exactly the Discord webhook file (if any) — at most 1
    expect(hits.length).toBeLessThanOrEqual(1);
    if (hits.length === 1) {
      expect(hits[0]).toContain("AdminWebhooks");
    }
  });

  it("DESIGN_TOKENS.md documents the blurple exception", () => {
    // repo root DESIGN_TOKENS.md is at forge/web/DESIGN_TOKENS.md vs project/DESIGN_TOKENS.md
    // Both document the exception; check at least one exists.
    const candidates = [
      resolve(__dirname, "../../..", "DESIGN_TOKENS.md"),
      resolve(__dirname, "../DESIGN_TOKENS.md"),
      resolve(__dirname, "../../DESIGN_TOKENS.md"),
    ].filter(existsSync);
    expect(candidates.length, "DESIGN_TOKENS.md not found").toBeGreaterThan(0);
    const doc = readFileSync(candidates[0], "utf8");
    expect(doc).toContain("5865f2");
  });
});

// ---------------------------------------------------------------------------
// 3. Space Grotesk / IBM Plex Sans / JetBrains Mono via next/font
// ---------------------------------------------------------------------------
describe("typography: next/font loading", () => {
  it("app/fonts.ts imports Manrope and JetBrains_Mono from next/font/google", () => {
    const src = readFileSync(resolve(__dirname, "../app/fonts.ts"), "utf8");
    expect(src).toContain('from "next/font/google"');
    expect(src).toContain("Manrope");
    expect(src).toContain("JetBrains_Mono");
  });

  it("fonts.ts exports sans/display/mono with correct variable names", () => {
    const src = readFileSync(resolve(__dirname, "../app/fonts.ts"), "utf8");
    expect(src).toContain('variable: "--font-sans"');
    expect(src).toContain('variable: "--font-mono"');
    expect(src).toContain("export const sans = Manrope");
    expect(src).toContain("export const mono = JetBrains_Mono");
    // weights cover body/display
    expect(src).toContain('weight: ["400", "500", "600", "700"]');
    expect(src).toContain('display: "swap"');
  });

  it("app/layout.tsx applies sans.variable display.variable mono.variable to <body>", () => {
    const src = readFileSync(resolve(__dirname, "../app/layout.tsx"), "utf8");
    expect(src).toContain('from "./fonts"');
    expect(src).toContain("sans.variable");
    expect(src).toContain("display.variable");
    expect(src).toContain("mono.variable");
    expect(src).toContain("${sans.variable} ${display.variable} ${mono.variable}");
  });

  it("globals.css uses var(--font-sans) and var(--font-mono) and --font-display", () => {
    const css = readFileSync(resolve(__dirname, "../app/globals.css"), "utf8");
    expect(css).toContain("var(--font-sans)");
    expect(css).toContain("var(--font-mono)");
    expect(css).toContain("var(--font-display");
  });

  it("tailwind.config.ts fontFamily prefers next/font vars", () => {
    const tw = readFileSync(resolve(__dirname, "../tailwind.config.ts"), "utf8");
    expect(tw).toContain('sans: ["var(--font-sans)", "Manrope"');
    expect(tw).toContain('display: ["var(--font-display)", "Space Grotesk"');
    expect(tw).toContain('mono: ["var(--font-mono)", "JetBrains Mono"');
  });

  it("DESIGN_TOKENS.md documents type scale", () => {
    const candidates = [
      resolve(__dirname, "../../..", "DESIGN_TOKENS.md"),
      resolve(__dirname, "../DESIGN_TOKENS.md"),
    ].filter(existsSync);
    if (candidates.length === 0) return;
    const doc = readFileSync(candidates[0], "utf8");
    expect(doc).toContain("Space Grotesk");
    expect(doc).toContain("IBM Plex Sans");
    expect(doc).toContain("JetBrains Mono");
  });
});

// ---------------------------------------------------------------------------
// 4. generation-fenced-dot renders two-dot badge correctly
// ---------------------------------------------------------------------------
describe("GenerationFencedDots — two-dot badge", () => {
  it("renders exactly two dots stacked vertically (desired on top, actual on bottom)", () => {
    const { container } = render(
      <GenerationFencedDots desired="running" actual="pending" generation={5} fenceGeneration={3} />
    );
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper).toBeTruthy();
    // flex-col gap-[2px] signature StateLane
    expect(wrapper.className).toContain("flex");
    expect(wrapper.className).toContain("flex-col");
    // two dot spans inside
    const dots = wrapper.querySelectorAll("span[aria-hidden]");
    expect(dots).toHaveLength(2);
    // top dot: desired=running => success
    expect(dots[0].className).toContain("bg-[var(--success)]");
    // bottom dot: actual=pending => sky, not fenced => opacity-60
    expect(dots[1].className).toContain("bg-sky-500");
    expect(dots[1].className).toContain("opacity-60");
    // when fenced, pending loses opacity and gains ring
    const { container: fenced } = render(<GenerationFencedDots desired="running" actual="pending" generation={1} fenceGeneration={5} />);
    const fencedDots = fenced.firstElementChild!.querySelectorAll("span[aria-hidden]");
    expect(fencedDots[1].className).toContain("ring-2");
    expect(fencedDots[1].className).not.toContain("opacity-60");
  });

  it("exposes aria-label and title with desired/actual + generation labels", () => {
    render(<GenerationFencedDots desired="installing" actual="failed" generation={2} fenceGeneration={2} />);
    const lane = screen.getByLabelText(/desired:installing actual:failed/);
    expect(lane).toHaveAttribute("title", expect.stringContaining("desired:installing"));
    expect(lane).toHaveAttribute("title", expect.stringContaining("actual:failed"));
    expect(lane).toHaveAttribute("title", expect.stringContaining("g2"));
  });

  it("defaults to pending when desired/actual are null", () => {
    const { container } = render(<GenerationFencedDots desired={null} actual={null} />);
    const dots = container.firstElementChild!.querySelectorAll("span[aria-hidden]");
    // both pending => sky
    expect(dots[0].className).toContain("bg-sky-500");
    expect(dots[1].className).toContain("bg-sky-500");
    expect(screen.getByLabelText(/desired:pending actual:pending/)).toBeInTheDocument();
  });

  it("respects size prop for both dots", () => {
    const { container } = render(<GenerationFencedDots desired="running" actual="running" size={12} />);
    const dots = container.firstElementChild!.querySelectorAll("span[aria-hidden]") as NodeListOf<HTMLElement>;
    expect(dots[0].style.width).toBe("12px");
    expect(dots[0].style.height).toBe("12px");
    expect(dots[1].style.width).toBe("12px");
    expect(dots[1].style.height).toBe("12px");
  });

  it("applies fenced ring to actual dot when generation < fenceGeneration", () => {
    const { container } = render(<GenerationFencedDots desired="running" actual="pending" generation={1} fenceGeneration={5} />);
    const dots = container.firstElementChild!.querySelectorAll("span[aria-hidden]");
    // second dot should have fenced ring
    expect(dots[1].className).toContain("ring-2");
    expect(dots[1].className).toContain("ring-[var(--danger)]");
    // title should contain fenced
    expect(screen.getByLabelText(/fenced/)).toBeInTheDocument();
  });

  it("isFenced=true overrides generation comparison (forces fenced ring)", () => {
    const { container } = render(<GenerationFencedDots desired="running" actual="stopped" generation={5} fenceGeneration={5} isFenced />);
    const dots = container.firstElementChild!.querySelectorAll("span[aria-hidden]");
    expect(dots[1].className).toContain("ring-2");
  });

  it("isFenced=false suppresses ring even when generation < fenceGeneration", () => {
    const { container } = render(<GenerationFencedDots desired="running" actual="stopped" generation={1} fenceGeneration={9} isFenced={false} />);
    const dots = container.firstElementChild!.querySelectorAll("span[aria-hidden]");
    expect(dots[1].className).not.toContain("ring-2");
  });

  it("showLabel renders generation label and fence indicator", () => {
    render(<GenerationFencedDots desired="running" actual="pending" generation={4} fenceGeneration={7} showLabel />);
    expect(screen.getByText(/g4/)).toBeInTheDocument();
    // fenced => shows ◉
    expect(screen.getByText(/g4.*◉/)).toBeInTheDocument();
  });

  it("maps state tones to correct Tailwind classes", () => {
    const cases: Array<[string, string]> = [
      ["running", "bg-[var(--success)]"],
      ["completed", "bg-[var(--success)]"],
      ["succeeded", "bg-[var(--success)]"],
      ["installing", "bg-[var(--warning)]"],
      ["provisioning", "bg-[var(--warning)]"],
      ["transferring", "bg-violet-500"],
      ["in_progress", "bg-violet-500"],
      ["failed", "bg-[var(--danger)]"],
      ["crashed", "bg-[var(--danger)]"],
      ["suspended", "bg-[var(--danger)]"],
      ["pending", "bg-sky-500"],
      ["queued", "bg-sky-500"],
      ["stopped", "bg-[var(--text-subtle)]"],
      ["cancelled", "bg-[var(--text-subtle)]"],
      ["unknown", "bg-[var(--text-subtle)]"],
    ];
    for (const [desired, expectedClass] of cases) {
      const { container, unmount } = render(<GenerationFencedDots desired={desired} actual={desired} />);
      const dots = container.firstElementChild!.querySelectorAll("span[aria-hidden]");
      expect(dots[0].className, `state ${desired} should map to ${expectedClass}`).toContain(expectedClass);
      unmount();
    }
  });
});

describe("StateLanesBadge — compact horizontal badge", () => {
  it("renders badge with GenerationFencedDots plus desired/actual text and generation", () => {
    render(<StateLanesBadge desired="running" actual="pending" generation={2} fenceGeneration={4} />);
    expect(screen.getByText("running / pending")).toBeInTheDocument();
    // horizontal pill
    const badge = screen.getByText("running / pending").closest("span")?.parentElement as HTMLElement;
    expect(badge.className).toContain("inline-flex");
    expect(badge.className).toContain("rounded-full");
    expect(badge.className).toContain("border-[var(--line)]");
    // still has lane inside
    expect(screen.getByLabelText(/desired:running actual:pending/)).toBeInTheDocument();
    // generation label inside badge
    expect(screen.getByText(/g2→f4/)).toBeInTheDocument();
    expect(screen.getByText(/fenced/)).toBeInTheDocument();
  });

  it("without generation shows only desired/actual", () => {
    render(<StateLanesBadge desired="stopped" actual="stopped" />);
    expect(screen.getByText("stopped / stopped")).toBeInTheDocument();
    expect(screen.queryByText(/g\d/)).not.toBeInTheDocument();
  });

  it("handles null desired/actual with em dash placeholder", () => {
    render(<StateLanesBadge desired={null} actual={null} />);
    expect(screen.getByText("— / —")).toBeInTheDocument();
  });
});

describe("ServerStateLaneBadge / NodeStateLaneBadge — aliases", () => {
  it("ServerStateLaneBadge proxies to StateLanesBadge", () => {
    render(<ServerStateLaneBadge desired="failed" actual="crashed" generation={1} fenceGeneration={2} />);
    expect(screen.getByText("failed / crashed")).toBeInTheDocument();
    expect(screen.getByLabelText(/desired:failed/)).toBeInTheDocument();
  });

  it("NodeStateLaneBadge proxies to StateLanesBadge", () => {
    render(<NodeStateLaneBadge desired="installing" actual="installing" />);
    expect(screen.getByText("installing / installing")).toBeInTheDocument();
  });
});

describe("shared states: empty/loading primitives exist and are theme-aware", () => {
  it("states-empty exports all empty variants", async () => {
    const mod = await import("@/components/shared/states-empty");
    expect(mod.EmptyList).toBeDefined();
    expect(mod.EmptySearch).toBeDefined();
    expect(mod.EmptyDeployments).toBeDefined();
    expect(mod.EmptyBackups).toBeDefined();
    expect(mod.EmptyDomains).toBeDefined();
    expect(mod.EmptyServices).toBeDefined();
    expect(mod.EmptyGit).toBeDefined();
    expect(mod.EmptyCertificates).toBeDefined();
    expect(mod.EmptyDNSProviders).toBeDefined();
    expect(mod.EmptyOrganizations).toBeDefined();
  });

  it("states-loading exports skeleton + spinner primitives", async () => {
    const mod = await import("@/components/shared/states-loading");
    expect(mod.SkeletonList).toBeDefined();
    expect(mod.SkeletonDetail).toBeDefined();
    expect(mod.SkeletonForm).toBeDefined();
    expect(mod.SpinnerInline).toBeDefined();
    expect(mod.SpinnerPage).toBeDefined();
    expect(mod.SpinnerButton).toBeDefined();
  });

  it("SkeletonList and SpinnerInline render with role=status", async () => {
    const { SpinnerInline, SkeletonList } = await import("@/components/shared/states-loading");
    const { container } = render(<SkeletonList rows={2} columns={2} />);
    expect(container.querySelector('[role="status"]')).toBeInTheDocument();
    const { container: c2 } = render(<SpinnerInline label="Loading buckets" />);
    expect(c2.querySelector('[role="status"]')).toBeInTheDocument();
    expect(screen.getByText("Loading buckets")).toBeInTheDocument();
  });

  it("EmptyList renders theme-aware dashed border card", async () => {
    const { EmptyList } = await import("@/components/shared/states-empty");
    const { container } = render(<EmptyList title="No eggs" description="Add an egg" />);
    expect(screen.getByText("No eggs")).toBeInTheDocument();
    expect(screen.getByText("Add an egg")).toBeInTheDocument();
    expect(container.firstElementChild?.className).toContain("border-dashed");
    expect(container.firstElementChild?.className).toContain("border-[var(--line)]");
  });
});
