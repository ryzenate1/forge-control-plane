/**
 * Status string → tone.
 *
 * This is the one place a raw status value off the wire is turned into a tone.
 * The tone vocabulary itself is not defined here — it is `ForgeTone` from
 * `components/ui/forge/status.ts`, the single source of status meaning. This
 * file owns only the mapping tables: which of the ~60 status strings Forge's
 * domains emit means healthy, in-flight, degraded or failed.
 *
 * Two rules to keep in mind when adding a status:
 *
 *   - An unrecognised, empty or missing status resolves to `"unknown"`, never
 *     to `"neutral"`. Neutral is a *reading*: it means inactive, stopped,
 *     cancelled — a state we observed. Unknown means we have no reading. They
 *     must not be collapsed, or a status we cannot interpret would render as a
 *     confident "inactive".
 *   - `"pending"` is for work in flight (deploying, building, restoring);
 *     `"warn"` is for a state that needs attention (degraded, queued too long,
 *     rolled back). Both are amber-ish families visually, but they mean
 *     different things to an operator.
 *
 * Previously duplicated across lib/api/apps.ts, components/server/*,
 * components/admin/AdminOperations, AdminMigrations, and the admin
 * deployments / preview-deployments / source-deployments / compose pages.
 */

import { resolveTone, type ForgeTone } from "@/components/ui/forge/status";

/**
 * The tone vocabulary. An alias of {@link ForgeTone} so there is exactly one
 * set of tone names in the app; kept under this name because a large number of
 * call sites import `StatusTone` from here.
 */
export type StatusTone = ForgeTone;

/** @deprecated Identical to {@link StatusTone}. Both pill components now take the same tones. */
export type StatusPillTone = ForgeTone;

// Canonical tone maps
const APP_STATUS_TONE: Record<string, StatusTone> = {
  running: "ok",
  stopped: "neutral",
  deploying: "pending",
  pending: "pending",
  installing: "pending",
  starting: "pending",
  restarting: "pending",
  stopping: "warn",
  failed: "danger",
  crashed: "danger",
  suspended: "danger",
};

const DEPLOYMENT_STATUS_TONE: Record<string, StatusTone> = {
  completed: "ok",
  done: "ok",
  succeeded: "ok",
  success: "ok",
  healthy: "ok",
  live: "ok",
  active: "ok",
  failed: "danger",
  error: "danger",
  unhealthy: "danger",
  canceled: "neutral",
  cancelled: "neutral",
  cleaned_up: "neutral",
  deleted: "neutral",
  superseded: "neutral",
  skipped: "neutral",
  stopped: "neutral",
  running: "pending",
  in_progress: "pending",
  building: "pending",
  deploying: "pending",
  cloning: "pending",
  pushing: "pending",
  queued: "warn",
  pending: "warn",
  awaiting_health: "warn",
  health_checking: "pending",
  degraded: "warn",
  rolling_back: "warn",
  rolled_back: "warn",
  updating: "pending",
  deleting: "danger",
  restoring: "pending",
  draining: "warn",
  planned: "warn",
};

// Compose: the 9 inventoried states. DEPLOYMENT_STATUS_TONE already covers
// them; this stays a separate table only because compose's `deleting` and
// `stopped` differ in emphasis from the deployment defaults.
export const COMPOSE_STATUS_INVENTORY: Record<string, StatusTone> = {
  running: "ok",
  deploying: "pending",
  awaiting_health: "warn",
  stopped: "neutral",
  degraded: "warn",
  failed: "danger",
  updating: "pending",
  deleting: "danger",
  deleted: "neutral",
};

// Preview deployments: 5 states.
export const PREVIEW_STATUS_INVENTORY: Record<string, StatusTone> = {
  deploying: "pending",
  running: "ok",
  stopped: "warn",
  failed: "danger",
  cleaned_up: "neutral",
};

// Source deployments: 11 states.
export const SOURCE_STATUS_INVENTORY: Record<string, StatusTone> = {
  pending: "warn",
  queued: "warn",
  cloning: "pending",
  building: "pending",
  pushing: "pending",
  deploying: "pending",
  healthy: "ok",
  completed: "ok",
  failed: "danger",
  canceled: "neutral",
  unhealthy: "danger",
};

// Builds: 5 states.
export const BUILD_STATUS_INVENTORY: Record<string, StatusTone> = {
  pending: "warn",
  running: "pending",
  succeeded: "ok",
  failed: "danger",
  canceled: "neutral",
};

// Server deployments view: 7 states.
export const SERVER_DEPLOYMENT_INVENTORY: Record<string, StatusTone> = {
  pending: "neutral",
  building: "pending",
  deploying: "pending",
  health_checking: "pending",
  live: "ok",
  rolled_back: "warn",
  failed: "danger",
};

export type StatusKind =
  | "app"
  | "deployment"
  | "compose"
  | "preview"
  | "source"
  | "build"
  | "server-deployment";

const KIND_TABLES: Record<StatusKind, ReadonlyArray<Record<string, StatusTone>>> = {
  app: [APP_STATUS_TONE, DEPLOYMENT_STATUS_TONE],
  deployment: [DEPLOYMENT_STATUS_TONE, APP_STATUS_TONE],
  compose: [COMPOSE_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE, APP_STATUS_TONE],
  preview: [PREVIEW_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE],
  source: [SOURCE_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE],
  build: [BUILD_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE],
  "server-deployment": [SERVER_DEPLOYMENT_INVENTORY, DEPLOYMENT_STATUS_TONE],
};

/**
 * Tone for a status string in a given domain.
 *
 * Domain tables are consulted first, then the shared deployment/app tables. A
 * status none of them know falls through to {@link resolveTone}, which handles
 * the generic vocabulary (`online`, `degraded`, …) and yields `"unknown"` for
 * anything it cannot interpret — so a new backend status shows up as unknown
 * rather than quietly rendering as inactive.
 */
export function statusTone(status: string | null | undefined, kind: StatusKind = "app"): StatusTone {
  if (typeof status !== "string") return "unknown";
  const normalized = status.trim().toLowerCase();
  if (!normalized) return "unknown";
  for (const table of KIND_TABLES[kind]) {
    const tone = table[normalized];
    if (tone) return tone;
  }
  return resolveTone(normalized);
}

/**
 * @deprecated Both pill components take {@link StatusTone} directly now; this
 * is the identity function and exists only so older call sites keep compiling.
 */
export function pillToneToStatusPillTone(tone: StatusTone): StatusPillTone {
  return tone;
}

/** Deployment statuses. */
export function deploymentStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "deployment");
}

/** App statuses (explicit). */
export function appStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "app");
}

/** Compose stack and service statuses. */
export function composeStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "compose");
}

/** Preview deployment statuses. */
export function previewStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "preview");
}

/** Source deployment statuses. */
export function sourceStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "source");
}

/** Build statuses. */
export function buildStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "build");
}

/** Server deployment statuses. */
export function serverDeploymentStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "server-deployment");
}

/** @deprecated Same as {@link statusTone}; kept for existing call sites. */
export function statusPillTone(status: string | null | undefined, kind: StatusKind = "app"): StatusPillTone {
  return statusTone(status, kind);
}
