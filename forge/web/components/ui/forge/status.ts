/**
 * Forge status vocabulary — the single source of status meaning in the UI.
 *
 * Seven states, one meaning each. The important one is `unknown`: it renders
 * grey with a dashed edge, never green and never as a zero value, because a
 * reading we do not have is not a healthy reading (AGENTS.md — "Unknown is not
 * zero, not-reported is not zero, and a stale reading is not a healthy one").
 * Anything that cannot determine a state must resolve to `unknown`, not `ok`.
 */

export const forgeStatusTones = [
  "ok",
  "warn",
  "danger",
  "info",
  "pending",
  "neutral",
  "unknown",
] as const;

export type ForgeTone = (typeof forgeStatusTones)[number];

/** Tones that must draw the operator's attention. */
export const attentionTones: readonly ForgeTone[] = ["warn", "danger", "unknown"];

type ToneStyle = {
  /** Text-only colour, for inline values and icons. */
  fg: string;
  /** Filled chip: border + background + text. */
  chip: string;
  /** Border colour on its own, for accent rails and outlined surfaces. */
  border: string;
  /** Background wash on its own. */
  bg: string;
  /** Status dot. */
  dot: string;
  /** Human label used when a caller does not supply one. */
  label: string;
};

export const toneStyles: Record<ForgeTone, ToneStyle> = {
  ok: {
    fg: "text-ok",
    chip: "border-ok-line bg-ok-subtle text-ok",
    border: "border-ok-line",
    bg: "bg-ok-subtle",
    dot: "bg-ok",
    label: "Healthy",
  },
  warn: {
    fg: "text-warn",
    chip: "border-warn-line bg-warn-subtle text-warn",
    border: "border-warn-line",
    bg: "bg-warn-subtle",
    dot: "bg-warn",
    label: "Degraded",
  },
  danger: {
    fg: "text-danger",
    chip: "border-danger-line bg-danger-subtle text-danger",
    border: "border-danger-line",
    bg: "bg-danger-subtle",
    dot: "bg-danger",
    label: "Failed",
  },
  info: {
    fg: "text-info",
    chip: "border-info-line bg-info-subtle text-info",
    border: "border-info-line",
    bg: "bg-info-subtle",
    dot: "bg-info",
    label: "Info",
  },
  pending: {
    fg: "text-info",
    chip: "border-info-line bg-info-subtle text-info",
    border: "border-info-line",
    bg: "bg-info-subtle",
    dot: "bg-info",
    label: "Pending",
  },
  neutral: {
    fg: "text-text-subtle",
    chip: "border-line bg-overlay text-text-subtle",
    border: "border-line",
    bg: "bg-overlay",
    dot: "bg-text-muted",
    label: "Inactive",
  },
  // Dashed edge so an unknown state is visually distinct from every known one
  // even in a dense grid, and never mistakable for `ok`.
  unknown: {
    fg: "text-unknown",
    chip: "border-dashed border-unknown-line bg-unknown-subtle text-unknown",
    border: "border-dashed border-unknown-line",
    bg: "bg-unknown-subtle",
    dot: "bg-unknown",
    label: "Unknown",
  },
};

/**
 * Every tone spelling a call site may pass: the canonical seven, plus the
 * meaning words (`success`) and colour words (`green`) that the existing call
 * sites across `admin-ui` and `ui/primitives` were written with. One type, so
 * the accepted vocabulary is stated in a single place.
 *
 * {@link resolveTone} additionally accepts an arbitrary string — a raw status
 * off the wire — and resolves anything it does not recognise to `unknown`. This
 * type is the narrower contract for props, so a typo is a build error rather
 * than a silently grey chip.
 */
export type ToneInput =
  | ForgeTone
  | "success"
  | "warning"
  | "error"
  | "green"
  | "red"
  | "yellow"
  | "blue";

/**
 * Accepts any tone name used historically across the app and maps it onto the
 * canonical seven. Unrecognised input resolves to `unknown` — not `neutral` and
 * not `ok` — so a tone we cannot interpret is reported as such.
 */
export function resolveTone(input: string | null | undefined): ForgeTone {
  if (!input) return "unknown";
  const key = String(input).toLowerCase().trim();
  switch (key) {
    case "ok":
    case "healthy":
    case "online":
    case "running":
    case "success":
    case "active":
    case "green":
    case "up":
    case "ready":
    case "connected":
    case "passed":
    // Beacon's per-subsystem health probes report "good".
    case "good":
      return "ok";
    case "warn":
    case "warning":
    case "degraded":
    case "yellow":
    case "amber":
    case "stale":
    case "drift":
    case "partial":
    // Beacon reports a subsystem under pressure but still serving as
    // "elevated".
    case "elevated":
      return "warn";
    case "danger":
    case "error":
    case "failed":
    case "failing":
    case "critical":
    case "offline":
    case "red":
    case "down":
    case "unhealthy":
    case "crashed":
    case "suspended":
      return "danger";
    case "info":
    case "blue":
    case "sky":
      return "info";
    case "pending":
    case "starting":
    case "stopping":
    case "installing":
    case "provisioning":
    case "restarting":
    case "queued":
    case "in_progress":
    case "progress":
      return "pending";
    case "neutral":
    case "inactive":
    case "stopped":
    case "disabled":
    case "idle":
    case "slate":
    case "gray":
    case "grey":
      return "neutral";
    default:
      return "unknown";
  }
}

/**
 * Tone for a freshness window. A reading older than `staleAfterMs` is `warn`;
 * a missing timestamp is `unknown`. Never `ok` by omission.
 */
export function freshnessTone(
  observedAt: string | number | Date | null | undefined,
  staleAfterMs = 60_000
): ForgeTone {
  if (observedAt === null || observedAt === undefined || observedAt === "") return "unknown";
  const ts = observedAt instanceof Date ? observedAt.getTime() : new Date(observedAt).getTime();
  if (!Number.isFinite(ts)) return "unknown";
  return Date.now() - ts > staleAfterMs ? "warn" : "ok";
}
