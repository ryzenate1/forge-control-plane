/**
 * Centralized status tone mapping.
 * Single source of truth for App and Deployment status -> Pill tone.
 * Previously duplicated across:
 *  - lib/api/apps.ts (statusTone, deploymentStatusTone)
 *  - components/server/* (inline maps)
 *  - components/admin/AdminOperations, AdminMigrations
 *  - app/server/[id]/database, etc.
 *  - forge/web/app/admin/deployments/* (pending/in_progress/completed/failed/rolled_back)
 *  - forge/web/app/admin/deployments/history (pending/running/done/error/cancelled)
 *  - forge/web/app/admin/preview-deployments (deploying/running/stopped/failed/cleaned_up)
 *  - forge/web/app/admin/source-deployments (pending/queued/cloning/building/pushing/deploying/healthy/completed/failed/canceled/unhealthy)
 *  - forge/web/app/admin/compose (running/deploying/awaiting_health/stopped/degraded/failed/updating/deleting/deleted) — 9 states inventoried
 *  - forge/web/components/server/builds-view (pending/running/succeeded/failed/canceled)
 *  - forge/web/components/server/deployments-view (pending/building/deploying/health_checking/live/rolled_back/failed)
 *
 * Consolidation per Phase 03 + Phase 06 (Deploy/Git beautify) finding: single statusTone + single deploymentStatusTone, eliminate per-file drift.
 */

export type StatusTone = "green" | "red" | "yellow" | "blue" | "neutral";
export type StatusPillTone = "neutral" | "success" | "warning" | "danger" | "info";

// Canonical tone maps
const APP_STATUS_TONE: Record<string, StatusTone> = {
  running: "green",
  stopped: "neutral",
  deploying: "blue",
  pending: "blue",
  installing: "blue",
  starting: "blue",
  restarting: "blue",
  stopping: "yellow",
  failed: "red",
};

const DEPLOYMENT_STATUS_TONE: Record<string, StatusTone> = {
  completed: "green",
  done: "green",
  succeeded: "green",
  success: "green",
  healthy: "green",
  live: "green",
  active: "green",
  failed: "red",
  error: "red",
  unhealthy: "red",
  canceled: "neutral",
  cancelled: "neutral",
  cleaned_up: "neutral",
  deleted: "neutral",
  superseded: "neutral",
  skipped: "neutral",
  stopped: "neutral",
  running: "blue",
  in_progress: "blue",
  building: "blue",
  deploying: "blue",
  cloning: "blue",
  pushing: "blue",
  queued: "yellow",
  pending: "yellow",
  awaiting_health: "yellow",
  health_checking: "blue",
  degraded: "yellow",
  rolling_back: "yellow",
  rolled_back: "yellow",
  updating: "blue",
  deleting: "red",
  restoring: "blue",
  draining: "yellow",
  planned: "yellow",
};

// Compose 9 states inventoried (forge/web/app/admin/compose/page.tsx:36) — explicit subset for docs, but DEPLOYMENT map above already covers them.
// Kept as separate const for inventory reference (do not duplicate elsewhere).
export const COMPOSE_STATUS_INVENTORY: Record<string, StatusTone> = {
  running: "green",
  deploying: "blue",
  awaiting_health: "yellow",
  stopped: "neutral",
  degraded: "yellow",
  failed: "red",
  updating: "blue",
  deleting: "red",
  deleted: "neutral",
};

// Preview 5 states (forge/web/app/admin/preview-deployments/page.tsx:45)
export const PREVIEW_STATUS_INVENTORY: Record<string, StatusTone> = {
  deploying: "blue",
  running: "green",
  stopped: "yellow",
  failed: "red",
  cleaned_up: "neutral",
};

// Source 11 states (forge/web/app/admin/source-deployments/*)
export const SOURCE_STATUS_INVENTORY: Record<string, StatusTone> = {
  pending: "yellow",
  queued: "yellow",
  cloning: "blue",
  building: "blue",
  pushing: "blue",
  deploying: "blue",
  healthy: "green",
  completed: "green",
  failed: "red",
  canceled: "neutral",
  unhealthy: "red",
};

// Build 5 states (forge/web/components/server/builds-view.tsx:21)
export const BUILD_STATUS_INVENTORY: Record<string, StatusTone> = {
  pending: "yellow",
  running: "blue",
  succeeded: "green",
  failed: "red",
  canceled: "neutral",
};

// Server deployments-view 7 states
export const SERVER_DEPLOYMENT_INVENTORY: Record<string, StatusTone> = {
  pending: "neutral",
  building: "blue",
  deploying: "blue",
  health_checking: "blue",
  live: "green",
  rolled_back: "yellow",
  failed: "red",
};

/** Pill (admin-ui Pill) -> StatusPill (ui/primitives StatusPill) tone adapter */
export function pillToneToStatusPillTone(tone: StatusTone): StatusPillTone {
  switch (tone) {
    case "green": return "success";
    case "red": return "danger";
    case "yellow": return "warning";
    case "blue": return "info";
    default: return "neutral";
  }
}

/**
 * Unified statusTone: resolves tone for either app or deployment statuses.
 * Extended in Phase 06 to cover compose/preview/source/build/server-deployment vocabularies so no per-file map diverges.
 * @param status - raw status string (case-insensitive trimming handled by caller)
 * @param kind - hint for disambiguation; defaults to app (covers deployment fallthrough)
 */
export function statusTone(status: string | null | undefined, kind: "app" | "deployment" | "compose" | "preview" | "source" | "build" | "server-deployment" = "app"): StatusTone {
  if (typeof status !== "string") return "neutral";
  const normalized = status.trim().toLowerCase();
  if (!normalized) return "neutral";
  // Kind-specific overrides first where semantics diverge (e.g., preview deploying neutral vs blue historically — now unified to blue)
  if (kind === "compose") return COMPOSE_STATUS_INVENTORY[normalized] ?? DEPLOYMENT_STATUS_TONE[normalized] ?? APP_STATUS_TONE[normalized] ?? "neutral";
  if (kind === "preview") return PREVIEW_STATUS_INVENTORY[normalized] ?? DEPLOYMENT_STATUS_TONE[normalized] ?? "neutral";
  if (kind === "source") return SOURCE_STATUS_INVENTORY[normalized] ?? DEPLOYMENT_STATUS_TONE[normalized] ?? "neutral";
  if (kind === "build") return BUILD_STATUS_INVENTORY[normalized] ?? DEPLOYMENT_STATUS_TONE[normalized] ?? "neutral";
  if (kind === "server-deployment") return SERVER_DEPLOYMENT_INVENTORY[normalized] ?? DEPLOYMENT_STATUS_TONE[normalized] ?? "neutral";
  if (kind === "deployment") {
    return DEPLOYMENT_STATUS_TONE[normalized] ?? APP_STATUS_TONE[normalized] ?? "neutral";
  }
  // app fallthrough covers deployment-like values gracefully
  return APP_STATUS_TONE[normalized] ?? DEPLOYMENT_STATUS_TONE[normalized] ?? "neutral";
}

/** Backward-compatible alias for deployment statuses */
export function deploymentStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "deployment");
}

/** Alias for app statuses (explicit) */
export function appStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "app");
}

/** Compose helper — thin wrapper so call sites read compose semantics but still single source */
export function composeStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "compose");
}

/** Preview helper */
export function previewStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "preview");
}

/** Source helper */
export function sourceStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "source");
}

/** Build helper (Pill tone; map to StatusPill via pillToneToStatusPillTone) */
export function buildStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "build");
}

/** Server deployment helper */
export function serverDeploymentStatusTone(status: string | null | undefined): StatusTone {
  return statusTone(status, "server-deployment");
}

/** Unified StatusPill tone (success/warning/danger/info/neutral) — uses same single source, just adapted */
export function statusPillTone(status: string | null | undefined, kind: "app" | "deployment" | "compose" | "preview" | "source" | "build" | "server-deployment" = "app"): StatusPillTone {
  return pillToneToStatusPillTone(statusTone(status, kind));
}
