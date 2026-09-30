// Canonical version source for the Web UI.
// Build-time value comes from NEXT_PUBLIC_APP_VERSION which the Dockerfile sets
// from the repo root VERSION file (same source that forge/api and beacon use).
// Runtime source of truth remains the API's /health version, but the static
// build carries this so the about/setup pages can show the panel version even
// before the API is reachable.

export const APP_VERSION: string =
  process.env.NEXT_PUBLIC_APP_VERSION?.trim() || "dev";

// Helper for display: omit "dev" noise in production UI unless needed.
export function displayVersion(v: string = APP_VERSION): string {
  if (!v || v === "dev" || v === "beacon-dev") return "dev";
  return v.startsWith("v") ? v : `v${v}`;
}
