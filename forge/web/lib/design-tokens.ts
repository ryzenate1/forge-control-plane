/**
 * Forge Design Tokens — typed re-export of globals.css semantic tokens for JS/TS consumers.
 *
 * Canonical source: `forge/web/app/globals.css:5` (:root + [data-theme="light"]).
 * Do not hardcode hex/rgba elsewhere — use `var(--*)` in CSS or these constants in JS (charts, timelines, canvas).
 * Tailwind maps via `tailwind.config.ts:theme.extend.colors` → `var(--*)`.
 *
 * @see audits/FORGE_IMPLEMENTATION_PLAN.md §1 — Industrial Terminal palette
 * @see audits/implementation-plan/subagent-10-orchestration-security-ux.md §10 — token spec
 */

// --brand / --brand-hover / --brand-dark / --brand-subtle
export const brand = {
  DEFAULT: "var(--brand)",
  hover: "var(--brand-hover)",
  dark: "var(--brand-dark)",
  subtle: "var(--brand-subtle)",
  hex: "#dc2626",
  hexHover: "#ef4444",
  hexDark: "#991b1b",
  hexSubtle: "rgba(220, 38, 38, 0.10)",
} as const;

// --canvas / --surface / --surface-raised / --surface-input / --surface-hover / --nav
export const canvas = {
  DEFAULT: "var(--canvas)",
  surface: "var(--surface)",
  raised: "var(--surface-raised)",
  input: "var(--surface-input)",
  hover: "var(--surface-hover)",
  nav: "var(--nav)",
  hexDark: "#0a0e16",
  hexSurface: "#111722",
  hexRaised: "#171f2d",
  hexInput: "#0d131d",
  hexHover: "#1a2233",
  hexNav: "#0f141f",
} as const;

// --line / --line-strong / --border / --border-strong / --text / --text-subtle / --focus
export const line = {
  DEFAULT: "var(--line)",
  strong: "var(--line-strong)",
  border: "var(--border)",
  borderStrong: "var(--border-strong)",
  hexLine: "rgba(148, 163, 184, 0.14)",
  hexLineStrong: "rgba(148, 163, 184, 0.25)",
} as const;

export const text = {
  DEFAULT: "var(--text)",
  subtle: "var(--text-subtle)",
  focus: "var(--focus)",
} as const;

// --success / --success-subtle / --warning / --warning-subtle / --danger / --danger-subtle
export const status = {
  success: "var(--success)",
  successSubtle: "var(--success-subtle)",
  warning: "var(--warning)",
  warningSubtle: "var(--warning-subtle)",
  danger: "var(--danger)",
  dangerSubtle: "var(--danger-subtle)",
} as const;

// --shadow-* / --radius-*
export const shadow = {
  card: "var(--shadow-card)",
  elevated: "var(--shadow-elevated)",
  dialog: "var(--shadow-dialog)",
} as const;

export const radius = {
  sm: "var(--radius-sm)",
  DEFAULT: "var(--radius)",
  lg: "var(--radius-lg)",
  full: "var(--radius-full)",
} as const;

/**
 * Chart / SVG palette.
 *
 * SVG presentation attributes (`fill`, `stroke`, `stopColor`) and canvas/terminal
 * config objects are *not* CSS contexts — `var(--token)` does not resolve there,
 * so every chart series color has to come from a literal. This is the single
 * sanctioned place for those literals; components must import from here instead
 * of writing hex inline (see the `bg-[#...]` scan in test/design-system.test.tsx).
 * Each mirror below names the CSS token it tracks where one exists.
 */
export const chart = {
  /** Axis labels/ticks — mirrors `--text-muted`. */
  axis: "#64748b",
  /** Stronger axis stroke (admin overview charts). */
  axisStrong: "#475569",
  /** Plot chrome (tooltip / crosshair panel) — mirrors the dark canvas family. */
  panel: "#0f172a",
  /** Series ramp used by the server resource charts. */
  cpu: "#3b82f6",
  memory: "#10b981",
  disk: "#f59e0b",
  networkIn: "#8b5cf6",
  networkOut: "#06b6d4",
  /** Broader categorical ramp (admin monitoring / overview multi-series charts). */
  sky: "#38bdf8",
  blue: "#0ea5e9",
  violet: "#a855f7",
  orange: "#f97316",
  lightOrange: "#fb923c",
  lightCyan: "#22d3ee",
  lightEmerald: "#34d399",
  lightViolet: "#c084fc",
  indigo: "#6366f1",
  /** Status hex mirrors for SVG attributes. */
  success: "#10b981",
  warning: "#f59e0b",
  dangerBright: "#ef4444",
  dangerSoft: "#f87171",
  critical: "#f43f5e",
  onColor: "#ffffff",
} as const;

/** Categorical swatches offered by the environment colour picker (persisted data values). */
export const environmentColorChoices = [
  "#6366f1",
  "#22c55e",
  "#f59e0b",
  "#ef4444",
  "#8b5cf6",
  "#06b6d4",
  "#ec4899",
  "#64748b",
] as const;

/** Shared xterm theme (server console + admin terminal) — xterm needs literals. */
export const terminalTheme = {
  background: "#020617",
  foreground: "#f1f5f9",
  cursor: "#94a3b8",
  black: "#0f172a",
  red: "#ef4444",
  green: "#22c55e",
  yellow: "#eab308",
  blue: "#3b82f6",
  magenta: "#a855f7",
  cyan: "#06b6d4",
  white: "#cbd5e1",
  brightBlack: "#475569",
  brightRed: "#f87171",
  brightGreen: "#4ade80",
  brightYellow: "#facc15",
  brightBlue: "#60a5fa",
  brightMagenta: "#c084fc",
  brightCyan: "#22d3ee",
  brightWhite: "#f8fafc",
} as const;

/** 10 semantic token groups (matches globals.css:5 `:root` — brand/canvas/line/text/status + nav/surface-hover/brand-subtle + shadow/radius). */
export const tokens = {
  brand,
  canvas,
  line,
  text,
  status,
  shadow,
  radius,
} as const;

// Legacy phosphor-named aliases (plan spec: FORGE_IMPLEMENTATION_PLAN.md §1)
// Kept for reference; prefer `tokens` above for new code.
export const colors = {
  ink: "#0B1118",
  steel: "#1B2636",
  concrete: "#8A9BA8",
  paper: "#E6EDF3",
  phosphor: "#FFB000",
  fault: "#E63E2A",
  amber700: "#B45309",
  // canonical hex (current globals.css values — use var() at runtime)
  brand: "#dc2626",
  canvas: "#0a0e16",
  surface: "#111722",
  surfaceRaised: "#171f2d",
} as const;

export const type = {
  display: "Space Grotesk",
  body: "IBM Plex Sans",
  mono: "JetBrains Mono",
} as const;

export const space = {
  xs: 4,
  sm: 8,
  md: 16,
  lg: 24,
  xl: 32,
} as const;

export const motion = {
  duration: 180,
  easing: "cubic-bezier(0.2,0,0,1)",
} as const;

/** Single source for status tone mapping (mirrors `lib/api/status.ts:46` `statusTone`). */
export const statusTones = [
  "neutral",
  "success",
  "warning",
  "danger",
  "info",
  "blue",
] as const;

export type StatusTone = (typeof statusTones)[number];
