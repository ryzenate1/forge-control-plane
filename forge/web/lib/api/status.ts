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
  // Waiting is not a warning: a queued or pending deployment has nothing wrong
  // with it, so it reads in the dedicated `pending` tone rather than amber.
  queued: "pending",
  pending: "pending",
  awaiting_health: "warn",
  health_checking: "pending",
  degraded: "warn",
  rolling_back: "warn",
  rolled_back: "warn",
  updating: "pending",
  deleting: "danger",
  restoring: "pending",
  draining: "warn",
  planned: "pending",
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

// Nomad: job `Status`, allocation `ClientStatus` and node `Status` share one
// table because the admin page renders all three through one pill. Nomad emits
// these lowercase; statusTone lowercases anyway, which is itself a fix — the
// page's own copy of this map used a case-sensitive switch.
export const NOMAD_STATUS_INVENTORY: Record<string, StatusTone> = {
  running: "ok",
  successful: "ok",
  active: "ok",
  ready: "ok",
  // An intentionally stopped job reads `dead`, and a finished batch allocation
  // reads `complete`: both are observed, inactive states rather than faults.
  dead: "neutral",
  stopped: "neutral",
  complete: "neutral",
  inactive: "neutral",
  pending: "pending",
  initializing: "pending",
  // A drained or ineligible node is not broken, but it is not taking work
  // either — that needs an operator's attention, not a red alarm.
  draining: "warn",
  ineligible: "warn",
  failed: "danger",
  lost: "danger",
  unhealthy: "danger",
  // A node that stopped answering is a fault, not an idle machine.
  down: "danger",
};

// Incus: instance `status` (Running/Stopped/Frozen/Error) and cluster member
// state (Online/Offline).
export const INCUS_STATUS_INVENTORY: Record<string, StatusTone> = {
  running: "ok",
  online: "ok",
  stopped: "neutral",
  frozen: "neutral",
  // A cluster member that has gone offline is a fault. The admin page's own
  // copy of this map filed it next to Stopped, which rendered a lost member in
  // the same grey as an instance the operator had deliberately shut down.
  offline: "danger",
  error: "danger",
  failure: "danger",
};

// Managed database services: 5 states.
export const DATABASE_STATUS_INVENTORY: Record<string, StatusTone> = {
  running: "ok",
  // Stopped is a state the operator chose. The services page's own copy of this
  // map had it as danger, so every deliberately stopped database rendered in
  // the same red as one that had crashed.
  stopped: "neutral",
  failed: "danger",
  provisioning: "pending",
  deleting: "danger",
};

// Service discovery: endpoint health as the reaper and beacon heartbeats report
// it. `unknown` is a first-class value here — discovery genuinely does not know
// an endpoint's health until a heartbeat touches it — so it must keep the
// unknown treatment rather than collapsing into the neutral "inactive" chip.
export const DISCOVERY_STATUS_INVENTORY: Record<string, StatusTone> = {
  healthy: "ok",
  unhealthy: "danger",
  unknown: "unknown",
  // A draining endpoint is being taken out of rotation deliberately, but it is
  // not serving either: the operator needs to see it, not be alarmed by it.
  draining: "warn",
};

export type StatusKind =
  | "app"
  | "deployment"
  | "compose"
  | "preview"
  | "source"
  | "build"
  | "server-deployment"
  | "nomad"
  | "incus"
  | "database"
  | "discovery";

const KIND_TABLES: Record<StatusKind, ReadonlyArray<Record<string, StatusTone>>> = {
  app: [APP_STATUS_TONE, DEPLOYMENT_STATUS_TONE],
  deployment: [DEPLOYMENT_STATUS_TONE, APP_STATUS_TONE],
  compose: [COMPOSE_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE, APP_STATUS_TONE],
  preview: [PREVIEW_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE],
  source: [SOURCE_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE],
  build: [BUILD_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE],
  "server-deployment": [SERVER_DEPLOYMENT_INVENTORY, DEPLOYMENT_STATUS_TONE],
  nomad: [NOMAD_STATUS_INVENTORY, DEPLOYMENT_STATUS_TONE],
  incus: [INCUS_STATUS_INVENTORY, APP_STATUS_TONE],
  database: [DATABASE_STATUS_INVENTORY, APP_STATUS_TONE],
  discovery: [DISCOVERY_STATUS_INVENTORY, APP_STATUS_TONE],
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

/** Nomad job, allocation and node statuses. */
export function nomadStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "nomad");
}

/** Incus instance statuses and cluster member state. */
export function incusStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "incus");
}

/** Service-discovery endpoint health. */
export function discoveryStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "discovery");
}

/** Managed database service statuses. */
export function databaseStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "database");
}

/** @deprecated Same as {@link statusTone}; kept for existing call sites. */
export function statusPillTone(status: string | null | undefined, kind: StatusKind = "app"): StatusPillTone {
  return statusTone(status, kind);
}
